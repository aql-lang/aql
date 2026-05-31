package modules

// Stage-0 equivalence coverage for the MODULE-WRAPPER dispatch path of
// the function-model consolidation refactor.
//
// The native-package harness cannot reach module wrappers because
// DefaultRegistry has no native-module resolver. Here we reuse the
// proven mathRegistry helper (BuildMathModule + exports installed as
// defs, exactly what the import handler does) and dispatch the
// paren-wrapped `arg ( math get NAME )` form — the same form the
// TestMathDot* tests use — which flows through execFnDefLiteral's
// captured-sub-registry branch (trivial-delegation short-circuit /
// CallAQL). The golden pins this byte-for-byte across refactor stages.
//
// Regenerate: go test ./modules -run TestFnModelWrapperEquivalence -update-wrapper

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aql-lang/aql/lang/go/native"
)

var updateWrapperGolden = flag.Bool("update-wrapper", false, "update the wrapper-equivalence golden file")

func wrapGet(arg native.Value, word string) []native.Value {
	return []native.Value{
		arg,
		native.NewOpenParen(),
		native.NewWord("math"), native.NewWord("get"), native.NewWord(word),
		native.NewCloseParen(),
	}
}

var wrapperCases = []struct {
	name   string
	tokens []native.Value
}{
	{"abs", wrapGet(native.NewInteger(-5), "abs")},
	{"sign", wrapGet(native.NewInteger(-3), "sign")},
	{"negate", wrapGet(native.NewInteger(4), "negate")},
	{"ceil", wrapGet(native.NewDecimal(2.3), "ceil")},
	{"floor", wrapGet(native.NewDecimal(2.7), "floor")},
}

func TestFnModelWrapperEquivalence(t *testing.T) {
	var b strings.Builder
	for _, c := range wrapperCases {
		r := mathRegistry(t)
		toks := make([]native.Value, len(c.tokens))
		copy(toks, c.tokens)
		out, err := native.New(r).Run(toks)
		var got string
		if err != nil {
			got = "RUNERR: " + strings.SplitN(err.Error(), "\n", 2)[0]
		} else {
			parts := make([]string, len(out))
			for i, v := range out {
				parts[i] = v.String()
			}
			got = strings.Join(parts, " ")
		}
		fmt.Fprintf(&b, "CASE math.%s\n  => %q\n", c.name, got)
	}

	goldenPath := filepath.Join("testdata", "fnmodel_wrapper_equivalence.golden")
	if *updateWrapperGolden {
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatalf("mkdir testdata: %v", err)
		}
		if err := os.WriteFile(goldenPath, []byte(b.String()), 0o644); err != nil {
			t.Fatalf("write golden: %v", err)
		}
		t.Logf("updated golden: %s", goldenPath)
		return
	}
	want, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("read golden (run with -update-wrapper to create): %v", err)
	}
	if b.String() != string(want) {
		t.Errorf("wrapper equivalence drift:\nWANT:\n%s\nGOT:\n%s", string(want), b.String())
	}
}
