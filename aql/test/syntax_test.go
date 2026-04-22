package test

import (
	"github.com/metsitaba/voxgig-exp/aql/internal/native"
	"bufio"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/metsitaba/voxgig-exp/aql/internal/engine"
	"github.com/metsitaba/voxgig-exp/aql/internal/parser"
)

// errorPatterns maps error codes used in syntax.tsv to substrings that must
// appear in the actual error message.
var errorPatterns = map[string]string{
	"syntax_error":     "syntax_error",
	"signature_error":  "signature_error",
	"division_by_zero": "division by zero",
	"modulo_by_zero":   "modulo by zero",
	"undefined_word":   "undefined_word",
}

func TestSyntax(t *testing.T) {
	f, err := os.Open("syntax.tsv")
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
				// We expect an error.
				if err == nil {
					t.Errorf("\n  expr: %s\n  expected error %q but got result: %s",
						expr, errorCode, formatStack(result))
					return
				}
				checkErrorCode(t, expr, err, errorCode)
				return
			}

			// We expect success.
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
		t.Fatal("no test cases found in syntax.tsv")
	}

	t.Logf("ran %d syntax test cases", ran)
}

// checkErrorCode verifies that the error message contains the expected pattern.
func checkErrorCode(t *testing.T, expr string, err error, code string) {
	t.Helper()
	pattern, ok := errorPatterns[code]
	if !ok {
		t.Errorf("\n  expr: %s\n  unknown error code %q", expr, code)
		return
	}
	if !strings.Contains(err.Error(), pattern) {
		t.Errorf("\n  expr: %s\n  error: %v\n  expected error containing %q", expr, err, pattern)
	}
}
