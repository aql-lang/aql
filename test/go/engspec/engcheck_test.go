// Check-mode spec runner for the engine kernel. Runs the corpus at
// eng/spec/check/*.tsv through the SAME kernel fixtures the value-mode
// engspec runner uses (registerSpecWords — already carrying Returns
// annotations), but in static-analysis (check) mode: input literals are
// stripped to carriers, dispatch produces carrier return values, and
// signature/return violations surface as diagnostics rather than hard
// errors.
//
// This is the Go half of the eng-level check parity contract: the
// TypeScript port (eng/ts) runs the identical .tsv files and must render
// each row to the identical string (see renderCheck for the format).
package engspec

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aql-lang/aql/eng/go"
	"github.com/aql-lang/aql/eng/go/parser"
	"github.com/aql-lang/aql/test/go/specrunner"
)

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
func renderCheck(stack []eng.Value, diags []eng.CheckDiagnostic) string {
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

// runCheckRow parses input, runs it through the engine in check mode
// against the kernel fixtures, and returns the rendered check result.
// Mirrors lang/go/aql.go::Check end-to-end (Begin → Run → rescue forward
// refs → emit unused-def warnings) at the engine-kernel level.
func runCheckRow(input string) (string, error) {
	values, err := parser.Parse(input)
	if err != nil {
		return "", err
	}
	r, err := eng.NewRegistry()
	if err != nil {
		return "", err
	}
	registerSpecWords(r)
	registerCheckExtras(r)
	r.InitRootContext()
	r.Source = input

	done := r.Check.Begin()
	out, runErr := eng.NewTop(r).Run(values)
	r.RescueForwardRefDiagnostics()
	r.Check.EmitUnusedDefDiagnostics()
	diags := r.Check.Diagnostics
	done()

	if runErr != nil {
		return "", runErr
	}
	return renderCheck(out, diags), nil
}

// registerCheckExtras adds fixtures that only the check runner needs —
// chiefly `noretq`, a word with a Handler but NO Returns/ReturnsFn
// annotation, so dispatch over it emits the missing_returns diagnostic
// and falls back to a dynamic(Any) carrier.
func registerCheckExtras(r *eng.Registry) {
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

// TestCheckSpec runs every eng/spec/check/*.tsv row through the checker
// and compares the rendered result to the row's expected column.
func TestCheckSpec(t *testing.T) {
	dir := filepath.Join("..", "..", "..", "eng", "spec", "check")
	specrunner.RunDirRendered(t, dir, runCheckRow)
}

// TestCheckDump prints `input<TAB>rendered` for each candidate input read
// from AQL_CHECK_DUMP (newline-separated). Used to author the corpus —
// the rendered output becomes the expected column. Skipped unless the
// env var is set.
func TestCheckDump(t *testing.T) {
	raw := os.Getenv("AQL_CHECK_DUMP")
	if raw == "" {
		t.Skip("set AQL_CHECK_DUMP to a newline-separated input list to dump rendered check results")
	}
	for _, line := range strings.Split(raw, "\n") {
		input := strings.TrimSpace(line)
		if input == "" {
			continue
		}
		got, err := runCheckRow(input)
		if err != nil {
			fmt.Printf("%s\tERROR:%s\n", input, err.Error())
			continue
		}
		fmt.Printf("%s\t%s\n", input, got)
	}
}
