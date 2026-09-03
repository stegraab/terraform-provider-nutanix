package vmmv2

import "testing"

func TestNGTInsertIsoVMExtIDForcesReplacement(t *testing.T) {
	t.Parallel()

	extID := ResourceNutanixNGTInsertIsoV2().Schema["ext_id"]
	if extID == nil {
		t.Fatal("expected ext_id in NGT insert ISO schema")
	}
	if !extID.Required || !extID.ForceNew || extID.Optional || extID.Computed {
		t.Fatalf("expected ext_id to be required and ForceNew, got %#v", extID)
	}
}
