package networkingv2

import "testing"

func TestExpandNLBHealthCheckDefaultsWhenMissing(t *testing.T) {
	got := expandNLBHealthCheck(nil)

	if got == nil {
		t.Fatal("expected health check, got nil")
	}
	if *got.IntervalSecs != 10 {
		t.Fatalf("expected interval 10, got %d", *got.IntervalSecs)
	}
	if *got.TimeoutSecs != 5 {
		t.Fatalf("expected timeout 5, got %d", *got.TimeoutSecs)
	}
	if *got.SuccessThreshold != 3 {
		t.Fatalf("expected success threshold 3, got %d", *got.SuccessThreshold)
	}
	if *got.FailureThreshold != 3 {
		t.Fatalf("expected failure threshold 3, got %d", *got.FailureThreshold)
	}
}

func TestExpandNLBHealthCheckUsesDefaultsForZeroValues(t *testing.T) {
	got := expandNLBHealthCheck([]interface{}{
		map[string]interface{}{
			"interval_secs":     0,
			"timeout_secs":      0,
			"success_threshold": 0,
			"failure_threshold": 0,
		},
	})

	if got == nil {
		t.Fatal("expected health check, got nil")
	}
	if *got.IntervalSecs != 10 {
		t.Fatalf("expected interval 10, got %d", *got.IntervalSecs)
	}
	if *got.TimeoutSecs != 5 {
		t.Fatalf("expected timeout 5, got %d", *got.TimeoutSecs)
	}
	if *got.SuccessThreshold != 3 {
		t.Fatalf("expected success threshold 3, got %d", *got.SuccessThreshold)
	}
	if *got.FailureThreshold != 3 {
		t.Fatalf("expected failure threshold 3, got %d", *got.FailureThreshold)
	}
}

func TestExpandNLBHealthCheckMergesPartialOverrides(t *testing.T) {
	got := expandNLBHealthCheck([]interface{}{
		map[string]interface{}{
			"interval_secs": 7,
		},
	})

	if got == nil {
		t.Fatal("expected health check, got nil")
	}
	if *got.IntervalSecs != 7 {
		t.Fatalf("expected interval 7, got %d", *got.IntervalSecs)
	}
	if *got.TimeoutSecs != 5 {
		t.Fatalf("expected timeout 5, got %d", *got.TimeoutSecs)
	}
	if *got.SuccessThreshold != 3 {
		t.Fatalf("expected success threshold 3, got %d", *got.SuccessThreshold)
	}
	if *got.FailureThreshold != 3 {
		t.Fatalf("expected failure threshold 3, got %d", *got.FailureThreshold)
	}
}
