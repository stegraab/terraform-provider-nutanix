package vmmv2

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/nutanix/ntnx-api-golang-clients/vmm-go-client/v4/models/vmm/v4/ahv/config"
	"github.com/terraform-providers/terraform-provider-nutanix/utils"
)

func TestVirtualMachineSchemasExposeVtpmDiskID(t *testing.T) {
	t.Parallel()

	tests := map[string]map[string]*schema.Schema{
		"resource":    ResourceNutanixVirtualMachineV2().Schema,
		"data source": DatasourceNutanixVirtualMachineV4().Schema,
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			attribute, ok := test["vtpm_disk_id"]
			if !ok {
				t.Fatal("expected vtpm_disk_id in schema")
			}
			if !attribute.Computed || attribute.Optional || attribute.Required {
				t.Fatalf("expected vtpm_disk_id to be computed-only, got %#v", attribute)
			}
		})
	}
}

func TestFlattenVtpmDiskExtID(t *testing.T) {
	t.Parallel()

	const diskExtID = "7ed4a644-2e6f-4d5e-ae51-a4741fcf9cab"

	tests := map[string]struct {
		config *config.VtpmConfig
		want   string
	}{
		"vTPM device disk": {
			config: &config.VtpmConfig{
				VtpmDevice: &config.VtpmDevice{
					DiskExtId: utils.StringPtr(diskExtID),
				},
			},
			want: diskExtID,
		},
		"vTPM disabled": {},
		"vTPM device absent": {
			config: &config.VtpmConfig{},
		},
		"vTPM disk absent": {
			config: &config.VtpmConfig{VtpmDevice: &config.VtpmDevice{}},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if got := flattenVtpmDiskExtID(test.config); got != test.want {
				t.Fatalf("expected %q, got %q", test.want, got)
			}
		})
	}
}
