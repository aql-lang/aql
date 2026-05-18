package test

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/aql-lang/aql/eng/go/parser"
	"github.com/aql-lang/aql/lang/go/engine"
	"github.com/aql-lang/aql/lang/go/native"
)

// TestBoolean runs every line of boolean.tsv as a parse+run+compare
// test. Format mirrors syntax.tsv:
//
//	<expr>\t<expected>[\t<error-code>]
func TestBoolean(t *testing.T) {
	f, err := os.Open("boolean.tsv")
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
		errorCode := ""
		if len(parts) > 2 {
			errorCode = parts[2]
		}

		ran++
		t.Run(fmt.Sprintf("L%d_%s", lineNum, expr), func(t *testing.T) {
			values, err := parser.Parse(expr)
			if err != nil {
				if errorCode != "" {
					checkErrorCode(t, expr, err, errorCode)
					return
				}
				t.Fatalf("parse error: %v", err)
			}

			reg, err := engine.DefaultRegistry(native.Register)
			if err != nil {
				t.Fatal(err)
			}
			eng := engine.NewTop(reg)
			result, err := eng.Run(values)

			if errorCode != "" {
				if err == nil {
					t.Errorf("\n  expr: %s\n  expected error %q but got result: %s",
						expr, errorCode, formatStack(result))
					return
				}
				checkErrorCode(t, expr, err, errorCode)
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
		t.Fatal("no test cases found in boolean.tsv")
	}

	t.Logf("ran %d boolean test cases", ran)
}
