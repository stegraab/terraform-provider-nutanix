package provider

import "testing"

func TestProviderSchemaInternalValidation(t *testing.T) {
	if err := Provider().InternalValidate(); err != nil {
		t.Fatal(err)
	}
}
