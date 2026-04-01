package objectstoresv2

import (
	"encoding/json"
	"testing"
)

func TestNormalizeObjectStoreCertificateJSONBody(t *testing.T) {
	body, err := normalizeObjectStoreCertificateJSONBody(`{
		"alternateFqdns": [
			{"value": "objects.example.com"}
		],
		"alternateIps": [
			{"ipv4": {"value": "10.44.77.123"}}
		],
		"shouldGenerate": true
	}`)
	if err != nil {
		t.Fatalf("expected json body to parse: %v", err)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(body), &parsed); err != nil {
		t.Fatalf("expected normalized json to unmarshal: %v", err)
	}

	if parsed["shouldGenerate"] != true {
		t.Fatalf("expected shouldGenerate to be true, got %#v", parsed["shouldGenerate"])
	}

	alternateFqdns, ok := parsed["alternateFqdns"].([]interface{})
	if !ok || len(alternateFqdns) != 1 {
		t.Fatalf("unexpected alternateFqdns: %#v", parsed["alternateFqdns"])
	}

	alternateIps, ok := parsed["alternateIps"].([]interface{})
	if !ok || len(alternateIps) != 1 {
		t.Fatalf("unexpected alternateIps: %#v", parsed["alternateIps"])
	}
}

func TestNormalizeObjectStoreCertificateJSONBodyInvalidJSON(t *testing.T) {
	if _, err := normalizeObjectStoreCertificateJSONBody(`{"alternateIps":[`); err == nil {
		t.Fatal("expected invalid json to return an error")
	}
}
