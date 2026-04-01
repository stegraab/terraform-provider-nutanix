package networkingv2

import (
	"testing"

	"github.com/nutanix/ntnx-api-golang-clients/networking-go-client/v4/models/common/v1/config"
)

func TestFlattenNtpServer_SortsIPv4Addresses(t *testing.T) {
	in := []config.IPAddress{
		{
			Ipv4: &config.IPv4Address{
				Value: strPtr("10.66.0.87"),
			},
		},
		{
			Ipv4: &config.IPv4Address{
				Value: strPtr("10.66.0.164"),
			},
		},
	}

	out := flattenNtpServer(in)
	if len(out) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(out))
	}

	first := *(out[0]["ipv4"].([]interface{})[0].(map[string]interface{})["value"].(*string))
	second := *(out[1]["ipv4"].([]interface{})[0].(map[string]interface{})["value"].(*string))

	if first != "10.66.0.164" || second != "10.66.0.87" {
		t.Fatalf("unexpected order: first=%v second=%v", first, second)
	}
}

func strPtr(s string) *string {
	return &s
}
