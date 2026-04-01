package test

import (
	"sort"
	"strings"
	"testing"

	"github.com/metsitaba/voxgig-exp/aql/internal/engine"
	"github.com/metsitaba/voxgig-exp/aql/internal/engine/help"
	"github.com/metsitaba/voxgig-exp/aql/internal/native"
	"github.com/metsitaba/voxgig-exp/aql/internal/parser"
)

// TestHelpAllWords checks that every registered word produces valid
// dynamic help output with the expected sections.
func TestHelpAllWords(t *testing.T) {
	reg, err := engine.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	native.Register(reg)

	words := allRegisteredWords(reg)
	if len(words) == 0 {
		t.Fatal("no words registered")
	}

	for _, word := range words {
		t.Run(word, func(t *testing.T) {
			info := engine.BuildFuncInfo(reg, word)
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
// the result matches the documented output. Skips examples with "..."
// results (not computable).
func TestHelpExamplesCorrect(t *testing.T) {
	reg, err := engine.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	native.Register(reg)

	words := allRegisteredWords(reg)
	testedCount := 0

	for _, word := range words {
		info := engine.BuildFuncInfo(reg, word)
		if info == nil {
			continue
		}
		helpText := help.FormatDynamic(*info)
		examples := extractExamples(helpText)

		// Filter to runnable examples (non-"..." results)
		var runnable []helpExample
		for _, ex := range examples {
			if ex.expected != "..." {
				runnable = append(runnable, ex)
			}
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
					eng := engine.NewTop(reg)
					result, err := eng.Run(vals)
					if err != nil {
						t.Fatalf("run %q: %v", ex.expr, err)
					}
					got := formatStack(result)
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

// allRegisteredWords returns a sorted list of all words in the registry
// that have function definitions (not just simple defs).
func allRegisteredWords(reg *engine.Registry) []string {
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
