package eng

import (
	"testing"
)

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
		AnalyseFnBody(r, "each$body", nil, []Value{tok}, nil, nil, nil, false)
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
