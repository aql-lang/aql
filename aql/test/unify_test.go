package test

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/metsitaba/voxgig-exp/aql/internal/engine"
	"github.com/metsitaba/voxgig-exp/aql/internal/parser"
)

func TestUnify(t *testing.T) {
	f, err := os.Open("unify.tsv")
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

		parts := strings.Split(line, "\t")
		if len(parts) < 3 {
			t.Fatalf("line %d: expected at least 3 tab-separated columns, got %d", lineNum, len(parts))
		}

		leftExpr := parts[0]
		rightExpr := parts[1]
		expectedBool := parts[2]
		expectedOut := ""
		if len(parts) > 3 {
			expectedOut = parts[3]
		}

		ran++
		t.Run(fmt.Sprintf("L%d_%s_unify_%s", lineNum, leftExpr, rightExpr), func(t *testing.T) {
			// Evaluate left expression.
			leftVal, err := evalSingle(leftExpr)
			if err != nil {
				t.Fatalf("left eval error: %v", err)
			}

			// Evaluate right expression.
			rightVal, err := evalSingle(rightExpr)
			if err != nil {
				t.Fatalf("right eval error: %v", err)
			}

			// Build and run the unify expression: left unify right
			eng := engine.NewTop(engine.DefaultRegistry())
			result, err := eng.Run([]engine.Value{leftVal, engine.NewWord("unify"), rightVal})
			if err != nil {
				t.Fatalf("engine error: %v", err)
			}

			if len(result) < 2 {
				t.Fatalf("expected at least 2 values on stack, got %d: %v", len(result), result)
			}

			// The stack should be [unified_value, boolean].
			gotBool := result[len(result)-1].String()
			if gotBool != expectedBool {
				t.Errorf("\n  left:  %s\n  right: %s\n  bool got:  %q\n  bool want: %q", leftExpr, rightExpr, gotBool, expectedBool)
			}

			// Check out value only when unification succeeds.
			if expectedBool == "true" && expectedOut != "" {
				gotOut := result[0].String()
				if gotOut != expectedOut {
					t.Errorf("\n  left:  %s\n  right: %s\n  out got:  %q\n  out want: %q", leftExpr, rightExpr, gotOut, expectedOut)
				}
			}
		})
	}

	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}

	if ran == 0 {
		t.Fatal("no test cases found in unify.tsv")
	}

	t.Logf("ran %d unify test cases", ran)
}

// evalSingle parses and evaluates an AQL expression, returning the single
// result value. It fails if the expression produces zero or more than one value.
func evalSingle(expr string) (engine.Value, error) {
	values, err := parser.Parse(expr)
	if err != nil {
		return engine.Value{}, fmt.Errorf("parse %q: %w", expr, err)
	}

	eng := engine.NewTop(engine.DefaultRegistry())
	result, err := eng.Run(values)
	if err != nil {
		return engine.Value{}, fmt.Errorf("run %q: %w", expr, err)
	}

	if len(result) != 1 {
		return engine.Value{}, fmt.Errorf("expression %q produced %d values, expected 1", expr, len(result))
	}

	return result[0], nil
}
