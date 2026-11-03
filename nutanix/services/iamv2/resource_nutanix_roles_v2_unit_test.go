package iamv2

import (
	"context"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
)

func TestRoleOperationsDiff(t *testing.T) {
	resource := ResourceNutanixRolesV2()
	state := &terraform.InstanceState{ID: "role", Attributes: map[string]string{
		"id": "role", "display_name": "role", "operations.#": "2", "operations.0": "read", "operations.1": "write",
	}}
	for _, test := range []struct {
		name       string
		operations []interface{}
		wantChange bool
	}{
		{"reordered", []interface{}{"write", "read"}, false},
		{"added", []interface{}{"read", "write", "delete"}, true},
		{"removed", []interface{}{"read"}, true},
		{"replaced", []interface{}{"read", "delete"}, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			config := terraform.NewResourceConfigRaw(map[string]interface{}{"display_name": "role", "operations": test.operations})
			diff, err := resource.Diff(context.Background(), state, config, nil)
			if err != nil {
				t.Fatal(err)
			}
			changed := false
			if diff != nil {
				for key := range diff.Attributes {
					if strings.HasPrefix(key, "operations.") {
						changed = true
					}
				}
			}
			if changed != test.wantChange {
				t.Fatalf("changed = %v, want %v; diff: %#v", changed, test.wantChange, diff)
			}
		})
	}
}
