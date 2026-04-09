package vmmv2

import (
	"testing"

	import4 "github.com/nutanix/ntnx-api-golang-clients/vmm-go-client/v4/models/common/v1/config"
	"github.com/nutanix/ntnx-api-golang-clients/vmm-go-client/v4/models/vmm/v4/ahv/config"
	"github.com/terraform-providers/terraform-provider-nutanix/utils"
)

func TestFlattenIpv4ConfigNormalizesStaticIPAssignment(t *testing.T) {
	t.Parallel()

	got := flattenIpv4Config(&config.Ipv4Config{
		ShouldAssignIp: utils.BoolPtr(false),
		IpAddress: &import4.IPv4Address{
			Value: utils.StringPtr("10.128.254.254"),
		},
	})

	if len(got) != 1 {
		t.Fatalf("expected one ipv4_config entry, got %d", len(got))
	}

	assignIP, ok := got[0]["should_assign_ip"].(bool)
	if !ok {
		t.Fatalf("expected bool should_assign_ip, got %#v", got[0]["should_assign_ip"])
	}
	if !assignIP {
		t.Fatalf("expected should_assign_ip=true for static IP config, got %#v", got[0]["should_assign_ip"])
	}
}
