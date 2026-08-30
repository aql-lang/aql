// bind_ledger_census_test.go sizes the bind-twin work before it is built.
//
// The bind twins (design/FULL-COMPILATION.0.md §6.5) are Stage 4's remaining
// piece and the named fix for NUR110, family L's conditional fn shadow, and the
// unledgered rebind-staleness gates. Every check-mode transition that affects a
// RUNTIME-VISIBLE binding has to emit a twin op, so the VM's registry state at
// instruction i equals the interpreter's tape state at the corresponding token.
//
// Before writing an op, this measures what the population actually is. That is
// the same order Stage 1 imposed on the interpreter-entry work, and it has paid
// every time since: the raw grep of `r.Defs` mutation sites ranks
// native_behave.go first with 18 and generics_instantiate.go third with 7, and
// NEITHER needs a twin — the first is a behaviour body's `Push("a", …)` +
// `defer Pop("a")`, the second a balanced pair around a compile-time product.
// A count of call sites is not a count of transitions.
//
// The ledger is INERT. Nothing reads it to decide anything, so this census
// cannot regress a program; it can only tell the next increment how big it is.
package langspec

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	core "github.com/boru-lang/boru/core/go"
	lang "github.com/boru-lang/boru/lang/go"
)

func bindKindName(k core.BindKind) string {
	switch k {
	case core.BindDef:
		return "def"
	case core.BindUndef:
		return "undef"
	case core.BindTypeInstall:
		return "type-install"
	}
	return fmt.Sprintf("kind(%d)", uint8(k))
}

func TestBindLedgerCensus(t *testing.T) {
	specDir := filepath.Join("..", "..", "..", "lang", "spec")
	entries, err := os.ReadDir(specDir)
	if err != nil {
		t.Fatal(err)
	}
	byKind := map[string]int{}
	rows, withLedger, maxLen := 0, 0, 0
	maxSrc := ""

	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".tsv") {
			continue
		}
		f, ferr := os.Open(filepath.Join(specDir, e.Name()))
		if ferr != nil {
			t.Fatal(ferr)
		}
		sc := bufio.NewScanner(f)
		sc.Buffer(make([]byte, 1024*1024), 1024*1024)
		for sc.Scan() {
			line := strings.TrimRight(sc.Text(), " \t")
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			parts := strings.Split(line, "\t")
			if len(parts) < 2 {
				continue
			}
			src := strings.TrimSpace(parts[0])
			rows++
			a, aerr := lang.New()
			if aerr != nil {
				continue
			}
			_, _, res, _ := a.CompileCheck(src)
			if len(res.BindLedger) == 0 {
				continue
			}
			withLedger++
			if len(res.BindLedger) > maxLen {
				maxLen, maxSrc = len(res.BindLedger), src
			}
			for _, tr := range res.BindLedger {
				byKind[bindKindName(tr.Kind)]++
			}
		}
		_ = f.Close()
	}

	kinds := make([]string, 0, len(byKind))
	total := 0
	for k, n := range byKind {
		kinds = append(kinds, k)
		total += n
	}
	sort.Slice(kinds, func(i, j int) bool { return byKind[kinds[i]] > byKind[kinds[j]] })

	t.Logf("bind-ledger census: %d rows, %d with transitions, %d transitions total", rows, withLedger, total)
	for _, k := range kinds {
		t.Logf("    %-14s %6d", k, byKind[k])
	}
	if maxLen > 0 {
		src := maxSrc
		if len(src) > 110 {
			src = src[:110] + "…"
		}
		t.Logf("    deepest row: %d transitions — %s", maxLen, src)
	}

	// The census must SEE something, or it is measuring its own wiring rather
	// than the corpus: every one of these kinds is reachable from a plain
	// `def`, and a zero here means the notes are not firing.
	if byKind["def"] == 0 {
		t.Error("no `def` transitions recorded — the ledger is not wired to installDef")
	}
}
