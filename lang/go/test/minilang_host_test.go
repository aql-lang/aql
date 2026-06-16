package test

import (
	"strings"
	"testing"

	lang "github.com/aql-lang/aql/lang/go"
	"github.com/aql-lang/aql/lang/go/native"
)

// The calc mini-language exercises the Go host registration API
// (lang.RegisterMiniLang). `iop` is an integer binary operation: src is
// "<a> <op> <b>", a and b name keys in the opts map, and op is one of
// + - * / %. It is a generator kind (no stack input — operands come from
// opts), so its standard call shape is [src:String opts:Map] -> [Integer].

// calcSpec builds the iop kind. Implemented entirely in Go — the point of
// the host API is that a kind needs no AQL source and no fork of the core
// minilang module.
func calcSpec() lang.MiniLangSpec {
	return lang.MiniLangSpec{
		Name:    "iop",
		Returns: []*lang.Type{lang.TInteger},
		Handler: func(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
			src, err := args[0].AsConcreteString()
			if err != nil {
				return nil, r.AqlError("mini_error", "iop: src: "+err.Error(), "lang_iop")
			}
			m, merr := native.AsMap(args[1])
			if merr != nil || m == nil {
				return nil, r.AqlError("mini_error", "iop: opts must be a map", "lang_iop")
			}
			parts := strings.Fields(src)
			if len(parts) != 3 {
				return nil, r.AqlErrorHint("mini_parse_error",
					"iop: expected '<a> <op> <b>', got "+src, "lang_iop",
					"write a binary expression like 'x + y'")
			}
			a, err := calcOperand(r, m, parts[0])
			if err != nil {
				return nil, err
			}
			b, err := calcOperand(r, m, parts[2])
			if err != nil {
				return nil, err
			}
			var out int64
			switch parts[1] {
			case "+":
				out = a + b
			case "-":
				out = a - b
			case "*":
				out = a * b
			case "/", "%":
				if b == 0 {
					return nil, r.AqlError("mini_eval_error", "iop: division by zero", "lang_iop")
				}
				if parts[1] == "/" {
					out = a / b
				} else {
					out = a % b
				}
			default:
				return nil, r.AqlErrorHint("mini_parse_error",
					"iop: unknown operator "+parts[1], "lang_iop", "use one of + - * / %")
			}
			return []native.Value{native.NewInteger(out)}, nil
		},
	}
}

// calcOperand resolves a variable name to its integer value in opts.
func calcOperand(r *native.Registry, m native.ReadMap, name string) (int64, error) {
	v, ok := m.Get(name)
	if !ok {
		return 0, r.AqlError("mini_eval_error", "iop: undefined variable "+name, "lang_iop")
	}
	n, err := v.AsConcreteInteger()
	if err != nil {
		return 0, r.AqlError("mini_error", "iop: "+name+": "+err.Error(), "lang_iop")
	}
	return n, nil
}

// newCalcInstance builds an AQL instance with the iop kind registered.
func newCalcInstance(t *testing.T) *lang.AQL {
	t.Helper()
	a, err := lang.New()
	if err != nil {
		t.Fatalf("lang.New: %v", err)
	}
	if err := a.RegisterMiniLang(calcSpec()); err != nil {
		t.Fatalf("RegisterMiniLang: %v", err)
	}
	return a
}

// runLast runs src and returns the single residual value, failing on error
// or on an unexpected stack shape.
func runLast(t *testing.T, a *lang.AQL, src string) any {
	t.Helper()
	res, err := a.Run(src)
	if err != nil {
		t.Fatalf("Run(%q): %v", src, err)
	}
	if len(res) != 1 {
		t.Fatalf("Run(%q): expected 1 result, got %d: %v", src, len(res), res)
	}
	return res[0]
}

// TestMiniLangHostCalcOperators pins every operator's result through a
// `mini iop` call site — registration BEFORE import.
func TestMiniLangHostCalcOperators(t *testing.T) {
	a := newCalcInstance(t)
	cases := []struct {
		expr string
		opts string
		want int64
	}{
		{"x + y", "{x:10, y:2}", 12},
		{"x - y", "{x:10, y:2}", 8},
		{"x * y", "{x:10, y:2}", 20},
		{"x / y", "{x:10, y:2}", 5},
		{"x % y", "{x:10, y:3}", 1},
		// operand names are arbitrary opts keys, not fixed to x/y
		{"a - b", "{a:3, b:9}", -6},
	}
	for _, c := range cases {
		src := `"aql:minilang" import end  mini iop '` + c.expr + `' ` + c.opts
		got := runLast(t, a, src)
		if got != c.want {
			t.Errorf("mini iop '%s' %s = %v, want %d", c.expr, c.opts, got, c.want)
		}
	}
}

// TestMiniLangHostDesugarEquivalence proves `mini iop` is pure sugar for the
// standard MiniLang.lang_iop call — both forms must agree.
func TestMiniLangHostDesugarEquivalence(t *testing.T) {
	a := newCalcInstance(t)
	sugar := runLast(t, a, `"aql:minilang" import end  mini iop 'x * y' {x:6, y:7}`)
	desugared := runLast(t, a, `"aql:minilang" import end  MiniLang.lang_iop 'x * y' {x:6, y:7} end`)
	if sugar != desugared {
		t.Fatalf("sugar=%v desugared=%v: mini must desugar to the standard call", sugar, desugared)
	}
	if sugar != int64(42) {
		t.Fatalf("got %v, want 42", sugar)
	}
}

// TestMiniLangHostRegisterAfterImport covers the post-import registration
// path: the module is already built and cached, so the kind must be injected
// live rather than wait for a re-import the loaded cache never triggers.
func TestMiniLangHostRegisterAfterImport(t *testing.T) {
	a, err := lang.New()
	if err != nil {
		t.Fatalf("lang.New: %v", err)
	}
	// Import first — this builds and caches the minilang module.
	if _, err := a.Run(`"aql:minilang" import end`); err != nil {
		t.Fatalf("import: %v", err)
	}
	// Register AFTER the import.
	if err := a.RegisterMiniLang(calcSpec()); err != nil {
		t.Fatalf("RegisterMiniLang after import: %v", err)
	}
	got := runLast(t, a, `mini iop 'x + y' {x:40, y:2}`)
	if got != int64(42) {
		t.Fatalf("post-import registration: got %v, want 42", got)
	}
}

// TestMiniLangHostIsolation confirms a kind registered on one instance does
// not leak into another — the registration is per-registry, not global.
func TestMiniLangHostIsolation(t *testing.T) {
	a := newCalcInstance(t) // has iop
	b, err := lang.New()    // does NOT
	if err != nil {
		t.Fatalf("lang.New: %v", err)
	}
	// a resolves iop.
	if got := runLast(t, a, `"aql:minilang" import end  mini iop 'x + y' {x:1, y:1}`); got != int64(2) {
		t.Fatalf("instance a: got %v, want 2", got)
	}
	// b must NOT — unknown kind is an expansion-time error.
	if _, err := b.Run(`"aql:minilang" import end  mini iop 'x + y' {x:1, y:1}`); err == nil {
		t.Fatal("instance b: expected mini_unknown_lang, got nil error")
	} else if !strings.Contains(err.Error(), "iop") {
		t.Fatalf("instance b: error should name the unknown kind, got: %v", err)
	}
}

// --- negative coverage: the contract is what gets refused -----------------

// TestMiniLangHostRuntimeErrors pins the handler's loud failures.
func TestMiniLangHostRuntimeErrors(t *testing.T) {
	a := newCalcInstance(t)
	cases := []struct {
		name string
		src  string
		want string
	}{
		{"unknown operator", `"aql:minilang" import end  mini iop 'x ^ y' {x:1, y:2}`, "unknown operator"},
		{"division by zero", `"aql:minilang" import end  mini iop 'x / y' {x:1, y:0}`, "division by zero"},
		{"modulo by zero", `"aql:minilang" import end  mini iop 'x % y' {x:1, y:0}`, "division by zero"},
		{"undefined variable", `"aql:minilang" import end  mini iop 'x + z' {x:1, y:2}`, "undefined variable"},
		{"malformed source", `"aql:minilang" import end  mini iop 'x +' {x:1}`, "expected"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := a.Run(c.src)
			if err == nil {
				t.Fatalf("%s: expected error, got nil", c.name)
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Fatalf("%s: error %q does not contain %q", c.name, err.Error(), c.want)
			}
		})
	}
}

// TestMiniLangHostRegistrationContract pins what RegisterMiniLang refuses:
// bad names, nil handlers, duplicate registration, and built-in collisions.
func TestMiniLangHostRegistrationContract(t *testing.T) {
	t.Run("empty name", func(t *testing.T) {
		a, _ := lang.New()
		if err := a.RegisterMiniLang(lang.MiniLangSpec{Handler: calcSpec().Handler}); err == nil {
			t.Fatal("expected error for empty name")
		}
	})
	t.Run("lang_ prefix rejected", func(t *testing.T) {
		a, _ := lang.New()
		spec := calcSpec()
		spec.Name = "lang_iop"
		if err := a.RegisterMiniLang(spec); err == nil {
			t.Fatal("expected error for lang_ prefix")
		}
	})
	t.Run("capitalised name rejected", func(t *testing.T) {
		a, _ := lang.New()
		spec := calcSpec()
		spec.Name = "Iop"
		if err := a.RegisterMiniLang(spec); err == nil {
			t.Fatal("expected error for capitalised name")
		}
	})
	t.Run("nil handler rejected", func(t *testing.T) {
		a, _ := lang.New()
		if err := a.RegisterMiniLang(lang.MiniLangSpec{Name: "iop"}); err == nil {
			t.Fatal("expected error for nil handler")
		}
	})
	t.Run("duplicate registration rejected", func(t *testing.T) {
		a := newCalcInstance(t)
		if err := a.RegisterMiniLang(calcSpec()); err == nil {
			t.Fatal("expected error registering iop twice")
		}
	})
	t.Run("collision with built-in re (pre-import)", func(t *testing.T) {
		// `re` is a built-in kind; a host kind of the same name must be
		// refused — here at import time, when the fold runs.
		a, _ := lang.New()
		spec := calcSpec()
		spec.Name = "re"
		if err := a.RegisterMiniLang(spec); err != nil {
			t.Fatalf("pre-import register should defer the collision to build: %v", err)
		}
		if _, err := a.Run(`"aql:minilang" import end`); err == nil {
			t.Fatal("import should fail: host kind collides with built-in re")
		}
	})
	t.Run("collision with built-in re (post-import)", func(t *testing.T) {
		// After import the built-ins are live, so the collision is caught
		// immediately at registration.
		a, _ := lang.New()
		if _, err := a.Run(`"aql:minilang" import end`); err != nil {
			t.Fatalf("import: %v", err)
		}
		spec := calcSpec()
		spec.Name = "re"
		if err := a.RegisterMiniLang(spec); err == nil {
			t.Fatal("post-import register of `re` should collide with the built-in")
		}
	})
}
