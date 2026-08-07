package native

import (
	"strings"
	"testing"

	check "github.com/boru-lang/boru/check/go"
	core "github.com/boru-lang/boru/core/go"
)

func sigArgsStr(s core.Signature) string {
	parts := make([]string, 0, len(s.ArgTypes()))
	for _, t := range s.ArgTypes() {
		if t == nil {
			parts = append(parts, "<nil>")
			continue
		}
		parts = append(parts, t.String())
	}
	return "[" + strings.Join(parts, " ") + "]"
}

// TestNoDeadNativeOverloads is the kernel regression gate for
// unreachable signatures. boru dispatch is first-match-wins over
// SortSignatures' order, so a registered word must not declare an
// overload that an earlier, higher-priority signature already subsumes
// — it could never win. This catches sig-ordering and duplicate-signature
// mistakes the way fixedid_stability_test catches FixedID drift. The
// whole built-in vocabulary is currently clean; a failure here means a
// new word added a dead overload (or the subsumption rule needs a new
// discriminator).
func TestNoDeadNativeOverloads(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range r.Defs.Names() {
		fn := r.Lookup(name)
		if fn == nil || len(fn.Signatures) < 2 {
			continue
		}
		for _, d := range check.DeadSignatures(fn.Signatures) {
			t.Errorf("dead overload: word %q sig[%d] %s is unreachable — shadowed by earlier %s",
				name, d.Index, sigArgsStr(d.Sig), sigArgsStr(d.ShadowedBy))
		}
	}
}
