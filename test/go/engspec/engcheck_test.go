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
	basic "github.com/boru-lang/boru/basic/go"
	"os"
	"path/filepath"
	"strings"
	"testing"

	core "github.com/boru-lang/boru/core/go"
	"github.com/boru-lang/boru/test/specfix"
	"github.com/boru-lang/boru/parser/go"
)

// runCheckRow parses input, runs it through the engine in check mode
// against the kernel fixtures, and returns the rendered check result.
// Mirrors lang/go/boru.go::Check end-to-end (Begin → Run → rescue forward
// refs → emit unused-def warnings) at the engine-kernel level.
func runCheckRow(input string) (string, error) {
	values, err := parser.Parse(input)
	if err != nil {
		return "", err
	}
	r, err := core.NewRegistry()
	if err != nil {
		return "", err
	}
	specfix.RegisterSpecWords(r)
	basic.InstallMicronIdeals(r)
	specfix.RegisterCheckExtras(r)
	r.InitRootContext()
	r.Source = input

	done := r.Check.Begin()
	out, runErr := core.NewTop(r).Run(values)
	r.RescueForwardRefDiagnostics()
	r.Check.EmitUnusedDefDiagnostics()
	diags := r.Check.Diagnostics
	done()

	if runErr != nil {
		return "", runErr
	}
	return specfix.RenderCheck(out, diags), nil
}

// TestCheckSpec runs every eng/spec/check/*.tsv row through the checker
// and compares the rendered result to the row's expected column.
func TestCheckSpec(t *testing.T) {
	dir := filepath.Join("..", "..", "..", "eng", "spec", "check")
	specfix.RunDirRendered(t, dir, runCheckRow)
}

// TestCheckDump prints `input<TAB>rendered` for each candidate input read
// from BORU_CHECK_DUMP (newline-separated). Used to author the corpus —
// the rendered output becomes the expected column. Skipped unless the
// env var is set.
func TestCheckDump(t *testing.T) {
	raw := os.Getenv("BORU_CHECK_DUMP")
	if raw == "" {
		t.Skip("set BORU_CHECK_DUMP to a newline-separated input list to dump rendered check results")
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
