package vmmv2

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func TestNGTInsertIsoDeleteSkipsEjectWhenISOIsNotInserted(t *testing.T) {
	t.Parallel()

	resourceSchema := ResourceNutanixNGTInsertIsoV2().Schema
	data := schema.TestResourceDataRaw(t, resourceSchema, map[string]interface{}{
		"ext_id":          "vm-ext-id",
		"vm_ext_id":       "vm-ext-id",
		"cdrom_ext_id":    "cdrom-ext-id",
		"action":          "insert",
		"is_iso_inserted": false,
	})

	diagnostics := ResourceNutanixNGTInsertIsoV2Delete(context.Background(), data, nil)
	if diagnostics.HasError() {
		t.Fatalf("expected an already-ejected NGT ISO to delete without error, got %#v", diagnostics)
	}
	if len(diagnostics) != 1 || diagnostics[0].Severity != diag.Warning {
		t.Fatalf("expected one warning for an already-ejected NGT ISO, got %#v", diagnostics)
	}
}
