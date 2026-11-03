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

const filesMountTargetsPathTemplate = "/api/files/v4.0/config/file-servers/%s/mount-targets"

func ResourceNutanixFileServerMountTargetV2() *schema.Resource {
	return &schema.Resource{
		CreateContext: resourceNutanixFileServerMountTargetV2Create,
		ReadContext:   resourceNutanixFileServerMountTargetV2Read,
		UpdateContext: resourceNutanixFileServerMountTargetV2Update,
		CustomizeDiff: validateMountTargetProtocol,
		DeleteContext: resourceNutanixFileServerMountTargetV2Delete,
		Importer: &schema.ResourceImporter{
			StateContext: schema.ImportStatePassthroughContext,
		},
		Timeouts: &schema.ResourceTimeout{
			Create: schema.DefaultTimeout(10 * time.Minute),
			Update: schema.DefaultTimeout(10 * time.Minute),
		},
		Schema: map[string]*schema.Schema{
			"file_server_ext_id": {
				Type:         schema.TypeString,
				Required:     true,
				ForceNew:     true,
				ValidateFunc: validation.StringIsNotEmpty,
			},
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
			"path": {
				Type:         schema.TypeString,
				Required:     true,
				ForceNew:     true,
				ValidateFunc: validation.StringIsNotEmpty,
			},
			"max_size_gb": {
				Type:         schema.TypeInt,
				Required:     true,
				ForceNew:     true,
				ValidateFunc: validation.IntAtLeast(1),
			},
			"protocol": {
				Type: schema.TypeString, Optional: true, Default: "NFS", ForceNew: true,
				ValidateFunc: validation.StringInSlice([]string{"NFS", "SMB"}, false),
			},
			"smb_properties": smbMountTargetSchema(),
			"state": {
				Type:     schema.TypeString,
				Computed: true,
			},
			"status_type": {
				Type:     schema.TypeString,
				Computed: true,
			},
		},
	}
}

func resourceNutanixFileServerMountTargetV2Create(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	apiClient := meta.(*conns.Client).FilesAPI.APIClientInstance
	if apiClient == nil {
		return diag.Errorf("files api client is not initialized")
	}

	existing, _, err := getFileServerMountTargetByPath(apiClient, d.Get("file_server_ext_id").(string), d.Get("path").(string))
	if err != nil {
		return diag.FromErr(err)
	}
	if existing != nil {
		if stringValue(existing["name"]) != d.Get("name").(string) || stringValue(existing["protocol"]) != d.Get("protocol").(string) {
			return diag.Errorf("existing mount target at the requested path has a different name or protocol; refusing to adopt it")
		}
		d.SetId(stringValue(existing["extId"]))
		_ = d.Set("ext_id", d.Id())
		return resourceNutanixFileServerMountTargetV2Read(ctx, d, meta)
	}

	respBody, statusCode, err := filesRequest(apiClient, http.MethodPost, fileServerMountTargetsPath(d), buildFileServerMountTargetPayload(d))
	if err != nil {
		return diag.Errorf("error while creating file server mount target: %v", err)
	}
	if statusCode >= http.StatusBadRequest || filesHasError(respBody) {
		return diag.Errorf("error while creating file server mount target: %s", filesErrorMessage(respBody, statusCode))
	}

	name := d.Get("name").(string)
	path := d.Get("path").(string)
	stateConf := &resource.StateChangeConf{
		Pending: []string{"NOT_FOUND"},
		Target:  []string{"FOUND"},
		Refresh: fileServerMountTargetByPathRefreshFunc(apiClient, d.Get("file_server_ext_id").(string), name, path),
		Timeout: d.Timeout(schema.TimeoutCreate),
	}
	result, err := stateConf.WaitForStateContext(ctx)
	if err != nil {
		return diag.Errorf("error waiting for file server mount target %q to be available: %v", name, err)
	}

	data, ok := result.(map[string]interface{})
	if !ok {
		return diag.Errorf("unexpected file server mount target state type: %T", result)
	}
	extID := stringValue(data["extId"])
	if extID == "" {
		return diag.Errorf("unable to determine file server mount target extId from create/list response")
	}

	d.SetId(extID)
	_ = d.Set("ext_id", extID)
	return resourceNutanixFileServerMountTargetV2Read(ctx, d, meta)
}

func resourceNutanixFileServerMountTargetV2Read(_ context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	apiClient := meta.(*conns.Client).FilesAPI.APIClientInstance
	if apiClient == nil {
		return diag.Errorf("files api client is not initialized")
	}

	item, _, notFound, err := getFileServerMountTargetByID(apiClient, d.Get("file_server_ext_id").(string), d.Id())
	if err != nil {
		return diag.FromErr(err)
	}
	if notFound {
		d.SetId("")
		return nil
	}

	_ = d.Set("ext_id", d.Id())
	_ = d.Set("name", stringValue(item["name"]))
	_ = d.Set("path", stringValue(item["path"]))
	_ = d.Set("max_size_gb", intValue(item["maxSizeGB"]))
	_ = d.Set("protocol", stringValue(item["protocol"]))
	if properties, ok := item["smbProperties"].(map[string]interface{}); ok && stringValue(item["protocol"]) == "SMB" {
		acl := []interface{}{}
		for _, raw := range mountTargetSlice(properties["shareACL"]) {
			ace, ok := raw.(map[string]interface{})
			if !ok {
				continue
			}
			acl = append(acl, map[string]interface{}{"user_or_group_name": stringValue(ace["userOrGroupName"]), "access_type": stringValue(ace["accessType"]), "permission_type": stringValue(ace["permissionType"])})
		}
		_ = d.Set("smb_properties", []interface{}{map[string]interface{}{"share_acl": acl, "is_smb3_encryption_enabled": properties["isSmb3EncryptionEnabled"], "is_access_based_enumeration_enabled": properties["isAccessBasedEnumerationEnabled"]}})
	} else {
		_ = d.Set("smb_properties", nil)
	}
	_ = d.Set("state", stringValue(item["state"]))
	_ = d.Set("status_type", stringValue(item["statusType"]))
	return nil
}

func resourceNutanixFileServerMountTargetV2Delete(_ context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	apiClient := meta.(*conns.Client).FilesAPI.APIClientInstance
	if apiClient == nil {
		return diag.Errorf("files api client is not initialized")
	}

	_, etag, notFound, err := getFileServerMountTargetByID(apiClient, d.Get("file_server_ext_id").(string), d.Id())
	if err != nil {
		return diag.FromErr(err)
	}
	if notFound {
		return nil
	}

	headers := map[string]string{}
	if etag != "" {
		headers["If-Match"] = etag
	}
	respBody, statusCode, _, err := filesRequestWithHeaders(apiClient, http.MethodDelete, fileServerMountTargetsPath(d)+"/"+d.Id(), nil, headers)
	if err != nil {
		return diag.Errorf("error while deleting file server mount target %q: %v", d.Id(), err)
	}
	if statusCode >= http.StatusBadRequest || filesHasError(respBody) {
		if filesIsNotFound(respBody) {
			return nil
		}
		return diag.Errorf("error while deleting file server mount target %q: %s", d.Id(), filesErrorMessage(respBody, statusCode))
	}
	return nil
}

func buildFileServerMountTargetPayload(d *schema.ResourceData) map[string]interface{} {
	path := d.Get("path").(string)
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	payload := map[string]interface{}{
		"name":      d.Get("name").(string),
		"maxSizeGB": d.Get("max_size_gb").(int),
		"protocol":  "NFS",
		"type":      "GENERAL",
		"nfsProperties": map[string]interface{}{
			"accessType":         "NO_ACCESS",
			"authenticationType": "SYSTEM",
			"squashType":         "ROOT_SQUASH",
			"anonymousIdentifier": map[string]interface{}{
				"uid": -2,
				"gid": -2,
			},
		},
	}
	if d.Get("protocol").(string) == "SMB" {
		delete(payload, "nfsProperties")
		payload["protocol"] = "SMB"
		payload["smbProperties"] = buildSmbMountTargetProperties(d)
	}
	if path != "/"+d.Get("name").(string) {
		payload["path"] = path
	}
	return payload
}

func fileServerMountTargetsPath(d *schema.ResourceData) string {
	return fmt.Sprintf(filesMountTargetsPathTemplate, d.Get("file_server_ext_id").(string))
}

func getFileServerMountTargetByPath(apiClient *filesClient.ApiClient, fileServerExtID string, path string) (map[string]interface{}, bool, error) {
	respBody, statusCode, err := filesRequest(apiClient, http.MethodGet, fmt.Sprintf(filesMountTargetsPathTemplate, fileServerExtID), nil)
	if err != nil {
		return nil, false, err
	}
	if statusCode >= http.StatusBadRequest || filesHasError(respBody) {
		if filesIsNotFound(respBody) {
			return nil, true, nil
		}
		return nil, false, fmt.Errorf("%s", filesErrorMessage(respBody, statusCode))
	}

	normalized := strings.TrimSuffix(path, "/")
	data, _ := respBody["data"].([]interface{})
	for _, item := range data {
		target, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		if strings.TrimSuffix(stringValue(target["path"]), "/") == normalized {
			return target, false, nil
		}
	}
	return nil, true, nil
}

func fileServerMountTargetByPathRefreshFunc(apiClient *filesClient.ApiClient, fileServerExtID string, name string, path string) resource.StateRefreshFunc {
	return func() (interface{}, string, error) {
		item, notFound, err := getFileServerMountTargetByPath(apiClient, fileServerExtID, path)
		if err != nil {
			return nil, "", err
		}
		if notFound || item == nil {
			return map[string]interface{}{}, "NOT_FOUND", nil
		}
		if stringValue(item["name"]) != name {
			return map[string]interface{}{}, "NOT_FOUND", nil
		}
		return item, "FOUND", nil
	}
}

func getFileServerMountTargetByID(apiClient *filesClient.ApiClient, fileServerExtID string, extID string) (map[string]interface{}, string, bool, error) {
	respBody, statusCode, headers, err := filesRequestWithHeaders(apiClient, http.MethodGet, fmt.Sprintf(filesMountTargetsPathTemplate, fileServerExtID)+"/"+extID, nil, nil)
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
		return nil, "", false, fmt.Errorf("unexpected file server mount target get response shape")
	}
	return data, headers.Get("Etag"), false, nil
}

func smbMountTargetSchema() *schema.Schema {
	return &schema.Schema{Type: schema.TypeList, Optional: true, MaxItems: 1, Elem: &schema.Resource{Schema: map[string]*schema.Schema{
		"is_smb3_encryption_enabled":          {Type: schema.TypeBool, Optional: true, Default: false},
		"is_access_based_enumeration_enabled": {Type: schema.TypeBool, Optional: true, Default: false},
		"share_acl": {Type: schema.TypeSet, Required: true, MinItems: 1, Elem: &schema.Resource{Schema: map[string]*schema.Schema{
			"user_or_group_name": {Type: schema.TypeString, Required: true, ValidateFunc: validation.StringIsNotWhiteSpace},
			"access_type":        {Type: schema.TypeString, Required: true, ValidateFunc: validation.StringInSlice([]string{"READ", "CHANGE", "FULL_CONTROL"}, false)},
			"permission_type":    {Type: schema.TypeString, Optional: true, Default: "ALLOW", ValidateFunc: validation.StringInSlice([]string{"ALLOW", "DENY"}, false)},
		}}},
	}}}
}

func validateMountTargetProtocol(_ context.Context, d *schema.ResourceDiff, _ interface{}) error {
	hasSMB := len(d.Get("smb_properties").([]interface{})) > 0
	if (d.Get("protocol").(string) == "SMB") != hasSMB {
		return fmt.Errorf("smb_properties with explicit share_acl is required for SMB and must be omitted for NFS")
	}
	return nil
}

func buildSmbMountTargetProperties(d *schema.ResourceData) map[string]interface{} {
	acl := []interface{}{}
	blocks := d.Get("smb_properties").([]interface{})
	if len(blocks) > 0 && blocks[0] != nil {
		for _, raw := range blocks[0].(map[string]interface{})["share_acl"].(*schema.Set).List() {
			ace := raw.(map[string]interface{})
			acl = append(acl, map[string]interface{}{"userOrGroupName": ace["user_or_group_name"], "accessType": ace["access_type"], "permissionType": ace["permission_type"]})
		}
	}
	properties := map[string]interface{}{"shareACL": acl}
	if len(blocks) > 0 && blocks[0] != nil {
		block := blocks[0].(map[string]interface{})
		properties["isSmb3EncryptionEnabled"] = block["is_smb3_encryption_enabled"]
		properties["isAccessBasedEnumerationEnabled"] = block["is_access_based_enumeration_enabled"]
	}
	return properties
}

func resourceNutanixFileServerMountTargetV2Update(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	apiClient := meta.(*conns.Client).FilesAPI.APIClientInstance
	if apiClient == nil {
		return diag.Errorf("files api client is not initialized")
	}
	item, etag, notFound, err := getFileServerMountTargetByID(apiClient, d.Get("file_server_ext_id").(string), d.Id())
	if err != nil {
		return diag.FromErr(err)
	}
	if notFound {
		return diag.Errorf("mount target disappeared during update")
	}
	properties, _ := item["smbProperties"].(map[string]interface{})
	if properties == nil {
		properties = map[string]interface{}{}
	}
	for key, value := range buildSmbMountTargetProperties(d) {
		properties[key] = value
	}
	payload := map[string]interface{}{"name": item["name"], "type": item["type"], "protocol": item["protocol"], "maxSizeGB": item["maxSizeGB"], "smbProperties": properties}
	body, code, _, err := filesRequestWithHeaders(apiClient, http.MethodPut, fileServerMountTargetsPath(d)+"/"+d.Id(), payload, map[string]string{"If-Match": etag})
	if err != nil {
		return diag.FromErr(err)
	}
	if code >= http.StatusBadRequest || filesHasError(body) {
		return diag.Errorf("error updating SMB share permissions: %s", filesErrorMessage(body, code))
	}
	if taskID := responseTaskExtID(body); taskID != "" {
		if err := waitForFilesDirectoryServiceTask(ctx, apiClient, d.Get("file_server_ext_id").(string), taskID, d.Timeout(schema.TimeoutUpdate), nil); err != nil {
			return diag.FromErr(err)
		}
	}
	return resourceNutanixFileServerMountTargetV2Read(ctx, d, meta)
}

func mountTargetSlice(raw interface{}) []interface{} { values, _ := raw.([]interface{}); return values }
