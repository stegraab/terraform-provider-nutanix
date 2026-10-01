package filesv4

import (
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func TestFileServerVersionChangeUpdatesInPlace(t *testing.T) {
	versionSchema := ResourceNutanixFileServerV2().Schema["version"]
	if versionSchema.ForceNew {
		t.Fatal("version changes must use the LCM update workflow instead of replacing the file server")
	}
}

func TestValidateFileServerVersionUpdate(t *testing.T) {
	tests := []struct {
		name           string
		current        string
		target         string
		wantUpgrade    bool
		wantErrorMatch string
	}{
		{name: "upgrade", current: "5.3.0.2", target: "5.3.0.3", wantUpgrade: true},
		{name: "same version", current: "5.3.0.3", target: "5.3.0.3"},
		{name: "downgrade", current: "5.3.0.3", target: "5.3.0.2", wantErrorMatch: "downgrading Nutanix Files"},
		{name: "invalid current", current: "not-a-version", target: "5.3.0.3", wantErrorMatch: "invalid current version"},
		{name: "invalid target", current: "5.3.0.2", target: "not-a-version", wantErrorMatch: "invalid target version"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			gotUpgrade, err := validateFileServerVersionUpdate(test.current, test.target)
			if gotUpgrade != test.wantUpgrade {
				t.Fatalf("upgrade decision: got %t, want %t", gotUpgrade, test.wantUpgrade)
			}
			if test.wantErrorMatch == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), test.wantErrorMatch) {
				t.Fatalf("error: got %v, want text %q", err, test.wantErrorMatch)
			}
		})
	}
}

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

func TestExpandFileServerUpdatePayloadUsesActivePlacementDuringFailover(t *testing.T) {
	resourceSchema := ResourceNutanixFileServerV2().Schema
	d := schema.TestResourceDataRaw(t, resourceSchema, map[string]interface{}{
		"name":              "files",
		"cluster_ext_id":    "dc1",
		"cvm_ip_addresses":  []interface{}{map[string]interface{}{"value": "10.0.0.10"}},
		"dns_servers":       []interface{}{map[string]interface{}{"value": "10.112.130.12"}},
		"ntp_servers":       []interface{}{map[string]interface{}{"fqdn": "ntp.example.com"}},
		"external_networks": []interface{}{map[string]interface{}{"network_ext_id": "dc1-external"}},
		"internal_networks": []interface{}{map[string]interface{}{"network_ext_id": "dc1-internal"}},
	})
	current := map[string]interface{}{
		"clusterExtId":   "dc2",
		"cvmIpAddresses": []interface{}{map[string]interface{}{"value": "10.1.0.10"}},
		"externalNetworks": []interface{}{
			map[string]interface{}{"networkExtId": "dc2-external"},
		},
		"internalNetworks": []interface{}{
			map[string]interface{}{"networkExtId": "dc2-internal"},
		},
	}

	payload := expandFileServerUpdatePayload(d, current)

	if got := payload["clusterExtId"]; got != "dc2" {
		t.Fatalf("clusterExtId: got %q, want active cluster %q", got, "dc2")
	}
	if got := payload["dnsServers"].([]map[string]interface{})[0]["value"]; got != "10.112.130.12" {
		t.Fatalf("dnsServers: got %q, want configured DNS server", got)
	}
	if got := payload["externalNetworks"].([]interface{})[0].(map[string]interface{})["networkExtId"]; got != "dc2-external" {
		t.Fatalf("externalNetworks: got %q, want active network", got)
	}
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
