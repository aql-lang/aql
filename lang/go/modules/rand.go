package modules

import (
	"fmt"
	mathrand "math/rand"
	"sync"
	"time"

	"github.com/aql-lang/aql/lang/go/native"
)

// randState holds a PRNG instance. Each instance is independent —
// successive calls on the same state share the stream, but two
// distinct states (e.g. top-level default vs `rand.with-seed N`) are
// fully isolated.
type randState struct {
	mu  sync.Mutex
	rng *mathrand.Rand
}

// BuildRandModule creates the "aql:rand" native module.
//
// The top-level `rand` namespace is **non-deterministic by default**:
// at module-build time we seed once from the host clock so a fresh
// `"aql:rand" import` produces genuinely random values.
//
// For deterministic / reproducible sequences (property tests, demo
// fixtures, replayable simulations) use `rand.with-seed N` — it
// returns a fresh isolated instance (an OrderedMap) carrying the same
// methods as the top-level (`int`, `bool`, `float`, `string`,
// `one-of`). The instance has its own PRNG sourced from `N` and does
// not affect the top-level rand or any other instance.
//
//	"aql:rand" import
//	rand.int 0 100              # random, [0, 100)
//	def r (rand.with-seed 42)   # isolated, seeded with 42
//	r.int 0 100                 # deterministic at seed 42
func BuildRandModule(parent *native.Registry) (native.ModuleDesc, error) {
	// Seed the top-level instance from the clock so default usage is
	// non-deterministic — what most developers expect.
	defaultState := newRandState(time.Now().UnixNano())
	exports, err := buildRandExportsForState(defaultState)
	if err != nil {
		return native.ModuleDesc{}, err
	}

	// `rand.with-seed` lives only at the top level. Its handler
	// constructs a new randState seeded with N, builds a separate
	// exports map with all the standard methods (int, bool, float,
	// string, one-of), and returns that map as an OrderedMap. Each
	// call mints a fresh instance — no global mutation.
	withSeedSubReg, err := native.DefaultRegistry()
	if err != nil {
		return native.ModuleDesc{}, err
	}
	withSeedSubReg.RegisterNativeFunc(native.NativeFunc{
		Name: "rand-with-seed",
		Signatures: []native.NativeSig{{
			Args:       []*native.Type{native.TInteger},
			Returns:    []*native.Type{native.TMap},
			BarrierPos: -1,
			Handler: func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
				seed, err := args[0].AsConcreteInteger()
				if err != nil {
					return nil, err
				}
				state := newRandState(seed)
				instance, err := buildRandExportsForState(state)
				if err != nil {
					return nil, err
				}
				return []native.Value{native.NewMap(instance)}, nil
			},
		}},
	})
	exports.Set("with-seed", wrapRandFnDef("rand-with-seed",
		[]native.FnParam{{Type: native.TInteger}},
		[]*native.Type{native.TMap}, withSeedSubReg))

	return native.ModuleDesc{
		ID:      parent.Modules.NextID(),
		Exports: map[string]*native.OrderedMap{"rand": exports},
	}, nil
}

// newRandState builds a fresh PRNG seeded with the given int64.
func newRandState(seed int64) *randState {
	return &randState{rng: mathrand.New(mathrand.NewSource(seed))}
}

// buildRandExportsForState builds the OrderedMap of dotted methods
// (`int`, `bool`, `float`, `string`, `one-of`) bound to the given
// state. Used for both the top-level default and for each
// `rand.with-seed` instance — each gets its own sub-registry of
// natives closing over its own randState.
func buildRandExportsForState(state *randState) (*native.OrderedMap, error) {
	subReg, err := native.DefaultRegistry()
	if err != nil {
		return nil, err
	}
	for _, n := range randNativesForState(state) {
		subReg.RegisterNativeFunc(n)
	}

	exports := native.NewOrderedMap()
	// Wrapper FnSig Params match the inner NativeSig.Args order
	// (top-first per SIG-ORDER-REFACTOR.0.md). Aligned with the
	// FORWARD canonical surface — sig[0] is the first arg written
	// after the word: `rand.int LO HI`, `rand.string CHARSET LEN`.
	exports.Set("int", wrapRandFnDef("rand-int",
		[]native.FnParam{{Type: native.TInteger}, {Type: native.TInteger}},
		[]*native.Type{native.TInteger}, subReg))
	exports.Set("bool", wrapRandFnDef("rand-bool",
		nil,
		[]*native.Type{native.TBoolean}, subReg))
	exports.Set("float", wrapRandFnDef("rand-float",
		nil,
		[]*native.Type{native.TDecimal}, subReg))
	exports.Set("string", wrapRandFnDef("rand-string",
		[]native.FnParam{{Type: native.TString}, {Type: native.TInteger}},
		[]*native.Type{native.TString}, subReg))
	exports.Set("one-of", wrapRandFnDef("rand-one-of",
		[]native.FnParam{{Type: native.TList}},
		[]*native.Type{native.TAny}, subReg))
	return exports, nil
}

// wrapRandFnDef builds the FnDef wrapper that dispatches a dotted
// rand.<word> call into the sub-registry's native handler. Body is
// `[Word(wordName)]` so execFnDefLiteral's trivial-delegation
// short-circuit fires (direct execMatch on the inner handler).
func wrapRandFnDef(wordName string, params []native.FnParam, returns []*native.Type, subReg *native.Registry) native.Value {
	fnDef := native.FnDefInfo{
		Name: wordName,
		Sigs: []native.FnSig{{
			Params:     params,
			Returns:    returns,
			Body:       []native.Value{native.NewWord(wordName)},
			BarrierPos: -1,
		}},
		Registry: subReg,
	}
	return native.NewFnDef(fnDef)
}

// randNativesForState builds the Go-implemented rand primitives.
// Every handler closes over `state` directly, so each call mints a
// new set of natives bound to a specific PRNG instance. No global
// capability lookup — the state pointer is captured at construction.
func randNativesForState(state *randState) []native.NativeFunc {
	return []native.NativeFunc{
		{
			Name: "rand-int",
			Signatures: []native.NativeSig{{
				// Canonical surface (forward form): `rand.int LO HI`.
				// sig[0]=lo, sig[1]=hi. Returns a uniform integer in
				// the HALF-OPEN range [lo, hi) — inclusive lower,
				// exclusive upper. Matches Python's random.randrange,
				// Rust's gen_range, Go's rand.Intn.
				Args:       []*native.Type{native.TInteger, native.TInteger},
				Returns:    []*native.Type{native.TInteger},
				BarrierPos: -1,
				Handler: func(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
					lo, err := args[0].AsConcreteInteger()
					if err != nil {
						return nil, err
					}
					hi, err := args[1].AsConcreteInteger()
					if err != nil {
						return nil, err
					}
					if hi <= lo {
						return nil, r.AqlError("rand_error",
							fmt.Sprintf("rand.int: hi (%d) <= lo (%d); range must be non-empty", hi, lo),
							"rand.int")
					}
					state.mu.Lock()
					n := lo + state.rng.Int63n(hi-lo)
					state.mu.Unlock()
					return []native.Value{native.NewInteger(n)}, nil
				},
			}},
		},
		{
			Name: "rand-bool",
			Signatures: []native.NativeSig{{
				Args:       []*native.Type{},
				Returns:    []*native.Type{native.TBoolean},
				BarrierPos: -1,
				Handler: func(_ []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
					state.mu.Lock()
					b := state.rng.Intn(2) == 1
					state.mu.Unlock()
					return []native.Value{native.NewBoolean(b)}, nil
				},
			}},
		},
		{
			Name: "rand-float",
			Signatures: []native.NativeSig{{
				// Returns a uniform decimal in [0.0, 1.0).
				Args:       []*native.Type{},
				Returns:    []*native.Type{native.TDecimal},
				BarrierPos: -1,
				Handler: func(_ []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
					state.mu.Lock()
					f := state.rng.Float64()
					state.mu.Unlock()
					return []native.Value{native.NewDecimal(f)}, nil
				},
			}},
		},
		{
			Name: "rand-string",
			Signatures: []native.NativeSig{{
				// Canonical surface (forward form):
				// `rand.string CHARSET LENGTH`. sig[0]=charset (String),
				// sig[1]=length (Integer).
				Args:       []*native.Type{native.TString, native.TInteger},
				Returns:    []*native.Type{native.TString},
				BarrierPos: -1,
				Handler: func(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
					charset, err := args[0].AsConcreteString()
					if err != nil {
						return nil, err
					}
					length, err := args[1].AsConcreteInteger()
					if err != nil {
						return nil, err
					}
					if length < 0 {
						return nil, r.AqlError("rand_error",
							fmt.Sprintf("rand.string: length (%d) < 0", length), "rand.string")
					}
					runes := []rune(charset)
					if len(runes) == 0 {
						if length == 0 {
							return []native.Value{native.NewString("")}, nil
						}
						return nil, r.AqlError("rand_error",
							"rand.string: empty charset", "rand.string")
					}
					out := make([]rune, length)
					state.mu.Lock()
					for i := range out {
						out[i] = runes[state.rng.Intn(len(runes))]
					}
					state.mu.Unlock()
					return []native.Value{native.NewString(string(out))}, nil
				},
			}},
		},
		{
			Name: "rand-one-of",
			Signatures: []native.NativeSig{{
				Args:       []*native.Type{native.TList},
				Returns:    []*native.Type{native.TAny},
				BarrierPos: -1,
				Handler: func(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
					lst, err := native.RequireConcreteList(args[0], "rand.one-of")
					if err != nil {
						return nil, err
					}
					n := lst.Len()
					if n == 0 {
						return nil, r.AqlError("rand_error",
							"rand.one-of: empty list", "rand.one-of")
					}
					state.mu.Lock()
					idx := state.rng.Intn(n)
					state.mu.Unlock()
					return []native.Value{lst.Get(idx)}, nil
				},
			}},
		},
	}
}
