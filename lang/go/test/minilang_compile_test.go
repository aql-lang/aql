package test

import (
	"strings"
	"testing"

	lang "github.com/aql-lang/aql/lang/go"
	"github.com/aql-lang/aql/lang/go/native"
)

// Compiled mini-languages (design/MINILANG.5.md §13). A kind may register an
// expansion-time compile hook alongside its runtime transducer; when src is
// concrete, `mini` runs the hook at the call site and splices its tokens
// instead of the standard call. The transducer stays the semantic reference,
// the check-mode target, and the non-concrete-src fallback.

const mcImp = `import "aql:minilang"  `

func mcRun(t *testing.T, src string) any {
	t.Helper()
	a, err := lang.New()
	if err != nil {
		t.Fatalf("lang.New: %v", err)
	}
	res, err := a.Run(src)
	if err != nil {
		t.Fatalf("Run(%q): %v", src, err)
	}
	if len(res) != 1 {
		t.Fatalf("Run(%q): expected 1 result, got %d: %v", src, len(res), res)
	}
	return res[0]
}

// TestMiniCompileReCarrier: the built-in `re` kind now compiles the pattern at
// the call site into a carrier consumed by run-re — results unchanged.
func TestMiniCompileReCarrier(t *testing.T) {
	if got := mcRun(t, mcImp+`("AbcD" mini re '[a-z]+').fst.m`); got != "bc" {
		t.Errorf("compiled re match = %v, want bc", got)
	}
	if got := mcRun(t, mcImp+`("a1b2c3" mini re '\\d').n`); got != int64(3) {
		t.Errorf("compiled re count = %v, want 3", got)
	}
	if got := mcRun(t, mcImp+`("a1b2c3" mini re '\\d' {limit:2}).n`); got != int64(2) {
		t.Errorf("compiled re opts.limit = %v, want 2", got)
	}
	// The +re/…/ literal desugars to mini re and hits the same compiled path.
	if got := mcRun(t, mcImp+`("AbcD" +re/[a-z]+/).fst.m`); got != "bc" {
		t.Errorf("+re literal compiled = %v, want bc", got)
	}
}

// TestMiniCompileReBadPattern: a malformed pattern still raises mini_parse_error
// — the hook defers to the transducer when it can't build a carrier, so the
// error is identical to (and parity-safe with) the standard call.
func TestMiniCompileReBadPattern(t *testing.T) {
	a, _ := lang.New()
	if _, err := a.Run(mcImp + `"x" mini re '('`); err == nil {
		t.Fatal("expected mini_parse_error for a bad pattern")
	} else if !strings.Contains(err.Error(), "mini_parse_error") {
		t.Fatalf("error = %v, want mini_parse_error", err)
	}
}

// TestMiniCompileGoHostHook: the Go host API's Compile field runs at the call
// site. `up`'s transducer returns src; its compile hook uppercases at compile.
func TestMiniCompileGoHostHook(t *testing.T) {
	a, err := lang.New()
	if err != nil {
		t.Fatalf("lang.New: %v", err)
	}
	err = a.RegisterMiniLang(lang.MiniLangSpec{
		Name:    "up",
		Returns: []*lang.Type{lang.TString},
		Handler: func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
			s, _ := args[0].AsConcreteString()
			return []native.Value{native.NewString(s)}, nil // transducer: identity
		},
		Compile: func(src string, _ native.Value, _ *native.Registry) ([]native.Value, error) {
			// compile-time: splice the uppercased src as an inert constant
			return []native.Value{native.NewString(strings.ToUpper(src))}, nil
		},
	})
	if err != nil {
		t.Fatalf("RegisterMiniLang: %v", err)
	}
	if out, err := a.Run(mcImp + `mini up 'hi'`); err != nil {
		t.Fatalf("run: %v", err)
	} else if len(out) != 1 || out[0] != "HI" {
		t.Fatalf("mini up 'hi' = %v, want HI (compile hook ran)", out)
	}
}

// TestMiniCompileAQLHook: an AQL compile hook (a macro) installed via
// register-compiled runs at the call site. `lit`'s transducer returns src; its
// compiler uppercases at expansion via unquote.
func TestMiniCompileAQLHook(t *testing.T) {
	setup := mcImp + `import "aql:string-util"  ` +
		`MiniLang.register lit (fn [[src:String opts:Map] [String] [src]]) end  ` +
		`MiniLang.register-compiled lit (macro [[src opts] [ quote [ unquote (StringUtil.upper src) ] ]]) end  `
	if got := mcRun(t, setup+`mini lit 'hello'`); got != "HELLO" {
		t.Errorf("AQL-compiled lit = %v, want HELLO", got)
	}
}

// --- negatives ------------------------------------------------------------

// TestMiniCompileRegisterCompiledContract pins what register-compiled refuses.
func TestMiniCompileRegisterCompiledContract(t *testing.T) {
	cases := []struct{ name, prog, want string }{
		{
			"no transducer",
			mcImp + `MiniLang.register-compiled ghost (macro [[s o] [ quote [ s ] ]])`,
			"mini_no_transducer",
		},
		{
			"compiler must be a macro",
			mcImp + `MiniLang.register lit (fn [[s:String o:Map] [String] [s]]) end  ` +
				`MiniLang.register-compiled lit (fn [[s:String o:Map] [List] [ [s] ]])`,
			"mini_bad_compiler",
		},
		{
			"bad name",
			mcImp + `MiniLang.register-compiled lang_x (macro [[s o] [ quote [ s ] ]])`,
			"mini_bad_name",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			a, _ := lang.New()
			if _, err := a.Run(c.prog); err == nil {
				t.Fatalf("%s: expected error", c.name)
			} else if !strings.Contains(err.Error(), c.want) {
				t.Fatalf("%s: error %q does not contain %q", c.name, err.Error(), c.want)
			}
		})
	}
}
