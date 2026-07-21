package docexamples

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/aql-lang/aql/lang/go/formatter"
)

// TestDocFencedBlocksFmtClean enforces that every ```aql fenced code block
// in the prose docs is already fmt-formatted: FormatMarkdown — which only
// touches ```aql fences and <!-- aqlfmt --> regions — must leave the file
// unchanged. This is the "code examples in docs run through fmt" gate for
// fenced blocks; `make fmt-docs` (root Makefile) applies the canonical
// form to every gated doc, and this keeps the blocks from drifting back
// out of it.
func TestDocFencedBlocksFmtClean(t *testing.T) {
	for _, name := range docFiles {
		path := filepath.Join(docRoot(), name)
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		if got := formatter.FormatMarkdown(string(body)); got != string(body) {
			t.Errorf("%s: an ```aql block is not fmt-clean; run `make fmt-docs` (or `aql fmt %s`)", name, name)
		}
	}
}
