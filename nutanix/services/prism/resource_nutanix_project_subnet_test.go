package prism

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"reflect"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/terraform-providers/terraform-provider-nutanix/nutanix/client"
	v3 "github.com/terraform-providers/terraform-provider-nutanix/nutanix/sdks/v3/prism"
)

type projectSubnetFake struct {
	project  map[string]interface{}
	getErr   error
	conflict int
	writes   int
}

func (f *projectSubnetFake) Get(context.Context, string) (map[string]interface{}, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
	b, _ := json.Marshal(f.project)
	var copy map[string]interface{}
	_ = json.Unmarshal(b, &copy)
	return copy, nil
}

func (f *projectSubnetFake) Update(_ context.Context, _ string, p map[string]interface{}) (*v3.Project, error) {
	f.writes++
	if f.conflict != 0 {
		status := f.conflict
		f.conflict = 0
		// Another state attached a subnet after the initial GET.
		r := f.project["spec"].(map[string]interface{})["resources"].(map[string]interface{})
		r["subnet_reference_list"] = append(r["subnet_reference_list"].([]interface{}), map[string]interface{}{"kind": "subnet", "uuid": "concurrent"})
		f.project["metadata"].(map[string]interface{})["spec_version"] = float64(2)
		return nil, &client.SensitiveHTTPError{StatusCode: status}
	}
	f.project = p
	return &v3.Project{Status: &v3.ProjectStatus{ExecutionContext: &v3.ExecutionContext{TaskUUID: "task"}}}, nil
}

func projectSubnetFixture() map[string]interface{} {
	return map[string]interface{}{
		"metadata": map[string]interface{}{"spec_version": float64(1), "categories": map[string]interface{}{"key": "value"}},
		"spec": map[string]interface{}{"name": "project", "resources": map[string]interface{}{
			"subnet_reference_list":    []interface{}{map[string]interface{}{"kind": "subnet", "uuid": "existing", "name": "existing subnet"}},
			"default_subnet_reference": map[string]interface{}{"uuid": "default"},
			"vpc_reference_list":       []interface{}{map[string]interface{}{"uuid": "vpc"}},
			"future_field":             map[string]interface{}{"preserve": true},
		}},
		"status": map[string]interface{}{"state": "COMPLETE"},
	}
}

func TestProjectSubnetAssociationLifecycle(t *testing.T) {
	f := &projectSubnetFake{project: projectSubnetFixture()}
	waitCalls := 0
	wait := func(task string) error {
		if task != "task" {
			t.Fatalf("unexpected task %q", task)
		}
		waitCalls++
		return nil
	}
	change := func(subnet string, attach bool) error {
		return changeProjectSubnet(context.Background(), f, "project", subnet, attach, 5*time.Second, wait)
	}
	if err := change("new", true); err != nil {
		t.Fatal(err)
	}
	r := f.project["spec"].(map[string]interface{})["resources"].(map[string]interface{})
	if !reflect.DeepEqual(r["future_field"], map[string]interface{}{"preserve": true}) || len(r["vpc_reference_list"].([]interface{})) != 1 {
		t.Fatal("unrelated project fields lost")
	}
	if _, ok := f.project["status"]; ok {
		t.Fatal("read-only status sent to update")
	}
	if err := change("new", true); err != nil {
		t.Fatal(err)
	}
	if f.writes != 1 {
		t.Fatal("existing association was rewritten")
	}
	if err := change("new", false); err != nil {
		t.Fatal(err)
	}
	refs, present, err := projectSubnetReferences(f.project, "existing")
	if err != nil || !present || len(refs) != 1 {
		t.Fatalf("unrelated subnet removed: %v", refs)
	}
	if err := change("new", false); err != nil {
		t.Fatal(err)
	}
	if f.writes != 2 || waitCalls != 2 {
		t.Fatalf("writes=%d waits=%d", f.writes, waitCalls)
	}
}

func TestProjectSubnetAssociationRetriesFreshProject(t *testing.T) {
	for _, status := range []int{http.StatusConflict, http.StatusPreconditionFailed} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			f := &projectSubnetFake{project: projectSubnetFixture(), conflict: status}
			if err := changeProjectSubnet(context.Background(), f, "project", "new", true, 5*time.Second, func(string) error { return nil }); err != nil {
				t.Fatal(err)
			}
			refs, present, err := projectSubnetReferences(f.project, "concurrent")
			if err != nil || !present || len(refs) != 3 || f.writes != 2 {
				t.Fatal("retry lost concurrent association")
			}
			if f.project["metadata"].(map[string]interface{})["spec_version"] != float64(2) {
				t.Fatal("retry used stale version")
			}
		})
	}
}

func TestProjectSubnetAssociationErrors(t *testing.T) {
	t.Run("missing project delete", func(t *testing.T) {
		f := &projectSubnetFake{getErr: &client.SensitiveHTTPError{StatusCode: 404}}
		if err := changeProjectSubnet(context.Background(), f, "p", "s", false, time.Second, nil); err != nil {
			t.Fatal(err)
		}
		if err := changeProjectSubnet(context.Background(), f, "p", "s", true, time.Second, nil); err == nil {
			t.Fatal("create swallowed missing project")
		}
	})
	t.Run("authorization", func(t *testing.T) {
		f := &projectSubnetFake{getErr: &client.SensitiveHTTPError{StatusCode: 403}}
		if err := changeProjectSubnet(context.Background(), f, "p", "s", false, time.Second, nil); err == nil {
			t.Fatal("delete swallowed authorization failure")
		}
	})
	t.Run("default subnet", func(t *testing.T) {
		f := &projectSubnetFake{project: projectSubnetFixture()}
		r := f.project["spec"].(map[string]interface{})["resources"].(map[string]interface{})
		r["subnet_reference_list"] = []interface{}{map[string]interface{}{"uuid": "default"}}
		if err := changeProjectSubnet(context.Background(), f, "p", "default", false, time.Second, nil); err == nil || f.writes != 0 {
			t.Fatal("default subnet detached")
		}
	})
	t.Run("failed task", func(t *testing.T) {
		f := &projectSubnetFake{project: projectSubnetFixture()}
		failed := errors.New("task failed")
		if err := changeProjectSubnet(context.Background(), f, "p", "new", true, time.Second, func(string) error { return failed }); !errors.Is(err, failed) {
			t.Fatalf("task error lost: %v", err)
		}
	})
	t.Run("missing specification version", func(t *testing.T) {
		f := &projectSubnetFake{project: projectSubnetFixture()}
		delete(f.project, "metadata")
		if err := changeProjectSubnet(context.Background(), f, "p", "new", true, time.Second, nil); err == nil || f.writes != 0 {
			t.Fatal("unversioned update accepted")
		}
	})

	t.Run("malformed response", func(t *testing.T) {
		if _, _, err := projectSubnetReferences(map[string]interface{}{}, "subnet"); err == nil {
			t.Fatal("malformed response treated as absent association")
		}
	})
}

func TestResourceNutanixProjectSubnetImport(t *testing.T) {
	r := ResourceNutanixProjectSubnet()
	d := schema.TestResourceDataRaw(t, r.Schema, nil)
	id := "11111111-1111-4111-8111-111111111111/22222222-2222-4222-8222-222222222222"
	d.SetId(id)
	if _, err := r.Importer.StateContext(context.Background(), d, nil); err != nil {
		t.Fatal(err)
	}
	if d.Get("project_uuid").(string)+"/"+d.Get("subnet_uuid").(string) != id {
		t.Fatal("import did not set pair")
	}
	d.SetId("invalid")
	if _, err := r.Importer.StateContext(context.Background(), d, nil); err == nil {
		t.Fatal("invalid import accepted")
	}
}
