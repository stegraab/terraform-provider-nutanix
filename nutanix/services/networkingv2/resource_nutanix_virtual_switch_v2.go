package networkingv2

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/nutanix/ntnx-api-golang-clients/networking-go-client/v4/models/networking/v4/config"
	conns "github.com/terraform-providers/terraform-provider-nutanix/nutanix"
	networkingClient "github.com/terraform-providers/terraform-provider-nutanix/nutanix/sdks/v4/networking"
	"github.com/terraform-providers/terraform-provider-nutanix/utils"
)

func ResourceNutanixVirtualSwitchV2() *schema.Resource {
	return &schema.Resource{
		CreateContext: resourceNutanixVirtualSwitchV2ImportOnly,
		ReadContext:   resourceNutanixVirtualSwitchV2Read,
		UpdateContext: resourceNutanixVirtualSwitchV2ImportOnly,
		DeleteContext: resourceNutanixVirtualSwitchV2ImportOnly,
		Importer: &schema.ResourceImporter{
			StateContext: schema.ImportStatePassthroughContext,
		},
		Schema: virtualSwitchSchemaV2(false),
	}
}

func DataSourceNutanixVirtualSwitchV2() *schema.Resource {
	return &schema.Resource{
		ReadContext: dataSourceNutanixVirtualSwitchV2Read,
		Schema:      virtualSwitchSchemaV2(true),
	}
}

func virtualSwitchSchemaV2(dataSource bool) map[string]*schema.Schema {
	s := map[string]*schema.Schema{
		"ext_id": {
			Type:     schema.TypeString,
			Optional: dataSource,
			Computed: true,
		},
		"name": {
			Type:     schema.TypeString,
			Optional: true,
			Computed: true,
		},
		"description": {
			Type:     schema.TypeString,
			Optional: !dataSource,
			Computed: true,
		},
		"bond_mode": {
			Type:     schema.TypeString,
			Optional: !dataSource,
			Computed: true,
		},
		"mtu": {
			Type:     schema.TypeInt,
			Optional: !dataSource,
			Computed: true,
		},
		"owner_type": {
			Type:     schema.TypeString,
			Computed: true,
		},
		"is_default": {
			Type:     schema.TypeBool,
			Computed: true,
		},
		"has_delete_in_progress": {
			Type:     schema.TypeBool,
			Computed: true,
		},
		"has_deployment_error": {
			Type:     schema.TypeBool,
			Computed: true,
		},
		"has_update_in_progress": {
			Type:     schema.TypeBool,
			Computed: true,
		},
		"tenant_id": {
			Type:     schema.TypeString,
			Computed: true,
		},
		"clusters": {
			Type:     schema.TypeList,
			Computed: true,
			Elem: &schema.Resource{
				Schema: map[string]*schema.Schema{
					"ext_id": {
						Type:     schema.TypeString,
						Computed: true,
					},
					"vlan_identifier": {
						Type:     schema.TypeInt,
						Computed: true,
					},
					"gateway_ip_address": {
						Type:     schema.TypeString,
						Computed: true,
					},
					"gateway_ip_prefix_length": {
						Type:     schema.TypeInt,
						Computed: true,
					},
				},
			},
		},
	}

	if dataSource {
		s["ext_id"].ExactlyOneOf = []string{"ext_id", "name"}
		s["name"].ExactlyOneOf = []string{"ext_id", "name"}
	}

	return s
}

func resourceNutanixVirtualSwitchV2ImportOnly(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	return diag.Errorf("nutanix_virtual_switch_v2 is import-only; import the existing switch and manage its references without applying create/update/delete")
}

func resourceNutanixVirtualSwitchV2Read(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	conn := meta.(*conns.Client).NetworkingAPI
	virtualSwitch, err := getVirtualSwitchByID(conn, d.Id())
	if err != nil {
		if isNetworkingNotFoundError(err) {
			d.SetId("")
			return nil
		}
		return diag.Errorf("error while fetching virtual switch %q: %v", d.Id(), err)
	}
	return setVirtualSwitchFields(d, virtualSwitch)
}

func dataSourceNutanixVirtualSwitchV2Read(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	conn := meta.(*conns.Client).NetworkingAPI

	var virtualSwitch *config.VirtualSwitch
	if extID, ok := d.GetOk("ext_id"); ok {
		value, err := getVirtualSwitchByID(conn, extID.(string))
		if err != nil {
			return diag.Errorf("error while fetching virtual switch %q: %v", extID.(string), err)
		}
		virtualSwitch = value
	} else {
		value, err := findVirtualSwitchByName(conn, d.Get("name").(string))
		if err != nil {
			return diag.FromErr(err)
		}
		if value == nil {
			return diag.Errorf("virtual switch %q was not found", d.Get("name").(string))
		}
		virtualSwitch = value
	}

	d.SetId(utils.StringValue(virtualSwitch.ExtId))
	return setVirtualSwitchFields(d, virtualSwitch)
}

func getVirtualSwitchByID(conn *networkingClient.Client, extID string) (*config.VirtualSwitch, error) {
	resp, err := conn.VirtualSwitchesAPIInstance.GetVirtualSwitchById(utils.StringPtr(extID), nil)
	if err != nil {
		return nil, err
	}
	value, ok := resp.Data.GetValue().(config.VirtualSwitch)
	if !ok {
		return nil, fmt.Errorf("unexpected response type from get virtual switch API")
	}
	return &value, nil
}

func findVirtualSwitchByName(conn *networkingClient.Client, name string) (*config.VirtualSwitch, error) {
	matches := make([]config.VirtualSwitch, 0, 1)
	page := 0
	limit := 100

	for {
		resp, err := conn.VirtualSwitchesAPIInstance.ListVirtualSwitches(nil, &page, &limit, nil, nil)
		if err != nil {
			return nil, fmt.Errorf("error while listing virtual switches: %w", err)
		}
		if resp.Data == nil {
			break
		}
		virtualSwitches, ok := resp.Data.GetValue().([]config.VirtualSwitch)
		if !ok {
			return nil, fmt.Errorf("unexpected response type from list virtual switches API")
		}
		for i := range virtualSwitches {
			if utils.StringValue(virtualSwitches[i].Name) == name {
				matches = append(matches, virtualSwitches[i])
			}
		}
		if len(virtualSwitches) < limit {
			break
		}
		page++
	}

	if len(matches) == 0 {
		return nil, nil
	}
	if len(matches) > 1 {
		return nil, fmt.Errorf("found multiple virtual switches named %q", name)
	}
	return &matches[0], nil
}

func setVirtualSwitchFields(d *schema.ResourceData, virtualSwitch *config.VirtualSwitch) diag.Diagnostics {
	_ = d.Set("ext_id", utils.StringValue(virtualSwitch.ExtId))
	_ = d.Set("name", utils.StringValue(virtualSwitch.Name))
	_ = d.Set("description", utils.StringValue(virtualSwitch.Description))
	if virtualSwitch.BondMode != nil {
		_ = d.Set("bond_mode", virtualSwitch.BondMode.GetName())
	}
	_ = d.Set("mtu", int(utils.Int64Value(virtualSwitch.Mtu)))
	if virtualSwitch.OwnerType != nil {
		_ = d.Set("owner_type", virtualSwitch.OwnerType.GetName())
	}
	_ = d.Set("is_default", utils.BoolValue(virtualSwitch.IsDefault))
	_ = d.Set("has_delete_in_progress", utils.BoolValue(virtualSwitch.HasDeleteInProgress))
	_ = d.Set("has_deployment_error", utils.BoolValue(virtualSwitch.HasDeploymentError))
	_ = d.Set("has_update_in_progress", utils.BoolValue(virtualSwitch.HasUpdateInProgress))
	_ = d.Set("tenant_id", utils.StringValue(virtualSwitch.TenantId))
	_ = d.Set("clusters", flattenVirtualSwitchClustersV2(virtualSwitch.Clusters))
	return nil
}

func flattenVirtualSwitchClustersV2(clusters []config.Cluster) []interface{} {
	result := make([]interface{}, 0, len(clusters))
	for _, cluster := range clusters {
		item := map[string]interface{}{
			"ext_id":          utils.StringValue(cluster.ExtId),
			"vlan_identifier": utils.IntValue(cluster.VlanIdentifier),
		}
		if cluster.GatewayIpAddress != nil {
			item["gateway_ip_address"] = utils.StringValue(cluster.GatewayIpAddress.Value)
			item["gateway_ip_prefix_length"] = utils.IntValue(cluster.GatewayIpAddress.PrefixLength)
		}
		result = append(result, item)
	}
	return result
}
