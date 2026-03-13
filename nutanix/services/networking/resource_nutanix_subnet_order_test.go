package networking

import "testing"

func TestStableOrderStringList(t *testing.T) {
	tests := []struct {
		name       string
		apiList     []string
		currentRaw  interface{}
		expectedOut []string
	}{
		{
			name:       "preserves current state ordering for same values",
			apiList:     []string{"10.66.0.164", "10.66.0.87"},
			currentRaw:  []interface{}{"10.66.0.87", "10.66.0.164"},
			expectedOut: []string{"10.66.0.87", "10.66.0.164"},
		},
		{
			name:       "appends unexpected api values",
			apiList:     []string{"10.66.0.164", "10.66.0.87", "10.66.0.200"},
			currentRaw:  []interface{}{"10.66.0.87", "10.66.0.164"},
			expectedOut: []string{"10.66.0.87", "10.66.0.164", "10.66.0.200"},
		},
		{
			name:       "returns api order when current is missing",
			apiList:     []string{"10.66.0.164", "10.66.0.87"},
			currentRaw:  nil,
			expectedOut: []string{"10.66.0.164", "10.66.0.87"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := stableOrderStringList(tc.apiList, tc.currentRaw)
			if len(result) != len(tc.expectedOut) {
				t.Fatalf("expected %d items, got %d (%v)", len(tc.expectedOut), len(result), result)
			}
			for i := range tc.expectedOut {
				if result[i] != tc.expectedOut[i] {
					t.Fatalf("unexpected result at index %d: expected %q got %q (full=%v)", i, tc.expectedOut[i], result[i], result)
				}
			}
		})
	}
}
