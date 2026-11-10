package prism

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/terraform-providers/terraform-provider-nutanix/nutanix/client"
)

func TestProjectSubnetServicePreservesDocumentAndHTTPStatus(t *testing.T) {
	status := http.StatusOK
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/nutanix/v3/projects/project" {
			t.Errorf("incorrect path %s", r.URL.Path)
		}
		if status != http.StatusOK {
			w.WriteHeader(status)
			return
		}
		if r.Method == http.MethodPut {
			var doc map[string]interface{}
			if err := json.NewDecoder(r.Body).Decode(&doc); err != nil {
				t.Error(err)
			}
			if doc["unknown"] != "preserved" {
				t.Error("unknown project field lost")
			}
			_, _ = w.Write([]byte(`{"status":{"execution_context":{"task_uuid":"task"}}}`))
			return
		}
		_, _ = w.Write([]byte(`{"unknown":"preserved","spec":{"resources":{}},"metadata":{"spec_version":7}}`))
	}))
	defer server.Close()
	base, err := client.NewClient(&client.Credentials{URL: strings.TrimPrefix(server.URL, "http://"), Username: "user", Password: "password"}, "test", "original", true)
	if err != nil {
		t.Fatal(err)
	}
	conn := &Client{client: base}
	api, err := conn.ProjectSubnets()
	if err != nil {
		t.Fatal(err)
	}
	defer api.Close()
	doc, err := api.Get(context.Background(), "project")
	if err != nil {
		t.Fatal(err)
	}
	result, err := api.Update(context.Background(), "project", doc)
	if err != nil || result.Status.ExecutionContext.TaskUUID != "task" {
		t.Fatalf("update: %v %v", result, err)
	}
	for _, code := range []int{404, 409, 412, 403, 500} {
		status = code
		_, err := api.Get(context.Background(), "project")
		var apiErr *client.SensitiveHTTPError
		if !errors.As(err, &apiErr) || apiErr.StatusCode != code {
			t.Fatalf("lost HTTP %d: %v", code, err)
		}
	}
}
