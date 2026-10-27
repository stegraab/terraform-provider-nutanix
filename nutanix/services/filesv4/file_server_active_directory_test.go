package filesv4

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	filesClient "github.com/terraform-providers/terraform-provider-nutanix/nutanix/sdks/v4/files"
)

func adTestValues(t *testing.T, protocol string) []interface{} {
	t.Helper()
	d := schema.TestResourceDataRaw(t, ResourceNutanixFileServerV2().Schema, map[string]interface{}{
		"directory_service": []interface{}{map[string]interface{}{
			"local_domain": "NFS",
			"ad_domain": []interface{}{map[string]interface{}{
				"domain_name": "ad.example.com", "protocol_type": protocol,
				"username": "join@ad.example.com", "password": "test-only-secret",
				"organization_unit":           "OU=Files,DC=ad,DC=example,DC=com",
				"preferred_domain_controller": "dc1.ad.example.com",
			}},
		}},
	})
	return d.Get("directory_service").([]interface{})
}

func TestActiveDirectoryPayloadKeepsUnmanagedNFS(t *testing.T) {
	payload, err := buildDirectoryServicePayload(adTestValues(t, "SMB"))
	if err != nil {
		t.Fatal(err)
	}
	if payload["localDomain"] != "NFS" || payload["nfsVersion"] != "NFSV3V4" {
		t.Fatalf("NFS settings changed: %#v", payload)
	}
	ad := payload["adDomain"].(map[string]interface{})
	if ad["protocolType"] != "SMB" || ad["domainName"] != "ad.example.com" || ad["organizationUnit"] != "OU=Files,DC=ad,DC=example,DC=com" {
		t.Fatalf("unexpected join settings: %#v", ad)
	}
	credential := ad["credential"].(map[string]interface{})
	if credential["username"] != "join@ad.example.com" || credential["password"] != "test-only-secret" {
		t.Fatal("join credentials missing")
	}
	for _, name := range []string{"shouldOverwriteUserAccount", "shouldAddUserAsFsAdmin", "shouldUseSameCredentialsForDns", "shouldForceLeaveDomain"} {
		if ad[name] != false {
			t.Fatalf("unsafe default for %s", name)
		}
	}
}

func TestActiveDirectoryRejectsProtocolOverlap(t *testing.T) {
	for _, protocol := range []string{"NFS", "SMB_NFS"} {
		_, err := buildDirectoryServicePayload(adTestValues(t, protocol))
		if err == nil || !strings.Contains(err.Error(), "overlaps") {
			t.Fatalf("%s: expected overlap error, got %v", protocol, err)
		}
	}
	values := adTestValues(t, "SMB")
	values[0].(map[string]interface{})["ldap_domain"] = []interface{}{map[string]interface{}{
		"protocol_type": "SMB", "servers": []interface{}{"ldap://ldap.example.com"}, "base_dn": "DC=example,DC=com",
	}}
	if _, err := buildDirectoryServicePayload(values); err == nil {
		t.Fatal("LDAP/AD overlap accepted")
	}
}

func TestActiveDirectoryRefreshPreservesInputsWithoutTrustingRemoteSecrets(t *testing.T) {
	values := adTestValues(t, "SMB")
	root := values[0].(map[string]interface{})
	previous := configuredActiveDirectory(values)
	remote, err := buildDirectoryServicePayload(values)
	if err != nil {
		t.Fatal(err)
	}
	remoteAD := remote["adDomain"].(map[string]interface{})
	delete(remoteAD, "credential")
	// Join options can be absent after the operation has completed.
	for _, key := range activeDirectoryInputFields {
		delete(remoteAD, key)
	}
	remoteAD["credential"] = map[string]interface{}{"username": "REDACTED", "password": "REDACTED"}
	flat := flattenDirectoryService(remote)
	preserveActiveDirectoryInputs(flat, values)
	d := schema.TestResourceDataRaw(t, ResourceNutanixFileServerV2().Schema, nil)
	if err := d.Set("directory_service", flat); err != nil {
		t.Fatal(err)
	}
	if got := d.Get("directory_service").([]interface{})[0]; !reflect.DeepEqual(got, root) {
		t.Fatalf("refresh changed configured state: got %#v, want %#v", got, root)
	}
	if previous["password"] != "test-only-secret" {
		t.Fatal("original credentials changed")
	}
}

func TestActiveDirectoryRefreshTracksRemoteDomainChanges(t *testing.T) {
	flat := flattenDirectoryService(map[string]interface{}{"localDomain": "NFS", "adDomain": map[string]interface{}{
		"domainName": "other.example.com", "protocolType": "SMB",
	}})
	preserveActiveDirectoryInputs(flat, adTestValues(t, "SMB"))
	if got := flat[0]["ad_domain"].([]map[string]interface{})[0]["domain_name"]; got != "other.example.com" {
		t.Fatalf("remote drift hidden: %v", got)
	}
}

func TestActiveDirectoryExplicitLeaveRefresh(t *testing.T) {
	flat := flattenDirectoryService(map[string]interface{}{"localDomain": "NFS", "nfsVersion": "NFSV3V4"})
	preserveActiveDirectoryInputs(flat, adTestValues(t, "NONE"))
	if flat[0]["ad_domain"].([]map[string]interface{})[0]["protocol_type"] != "NONE" {
		t.Fatal("completed leave must preserve NONE sentinel")
	}
	joinedFlat := flattenDirectoryService(map[string]interface{}{"localDomain": "NFS"})
	preserveActiveDirectoryInputs(joinedFlat, adTestValues(t, "SMB"))
	if _, exists := joinedFlat[0]["ad_domain"]; exists {
		t.Fatal("remote AD removal was hidden")
	}
}

func TestPreferredDomainControllerValidation(t *testing.T) {
	for _, value := range []string{"10.0.0.1", "::1", "dc1", "https://dc1.example.com", "dc1.example.com:389"} {
		if _, errs := validatePreferredDomainController(value, "dc"); len(errs) == 0 {
			t.Fatalf("accepted %q", value)
		}
	}
	for _, value := range []string{"", "dc1.example.com", "dc1.example.com."} {
		if _, errs := validatePreferredDomainController(value, "dc"); len(errs) != 0 {
			t.Fatalf("rejected %q: %v", value, errs)
		}
	}
}

func TestActiveDirectorySchemaUpdatesInPlace(t *testing.T) {
	ds := ResourceNutanixFileServerV2().Schema["directory_service"]
	if ds.ForceNew {
		t.Fatal("directory changes must not replace file server")
	}
	ad := ds.Elem.(*schema.Resource).Schema["ad_domain"]
	if ad.ForceNew {
		t.Fatal("AD changes must not replace file server")
	}
	for key, field := range ad.Elem.(*schema.Resource).Schema {
		if field.ForceNew {
			t.Fatalf("%s forces replacement", key)
		}
	}
	if !ad.Elem.(*schema.Resource).Schema["password"].Sensitive {
		t.Fatal("AD password must be sensitive")
	}
}

func testDirectoryClient(t *testing.T, handler http.HandlerFunc) *filesClient.ApiClient {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	u, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(u.Port())
	if err != nil {
		t.Fatal(err)
	}
	return &filesClient.ApiClient{Host: u.Hostname(), Port: port, Scheme: "http"}
}

func TestActiveDirectoryUpdateUsesStableAPIAndETag(t *testing.T) {
	var updated bool
	client := testDirectoryClient(t, func(w http.ResponseWriter, r *http.Request) {
		base := "/api/files/v4.0/config/file-servers/fs/directory-services"
		switch {
		case r.Method == "GET" && r.URL.Path == base:
			fmt.Fprint(w, `{"data":[{"extId":"ds"}]}`)
		case r.Method == "GET" && r.URL.Path == base+"/ds":
			w.Header().Set("Etag", "12")
			fmt.Fprint(w, `{"data":{"extId":"ds","localDomain":"NFS","nfsVersion":"NFSV3V4"}}`)
		case r.Method == "PUT" && r.URL.Path == base+"/ds":
			if r.Header.Get("If-Match") != "12" {
				t.Error("missing ETag")
			}
			var payload map[string]interface{}
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Error(err)
			}
			if payload["localDomain"] != "NFS" || payload["adDomain"].(map[string]interface{})["protocolType"] != "SMB" {
				t.Error("wrong protocol configuration")
			}
			updated = true
			fmt.Fprint(w, `{}`)
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(404)
		}
	})
	if err := updateDirectoryService(context.Background(), client, "fs", adTestValues(t, "SMB"), time.Minute); err != nil {
		t.Fatal(err)
	}
	if !updated {
		t.Fatal("no update made")
	}
}

func TestActiveDirectoryRemovalAndDomainSwitchRequireExplicitLeave(t *testing.T) {
	for _, action := range []string{"remove", "switch", "leave"} {
		t.Run(action, func(t *testing.T) {
			writes := 0
			client := testDirectoryClient(t, func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "GET" {
					writes++
					fmt.Fprint(w, `{}`)
					return
				}
				if strings.HasSuffix(r.URL.Path, "/ds") {
					fmt.Fprint(w, `{"data":{"extId":"ds","localDomain":"NFS","adDomain":{"domainName":"ad.example.com","protocolType":"SMB"}}}`)
				} else {
					fmt.Fprint(w, `{"data":[{"extId":"ds"}]}`)
				}
			})
			values := adTestValues(t, "SMB")
			switch action {
			case "remove":
				delete(values[0].(map[string]interface{}), "ad_domain")
			case "switch":
				configuredActiveDirectory(values)["domain_name"] = "other.example.com"
			case "leave":
				configuredActiveDirectory(values)["protocol_type"] = "NONE"
			}
			err := updateDirectoryService(context.Background(), client, "fs", values, time.Minute)
			if action == "leave" {
				if err != nil || writes != 1 {
					t.Fatalf("leave failed: writes=%d err=%v", writes, err)
				}
			} else if err == nil || writes != 0 {
				t.Fatalf("unsafe change allowed: writes=%d err=%v", writes, err)
			}
		})
	}
}

func TestActiveDirectoryAPIErrorDoesNotEchoPassword(t *testing.T) {
	client := testDirectoryClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "PUT" {
			w.WriteHeader(400)
			fmt.Fprint(w, `{"error":"test-only-secret"}`)
			return
		}
		if strings.HasSuffix(r.URL.Path, "/ds") {
			fmt.Fprint(w, `{"data":{"extId":"ds","localDomain":"NFS"}}`)
		} else {
			fmt.Fprint(w, `{"data":[{"extId":"ds"}]}`)
		}
	})
	err := updateDirectoryService(context.Background(), client, "fs", adTestValues(t, "SMB"), time.Minute)
	if err == nil || strings.Contains(err.Error(), "test-only-secret") {
		t.Fatalf("unsafe error: %v", err)
	}
}

func TestActiveDirectoryCreateBootstrapsBeforeStableJoin(t *testing.T) {
	configured := false
	var methods []string
	client := testDirectoryClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			if !configured {
				fmt.Fprint(w, `{"data":[]}`)
			} else if strings.HasSuffix(r.URL.Path, "/ds") {
				fmt.Fprint(w, `{"data":{"extId":"ds","localDomain":"NFS"}}`)
			} else {
				fmt.Fprint(w, `{"data":[{"extId":"ds"}]}`)
			}
			return
		}
		methods = append(methods, r.Method)
		var payload map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		if r.Method == "POST" {
			if !strings.Contains(r.URL.Path, "configure-name-services") {
				t.Error("unexpected bootstrap path")
			}
			if _, ok := payload["adDomain"]; ok {
				t.Error("AD credentials sent to alpha bootstrap action")
			}
			configured = true
		} else if r.Method == "PUT" {
			if _, ok := payload["adDomain"]; !ok {
				t.Error("stable update missing AD")
			}
		}
		fmt.Fprint(w, `{}`)
	})
	if err := updateDirectoryService(context.Background(), client, "fs", adTestValues(t, "SMB"), time.Minute); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(methods, []string{"POST", "PUT"}) {
		t.Fatalf("wrong write sequence: %v", methods)
	}
}

func TestActiveDirectoryUpdateWaitsForTaskAndRedactsFailure(t *testing.T) {
	for _, status := range []string{"Succeeded", "Failed"} {
		t.Run(status, func(t *testing.T) {
			polled := false
			client := testDirectoryClient(t, func(w http.ResponseWriter, r *http.Request) {
				switch {
				case strings.HasPrefix(r.URL.Path, "/api/nutanix/v2.0/tasks/"):
					polled = true
					fmt.Fprintf(w, `{"progress_status":%q,"error":"test-only-secret"}`, status)
				case r.Method == "PUT":
					if r.Header.Get("NTNX-Request-Id") == "" {
						t.Error("missing request id")
					}
					fmt.Fprint(w, `{"data":{"$objectType":"prism.v4.config.TaskReference","extId":"join-task"}}`)
				case strings.HasSuffix(r.URL.Path, "/ds"):
					fmt.Fprint(w, `{"data":{"extId":"ds","localDomain":"NFS"}}`)
				default:
					fmt.Fprint(w, `{"data":[{"extId":"ds"}]}`)
				}
			})
			err := updateDirectoryService(context.Background(), client, "fs", adTestValues(t, "SMB"), time.Minute)
			if !polled {
				t.Fatal("task was not awaited")
			}
			if status == "Succeeded" && err != nil {
				t.Fatal(err)
			}
			if status == "Failed" && (err == nil || strings.Contains(err.Error(), "test-only-secret")) {
				t.Fatalf("unsafe task failure: %v", err)
			}
		})
	}
}

func TestExistingNFSDirectoryPayloadUnchanged(t *testing.T) {
	root := adTestValues(t, "SMB")[0].(map[string]interface{})
	delete(root, "ad_domain")
	got, err := buildDirectoryServicePayload([]interface{}{root})
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]interface{}{"localDomain": "NFS", "nfsVersion": "NFSV3V4"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("existing NFS payload changed: got %#v", got)
	}
}
