package objectstoresv2

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
)

func TestBucketReplicationHasStaleEndpointConflict(t *testing.T) {
	tests := []struct {
		name        string
		diagnostics diag.Diagnostics
		expected    bool
	}{
		{
			name:        "legacy duplicate target message",
			diagnostics: diag.Errorf("Unable to register duplicate endpoint with same target"),
			expected:    true,
		},
		{
			name:        "current duplicate endpoint name message",
			diagnostics: diag.Errorf("Failed to create/update replication rule. Registering endpoint failed with err: Failed to create new endpoint, an endpoint with the same name already exists: 9"),
			expected:    true,
		},
		{
			name: "duplicate endpoint message in detail",
			diagnostics: diag.Diagnostics{
				{
					Severity: diag.Error,
					Summary:  "replication endpoint registration failed",
					Detail:   "an endpoint with the same name already exists: 9",
				},
			},
			expected: true,
		},
		{
			name:        "unrelated replication failure",
			diagnostics: diag.Errorf("target object store is unavailable"),
			expected:    false,
		},
		{
			name: "warning is not a conflict",
			diagnostics: diag.Diagnostics{
				{
					Severity: diag.Warning,
					Summary:  "an endpoint with the same name already exists",
				},
			},
			expected: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			actual := bucketReplicationHasStaleEndpointConflict(test.diagnostics)
			if actual != test.expected {
				t.Fatalf("bucketReplicationHasStaleEndpointConflict() = %t, expected %t", actual, test.expected)
			}
		})
	}
}

func TestBucketReplicationJSONEquivalentAllowsCanonicalTargetFQDN(t *testing.T) {
	actual := `{
		"api_version":"3.0",
		"spec":{
			"target_oss_uuid":"target-uuid",
			"target_oss_fqdn":"dev-objects-2.prism-central.cluster.local",
			"target_endpoint_list":["10.128.26.66","10.128.26.67"],
			"status":"Enabled"
		}
	}`
	expected := `{
		"api_version":"3.0",
		"spec":{
			"target_oss_uuid":"target-uuid",
			"target_oss_fqdn":"objects-dc2.dev.se-bod.stegra.tech",
			"target_endpoint_list":["10.128.26.66","10.128.26.67"],
			"status":"Enabled"
		}
	}`

	if !bucketReplicationJSONEquivalent(actual, expected) {
		t.Fatal("expected canonical and configured target FQDNs to be equivalent for the same target")
	}
}

func TestBucketReplicationJSONEquivalentRejectsDifferentTargetUUID(t *testing.T) {
	actual := `{"spec":{"target_oss_uuid":"actual-uuid","target_oss_fqdn":"internal","target_endpoint_list":["10.0.0.1"]}}`
	expected := `{"spec":{"target_oss_uuid":"expected-uuid","target_oss_fqdn":"external","target_endpoint_list":["10.0.0.1"]}}`

	if bucketReplicationJSONEquivalent(actual, expected) {
		t.Fatal("expected different target UUIDs not to be equivalent")
	}
}

func TestBucketReplicationJSONEquivalentRejectsDifferentTargetEndpoints(t *testing.T) {
	actual := `{"spec":{"target_oss_uuid":"target-uuid","target_oss_fqdn":"internal","target_endpoint_list":["10.0.0.1"]}}`
	expected := `{"spec":{"target_oss_uuid":"target-uuid","target_oss_fqdn":"external","target_endpoint_list":["10.0.0.2"]}}`

	if bucketReplicationJSONEquivalent(actual, expected) {
		t.Fatal("expected different target endpoint lists not to be equivalent")
	}
}

func TestBucketReplicationJSONEquivalentStillDetectsOtherChanges(t *testing.T) {
	actual := `{"spec":{"target_oss_uuid":"target-uuid","target_oss_fqdn":"internal","target_endpoint_list":["10.0.0.1"],"status":"Enabled"}}`
	expected := `{"spec":{"target_oss_uuid":"target-uuid","target_oss_fqdn":"external","target_endpoint_list":["10.0.0.1"],"status":"Disabled"}}`

	if bucketReplicationJSONEquivalent(actual, expected) {
		t.Fatal("expected non-FQDN replication changes not to be equivalent")
	}
}
