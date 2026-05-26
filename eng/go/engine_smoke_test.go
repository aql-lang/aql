package eng

import (
	"strings"
	"testing"
)

// runWith creates a fresh registry, applies the supplied setup fn (which
// typically registers a few test words), then parses the input slice and
// runs it. Tests in this file exercise the engine via the public native
// registration API only — no aql parser, no built-in word library.
func runWith(t *testing.T, setup func(*Registry), input []Value) []Value {
	t.Helper()
	r, err := NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	if setup != nil {
		setup(r)
	}
	if err := r.Err(); err != nil {
		t.Fatalf("registration: %v", err)
	}
	r.InitRootContext()
	out, err := NewTop(r).Run(input)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	return out
}

// registerAdd is a tiny native word that adds two integers.
// Used as the canonical "engine works at all" probe.
func registerAdd(r *Registry) {
	r.RegisterNativeFunc(NativeFunc{
		Name:        "add",
		ForwardArgs: true,
		Signatures: []NativeSig{{
			Args: []*Type{TInteger, TInteger},
			Handler: func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
				a, _ := AsInteger(args[0])
				b, _ := AsInteger(args[1])
				return []Value{NewInteger(a + b)}, nil
			},
			Returns: []*Type{TInteger},
		}},
	})
}

// registerMul adds an integer multiplier. Used together with add for a
// multi-word dispatch test.
func registerMul(r *Registry) {
	r.RegisterNativeFunc(NativeFunc{
		Name:        "mul",
		ForwardArgs: true,
		Signatures: []NativeSig{{
			Args: []*Type{TInteger, TInteger},
			Handler: func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
				a, _ := AsInteger(args[0])
				b, _ := AsInteger(args[1])
				return []Value{NewInteger(a * b)}, nil
			},
			Returns: []*Type{TInteger},
		}},
	})
}

// registerNeg is a stack-only unary word for testing the path where
// a word's sigs have BarrierPos=0 (no forward arg collection).
func registerNeg(r *Registry) {
	r.RegisterNativeFunc(NativeFunc{
		Name: "neg",
		Signatures: []NativeSig{{
			Args: []*Type{TInteger},
			Handler: func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
				n, _ := AsInteger(args[0])
				return []Value{NewInteger(-n)}, nil
			},
			Returns: []*Type{TInteger},
		}},
	})
}

func TestSmokeRegistryStartsBare(t *testing.T) {
	r, err := NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	// A fresh registry ships only the kernel-level words. `ref` is the
	// one such word — every other built-in flows in from the language
	// layer via Register / RegisterNativeFunc.
	names := r.Defs.Names()
	if len(names) != 1 || names[0] != "ref" {
		t.Errorf("expected kernel binding {ref}, got %v", names)
	}
}

func TestSmokeRunWithNoWords(t *testing.T) {
	// A program of pure literal values should round-trip via Run with
	// no registered words at all.
	out := runWith(t, nil, []Value{NewInteger(7)})
	if len(out) != 1 {
		t.Fatalf("got %d results, want 1", len(out))
	}
	got, _ := AsInteger(out[0])
	if got != 7 {
		t.Errorf("got %d, want 7", got)
	}
}

func TestSmokeAddForwardArgs(t *testing.T) {
	// `add 2 3` uses forward collection; the handler should see args[0]=2, args[1]=3.
	out := runWith(t, registerAdd, []Value{NewWord("add"), NewInteger(2), NewInteger(3)})
	if len(out) != 1 {
		t.Fatalf("got %d results, want 1", len(out))
	}
	got, _ := AsInteger(out[0])
	if got != 5 {
		t.Errorf("got %d, want 5", got)
	}
}

func TestSmokeAddPrefixForm(t *testing.T) {
	// `2 3 add` is the all-prefix form; matchSignature reads top-of-stack first.
	out := runWith(t, registerAdd, []Value{NewInteger(2), NewInteger(3), NewWord("add")})
	if len(out) != 1 {
		t.Fatalf("got %d results, want 1", len(out))
	}
	got, _ := AsInteger(out[0])
	if got != 5 {
		t.Errorf("got %d, want 5", got)
	}
}

func TestSmokeMultipleWords(t *testing.T) {
	// `add 2 3 mul 4` = (2+3)*4 = 20. The result of `add` lands on the
	// stack as 5, then `mul` consumes 5 (prefix) and 4 (forward).
	setup := func(r *Registry) {
		registerAdd(r)
		registerMul(r)
	}
	input := []Value{
		NewWord("add"), NewInteger(2), NewInteger(3),
		NewWord("mul"), NewInteger(4),
	}
	out := runWith(t, setup, input)
	if len(out) != 1 {
		t.Fatalf("got %d results, want 1", len(out))
	}
	got, _ := AsInteger(out[0])
	if got != 20 {
		t.Errorf("got %d, want 20", got)
	}
}

func TestSmokeStackOnlyDispatch(t *testing.T) {
	// `5 neg` — neg is registered without ForwardArgs so the
	// engine must consume the prefix value.
	out := runWith(t, registerNeg, []Value{NewInteger(5), NewWord("neg")})
	if len(out) != 1 {
		t.Fatalf("got %d results, want 1", len(out))
	}
	got, _ := AsInteger(out[0])
	if got != -5 {
		t.Errorf("got %d, want -5", got)
	}
}

func TestSmokeUndefinedWordIsAnError(t *testing.T) {
	// An unregistered word reaching the pointer must error rather than
	// silently turn into an atom (cf. CLAUDE.md "Undefined Words" rule).
	r, err := NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	r.InitRootContext()
	_, runErr := NewTop(r).Run([]Value{NewWord("nope")})
	if runErr == nil {
		t.Fatal("expected undefined_word error, got nil")
	}
	if !strings.Contains(runErr.Error(), "nope") {
		t.Errorf("error message should mention the word, got: %v", runErr)
	}
}

func TestSmokeSignatureMismatchIsAnError(t *testing.T) {
	// `add "hello" 3` — handler expects two integers; passing a string
	// should fail at dispatch time, not panic.
	r, err := NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	registerAdd(r)
	r.InitRootContext()
	_, runErr := NewTop(r).Run([]Value{NewWord("add"), NewString("hello"), NewInteger(3)})
	if runErr == nil {
		t.Fatal("expected signature error, got nil")
	}
}
