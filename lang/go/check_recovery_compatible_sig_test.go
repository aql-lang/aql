package lang

import (
	"strings"
	"testing"
)

// TestRecoveryCompatibleSigNoFalseError pins the bestMatch gate on the recovery
// no_signature diagnostic (engine.go checkModeAssumeSig). A recursive call whose args
// resolve LATE (forward-collection during the fn's own in-flight analysis) makes
// matchSignature transiently return nil and fall into the recovery — but the recovery's
// own lenient scoring (sigArgMatches) DOES find a sig the args satisfy (bestMatch >= 0),
// picks it, and produces the right result. The error diagnostic was therefore spurious
// (the args match the sig). Emit it ONLY when no compatible sig exists (bestMatch < 0 =
// a genuine mismatch).
//
// Repro: a self-recursive fn whose recursive arg is `(blk get "next")` (an Integer that
// resolves through the in-flight body). Its args (List, Integer) match the sole sig, so
// no no_signature must be reported. A GENUINE mismatch (provable String → Integer) is
// bestMatch < 0 and STILL flags — see TestAddGradualConcatReturnTyping /
// TestSliceDynamicReceiverRefines, which still pass. Compile is unaffected
// (MarkUncompilable still refuses); this is check-diagnostic-only.
func TestRecoveryCompatibleSigNoFalseError(t *testing.T) {
	const src = `def f fn [ [t:List i:Integer] [Map] [
  if (i gte (t size)) [ do {next:[i]} ] [
    def blk  (f t (i add 1))
    def more (f t (blk get "next"))
    do { next:[(more get "next")] }
  ]
] ]
def _ ([{}] f 0)`

	a, _ := New()
	res, _ := a.Check(src)
	for _, d := range res.Diagnostics {
		if d.Code == "no_signature" && (d.Word == "f" || strings.Contains(d.Detail, "for f")) {
			t.Errorf("recursive call whose args match the sole sig must not emit no_signature: %s", d.Detail)
		}
	}

	// NEGATIVE: a provable concrete mismatch (no compatible sig, bestMatch < 0) MUST
	// still raise no_signature (or a hard signature error) — the gate only drops the
	// spurious compatible-sig case.
	const bad = `def need-map fn [ [m:Map] [Integer] [ 1 ] ]
def _ (need-map "not-a-map")`
	b, _ := New()
	res2, _ := b.Check(bad)
	flagged := false
	for _, d := range res2.Diagnostics {
		if d.Code == "no_signature" || d.Code == "signature_error" || strings.Contains(d.Detail, "no matching signature") {
			flagged = true
			break
		}
	}
	if !flagged {
		t.Errorf("a provable concrete mismatch (String → Map param) must still be flagged")
	}
}
