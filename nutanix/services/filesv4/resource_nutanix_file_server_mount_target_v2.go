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
		DeleteContext: resourceNutanixFileServerMountTargetV2Delete,
		Importer: &schema.ResourceImporter{
			StateContext: schema.ImportStatePassthroughContext,
		},
		Timeouts: &schema.ResourceTimeout{
			Create: schema.DefaultTimeout(10 * time.Minute),
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
				Type:     schema.TypeString,
				Computed: true,
			},
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
