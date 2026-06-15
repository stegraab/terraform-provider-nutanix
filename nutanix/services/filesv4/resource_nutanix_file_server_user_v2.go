package filesv4

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"
	conns "github.com/terraform-providers/terraform-provider-nutanix/nutanix"
	filesClient "github.com/terraform-providers/terraform-provider-nutanix/nutanix/sdks/v4/files"
)

const filesUsersPathTemplate = "/api/files/v4.0/config/file-servers/%s/users"

func ResourceNutanixFileServerUserV2() *schema.Resource {
	return &schema.Resource{
		CreateContext: resourceNutanixFileServerUserV2Create,
		ReadContext:   resourceNutanixFileServerUserV2Read,
		UpdateContext: resourceNutanixFileServerUserV2Update,
		DeleteContext: resourceNutanixFileServerUserV2Delete,
		Importer: &schema.ResourceImporter{
			StateContext: schema.ImportStatePassthroughContext,
		},
		Schema: map[string]*schema.Schema{
			"file_server_ext_id": {
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
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
			"password": {
				Type:      schema.TypeString,
				Required:  true,
				Sensitive: true,
			},
			"roles": {
				Type:     schema.TypeSet,
				Optional: true,
				MinItems: 1,
				Elem: &schema.Schema{
					Type:         schema.TypeString,
					ValidateFunc: validation.StringInSlice([]string{"USER_ADMIN"}, false),
				},
				Set: schema.HashString,
			},
		},
	}
}

func resourceNutanixFileServerUserV2Create(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	apiClient := meta.(*conns.Client).FilesAPI.APIClientInstance
	if apiClient == nil {
		return diag.Errorf("files api client is not initialized")
	}

	existing, _, err := getFileServerUserByName(apiClient, d.Get("file_server_ext_id").(string), d.Get("name").(string))
	if err != nil {
		return diag.FromErr(err)
	}
	if existing != nil {
		d.SetId(stringValue(existing["extId"]))
		_ = d.Set("ext_id", d.Id())
		return resourceNutanixFileServerUserV2Update(ctx, d, meta)
	}

	respBody, statusCode, err := filesRequest(apiClient, http.MethodPost, fileServerUsersPath(d), buildFileServerUserPayload(d))
	if err != nil {
		return diag.FromErr(err)
	}
	if statusCode >= http.StatusBadRequest || filesHasError(respBody) {
		return diag.Errorf("%s", filesErrorMessage(respBody, statusCode))
	}

	data, ok := respBody["data"].(map[string]interface{})
	if !ok {
		return diag.Errorf("unexpected file server user create response shape")
	}
	extID := stringValue(data["extId"])
	if extID == "" {
		return diag.Errorf("file server user create response did not include extId")
	}
	d.SetId(extID)
	_ = d.Set("ext_id", extID)
	return resourceNutanixFileServerUserV2Read(ctx, d, meta)
}

func resourceNutanixFileServerUserV2Read(_ context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	apiClient := meta.(*conns.Client).FilesAPI.APIClientInstance
	user, _, notFound, err := getFileServerUserByID(apiClient, d.Get("file_server_ext_id").(string), d.Id())
	if err != nil {
		return diag.FromErr(err)
	}
	if notFound {
		d.SetId("")
		return nil
	}

	_ = d.Set("ext_id", d.Id())
	_ = d.Set("name", stringValue(user["name"]))
	if roles, ok := user["roles"].([]interface{}); ok {
		_ = d.Set("roles", flattenStringSet(roles))
	}
	return nil
}

func resourceNutanixFileServerUserV2Update(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	apiClient := meta.(*conns.Client).FilesAPI.APIClientInstance
	_, etag, notFound, err := getFileServerUserByID(apiClient, d.Get("file_server_ext_id").(string), d.Id())
	if err != nil {
		return diag.FromErr(err)
	}
	if notFound {
		d.SetId("")
		return nil
	}

	headers := map[string]string{}
	if etag != "" {
		headers["If-Match"] = etag
	}

	respBody, statusCode, _, err := filesRequestWithHeaders(apiClient, http.MethodPut, fileServerUsersPath(d)+"/"+d.Id(), buildFileServerUserPayload(d), headers)
	if err != nil {
		return diag.FromErr(err)
	}
	if statusCode >= http.StatusBadRequest || filesHasError(respBody) {
		return diag.Errorf("%s", filesErrorMessage(respBody, statusCode))
	}
	return resourceNutanixFileServerUserV2Read(ctx, d, meta)
}

func resourceNutanixFileServerUserV2Delete(_ context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	apiClient := meta.(*conns.Client).FilesAPI.APIClientInstance
	_, etag, notFound, err := getFileServerUserByID(apiClient, d.Get("file_server_ext_id").(string), d.Id())
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

	respBody, statusCode, _, err := filesRequestWithHeaders(apiClient, http.MethodDelete, fileServerUsersPath(d)+"/"+d.Id(), nil, headers)
	if err != nil {
		return diag.FromErr(err)
	}
	if statusCode >= http.StatusBadRequest || filesHasError(respBody) {
		return diag.Errorf("%s", filesErrorMessage(respBody, statusCode))
	}
	return nil
}

func buildFileServerUserPayload(d *schema.ResourceData) map[string]interface{} {
	return map[string]interface{}{
		"name":     d.Get("name").(string),
		"password": d.Get("password").(string),
		"roles":    expandFileServerUserRoles(d),
	}
}

func expandFileServerUserRoles(d *schema.ResourceData) []string {
	raw := d.Get("roles").(*schema.Set).List()
	if len(raw) == 0 {
		return []string{"USER_ADMIN"}
	}
	roles := make([]string, 0, len(raw))
	for _, role := range raw {
		roles = append(roles, role.(string))
	}
	return roles
}

func flattenStringSet(raw []interface{}) []string {
	result := make([]string, 0, len(raw))
	for _, v := range raw {
		if s := stringValue(v); s != "" {
			result = append(result, s)
		}
	}
	return result
}

func fileServerUsersPath(d *schema.ResourceData) string {
	return fmt.Sprintf(filesUsersPathTemplate, d.Get("file_server_ext_id").(string))
}

func getFileServerUserByName(apiClient *filesClient.ApiClient, fileServerExtID string, name string) (map[string]interface{}, bool, error) {
	respBody, statusCode, err := filesRequest(apiClient, http.MethodGet, fmt.Sprintf(filesUsersPathTemplate, fileServerExtID), nil)
	if err != nil {
		return nil, false, err
	}
	if statusCode >= http.StatusBadRequest || filesHasError(respBody) {
		if filesIsNotFound(respBody) {
			return nil, true, nil
		}
		return nil, false, fmt.Errorf("%s", filesErrorMessage(respBody, statusCode))
	}

	data, _ := respBody["data"].([]interface{})
	for _, item := range data {
		user, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		if strings.EqualFold(stringValue(user["name"]), name) {
			return user, false, nil
		}
	}
	return nil, true, nil
}

func getFileServerUserByID(apiClient *filesClient.ApiClient, fileServerExtID string, extID string) (map[string]interface{}, string, bool, error) {
	respBody, statusCode, headers, err := filesRequestWithHeaders(apiClient, http.MethodGet, fmt.Sprintf(filesUsersPathTemplate, fileServerExtID)+"/"+extID, nil, nil)
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
		return nil, "", false, fmt.Errorf("unexpected file server user get response shape")
	}
	return data, headers.Get("Etag"), false, nil
}
