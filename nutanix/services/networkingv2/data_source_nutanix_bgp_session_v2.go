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

func DataSourceNutanixBgpSessionV2() *schema.Resource {
	return &schema.Resource{
		ReadContext: dataSourceNutanixBgpSessionV2Read,
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
			"local_gateway_reference": {
				Type:     schema.TypeString,
				Computed: true,
			},
			"remote_gateway_reference": {
				Type:     schema.TypeString,
				Computed: true,
			},
			"local_gateway_interface_ip_address": {
				Type:     schema.TypeString,
				Computed: true,
			},
			"dynamic_route_priority": {
				Type:     schema.TypeInt,
				Computed: true,
			},
			"should_advertise_all_externally_routable_prefixes": {
				Type:     schema.TypeBool,
				Computed: true,
			},
			"tenant_id": {
				Type:     schema.TypeString,
				Computed: true,
			},
		},
	}
}

func dataSourceNutanixBgpSessionV2Read(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	conn := meta.(*conns.Client).NetworkingAPI

	var session *config.BgpSession
	if extID, ok := d.GetOk("ext_id"); ok {
		resp, err := conn.BgpSessionsAPIInstance.GetBgpSessionById(utils.StringPtr(extID.(string)))
		if err != nil {
			return diag.Errorf("error while fetching BGP session %q: %v", extID.(string), err)
		}
		value, ok := resp.Data.GetValue().(config.BgpSession)
		if !ok {
			return diag.Errorf("error: unexpected response type from get BGP session API, expected BgpSession")
		}
		session = &value
	} else {
		value, err := findBgpSessionByName(conn, d.Get("name").(string))
		if err != nil {
			return diag.FromErr(err)
		}
		if value == nil {
			return diag.Errorf("BGP session %q was not found", d.Get("name").(string))
		}
		session = value
	}

	d.SetId(utils.StringValue(session.ExtId))
	_ = d.Set("ext_id", utils.StringValue(session.ExtId))
	_ = d.Set("name", utils.StringValue(session.Name))
	_ = d.Set("local_gateway_reference", utils.StringValue(session.LocalGatewayReference))
	_ = d.Set("remote_gateway_reference", utils.StringValue(session.RemoteGatewayReference))
	if session.LocalGatewayInterfaceIpAddress != nil && session.LocalGatewayInterfaceIpAddress.Ipv4 != nil {
		_ = d.Set("local_gateway_interface_ip_address", utils.StringValue(session.LocalGatewayInterfaceIpAddress.Ipv4.Value))
	}
	_ = d.Set("dynamic_route_priority", utils.IntValue(session.DynamicRoutePriority))
	_ = d.Set("should_advertise_all_externally_routable_prefixes", utils.BoolValue(session.ShouldAdvertiseAllExternallyRoutablePrefixes))
	_ = d.Set("tenant_id", utils.StringValue(session.TenantId))
	return nil
}

func findBgpSessionByName(conn *networkingClient.Client, name string) (*config.BgpSession, error) {
	matches := make([]config.BgpSession, 0, 1)
	page := 0
	limit := 100

	for {
		resp, err := conn.BgpSessionsAPIInstance.ListBgpSessions(&page, &limit, nil, nil, nil)
		if err != nil {
			return nil, fmt.Errorf("error while listing BGP sessions: %w", err)
		}
		if resp.Data == nil {
			break
		}
		sessions, ok := resp.Data.GetValue().([]config.BgpSession)
		if !ok {
			return nil, fmt.Errorf("unexpected response type from list BGP sessions API")
		}
		for i := range sessions {
			if utils.StringValue(sessions[i].Name) == name {
				matches = append(matches, sessions[i])
			}
		}
		if len(sessions) < limit {
			break
		}
		page++
	}

	if len(matches) == 0 {
		return nil, nil
	}
	if len(matches) > 1 {
		return nil, fmt.Errorf("found multiple BGP sessions named %q", name)
	}
	return &matches[0], nil
}
