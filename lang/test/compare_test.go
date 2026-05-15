package test

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/aql-lang/aql/eng"
	"github.com/aql-lang/aql/eng/parser"
	"github.com/aql-lang/aql/lang/engine"
	"github.com/aql-lang/aql/lang/internal/nativemod"
	"github.com/aql-lang/aql/lang/native"
)

// TestCompare runs the compare-dispatch spec at compare.tsv. Each row
// exercises one of three layers wired through eng.CompareValues:
// kernel scalar Comparers, lang-layer native Comparers (Date /
// DateTime / Instant / TimeOfDay), and user-defined Comparers
// installed via the `cmp [Type/q List]` word.
//
// The native rows use `"aql:time" import` to construct domain values,
// so the runner wires `nativemod.Resolve` as the module resolver
// (langspec doesn't, since it can't import the lang-internal nativemod
// package across module boundaries — this runner lives inside the
// lang module so the wiring is local).
func TestCompare(t *testing.T) {
	f, err := os.Open("compare.tsv")
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
			reg, err := engine.DefaultRegistry(native.Register)
			if err != nil {
				t.Fatal(err)
			}
			reg.SetParseFunc(parser.Parse)
			reg.Modules.Resolver = nativemod.Resolve
			// Pre-install the aql:time module so spec rows can use
			// `time.unix`, `time.seconds`, … without the
			// `"aql:time" import` boilerplate on every native row.
			if err := nativemod.InstallTimeExports(reg); err != nil {
				t.Fatalf("install time exports: %v", err)
			}

			values, err := parser.Parse(expr)
			if err != nil {
				t.Fatalf("parse error: %v", err)
			}
			result, err := engine.NewTop(reg).Run(values)

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
		t.Fatal("no test cases found in compare.tsv")
	}
	t.Logf("ran %d compare rows", ran)
}

func sanitiseName(s string) string {
	s = strings.ReplaceAll(s, " ", "_")
	if len(s) > 40 {
		s = s[:40]
	}
	return s
}
