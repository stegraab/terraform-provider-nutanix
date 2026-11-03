package filesv4

import (
	"context"
	"encoding/json"
	"strings"

	"fmt"
	"net/http"

	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	conns "github.com/terraform-providers/terraform-provider-nutanix/nutanix"
	filesClient "github.com/terraform-providers/terraform-provider-nutanix/nutanix/sdks/v4/files"
)

func TestMountTargetPayloadKeepsNFSDefaultAndScopesSMBPermissions(t *testing.T) {
	r := ResourceNutanixFileServerMountTargetV2()
	nfs := schema.TestResourceDataRaw(t, r.Schema, map[string]interface{}{"file_server_ext_id": "fs", "name": "export", "path": "/export", "max_size_gb": 1})
	payload := buildFileServerMountTargetPayload(nfs)
	if payload["protocol"] != "NFS" || payload["nfsProperties"] == nil || payload["smbProperties"] != nil {
		t.Fatalf("NFS defaults changed: %#v", payload)
	}
	smb := schema.TestResourceDataRaw(t, r.Schema, map[string]interface{}{"file_server_ext_id": "fs", "name": "share", "path": "/share", "max_size_gb": 1, "protocol": "SMB", "smb_properties": []interface{}{map[string]interface{}{"share_acl": []interface{}{map[string]interface{}{"user_or_group_name": "DEV\\Files-Read", "access_type": "READ"}, map[string]interface{}{"user_or_group_name": "DEV\\Files-Modify", "access_type": "CHANGE"}}}}})
	payload = buildFileServerMountTargetPayload(smb)
	if payload["protocol"] != "SMB" || payload["nfsProperties"] != nil {
		t.Fatalf("unexpected SMB payload: %#v", payload)
	}
	acl := payload["smbProperties"].(map[string]interface{})["shareACL"].([]interface{})
	if len(acl) != 2 {
		t.Fatalf("expected exactly configured access rules: %#v", acl)
	}
	for _, raw := range acl {
		a := raw.(map[string]interface{})
		if a["permissionType"] != "ALLOW" || a["sid"] != nil {
			t.Fatalf("unexpected ACE: %#v", a)
		}
	}
	if r.Schema["smb_properties"].ForceNew || !r.Schema["protocol"].ForceNew {
		t.Fatal("ACL updates must be in place; protocol changes must replace")
	}
}

func TestSMBShareReadDropsResolvedSIDAndPreservesDeclaredACL(t *testing.T) {
	client := testDirectoryClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" || r.URL.Path != "/api/files/v4.0/config/file-servers/fs/mount-targets/share" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		fmt.Fprint(w, `{"data":{"extId":"share","name":"share","path":"/share","maxSizeGB":1,"protocol":"SMB","state":"ONLINE","smbProperties":{"shareACL":[{"userOrGroupName":"DEV\\Files-Read","permissionType":"ALLOW","accessType":"READ","sid":"S-1-5-21-1"}]}}}`)
	})
	d := schema.TestResourceDataRaw(t, ResourceNutanixFileServerMountTargetV2().Schema, map[string]interface{}{"file_server_ext_id": "fs", "name": "share", "path": "/share", "max_size_gb": 1, "protocol": "SMB"})
	d.SetId("share")
	meta := &conns.Client{FilesAPI: &filesClient.Client{APIClientInstance: client}}
	if diags := resourceNutanixFileServerMountTargetV2Read(context.Background(), d, meta); diags.HasError() {
		t.Fatal(diags)
	}
	blocks := d.Get("smb_properties").([]interface{})
	acl := blocks[0].(map[string]interface{})["share_acl"].(*schema.Set).List()
	if len(acl) != 1 || acl[0].(map[string]interface{})["user_or_group_name"] != `DEV\Files-Read` || acl[0].(map[string]interface{})["access_type"] != "READ" {
		t.Fatalf("read lost permission: %#v", acl)
	}
	if _, ok := acl[0].(map[string]interface{})["sid"]; ok {
		t.Fatal("read-only SID must not be stored in managed ACL")
	}
}

func TestSMBShareUpdateUsesETagAndFilesScopedTask(t *testing.T) {
	updated := false
	client := testDirectoryClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "GET" && r.URL.Path == "/api/files/v4.0/config/file-servers/fs/mount-targets/share":
			w.Header().Set("Etag", "12")
			access := "READ"
			if updated {
				access = "CHANGE"
			}
			fmt.Fprintf(w, `{"data":{"extId":"share","name":"share","path":"/share","type":"GENERAL","maxSizeGB":1,"protocol":"SMB","smbProperties":{"isCaEnabled":true,"isSmb3EncryptionEnabled":true,"shareACL":[{"userOrGroupName":"DEV\\Files-Modify","permissionType":"ALLOW","accessType":%q}]}}}`, access)
		case r.Method == "PUT":
			if r.Header.Get("If-Match") != "12" {
				t.Error("missing ETag")
			}
			var body map[string]interface{}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			props := body["smbProperties"].(map[string]interface{})
			if props["isCaEnabled"] != true || props["isSmb3EncryptionEnabled"] != true {
				t.Fatal("update lost SMB settings")
			}
			acl := props["shareACL"].([]interface{})
			if len(acl) != 1 || acl[0].(map[string]interface{})["accessType"] != "CHANGE" {
				t.Fatal("wrong ACL")
			}
			updated = true
			fmt.Fprint(w, `{"data":{"extId":"ZmlsZXNfdGFzazpmcw==:task"}}`)
		case r.Method == "GET" && strings.Contains(r.URL.Path, "/file-servers/fs/tasks/"):
			fmt.Fprint(w, `{"data":{"status":"SUCCEEDED"}}`)
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(404)
		}
	})
	d := schema.TestResourceDataRaw(t, ResourceNutanixFileServerMountTargetV2().Schema, map[string]interface{}{"file_server_ext_id": "fs", "name": "share", "path": "/share", "max_size_gb": 1, "protocol": "SMB", "smb_properties": []interface{}{map[string]interface{}{"is_smb3_encryption_enabled": true, "share_acl": []interface{}{map[string]interface{}{"user_or_group_name": `DEV\Files-Modify`, "access_type": "CHANGE"}}}}})
	d.SetId("share")
	meta := &conns.Client{FilesAPI: &filesClient.Client{APIClientInstance: client}}
	if diags := resourceNutanixFileServerMountTargetV2Update(context.Background(), d, meta); diags.HasError() {
		t.Fatal(diags)
	}
	if !updated {
		t.Fatal("no ACL update performed")
	}
}
