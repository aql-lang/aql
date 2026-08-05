package specfix

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/boru-lang/boru/eng/go"
	"github.com/boru-lang/boru/eng/go/parser"
)

// TestRunFileRenderedSkipRow pins the rendered runner's ErrSkipRow arm:
// a RenderRun that reports a row as content-layer-only must skip the
// subtest, not fail it (the value-mode runner's arm is exercised by the
// standalone corpus lanes; the rendered runner's only by this probe,
// since the check lane runs every row).
func TestRunFileRenderedSkipRow(t *testing.T) {
	path := filepath.Join(t.TempDir(), "skip.tsv")
	if err := os.WriteFile(path, []byte("skip me\tunused\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ran := false
	RunFileRendered(t, path, func(string) (string, error) {
		ran = true
		return "", ErrSkipRow
	})
	if !ran {
		t.Error("RenderRun was never invoked")
	}
}

// TestDoEvalMapValueErrorArms drives doEvalMapValue's error paths
// directly: through the `do` word they cannot fire, because dispatch
// auto-evaluates plain-map arguments (and propagates their errors)
// before the handler's own walk runs. A raw map holding an inert code
// list that errors when run reaches both the list arm's sub-run error
// and the map loop's recursion-error check.
func TestDoEvalMapValueErrorArms(t *testing.T) {
	r, err := eng.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	RegisterSpecWords(r)
	r.InitRootContext()

	vals, err := parser.Parse("[refine 5]")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(vals) != 1 {
		t.Fatalf("want one inert list value, got %d", len(vals))
	}
	om := eng.NewOrderedMap()
	om.Set("a", vals[0])
	if _, err := doEvalMapValue(r, eng.NewMap(om)); err == nil ||
		!strings.Contains(err.Error(), "argument must be a type") {
		t.Errorf("want the sub-run's refine error, got %v", err)
	}
}
