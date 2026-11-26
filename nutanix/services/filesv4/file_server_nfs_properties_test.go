package filesv4

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
	conns "github.com/terraform-providers/terraform-provider-nutanix/nutanix"
	filesClient "github.com/terraform-providers/terraform-provider-nutanix/nutanix/sdks/v4/files"
)

func TestNfsMountTargetPolicyUpdatesInPlaceAndReadsDrift(t *testing.T) {
	ctx := context.Background()
	r := ResourceNutanixFileServerMountTargetV2()
	base := map[string]interface{}{"file_server_ext_id": "fs", "name": "backup", "path": "/backup", "max_size_gb": 10}
	config := map[string]interface{}{}
	for k, v := range base {
		config[k] = v
	}
	policy := map[string]interface{}{"squash_type": "ALL_SQUASH", "client_exception": []interface{}{
		map[string]interface{}{"access_type": "READ_WRITE", "clients": []interface{}{"192.0.2.8", "192.0.2.7"}},
	}}
	config["nfs_properties"] = []interface{}{policy}
	existing := map[string]interface{}{
		"accessType": "NO_ACCESS", "authenticationType": "SYSTEM", "squashType": "ROOT_SQUASH",
		"anonymousIdentifier": map[string]interface{}{"uid": -2, "gid": -2},
	}
	mount := map[string]interface{}{
		"extId": "export", "name": "backup", "path": "/backup", "protocol": "NFS", "type": "GENERAL", "maxSizeGB": 10, "nfsProperties": existing,
	}
	var mutex sync.Mutex
	updates := 0
	client := testDirectoryClient(t, func(w http.ResponseWriter, req *http.Request) {
		mutex.Lock()
		defer mutex.Unlock()
		if req.URL.Path != "/api/files/v4.0/config/file-servers/fs/mount-targets/export" {
			t.Errorf("unexpected request: %s %s", req.Method, req.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		switch req.Method {
		case http.MethodGet:
			w.Header().Set("ETag", "12")
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"data": mount})
		case http.MethodPut:
			if req.Header.Get("If-Match") != "12" {
				t.Error("missing ETag")
			}
			var body map[string]interface{}
			if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
				t.Error(err)
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			props, ok := body["nfsProperties"].(map[string]interface{})
			if !ok || body["smbProperties"] != nil || props["accessType"] != "NO_ACCESS" || props["squashType"] != "ALL_SQUASH" {
				t.Errorf("incorrect NFS update: %#v", body)
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			rules := mountTargetSlice(props["clientExceptions"])
			if updates == 0 {
				if len(rules) != 1 || rules[0].(map[string]interface{})["clients"] != "192.0.2.7,192.0.2.8" || rules[0].(map[string]interface{})["accessType"] != "READ_WRITE" {
					t.Errorf("wrong NFS client rules: %#v", rules)
				} else {
					// Files may return the clients in a different order.
					rules[0].(map[string]interface{})["clients"] = "192.0.2.8, 192.0.2.7"
				}
			} else if props["clientExceptions"] == nil || len(rules) != 0 {
				t.Errorf("expected an explicit empty ACL: %#v", props)
			}
			mount["nfsProperties"] = props
			updates++
			_, _ = w.Write([]byte(`{}`))
		default:
			t.Errorf("unexpected request method: %s", req.Method)
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})
	meta := &conns.Client{FilesAPI: &filesClient.Client{APIClientInstance: client}}
	state := schema.TestResourceDataRaw(t, r.Schema, base)
	state.SetId("export")
	if diags := r.ReadContext(ctx, state, meta); diags.HasError() {
		t.Fatal(diags)
	}
	checkDiff := func(data *schema.ResourceData, cfg map[string]interface{}, wantChange bool) {
		t.Helper()
		diff, err := r.Diff(ctx, data.State(), terraform.NewResourceConfigRaw(cfg), meta)
		if err != nil {
			t.Fatal(err)
		}
		if changed := diff != nil && !diff.Empty(); changed != wantChange || (changed && diff.RequiresNew()) {
			t.Fatalf("wanted in-place change=%t, got %#v", wantChange, diff)
		}
	}
	checkDiff(state, base, false)
	checkDiff(state, config, true)
	smbConfig := map[string]interface{}{
		"file_server_ext_id": "fs", "name": "backup", "path": "/backup", "max_size_gb": 10, "protocol": "SMB",
		"smb_properties": []interface{}{map[string]interface{}{"share_acl": []interface{}{
			map[string]interface{}{"user_or_group_name": `EXAMPLE\Backup`, "access_type": "CHANGE"},
		}}},
	}
	if diff, err := r.Diff(ctx, state.State(), terraform.NewResourceConfigRaw(smbConfig), meta); err != nil || diff == nil || !diff.RequiresNew() {
		t.Fatalf("NFS state should not block a protocol replacement: diff=%#v err=%v", diff, err)
	}
	smbConfig["nfs_properties"] = config["nfs_properties"]
	if diags := r.Validate(terraform.NewResourceConfigRaw(smbConfig)); !diags.HasError() {
		t.Fatal("explicit NFS policy must be rejected for SMB")
	}
	desired := schema.TestResourceDataRaw(t, r.Schema, config)
	desired.SetId("export")
	create := buildFileServerMountTargetPayload(desired)["nfsProperties"].(map[string]interface{})
	if create["accessType"] != "NO_ACCESS" || create["squashType"] != "ALL_SQUASH" || len(mountTargetSlice(create["clientExceptions"])) != 1 {
		t.Fatalf("create did not use the declared NFS policy: %#v", create)
	}
	if diags := r.UpdateContext(ctx, desired, meta); diags.HasError() {
		t.Fatal(diags)
	}
	checkDiff(desired, config, false)
	checkDiff(desired, base, false)
	mutex.Lock()
	mount["nfsProperties"].(map[string]interface{})["accessType"] = "READ_WRITE"
	mutex.Unlock()
	if diags := r.ReadContext(ctx, desired, meta); diags.HasError() {
		t.Fatal(diags)
	}
	checkDiff(desired, config, true)
	delete(policy, "client_exception")
	checkDiff(desired, config, true)
	revoke := schema.TestResourceDataRaw(t, r.Schema, config)
	revoke.SetId("export")
	if diags := r.UpdateContext(ctx, revoke, meta); diags.HasError() {
		t.Fatal(diags)
	}
	checkDiff(revoke, config, false)
	mutex.Lock()
	defer mutex.Unlock()
	if updates != 2 || revoke.Id() != "export" {
		t.Fatalf("expected two in-place updates, got %d for %s", updates, revoke.Id())
	}
}
