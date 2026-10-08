package networkingv2

import (
	"testing"

	import1 "github.com/nutanix/ntnx-api-golang-clients/microseg-go-client/v4/models/microseg/v4/config"
	"github.com/terraform-providers/terraform-provider-nutanix/nutanix/common"
	"github.com/terraform-providers/terraform-provider-nutanix/utils"
)

func TestSameNetworkSecurityPolicyRuleIdentitiesDetectsMissingRule(t *testing.T) {
	applicationType := common.ExpandEnum[import1.RuleType]("APPLICATION")
	local := []import1.NetworkSecurityPolicyRule{
		{Description: utils.StringPtr("Allow Prism Central IAM communication"), Type: applicationType},
		{Description: utils.StringPtr("Allow Prism Central Objects proxy communication"), Type: applicationType},
	}
	remote := []import1.NetworkSecurityPolicyRule{
		{Description: utils.StringPtr("Allow Prism Central IAM communication"), Type: applicationType},
		{Description: utils.StringPtr("Another application rule"), Type: applicationType},
	}

	if sameNetworkSecurityPolicyRuleIdentities(local, remote) {
		t.Fatal("expected a missing Objects proxy rule to be detected")
	}
}

func TestManagedNetworkSecurityPolicyRulesDropsOnlyPrismGeneratedDefaults(t *testing.T) {
	applicationType := common.ExpandEnum[import1.RuleType]("APPLICATION")
	rules := []import1.NetworkSecurityPolicyRule{
		{Description: utils.StringPtr("Allow Prism Central IAM communication"), Type: applicationType},
		{Description: utils.StringPtr("Inbound default rule"), Type: applicationType},
		{Description: utils.StringPtr("Outbound default rule"), Type: applicationType},
	}

	managed := managedNetworkSecurityPolicyRules(rules)
	if len(managed) != 1 || utils.StringValue(managed[0].Description) != "Allow Prism Central IAM communication" {
		t.Fatalf("expected only Prism-generated defaults to be removed, got %#v", managed)
	}
}

func TestExpandNetworkSecurityPolicyRuleSpecUsesDeclaredRuleType(t *testing.T) {
	specWithComputedPlaceholder := []interface{}{
		map[string]interface{}{
			"application_rule_spec": []interface{}{map[string]interface{}{
				"secured_group_category_references": []interface{}{},
			}},
			"intra_entity_group_rule_spec": []interface{}{map[string]interface{}{
				"secured_group_category_references": []interface{}{},
			}},
		},
	}

	application := expandOneOfNetworkSecurityPolicyRuleSpec(nil, "rules.0.spec", specWithComputedPlaceholder, "APPLICATION")
	if _, ok := application.GetValue().(import1.ApplicationRuleSpec); !ok {
		t.Fatalf("expected APPLICATION rule to expand to ApplicationRuleSpec, got %T", application.GetValue())
	}

	intraGroup := expandOneOfNetworkSecurityPolicyRuleSpec(nil, "rules.0.spec", specWithComputedPlaceholder, "INTRA_GROUP")
	if _, ok := intraGroup.GetValue().(import1.IntraEntityGroupRuleSpec); !ok {
		t.Fatalf("expected INTRA_GROUP rule to expand to IntraEntityGroupRuleSpec, got %T", intraGroup.GetValue())
	}
}

func TestExpandNetworkSecurityPolicyRuleSpecKeepsFlexAndApplicationSeparate(t *testing.T) {
	spec := []interface{}{map[string]interface{}{
		"application_rule_spec": []interface{}{map[string]interface{}{"src_allow_spec": "ALL"}},
		"flex_rule_spec":        []interface{}{map[string]interface{}{"action": "DENY", "direction": "INBOUND"}},
	}}
	application := expandOneOfNetworkSecurityPolicyRuleSpec(nil, "rules.0.spec", spec, "APPLICATION")
	if _, ok := application.GetValue().(import1.ApplicationRuleSpec); !ok {
		t.Fatalf("computed FLEX placeholder replaced APPLICATION rule: %T", application.GetValue())
	}
	flex := expandOneOfNetworkSecurityPolicyRuleSpec(nil, "rules.0.spec", spec, "FLEX")
	value, ok := flex.GetValue().(import1.FlexRuleSpec)
	if !ok || common.FlattenPtrEnum(value.Action) != "DENY" {
		t.Fatalf("FLEX action was not preserved: %#v", flex.GetValue())
	}
}
