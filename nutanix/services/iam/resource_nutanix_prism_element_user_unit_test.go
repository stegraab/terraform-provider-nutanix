package iam

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	conns "github.com/terraform-providers/terraform-provider-nutanix/nutanix"
	"github.com/terraform-providers/terraform-provider-nutanix/nutanix/client"
	"github.com/terraform-providers/terraform-provider-nutanix/nutanix/sdks/v3/prism"
)

const peTestCluster = "00000000-0000-0000-0000-000000000001"

type peTestAPI struct {
	user         *prism.PrismElementUser
	failList     int
	failRoles    bool
	postCount    int
	lastPassword string
	lastMethod   string
	t            *testing.T
}

func (a *peTestAPI) serve(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/api/nutanix/v3/users/me" {
		http.SetCookie(w, &http.Cookie{Name: "session", Value: "test-session"})
		w.Write([]byte(`{}`))
		return
	}
	if cookie, err := r.Cookie("session"); err != nil || cookie.Value != "test-session" {
		a.t.Error("PC session cookie was not reused")
	}
	if r.Header.Get("X-Test-Header") != "preserved" {
		a.t.Error("custom header was not reused")
	}
	if r.URL.Query().Get("proxyClusterUuid") != peTestCluster {
		a.t.Error("missing cluster proxy selector")
	}
	if r.Method == http.MethodGet {
		if a.failList != 0 {
			w.WriteHeader(a.failList)
			w.Write([]byte("server echoed secret-password"))
			return
		}
		users := []prism.PrismElementUser{}
		if a.user != nil {
			users = append(users, *a.user)
		}
		json.NewEncoder(w).Encode(users)
		return
	}
	if strings.HasSuffix(r.URL.Path, "/roles") {
		if a.failRoles {
			w.WriteHeader(500)
			return
		}
		if r.Method != http.MethodPut || a.user == nil {
			a.t.Error("invalid role request")
		}
		json.NewDecoder(r.Body).Decode(&a.user.Roles)
		w.WriteHeader(204)
		return
	}
	if r.Method == http.MethodDelete {
		a.user = nil
		w.WriteHeader(204)
		return
	}
	if r.URL.Path != "/PrismGateway/services/rest/v1/users" {
		a.t.Errorf("wrong write path: %s", r.URL.Path)
	}
	var user prism.PrismElementUser
	if err := json.NewDecoder(r.Body).Decode(&user); err != nil {
		a.t.Fatal(err)
	}
	if len(user.Roles) != 0 {
		a.t.Error("roles must be managed through separate endpoint")
	}
	if r.Method == http.MethodPost {
		a.postCount++
	}
	a.lastPassword = user.Profile.Password
	a.lastMethod = r.Method
	user.Profile.Password = "MASKED"
	if a.user != nil {
		user.Roles = a.user.Roles
	} else {
		user.Roles = []string{"ROLE_CLUSTER_VIEWER"}
	}
	a.user = &user
	w.WriteHeader(204)
}

func peTestSetup(t *testing.T) (*peTestAPI, *conns.Client, *schema.ResourceData) {
	t.Helper()
	api := &peTestAPI{t: t}
	server := httptest.NewTLSServer(http.HandlerFunc(api.serve))
	t.Cleanup(server.Close)
	host := strings.TrimPrefix(server.URL, "https://")
	pc, err := prism.NewV3Client(client.Credentials{URL: host, Endpoint: host, Username: "admin", Password: "provisioning-password", Insecure: true, SessionAuth: true, CustomHeaders: map[string]string{"X-Test-Header": "preserved"}})
	if err != nil {
		t.Fatal(err)
	}
	d := schema.TestResourceDataRaw(t, ResourceNutanixPrismElementUser().Schema, map[string]interface{}{"cluster_uuid": peTestCluster, "username": "metrics-user", "password": "secret-password", "first_name": "Metrics", "roles": []interface{}{"ROLE_CLUSTER_VIEWER"}})
	return api, &conns.Client{API: pc}, d
}

func TestPEUserLifecycle(t *testing.T) {
	api, meta, d := peTestSetup(t)
	if diags := peUserCreate(context.Background(), d, meta); diags.HasError() {
		t.Fatal(diags)
	}
	if d.Id() != peTestCluster+"/metrics-user" || api.postCount != 1 {
		t.Fatal("create did not store composite ID")
	}
	if d.Get("password") != "secret-password" {
		t.Fatal("refresh overwrote password with API mask")
	}
	// Construct data from persisted state: unchanged passwords must not be resent.
	unchanged := ResourceNutanixPrismElementUser().Data(d.State())
	if diags := peUserUpdate(context.Background(), unchanged, meta); diags.HasError() {
		t.Fatal(diags)
	}
	if api.lastPassword != "" || api.lastMethod != http.MethodPut {
		t.Fatal("profile update resent unchanged password")
	}
	rotated := schema.TestResourceDataRaw(t, ResourceNutanixPrismElementUser().Schema, map[string]interface{}{"cluster_uuid": peTestCluster, "username": "metrics-user", "password": "rotated-password", "roles": []interface{}{"ROLE_CLUSTER_VIEWER"}})
	rotated.SetId(d.Id())
	if diags := peUserUpdate(context.Background(), rotated, meta); diags.HasError() {
		t.Fatal(diags)
	}
	if api.lastPassword != "rotated-password" {
		t.Fatal("password rotation was not sent")
	}
	api.user.Enabled = false
	api.user.Roles = []string{"ROLE_CLUSTER_VIEWER", "ROLE_CLUSTER_ADMIN"}
	if diags := peUserRead(context.Background(), d, meta); diags.HasError() {
		t.Fatal(diags)
	}
	if d.Get("enabled").(bool) || !d.Get("roles").(*schema.Set).Contains("ROLE_CLUSTER_ADMIN") {
		t.Fatal("remote drift not refreshed")
	}
	if diags := peUserDelete(context.Background(), d, meta); diags.HasError() {
		t.Fatal(diags)
	}
	if d.Id() != "" || api.user != nil {
		t.Fatal("delete failed")
	}
	d.SetId(peTestCluster + "/metrics-user")
	if diags := peUserDelete(context.Background(), d, meta); diags.HasError() {
		t.Fatal(diags)
	}
}

func TestPEUserReadFailureRetainsState(t *testing.T) {
	for _, status := range []int{401, 403, 404, 500} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			api, meta, d := peTestSetup(t)
			d.SetId(peTestCluster + "/metrics-user")
			api.failList = status
			diags := peUserRead(context.Background(), d, meta)
			if !diags.HasError() || d.Id() == "" {
				t.Fatal("API failure was treated as deletion")
			}
			for _, diagnostic := range diags {
				if strings.Contains(diagnostic.Summary, "secret-password") {
					t.Fatal("response secret leaked into diagnostics")
				}
			}
		})
	}
	api, meta, d := peTestSetup(t)
	d.SetId(peTestCluster + "/metrics-user")
	if diags := peUserRead(context.Background(), d, meta); diags.HasError() || d.Id() != "" {
		t.Fatal("successful empty list did not clear missing resource")
	}
	_ = api
}

func TestPEUserPartialCreateAndConflict(t *testing.T) {
	api, meta, d := peTestSetup(t)
	api.failRoles = true
	if diags := peUserCreate(context.Background(), d, meta); !diags.HasError() || d.Id() == "" {
		t.Fatal("partial creation lost resource ID")
	}
	api.failRoles = false
	duplicate := schema.TestResourceDataRaw(t, ResourceNutanixPrismElementUser().Schema, map[string]interface{}{"cluster_uuid": peTestCluster, "username": "metrics-user", "password": "secret-password", "roles": []interface{}{"ROLE_CLUSTER_VIEWER"}})
	if diags := peUserCreate(context.Background(), duplicate, meta); !diags.HasError() || api.postCount != 1 {
		t.Fatal("existing user was overwritten")
	}
}

func TestPEUserImportAndValidation(t *testing.T) {
	_, meta, d := peTestSetup(t)
	for _, id := range []string{"invalid", "not-a-uuid/user", peTestCluster + "/", peTestCluster + "/..", peTestCluster + "/../admin"} {
		d.SetId(id)
		if _, err := peUserImport(context.Background(), d, meta); err == nil {
			t.Fatalf("accepted invalid import %s", id)
		}
	}
	d.SetId(peTestCluster + "/imported-user")
	if _, err := peUserImport(context.Background(), d, meta); err != nil || d.Get("username") != "imported-user" {
		t.Fatal("valid import failed", err)
	}
	api, meta, d := peTestSetup(t)
	d.Set("password", "")
	if diags := peUserCreate(context.Background(), d, meta); !diags.HasError() || api.postCount != 0 {
		t.Fatal("creation without password was accepted")
	}
	d.Set("password", "secret-password")
	d.Set("roles", []interface{}{"ROLE_CLUSTER_ADMIN"})
	if diags := peUserCreate(context.Background(), d, meta); !diags.HasError() || api.postCount != 0 {
		t.Fatal("PE mandatory viewer role was omitted")
	}
}

func TestPEUserImportedPasswordIsNotReadFromAPI(t *testing.T) {
	api, meta, _ := peTestSetup(t)
	api.user = &prism.PrismElementUser{Profile: prism.PrismElementUserProfile{Username: "imported-user", Password: "MASKED"}, Enabled: true, Roles: []string{"ROLE_CLUSTER_VIEWER"}}
	d := schema.TestResourceDataRaw(t, ResourceNutanixPrismElementUser().Schema, map[string]interface{}{})
	d.SetId(peTestCluster + "/imported-user")
	if _, err := peUserImport(context.Background(), d, meta); err != nil {
		t.Fatal(err)
	}
	if diags := peUserRead(context.Background(), d, meta); diags.HasError() {
		t.Fatal(diags)
	}
	if d.Get("password").(string) != "" {
		t.Fatal("import copied API password into state")
	}
	if diags := peUserUpdate(context.Background(), d, meta); diags.HasError() {
		t.Fatal(diags)
	}
	if api.lastPassword != "" {
		t.Fatal("imported password was overwritten")
	}
}
