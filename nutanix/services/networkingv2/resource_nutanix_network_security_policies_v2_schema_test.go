package networkingv2

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func TestNetworkSecurityPolicyAllowSpecsAreConfigurationOnly(t *testing.T) {
	resourceSchema := ResourceNutanixNetworkSecurityPolicyV2().Schema
	ruleSchema := resourceSchema["rules"].Elem.(*schema.Resource).Schema
	specSchema := ruleSchema["spec"].Elem.(*schema.Resource).Schema
	applicationRuleSchema := specSchema["application_rule_spec"].Elem.(*schema.Resource).Schema

	for _, name := range []string{"src_allow_spec", "dest_allow_spec"} {
		field := applicationRuleSchema[name]
		if !field.Optional {
			t.Errorf("%s must remain optional", name)
		}
		if field.Computed {
			t.Errorf("%s must not be computed because prior positional list state can leak into a different rule", name)
		}
	}
}
