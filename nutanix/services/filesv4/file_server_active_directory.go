package filesv4

import (
	"context"
	"fmt"
	"net/netip"
	"strings"

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
		"organization_unit":           {Type: schema.TypeString, Optional: true},
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
