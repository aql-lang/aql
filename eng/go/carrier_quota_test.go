package eng

import (
	"strconv"
	"testing"
)

// Per-fn analysis quota (design/checker-accuracy-review.10.md A9):
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

	// The counter is keyed by definition site (scope + name + body
	// position), not bare name — every iteration analyses the SAME body at
	// the SAME position under a fresh capture, so they share one counter.
	polyKey := fnQuotaKey(r.AnalysisScopeID(), "poly", body)
	if got := r.Check.FnAnalysisCounts[polyKey]; got <= FnAnalysisQuota {
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

// The quota is keyed by DEFINITION SITE, not bare name: many distinct closure
// bodies that share a synthetic "<word>$body" name (each$body across a whole
// library) must NOT pool one budget. Each distinct source position gets its
// own counter, so no single site trips the quota and none bails to a
// provenance-less Any (the "code-body word each (Stage 2)" refusal this fixes).
func TestFnAnalysisQuotaKeyedByDefinitionSite(t *testing.T) {
	r, _ := NewRegistry()
	done := r.Check.Begin()
	defer done()

	// Far more distinct closure bodies than the quota — but each at its own
	// source position, so each is analysed exactly once and none truncates.
	for i := 0; i <= FnAnalysisQuota+20; i++ {
		tok := NewInteger(int64(i))
		tok.SetPos(SrcPos{Row: i + 1, Col: 1})
		AnalyseFnBody(r, "each$body", nil, []Value{tok}, nil, nil, nil)
	}
	for _, d := range r.Check.Diagnostics {
		if d.Code == "analysis_truncated" {
			t.Fatalf("distinct closure sites were pooled and truncated: %s", d.Detail)
		}
	}
	if got := len(r.Check.FnAnalysisCounts); got < FnAnalysisQuota {
		t.Fatalf("expected one counter per distinct site, got %d", got)
	}
}
