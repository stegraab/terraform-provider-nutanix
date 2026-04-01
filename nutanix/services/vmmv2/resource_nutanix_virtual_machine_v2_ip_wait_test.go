package vmmv2

import (
	"testing"

	commonConfig "github.com/nutanix/ntnx-api-golang-clients/vmm-go-client/v4/models/common/v1/config"
	"github.com/nutanix/ntnx-api-golang-clients/vmm-go-client/v4/models/vmm/v4/ahv/config"
)

func TestFirstAvailableIPv4Address(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		vm       config.Vm
		expected string
		found    bool
	}{
		{
			name: "returns learned ip address",
			vm: config.Vm{
				Nics: []config.Nic{
					{
						NetworkInfo: &config.NicNetworkInfo{
							Ipv4Info: &config.Ipv4Info{
								LearnedIpAddresses: []commonConfig.IPv4Address{
									{Value: strPtr("10.1.2.3")},
								},
							},
						},
					},
				},
			},
			expected: "10.1.2.3",
			found:    true,
		},
		{
			name: "returns configured primary ip address",
			vm: config.Vm{
				Nics: []config.Nic{
					{
						NetworkInfo: &config.NicNetworkInfo{
							Ipv4Config: &config.Ipv4Config{
								IpAddress: &commonConfig.IPv4Address{Value: strPtr("192.168.1.20")},
							},
						},
					},
				},
			},
			expected: "192.168.1.20",
			found:    true,
		},
		{
			name: "returns configured secondary ip address",
			vm: config.Vm{
				Nics: []config.Nic{
					{
						NetworkInfo: &config.NicNetworkInfo{
							Ipv4Config: &config.Ipv4Config{
								SecondaryIpAddressList: []commonConfig.IPv4Address{
									{Value: strPtr("172.16.0.10")},
								},
							},
						},
					},
				},
			},
			expected: "172.16.0.10",
			found:    true,
		},
		{
			name: "returns not found when no ips are present",
			vm: config.Vm{
				Nics: []config.Nic{
					{NetworkInfo: &config.NicNetworkInfo{}},
				},
			},
			expected: "",
			found:    false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, ok := firstAvailableIPv4Address(tt.vm)
			if ok != tt.found {
				t.Fatalf("unexpected found result: got %v want %v", ok, tt.found)
			}
			if got != tt.expected {
				t.Fatalf("unexpected ip: got %q want %q", got, tt.expected)
			}
		})
	}
}

func strPtr(s string) *string {
	return &s
}
