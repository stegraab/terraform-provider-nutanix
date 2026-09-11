package iamv2

import "testing"

func TestShouldPreserveExistingKeySecret(t *testing.T) {
	tests := []struct {
		name     string
		read     interface{}
		existing interface{}
		want     bool
	}{
		{
			name:     "preserves API key omitted by read",
			read:     apiKeyDetails(""),
			existing: apiKeyDetails("api-secret"),
			want:     true,
		},
		{
			name:     "uses API key returned by read",
			read:     apiKeyDetails("replacement"),
			existing: apiKeyDetails("api-secret"),
			want:     false,
		},
		{
			name:     "preserves object key omitted by read",
			read:     objectKeyDetails(""),
			existing: objectKeyDetails("object-secret"),
			want:     true,
		},
		{
			name:     "does not invent a missing secret",
			read:     nil,
			existing: nil,
			want:     false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := shouldPreserveExistingKeySecret(test.read, test.existing); got != test.want {
				t.Fatalf("shouldPreserveExistingKeySecret() = %v, want %v", got, test.want)
			}
		})
	}
}

func apiKeyDetails(secret string) interface{} {
	return []interface{}{map[string]interface{}{
		"api_key_details": []interface{}{map[string]interface{}{
			"api_key": secret,
		}},
	}}
}

func objectKeyDetails(secret string) interface{} {
	return []interface{}{map[string]interface{}{
		"object_key_details": []interface{}{map[string]interface{}{
			"secret_key": secret,
		}},
	}}
}
