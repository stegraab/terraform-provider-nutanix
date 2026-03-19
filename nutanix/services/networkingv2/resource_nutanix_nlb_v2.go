package networkingv2

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"
	netclient "github.com/nutanix/ntnx-api-golang-clients/networking-go-client/v4/client"
	import1 "github.com/nutanix/ntnx-api-golang-clients/networking-go-client/v4/models/networking/v4/config"
	import4 "github.com/nutanix/ntnx-api-golang-clients/networking-go-client/v4/models/prism/v4/config"
	conns "github.com/terraform-providers/terraform-provider-nutanix/nutanix"
	networkingsdk "github.com/terraform-providers/terraform-provider-nutanix/nutanix/sdks/v4/networking"
	"github.com/terraform-providers/terraform-provider-nutanix/utils"
)

func ResourceNutanixNLBV2() *schema.Resource {
	return &schema.Resource{
		CreateContext: resourceNutanixNLBV2Create,
		ReadContext:   resourceNutanixNLBV2Read,
		UpdateContext: resourceNutanixNLBV2Update,
		DeleteContext: resourceNutanixNLBV2Delete,
		Importer: &schema.ResourceImporter{
			StateContext: schema.ImportStatePassthroughContext,
		},
		Schema: map[string]*schema.Schema{
			"ext_id": {
				Type:     schema.TypeString,
				Computed: true,
			},
			"name": {
				Type:     schema.TypeString,
				Required: true,
			},
			"description": {
				Type:     schema.TypeString,
				Optional: true,
			},
			"type": {
				Type:     schema.TypeString,
				Computed: true,
			},
			"algorithm": {
				Type:     schema.TypeString,
				Computed: true,
			},
			"vpc_reference": {
				Type:     schema.TypeString,
				Required: true,
			},
			"listener": {
				Type:     schema.TypeList,
				Required: true,
				MaxItems: 1,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"protocol": {
							Type:         schema.TypeString,
							Optional:     true,
							Default:      "TCP",
							ValidateFunc: validation.StringInSlice([]string{"TCP", "UDP"}, false),
						},
						"port_ranges": {
							Type:     schema.TypeList,
							Required: true,
							MinItems: 1,
							Elem: &schema.Resource{
								Schema: map[string]*schema.Schema{
									"start_port": {
										Type:     schema.TypeInt,
										Required: true,
									},
									"end_port": {
										Type:     schema.TypeInt,
										Required: true,
									},
								},
							},
						},
						"virtual_ip": {
							Type:     schema.TypeList,
							Required: true,
							MaxItems: 1,
							Elem: &schema.Resource{
								Schema: map[string]*schema.Schema{
									"subnet_reference": {
										Type:     schema.TypeString,
										Required: true,
									},
									"assignment_type": {
										Type:         schema.TypeString,
										Optional:     true,
										Default:      "DYNAMIC",
										ValidateFunc: validation.StringInSlice([]string{"DYNAMIC", "STATIC"}, false),
									},
									"ip_address": {
										Type:     schema.TypeList,
										Optional: true,
										Computed: true,
										MaxItems: 1,
										Elem: &schema.Resource{
											Schema: map[string]*schema.Schema{
												"ipv4": SchemaForValuePrefixLength(),
												"ipv6": SchemaForValuePrefixLength(),
											},
										},
									},
								},
							},
						},
					},
				},
			},
			"health_check_config": {
				Type:     schema.TypeList,
				Optional: true,
				MaxItems: 1,
				Computed: true,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"interval_secs": {
							Type:         schema.TypeInt,
							Optional:     true,
							Computed:     true,
							ValidateFunc: validation.IntAtLeast(1),
						},
						"timeout_secs": {
							Type:         schema.TypeInt,
							Optional:     true,
							Computed:     true,
							ValidateFunc: validation.IntAtLeast(1),
						},
						"success_threshold": {
							Type:         schema.TypeInt,
							Optional:     true,
							Computed:     true,
							ValidateFunc: validation.IntAtLeast(1),
						},
						"failure_threshold": {
							Type:         schema.TypeInt,
							Optional:     true,
							Computed:     true,
							ValidateFunc: validation.IntAtLeast(1),
						},
					},
				},
			},
			"targets_config": {
				Type:     schema.TypeList,
				Required: true,
				MaxItems: 1,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"nic_targets": {
							Type:     schema.TypeList,
							Required: true,
							MinItems: 1,
							Elem: &schema.Resource{
								Schema: map[string]*schema.Schema{
									"virtual_nic_reference": {
										Type:     schema.TypeString,
										Required: true,
									},
									"vm_reference": {
										Type:     schema.TypeString,
										Computed: true,
									},
									"port": {
										Type:     schema.TypeInt,
										Optional: true,
									},
									"health": {
										Type:     schema.TypeString,
										Computed: true,
									},
								},
							},
						},
					},
				},
			},
			"tenant_id": {
				Type:     schema.TypeString,
				Computed: true,
			},
			"links": {
				Type:     schema.TypeList,
				Computed: true,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"href": {
							Type:     schema.TypeString,
							Computed: true,
						},
						"rel": {
							Type:     schema.TypeString,
							Computed: true,
						},
					},
				},
			},
			"metadata": {
				Type:     schema.TypeList,
				Computed: true,
				Elem: &schema.Resource{
					Schema: DatasourceMetadataSchemaV2(),
				},
			},
		},
	}
}

func resourceNutanixNLBV2Create(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	conn := meta.(*conns.Client).NetworkingAPI

	name := d.Get("name").(string)
	filter := fmt.Sprintf("name eq '%s'", name)
	listResp, err := conn.LoadBalancerSessionsAPIInstance.ListLoadBalancerSessions(nil, nil, &filter, nil, nil)
	if err != nil {
		return diag.Errorf("error while listing load balancer sessions : %v", err)
	}

	items, err := extractNLBSessionList(listResp)
	if err != nil {
		return diag.Errorf("error while parsing load balancer sessions list : %v", err)
	}
	if len(items) > 0 && items[0].ExtId != nil {
		d.SetId(*items[0].ExtId)
		return resourceNutanixNLBV2Read(ctx, d, meta)
	}

	reqBody := expandNLBSessionFromResourceData(d)
	resp, err := conn.LoadBalancerSessionsAPIInstance.CreateLoadBalancerSession(reqBody)
	if err != nil {
		return diag.Errorf("error while creating load balancer session : %v", err)
	}

	taskRef, err := extractNLBTaskReference(resp)
	if err != nil {
		return diag.Errorf("error while parsing create task response : %v", err)
	}
	if diags := waitForNLBTask(ctx, d, meta, taskRef.ExtId, "create"); diags != nil {
		return diags
	}

	readResp, err := conn.LoadBalancerSessionsAPIInstance.ListLoadBalancerSessions(nil, nil, &filter, nil, nil)
	if err != nil {
		return diag.Errorf("error while fetching load balancer session after create : %v", err)
	}

	created, err := extractNLBSessionList(readResp)
	if err != nil {
		return diag.Errorf("error while parsing load balancer session after create : %v", err)
	}
	if len(created) == 0 || created[0].ExtId == nil {
		return diag.Errorf("unable to find load balancer session %q after create", name)
	}

	d.SetId(*created[0].ExtId)
	return resourceNutanixNLBV2Read(ctx, d, meta)
}

func resourceNutanixNLBV2Read(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	conn := meta.(*conns.Client).NetworkingAPI

	resp, err := conn.LoadBalancerSessionsAPIInstance.GetLoadBalancerSessionById(utils.StringPtr(d.Id()), nil)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "not found") {
			d.SetId("")
			return nil
		}
		return diag.Errorf("error while reading load balancer session : %v", err)
	}

	session, err := extractNLBSession(resp)
	if err != nil {
		return diag.Errorf("error while parsing load balancer session : %v", err)
	}
	flattenNLBToResourceData(d, &session)
	return nil
}

func resourceNutanixNLBV2Update(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	conn := meta.(*conns.Client).NetworkingAPI

	eTag, err := fetchNLBETag(conn, d.Id())
	if err != nil {
		return diag.Errorf("error while fetching load balancer session eTag for update : %v", err)
	}

	reqBody := expandNLBSessionFromResourceData(d)
	headerArgs := map[string]interface{}{
		"If-Match": utils.StringPtr(eTag),
	}
	resp, err := conn.LoadBalancerSessionsAPIInstance.UpdateLoadBalancerSessionById(utils.StringPtr(d.Id()), reqBody, headerArgs)
	if err != nil {
		return diag.Errorf("error while updating load balancer session : %v", err)
	}

	taskRef, err := extractNLBTaskReference(resp)
	if err != nil {
		return diag.Errorf("error while parsing update task response : %v", err)
	}
	if diags := waitForNLBTask(ctx, d, meta, taskRef.ExtId, "update"); diags != nil {
		return diags
	}

	return resourceNutanixNLBV2Read(ctx, d, meta)
}

func resourceNutanixNLBV2Delete(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	conn := meta.(*conns.Client).NetworkingAPI

	eTag, err := fetchNLBETag(conn, d.Id())
	if err != nil {
		return diag.Errorf("error while fetching load balancer session eTag for delete : %v", err)
	}

	headerArgs := map[string]interface{}{
		"If-Match": utils.StringPtr(eTag),
	}
	resp, err := conn.LoadBalancerSessionsAPIInstance.DeleteLoadBalancerSessionById(utils.StringPtr(d.Id()), headerArgs)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "not found") {
			d.SetId("")
			return nil
		}
		return diag.Errorf("error while deleting load balancer session : %v", err)
	}

	taskRef, err := extractNLBTaskReference(resp)
	if err != nil {
		return diag.Errorf("error while parsing delete task response : %v", err)
	}
	if diags := waitForNLBTask(ctx, d, meta, taskRef.ExtId, "delete"); diags != nil {
		return diags
	}

	d.SetId("")
	return nil
}

func waitForNLBTask(ctx context.Context, d *schema.ResourceData, meta interface{}, taskUUID *string, action string) diag.Diagnostics {
	taskconn := meta.(*conns.Client).PrismAPI
	stateConf := &resource.StateChangeConf{
		Pending: []string{"QUEUED", "RUNNING"},
		Target:  []string{"SUCCEEDED"},
		Refresh: taskStateRefreshPrismTaskGroupFunc(ctx, taskconn, utils.StringValue(taskUUID)),
		Timeout: d.Timeout(schema.TimeoutCreate),
	}

	if _, errWaitTask := stateConf.WaitForStateContext(ctx); errWaitTask != nil {
		return diag.Errorf("error waiting for load balancer session (%s) to %s: %s", utils.StringValue(taskUUID), action, errWaitTask)
	}

	return nil
}

func expandNLBSessionFromResourceData(d *schema.ResourceData) *import1.LoadBalancerSession {
	reqBody := &import1.LoadBalancerSession{}

	reqBody.Name = utils.StringPtr(d.Get("name").(string))
	if desc, ok := d.GetOk("description"); ok {
		reqBody.Description = utils.StringPtr(desc.(string))
	}
	reqBody.VpcReference = utils.StringPtr(d.Get("vpc_reference").(string))

	algorithm := import1.ALGORITHM_FIVE_TUPLE_HASH
	reqBody.Algorithm = &algorithm

	lbType := import1.LOADBALANCERSESSIONTYPE_NETWORK_LOAD_BALANCER
	reqBody.Type = &lbType

	listener := d.Get("listener").([]interface{})
	reqBody.Listener = expandNLBListener(listener)

	reqBody.HealthCheckConfig = expandNLBHealthCheck(d.Get("health_check_config").([]interface{}))

	targets := d.Get("targets_config").([]interface{})
	reqBody.TargetsConfig = expandNLBTargets(targets)

	return reqBody
}

func expandNLBListener(pr []interface{}) *import1.Listener {
	if len(pr) == 0 {
		return nil
	}

	val := pr[0].(map[string]interface{})
	listener := &import1.Listener{}

	if protocol, ok := val["protocol"].(string); ok {
		switch protocol {
		case "UDP":
			p := import1.PROTOCOL_UDP
			listener.Protocol = &p
		default:
			p := import1.PROTOCOL_TCP
			listener.Protocol = &p
		}
	}

	if portRanges, ok := val["port_ranges"].([]interface{}); ok {
		ranges := make([]import1.PortRange, 0, len(portRanges))
		for _, prI := range portRanges {
			if prI == nil {
				continue
			}
			prMap := prI.(map[string]interface{})
			start := prMap["start_port"].(int)
			end := prMap["end_port"].(int)
			ranges = append(ranges, import1.PortRange{
				StartPort: utils.IntPtr(start),
				EndPort:   utils.IntPtr(end),
			})
		}
		listener.PortRanges = ranges
	}

	if vip, ok := val["virtual_ip"].([]interface{}); ok && len(vip) > 0 {
		vipMap := vip[0].(map[string]interface{})
		virtualIP := &import1.VirtualIP{
			SubnetReference: utils.StringPtr(vipMap["subnet_reference"].(string)),
		}

		if atype, ok := vipMap["assignment_type"].(string); ok {
			switch atype {
			case "STATIC":
				a := import1.ASSIGNMENTTYPE_STATIC
				virtualIP.AssignmentType = &a
			default:
				a := import1.ASSIGNMENTTYPE_DYNAMIC
				virtualIP.AssignmentType = &a
			}
		}

		if ipAddress, ok := vipMap["ip_address"].([]interface{}); ok && len(ipAddress) > 0 {
			vipIPAddress := expandIPAddressMap(ipAddress)
			// NLB listener static VIP expects a plain address value (no CIDR suffix).
			// Some Terraform paths populate prefix_length as 0 when omitted, which
			// results in malformed values like "10.1.2.3/0" in API requests.
			if vipIPAddress != nil {
				if vipIPAddress.Ipv4 != nil {
					vipIPAddress.Ipv4.PrefixLength = nil
				}
				if vipIPAddress.Ipv6 != nil {
					vipIPAddress.Ipv6.PrefixLength = nil
				}
			}
			virtualIP.IpAddress = vipIPAddress
		}

		listener.VirtualIP = virtualIP
	}

	return listener
}

func expandNLBHealthCheck(pr []interface{}) *import1.HealthCheck {
	hc := &import1.HealthCheck{
		IntervalSecs:     utils.IntPtr(10),
		TimeoutSecs:      utils.IntPtr(5),
		SuccessThreshold: utils.IntPtr(3),
		FailureThreshold: utils.IntPtr(3),
	}

	if len(pr) == 0 || pr[0] == nil {
		return hc
	}

	val := pr[0].(map[string]interface{})
	if interval, ok := val["interval_secs"].(int); ok && interval > 0 {
		hc.IntervalSecs = utils.IntPtr(interval)
	}
	if timeout, ok := val["timeout_secs"].(int); ok && timeout > 0 {
		hc.TimeoutSecs = utils.IntPtr(timeout)
	}
	if success, ok := val["success_threshold"].(int); ok && success > 0 {
		hc.SuccessThreshold = utils.IntPtr(success)
	}
	if failure, ok := val["failure_threshold"].(int); ok && failure > 0 {
		hc.FailureThreshold = utils.IntPtr(failure)
	}

	return hc
}

func expandNLBTargets(pr []interface{}) *import1.Target {
	if len(pr) == 0 || pr[0] == nil {
		return nil
	}

	val := pr[0].(map[string]interface{})
	target := &import1.Target{}

	if nicTargets, ok := val["nic_targets"].([]interface{}); ok {
		targets := make([]import1.NicTarget, 0, len(nicTargets))
		for _, ni := range nicTargets {
			if ni == nil {
				continue
			}
			targetMap := ni.(map[string]interface{})
			t := import1.NicTarget{
				VirtualNicReference: utils.StringPtr(targetMap["virtual_nic_reference"].(string)),
			}
			if portRaw, ok := targetMap["port"]; ok {
				port := portRaw.(int)
				if port > 0 {
					t.Port = utils.IntPtr(port)
				}
			}
			targets = append(targets, t)
		}
		target.NicTargets = targets
	}

	return target
}

func flattenNLBToResourceData(d *schema.ResourceData, session *import1.LoadBalancerSession) {
	if session.ExtId != nil {
		d.SetId(*session.ExtId)
		_ = d.Set("ext_id", session.ExtId)
	}
	_ = d.Set("name", session.Name)
	_ = d.Set("description", session.Description)
	_ = d.Set("vpc_reference", session.VpcReference)
	_ = d.Set("listener", flattenNLBListener(session.Listener))
	_ = d.Set("health_check_config", flattenNLBHealthCheck(session.HealthCheckConfig))
	_ = d.Set("targets_config", flattenNLBTargets(session.TargetsConfig))
	_ = d.Set("tenant_id", session.TenantId)
	_ = d.Set("links", flattenLinks(session.Links))
	_ = d.Set("metadata", flattenMetadata(session.Metadata))
	_ = d.Set("type", flattenNLBType(session.Type))
	_ = d.Set("algorithm", flattenNLBAlgorithm(session.Algorithm))
}

func flattenNLBListener(pr *import1.Listener) []map[string]interface{} {
	if pr == nil {
		return nil
	}

	listener := map[string]interface{}{
		"protocol":    flattenNLBProtocol(pr.Protocol),
		"port_ranges": flattenNLBPortRanges(pr.PortRanges),
		"virtual_ip":  flattenNLBVirtualIP(pr.VirtualIP),
	}

	return []map[string]interface{}{listener}
}

func flattenNLBPortRanges(pr []import1.PortRange) []map[string]interface{} {
	if len(pr) == 0 {
		return nil
	}

	ranges := make([]map[string]interface{}, 0, len(pr))
	for _, r := range pr {
		portRange := map[string]interface{}{
			"start_port": utils.IntValue(r.StartPort),
			"end_port":   utils.IntValue(r.EndPort),
		}
		ranges = append(ranges, portRange)
	}
	return ranges
}

func flattenNLBVirtualIP(pr *import1.VirtualIP) []map[string]interface{} {
	if pr == nil {
		return nil
	}

	vip := map[string]interface{}{
		"subnet_reference": utils.StringValue(pr.SubnetReference),
		"assignment_type":  flattenNLBAssignmentType(pr.AssignmentType),
		"ip_address":       flattenIPAddress(pr.IpAddress),
	}

	return []map[string]interface{}{vip}
}

func flattenNLBHealthCheck(pr *import1.HealthCheck) []map[string]interface{} {
	if pr == nil {
		return nil
	}

	hc := map[string]interface{}{
		"interval_secs":     utils.IntValue(pr.IntervalSecs),
		"timeout_secs":      utils.IntValue(pr.TimeoutSecs),
		"success_threshold": utils.IntValue(pr.SuccessThreshold),
		"failure_threshold": utils.IntValue(pr.FailureThreshold),
	}
	return []map[string]interface{}{hc}
}

func flattenNLBTargets(pr *import1.Target) []map[string]interface{} {
	if pr == nil {
		return nil
	}

	targets := map[string]interface{}{
		"nic_targets": flattenNLBNicTargets(pr.NicTargets),
	}
	return []map[string]interface{}{targets}
}

func flattenNLBNicTargets(pr []import1.NicTarget) []map[string]interface{} {
	if len(pr) == 0 {
		return nil
	}

	targets := make([]map[string]interface{}, 0, len(pr))
	for _, t := range pr {
		target := map[string]interface{}{
			"virtual_nic_reference": utils.StringValue(t.VirtualNicReference),
			"vm_reference":          utils.StringValue(t.VmReference),
			"port":                  utils.IntValue(t.Port),
			"health":                flattenNLBTargetHealth(t.Health),
		}
		targets = append(targets, target)
	}
	return targets
}

func flattenNLBProtocol(pr *import1.Protocol) string {
	const three = 3
	if pr != nil && *pr == import1.Protocol(three) {
		return "UDP"
	}
	return "TCP"
}

func flattenNLBAssignmentType(pr *import1.AssignmentType) string {
	const three = 3
	if pr != nil && *pr == import1.AssignmentType(three) {
		return "STATIC"
	}
	return "DYNAMIC"
}

func flattenNLBType(pr *import1.LoadBalancerSessionType) string {
	const two = 2
	if pr != nil && *pr == import1.LoadBalancerSessionType(two) {
		return "NETWORK_LOAD_BALANCER"
	}
	return "UNKNOWN"
}

func flattenNLBAlgorithm(pr *import1.Algorithm) string {
	const two = 2
	if pr != nil && *pr == import1.Algorithm(two) {
		return "FIVE_TUPLE_HASH"
	}
	return "UNKNOWN"
}

func flattenNLBTargetHealth(pr *import1.TargetHealth) string {
	const two, three = 2, 3
	if pr != nil {
		if *pr == import1.TargetHealth(two) {
			return "HEALTHY"
		}
		if *pr == import1.TargetHealth(three) {
			return "UNHEALTHY"
		}
	}
	return "UNKNOWN"
}

func fetchNLBETag(conn *networkingsdk.Client, extID string) (string, error) {
	if conn == nil || conn.APIClientInstance == nil {
		return "", fmt.Errorf("networking api client is not initialized")
	}

	apiClient := conn.APIClientInstance
	url := buildNLBURL(apiClient, extID)

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.SetBasicAuth(apiClient.Username, apiClient.Password)
	req.Header.Set("Accept", "application/json")

	httpClient := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: !apiClient.VerifySSL,
			},
		},
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected status %d while fetching eTag: %s", resp.StatusCode, string(body))
	}

	eTag := resp.Header.Get("Etag")
	if eTag == "" {
		eTag = resp.Header.Get("ETag")
	}
	if eTag == "" {
		return "", fmt.Errorf("missing eTag header in response")
	}

	return eTag, nil
}

func buildNLBURL(apiClient *netclient.ApiClient, extID string) string {
	scheme := apiClient.Scheme
	if scheme == "" {
		scheme = "https"
	}

	port := apiClient.Port
	if port == 0 {
		port = 9440
	}

	return fmt.Sprintf("%s://%s:%d/api/networking/v4.1/config/load-balancer-sessions/%s", scheme, apiClient.Host, port, extID)
}

func extractNLBSessionList(resp *import1.ListLoadBalancerSessionsApiResponse) ([]import1.LoadBalancerSession, error) {
	if resp == nil || resp.Data == nil {
		return []import1.LoadBalancerSession{}, nil
	}

	value := resp.Data.GetValue()
	if value == nil {
		return []import1.LoadBalancerSession{}, nil
	}

	items, ok := value.([]import1.LoadBalancerSession)
	if !ok {
		return nil, fmt.Errorf("unexpected list response type: %T", value)
	}

	return items, nil
}

func extractNLBSession(resp *import1.GetLoadBalancerSessionApiResponse) (import1.LoadBalancerSession, error) {
	if resp == nil || resp.Data == nil {
		return import1.LoadBalancerSession{}, fmt.Errorf("empty response data")
	}

	value := resp.Data.GetValue()
	if value == nil {
		return import1.LoadBalancerSession{}, fmt.Errorf("empty response data value")
	}

	session, ok := value.(import1.LoadBalancerSession)
	if !ok {
		return import1.LoadBalancerSession{}, fmt.Errorf("unexpected get response type: %T", value)
	}

	return session, nil
}

func extractNLBTaskReference(resp *import1.TaskReferenceApiResponse) (import4.TaskReference, error) {
	if resp == nil || resp.Data == nil {
		return import4.TaskReference{}, fmt.Errorf("empty task response data")
	}

	value := resp.Data.GetValue()
	if value == nil {
		return import4.TaskReference{}, fmt.Errorf("empty task response data value")
	}

	taskRef, ok := value.(import4.TaskReference)
	if !ok {
		return import4.TaskReference{}, fmt.Errorf("unexpected task response type: %T", value)
	}

	return taskRef, nil
}
