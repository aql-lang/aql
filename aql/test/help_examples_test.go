package test

import (
	"strings"
	"testing"

	"github.com/metsitaba/voxgig-exp/aql/internal/engine"
	"github.com/metsitaba/voxgig-exp/aql/internal/engine/help"
	"github.com/metsitaba/voxgig-exp/aql/internal/native"
	"github.com/metsitaba/voxgig-exp/aql/internal/parser"
)

// TestHelpExamplesCorrect extracts the dynamically generated examples from
// help output for add and sub, runs each expression through the engine, and
// verifies the result matches the documented output.
func TestHelpExamplesCorrect(t *testing.T) {
	reg, err := engine.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	native.Register(reg)

	for _, word := range []string{"add", "sub"} {
		t.Run(word, func(t *testing.T) {
			info := engine.BuildFuncInfo(reg, word)
			if info == nil {
				t.Fatalf("no func info for %q", word)
			}
			helpText := help.FormatDynamic(*info)

			examples := extractExamples(helpText)
			if len(examples) == 0 {
				t.Fatalf("no examples found in help for %q", word)
			}

			for _, ex := range examples {
				t.Run(ex.expr, func(t *testing.T) {
					vals, err := parser.Parse(ex.expr)
					if err != nil {
						t.Fatalf("parse %q: %v", ex.expr, err)
					}
					eng := engine.NewTop(reg)
					result, err := eng.Run(vals)
					if err != nil {
						t.Fatalf("run %q: %v", ex.expr, err)
					}
					got := formatStack(result)
					if got != ex.expected {
						t.Errorf("%s ;# got %q, want %q", ex.expr, got, ex.expected)
					}
				})
			}
		})
	}
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
				if expr != "" && expected != "" {
					examples = append(examples, helpExample{expr: expr, expected: expected})
				}
			}
		}
	}
	return examples
}
