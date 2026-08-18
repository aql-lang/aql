package test

import (
	"errors"
	"sort"
	"strings"
	"testing"

	core "github.com/boru-lang/boru/core/go"
	"github.com/boru-lang/boru/lang/go/capabilities"
	"github.com/boru-lang/boru/lang/go/native"
	"github.com/boru-lang/boru/lang/go/native/help"
	parser "github.com/boru-lang/boru/parser/go"
)

// TestHandAuthoredExamplesWin verifies §5.2/§9.5: when a word's help
// Entry carries hand-authored Examples, `describe` renders those instead
// of the auto-generated positional permutations.
func TestHandAuthoredExamplesWin(t *testing.T) {
	reg, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(reg)
	cases := []struct {
		word     string
		wantLine string // a hand-authored example substring that must appear
		notWant  string // an auto-generated permutation that must NOT appear
	}{
		{"set", `c 1 "count" set`, `set 'a'`},
		{"get", `[10 20 30] 0 get`, ``},
		{"make", `make Point {x:3 y:4}`, ``},
		{"each", `[1 2 3] each [dup mul]`, ``},
		{"fold", `0 fold [add] [1 2 3 4]`, ``},
	}
	for _, c := range cases {
		info := native.BuildFuncInfo(reg, c.word)
		if info == nil {
			t.Errorf("%s: no FuncInfo", c.word)
			continue
		}
		out := help.FormatDynamic(*info)
		if !strings.Contains(out, c.wantLine) {
			t.Errorf("describe %s missing hand-authored example %q:\n%s", c.word, c.wantLine, out)
		}
		if c.notWant != "" && strings.Contains(out, c.notWant) {
			t.Errorf("describe %s still shows auto-generated permutation %q (hand-authored should win):\n%s", c.word, c.notWant, out)
		}
	}
}

// TestHelpAllWords checks that every registered word produces valid
// dynamic help output with the expected sections.
func TestHelpAllWords(t *testing.T) {
	reg, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(reg)

	words := allRegisteredWords(reg)
	if len(words) == 0 {
		t.Fatal("no words registered")
	}

	for _, word := range words {
		t.Run(word, func(t *testing.T) {
			info := native.BuildFuncInfo(reg, word)
			if info == nil {
				t.Skipf("no func info for %q (simple def)", word)
				return
			}
			helpText := help.FormatDynamic(*info)

			// Must contain the word name and " — "
			if !strings.Contains(helpText, word+" — ") {
				t.Errorf("missing header for %q", word)
			}

			// Must contain Precedence section
			if !strings.Contains(helpText, "Precedence:") {
				t.Errorf("missing Precedence section for %q", word)
			}

			// Must contain Signatures section
			if !strings.Contains(helpText, "Signatures:") {
				t.Errorf("missing Signatures section for %q", word)
			}

			// Must have at least one signature line with [ ... ]
			if !strings.Contains(helpText, "[ [") && !strings.Contains(helpText, "[ ]") {
				// 0-arg words have [ ] without inner brackets
				if !strings.Contains(helpText, "(none)") {
					t.Errorf("missing signature entries for %q", word)
				}
			}

			// Must contain Description section
			if !strings.Contains(helpText, "Description:") {
				t.Errorf("missing Description section for %q", word)
			}
		})
	}
}

// TestHelpExamplesCorrect extracts dynamically generated examples from
// help output, runs each expression through the engine, and verifies
// the result matches the documented output. Uses in-memory filesystem
// for read/write validation.
func TestHelpExamplesCorrect(t *testing.T) {
	reg, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(reg)
	reg.SetParseFunc(parser.Parse)

	// Enable in-memory filesystem for read/write examples.
	mem := capabilities.NewMem()
	if err := reg.Capabilities.Set(native.CapMemFileOps, capabilities.FileOps(mem)); err != nil {
		t.Fatalf("set capability: %v", err)
	}

	// Seed in-memory files that the generated read examples will access.
	// The help system generates single-letter filenames ('a', 'b', etc.)
	// These are plain text files so read returns their content as a string.
	mem.Files["a"] = []byte("file-a-content")
	mem.Files["b"] = []byte("file-b-content")
	mem.Files["c"] = []byte("file-c-content")
	mem.Files["d"] = []byte("file-d-content")
	mem.Files["e"] = []byte("file-e-content")

	// Set __sys.fs.mem = true in the root context so EffectiveFileOps
	// returns the in-memory filesystem.
	enableMemFS(t, reg)

	// Words with non-deterministic output.
	skipWords := map[string]bool{"module": true}

	words := allRegisteredWords(reg)
	testedCount := 0

	for _, word := range words {
		if skipWords[word] {
			continue
		}
		// Hand-authored examples (Entry.Examples) are curated prose +
		// results, not the `;# <exact-stack-render>` shape this matcher
		// validates — and some are side-effecting or multi-statement.
		// They replace the auto-generated permutations in `describe`, so
		// there is nothing auto-generated left to check here. See
		// §5.2/§9.5 and help.Entry.Examples.
		if e := help.Lookup(word); e != nil && len(e.Examples) > 0 {
			continue
		}
		info := native.BuildFuncInfo(reg, word)
		if info == nil {
			continue
		}
		helpText := help.FormatDynamic(*info)
		examples := extractExamples(helpText)

		// Every rendered example is now checkable: a concrete stack
		// render, `(no value)` for an empty stack, or
		// `error [boru/CODE]` for one the engine refuses. Only the
		// placeholder is not, and placeholderWords is the tracked,
		// shrink-only record of where one still ships.
		var runnable []helpExample
		for _, ex := range examples {
			if ex.expected == examplePlaceholder {
				if !placeholderWords[word] {
					t.Errorf("%s: example %q renders the %q placeholder; "+
						"`describe` output is documented as engine-derived, so either "+
						"make it evaluate or add the word to placeholderWords with a reason",
						word, ex.expr, examplePlaceholder)
				}
				continue
			}
			runnable = append(runnable, ex)
		}
		if len(runnable) == 0 {
			continue
		}

		t.Run(word, func(t *testing.T) {
			for _, ex := range runnable {
				t.Run(ex.expr, func(t *testing.T) {
					vals, err := parser.Parse(ex.expr)
					if err != nil {
						t.Fatalf("parse %q: %v", ex.expr, err)
					}
					// Use a fresh engine per example to avoid state leaks
					eng := native.NewTop(reg)
					result, err := eng.Run(vals)

					// An `error [boru/CODE]` render is a claim about a
					// refusal: the run must fail, and with that code.
					if code, isErr := strings.CutPrefix(ex.expected, "error [boru/"); isErr {
						code = strings.TrimSuffix(code, "]")
						if err == nil {
							t.Fatalf("%s ;# documented as error [boru/%s], but it succeeded with %q",
								ex.expr, code, formatStack(result))
						}
						var be *core.BoruError
						if !errors.As(err, &be) {
							t.Fatalf("%s ;# documented as error [boru/%s], but the failure carries no code: %v",
								ex.expr, code, err)
						}
						if be.Code != code {
							t.Errorf("%s ;# documented as error [boru/%s], got [boru/%s]",
								ex.expr, code, be.Code)
						}
						testedCount++
						return
					}

					if err != nil {
						t.Fatalf("run %q: %v", ex.expr, err)
					}
					got := formatStack(result)
					// An empty stack renders as the marker, not as "".
					if got == "" {
						got = noValueMarker
					}
					if got != ex.expected {
						t.Errorf("%s ;# got %q, want %q", ex.expr, got, ex.expected)
					}
					testedCount++
				})
			}
		})
	}

	t.Logf("validated %d examples across all words", testedCount)
}

// enableMemFS sets __sys.fs.mem = true in the registry's root context.
func enableMemFS(t *testing.T, reg *native.Registry) {
	t.Helper()
	eng := native.New(reg)
	_, err := eng.Run([]native.Value{
		native.NewWord("context"), native.NewWord("dot"), native.NewWord("__sys"),
		native.NewWord("dot"), native.NewWord("fs"),
		native.NewWord("set"), native.NewWord("mem"), native.NewBoolean(true),
	})
	if err != nil {
		t.Fatalf("failed to enable mem fs: %v", err)
	}
}

// allRegisteredWords returns a sorted list of all words in the registry
// that have function definitions (not just simple defs).
func allRegisteredWords(reg *native.Registry) []string {
	// Collect words that have help entries
	helpWords := help.Words()

	// Also collect words from the registry's function table
	// by trying BuildFuncInfo on help words
	seen := map[string]bool{}
	var words []string
	for _, w := range helpWords {
		if !seen[w] {
			seen[w] = true
			words = append(words, w)
		}
	}
	sort.Strings(words)
	return words
}

type helpExample struct {
	expr     string
	expected string
}

// extractExamples parses the Examples section of help output, extracting
// each "expr ;# result" line.
func extractExamples(helpText string) []helpExample {
	var examples []helpExample
	inExamples := false
	for _, line := range strings.Split(helpText, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "Examples:" {
			inExamples = true
			continue
		}
		if inExamples {
			if trimmed == "" || (!strings.HasPrefix(trimmed, "") && strings.HasSuffix(trimmed, ":")) {
				break // End of examples section
			}
			if idx := strings.Index(trimmed, ";#"); idx >= 0 {
				expr := strings.TrimSpace(trimmed[:idx])
				expected := strings.TrimSpace(trimmed[idx+2:])
				// Strip trailing NOTE annotation if present
				if ni := strings.Index(expected, "  NOTE:"); ni >= 0 {
					expected = strings.TrimSpace(expected[:ni])
				}
				if expr != "" && expected != "" {
					examples = append(examples, helpExample{expr: expr, expected: expected})
				}
			}
		}
	}
	return examples
}

// examplePlaceholder is what the Go-side fallback in
// lang/go/native/help renders when the generated map has no entry for an
// expression. It is the one example form this file cannot verify, which
// is why its remaining sites are tracked rather than tolerated.
const examplePlaceholder = "..."

// noValueMarker mirrors cmd/go/genhelp's marker for an example that
// leaves the stack empty.
const noValueMarker = "(no value)"

// placeholderWords records every word that still ships an example
// rendering the `...` placeholder. It is a RATCHET: entries may be
// removed, never added — a new placeholder is a test failure, and the
// fix is to make the example evaluate (see cmd/go/genhelp), not to widen
// this map.
//
// Every entry below has ONE cause, measured 2026-08-18: cmd/go/genhelp
// evaluates each example on a `native.DefaultRegistry` with no imports,
// so a word that lives in a loadable module (`boru:string-util`,
// `boru:bin-util`, `boru:logic-util`, `boru:type-util`, `boru:debug`)
// raises `undefined_word` there and gets no recorded result. That code
// is deliberately NOT documented — the word is defined, it just needs an
// import — so these fall through to the Go-side fallback. Teaching the
// generator to load the modules before evaluating empties this map; see
// design/ROC-ADOPTION-PLAN.0.md (A4).
//
// `roll` is the one exception and has a different cause: `describe`
// renders `2 roll`, but the generator records nothing for it, so the
// display expression and the generated expression diverge somewhere
// between help.ExampleExprs and writeExamples.
var placeholderWords = map[string]bool{
	"roll":         true,
	"band":         true,
	"bnot":         true,
	"bor":          true,
	"bsl":          true,
	"bsr":          true,
	"busr":         true,
	"bxor":         true,
	"changecase":   true,
	"concat":       true,
	"contains":     true,
	"create":       true,
	"escape":       true,
	"fnsig":        true,
	"forward-args": true,
	"iff":          true,
	"implies":      true,
	"indexof":      true,
	"lower":        true,
	"match":        true,
	"nand":         true,
	"nor":          true,
	"normalize":    true,
	"pad":          true,
	"pick":         true,
	"printstr":     true,
	"ref":          true,
	"remove":       true,
	"repeat":       true,
	"replace":      true,
	"split":        true,
	"stack":        true,
	"stack-args":   true,
	"tpartial":     true,
	"trace":        true,
	"trim":         true,
	"update":       true,
	"upper":        true,
	"xnor":         true,
}
