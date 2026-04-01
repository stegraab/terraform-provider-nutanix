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
	filesClient "github.com/nutanix/ntnx-api-golang-clients/files-go-client/v4/client"
	conns "github.com/terraform-providers/terraform-provider-nutanix/nutanix"
)

const filesAPIBasePath = "/api/files/v4.0.a6/config/file-servers"
const filesDirectoryServicesPathTemplate = "/api/files/v4.0/config/file-servers/%s/directory-services"

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
			"ntp_servers": {
				Type:     schema.TypeList,
				Optional: true,
				ForceNew: true,
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
						},
						"network_ext_id": {
							Type:     schema.TypeString,
							Required: true,
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
						},
						"network_ext_id": {
							Type:     schema.TypeString,
							Required: true,
						},
					},
				},
			},
			"deployment_status": {
				Type:     schema.TypeString,
				Computed: true,
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

	respBody, statusCode, err := filesRequest(apiClient, http.MethodDelete, filesAPIBasePath+"/"+d.Id(), nil)
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
	return map[string]interface{}{
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

func expandNetworkList(values []interface{}) []map[string]interface{} {
	result := make([]map[string]interface{}, 0, len(values))
	for _, entry := range values {
		if entry == nil {
			continue
		}
		v := entry.(map[string]interface{})
		result = append(result, map[string]interface{}{
			"isManaged":    v["is_managed"].(bool),
			"networkExtId": v["network_ext_id"].(string),
		})
	}
	return result
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
		result = append(result, map[string]interface{}{
			"is_managed":     boolValue(v["isManaged"]),
			"network_ext_id": stringValue(v["networkExtId"]),
		})
	}
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

func getFileServerByName(apiClient *filesClient.ApiClient, name string) (map[string]interface{}, error) {
	respBody, statusCode, err := filesRequest(apiClient, http.MethodGet, filesAPIBasePath, nil)
	if err != nil {
		return nil, err
	}
	if statusCode >= http.StatusBadRequest || filesHasError(respBody) {
		return nil, fmt.Errorf(filesErrorMessage(respBody, statusCode))
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
		return nil, false, fmt.Errorf(filesErrorMessage(respBody, statusCode))
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
		return nil
	}

	flattened := flattenDirectoryService(ds)
	if len(flattened) == 0 {
		return nil
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
		return nil, "", false, fmt.Errorf(filesErrorMessage(listResp, statusCode))
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
		return nil, "", false, fmt.Errorf(filesErrorMessage(getResp, statusCode))
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
		return fmt.Errorf("directory service configuration for file server %q was not found", fileServerExtID)
	}

	dsExtID := stringValue(current["extId"])
	if dsExtID == "" {
		return fmt.Errorf("directory service extId missing in current configuration")
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
		return fmt.Errorf(filesErrorMessage(respBody, statusCode))
	}
	return nil
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

func flattenDirectoryService(item map[string]interface{}) []map[string]interface{} {
	if len(item) == 0 {
		return nil
	}

	result := map[string]interface{}{}
	localDomain := stringValue(item["localDomain"])
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
