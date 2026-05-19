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

func DataSourceNutanixGatewayV2() *schema.Resource {
	return &schema.Resource{
		ReadContext: dataSourceNutanixGatewayV2Read,
		Schema: map[string]*schema.Schema{
			"ext_id": {
				Type:         schema.TypeString,
				Optional:     true,
				Computed:     true,
				ExactlyOneOf: []string{"ext_id", "name"},
			},
			"name": {
				Type:         schema.TypeString,
				Optional:     true,
				Computed:     true,
				ExactlyOneOf: []string{"ext_id", "name"},
			},
			"remote_bgp_service": {
				Type:     schema.TypeList,
				Computed: true,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"address": {
							Type:     schema.TypeString,
							Computed: true,
						},
						"prefix_length": {
							Type:     schema.TypeInt,
							Computed: true,
						},
						"asn": {
							Type:     schema.TypeInt,
							Computed: true,
						},
					},
				},
			},
			"tenant_id": {
				Type:     schema.TypeString,
				Computed: true,
			},
		},
	}
}

func dataSourceNutanixGatewayV2Read(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	conn := meta.(*conns.Client).NetworkingAPI

	var gateway *config.Gateway
	if extID, ok := d.GetOk("ext_id"); ok {
		resp, err := conn.GatewaysAPIInstance.GetGatewayById(utils.StringPtr(extID.(string)))
		if err != nil {
			return diag.Errorf("error while fetching gateway %q: %v", extID.(string), err)
		}
		value, ok := resp.Data.GetValue().(config.Gateway)
		if !ok {
			return diag.Errorf("error: unexpected response type from get gateway API, expected Gateway")
		}
		gateway = &value
	} else {
		value, err := findGatewayByName(conn, d.Get("name").(string))
		if err != nil {
			return diag.FromErr(err)
		}
		if value == nil {
			return diag.Errorf("gateway %q was not found", d.Get("name").(string))
		}
		gateway = value
	}

	d.SetId(utils.StringValue(gateway.ExtId))
	_ = d.Set("ext_id", utils.StringValue(gateway.ExtId))
	_ = d.Set("name", utils.StringValue(gateway.Name))
	_ = d.Set("tenant_id", utils.StringValue(gateway.TenantId))
	if services, ok := gateway.GetServices().(config.RemoteNetworkServices); ok && services.RemoteBgpService != nil {
		_ = d.Set("remote_bgp_service", flattenRemoteBgpService(services.RemoteBgpService))
	}
	return nil
}

func findGatewayByName(conn *networkingClient.Client, name string) (*config.Gateway, error) {
	matches := make([]config.Gateway, 0, 1)
	page := 0
	limit := 100

	for {
		resp, err := conn.GatewaysAPIInstance.ListGateways(&page, &limit, nil, nil, nil, nil)
		if err != nil {
			return nil, fmt.Errorf("error while listing gateways: %w", err)
		}
		if resp.Data == nil {
			break
		}
		gateways, ok := resp.Data.GetValue().([]config.Gateway)
		if !ok {
			return nil, fmt.Errorf("unexpected response type from list gateways API")
		}
		for i := range gateways {
			if utils.StringValue(gateways[i].Name) == name {
				matches = append(matches, gateways[i])
			}
		}
		if len(gateways) < limit {
			break
		}
		page++
	}

	if len(matches) == 0 {
		return nil, nil
	}
	if len(matches) > 1 {
		return nil, fmt.Errorf("found multiple gateways named %q", name)
	}
	return &matches[0], nil
}
