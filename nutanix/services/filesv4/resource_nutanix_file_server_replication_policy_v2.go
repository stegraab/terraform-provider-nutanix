package filesv4

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"
	conns "github.com/terraform-providers/terraform-provider-nutanix/nutanix"
	filesClient "github.com/terraform-providers/terraform-provider-nutanix/nutanix/sdks/v4/files"
)

const filesReplicationPoliciesPath = "/api/files/v4.1.a2/config/replication-policies"

func ResourceNutanixFileServerReplicationPolicyV2() *schema.Resource {
	return &schema.Resource{
		CreateContext: resourceNutanixFileServerReplicationPolicyV2Create,
		ReadContext:   resourceNutanixFileServerReplicationPolicyV2Read,
		DeleteContext: resourceNutanixFileServerReplicationPolicyV2Delete,
		Importer: &schema.ResourceImporter{
			StateContext: schema.ImportStatePassthroughContext,
		},
		Timeouts: &schema.ResourceTimeout{
			Create: schema.DefaultTimeout(30 * time.Minute),
			Delete: schema.DefaultTimeout(30 * time.Minute),
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
				ValidateFunc: validation.StringIsNotEmpty,
			},
			"type": {
				Type:         schema.TypeString,
				Optional:     true,
				Default:      "SMART_DR",
				ForceNew:     true,
				ValidateFunc: validation.StringInSlice([]string{"SMART_DR", "DATA_SYNC", "VDI_SYNC", "METRO"}, false),
			},
			"primary_file_server_ext_id": {
				Type:         schema.TypeString,
				Required:     true,
				ForceNew:     true,
				ValidateFunc: validation.StringIsNotEmpty,
			},
			"secondary_file_server_ext_id": {
				Type:         schema.TypeString,
				Optional:     true,
				ForceNew:     true,
				ValidateFunc: validation.StringIsNotEmpty,
			},
			"primary_cluster_ext_id": {
				Type:         schema.TypeString,
				Optional:     true,
				ForceNew:     true,
				ValidateFunc: validation.StringIsNotEmpty,
			},
			"secondary_cluster_ext_id": {
				Type:         schema.TypeString,
				Optional:     true,
				ForceNew:     true,
				ValidateFunc: validation.StringIsNotEmpty,
			},
			"primary_domain_manager_ext_id": {
				Type:         schema.TypeString,
				Optional:     true,
				ForceNew:     true,
				ValidateFunc: validation.StringIsNotEmpty,
			},
			"secondary_domain_manager_ext_id": {
				Type:         schema.TypeString,
				Optional:     true,
				ForceNew:     true,
				ValidateFunc: validation.StringIsNotEmpty,
			},
			"witness_ext_id": {
				Type:         schema.TypeString,
				Optional:     true,
				ForceNew:     true,
				ValidateFunc: validation.StringIsNotEmpty,
			},
			"witness_timeout_secs": {
				Type:         schema.TypeInt,
				Optional:     true,
				Default:      30,
				ForceNew:     true,
				ValidateFunc: validation.IntAtLeast(1),
			},
			"network_mapping": {
				Type:     schema.TypeList,
				Optional: true,
				ForceNew: true,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"primary_subnet_name": {
							Type:         schema.TypeString,
							Optional:     true,
							ForceNew:     true,
							ValidateFunc: validation.StringIsNotEmpty,
						},
						"primary_subnet_ext_id": {
							Type:         schema.TypeString,
							Optional:     true,
							ForceNew:     true,
							ValidateFunc: validation.StringIsNotEmpty,
						},
						"recovery_subnet_name": {
							Type:         schema.TypeString,
							Optional:     true,
							ForceNew:     true,
							ValidateFunc: validation.StringIsNotEmpty,
						},
						"recovery_subnet_ext_id": {
							Type:         schema.TypeString,
							Optional:     true,
							ForceNew:     true,
							ValidateFunc: validation.StringIsNotEmpty,
						},
						"primary_vpc_ext_id": {
							Type:         schema.TypeString,
							Optional:     true,
							ForceNew:     true,
							ValidateFunc: validation.StringIsNotEmpty,
						},
						"recovery_vpc_ext_id": {
							Type:         schema.TypeString,
							Optional:     true,
							ForceNew:     true,
							ValidateFunc: validation.StringIsNotEmpty,
						},
					},
				},
			},
			"status": {
				Type:         schema.TypeString,
				Optional:     true,
				Default:      "ENABLED",
				ForceNew:     true,
				ValidateFunc: validation.StringInSlice([]string{"ENABLED", "DISABLED"}, false),
			},
			"should_include_new_mount_targets": {
				Type:     schema.TypeBool,
				Optional: true,
				Default:  true,
				ForceNew: true,
			},
			"schedule_frequency": {
				Type:         schema.TypeInt,
				Optional:     true,
				Default:      1,
				ForceNew:     true,
				ValidateFunc: validation.IntAtLeast(1),
			},
			"schedule_interval": {
				Type:         schema.TypeString,
				Optional:     true,
				Default:      "MINUTE",
				ForceNew:     true,
				ValidateFunc: validation.StringInSlice([]string{"YEAR", "MONTH", "WEEK", "DAY", "HOUR", "MINUTE", "SECOND"}, false),
			},
			"replication_entities": {
				Type:     schema.TypeList,
				Optional: true,
				Computed: true,
				ForceNew: true,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"primary_file_server_mount_target_ext_id": {
							Type:     schema.TypeString,
							Optional: true,
							Computed: true,
							ForceNew: true,
						},
						"primary_file_server_mount_target_path": {
							Type:     schema.TypeString,
							Required: true,
							ForceNew: true,
						},
						"secondary_file_server_mount_target_ext_id": {
							Type:     schema.TypeString,
							Computed: true,
						},
						"secondary_file_server_mount_target_path": {
							Type:     schema.TypeString,
							Required: true,
							ForceNew: true,
						},
					},
				},
			},
		},
	}
}

func resourceNutanixFileServerReplicationPolicyV2Create(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	apiClient := meta.(*conns.Client).FilesAPI.APIClientInstance
	if apiClient == nil {
		return diag.Errorf("files api client is not initialized")
	}

	name := d.Get("name").(string)
	existing, err := getFileServerReplicationPolicyByName(apiClient, name)
	if err != nil {
		return diag.FromErr(err)
	}
	if existing != nil {
		d.SetId(stringValue(existing["extId"]))
		return putFileServerReplicationPolicyWithRetry(ctx, apiClient, d.Id(), d)
	}

	respBody, statusCode, err := filesRequest(apiClient, http.MethodPost, filesReplicationPoliciesPath, buildFileServerReplicationPolicyPayload(d))
	if err != nil {
		return diag.Errorf("error while creating file server replication policy: %v", err)
	}
	if statusCode >= http.StatusBadRequest || filesHasError(respBody) {
		return diag.Errorf("error while creating file server replication policy: %s", filesErrorMessage(respBody, statusCode))
	}

	stateConf := &resource.StateChangeConf{
		Pending: []string{"NOT_FOUND"},
		Target:  []string{"FOUND"},
		Refresh: fileServerReplicationPolicyByNameRefreshFunc(apiClient, name),
		Timeout: d.Timeout(schema.TimeoutCreate),
	}
	result, err := stateConf.WaitForStateContext(ctx)
	if err != nil {
		return diag.Errorf("error waiting for file server replication policy %q to be available: %v", name, err)
	}

	created, ok := result.(map[string]interface{})
	if !ok {
		return diag.Errorf("unexpected file server replication policy state type: %T", result)
	}
	extID := stringValue(created["extId"])
	if extID == "" {
		return diag.Errorf("unable to determine file server replication policy extId from create/list response")
	}

	d.SetId(extID)
	flattenFileServerReplicationPolicyToState(d, created)
	return nil
}

func putFileServerReplicationPolicyWithRetry(ctx context.Context, apiClient *filesClient.ApiClient, extID string, d *schema.ResourceData) diag.Diagnostics {
	name := d.Get("name").(string)
	deadline := time.Now().Add(d.Timeout(schema.TimeoutCreate))

	for {
		_, etag, notFound, err := getFileServerReplicationPolicyByID(apiClient, extID)
		if err != nil {
			return diag.Errorf("error reading existing file server replication policy %q before update: %v", extID, err)
		}
		if notFound {
			d.SetId("")
			return nil
		}

		headers := map[string]string{}
		if etag != "" {
			headers["If-Match"] = etag
		}

		respBody, statusCode, _, err := filesRequestWithHeaders(apiClient, http.MethodPut, filesReplicationPoliciesPath+"/"+extID, buildFileServerReplicationPolicyPayload(d), headers)
		if err != nil {
			return diag.Errorf("error while updating existing file server replication policy %q: %v", extID, err)
		}
		if statusCode < http.StatusBadRequest && !filesHasError(respBody) {
			item, _, notFound, err := getFileServerReplicationPolicyByID(apiClient, extID)
			if err != nil {
				return diag.Errorf("error reading file server replication policy %q after update: %v", extID, err)
			}
			if notFound {
				d.SetId("")
				return nil
			}
			flattenFileServerReplicationPolicyToState(d, item)
			return nil
		}

		errMsg := filesErrorMessage(respBody, statusCode)
		if statusCode == http.StatusConflict && strings.Contains(errMsg, "another policy task with the same primary File Server is in progress") && time.Now().Before(deadline) {
			select {
			case <-ctx.Done():
				return diag.FromErr(ctx.Err())
			case <-time.After(30 * time.Second):
				continue
			}
		}
		return diag.Errorf("error while updating existing file server replication policy %q: %s", name, errMsg)
	}
}

func resourceNutanixFileServerReplicationPolicyV2Read(_ context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	apiClient := meta.(*conns.Client).FilesAPI.APIClientInstance
	if apiClient == nil {
		return diag.Errorf("files api client is not initialized")
	}

	item, _, notFound, err := getFileServerReplicationPolicyByID(apiClient, d.Id())
	if err != nil {
		if name := d.Get("name").(string); name != "" {
			itemByName, nameErr := getFileServerReplicationPolicyByName(apiClient, name)
			if nameErr != nil {
				return diag.Errorf("error while reading file server replication policy %q: %v; fallback lookup by name failed: %v", d.Id(), err, nameErr)
			}
			if itemByName != nil {
				d.SetId(stringValue(itemByName["extId"]))
				flattenFileServerReplicationPolicyToState(d, itemByName)
				return nil
			}
		}
		return diag.Errorf("error while reading file server replication policy %q: %v", d.Id(), err)
	}
	if notFound {
		d.SetId("")
		return nil
	}

	flattenFileServerReplicationPolicyToState(d, item)
	return nil
}

func resourceNutanixFileServerReplicationPolicyV2Delete(_ context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	apiClient := meta.(*conns.Client).FilesAPI.APIClientInstance
	if apiClient == nil {
		return diag.Errorf("files api client is not initialized")
	}

	_, etag, notFound, err := getFileServerReplicationPolicyByID(apiClient, d.Id())
	if err != nil {
		return diag.Errorf("error while reading file server replication policy %q before delete: %v", d.Id(), err)
	}
	if notFound {
		return nil
	}

	headers := map[string]string{}
	if etag != "" {
		headers["If-Match"] = etag
	}

	respBody, statusCode, _, err := filesRequestWithHeaders(apiClient, http.MethodDelete, filesReplicationPoliciesPath+"/"+d.Id(), nil, headers)
	if err != nil {
		return diag.Errorf("error while deleting file server replication policy %q: %v", d.Id(), err)
	}
	if statusCode >= http.StatusBadRequest || filesHasError(respBody) {
		return diag.Errorf("error while deleting file server replication policy %q: %s", d.Id(), filesErrorMessage(respBody, statusCode))
	}

	d.SetId("")
	return nil
}

func buildFileServerReplicationPolicyPayload(d *schema.ResourceData) map[string]interface{} {
	payload := map[string]interface{}{
		"name":                         d.Get("name").(string),
		"type":                         d.Get("type").(string),
		"status":                       d.Get("status").(string),
		"shouldIncludeNewMountTargets": d.Get("should_include_new_mount_targets").(bool),
	}

	if d.Get("type").(string) == "METRO" {
		config := map[string]interface{}{
			"primaryFileServerExtId":      d.Get("primary_file_server_ext_id").(string),
			"secondaryFileServerExtId":    d.Get("secondary_file_server_ext_id").(string),
			"primaryClusterExtId":         d.Get("primary_cluster_ext_id").(string),
			"secondaryClusterExtId":       d.Get("secondary_cluster_ext_id").(string),
			"primaryDomainManagerExtId":   d.Get("primary_domain_manager_ext_id").(string),
			"secondaryDomainManagerExtId": d.Get("secondary_domain_manager_ext_id").(string),
			"status":                      d.Get("status").(string),
			"witness": map[string]interface{}{
				"timeoutSecs": d.Get("witness_timeout_secs").(int),
			},
			"networkMappings": expandFileServerReplicationNetworkMappings(d.Get("network_mapping").([]interface{})),
		}
		if witnessExtID := strings.TrimSpace(d.Get("witness_ext_id").(string)); witnessExtID != "" {
			config["witness"].(map[string]interface{})["extId"] = witnessExtID
		}
		payload["replicationConfigurations"] = []map[string]interface{}{config}
		return payload
	}

	payload["replicationConfigurations"] = []map[string]interface{}{
		{
			"primaryFileServerExtId":   d.Get("primary_file_server_ext_id").(string),
			"secondaryFileServerExtId": d.Get("secondary_file_server_ext_id").(string),
			"status":                   d.Get("status").(string),
			"replicationEntities":      expandFileServerReplicationEntities(d.Get("replication_entities").([]interface{})),
			"schedule": map[string]interface{}{
				"frequency":        d.Get("schedule_frequency").(int),
				"scheduleInterval": d.Get("schedule_interval").(string),
			},
		},
	}
	return payload
}

func expandFileServerReplicationNetworkMappings(values []interface{}) []map[string]interface{} {
	result := make([]map[string]interface{}, 0, len(values))
	for _, entry := range values {
		if entry == nil {
			continue
		}
		v := entry.(map[string]interface{})
		result = append(result, map[string]interface{}{
			"primaryNetwork":  buildFileServerReplicationNetworkReference(stringValue(v["primary_subnet_name"]), stringValue(v["primary_subnet_ext_id"]), stringValue(v["primary_vpc_ext_id"])),
			"recoveryNetwork": buildFileServerReplicationNetworkReference(stringValue(v["recovery_subnet_name"]), stringValue(v["recovery_subnet_ext_id"]), stringValue(v["recovery_vpc_ext_id"])),
		})
	}
	return result
}

func buildFileServerReplicationNetworkReference(subnetName, subnetExtID, vpcExtID string) map[string]interface{} {
	subnetReference := map[string]interface{}{
		"$objectType": "datapolicies.v4.config.NameReference",
		"name":        subnetName,
	}
	if subnetExtID != "" {
		subnetReference = map[string]interface{}{
			"$objectType": "datapolicies.v4.config.ExtIdReference",
			"extId":       subnetExtID,
		}
	}
	network := map[string]interface{}{
		"subnetReference": subnetReference,
	}
	if vpcExtID != "" {
		network["vpcExtId"] = vpcExtID
	}
	return network
}

func expandFileServerReplicationEntities(values []interface{}) []map[string]interface{} {
	result := make([]map[string]interface{}, 0, len(values))
	for _, entry := range values {
		if entry == nil {
			continue
		}
		v := entry.(map[string]interface{})
		entity := map[string]interface{}{
			"primaryFileServerMountTargetPath":   v["primary_file_server_mount_target_path"].(string),
			"secondaryFileServerMountTargetPath": v["secondary_file_server_mount_target_path"].(string),
		}
		if extID := strings.TrimSpace(v["primary_file_server_mount_target_ext_id"].(string)); extID != "" {
			entity["primaryFileServerMountTargetExtId"] = extID
		}
		result = append(result, entity)
	}
	return result
}

func flattenFileServerReplicationPolicyToState(d *schema.ResourceData, item map[string]interface{}) {
	if extID := stringValue(item["extId"]); extID != "" {
		d.SetId(extID)
		_ = d.Set("ext_id", extID)
	}
	_ = d.Set("name", stringValue(item["name"]))
	_ = d.Set("type", stringValue(item["type"]))
	_ = d.Set("should_include_new_mount_targets", boolValue(item["shouldIncludeNewMountTargets"]))
	_ = d.Set("status", stringValue(item["status"]))

	config := firstReplicationConfiguration(item)
	if len(config) == 0 {
		return
	}
	_ = d.Set("primary_file_server_ext_id", stringValue(config["primaryFileServerExtId"]))
	_ = d.Set("secondary_file_server_ext_id", stringValue(config["secondaryFileServerExtId"]))
	_ = d.Set("primary_cluster_ext_id", stringValue(config["primaryClusterExtId"]))
	_ = d.Set("secondary_cluster_ext_id", stringValue(config["secondaryClusterExtId"]))
	_ = d.Set("primary_domain_manager_ext_id", stringValue(config["primaryDomainManagerExtId"]))
	_ = d.Set("secondary_domain_manager_ext_id", stringValue(config["secondaryDomainManagerExtId"]))
	if witness, ok := config["witness"].(map[string]interface{}); ok {
		_ = d.Set("witness_ext_id", stringValue(witness["extId"]))
		if timeout := intValue(witness["timeoutSecs"]); timeout > 0 {
			_ = d.Set("witness_timeout_secs", timeout)
		}
	}
	if status := stringValue(config["status"]); status != "" {
		_ = d.Set("status", status)
	}
	if schedule, ok := config["schedule"].(map[string]interface{}); ok {
		_ = d.Set("schedule_frequency", intValue(schedule["frequency"]))
		_ = d.Set("schedule_interval", stringValue(schedule["scheduleInterval"]))
	}
}

func flattenFileServerReplicationEntities(raw interface{}) []map[string]interface{} {
	list, ok := raw.([]interface{})
	if !ok {
		return nil
	}
	result := make([]map[string]interface{}, 0, len(list))
	for _, entry := range list {
		entity, ok := entry.(map[string]interface{})
		if !ok {
			continue
		}
		result = append(result, map[string]interface{}{
			"primary_file_server_mount_target_ext_id":   stringValue(entity["primaryFileServerMountTargetExtId"]),
			"primary_file_server_mount_target_path":     stringValue(entity["primaryFileServerMountTargetPath"]),
			"secondary_file_server_mount_target_ext_id": stringValue(entity["secondaryFileServerMountTargetExtId"]),
			"secondary_file_server_mount_target_path":   stringValue(entity["secondaryFileServerMountTargetPath"]),
		})
	}
	return result
}

func firstReplicationConfiguration(item map[string]interface{}) map[string]interface{} {
	configs, ok := item["replicationConfigurations"].([]interface{})
	if !ok || len(configs) == 0 {
		return nil
	}
	config, _ := configs[0].(map[string]interface{})
	return config
}

func fileServerReplicationPolicyByNameRefreshFunc(apiClient *filesClient.ApiClient, name string) resource.StateRefreshFunc {
	return func() (interface{}, string, error) {
		item, err := getFileServerReplicationPolicyByName(apiClient, name)
		if err != nil {
			return nil, "", err
		}
		if item == nil {
			return map[string]interface{}{}, "NOT_FOUND", nil
		}
		return item, "FOUND", nil
	}
}

func getFileServerReplicationPolicyByName(apiClient *filesClient.ApiClient, name string) (map[string]interface{}, error) {
	respBody, statusCode, err := filesRequest(apiClient, http.MethodGet, filesReplicationPoliciesPath, nil)
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

func getFileServerReplicationPolicyByID(apiClient *filesClient.ApiClient, extID string) (map[string]interface{}, string, bool, error) {
	respBody, statusCode, headers, err := filesRequestWithHeaders(apiClient, http.MethodGet, filesReplicationPoliciesPath+"/"+extID, nil, nil)
	if err != nil {
		return nil, "", false, err
	}
	if statusCode >= http.StatusBadRequest || filesHasError(respBody) {
		if filesIsNotFound(respBody) {
			return nil, "", true, nil
		}
		return nil, "", false, fmt.Errorf("%s", filesErrorMessage(respBody, statusCode))
	}

	data, ok := respBody["data"].(map[string]interface{})
	if !ok {
		return nil, "", false, fmt.Errorf("unexpected file server replication policy get response shape")
	}
	return data, headers.Get("Etag"), false, nil
}
