package modules

import (
	"sort"
	"strings"
	"testing"

	"github.com/boru-lang/boru/eng/go/parser"
	"github.com/boru-lang/boru/lang/go/native"
)

// TestModuleExportDocs asserts that every native-module FUNCTION export has a
// one-line Doc (sourced from the moduleDocs table in docs.go and applied by
// stampExportProvenance). It also pins the provenance: each function export
// gets its Module (import id) and Export (namespace) stamped. This keeps the
// doc table from falling behind the export set — a new export without a doc
// fails here, the describe-DX analogue of the ADR-003 spec-coverage guard.
//
// Type exports (capitalised names) are not functions and are
// intentionally not required to carry a Doc.
func TestModuleExportDocs(t *testing.T) {
	reg, err := native.DefaultRegistry()
	if err != nil {
		t.Fatalf("DefaultRegistry: %v", err)
	}
	reg.SetParseFunc(parser.Parse) // boru modules parse their source on import
	InstallResolver(reg)           // boru:repl's boru preamble imports boru:net / boru:vm

	var missing []string
	names := Names()
	sort.Strings(names)
	for _, name := range names {
		desc, derr := Resolve(name, reg)
		if derr != nil {
			t.Fatalf("resolve module %q: %v", name, derr)
		}
		nss := make([]string, 0, len(desc.Exports))
		for ns := range desc.Exports {
			nss = append(nss, ns)
		}
		sort.Strings(nss)
		for _, ns := range nss {
			om := desc.Exports[ns]
			for _, key := range om.Keys() {
				v, _ := om.Get(key)
				fn, ok := native.FnDefFromValue(v)
				if !ok {
					continue // type export or non-function value
				}
				qualified := ns + "." + key
				if fn.Module != desc.Ref {
					t.Errorf("%s: Module = %q, want %q", qualified, fn.Module, desc.Ref)
				}
				if fn.Export != ns {
					t.Errorf("%s: Export = %q, want %q", qualified, fn.Export, ns)
				}
				if fn.Doc == "" {
					missing = append(missing, qualified)
				}
			}
		}
	}

	if len(missing) > 0 {
		sort.Strings(missing)
		t.Fatalf("%d native-module function export(s) have no Doc summary. "+
			"Add an entry per word to moduleDocs in lang/go/modules/docs.go:\n  %s",
			len(missing), join(missing))
	}
}

// TestModuleExamples is the moduleExamples table's ratchet. Unlike docs,
// examples are OPT-IN — an export without them keeps the generated
// positional permutations, which read fine for the pure helpers. So the
// guard runs the other way: every KEY must name a real function export,
// so a renamed or removed word cannot leave a stale example behind
// describing a word that no longer exists. It also pins that whatever is
// registered actually reaches the FnDefInfo `describe` renders from —
// a table nothing reads would rot silently.
func TestModuleExamples(t *testing.T) {
	reg, err := native.DefaultRegistry()
	if err != nil {
		t.Fatalf("DefaultRegistry: %v", err)
	}
	reg.SetParseFunc(parser.Parse)
	InstallResolver(reg)

	// Gather every (import id, export name) the module set actually has,
	// and the stamped FnDefInfo behind it.
	stamped := map[string]map[string]*native.FnDefInfo{}
	for _, name := range Names() {
		desc, derr := Resolve(name, reg)
		if derr != nil {
			t.Fatalf("resolve module %q: %v", name, derr)
		}
		byWord := map[string]*native.FnDefInfo{}
		for _, om := range desc.Exports {
			for _, key := range om.Keys() {
				v, _ := om.Get(key)
				if fn, ok := native.FnDefFromValue(v); ok {
					byWord[key] = fn
				}
			}
		}
		stamped[desc.Ref] = byWord
	}

	refs := make([]string, 0, len(moduleExamples))
	for ref := range moduleExamples {
		refs = append(refs, ref)
	}
	sort.Strings(refs)

	total := 0
	for _, ref := range refs {
		byWord, ok := stamped[ref]
		if !ok {
			t.Errorf("moduleExamples has entries for %q, which is not a native module", ref)
			continue
		}
		words := make([]string, 0, len(moduleExamples[ref]))
		for w := range moduleExamples[ref] {
			words = append(words, w)
		}
		sort.Strings(words)
		for _, word := range words {
			lines := moduleExamples[ref][word]
			fn, ok := byWord[word]
			if !ok {
				t.Errorf("moduleExamples[%q] has examples for %q, which that module does not export",
					ref, word)
				continue
			}
			if len(lines) == 0 {
				t.Errorf("%s: an empty example list is worse than none — remove the key", word)
			}
			for i, line := range lines {
				if strings.TrimSpace(line) == "" {
					t.Errorf("%s.%s example %d is blank", ref, word, i)
				}
			}
			// The point of the table: the lines must reach the value
			// `describe` renders from.
			if len(fn.Examples) != len(lines) {
				t.Errorf("%s.%s: %d examples stamped, %d registered — stampExportProvenance did not apply them",
					ref, word, len(fn.Examples), len(lines))
			}
			// An example is a promise that this is what you type. The
			// cheap half of that promise is checkable: strip the `;#`
			// commentary and the block must PARSE. It cannot catch a
			// wrong result, but it does catch the realistic authoring
			// error — an unbalanced bracket or quote in a multi-line
			// options map.
			if src := exampleSource(lines); src != "" {
				if _, pErr := parser.Parse(src); pErr != nil {
					t.Errorf("%s.%s examples do not parse: %v\n%s", ref, word, pErr, src)
				}
			}
			total++
		}
	}
	if total == 0 {
		t.Error("no examples registered at all — the table is not being loaded")
	}
}

// Authored examples must actually REPLACE the generated permutations in
// the rendered output, for both the qualified-name path (an imported
// namespace) and the module-path path (describe "boru:net:fetch"). This is
// the behaviour the whole registry exists to produce.
func TestDescribeUsesAuthoredExamples(t *testing.T) {
	reg, err := native.DefaultRegistry()
	if err != nil {
		t.Fatalf("DefaultRegistry: %v", err)
	}
	reg.SetParseFunc(parser.Parse)
	InstallResolver(reg)

	var b strings.Builder
	native.DescribeName(reg, &b, "boru:net:fetch")
	out := b.String()
	if !strings.Contains(out, `Net.fetch "https://api.example.com/v1/orders"`) {
		t.Errorf("describe did not show the authored fetch examples:\n%s", out)
	}
	// The generated permutation for fetch's [String Map] sig. Its
	// presence would mean the authored lines did not take precedence.
	if strings.Contains(out, "Net.fetch 'a' {a:1,b:2}") {
		t.Errorf("the generated permutations survived alongside authored examples:\n%s", out)
	}

	// An export with no authored examples keeps the generated ones —
	// opt-in, not a silent loss of the existing output.
	b.Reset()
	native.DescribeName(reg, &b, "boru:array-util:reshape")
	if got := b.String(); !strings.Contains(got, ";#") {
		t.Errorf("an export without authored examples lost its generated ones:\n%s", got)
	}
}

// exampleSource reassembles a word's example lines into parseable boru:
// the `;#` commentary comes off (it is prose, and a multi-line example
// puts it mid-literal), and lines that were only a comment drop out.
func exampleSource(lines []string) string {
	var kept []string
	for _, line := range lines {
		if i := strings.Index(line, ";#"); i >= 0 {
			line = line[:i]
		}
		if strings.TrimSpace(line) == "" {
			continue
		}
		kept = append(kept, line)
	}
	return strings.Join(kept, "\n")
}

func join(s []string) string {
	out := ""
	for i, v := range s {
		if i > 0 {
			out += "\n  "
		}
		out += v
	}
	return out
}
