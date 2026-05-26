package networkingv2

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	networkingCommon "github.com/nutanix/ntnx-api-golang-clients/networking-go-client/v4/models/common/v1/config"
	"github.com/nutanix/ntnx-api-golang-clients/networking-go-client/v4/models/networking/v4/config"
	networkingPrism "github.com/nutanix/ntnx-api-golang-clients/networking-go-client/v4/models/prism/v4/config"
	conns "github.com/terraform-providers/terraform-provider-nutanix/nutanix"
	"github.com/terraform-providers/terraform-provider-nutanix/nutanix/common"
	"github.com/terraform-providers/terraform-provider-nutanix/utils"
)

func ResourceNutanixGatewayV2() *schema.Resource {
	return &schema.Resource{
		CreateContext: resourceNutanixGatewayV2Create,
		ReadContext:   resourceNutanixGatewayV2Read,
		DeleteContext: resourceNutanixGatewayV2Delete,
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
			"remote_bgp_service": {
				Type:     schema.TypeList,
				Optional: true,
				ForceNew: true,
				MaxItems: 1,
				ExactlyOneOf: []string{
					"remote_bgp_service",
					"local_bgp_service",
				},
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"address": {
							Type:     schema.TypeString,
							Required: true,
							ForceNew: true,
						},
						"prefix_length": {
							Type:     schema.TypeInt,
							Optional: true,
							Default:  32,
							ForceNew: true,
						},
						"asn": {
							Type:     schema.TypeInt,
							Required: true,
							ForceNew: true,
						},
					},
				},
			},
			"local_bgp_service": {
				Type:     schema.TypeList,
				Optional: true,
				ForceNew: true,
				MaxItems: 1,
				ExactlyOneOf: []string{
					"remote_bgp_service",
					"local_bgp_service",
				},
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"vpc_reference": {
							Type:     schema.TypeString,
							Required: true,
							ForceNew: true,
						},
						"asn": {
							Type:     schema.TypeInt,
							Required: true,
							ForceNew: true,
						},
						"is_bgp_add_path_enabled": {
							Type:     schema.TypeBool,
							Optional: true,
							Default:  false,
							ForceNew: true,
						},
					},
				},
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

func resourceNutanixGatewayV2Create(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	conn := meta.(*conns.Client).NetworkingAPI
	name := d.Get("name").(string)

	body := expandGateway(d)
	resp, err := conn.GatewaysAPIInstance.CreateGateway(body)
	if err != nil {
		return diag.Errorf("error while creating gateway %q: %v", name, err)
	}

	taskRef, ok := resp.Data.GetValue().(networkingPrism.TaskReference)
	if !ok {
		return diag.Errorf("error: unexpected response type from create gateway API, expected TaskReference")
	}
	if err := waitForNetworkingTask(ctx, d, meta, taskRef.ExtId, schema.TimeoutCreate, "gateway create"); err != nil {
		return diag.FromErr(err)
	}

	created, err := findGatewayByName(conn, name)
	if err != nil {
		return diag.FromErr(err)
	}
	if created == nil {
		return diag.Errorf("gateway %q was created but could not be found by name", name)
	}
	d.SetId(utils.StringValue(created.ExtId))
	return resourceNutanixGatewayV2Read(ctx, d, meta)
}

func resourceNutanixGatewayV2Read(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	conn := meta.(*conns.Client).NetworkingAPI
	resp, err := conn.GatewaysAPIInstance.GetGatewayById(utils.StringPtr(d.Id()))
	if err != nil {
		if isNetworkingNotFoundError(err) {
			d.SetId("")
			return nil
		}
		return diag.Errorf("error while fetching gateway %q: %v", d.Id(), err)
	}
	gateway, ok := resp.Data.GetValue().(config.Gateway)
	if !ok {
		return diag.Errorf("error: unexpected response type from get gateway API, expected Gateway")
	}

	_ = d.Set("name", utils.StringValue(gateway.Name))
	_ = d.Set("ext_id", utils.StringValue(gateway.ExtId))
	_ = d.Set("tenant_id", utils.StringValue(gateway.TenantId))
	if services, ok := gateway.GetServices().(config.RemoteNetworkServices); ok && services.RemoteBgpService != nil {
		_ = d.Set("remote_bgp_service", flattenRemoteBgpService(services.RemoteBgpService))
		_ = d.Set("local_bgp_service", []interface{}{})
	}
	if services, ok := gateway.GetServices().(config.LocalNetworkServices); ok && services.LocalBgpService != nil {
		_ = d.Set("local_bgp_service", flattenLocalBgpService(services.LocalBgpService))
		_ = d.Set("remote_bgp_service", []interface{}{})
	}
	return nil
}

func resourceNutanixGatewayV2Delete(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	conn := meta.(*conns.Client).NetworkingAPI
	resp, err := conn.GatewaysAPIInstance.DeleteGatewayById(utils.StringPtr(d.Id()))
	if err != nil {
		if isNetworkingNotFoundError(err) {
			d.SetId("")
			return nil
		}
		return diag.Errorf("error while deleting gateway %q: %v", d.Id(), err)
	}

	taskRef, ok := resp.Data.GetValue().(networkingPrism.TaskReference)
	if !ok {
		return diag.Errorf("error: unexpected response type from delete gateway API, expected TaskReference")
	}
	if err := waitForNetworkingTask(ctx, d, meta, taskRef.ExtId, schema.TimeoutDelete, "gateway delete"); err != nil {
		return diag.FromErr(err)
	}
	d.SetId("")
	return nil
}

func expandGateway(d *schema.ResourceData) *config.Gateway {
	gateway := config.NewGateway()
	gateway.Name = utils.StringPtr(d.Get("name").(string))
	if raw, ok := d.GetOk("remote_bgp_service"); ok && len(raw.([]interface{})) > 0 {
		services := config.NewRemoteNetworkServices()
		services.RemoteBgpService = expandRemoteBgpService(raw.([]interface{}))
		_ = gateway.SetServices(*services)
	}
	if raw, ok := d.GetOk("local_bgp_service"); ok && len(raw.([]interface{})) > 0 {
		services := config.NewLocalNetworkServices()
		services.LocalBgpService = expandLocalBgpService(raw.([]interface{}))
		_ = gateway.SetServices(*services)
	}
	return gateway
}

func expandRemoteBgpService(raw []interface{}) *config.RemoteBgpService {
	service := config.NewRemoteBgpService()
	values := raw[0].(map[string]interface{})
	service.Address = ipv4Address(values["address"].(string), values["prefix_length"].(int))
	service.Asn = utils.Int64Ptr(int64(values["asn"].(int)))
	return service
}

func flattenRemoteBgpService(service *config.RemoteBgpService) []interface{} {
	result := map[string]interface{}{
		"asn": utils.Int64Value(service.Asn),
	}
	if service.Address != nil && service.Address.Ipv4 != nil {
		result["address"] = utils.StringValue(service.Address.Ipv4.Value)
		result["prefix_length"] = utils.IntValue(service.Address.Ipv4.PrefixLength)
	}
	return []interface{}{result}
}

func expandLocalBgpService(raw []interface{}) *config.LocalBgpService {
	service := config.NewLocalBgpService()
	values := raw[0].(map[string]interface{})
	service.VpcReference = utils.StringPtr(values["vpc_reference"].(string))
	service.Asn = utils.Int64Ptr(int64(values["asn"].(int)))
	service.IsBgpAddPathEnabled = utils.BoolPtr(values["is_bgp_add_path_enabled"].(bool))
	return service
}

func flattenLocalBgpService(service *config.LocalBgpService) []interface{} {
	result := map[string]interface{}{
		"asn":                     utils.Int64Value(service.Asn),
		"vpc_reference":           utils.StringValue(service.VpcReference),
		"is_bgp_add_path_enabled": utils.BoolValue(service.IsBgpAddPathEnabled),
	}
	return []interface{}{result}
}

func ipv4Address(value string, prefixLength int) *networkingCommon.IPAddress {
	address := networkingCommon.NewIPAddress()
	address.Ipv4 = networkingCommon.NewIPv4Address()
	address.Ipv4.Value = utils.StringPtr(value)
	address.Ipv4.PrefixLength = utils.IntPtr(prefixLength)
	return address
}

func waitForNetworkingTask(ctx context.Context, d *schema.ResourceData, meta interface{}, taskID *string, timeoutKey string, action string) error {
	taskconn := meta.(*conns.Client).PrismAPI
	stateConf := &resource.StateChangeConf{
		Pending: []string{"PENDING", "RUNNING", "QUEUED"},
		Target:  []string{"SUCCEEDED"},
		Refresh: common.TaskStateRefreshPrismTaskGroupFunc(ctx, taskconn, utils.StringValue(taskID)),
		Timeout: d.Timeout(timeoutKey),
	}
	if _, err := stateConf.WaitForStateContext(ctx); err != nil {
		return fmt.Errorf("error waiting for %s task (%s): %w", action, utils.StringValue(taskID), err)
	}
	return nil
}

func isNetworkingNotFoundError(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "404") ||
		strings.Contains(message, "not found") ||
		strings.Contains(message, "entity_not_found") ||
		strings.Contains(message, "could not find the entity")
}
