package filesv4

import (
	"regexp"
	"sort"
	"strings"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"
)

var nfsClientPattern = regexp.MustCompile(`^[^,[:space:]]+$`)

func nfsMountTargetSchema() *schema.Schema {
	accessType := validation.StringInSlice([]string{"NO_ACCESS", "READ_ONLY", "READ_WRITE"}, false)
	squashType := validation.StringInSlice([]string{"NONE", "ROOT_SQUASH", "ALL_SQUASH"}, false)
	return &schema.Schema{Type: schema.TypeList, Optional: true, Computed: true, MaxItems: 1, ConflictsWith: []string{"smb_properties"}, Elem: &schema.Resource{Schema: map[string]*schema.Schema{
		"access_type":         {Type: schema.TypeString, Optional: true, Default: "NO_ACCESS", ValidateFunc: accessType},
		"authentication_type": {Type: schema.TypeString, Optional: true, Default: "SYSTEM", ValidateFunc: validation.StringInSlice([]string{"NONE", "SYSTEM", "KERBEROS"}, false)},
		"squash_type":         {Type: schema.TypeString, Optional: true, Default: "ROOT_SQUASH", ValidateFunc: squashType},
		"anonymous_uid":       {Type: schema.TypeInt, Optional: true, Default: -2, ValidateFunc: validation.IntAtLeast(-2)},
		"anonymous_gid":       {Type: schema.TypeInt, Optional: true, Default: -2, ValidateFunc: validation.IntAtLeast(-2)},
		"client_exception": {Type: schema.TypeList, Optional: true, Elem: &schema.Resource{Schema: map[string]*schema.Schema{
			"access_type": {Type: schema.TypeString, Required: true, ValidateFunc: accessType},
			"clients": {Type: schema.TypeSet, Required: true, MinItems: 1, Elem: &schema.Schema{
				Type: schema.TypeString, ValidateFunc: validation.StringMatch(nfsClientPattern, "must be a single non-empty client without commas or whitespace"),
			}},
			"squash_type": {Type: schema.TypeString, Optional: true, ValidateFunc: squashType},
		}}},
	}}}
}

func buildNfsMountTargetProperties(d *schema.ResourceData) map[string]interface{} {
	blocks := d.Get("nfs_properties").([]interface{})
	if len(blocks) == 0 || blocks[0] == nil {
		return nil
	}
	block := blocks[0].(map[string]interface{})
	// Send an explicit empty array to clear any previously configured exceptions.
	exceptions := make([]interface{}, 0)
	for _, raw := range block["client_exception"].([]interface{}) {
		rule := raw.(map[string]interface{})
		clients := make([]string, 0)
		for _, client := range rule["clients"].(*schema.Set).List() {
			clients = append(clients, client.(string))
		}
		sort.Strings(clients)
		exception := map[string]interface{}{"accessType": rule["access_type"], "clients": strings.Join(clients, ",")}
		if squash := rule["squash_type"].(string); squash != "" {
			exception["squashType"] = squash
		}
		exceptions = append(exceptions, exception)
	}
	return map[string]interface{}{
		"accessType":          block["access_type"],
		"authenticationType":  block["authentication_type"],
		"squashType":          block["squash_type"],
		"anonymousIdentifier": map[string]interface{}{"uid": block["anonymous_uid"], "gid": block["anonymous_gid"]},
		"clientExceptions":    exceptions,
	}
}

func flattenNfsMountTargetProperties(properties map[string]interface{}) []interface{} {
	block := map[string]interface{}{
		"access_type": properties["accessType"], "authentication_type": properties["authenticationType"], "squash_type": properties["squashType"],
		"anonymous_uid": -2, "anonymous_gid": -2,
	}
	if anonymous, ok := properties["anonymousIdentifier"].(map[string]interface{}); ok {
		for _, key := range []string{"uid", "gid"} {
			if value := anonymous[key]; value != nil {
				block["anonymous_"+key] = intValue(value)
			}
		}
	}
	exceptions := make([]interface{}, 0)
	for _, raw := range mountTargetSlice(properties["clientExceptions"]) {
		rule, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		clients := make([]string, 0)
		for _, client := range strings.Split(stringValue(rule["clients"]), ",") {
			if client = strings.TrimSpace(client); client != "" {
				clients = append(clients, client)
			}
		}
		exceptions = append(exceptions, map[string]interface{}{
			"access_type": rule["accessType"], "clients": clients, "squash_type": stringValue(rule["squashType"]),
		})
	}
	block["client_exception"] = exceptions
	return []interface{}{block}
}
