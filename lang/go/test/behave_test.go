package test

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/aql-lang/aql/eng/go"
	"github.com/aql-lang/aql/eng/go/parser"
	"github.com/aql-lang/aql/lang/go/internal/nativemod"
	"github.com/aql-lang/aql/lang/go/native"
)

// TestBehave runs the behavior-dispatch spec at behave.tsv. Each row
// exercises one of the kernel capabilities wired through `reg`:
// kernel scalar Comparers / Formatters / Jsonifiers, lang-layer
// native variants (Date, Instant, ClkDuration), and user-defined
// behaviors installed via `reg compare/q | canon/q | jsonify/q`.
//
// The native rows use the `aql:time` module, so the runner wires
// `nativemod.Resolve` and pre-installs the time exports
// (langspec doesn't, since it can't import lang-internal nativemod
// across module boundaries — this runner lives inside the lang
// module so the wiring is local).
func TestBehave(t *testing.T) {
	f, err := os.Open("behave.tsv")
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
		t.Fatal("no test cases found in behave.tsv")
	}
	t.Logf("ran %d behave rows", ran)
}

func sanitiseName(s string) string {
	s = strings.ReplaceAll(s, " ", "_")
	if len(s) > 40 {
		s = s[:40]
	}
	return s
}
