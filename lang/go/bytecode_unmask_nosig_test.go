package lang

import (
	"strings"
	"testing"
)

// TestUnmaskNoSigRefusalReason pins Leaf-5 Part 1: on the COMPILE pass an
// unmatched dispatch over a dynamic / Disjunct receiver must surface its
// SPECIFIC refusal reason ("unmatched dispatch recovered at <w>"), not be masked
// as the generic "check diagnostics".
//
// checkModeAssumeSig used to MarkUncompilable AND emit an error-severity
// no_signature diagnostic. The MarkUncompilable already refuses (Finalize
// surfaces its reason), so the diagnostic was redundant on the compile pass and
// spurious: it tripped CompileCheck's diagnostic gate (aql.go:297) so the
// program refused as "check diagnostics", masking the real reason — while a
// plain `aql check` of the SAME program reports zero errors (the fn body is
// analysed once at its def site, not re-entered per call). The fix gates the
// diagnostic on Emit==nil (plain check), NOT es.active(): the get latch happens
// inside a SUSPENDED carrier re-dispatch where es.active() is false yet the
// program is still being compiled.
//
// Step 1 flips no files — it only makes the refusal reason honest — so the
// programs below MUST still refuse (prog==nil).
func TestUnmaskNoSigRefusalReason(t *testing.T) {
	cases := []struct{ name, src, wantWord string }{
		{
			// get over a Disjunct(Map|...) receiver bound by an if-union: the
			// minimized tst_unit shape. The inner get latches under a suspended
			// re-dispatch, the case the Emit==nil (not es.active()) gate fixes.
			"get over disjunct receiver",
			`def pluck fn [[nd:Any] [Any] [(nd "ch" get)]] ` +
				`def insert fn [[x:Any] [Any] [ def nd (if (x eq none) [{ch:1}] [x]) (nd pluck) ]] ` +
				`(none insert)`,
			"get",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			a, _ := New()
			prog, reason, _, _ := a.CompileCheck(c.src)
			if prog != nil {
				t.Fatalf("expected refusal (Step 1 flips no files), but it compiled")
			}
			if strings.Contains(reason, "check diagnostics") {
				t.Errorf("reason still masked as %q — want the specific %q",
					reason, "unmatched dispatch recovered at "+c.wantWord)
			}
			if want := "unmatched dispatch recovered at " + c.wantWord; !strings.Contains(reason, want) {
				t.Errorf("reason = %q, want it to contain %q", reason, want)
			}
			// The SAME program is clean under a plain `aql check` — that
			// check-vs-compile divergence is exactly the wart being removed.
			b, _ := New()
			cr, _ := b.Check(c.src)
			for _, d := range cr.Diagnostics {
				if d.Severity == SeverityError {
					t.Errorf("plain check should be clean, got error: %s %s", d.Code, d.Detail)
				}
			}
		})
	}
}

// TestNoSigStillReportedInPlainCheck guards the unmask above: suppressing the
// no_signature diagnostic on a REAL compile pass must NOT suppress it during a
// plain `aql check`, where a genuine unmatched dispatch is a real static error
// the user must still see. The discriminator MUST be Check.Compiling, NOT
// Emit==nil / es.active(): a plain check arms a fresh ACTIVE Emit while
// analysing each fn body (IsolateEmit), so the fn-body-nested case below — a
// genuine type error inside `def f`'s body — is the one that pins it. (An
// earlier Emit==nil gate silently dropped exactly this error.)
func TestNoSigStillReportedInPlainCheck(t *testing.T) {
	for _, c := range []struct{ name, src string }{
		{"top-level concrete type error", `({a:1} sub 3)`},
		{"top-level boolean+integer", `(true add 1)`},
		// Nested inside a fn body → analysed under an armed Emit (IsolateEmit);
		// the Compiling discriminator is what keeps this reported.
		{"fn-body-nested type error", `def use fn [[s:String] [Integer] [ 1 ]] ` +
			`def f fn [[] [Integer] [ [1 2 3] use ]]`},
	} {
		a, _ := New()
		cr, _ := a.Check(c.src)
		found := false
		for _, d := range cr.Diagnostics {
			if d.Severity == SeverityError && d.Code == "no_signature" {
				found = true
			}
		}
		if !found {
			t.Errorf("%s (%q): plain check must still report a no_signature error (over-suppression regression)", c.name, c.src)
		}
	}
}
