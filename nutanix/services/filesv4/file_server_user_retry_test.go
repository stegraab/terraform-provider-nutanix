package filesv4

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	conns "github.com/terraform-providers/terraform-provider-nutanix/nutanix"
	filesClient "github.com/terraform-providers/terraform-provider-nutanix/nutanix/sdks/v4/files"
)

func TestFileServerUserCreateRetriesVersionConflict(t *testing.T) {
	posts := 0
	client := testDirectoryClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/files/v4.0/config/file-servers/fs/users":
			fmt.Fprint(w, `{"data":[]}`)
		case r.Method == http.MethodPost:
			posts++
			if posts == 1 {
				w.WriteHeader(http.StatusConflict)
				fmt.Fprint(w, `{"data":{"error":[{"message":"Failed to create user: zk: version conflict"}]}}`)
				return
			}
			fmt.Fprint(w, `{"data":{"extId":"user-id"}}`)
		case r.Method == http.MethodGet && r.URL.Path == "/api/files/v4.0/config/file-servers/fs/users/user-id":
			fmt.Fprint(w, `{"data":{"extId":"user-id","name":"user","roles":["USER_ADMIN"]}}`)
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	})
	d := schema.TestResourceDataRaw(t, ResourceNutanixFileServerUserV2().Schema, map[string]interface{}{"file_server_ext_id": "fs", "name": "user", "password": "secret", "roles": []interface{}{"USER_ADMIN"}})
	meta := &conns.Client{FilesAPI: &filesClient.Client{APIClientInstance: client}}
	if diags := resourceNutanixFileServerUserV2Create(context.Background(), d, meta); diags.HasError() {
		t.Fatal(diags)
	}
	if posts != 2 || d.Id() != "user-id" {
		t.Fatalf("expected one retry and the created user, got %d requests and ID %q", posts, d.Id())
	}
}

func TestFileServerUserUpdateRefetchesETagAfterVersionConflict(t *testing.T) {
	gets, puts := 0, 0
	client := testDirectoryClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			gets++
			w.Header().Set("ETag", fmt.Sprint(gets))
			fmt.Fprint(w, `{"data":{"extId":"user-id","name":"user","roles":["USER_ADMIN"]}}`)
		case http.MethodPut:
			puts++
			if r.Header.Get("If-Match") != fmt.Sprint(puts) {
				t.Errorf("request %d used unexpected ETag %q", puts, r.Header.Get("If-Match"))
			}
			if puts == 1 {
				w.WriteHeader(http.StatusConflict)
				fmt.Fprint(w, `{"data":{"error":[{"message":"zk: version conflict"}]}}`)
				return
			}
			fmt.Fprint(w, `{}`)
		default:
			t.Errorf("unexpected method %s", r.Method)
		}
	})
	d := schema.TestResourceDataRaw(t, ResourceNutanixFileServerUserV2().Schema, map[string]interface{}{"file_server_ext_id": "fs", "name": "user", "password": "secret", "roles": []interface{}{"USER_ADMIN"}})
	d.SetId("user-id")
	meta := &conns.Client{FilesAPI: &filesClient.Client{APIClientInstance: client}}
	if diags := resourceNutanixFileServerUserV2Update(context.Background(), d, meta); diags.HasError() {
		t.Fatal(diags)
	}
	if puts != 2 || gets != 3 {
		t.Fatalf("expected two updates and fresh ETags, got %d updates and %d reads", puts, gets)
	}
}

func TestFileServerUserDoesNotRetryOtherErrors(t *testing.T) {
	attempts := 0
	_, err := retryFileServerUserMutation(context.Background(), func() (map[string]interface{}, int, error) {
		attempts++
		return map[string]interface{}{"error": "invalid user"}, http.StatusBadRequest, nil
	})
	if err == nil || !strings.Contains(err.Error(), "invalid user") || attempts != 1 {
		t.Fatalf("expected the original error without a retry, got %v after %d attempts", err, attempts)
	}
}

func TestFileServerUserDeleteRefetchesETagAfterVersionConflict(t *testing.T) {
	gets, deletes := 0, 0
	client := testDirectoryClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			gets++
			w.Header().Set("ETag", fmt.Sprint(gets))
			fmt.Fprint(w, `{"data":{"extId":"user-id","name":"user"}}`)
		case http.MethodDelete:
			deletes++
			if r.Header.Get("If-Match") != fmt.Sprint(deletes) {
				t.Errorf("request %d used unexpected ETag %q", deletes, r.Header.Get("If-Match"))
			}
			if deletes == 1 {
				w.WriteHeader(http.StatusConflict)
				fmt.Fprint(w, `{"data":{"error":[{"message":"zk: version conflict"}]}}`)
				return
			}
			fmt.Fprint(w, `{}`)
		default:
			t.Errorf("unexpected method %s", r.Method)
		}
	})
	d := schema.TestResourceDataRaw(t, ResourceNutanixFileServerUserV2().Schema, map[string]interface{}{"file_server_ext_id": "fs", "name": "user", "password": "secret"})
	d.SetId("user-id")
	meta := &conns.Client{FilesAPI: &filesClient.Client{APIClientInstance: client}}
	if diags := resourceNutanixFileServerUserV2Delete(context.Background(), d, meta); diags.HasError() {
		t.Fatal(diags)
	}
	if deletes != 2 || gets != 2 {
		t.Fatalf("expected two deletes and fresh ETags, got %d deletes and %d reads", deletes, gets)
	}
}

func TestFileServerUserNameLength(t *testing.T) {
	validate := ResourceNutanixFileServerUserV2().Schema["name"].ValidateFunc
	if _, errors := validate(strings.Repeat("a", 40), "name"); len(errors) != 0 {
		t.Fatalf("40-character name should be valid: %v", errors)
	}
	if _, errors := validate(strings.Repeat("a", 41), "name"); len(errors) == 0 {
		t.Fatal("41-character name must be rejected")
	}
}
