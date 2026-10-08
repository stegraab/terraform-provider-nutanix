package networkingv2

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

// virtualSwitchV2LegacyStateSchema describes version 0 of the import-only
// resource. Keep this snapshot independent of the current resource schema so
// Terraform can also decode legacy flatmap state before upgrading it.
func virtualSwitchV2LegacyStateSchema() *schema.Resource {
	fields := map[string]*schema.Schema{}
	for _, name := range []string{"ext_id", "name", "description", "bond_mode", "owner_type", "tenant_id"} {
		fields[name] = &schema.Schema{Type: schema.TypeString, Computed: true}
	}
	fields["mtu"] = &schema.Schema{Type: schema.TypeInt, Computed: true}
	for _, name := range []string{"is_default", "has_delete_in_progress", "has_deployment_error", "has_update_in_progress"} {
		fields[name] = &schema.Schema{Type: schema.TypeBool, Computed: true}
	}
	fields["clusters"] = &schema.Schema{
		Type: schema.TypeList, Computed: true,
		Elem: &schema.Resource{Schema: map[string]*schema.Schema{
			"ext_id":                   {Type: schema.TypeString, Computed: true},
			"vlan_identifier":          {Type: schema.TypeInt, Computed: true},
			"gateway_ip_address":       {Type: schema.TypeString, Computed: true},
			"gateway_ip_prefix_length": {Type: schema.TypeInt, Computed: true},
		}},
	}
	return &schema.Resource{Schema: fields}
}

// upgradeVirtualSwitchV2StateV0 converts the old flat gateway into the upstream
// nested block. Upstream version 0 already uses that block, so leave it intact.
// The SDK removes obsolete fields and fills new computed fields after this step.
func upgradeVirtualSwitchV2StateV0(_ context.Context, state map[string]interface{}, _ interface{}) (map[string]interface{}, error) {
	if state["clusters"] == nil {
		return state, nil
	}
	clusters, ok := state["clusters"].([]interface{})
	if !ok {
		return nil, fmt.Errorf("virtual switch state: expected a cluster list")
	}
	for i, raw := range clusters {
		cluster, ok := raw.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("virtual switch state: expected an object at cluster index %d", i)
		}
		switch gateway := cluster["gateway_ip_address"].(type) {
		case string:
			if gateway == "" {
				cluster["gateway_ip_address"] = []interface{}{}
			} else {
				cluster["gateway_ip_address"] = []interface{}{map[string]interface{}{
					"value": gateway, "prefix_length": cluster["gateway_ip_prefix_length"],
				}}
			}
		case nil:
			cluster["gateway_ip_address"] = []interface{}{}
		case []interface{}:
			// Native upstream state already has the current gateway shape.
		default:
			return nil, fmt.Errorf("virtual switch state: unexpected gateway type at cluster index %d", i)
		}
		delete(cluster, "gateway_ip_prefix_length")
	}
	return state, nil
}
