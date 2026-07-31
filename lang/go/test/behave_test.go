package test

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/boru-lang/boru/eng/go"
	"github.com/boru-lang/boru/eng/go/parser"
	"github.com/boru-lang/boru/lang/go/modules"
	"github.com/boru-lang/boru/lang/go/native"
)

// TestBehave runs the behavior-dispatch spec at behave.tsv. Each row
// exercises one of the kernel capabilities wired through `reg`:
// kernel scalar Comparers / Formatters / Jsonifiers, lang-layer
// native variants (Date, Instant, ClockDuration), and user-defined
// behaviors installed via `reg compare/q | canon/q | jsonify/q`.
//
// The native rows use the `boru:time` module, so the runner wires
// `modules.Resolve` and pre-installs the time exports
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
			registerIOWords(reg)
			if err != nil {
				t.Fatal(err)
			}
			reg.SetParseFunc(parser.Parse)
			reg.Modules.Resolver = modules.Resolve
			// Pre-install the boru:time module so spec rows can use
			// `TimeUtil.unix`, `TimeUtil.seconds`, … without the
			// `import "boru:time-util"` boilerplate on every native row.
			if err := modules.InstallTimeExports(reg); err != nil {
				t.Fatalf("install time exports: %v", err)
			}
			// nodify moved to boru:struct; pre-install Struct so spec rows can
			// use `StructUtil.nodify` (the projection word) without import
			// boilerplate. The `nodify` *behavior* name (`behave nodify/q …`)
			// is unaffected — it is a quoted atom, not the word.
			if err := modules.InstallStructExports(reg); err != nil {
				t.Fatalf("install struct exports: %v", err)
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
