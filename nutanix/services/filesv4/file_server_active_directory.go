package filesv4

import (
	"context"
	"encoding/base64"
	"fmt"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	filesClient "github.com/terraform-providers/terraform-provider-nutanix/nutanix/sdks/v4/files"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"
)

// These options describe join/leave requests; Files need not return them on GET.
var activeDirectoryInputFields = map[string]string{
	"username": "", "password": "",
	"organization_unit":                   "organizationUnit",
	"preferred_domain_controller":         "preferredDomainController",
	"should_overwrite_user_account":       "shouldOverwriteUserAccount",
	"should_add_user_as_fs_admin":         "shouldAddUserAsFsAdmin",
	"should_use_same_credentials_for_dns": "shouldUseSameCredentialsForDns",
	"should_force_leave_domain":           "shouldForceLeaveDomain",
	"should_validate_ad_credential":       "shouldValidateAdCredential",
}

func activeDirectorySchema() *schema.Schema {
	fields := map[string]*schema.Schema{
		"domain_name":                 {Type: schema.TypeString, Required: true, ValidateFunc: validation.StringIsNotWhiteSpace},
		"protocol_type":               {Type: schema.TypeString, Optional: true, Default: "SMB", ValidateFunc: validation.StringInSlice([]string{"NONE", "NFS", "SMB", "SMB_NFS"}, false)},
		"username":                    {Type: schema.TypeString, Required: true, ValidateFunc: validation.StringIsNotWhiteSpace},
		"password":                    {Type: schema.TypeString, Required: true, Sensitive: true, ValidateFunc: validation.StringIsNotEmpty},
		"organization_unit":           {Type: schema.TypeString, Optional: true, ValidateFunc: validateFilesOrganizationUnit},
		"preferred_domain_controller": {Type: schema.TypeString, Optional: true, ValidateFunc: validatePreferredDomainController},
		"is_rfc2307_enabled":          {Type: schema.TypeBool, Optional: true, Default: false},
	}
	for field := range activeDirectoryInputFields {
		if _, exists := fields[field]; !exists {
			fields[field] = &schema.Schema{Type: schema.TypeBool, Optional: true, Default: false}
		}
	}
	return &schema.Schema{Type: schema.TypeList, Optional: true, MaxItems: 1, Elem: &schema.Resource{Schema: fields}}
}

func validatePreferredDomainController(v interface{}, key string) ([]string, []error) {
	value := v.(string)
	if value == "" {
		return nil, nil
	}
	_, ipErr := netip.ParseAddr(value)
	if ipErr == nil || !strings.Contains(strings.TrimSuffix(value, "."), ".") || strings.ContainsAny(value, " /\\:\t\r\n") {
		return nil, []error{fmt.Errorf("%s must be a domain controller FQDN, not an IP address or short name", key)}
	}
	return nil, nil
}

func configuredActiveDirectory(values []interface{}) map[string]interface{} {
	if len(values) == 0 || values[0] == nil {
		return nil
	}
	root, ok := values[0].(map[string]interface{})
	if !ok {
		return nil
	}
	blocks, ok := root["ad_domain"].([]interface{})
	if !ok || len(blocks) == 0 || blocks[0] == nil {
		return nil
	}
	ad, _ := blocks[0].(map[string]interface{})
	return ad
}

func hasConfiguredActiveDirectory(values []interface{}) bool {
	return configuredActiveDirectory(values) != nil
}

func hasActiveDirectory(current map[string]interface{}) bool {
	ad, ok := current["adDomain"].(map[string]interface{})
	return ok && localDomainProtocol(ad["protocolType"]) != "NONE" && stringValue(ad["domainName"]) != ""
}

func addActiveDirectoryPayload(root, payload map[string]interface{}) error {
	ad := configuredActiveDirectory([]interface{}{root})
	if ad == nil {
		return nil
	}
	protocol := stringValue(ad["protocol_type"])
	if protocol != "NONE" {
		for _, other := range []string{"localDomain", "ldapDomain"} {
			if protocolsOverlap(protocol, localDomainProtocol(payload[other])) {
				return fmt.Errorf("ad_domain.protocol_type overlaps with %s; each protocol must use only one directory service", other)
			}
		}
	}
	result := map[string]interface{}{
		"$objectType": "files.v4.config.ActiveDirectoryConfig",
		"domainName":  ad["domain_name"], "protocolType": protocol,
		"isRfc2307Enabled": ad["is_rfc2307_enabled"],
		"credential":       map[string]interface{}{"username": ad["username"], "password": ad["password"]},
	}
	for field, apiField := range activeDirectoryInputFields {
		if apiField == "" {
			continue
		}
		if value, exists := ad[field]; exists {
			result[apiField] = value
		}
	}
	payload["adDomain"] = result
	return nil
}

func protocolsOverlap(a, b string) bool {
	if a == "" || b == "" || a == "NONE" || b == "NONE" {
		return false
	}
	return a == b || a == "SMB_NFS" || b == "SMB_NFS"
}

func validateActiveDirectoryTransition(current, payload map[string]interface{}) error {
	if !hasActiveDirectory(current) {
		return nil
	}
	old := current["adDomain"].(map[string]interface{})
	next, ok := payload["adDomain"].(map[string]interface{})
	if !ok {
		return fmt.Errorf("explicit AD leave configuration is required before removing ad_domain")
	}
	if !strings.EqualFold(stringValue(old["domainName"]), stringValue(next["domainName"])) {
		return fmt.Errorf("changing AD domains requires leaving the current domain first using protocol_type = NONE")
	}
	return nil
}

func flattenActiveDirectory(item, result map[string]interface{}) {
	if !hasActiveDirectory(item) {
		return
	}
	raw := item["adDomain"].(map[string]interface{})
	ad := map[string]interface{}{
		"domain_name":   raw["domainName"],
		"protocol_type": localDomainProtocol(raw["protocolType"]),
	}
	if value, ok := raw["isRfc2307Enabled"]; ok {
		ad["is_rfc2307_enabled"] = value
	}
	for _, field := range []string{"organization_unit", "preferred_domain_controller"} {
		if value, ok := raw[activeDirectoryInputFields[field]]; ok {
			ad[field] = value
		}
	}
	result["ad_domain"] = []map[string]interface{}{ad}
}

// Never trust credential values from GET (which can be absent or redacted).
func preserveActiveDirectoryInputs(flattened []map[string]interface{}, previous []interface{}) {
	if len(flattened) == 0 {
		return
	}
	prior := configuredActiveDirectory(previous)
	if prior == nil {
		return
	}
	blocks, ok := flattened[0]["ad_domain"].([]map[string]interface{})
	if !ok || len(blocks) == 0 {
		// A successful explicit leave has no remote AD block. Retain the requested
		// NONE sentinel until the user removes it in the following apply.
		if prior["protocol_type"] == "NONE" {
			flattened[0]["ad_domain"] = []map[string]interface{}{prior}
		}
		return
	}
	for field := range activeDirectoryInputFields {
		if _, returned := blocks[0][field]; !returned {
			if value, exists := prior[field]; exists {
				blocks[0][field] = value
			}
		}
	}
}

func validateFileServerDirectoryServiceDiff(ctx context.Context, diff *schema.ResourceDiff, meta interface{}) error {
	if !diff.NewValueKnown("directory_service") {
		return nil
	}
	_, err := buildDirectoryServicePayload(diff.Get("directory_service").([]interface{}))
	return err
}

// Files proxies FSVM tasks through a scoped endpoint. The encoded task prefix
// includes both the task service (for example Ergon) and the file-server UUID;
// querying Prism Central's task database cannot find these FSVM-local tasks.
func filesDirectoryServiceTaskPath(fileServerID, taskID string) (string, error) {
	parts := strings.SplitN(taskID, ":", 2)
	if len(parts) != 2 || parts[1] == "" {
		return "", fmt.Errorf("invalid Files task identifier")
	}
	service, err := base64.StdEncoding.DecodeString(parts[0])
	if err != nil {
		return "", fmt.Errorf("invalid Files task service encoding")
	}
	scopedService := string(service)
	if !strings.Contains(scopedService, ":") {
		scopedService += ":" + fileServerID
	}
	scopedID := base64.StdEncoding.EncodeToString([]byte(scopedService)) + ":" + parts[1]
	return fmt.Sprintf("/api/files/v4.0/config/file-servers/%s/tasks/%s", url.PathEscape(fileServerID), url.PathEscape(scopedID)), nil
}

func waitForFilesDirectoryServiceTask(ctx context.Context, client *filesClient.ApiClient, fileServerID, taskID string, timeout time.Duration, values []interface{}) error {
	path, err := filesDirectoryServiceTaskPath(fileServerID, taskID)
	if err != nil {
		return err
	}
	wait := &resource.StateChangeConf{
		Pending: []string{"QUEUED", "RUNNING", "PENDING"}, Target: []string{"SUCCEEDED"},
		Timeout: timeout, MinTimeout: 2 * time.Second,
		Refresh: func() (interface{}, string, error) {
			response, statusCode, err := filesRequest(client, http.MethodGet, path, nil)
			if err != nil {
				return nil, "", err
			}
			if statusCode >= 400 || filesHasError(response) {
				return nil, "", directoryServiceError(filesErrorMessage(response, statusCode), values)
			}
			task, _ := response["data"].(map[string]interface{})
			status := stringValue(task["status"])
			switch status {
			case "FAILED", "CANCELED", "CANCELLED":
				message := stringValue(task["legacyErrorMessage"])
				if message == "" {
					message = filesErrorMessage(task, statusCode)
				}
				return task, status, directoryServiceError(message, values)
			case "":
				return nil, "", fmt.Errorf("Files directory-service task response did not contain a status")
			}
			return task, status, nil
		},
	}
	_, err = wait.WaitForStateContext(ctx)
	return err
}

func directoryServiceError(message string, values []interface{}) error {
	if ad := configuredActiveDirectory(values); ad != nil {
		if password := stringValue(ad["password"]); password != "" {
			message = strings.ReplaceAll(message, password, "[REDACTED]")
		}
	}
	return fmt.Errorf("Files directory-service operation failed: %s", message)
}

func validateFilesOrganizationUnit(value interface{}, key string) ([]string, []error) {
	text := strings.ToUpper(value.(string))
	if strings.Contains(text, "OU=") || strings.Contains(text, "DC=") || strings.Contains(text, "CN=") {
		return nil, []error{fmt.Errorf("%s must be a relative OU path such as Files or Parent/Files, not an LDAP distinguished name", key)}
	}
	return nil, nil
}
