package test

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/aql-lang/aql/eng/go"
	"github.com/aql-lang/aql/eng/go/parser"
	"github.com/aql-lang/aql/lang/go/modules"
	"github.com/aql-lang/aql/lang/go/native"
)

// TestSigOrder runs the signature-matcher ordering spec at sigorder.tsv.
// Each row exercises one shape of the reversed-lattice sig sort —
// user-defined scalar / node / object subtypes, Go-defined Ideals
// (Tensor/Matrix/Vector from aql:matrix), and pattern-driven dispatch.
// The runner installs the matrix module so rows can reference Matrix /
// Tensor / Vector without a per-row `"aql:matrix" import`.
//
// The unit-test counterpart for user-defined comparators
// (`behave compare/q`-style custom Comparer on a type) lives in
// signature_user_compare_test.go — fn syntax can't synthesise patterns
// of arbitrary user types, so that case is exercised at the Go API
// directly.
func TestSigOrder(t *testing.T) {
	f, err := os.Open("sigorder.tsv")
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
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.SplitN(line, "\t", 3)
		expr := parts[0]
		expected := ""
		if len(parts) > 1 {
			expected = parts[1]
		}

		ran++
		t.Run(fmt.Sprintf("L%d_%s", lineNum, sanitiseName(expr)), func(t *testing.T) {
			reg, err := native.DefaultRegistry()
			if err != nil {
				t.Fatal(err)
			}
			reg.SetParseFunc(parser.Parse)
			reg.Modules.Resolver = modules.Resolve
			if err := modules.InstallMatrixExports(reg); err != nil {
				t.Fatalf("install matrix exports: %v", err)
			}

			values, err := parser.Parse(expr)
			if err != nil {
				t.Fatalf("parse error: %v", err)
			}
			result, err := native.NewTop(reg).Run(values)

			if strings.HasPrefix(expected, "ERROR:") {
				want := strings.TrimPrefix(expected, "ERROR:")
				if err == nil {
					t.Errorf("\n  expr: %s\n  expected error containing %q but got: %s",
						expr, want, formatStack(result))
					return
				}
				if want != "" && !strings.Contains(err.Error(), want) {
					t.Errorf("\n  expr: %s\n  error: %v\n  expected substring %q",
						expr, err, want)
				}
				return
			}

			if err != nil {
				t.Fatalf("engine error: %v", err)
			}
			got := eng.Canon(result)
			if got != expected {
				t.Errorf("\n  expr: %s\n  got:  %q\n  want: %q", expr, got, expected)
			}
		})
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	if ran == 0 {
		t.Fatal("no test cases found in sigorder.tsv")
	}
	t.Logf("ran %d sigorder rows", ran)
}
