package eng

import (
	"strings"
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// runWith creates a fresh registry, applies the supplied setup fn (which
// typically registers a few test words), then parses the input slice and
// runs it. Tests in this file exercise the engine via the public native
// registration API only — no boru parser, no built-in word library.
func runWith(t *testing.T, setup func(*core.Registry), input []core.Value) []core.Value {
	t.Helper()
	r, err := core.NewRegistry()
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
	out, err := core.NewTop(r).Run(input)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	return out
}

// registerAdd is a tiny native word that adds two integers.
// Used as the canonical "engine works at all" probe.
func registerAdd(r *core.Registry) {
	r.RegisterNativeFunc(core.NativeFunc{
		Name: "add",

		Signatures: []core.Signature{{
			Args: []*core.Type{core.TInteger, core.TInteger},
			Impl: core.Go(func(args []core.Value, _ map[string]core.Value, _ []core.Value, _ *core.Registry) ([]core.Value, error) {
				a, _ := core.AsInteger(args[0])
				b, _ := core.AsInteger(args[1])
				return []core.Value{core.NewInteger(a + b)}, nil
			}),
			Returns: []*core.Type{core.TInteger}, BarrierPos: -1,
		}},
	})
}

// registerMul adds an integer multiplier. Used together with add for a
// multi-word dispatch test.
func registerMul(r *core.Registry) {
	r.RegisterNativeFunc(core.NativeFunc{
		Name: "mul",

		Signatures: []core.Signature{{
			Args: []*core.Type{core.TInteger, core.TInteger},
			Impl: core.Go(func(args []core.Value, _ map[string]core.Value, _ []core.Value, _ *core.Registry) ([]core.Value, error) {
				a, _ := core.AsInteger(args[0])
				b, _ := core.AsInteger(args[1])
				return []core.Value{core.NewInteger(a * b)}, nil
			}),
			Returns: []*core.Type{core.TInteger}, BarrierPos: -1,
		}},
	})
}

// registerNeg is a stack-only unary word for testing the path where
// a word's sigs have BarrierPos=0 (no forward arg collection).
func registerNeg(r *core.Registry) {
	r.RegisterNativeFunc(core.NativeFunc{
		Name: "neg",
		Signatures: []core.Signature{{
			Args: []*core.Type{core.TInteger},
			Impl: core.Go(func(args []core.Value, _ map[string]core.Value, _ []core.Value, _ *core.Registry) ([]core.Value, error) {
				n, _ := core.AsInteger(args[0])
				return []core.Value{core.NewInteger(-n)}, nil
			}),
			Returns: []*core.Type{core.TInteger}, BarrierPos: 0,
		}},
	})
}

func TestSmokeRegistryStartsBare(t *testing.T) {
	r, err := core.NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	// A fresh registry has no bindings — proves the engine ships zero
	// words by itself. The /v suffix is a parser+stepWord feature
	// that needs no registration; `ref` itself lives in the language
	// layer.
	if names := r.Defs.Names(); len(names) != 0 {
		t.Errorf("expected empty binding store, got %v", names)
	}
}

func TestSmokeRunWithNoWords(t *testing.T) {
	// A program of pure literal values should round-trip via Run with
	// no registered words at all.
	out := runWith(t, nil, []core.Value{core.NewInteger(7)})
	if len(out) != 1 {
		t.Fatalf("got %d results, want 1", len(out))
	}
	got, _ := core.AsInteger(out[0])
	if got != 7 {
		t.Errorf("got %d, want 7", got)
	}
}

func TestSmokeAddForwardArgs(t *testing.T) {
	// `add 2 3` uses forward collection; the handler should see args[0]=2, args[1]=3.
	out := runWith(t, registerAdd, []core.Value{core.NewWord("add"), core.NewInteger(2), core.NewInteger(3)})
	if len(out) != 1 {
		t.Fatalf("got %d results, want 1", len(out))
	}
	got, _ := core.AsInteger(out[0])
	if got != 5 {
		t.Errorf("got %d, want 5", got)
	}
}

func TestSmokeAddPrefixForm(t *testing.T) {
	// `2 3 add` is the all-prefix form; matchSignature reads top-of-stack first.
	out := runWith(t, registerAdd, []core.Value{core.NewInteger(2), core.NewInteger(3), core.NewWord("add")})
	if len(out) != 1 {
		t.Fatalf("got %d results, want 1", len(out))
	}
	got, _ := core.AsInteger(out[0])
	if got != 5 {
		t.Errorf("got %d, want 5", got)
	}
}

func TestSmokeMultipleWords(t *testing.T) {
	// `add 2 3 mul 4` = (2+3)*4 = 20. The result of `add` lands on the
	// stack as 5, then `mul` consumes 5 (prefix) and 4 (forward).
	setup := func(r *core.Registry) {
		registerAdd(r)
		registerMul(r)
	}
	input := []core.Value{
		core.NewWord("add"), core.NewInteger(2), core.NewInteger(3),
		core.NewWord("mul"), core.NewInteger(4),
	}
	out := runWith(t, setup, input)
	if len(out) != 1 {
		t.Fatalf("got %d results, want 1", len(out))
	}
	got, _ := core.AsInteger(out[0])
	if got != 20 {
		t.Errorf("got %d, want 20", got)
	}
}

func TestSmokeStackOnlyDispatch(t *testing.T) {
	// `5 neg` — neg is registered with BarrierPos:0 so the engine
	// must consume the value from the prefix stack rather than
	// forward-collecting.
	out := runWith(t, registerNeg, []core.Value{core.NewInteger(5), core.NewWord("neg")})
	if len(out) != 1 {
		t.Fatalf("got %d results, want 1", len(out))
	}
	got, _ := core.AsInteger(out[0])
	if got != -5 {
		t.Errorf("got %d, want -5", got)
	}
}

func TestSmokeUndefinedWordIsAnError(t *testing.T) {
	// An unregistered word reaching the pointer must error rather than
	// silently turn into an atom (cf. CLAUDE.md "Undefined Words" rule).
	r, err := core.NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	r.InitRootContext()
	_, runErr := core.NewTop(r).Run([]core.Value{core.NewWord("nope")})
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
	r, err := core.NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	registerAdd(r)
	r.InitRootContext()
	_, runErr := core.NewTop(r).Run([]core.Value{core.NewWord("add"), core.NewString("hello"), core.NewInteger(3)})
	if runErr == nil {
		t.Fatal("expected signature error, got nil")
	}
}
