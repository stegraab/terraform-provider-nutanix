package vmmv2

import (
	"context"
	"time"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"
	"github.com/nutanix/ntnx-api-golang-clients/vmm-go-client/v4/models/vmm/v4/ahv/config"
	conns "github.com/terraform-providers/terraform-provider-nutanix/nutanix"
	"github.com/terraform-providers/terraform-provider-nutanix/utils"
)

func ResourceNutanixVMPowerActionV2() *schema.Resource {
	return &schema.Resource{
		CreateContext: ResourceNutanixVMPowerActionV2Create,
		ReadContext:   ResourceNutanixVMPowerActionV2Read,
		DeleteContext: ResourceNutanixVMPowerActionV2Delete,
		Timeouts: &schema.ResourceTimeout{
			Create: schema.DefaultTimeout(30 * time.Minute),
			Update: schema.DefaultTimeout(30 * time.Minute),
		},
		Schema: map[string]*schema.Schema{
			"ext_id": {
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
			},
			"action": {
				Type:         schema.TypeString,
				Optional:     true,
				Default:      "power_on",
				ForceNew:     true,
				ValidateFunc: validation.StringInSlice([]string{"power_on", "power_off"}, false),
			},
			"power_state": {
				Type:     schema.TypeString,
				Computed: true,
			},
		},
	}
}

func ResourceNutanixVMPowerActionV2Create(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	conn := meta.(*conns.Client).VmmAPI
	vmExtID := d.Get("ext_id").(string)

	d.SetId(vmExtID)

	switch d.Get("action").(string) {
	case "power_on":
		if diagErr := callForPowerOnVM(ctx, conn, d, meta); diagErr != nil {
			return diagErr
		}
	case "power_off":
		if diagErr := callForPowerOffVM(ctx, conn, d, meta); diagErr != nil {
			return diagErr
		}
	default:
		return diag.Errorf("unsupported VM power action %q", d.Get("action").(string))
	}

	return ResourceNutanixVMPowerActionV2Read(ctx, d, meta)
}

func ResourceNutanixVMPowerActionV2Read(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	conn := meta.(*conns.Client).VmmAPI
	vmExtID := d.Get("ext_id").(string)

	readResp, err := conn.VMAPIInstance.GetVmById(utils.StringPtr(vmExtID))
	if err != nil {
		return diag.Errorf("error while reading VM power state: %v", err)
	}

	vmResp := readResp.Data.GetValue().(config.Vm)
	if err := d.Set("power_state", vmResp.PowerState.GetName()); err != nil {
		return diag.FromErr(err)
	}

	return nil
}

func ResourceNutanixVMPowerActionV2Delete(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	return nil
}
