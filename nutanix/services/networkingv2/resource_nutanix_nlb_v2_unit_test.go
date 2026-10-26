package networkingv2

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"

	"github.com/hashicorp/go-cty/cty"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
	netclient "github.com/nutanix/ntnx-api-golang-clients/networking-go-client/v4/client"
	networkingsdk "github.com/terraform-providers/terraform-provider-nutanix/nutanix/sdks/v4/networking"
)

func TestNLBTargetOwnership(t *testing.T) {
	resource := ResourceNutanixNLBV2()
	for _, test := range []struct {
		name    string
		config  cty.Value
		managed bool
	}{
		{"omitted", cty.ObjectVal(map[string]cty.Value{"targets_config": cty.NullVal(cty.List(cty.EmptyObject))}), false},
		{"omitted block list", cty.ObjectVal(map[string]cty.Value{"targets_config": cty.ListValEmpty(cty.EmptyObject)}), false},
		{"explicit empty block", cty.ObjectVal(map[string]cty.Value{"targets_config": cty.ListVal([]cty.Value{cty.EmptyObjectVal})}), true},
	} {
		t.Run(test.name, func(t *testing.T) {
			data := resource.Data(&terraform.InstanceState{ID: "nlb", RawConfig: test.config})
			if got := nlbTargetsConfigured(data); got != test.managed {
				t.Fatalf("managed = %v, want %v", got, test.managed)
			}
		})
	}
	if got := expandNLBTargets(nil); got == nil || len(got.NicTargets) != 0 {
		t.Fatal("an omitted target block must create an empty target specification")
	}
}

func TestNLBSnapshotUsesTargetsAndETagFromSameResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("ETag", "revision-2")
		_, _ = w.Write([]byte(`{"data":{"extId":"nlb","targetsConfig":{"nicTargets":[{"virtualNicReference":"new-controller-target","port":6443}]}}}`))
	}))
	defer server.Close()
	parsed, _ := url.Parse(server.URL)
	host, portText, _ := net.SplitHostPort(parsed.Host)
	port, _ := strconv.Atoi(portText)
	client := netclient.NewApiClient()
	client.Scheme, client.Host, client.Port = "http", host, port
	connection := &networkingsdk.Client{APIClientInstance: client}
	snapshot, etag, err := fetchNLBSnapshot(context.Background(), connection, "nlb")
	if err != nil {
		t.Fatal(err)
	}
	if etag != "revision-2" || *snapshot.TargetsConfig.NicTargets[0].VirtualNicReference != "new-controller-target" {
		t.Fatal("expected latest controller target and matching ETag")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, err := fetchNLBSnapshot(ctx, connection, "nlb"); err == nil {
		t.Fatal("cancelled reads must not produce an update snapshot")
	}
}

func TestExpandNLBHealthCheckDefaultsWhenMissing(t *testing.T) {
	got := expandNLBHealthCheck(nil)

	if got == nil {
		t.Fatal("expected health check, got nil")
	}
	if *got.IntervalSecs != 10 {
		t.Fatalf("expected interval 10, got %d", *got.IntervalSecs)
	}
	if *got.TimeoutSecs != 5 {
		t.Fatalf("expected timeout 5, got %d", *got.TimeoutSecs)
	}
	if *got.SuccessThreshold != 3 {
		t.Fatalf("expected success threshold 3, got %d", *got.SuccessThreshold)
	}
	if *got.FailureThreshold != 3 {
		t.Fatalf("expected failure threshold 3, got %d", *got.FailureThreshold)
	}
}

func TestExpandNLBHealthCheckUsesDefaultsForZeroValues(t *testing.T) {
	got := expandNLBHealthCheck([]interface{}{
		map[string]interface{}{
			"interval_secs":     0,
			"timeout_secs":      0,
			"success_threshold": 0,
			"failure_threshold": 0,
		},
	})

	if got == nil {
		t.Fatal("expected health check, got nil")
	}
	if *got.IntervalSecs != 10 {
		t.Fatalf("expected interval 10, got %d", *got.IntervalSecs)
	}
	if *got.TimeoutSecs != 5 {
		t.Fatalf("expected timeout 5, got %d", *got.TimeoutSecs)
	}
	if *got.SuccessThreshold != 3 {
		t.Fatalf("expected success threshold 3, got %d", *got.SuccessThreshold)
	}
	if *got.FailureThreshold != 3 {
		t.Fatalf("expected failure threshold 3, got %d", *got.FailureThreshold)
	}
}

func TestExpandNLBHealthCheckMergesPartialOverrides(t *testing.T) {
	got := expandNLBHealthCheck([]interface{}{
		map[string]interface{}{
			"interval_secs": 7,
		},
	})

	if got == nil {
		t.Fatal("expected health check, got nil")
	}
	if *got.IntervalSecs != 7 {
		t.Fatalf("expected interval 7, got %d", *got.IntervalSecs)
	}
	if *got.TimeoutSecs != 5 {
		t.Fatalf("expected timeout 5, got %d", *got.TimeoutSecs)
	}
	if *got.SuccessThreshold != 3 {
		t.Fatalf("expected success threshold 3, got %d", *got.SuccessThreshold)
	}
	if *got.FailureThreshold != 3 {
		t.Fatalf("expected failure threshold 3, got %d", *got.FailureThreshold)
	}
}

func TestExpandNLBListenerDropsIPv4PrefixLengthWhenZero(t *testing.T) {
	got := expandNLBListener([]interface{}{
		map[string]interface{}{
			"protocol": "TCP",
			"port_ranges": []interface{}{
				map[string]interface{}{
					"start_port": 443,
					"end_port":   443,
				},
			},
			"virtual_ip": []interface{}{
				map[string]interface{}{
					"subnet_reference": "subnet-uuid",
					"assignment_type":  "STATIC",
					"ip_address": []interface{}{
						map[string]interface{}{
							"ipv4": []interface{}{
								map[string]interface{}{
									"value":         "10.128.2.19",
									"prefix_length": 0,
								},
							},
						},
					},
				},
			},
		},
	})

	if got == nil || got.VirtualIP == nil || got.VirtualIP.IpAddress == nil || got.VirtualIP.IpAddress.Ipv4 == nil {
		t.Fatal("expected listener virtual IP with IPv4 address")
	}
	if got.VirtualIP.IpAddress.Ipv4.PrefixLength != nil {
		t.Fatalf("expected nil prefix length, got %v", *got.VirtualIP.IpAddress.Ipv4.PrefixLength)
	}
}

func TestExpandNLBListenerDropsIPv4PrefixLengthWhenProvided(t *testing.T) {
	got := expandNLBListener([]interface{}{
		map[string]interface{}{
			"protocol": "TCP",
			"port_ranges": []interface{}{
				map[string]interface{}{
					"start_port": 443,
					"end_port":   443,
				},
			},
			"virtual_ip": []interface{}{
				map[string]interface{}{
					"subnet_reference": "subnet-uuid",
					"assignment_type":  "STATIC",
					"ip_address": []interface{}{
						map[string]interface{}{
							"ipv4": []interface{}{
								map[string]interface{}{
									"value":         "10.128.2.19",
									"prefix_length": 28,
								},
							},
						},
					},
				},
			},
		},
	})

	if got == nil || got.VirtualIP == nil || got.VirtualIP.IpAddress == nil || got.VirtualIP.IpAddress.Ipv4 == nil {
		t.Fatal("expected listener virtual IP with IPv4 address")
	}
	if got.VirtualIP.IpAddress.Ipv4.PrefixLength != nil {
		t.Fatalf("expected nil prefix length, got %v", *got.VirtualIP.IpAddress.Ipv4.PrefixLength)
	}
}
