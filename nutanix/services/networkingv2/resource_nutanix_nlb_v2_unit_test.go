package networkingv2

import "testing"

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
