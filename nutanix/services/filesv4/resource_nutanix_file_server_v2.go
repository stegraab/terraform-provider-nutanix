package filesv4

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"
	conns "github.com/terraform-providers/terraform-provider-nutanix/nutanix"
	filesClient "github.com/terraform-providers/terraform-provider-nutanix/nutanix/sdks/v4/files"
)

const filesAPIBasePath = "/api/files/v4.0.a6/config/file-servers"
const filesDirectoryServicesPathTemplate = "/api/files/v4.0/config/file-servers/%s/directory-services"
const filesConfigureNameServicesPathTemplate = "/api/files/v4.0.a6/config/file-servers/%s/$actions/configure-name-services"

func ResourceNutanixFileServerV2() *schema.Resource {
	return &schema.Resource{
		CreateContext: resourceNutanixFileServerV2Create,
		ReadContext:   resourceNutanixFileServerV2Read,
		UpdateContext: resourceNutanixFileServerV2Update,
		DeleteContext: resourceNutanixFileServerV2Delete,
		Importer: &schema.ResourceImporter{
			StateContext: schema.ImportStatePassthroughContext,
		},
		Timeouts: &schema.ResourceTimeout{
			Create: schema.DefaultTimeout(2 * time.Hour),
			Delete: schema.DefaultTimeout(2 * time.Hour),
		},
		Schema: map[string]*schema.Schema{
			"ext_id": {
				Type:     schema.TypeString,
				Computed: true,
			},
			"name": {
				Type:         schema.TypeString,
				Required:     true,
				ForceNew:     true,
				ValidateFunc: validation.StringLenBetween(1, 15),
			},
			"size_in_gib": {
				Type:         schema.TypeInt,
				Required:     true,
				ForceNew:     true,
				ValidateFunc: validation.IntAtLeast(1),
			},
			"nvms_count": {
				Type:         schema.TypeInt,
				Required:     true,
				ForceNew:     true,
				ValidateFunc: validation.IntAtLeast(1),
			},
			"dns_domain_name": {
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
			},
			"dns_servers": {
				Type:     schema.TypeList,
				Required: true,
				MinItems: 1,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"value": {
							Type:     schema.TypeString,
							Required: true,
						},
					},
				},
			},
			"ntp_servers": {
				Type:     schema.TypeList,
				Optional: true,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"fqdn": {
							Type:     schema.TypeString,
							Required: true,
						},
					},
				},
			},
			"memory_gib": {
				Type:         schema.TypeInt,
				Required:     true,
				ForceNew:     true,
				ValidateFunc: validation.IntAtLeast(1),
			},
			"vcpus": {
				Type:         schema.TypeInt,
				Required:     true,
				ForceNew:     true,
				ValidateFunc: validation.IntAtLeast(1),
			},
			"version": {
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
			},
			"cvm_ip_addresses": {
				Type:     schema.TypeList,
				Required: true,
				MinItems: 1,
				ForceNew: true,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"value": {
							Type:     schema.TypeString,
							Required: true,
						},
					},
				},
			},
			"cluster_ext_id": {
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
			},
			"deployment_profile_types": {
				Type:     schema.TypeList,
				Optional: true,
				ForceNew: true,
				Elem: &schema.Schema{
					Type:         schema.TypeString,
					ValidateFunc: validation.StringInSlice([]string{"DEFAULT", "GENERAL_DEDICATED", "AIML_DEDICATED"}, false),
				},
			},
			"external_networks": {
				Type:     schema.TypeList,
				Required: true,
				MinItems: 1,
				ForceNew: true,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"is_managed": {
							Type:     schema.TypeBool,
							Optional: true,
							Default:  true,
							ForceNew: true,
						},
						"network_ext_id": {
							Type:     schema.TypeString,
							Required: true,
							ForceNew: true,
						},
						"vlan_id": {
							Type:         schema.TypeInt,
							Optional:     true,
							ForceNew:     true,
							ValidateFunc: validation.IntBetween(0, 4095),
						},
						"default_gateway": {
							Type:     schema.TypeString,
							Optional: true,
							ForceNew: true,
						},
						"subnet_mask": {
							Type:     schema.TypeString,
							Optional: true,
							ForceNew: true,
						},
						"ip_addresses": {
							Type:     schema.TypeList,
							Optional: true,
							ForceNew: true,
							Elem: &schema.Schema{
								Type: schema.TypeString,
							},
						},
					},
				},
			},
			"internal_networks": {
				Type:     schema.TypeList,
				Required: true,
				MinItems: 1,
				ForceNew: true,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"is_managed": {
							Type:     schema.TypeBool,
							Optional: true,
							Default:  true,
							ForceNew: true,
						},
						"network_ext_id": {
							Type:     schema.TypeString,
							Required: true,
							ForceNew: true,
						},
						"vlan_id": {
							Type:         schema.TypeInt,
							Optional:     true,
							ForceNew:     true,
							ValidateFunc: validation.IntBetween(0, 4095),
						},
						"default_gateway": {
							Type:     schema.TypeString,
							Optional: true,
							ForceNew: true,
						},
						"subnet_mask": {
							Type:     schema.TypeString,
							Optional: true,
							ForceNew: true,
						},
						"ip_addresses": {
							Type:     schema.TypeList,
							Optional: true,
							ForceNew: true,
							Elem: &schema.Schema{
								Type: schema.TypeString,
							},
						},
					},
				},
			},
			"deployment_status": {
				Type:     schema.TypeString,
				Computed: true,
			},
			"external_ip_addresses": {
				Type:     schema.TypeList,
				Computed: true,
				Elem: &schema.Schema{
					Type: schema.TypeString,
				},
			},
			"vms": {
				Type:     schema.TypeList,
				Computed: true,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"ext_id": {
							Type:     schema.TypeString,
							Computed: true,
						},
						"name": {
							Type:     schema.TypeString,
							Computed: true,
						},
						"fsvm_uuid": {
							Type:     schema.TypeString,
							Computed: true,
						},
						"external_ip_addresses": {
							Type:     schema.TypeList,
							Computed: true,
							Elem: &schema.Schema{
								Type: schema.TypeString,
							},
						},
						"internal_ip_addresses": {
							Type:     schema.TypeList,
							Computed: true,
							Elem: &schema.Schema{
								Type: schema.TypeString,
							},
						},
					},
				},
			},
			"directory_service": {
				Type:     schema.TypeList,
				Optional: true,
				MaxItems: 1,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"local_domain": {
							Type:         schema.TypeString,
							Optional:     true,
							Default:      "NONE",
							ValidateFunc: validation.StringInSlice([]string{"NONE", "NFS", "SMB", "SMB_NFS"}, false),
						},
						"nfs_version": {
							Type:         schema.TypeString,
							Optional:     true,
							Default:      "NFSV3V4",
							ValidateFunc: validation.StringInSlice([]string{"NFSV3", "NFSV4", "NFSV3V4"}, false),
						},
						"nfs_v4_domain": {
							Type:     schema.TypeString,
							Optional: true,
						},
						"ldap_domain": {
							Type:     schema.TypeList,
							Optional: true,
							MaxItems: 1,
							Elem: &schema.Resource{
								Schema: map[string]*schema.Schema{
									"protocol_type": {
										Type:         schema.TypeString,
										Optional:     true,
										Default:      "NFS",
										ValidateFunc: validation.StringInSlice([]string{"NFS", "SMB", "SMB_NFS"}, false),
									},
									"servers": {
										Type:     schema.TypeList,
										Required: true,
										MinItems: 1,
										Elem: &schema.Schema{
											Type: schema.TypeString,
										},
									},
									"base_dn": {
										Type:     schema.TypeString,
										Required: true,
									},
									"bind_dn": {
										Type:     schema.TypeString,
										Optional: true,
									},
									"bind_password": {
										Type:      schema.TypeString,
										Optional:  true,
										Sensitive: true,
									},
								},
							},
						},
					},
				},
			},
		},
	}
}

func resourceNutanixFileServerV2Create(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	apiClient := meta.(*conns.Client).FilesAPI.APIClientInstance
	if apiClient == nil {
		return diag.Errorf("files api client is not initialized")
	}

	payload := expandFileServerPayload(d)
	respBody, statusCode, err := filesRequest(apiClient, http.MethodPost, filesAPIBasePath, payload)
	if err != nil {
		return diag.Errorf("error while creating file server: %v", err)
	}
	if statusCode >= http.StatusBadRequest || filesHasError(respBody) {
		return diag.Errorf("error while creating file server: %s", filesErrorMessage(respBody, statusCode))
	}

	name := d.Get("name").(string)
	stateConf := &resource.StateChangeConf{
		Pending: []string{"NOT_FOUND"},
		Target:  []string{"FOUND"},
		Refresh: fileServerByNameRefreshFunc(apiClient, name),
		Timeout: d.Timeout(schema.TimeoutCreate),
	}
	result, err := stateConf.WaitForStateContext(ctx)
	if err != nil {
		return diag.Errorf("error waiting for file server %q to be available: %v", name, err)
	}

	created, ok := result.(map[string]interface{})
	if !ok {
		return diag.Errorf("unexpected file server state type: %T", result)
	}
	extID := stringValue(created["extId"])
	if extID == "" {
		return diag.Errorf("unable to determine file server extId from create/list response")
	}

	d.SetId(extID)

	if v, ok := d.GetOk("directory_service"); ok {
		if err := updateDirectoryService(apiClient, extID, v.([]interface{})); err != nil {
			return diag.Errorf("error while configuring file server directory service for %q: %v", extID, err)
		}
	}

	return resourceNutanixFileServerV2Read(ctx, d, meta)
}

func resourceNutanixFileServerV2Read(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	apiClient := meta.(*conns.Client).FilesAPI.APIClientInstance
	if apiClient == nil {
		return diag.Errorf("files api client is not initialized")
	}

	item, notFound, err := getFileServerByID(apiClient, d.Id())
	if err != nil {
		return diag.Errorf("error while reading file server %q: %v", d.Id(), err)
	}
	if notFound {
		d.SetId("")
		return nil
	}

	flattenFileServerToState(d, item)
	if err := refreshDirectoryServiceState(d, apiClient); err != nil {
		return diag.Errorf("error while reading file server directory services for %q: %v", d.Id(), err)
	}
	return nil
}

func resourceNutanixFileServerV2Update(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	apiClient := meta.(*conns.Client).FilesAPI.APIClientInstance
	if apiClient == nil {
		return diag.Errorf("files api client is not initialized")
	}

	if d.HasChanges("dns_servers", "ntp_servers") {
		if err := updateFileServer(ctx, apiClient, d); err != nil {
			return diag.Errorf("error while updating file server %q: %v", d.Id(), err)
		}
	}

	if d.HasChange("directory_service") {
		if err := updateDirectoryService(apiClient, d.Id(), d.Get("directory_service").([]interface{})); err != nil {
			return diag.Errorf("error while updating file server directory service for %q: %v", d.Id(), err)
		}
	}

	return resourceNutanixFileServerV2Read(ctx, d, meta)
}

func resourceNutanixFileServerV2Delete(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	apiClient := meta.(*conns.Client).FilesAPI.APIClientInstance
	if apiClient == nil {
		return diag.Errorf("files api client is not initialized")
	}

	_, _, headers, err := filesRequestWithHeaders(apiClient, http.MethodGet, filesAPIBasePath+"/"+d.Id(), nil, nil)
	if err != nil {
		return diag.Errorf("error while reading file server %q before delete: %v", d.Id(), err)
	}

	deleteHeaders := map[string]string{}
	if etag := headers.Get("Etag"); etag != "" {
		deleteHeaders["If-Match"] = etag
	}

	respBody, statusCode, _, err := filesRequestWithHeaders(apiClient, http.MethodDelete, filesAPIBasePath+"/"+d.Id(), nil, deleteHeaders)
	if err != nil {
		return diag.Errorf("error while deleting file server %q: %v", d.Id(), err)
	}
	if statusCode >= http.StatusBadRequest || filesHasError(respBody) {
		if filesIsNotFound(respBody) {
			d.SetId("")
			return nil
		}
		return diag.Errorf("error while deleting file server %q: %s", d.Id(), filesErrorMessage(respBody, statusCode))
	}

	stateConf := &resource.StateChangeConf{
		Pending: []string{"FOUND"},
		Target:  []string{"NOT_FOUND"},
		Refresh: fileServerByIDRefreshFunc(apiClient, d.Id()),
		Timeout: d.Timeout(schema.TimeoutDelete),
	}
	if _, err := stateConf.WaitForStateContext(ctx); err != nil {
		return diag.Errorf("error waiting for file server %q to be deleted: %v", d.Id(), err)
	}

	d.SetId("")
	return nil
}

func expandFileServerPayload(d *schema.ResourceData) map[string]interface{} {
	payload := map[string]interface{}{
		"name":             d.Get("name").(string),
		"sizeInGib":        d.Get("size_in_gib").(int),
		"nvmsCount":        d.Get("nvms_count").(int),
		"dnsDomainName":    d.Get("dns_domain_name").(string),
		"dnsServers":       expandValueList(d.Get("dns_servers").([]interface{})),
		"ntpServers":       expandNTPServers(d.Get("ntp_servers").([]interface{})),
		"memoryGib":        d.Get("memory_gib").(int),
		"vcpus":            d.Get("vcpus").(int),
		"version":          d.Get("version").(string),
		"cvmIpAddresses":   expandValueList(d.Get("cvm_ip_addresses").([]interface{})),
		"clusterExtId":     d.Get("cluster_ext_id").(string),
		"externalNetworks": expandNetworkList(d.Get("external_networks").([]interface{})),
		"internalNetworks": expandNetworkList(d.Get("internal_networks").([]interface{})),
	}
	if deploymentProfiles, ok := d.GetOk("deployment_profile_types"); ok {
		payload["deploymentProfileTypes"] = stringList(deploymentProfiles.([]interface{}))
	}
	return payload
}

func expandValueList(values []interface{}) []map[string]interface{} {
	result := make([]map[string]interface{}, 0, len(values))
	for _, entry := range values {
		if entry == nil {
			continue
		}
		v := entry.(map[string]interface{})
		result = append(result, map[string]interface{}{
			"value": v["value"].(string),
		})
	}
	return result
}

func expandNTPServers(values []interface{}) []map[string]interface{} {
	result := make([]map[string]interface{}, 0, len(values))
	for _, entry := range values {
		if entry == nil {
			continue
		}
		v := entry.(map[string]interface{})
		result = append(result, map[string]interface{}{
			"fqdn": map[string]interface{}{
				"value": v["fqdn"].(string),
			},
		})
	}
	return result
}

func stringList(values []interface{}) []string {
	result := make([]string, 0, len(values))
	for _, entry := range values {
		if entry == nil {
			continue
		}
		result = append(result, entry.(string))
	}
	return result
}

func expandNetworkList(values []interface{}) []map[string]interface{} {
	result := make([]map[string]interface{}, 0, len(values))
	for _, entry := range values {
		if entry == nil {
			continue
		}
		v := entry.(map[string]interface{})
		network := map[string]interface{}{
			"isManaged":    v["is_managed"].(bool),
			"networkExtId": v["network_ext_id"].(string),
		}
		if rawVLANID, ok := v["vlan_id"]; ok && rawVLANID != nil {
			if vlanID := intValue(rawVLANID); vlanID > 0 {
				network["vlanId"] = vlanID
			}
		}
		if gateway := strings.TrimSpace(stringValue(v["default_gateway"])); gateway != "" {
			network["defaultGateway"] = map[string]interface{}{
				"ipv4": map[string]interface{}{"value": gateway},
			}
		}
		if subnetMask := strings.TrimSpace(stringValue(v["subnet_mask"])); subnetMask != "" {
			network["subnetMask"] = map[string]interface{}{
				"ipv4": map[string]interface{}{"value": subnetMask},
			}
		}
		if addresses := expandIPv4AddressList(v["ip_addresses"].([]interface{})); len(addresses) > 0 {
			network["ipAddresses"] = addresses
		}
		result = append(result, network)
	}
	return result
}

func expandIPv4AddressList(values []interface{}) []map[string]interface{} {
	result := make([]map[string]interface{}, 0, len(values))
	for _, entry := range values {
		value := strings.TrimSpace(stringValue(entry))
		if value == "" {
			continue
		}
		result = append(result, map[string]interface{}{
			"ipv4": map[string]interface{}{"value": value},
		})
	}
	return result
}

func normalizeValueList(values []map[string]interface{}) []map[string]interface{} {
	result := make([]map[string]interface{}, 0, len(values))
	for _, entry := range values {
		value := strings.TrimSpace(stringValue(entry["value"]))
		if value == "" {
			continue
		}
		result = append(result, map[string]interface{}{"value": value})
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i]["value"].(string) < result[j]["value"].(string)
	})
	return result
}

func normalizeNTPServers(values []map[string]interface{}) []map[string]interface{} {
	result := make([]map[string]interface{}, 0, len(values))
	for _, entry := range values {
		value := strings.TrimSpace(stringValue(entry["fqdn"]))
		if value == "" {
			if fqdnMap, ok := entry["fqdn"].(map[string]interface{}); ok {
				value = strings.TrimSpace(stringValue(fqdnMap["value"]))
			}
		}
		if value == "" {
			continue
		}
		result = append(result, map[string]interface{}{"fqdn": value})
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i]["fqdn"].(string) < result[j]["fqdn"].(string)
	})
	return result
}

func listValuesEqual(current, desired []map[string]interface{}) bool {
	if len(current) != len(desired) {
		return false
	}

	for i := range current {
		if len(current[i]) != len(desired[i]) {
			return false
		}
		for key, currentValue := range current[i] {
			if desired[i][key] != currentValue {
				return false
			}
		}
	}

	return true
}

func flattenFileServerToState(d *schema.ResourceData, item map[string]interface{}) {
	if extID := stringValue(item["extId"]); extID != "" {
		d.SetId(extID)
		_ = d.Set("ext_id", extID)
	}
	_ = d.Set("name", stringValue(item["name"]))
	_ = d.Set("size_in_gib", intValue(item["sizeInGib"]))
	_ = d.Set("nvms_count", intValue(item["nvmsCount"]))
	_ = d.Set("dns_domain_name", stringValue(item["dnsDomainName"]))
	_ = d.Set("dns_servers", flattenValueList(item["dnsServers"]))
	_ = d.Set("ntp_servers", flattenNTPServers(item["ntpServers"]))
	_ = d.Set("memory_gib", intValue(item["memoryGib"]))
	_ = d.Set("vcpus", intValue(item["vcpus"]))
	_ = d.Set("version", stringValue(item["version"]))
	_ = d.Set("cvm_ip_addresses", flattenValueList(item["cvmIpAddresses"]))
	_ = d.Set("cluster_ext_id", stringValue(item["clusterExtId"]))
	_ = d.Set("external_networks", flattenNetworks(item["externalNetworks"]))
	_ = d.Set("internal_networks", flattenNetworks(item["internalNetworks"]))
	_ = d.Set("deployment_status", stringValue(item["deploymentStatus"]))
	_ = d.Set("external_ip_addresses", flattenNetworkIPAddresses(item["externalNetworks"]))
	_ = d.Set("vms", flattenFileServerVMs(item["vms"]))
}

func flattenValueList(raw interface{}) []map[string]interface{} {
	list, ok := raw.([]interface{})
	if !ok {
		return nil
	}
	result := make([]map[string]interface{}, 0, len(list))
	for _, entry := range list {
		v, ok := entry.(map[string]interface{})
		if !ok {
			continue
		}
		result = append(result, map[string]interface{}{
			"value": stringValue(v["value"]),
		})
	}
	return result
}

func flattenNTPServers(raw interface{}) []map[string]interface{} {
	list, ok := raw.([]interface{})
	if !ok {
		return nil
	}
	result := make([]map[string]interface{}, 0, len(list))
	for _, entry := range list {
		v, ok := entry.(map[string]interface{})
		if !ok {
			continue
		}
		fqdn := ""
		if fqdnMap, ok := v["fqdn"].(map[string]interface{}); ok {
			fqdn = stringValue(fqdnMap["value"])
		}
		if fqdn != "" {
			result = append(result, map[string]interface{}{"fqdn": fqdn})
		}
	}
	return result
}

func flattenNetworks(raw interface{}) []map[string]interface{} {
	list, ok := raw.([]interface{})
	if !ok {
		return nil
	}
	result := make([]map[string]interface{}, 0, len(list))
	for _, entry := range list {
		v, ok := entry.(map[string]interface{})
		if !ok {
			continue
		}
		isManaged := boolValue(v["isManaged"])
		network := map[string]interface{}{
			"is_managed":     isManaged,
			"network_ext_id": stringValue(v["networkExtId"]),
			"vlan_id":        intValue(v["vlanId"]),
		}
		if !isManaged {
			network["default_gateway"] = flattenIPv4Value(v["defaultGateway"])
			network["subnet_mask"] = flattenIPv4Value(v["subnetMask"])
			network["ip_addresses"] = flattenNetworkIPAddresses([]interface{}{v})
		}
		result = append(result, map[string]interface{}{
			"is_managed":      network["is_managed"],
			"network_ext_id":  network["network_ext_id"],
			"vlan_id":         network["vlan_id"],
			"default_gateway": network["default_gateway"],
			"subnet_mask":     network["subnet_mask"],
			"ip_addresses":    network["ip_addresses"],
		})
	}
	return result
}

func flattenIPv4Value(raw interface{}) string {
	value, ok := raw.(map[string]interface{})
	if !ok {
		return ""
	}
	ipv4, ok := value["ipv4"].(map[string]interface{})
	if !ok {
		return ""
	}
	return stringValue(ipv4["value"])
}

func flattenNetworkIPAddresses(raw interface{}) []string {
	list, ok := raw.([]interface{})
	if !ok {
		return nil
	}

	var result []string
	for _, entry := range list {
		network, ok := entry.(map[string]interface{})
		if !ok {
			continue
		}
		addresses, ok := network["ipAddresses"].([]interface{})
		if !ok {
			continue
		}
		for _, address := range addresses {
			ip, ok := address.(map[string]interface{})
			if !ok {
				continue
			}
			ipv4, ok := ip["ipv4"].(map[string]interface{})
			if !ok {
				continue
			}
			if value := stringValue(ipv4["value"]); value != "" {
				result = append(result, value)
			}
		}
	}
	sort.Strings(result)
	return result
}

func flattenFileServerVMs(raw interface{}) []map[string]interface{} {
	list, ok := raw.([]interface{})
	if !ok {
		return nil
	}

	result := make([]map[string]interface{}, 0, len(list))
	for _, entry := range list {
		vm, ok := entry.(map[string]interface{})
		if !ok {
			continue
		}

		result = append(result, map[string]interface{}{
			"ext_id":                stringValue(vm["extId"]),
			"name":                  stringValue(vm["name"]),
			"fsvm_uuid":             stringValue(vm["fsvmUuid"]),
			"external_ip_addresses": flattenNetworkIPAddresses(vm["externalNetworks"]),
			"internal_ip_addresses": flattenNetworkIPAddresses(vm["internalNetworks"]),
		})
	}

	sort.Slice(result, func(i, j int) bool {
		left := result[i]["name"].(string)
		right := result[j]["name"].(string)
		if left == right {
			return result[i]["ext_id"].(string) < result[j]["ext_id"].(string)
		}
		return left < right
	})

	return result
}

func fileServerByNameRefreshFunc(apiClient *filesClient.ApiClient, name string) resource.StateRefreshFunc {
	return func() (interface{}, string, error) {
		item, err := getFileServerByName(apiClient, name)
		if err != nil {
			return nil, "", err
		}
		if item == nil {
			return map[string]interface{}{}, "NOT_FOUND", nil
		}
		return item, "FOUND", nil
	}
}

func fileServerByIDRefreshFunc(apiClient *filesClient.ApiClient, extID string) resource.StateRefreshFunc {
	return func() (interface{}, string, error) {
		item, notFound, err := getFileServerByID(apiClient, extID)
		if err != nil {
			return nil, "", err
		}
		if notFound || item == nil {
			return map[string]interface{}{}, "NOT_FOUND", nil
		}
		return item, "FOUND", nil
	}
}

func fileServerConfigRefreshFunc(apiClient *filesClient.ApiClient, extID string, desiredDNSRaw []interface{}, desiredNTPRaw []interface{}) resource.StateRefreshFunc {
	desiredDNS := normalizeValueList(expandValueList(desiredDNSRaw))
	desiredNTP := normalizeNTPServers(expandNTPServers(desiredNTPRaw))

	return func() (interface{}, string, error) {
		item, notFound, err := getFileServerByID(apiClient, extID)
		if err != nil {
			return nil, "", err
		}
		if notFound || item == nil {
			return nil, "PENDING", nil
		}

		currentDNS := normalizeValueList(flattenValueList(item["dnsServers"]))
		currentNTP := normalizeNTPServers(flattenNTPServers(item["ntpServers"]))
		if listValuesEqual(currentDNS, desiredDNS) && listValuesEqual(currentNTP, desiredNTP) {
			return item, "UPDATED", nil
		}

		return item, "PENDING", nil
	}
}

func updateFileServer(ctx context.Context, apiClient *filesClient.ApiClient, d *schema.ResourceData) error {
	_, _, headers, err := filesRequestWithHeaders(apiClient, http.MethodGet, filesAPIBasePath+"/"+d.Id(), nil, nil)
	if err != nil {
		return fmt.Errorf("error while reading file server %q before update: %w", d.Id(), err)
	}

	updateHeaders := map[string]string{}
	if etag := headers.Get("Etag"); etag != "" {
		updateHeaders["If-Match"] = etag
	}

	payload := expandFileServerPayload(d)
	respBody, statusCode, _, err := filesRequestWithHeaders(apiClient, http.MethodPut, filesAPIBasePath+"/"+d.Id(), payload, updateHeaders)
	if err != nil {
		return err
	}
	if statusCode >= http.StatusBadRequest || filesHasError(respBody) {
		return fmt.Errorf("%s", filesErrorMessage(respBody, statusCode))
	}

	stateConf := &resource.StateChangeConf{
		Pending:    []string{"PENDING"},
		Target:     []string{"UPDATED"},
		Refresh:    fileServerConfigRefreshFunc(apiClient, d.Id(), d.Get("dns_servers").([]interface{}), d.Get("ntp_servers").([]interface{})),
		Timeout:    30 * time.Minute,
		Delay:      5 * time.Second,
		MinTimeout: 5 * time.Second,
	}

	_, err = stateConf.WaitForStateContext(ctx)
	return err
}

func getFileServerByName(apiClient *filesClient.ApiClient, name string) (map[string]interface{}, error) {
	respBody, statusCode, err := filesRequest(apiClient, http.MethodGet, filesAPIBasePath, nil)
	if err != nil {
		return nil, err
	}
	if statusCode >= http.StatusBadRequest || filesHasError(respBody) {
		return nil, fmt.Errorf("%s", filesErrorMessage(respBody, statusCode))
	}

	data, ok := respBody["data"].([]interface{})
	if !ok {
		return nil, nil
	}
	for _, item := range data {
		m, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		if stringValue(m["name"]) == name {
			return m, nil
		}
	}
	return nil, nil
}

func getFileServerByID(apiClient *filesClient.ApiClient, extID string) (map[string]interface{}, bool, error) {
	respBody, statusCode, err := filesRequest(apiClient, http.MethodGet, filesAPIBasePath+"/"+extID, nil)
	if err != nil {
		return nil, false, err
	}
	if statusCode >= http.StatusBadRequest || filesHasError(respBody) {
		if filesIsNotFound(respBody) {
			return nil, true, nil
		}
		return nil, false, fmt.Errorf("%s", filesErrorMessage(respBody, statusCode))
	}

	data, ok := respBody["data"].(map[string]interface{})
	if !ok {
		return nil, false, fmt.Errorf("unexpected file server get response shape")
	}
	return data, false, nil
}

func refreshDirectoryServiceState(d *schema.ResourceData, apiClient *filesClient.ApiClient) error {
	ds, _, notFound, err := getDirectoryService(apiClient, d.Id())
	if err != nil {
		return err
	}
	if notFound || ds == nil {
		return d.Set("directory_service", []interface{}{})
	}

	flattened := flattenDirectoryService(ds)
	if len(flattened) == 0 {
		return d.Set("directory_service", []interface{}{})
	}
	return d.Set("directory_service", flattened)
}

func getDirectoryService(apiClient *filesClient.ApiClient, fileServerExtID string) (map[string]interface{}, string, bool, error) {
	basePath := fmt.Sprintf(filesDirectoryServicesPathTemplate, fileServerExtID)
	listResp, statusCode, err := filesRequest(apiClient, http.MethodGet, basePath, nil)
	if err != nil {
		return nil, "", false, err
	}
	if statusCode >= http.StatusBadRequest || filesHasError(listResp) {
		if filesIsNotFound(listResp) {
			return nil, "", true, nil
		}
		return nil, "", false, fmt.Errorf("%s", filesErrorMessage(listResp, statusCode))
	}

	data, _ := listResp["data"].([]interface{})
	if len(data) == 0 {
		return nil, "", true, nil
	}
	item, ok := data[0].(map[string]interface{})
	if !ok {
		return nil, "", false, fmt.Errorf("unexpected directory services list response shape")
	}
	dsExtID := stringValue(item["extId"])
	if dsExtID == "" {
		return nil, "", false, fmt.Errorf("directory service extId missing in list response")
	}

	getPath := basePath + "/" + dsExtID
	getResp, statusCode, headers, err := filesRequestWithHeaders(apiClient, http.MethodGet, getPath, nil, nil)
	if err != nil {
		return nil, "", false, err
	}
	if statusCode >= http.StatusBadRequest || filesHasError(getResp) {
		if filesIsNotFound(getResp) {
			return nil, "", true, nil
		}
		return nil, "", false, fmt.Errorf("%s", filesErrorMessage(getResp, statusCode))
	}

	dataObj, ok := getResp["data"].(map[string]interface{})
	if !ok {
		return nil, "", false, fmt.Errorf("unexpected directory service get response shape")
	}
	return dataObj, headers.Get("Etag"), false, nil
}

func updateDirectoryService(apiClient *filesClient.ApiClient, fileServerExtID string, values []interface{}) error {
	current, etag, notFound, err := getDirectoryService(apiClient, fileServerExtID)
	if err != nil {
		return err
	}
	if notFound || current == nil {
		return configureDirectoryService(apiClient, fileServerExtID, values)
	}

	dsExtID := stringValue(current["extId"])
	if dsExtID == "" {
		return fmt.Errorf("directory service extId missing in current configuration")
	}

	if len(values) == 0 || values[0] == nil {
		return unconfigureDirectoryService(apiClient, fileServerExtID, current, etag)
	}

	payload, err := buildDirectoryServicePayload(values)
	if err != nil {
		return err
	}

	path := fmt.Sprintf(filesDirectoryServicesPathTemplate, fileServerExtID) + "/" + dsExtID
	headers := map[string]string{}
	if etag != "" {
		headers["If-Match"] = etag
	}

	respBody, statusCode, _, err := filesRequestWithHeaders(apiClient, http.MethodPut, path, payload, headers)
	if err != nil {
		return err
	}
	if statusCode >= http.StatusBadRequest || filesHasError(respBody) {
		return fmt.Errorf("%s", filesErrorMessage(respBody, statusCode))
	}
	return nil
}

func configureDirectoryService(apiClient *filesClient.ApiClient, fileServerExtID string, values []interface{}) error {
	if len(values) == 0 || values[0] == nil {
		return nil
	}

	payload, err := buildDirectoryServiceActionPayload(values)
	if err != nil {
		return err
	}

	path := fmt.Sprintf(filesConfigureNameServicesPathTemplate, fileServerExtID)
	respBody, statusCode, _, err := filesRequestWithHeaders(apiClient, http.MethodPost, path, payload, nil)
	if err != nil {
		return err
	}
	if statusCode >= http.StatusBadRequest || filesHasError(respBody) {
		return fmt.Errorf("%s", filesErrorMessage(respBody, statusCode))
	}
	return nil
}

func unconfigureDirectoryService(apiClient *filesClient.ApiClient, fileServerExtID string, current map[string]interface{}, etag string) error {
	if _, hasLDAP := current["ldapDomain"].(map[string]interface{}); !hasLDAP && localDomainProtocol(current["localDomain"]) == "NFS" {
		return nil
	}

	nfsVersion := stringValue(current["nfsVersion"])
	if nfsVersion == "" {
		nfsVersion = "NFSV3V4"
	}

	payload := map[string]interface{}{
		"ldapDomain": map[string]interface{}{
			"protocolType": "NONE",
		},
		"localDomain": map[string]interface{}{
			"protocolType": "NFS",
		},
		"nfsVersion":  nfsVersion,
		"$objectType": "files.v4.config.NameServiceSpec",
	}
	if nfsV4Domain := stringValue(current["nfsV4Domain"]); nfsV4Domain != "" {
		payload["nfsV4Domain"] = nfsV4Domain
	}

	headers := map[string]string{}
	if etag != "" {
		headers["If-Match"] = etag
	}

	path := fmt.Sprintf(filesConfigureNameServicesPathTemplate, fileServerExtID)
	respBody, statusCode, _, err := filesRequestWithHeaders(apiClient, http.MethodPost, path, payload, headers)
	if err != nil {
		return err
	}
	if statusCode >= http.StatusBadRequest || filesHasError(respBody) {
		return fmt.Errorf("%s", filesErrorMessage(respBody, statusCode))
	}
	return nil
}

func localDomainProtocol(raw interface{}) string {
	if value := stringValue(raw); value != "" {
		return value
	}
	if value, ok := raw.(map[string]interface{}); ok {
		return stringValue(value["protocolType"])
	}
	return ""
}

func buildDirectoryServicePayload(values []interface{}) (map[string]interface{}, error) {
	if len(values) == 0 || values[0] == nil {
		return map[string]interface{}{
			"localDomain": "NONE",
			"nfsVersion":  "NFSV3V4",
		}, nil
	}

	root, ok := values[0].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid directory_service block type")
	}

	payload := map[string]interface{}{
		"localDomain": root["local_domain"].(string),
		"nfsVersion":  root["nfs_version"].(string),
	}
	if v, ok := root["nfs_v4_domain"].(string); ok && strings.TrimSpace(v) != "" {
		payload["nfsV4Domain"] = v
	}

	ldapRaw, hasLDAP := root["ldap_domain"].([]interface{})
	if hasLDAP && len(ldapRaw) > 0 && ldapRaw[0] != nil {
		ldap, ok := ldapRaw[0].(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("invalid ldap_domain block type")
		}

		servers := expandStringList(ldap["servers"].([]interface{}))
		sort.Strings(servers)
		location := strings.Join(servers, ",")
		if len(servers) == 1 {
			location = servers[0]
		}
		if location == "" {
			return nil, fmt.Errorf("ldap_domain.servers must not be empty")
		}

		ldapPayload := map[string]interface{}{
			"protocolType": ldap["protocol_type"].(string),
			"basedn":       ldap["base_dn"].(string),
			"location":     location,
		}
		if v, ok := ldap["bind_dn"].(string); ok && strings.TrimSpace(v) != "" {
			ldapPayload["binddn"] = v
		}
		if v, ok := ldap["bind_password"].(string); ok && strings.TrimSpace(v) != "" {
			ldapPayload["bindpw"] = v
		}
		payload["ldapDomain"] = ldapPayload
	}

	return payload, nil
}

func buildDirectoryServiceActionPayload(values []interface{}) (map[string]interface{}, error) {
	payload, err := buildDirectoryServicePayload(values)
	if err != nil {
		return nil, err
	}

	if localDomain := stringValue(payload["localDomain"]); localDomain != "" {
		payload["localDomain"] = map[string]interface{}{
			"protocolType": localDomain,
		}
	}
	if _, hasLDAP := payload["ldapDomain"]; !hasLDAP {
		payload["ldapDomain"] = map[string]interface{}{
			"protocolType": "NONE",
		}
	}
	payload["$objectType"] = "files.v4.config.NameServiceSpec"
	return payload, nil
}

func flattenDirectoryService(item map[string]interface{}) []map[string]interface{} {
	if len(item) == 0 {
		return nil
	}

	result := map[string]interface{}{}
	localDomain := localDomainProtocol(item["localDomain"])
	if localDomain != "" {
		result["local_domain"] = localDomain
	}
	nfsVersion := stringValue(item["nfsVersion"])
	if nfsVersion != "" {
		result["nfs_version"] = nfsVersion
	}
	if nfsV4Domain := stringValue(item["nfsV4Domain"]); nfsV4Domain != "" {
		result["nfs_v4_domain"] = nfsV4Domain
	}

	if ldapRaw, ok := item["ldapDomain"].(map[string]interface{}); ok {
		ldap := map[string]interface{}{}
		if protocolType := stringValue(ldapRaw["protocolType"]); protocolType != "" {
			ldap["protocol_type"] = protocolType
		}
		if baseDN := stringValue(ldapRaw["basedn"]); baseDN != "" {
			ldap["base_dn"] = baseDN
		}
		if bindDN := stringValue(ldapRaw["binddn"]); bindDN != "" {
			ldap["bind_dn"] = bindDN
		}
		if location := stringValue(ldapRaw["location"]); location != "" {
			ldap["servers"] = splitLDAPLocations(location)
		}
		if len(ldap) > 0 {
			result["ldap_domain"] = []map[string]interface{}{ldap}
		}
	}

	if len(result) == 0 {
		return nil
	}
	return []map[string]interface{}{result}
}

func splitLDAPLocations(raw string) []string {
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		v := strings.TrimSpace(p)
		if v == "" {
			continue
		}
		out = append(out, v)
	}
	return out
}

func expandStringList(values []interface{}) []string {
	result := make([]string, 0, len(values))
	for _, v := range values {
		s, ok := v.(string)
		if !ok {
			continue
		}
		if strings.TrimSpace(s) == "" {
			continue
		}
		result = append(result, s)
	}
	return result
}

func filesRequest(apiClient *filesClient.ApiClient, method, uri string, body interface{}) (map[string]interface{}, int, error) {
	respBody, statusCode, _, err := filesRequestWithHeaders(apiClient, method, uri, body, nil)
	return respBody, statusCode, err
}

func filesRequestWithHeaders(apiClient *filesClient.ApiClient, method, uri string, body interface{}, headers map[string]string) (map[string]interface{}, int, http.Header, error) {
	if apiClient == nil {
		return nil, 0, nil, fmt.Errorf("files api client is nil")
	}

	scheme := apiClient.Scheme
	if scheme == "" {
		scheme = "https"
	}
	port := apiClient.Port
	if port == 0 {
		port = 9440
	}

	url := fmt.Sprintf("%s://%s:%d%s", scheme, apiClient.Host, port, uri)
	var reqBody io.Reader
	if body != nil {
		payload, err := json.Marshal(body)
		if err != nil {
			return nil, 0, nil, err
		}
		reqBody = bytes.NewReader(payload)
	}

	req, err := http.NewRequest(method, url, reqBody)
	if err != nil {
		return nil, 0, nil, err
	}
	req.SetBasicAuth(apiClient.Username, apiClient.Password)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	for key, value := range headers {
		req.Header.Set(key, value)
	}

	httpClient := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: !apiClient.VerifySSL,
			},
		},
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, 0, nil, err
	}
	defer resp.Body.Close()

	payload, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, resp.Header, err
	}
	if len(payload) == 0 {
		return map[string]interface{}{}, resp.StatusCode, resp.Header, nil
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(payload, &parsed); err != nil {
		var text string
		if stringErr := json.Unmarshal(payload, &text); stringErr == nil {
			return map[string]interface{}{"error": text}, resp.StatusCode, resp.Header, nil
		}
		return nil, resp.StatusCode, resp.Header, fmt.Errorf("unable to parse response body: %w", err)
	}
	return parsed, resp.StatusCode, resp.Header, nil
}

func filesHasError(resp map[string]interface{}) bool {
	metadata, ok := resp["metadata"].(map[string]interface{})
	if !ok {
		return false
	}
	flags, ok := metadata["flags"].([]interface{})
	if !ok {
		return false
	}
	for _, f := range flags {
		flag, ok := f.(map[string]interface{})
		if !ok {
			continue
		}
		if stringValue(flag["name"]) == "hasError" {
			return boolValue(flag["value"])
		}
	}
	return false
}

func filesIsNotFound(resp map[string]interface{}) bool {
	msg := strings.ToLower(filesErrorMessage(resp, 0))
	return strings.Contains(msg, "entity_not_found") ||
		strings.Contains(msg, "cannot be found") ||
		strings.Contains(msg, "failed to fetch data replication policy by extid") ||
		strings.Contains(msg, "fil-40011")
}

func filesErrorMessage(resp map[string]interface{}, statusCode int) string {
	var msgs []string

	data, _ := resp["data"].(map[string]interface{})
	errData, _ := data["error"].(interface{})

	switch e := errData.(type) {
	case map[string]interface{}:
		if validationMsgs, ok := e["validationErrorMessages"].([]interface{}); ok {
			for _, vm := range validationMsgs {
				vmMap, ok := vm.(map[string]interface{})
				if !ok {
					continue
				}
				if msg := stringValue(vmMap["message"]); msg != "" {
					msgs = append(msgs, msg)
				}
			}
		}
		if msg := stringValue(e["message"]); msg != "" {
			msgs = append(msgs, msg)
		}
	case []interface{}:
		for _, entry := range e {
			errMap, ok := entry.(map[string]interface{})
			if !ok {
				continue
			}
			if msg := stringValue(errMap["message"]); msg != "" {
				msgs = append(msgs, msg)
			}
		}
	}

	if len(msgs) > 0 {
		return strings.Join(msgs, "; ")
	}
	if msg := stringValue(resp["error"]); msg != "" {
		return msg
	}
	if len(resp) > 0 {
		if payload, err := json.Marshal(resp); err == nil {
			return string(payload)
		}
	}
	if statusCode > 0 {
		return fmt.Sprintf("request failed with status code %d", statusCode)
	}
	return "request failed"
}

func stringValue(v interface{}) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func intValue(v interface{}) int {
	switch t := v.(type) {
	case float64:
		return int(t)
	case int:
		return t
	default:
		return 0
	}
}

func boolValue(v interface{}) bool {
	if b, ok := v.(bool); ok {
		return b
	}
	return false
}
