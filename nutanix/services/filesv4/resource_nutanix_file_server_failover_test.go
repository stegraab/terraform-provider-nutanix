package filesv4

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func TestFlattenFileServerToStatePreservesHomePlacementDuringFailover(t *testing.T) {
	resourceSchema := ResourceNutanixFileServerV2().Schema
	d := schema.TestResourceDataRaw(t, resourceSchema, map[string]interface{}{
		"cluster_ext_id":   "dc1",
		"cvm_ip_addresses": []interface{}{map[string]interface{}{"value": "10.0.0.10"}},
		"external_networks": []interface{}{map[string]interface{}{
			"network_ext_id": "dc1-external",
		}},
		"internal_networks": []interface{}{map[string]interface{}{
			"network_ext_id": "dc1-internal",
		}},
	})
	d.SetId("file-server")

	flattenFileServerToState(d, map[string]interface{}{
		"extId":        "file-server",
		"clusterExtId": "dc2",
		"cvmIpAddresses": []interface{}{
			map[string]interface{}{"value": "10.1.0.10"},
		},
		"externalNetworks": []interface{}{
			map[string]interface{}{"networkExtId": "dc2-external", "isManaged": true},
		},
		"internalNetworks": []interface{}{
			map[string]interface{}{"networkExtId": "dc2-internal", "isManaged": true},
		},
	})

	if got := d.Get("cluster_ext_id").(string); got != "dc1" {
		t.Fatalf("cluster_ext_id changed during failover: got %q, want %q", got, "dc1")
	}
	assertFirstNetworkExtID(t, d, "external_networks", "dc1-external")
	assertFirstNetworkExtID(t, d, "internal_networks", "dc1-internal")
	if got := d.Get("cvm_ip_addresses").([]interface{})[0].(map[string]interface{})["value"]; got != "10.0.0.10" {
		t.Fatalf("cvm_ip_addresses changed during failover: got %q", got)
	}
}

func TestFlattenFileServerToStateRefreshesPlacementOnHomeCluster(t *testing.T) {
	resourceSchema := ResourceNutanixFileServerV2().Schema
	d := schema.TestResourceDataRaw(t, resourceSchema, map[string]interface{}{
		"cluster_ext_id":   "dc1",
		"cvm_ip_addresses": []interface{}{map[string]interface{}{"value": "10.0.0.10"}},
		"external_networks": []interface{}{map[string]interface{}{
			"network_ext_id": "old-external",
		}},
		"internal_networks": []interface{}{map[string]interface{}{
			"network_ext_id": "old-internal",
		}},
	})
	d.SetId("file-server")

	flattenFileServerToState(d, map[string]interface{}{
		"extId":        "file-server",
		"clusterExtId": "dc1",
		"cvmIpAddresses": []interface{}{
			map[string]interface{}{"value": "10.0.0.11"},
		},
		"externalNetworks": []interface{}{
			map[string]interface{}{"networkExtId": "new-external", "isManaged": true},
		},
		"internalNetworks": []interface{}{
			map[string]interface{}{"networkExtId": "new-internal", "isManaged": true},
		},
	})

	assertFirstNetworkExtID(t, d, "external_networks", "new-external")
	assertFirstNetworkExtID(t, d, "internal_networks", "new-internal")
	if got := d.Get("cvm_ip_addresses").([]interface{})[0].(map[string]interface{})["value"]; got != "10.0.0.11" {
		t.Fatalf("cvm_ip_addresses was not refreshed: got %q", got)
	}
}

func TestFlattenFileServerReplicationPolicyPreservesConfiguredDirectionDuringFailover(t *testing.T) {
	resourceSchema := ResourceNutanixFileServerReplicationPolicyV2().Schema
	d := schema.TestResourceDataRaw(t, resourceSchema, map[string]interface{}{
		"name":                         "files-metro",
		"type":                         "METRO",
		"primary_file_server_ext_id":   "files-dc1",
		"secondary_file_server_ext_id": "files-dc2",
		"primary_cluster_ext_id":       "dc1",
		"secondary_cluster_ext_id":     "dc2",
	})
	d.SetId("replication-policy")

	flattenFileServerReplicationPolicyToState(d, map[string]interface{}{
		"extId": "replication-policy",
		"name":  "files-metro",
		"type":  "METRO",
		"replicationConfigurations": []interface{}{map[string]interface{}{
			"primaryFileServerExtId":   "files-dc2",
			"secondaryFileServerExtId": "files-dc1",
			"primaryClusterExtId":      "dc2",
			"secondaryClusterExtId":    "dc1",
		}},
	})

	assertStringState(t, d, "primary_file_server_ext_id", "files-dc1")
	assertStringState(t, d, "secondary_file_server_ext_id", "files-dc2")
	assertStringState(t, d, "primary_cluster_ext_id", "dc1")
	assertStringState(t, d, "secondary_cluster_ext_id", "dc2")
}

func assertFirstNetworkExtID(t *testing.T, d *schema.ResourceData, key, want string) {
	t.Helper()
	networks := d.Get(key).([]interface{})
	if len(networks) != 1 {
		t.Fatalf("%s has %d entries, want 1", key, len(networks))
	}
	if got := networks[0].(map[string]interface{})["network_ext_id"]; got != want {
		t.Fatalf("%s network_ext_id: got %q, want %q", key, got, want)
	}
}

func assertStringState(t *testing.T, d *schema.ResourceData, key, want string) {
	t.Helper()
	if got := d.Get(key).(string); got != want {
		t.Fatalf("%s: got %q, want %q", key, got, want)
	}
}
