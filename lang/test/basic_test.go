package test

import (
	"bufio"
	"fmt"
	"github.com/aql-lang/aql/lang/native"
	"os"
	"strings"
	"testing"

	"github.com/aql-lang/aql/eng/parser"
	"github.com/aql-lang/aql/lang/engine"
)

func TestBasic(t *testing.T) {
	f, err := os.Open("basic.tsv")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	lineNum := 0
	ran := 0

	for scanner.Scan() {
		lineNum++
		line := scanner.Text()

		// Skip empty lines and comments.
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.SplitN(line, "\t", 2)
		expr := parts[0]
		expected := ""
		if len(parts) > 1 {
			expected = parts[1]
		}

		ran++
		t.Run(fmt.Sprintf("L%d_%s", lineNum, expr), func(t *testing.T) {
			// Parse the expression into engine values.
			values, err := parser.Parse(expr)
			if err != nil {
				t.Fatalf("parse error: %v", err)
			}

			// Run through the engine with a fresh registry.
			reg, err := engine.DefaultRegistry(native.Register)
			if err != nil {
				t.Fatal(err)
			}
			eng := engine.NewTop(reg)
			result, err := eng.Run(values)

			// Expected error: "ERROR:substring"
			if strings.HasPrefix(expected, "ERROR:") {
				errSubstr := expected[len("ERROR:"):]
				if err == nil {
					t.Errorf("\n  expr: %s\n  expected error containing %q but got result: %s",
						expr, errSubstr, formatStack(result))
					return
				}
				if !strings.Contains(err.Error(), errSubstr) {
					t.Errorf("\n  expr: %s\n  error: %v\n  expected error containing %q",
						expr, err, errSubstr)
				}
				return
			}

			if err != nil {
				t.Fatalf("engine error: %v", err)
			}

			got := formatStack(result)
			if got != expected {
				t.Errorf("\n  expr: %s\n  got:  %q\n  want: %q", expr, got, expected)
			}
		})
	}

	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}

	if ran == 0 {
		t.Fatal("no test cases found in basic.tsv")
	}

	t.Logf("ran %d test cases", ran)
}

// formatStack converts a result stack to a string for comparison.
// Each value uses Value.String(), joined by spaces.
func formatStack(values []engine.Value) string {
	parts := make([]string, len(values))
	for i, v := range values {
		parts[i] = v.String()
	}
	return strings.Join(parts, " ")
}
