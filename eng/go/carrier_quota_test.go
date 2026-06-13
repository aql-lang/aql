package eng

import (
	"strconv"
	"testing"
)

// Per-fn analysis quota (design/checker-accuracy-review.0.md A9):
// past FnAnalysisQuota distinct call shapes the analyser answers
// without body re-analysis and emits exactly one analysis_truncated
// diagnostic.
func TestFnAnalysisQuota(t *testing.T) {
	r, _ := NewRegistry()
	done := r.Check.Begin()
	defer done()

	body := []Value{NewInteger(1)}
	// Distinct memo keys via distinct capture names — the cheap way to
	// force a fresh analysis per call without minting types.
	for i := 0; i <= FnAnalysisQuota+5; i++ {
		caps := []CapturedBinding{{Name: "c" + strconv.Itoa(i), Value: NewCarrier(TInteger)}}
		AnalyseFnBody(r, "poly", nil, body, nil, caps, nil)
	}

	if got := r.Check.FnAnalysisCounts["poly"]; got <= FnAnalysisQuota {
		t.Fatalf("expected the counter past the quota, got %d", got)
	}
	count := 0
	for _, d := range r.Check.Diagnostics {
		if d.Code == "analysis_truncated" {
			count++
			if d.Severity != SeverityInfo {
				t.Errorf("analysis_truncated severity = %s, want info", d.Severity)
			}
		}
	}
	if count != 1 {
		t.Errorf("analysis_truncated emitted %d times, want exactly 1", count)
	}

	// Under-quota fns are never truncated (negative).
	r2, _ := NewRegistry()
	done2 := r2.Check.Begin()
	defer done2()
	for i := 0; i < 3; i++ {
		caps := []CapturedBinding{{Name: "c" + strconv.Itoa(i), Value: NewCarrier(TInteger)}}
		AnalyseFnBody(r2, "small", nil, body, nil, caps, nil)
	}
	for _, d := range r2.Check.Diagnostics {
		if d.Code == "analysis_truncated" {
			t.Errorf("under-quota fn was truncated: %s", d.Detail)
		}
	}
}
