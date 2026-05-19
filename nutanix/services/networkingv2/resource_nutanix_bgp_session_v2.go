package networkingv2

import (
	"context"
	"time"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/nutanix/ntnx-api-golang-clients/networking-go-client/v4/models/networking/v4/config"
	networkingPrism "github.com/nutanix/ntnx-api-golang-clients/networking-go-client/v4/models/prism/v4/config"
	conns "github.com/terraform-providers/terraform-provider-nutanix/nutanix"
	"github.com/terraform-providers/terraform-provider-nutanix/utils"
)

func ResourceNutanixBgpSessionV2() *schema.Resource {
	return &schema.Resource{
		CreateContext: resourceNutanixBgpSessionV2Create,
		ReadContext:   resourceNutanixBgpSessionV2Read,
		DeleteContext: resourceNutanixBgpSessionV2Delete,
		Importer: &schema.ResourceImporter{
			StateContext: schema.ImportStatePassthroughContext,
		},
		Timeouts: &schema.ResourceTimeout{
			Create: schema.DefaultTimeout(15 * time.Minute),
			Read:   schema.DefaultTimeout(15 * time.Minute),
			Delete: schema.DefaultTimeout(15 * time.Minute),
		},
		Schema: map[string]*schema.Schema{
			"name": {
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
			},
			"local_gateway_reference": {
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
			},
			"remote_gateway_reference": {
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
			},
			"local_gateway_interface_ip_address": {
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
			},
			"dynamic_route_priority": {
				Type:     schema.TypeInt,
				Required: true,
				ForceNew: true,
			},
			"should_advertise_all_externally_routable_prefixes": {
				Type:     schema.TypeBool,
				Optional: true,
				Default:  false,
				ForceNew: true,
			},
			"ext_id": {
				Type:     schema.TypeString,
				Computed: true,
			},
			"tenant_id": {
				Type:     schema.TypeString,
				Computed: true,
			},
		},
	}
}

func resourceNutanixBgpSessionV2Create(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	conn := meta.(*conns.Client).NetworkingAPI
	name := d.Get("name").(string)

	body := expandBgpSession(d)
	resp, err := conn.BgpSessionsAPIInstance.CreateBgpSession(body)
	if err != nil {
		return diag.Errorf("error while creating BGP session %q: %v", name, err)
	}

	taskRef, ok := resp.Data.GetValue().(networkingPrism.TaskReference)
	if !ok {
		return diag.Errorf("error: unexpected response type from create BGP session API, expected TaskReference")
	}
	if err := waitForNetworkingTask(ctx, d, meta, taskRef.ExtId, schema.TimeoutCreate, "BGP session create"); err != nil {
		return diag.FromErr(err)
	}

	created, err := findBgpSessionByName(conn, name)
	if err != nil {
		return diag.FromErr(err)
	}
	if created == nil {
		return diag.Errorf("BGP session %q was created but could not be found by name", name)
	}
	d.SetId(utils.StringValue(created.ExtId))
	return resourceNutanixBgpSessionV2Read(ctx, d, meta)
}

func resourceNutanixBgpSessionV2Read(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	conn := meta.(*conns.Client).NetworkingAPI
	resp, err := conn.BgpSessionsAPIInstance.GetBgpSessionById(utils.StringPtr(d.Id()))
	if err != nil {
		if isNetworkingNotFoundError(err) {
			d.SetId("")
			return nil
		}
		return diag.Errorf("error while fetching BGP session %q: %v", d.Id(), err)
	}
	session, ok := resp.Data.GetValue().(config.BgpSession)
	if !ok {
		return diag.Errorf("error: unexpected response type from get BGP session API, expected BgpSession")
	}

	_ = d.Set("name", utils.StringValue(session.Name))
	_ = d.Set("local_gateway_reference", utils.StringValue(session.LocalGatewayReference))
	_ = d.Set("remote_gateway_reference", utils.StringValue(session.RemoteGatewayReference))
	if session.LocalGatewayInterfaceIpAddress != nil && session.LocalGatewayInterfaceIpAddress.Ipv4 != nil {
		_ = d.Set("local_gateway_interface_ip_address", utils.StringValue(session.LocalGatewayInterfaceIpAddress.Ipv4.Value))
	}
	_ = d.Set("dynamic_route_priority", utils.IntValue(session.DynamicRoutePriority))
	_ = d.Set("should_advertise_all_externally_routable_prefixes", utils.BoolValue(session.ShouldAdvertiseAllExternallyRoutablePrefixes))
	_ = d.Set("ext_id", utils.StringValue(session.ExtId))
	_ = d.Set("tenant_id", utils.StringValue(session.TenantId))
	return nil
}

func resourceNutanixBgpSessionV2Delete(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	conn := meta.(*conns.Client).NetworkingAPI
	resp, err := conn.BgpSessionsAPIInstance.DeleteBgpSessionById(utils.StringPtr(d.Id()))
	if err != nil {
		if isNetworkingNotFoundError(err) {
			d.SetId("")
			return nil
		}
		return diag.Errorf("error while deleting BGP session %q: %v", d.Id(), err)
	}

	taskRef, ok := resp.Data.GetValue().(networkingPrism.TaskReference)
	if !ok {
		return diag.Errorf("error: unexpected response type from delete BGP session API, expected TaskReference")
	}
	if err := waitForNetworkingTask(ctx, d, meta, taskRef.ExtId, schema.TimeoutDelete, "BGP session delete"); err != nil {
		return diag.FromErr(err)
	}
	d.SetId("")
	return nil
}

func expandBgpSession(d *schema.ResourceData) *config.BgpSession {
	session := config.NewBgpSession()
	session.Name = utils.StringPtr(d.Get("name").(string))
	session.LocalGatewayReference = utils.StringPtr(d.Get("local_gateway_reference").(string))
	session.RemoteGatewayReference = utils.StringPtr(d.Get("remote_gateway_reference").(string))
	session.LocalGatewayInterfaceIpAddress = ipv4Address(d.Get("local_gateway_interface_ip_address").(string), 32)
	session.DynamicRoutePriority = utils.IntPtr(d.Get("dynamic_route_priority").(int))
	session.ShouldAdvertiseAllExternallyRoutablePrefixes = utils.BoolPtr(d.Get("should_advertise_all_externally_routable_prefixes").(bool))
	return session
}
