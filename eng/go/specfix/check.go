package specfix

import (
	"strings"

	eng "github.com/boru-lang/boru/eng/go"
)

// Check-corpus support, shared by the engspec check harness and eng's
// standalone check run. Moved from test/go/engspec/engcheck_test.go.

// renderCheck renders a check result — the residual carrier stack plus
// the collected diagnostics — to a single comparable string. The format
// (mirrored verbatim by the TypeScript runner) is:
//
//	<stack> :: <diag> <diag> …
//
// where <stack> is the carrier leaves joined by spaces (a dynamic
// carrier renders as `dynamic(Leaf)`, a strict one as `Leaf`), and each
// <diag> is a severity sigil (`!` error, `~` warning, `?` info) followed
// by the diagnostic code, in emission order. The ` :: …` suffix is
// omitted when there are no diagnostics; an empty stack with diagnostics
// renders as `:: <diag> …`.
func RenderCheck(stack []eng.Value, diags []eng.CheckDiagnostic) string {
	parts := make([]string, len(stack))
	for i, v := range stack {
		if v.Dynamic {
			parts[i] = "dynamic(" + v.Parent.Leaf() + ")"
		} else {
			parts[i] = v.Parent.Leaf()
		}
	}
	s := strings.Join(parts, " ")
	if len(diags) == 0 {
		return s
	}
	ds := make([]string, len(diags))
	for i, d := range diags {
		sigil := "?"
		switch d.Severity {
		case eng.SeverityError:
			sigil = "!"
		case eng.SeverityWarning:
			sigil = "~"
		}
		ds[i] = sigil + d.Code
	}
	return strings.TrimSpace(s + " :: " + strings.Join(ds, " "))
}

// RegisterCheckExtras adds fixtures that only the check runner needs —
// chiefly `noretq`, a word with a Handler but NO Returns/ReturnsFn
// annotation, so dispatch over it emits the missing_returns diagnostic
// and falls back to a dynamic(Any) carrier.
func RegisterCheckExtras(r *eng.Registry) {
	r.RegisterNativeFunc(eng.NativeFunc{
		Name: "noretq",
		Signatures: []eng.Signature{{
			Args:       []*eng.Type{eng.TInteger},
			BarrierPos: -1,
			Impl: eng.Go(func(args []eng.Value, _ map[string]eng.Value, _ []eng.Value, _ *eng.Registry) ([]eng.Value, error) {
				return []eng.Value{args[0]}, nil
			}),
			// Deliberately no Returns / ReturnsFn.
		}},
	})
}
