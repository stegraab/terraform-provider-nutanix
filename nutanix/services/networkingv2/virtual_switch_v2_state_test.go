package networkingv2

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	ctyjson "github.com/hashicorp/go-cty/cty/json"
	"github.com/hashicorp/go-cty/cty/msgpack"
	"github.com/hashicorp/terraform-plugin-go/tfprotov5"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func upgradeVirtualSwitchStateForTest(t *testing.T, version int64, raw *tfprotov5.RawState) map[string]interface{} {
	t.Helper()
	r := ResourceNutanixVirtualSwitchV2()
	server := schema.NewGRPCProviderServer(&schema.Provider{
		ResourcesMap: map[string]*schema.Resource{"nutanix_virtual_switch_v2": r},
	})
	resp, err := server.UpgradeResourceState(context.Background(), &tfprotov5.UpgradeResourceStateRequest{
		TypeName: "nutanix_virtual_switch_v2", Version: version, RawState: raw,
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, diagnostic := range resp.Diagnostics {
		if diagnostic.Severity == tfprotov5.DiagnosticSeverityError {
			t.Fatalf("state upgrade failed: %s: %s", diagnostic.Summary, diagnostic.Detail)
		}
	}
	if resp.UpgradedState == nil {
		t.Fatal("state upgrade returned no state")
	}
	typ := r.CoreConfigSchema().ImpliedType()
	value, err := msgpack.Unmarshal(resp.UpgradedState.MsgPack, typ)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := ctyjson.Marshal(value, typ)
	if err != nil {
		t.Fatal(err)
	}
	var result map[string]interface{}
	if err := json.Unmarshal(encoded, &result); err != nil {
		t.Fatal(err)
	}
	return result
}

func TestVirtualSwitchV2UpgradeLegacyState(t *testing.T) {
	for _, test := range []struct {
		name    string
		gateway interface{}
		prefix  interface{}
		want    []interface{}
	}{
		{"gateway", "192.0.2.1", 24, []interface{}{map[string]interface{}{"value": "192.0.2.1", "prefix_length": float64(24)}}},
		{"zero prefix", "192.0.2.1", 0, []interface{}{map[string]interface{}{"value": "192.0.2.1", "prefix_length": float64(0)}}},
		{"missing prefix", "192.0.2.1", nil, []interface{}{map[string]interface{}{"value": "192.0.2.1", "prefix_length": nil}}},
		{"empty gateway", "", 0, []interface{}{}},
		{"null gateway", nil, nil, []interface{}{}},
	} {
		t.Run(test.name, func(t *testing.T) {
			state := map[string]interface{}{
				"id": "switch-id", "ext_id": "switch-id", "name": "vs0", "description": "existing switch",
				"mtu": float64(9000), "bond_mode": "ACTIVE_BACKUP", "is_default": true,
				"owner_type": "USER", "tenant_id": "tenant-id", "has_deployment_error": false,
				"has_update_in_progress": false, "has_delete_in_progress": false,
				"clusters": []interface{}{
					map[string]interface{}{"ext_id": "cluster-a", "vlan_identifier": float64(100),
						"gateway_ip_address": test.gateway, "gateway_ip_prefix_length": test.prefix},
					map[string]interface{}{"ext_id": "cluster-b", "vlan_identifier": float64(0),
						"gateway_ip_address": "", "gateway_ip_prefix_length": float64(0)},
				},
			}
			encoded, err := json.Marshal(state)
			if err != nil {
				t.Fatal(err)
			}
			got := upgradeVirtualSwitchStateForTest(t, 0, &tfprotov5.RawState{JSON: encoded})
			for name, expected := range state {
				if name != "clusters" && !reflect.DeepEqual(got[name], expected) {
					t.Errorf("%s changed: got %#v, want %#v", name, got[name], expected)
				}
			}
			clusters := got["clusters"].([]interface{})
			if len(clusters) != 2 {
				t.Fatalf("cluster count changed: got %d", len(clusters))
			}
			for i, raw := range clusters {
				cluster := raw.(map[string]interface{})
				old := state["clusters"].([]interface{})[i].(map[string]interface{})
				for _, field := range []string{"ext_id", "vlan_identifier"} {
					if !reflect.DeepEqual(cluster[field], old[field]) {
						t.Errorf("cluster %d %s changed", i, field)
					}
				}
				if _, exists := cluster["gateway_ip_prefix_length"]; exists {
					t.Error("obsolete flat prefix remains")
				}
			}
			if gateway := clusters[0].(map[string]interface{})["gateway_ip_address"]; !reflect.DeepEqual(gateway, test.want) {
				t.Errorf("gateway = %#v, want %#v", gateway, test.want)
			}
			if gateway := clusters[1].(map[string]interface{})["gateway_ip_address"]; len(gateway.([]interface{})) != 0 {
				t.Error("empty gateway was populated")
			}
		})
	}
}

func TestVirtualSwitchV2UpgradeEmptyClusters(t *testing.T) {
	for _, raw := range []string{
		`{"id":"switch-id","name":"vs0"}`,
		`{"id":"switch-id","name":"vs0","clusters":null}`,
		`{"id":"switch-id","name":"vs0","clusters":[]}`,
	} {
		got := upgradeVirtualSwitchStateForTest(t, 0, &tfprotov5.RawState{JSON: []byte(raw)})
		if got["id"] != "switch-id" || len(got["clusters"].([]interface{})) != 0 {
			t.Fatal("empty cluster state or identity changed")
		}
	}
}

func TestVirtualSwitchV2UpgradeNativeState(t *testing.T) {
	raw := []byte(`{"id":"switch-id","name":"vs0","project_ext_id":"project-id","clusters":[{"ext_id":"cluster-id","gateway_ip_address":[{"value":"192.0.2.1","prefix_length":24}],"existing_bridge_name":"br0","vlan_identifier":100,"hosts":[{"ext_id":"host-id","host_nics":["eth0","eth1"],"internal_bridge_name":"br0","active_uplink":"eth0","route_table":100}]}]}`)
	previous := upgradeVirtualSwitchStateForTest(t, 0, &tfprotov5.RawState{JSON: raw})
	current := upgradeVirtualSwitchStateForTest(t, 1, &tfprotov5.RawState{JSON: raw})
	if !reflect.DeepEqual(previous, current) {
		t.Fatal("native upstream version 0 state changed during upgrade")
	}
	if previous["id"] != "switch-id" || previous["project_ext_id"] != "project-id" {
		t.Fatal("native identity or project was lost")
	}
	cluster := previous["clusters"].([]interface{})[0].(map[string]interface{})
	host := cluster["hosts"].([]interface{})[0].(map[string]interface{})
	if cluster["existing_bridge_name"] != "br0" || host["ext_id"] != "host-id" || len(host["host_nics"].([]interface{})) != 2 {
		t.Fatal("native bridge or host configuration was lost")
	}
}

func TestVirtualSwitchV2UpgradeLegacyFlatmap(t *testing.T) {
	got := upgradeVirtualSwitchStateForTest(t, 0, &tfprotov5.RawState{Flatmap: map[string]string{
		"id": "switch-id", "ext_id": "switch-id", "name": "vs0", "mtu": "9000",
		"bond_mode": "ACTIVE_BACKUP", "clusters.#": "1", "clusters.0.ext_id": "cluster-id",
		"clusters.0.vlan_identifier": "100", "clusters.0.gateway_ip_address": "192.0.2.1",
		"clusters.0.gateway_ip_prefix_length": "24",
	}})
	cluster := got["clusters"].([]interface{})[0].(map[string]interface{})
	gateway := cluster["gateway_ip_address"].([]interface{})[0].(map[string]interface{})
	if got["id"] != "switch-id" || got["mtu"] != float64(9000) || cluster["ext_id"] != "cluster-id" || gateway["value"] != "192.0.2.1" || gateway["prefix_length"] != float64(24) {
		t.Fatal("legacy flatmap state was not preserved")
	}
}

func TestVirtualSwitchV2UpgradeRejectsMalformedState(t *testing.T) {
	for _, raw := range []string{
		`{"clusters":{}}`, `{"clusters":[null]}`, `{"clusters":[{"gateway_ip_address":123}]}`,
	} {
		var state map[string]interface{}
		if err := json.Unmarshal([]byte(raw), &state); err != nil {
			t.Fatal(err)
		}
		if _, err := upgradeVirtualSwitchV2StateV0(context.Background(), state, nil); err == nil {
			t.Errorf("malformed state accepted: %s", raw)
		}
	}
}
