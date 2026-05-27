package modules

import (
	"fmt"
	mathrand "math/rand"
	"sync"
	"time"

	"github.com/aql-lang/aql/eng/go"
	"github.com/aql-lang/aql/lang/go/native"
)

// randState holds the PRNG bound to a registry. Stored under capRandRng
// on the parent (caller's) registry so successive rand.* calls share
// the same source, and rand.seed re-seeds them deterministically.
//
// Default seed is 1, so a fresh registry that never calls rand.seed
// still produces a reproducible sequence — useful for property tests
// where deterministic replay matters more than uniqueness.
type randState struct {
	mu  sync.Mutex
	rng *mathrand.Rand
}

const capRandRng = "rand.rng.active"

// activeRand returns the randState for parent, lazily creating it
// with the default seed on first access.
func activeRand(parent *native.Registry) *randState {
	if s, ok, _ := eng.Cap[*randState](parent, capRandRng); ok && s != nil {
		return s
	}
	s := &randState{rng: mathrand.New(mathrand.NewSource(1))}
	_ = parent.Capabilities.Set(capRandRng, s)
	return s
}

// BuildRandModule creates the "aql:rand" native module. Exposes seeded
// deterministic randomness as words in the "rand" namespace: seed, int,
// bool, float, string, one-of, list-of.
//
// The module is intentionally minimal in v1 — frequency/map-from and
// other convenience generators can be layered in pure AQL on top of
// these primitives.
func BuildRandModule(parent *native.Registry) (native.ModuleDesc, error) {
	subReg, err := native.DefaultRegistry()
	if err != nil {
		return native.ModuleDesc{}, err
	}

	for _, n := range randNatives(parent) {
		subReg.RegisterNativeFunc(n)
	}

	exports := native.NewOrderedMap()
	// Param ordering follows the unified dispatch rule: sig[0] is the
	// top of the stack. Stack-only surface form `lo hi rand.int` puts
	// hi on top, so sig[0]=hi, sig[1]=lo. Likewise for two-arg sigs
	// below.
	exports.Set("seed", wrapRandFnDef("rand-seed",
		[]native.FnParam{{Type: native.TInteger}},
		nil, subReg))
	// fresh-seed reseeds the PRNG from the host clock. Use this when
	// you actually want non-reproducible randomness — the default
	// seed of 1 is deterministic on purpose so property tests and
	// demos replay. Profile note: `rand.fresh-seed` is denied by the
	// `gen` profile because it reads the clock.
	exports.Set("fresh-seed", wrapRandFnDef("rand-fresh-seed",
		nil, nil, subReg))
	exports.Set("int", wrapRandFnDef("rand-int",
		// stack `lo hi`: sig[0]=hi, sig[1]=lo
		[]native.FnParam{{Type: native.TInteger}, {Type: native.TInteger}},
		[]*native.Type{native.TInteger}, subReg))
	exports.Set("bool", wrapRandFnDef("rand-bool",
		nil,
		[]*native.Type{native.TBoolean}, subReg))
	exports.Set("float", wrapRandFnDef("rand-float",
		nil,
		[]*native.Type{native.TDecimal}, subReg))
	// Wrapper FnSig Params are matched in source order (bottom-of-
	// stack first), the OPPOSITE of inner NativeSig.Args which use
	// sig order (top-of-stack first). Compare time.format for the
	// same convention.
	exports.Set("string", wrapRandFnDef("rand-string",
		// source `charset length`: Params[0]=charset, Params[1]=length
		[]native.FnParam{{Type: native.TString}, {Type: native.TInteger}},
		[]*native.Type{native.TString}, subReg))
	exports.Set("one-of", wrapRandFnDef("rand-one-of",
		[]native.FnParam{{Type: native.TList}},
		[]*native.Type{native.TAny}, subReg))
	// list-of / map-from are deferred. Module FnDef wrappers always
	// auto-evaluate list args (FnSig has no NoEvalArgs equivalent),
	// so a quoted generator body cannot be threaded through the
	// `rand.list-of` surface today without forcing the user to write
	// `(quote [...]) 5 rand.list-of`. The PBT framework (Stage 3)
	// iterates generator bodies on the Go side anyway, so this
	// limitation costs nothing at the property-test layer.

	return native.ModuleDesc{
		ID:      parent.Modules.NextID(),
		Exports: map[string]*native.OrderedMap{"rand": exports},
	}, nil
}

// wrapRandFnDef builds the FnDef wrapper that dispatches a dotted
// rand.<word> call into the sub-registry's native handler. BarrierPos
// is -1 (all forward-eligible) per the module-wrapper rule in
// lang/go/CLAUDE.md "Module FnDef Wrappers — inner sig BarrierPos".
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

// randNatives builds the Go-implemented rand primitives. Each handler
// reaches into activeRand(parent) so successive calls share the PRNG.
func randNatives(parent *native.Registry) []native.NativeFunc {
	return []native.NativeFunc{
		{
			Name: "rand-seed",
			Signatures: []native.NativeSig{{
				Args:       []*native.Type{native.TInteger},
				Returns:    []*native.Type{},
				BarrierPos: -1,
				Handler: func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
					seed, err := args[0].AsConcreteInteger()
					if err != nil {
						return nil, err
					}
					s := activeRand(parent)
					s.mu.Lock()
					s.rng = mathrand.New(mathrand.NewSource(seed))
					s.mu.Unlock()
					return nil, nil
				},
			}},
		},
		{
			Name: "rand-fresh-seed",
			Signatures: []native.NativeSig{{
				Args:       []*native.Type{},
				Returns:    []*native.Type{},
				BarrierPos: -1,
				Handler: func(_ []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
					s := activeRand(parent)
					s.mu.Lock()
					s.rng = mathrand.New(mathrand.NewSource(time.Now().UnixNano()))
					s.mu.Unlock()
					return nil, nil
				},
			}},
		},
		{
			Name: "rand-int",
			Signatures: []native.NativeSig{{
				// stack `lo hi`: sig[0]=hi (top), sig[1]=lo (deeper).
				Args:       []*native.Type{native.TInteger, native.TInteger},
				Returns:    []*native.Type{native.TInteger},
				BarrierPos: -1,
				Handler: func(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
					hi, err := args[0].AsConcreteInteger()
					if err != nil {
						return nil, err
					}
					lo, err := args[1].AsConcreteInteger()
					if err != nil {
						return nil, err
					}
					if hi < lo {
						return nil, r.AqlError("rand_error",
							fmt.Sprintf("rand.int: hi (%d) < lo (%d)", hi, lo), "rand.int")
					}
					s := activeRand(parent)
					s.mu.Lock()
					n := lo + s.rng.Int63n(hi-lo+1)
					s.mu.Unlock()
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
					s := activeRand(parent)
					s.mu.Lock()
					b := s.rng.Intn(2) == 1
					s.mu.Unlock()
					return []native.Value{native.NewBoolean(b)}, nil
				},
			}},
		},
		{
			Name: "rand-float",
			Signatures: []native.NativeSig{{
				Args:       []*native.Type{},
				Returns:    []*native.Type{native.TDecimal},
				BarrierPos: -1,
				Handler: func(_ []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
					s := activeRand(parent)
					s.mu.Lock()
					f := s.rng.Float64()
					s.mu.Unlock()
					return []native.Value{native.NewDecimal(f)}, nil
				},
			}},
		},
		{
			Name: "rand-string",
			Signatures: []native.NativeSig{{
				// stack `charset length`: sig[0]=length, sig[1]=charset.
				Args:       []*native.Type{native.TInteger, native.TString},
				Returns:    []*native.Type{native.TString},
				BarrierPos: -1,
				Handler: func(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
					length, err := args[0].AsConcreteInteger()
					if err != nil {
						return nil, err
					}
					charset, err := args[1].AsConcreteString()
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
					s := activeRand(parent)
					s.mu.Lock()
					for i := range out {
						out[i] = runes[s.rng.Intn(len(runes))]
					}
					s.mu.Unlock()
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
					s := activeRand(parent)
					s.mu.Lock()
					idx := s.rng.Intn(n)
					s.mu.Unlock()
					return []native.Value{lst.Get(idx)}, nil
				},
			}},
		},
	}
}
