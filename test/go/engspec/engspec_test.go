// Spec-runner test for the engine kernel — runs the shared corpus at
// aql/eng/spec/*.tsv (sibling of eng/go/ and eng/ts/, so Go and TypeScript
// ports run the same .tsv files). Each row is parsed with the AQL parser
// (eng/parser) and run against a fresh eng.Registry pre-populated with
// eng.RegisterCoreWords plus a fixed set of spec-runner test fixtures
// (q-suffixed). No production native words (add, upper, …) are installed
// — the q-fixtures cover dispatch / value / type-lattice ground in
// spec-stable minimal forms.
//
// The "q" suffix on most fixtures marks them as SPEC-RUNNER FIXTURES,
// distinct from production AQL words of the same root name. Language-
// fundamental keywords (def, fn, quote, args, type, untype, typeof,
// is, none, end, …) keep their bare names because what's being tested
// IS the keyword itself, not a fixture for it.
//
// This file lives in the test module (not eng/go) so eng/go has no
// dependency on test — the dep arrow points one way: test → eng.
package engspec

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/aql-lang/aql/eng"
	"github.com/aql-lang/aql/eng/parser"
	"github.com/aql-lang/aql/test/go/specrunner"
)

// specReplayCounter is bumped per call to the `replayq` test fixture so
// each Mark/Move pair gets a unique ID across a spec file.
var specReplayCounter int

// registerSpecWords installs the eng core words plus the spec-runner
// test fixtures on a registry. The fixtures are minimal, single-overload
// variants tailored for spec coverage of the dispatch / value /
// type-lattice core.
func registerSpecWords(r *eng.Registry) {
	eng.RegisterCoreWords(r)

	toFloat := func(v eng.Value) float64 {
		if v.VType.Matches(eng.TInteger) {
			n, _ := eng.AsInteger(v)
			return float64(n)
		}
		f, _ := eng.AsDecimal(v)
		return f
	}
	numericBinary := func(intOp func(a, b int64) int64, floatOp func(a, b float64) float64) eng.Handler {
		return func(args []eng.Value, _ map[string]eng.Value, _ []eng.Value, _ *eng.Registry) ([]eng.Value, error) {
			if args[0].VType.Matches(eng.TInteger) && args[1].VType.Matches(eng.TInteger) {
				a, _ := eng.AsInteger(args[0])
				b, _ := eng.AsInteger(args[1])
				return []eng.Value{eng.NewInteger(intOp(a, b))}, nil
			}
			return []eng.Value{eng.NewDecimal(floatOp(toFloat(args[0]), toFloat(args[1])))}, nil
		}
	}
	numberPair := []*eng.Type{eng.TNumber, eng.TNumber}

	r.RegisterNativeFunc(eng.NativeFunc{
		Name: "addq", ForwardArgs: true,
		Signatures: []eng.NativeSig{{
			Args:    numberPair,
			Handler: numericBinary(func(a, b int64) int64 { return b + a }, func(a, b float64) float64 { return b + a }),
			Returns: []*eng.Type{eng.TNumber},
		}},
	})
	r.RegisterNativeFunc(eng.NativeFunc{
		Name: "subq", ForwardArgs: true,
		Signatures: []eng.NativeSig{{
			Args:    numberPair,
			Handler: numericBinary(func(a, b int64) int64 { return b - a }, func(a, b float64) float64 { return b - a }),
			Returns: []*eng.Type{eng.TNumber},
		}},
	})
	r.RegisterNativeFunc(eng.NativeFunc{
		Name: "mulq", ForwardArgs: true,
		Signatures: []eng.NativeSig{{
			Args:    numberPair,
			Handler: numericBinary(func(a, b int64) int64 { return b * a }, func(a, b float64) float64 { return b * a }),
			Returns: []*eng.Type{eng.TNumber},
		}},
	})
	r.RegisterNativeFunc(eng.NativeFunc{
		Name: "negq", ForwardArgs: true,
		Signatures: []eng.NativeSig{{
			Args: []*eng.Type{eng.TNumber}, BarrierPos: 1,
			Handler: func(args []eng.Value, _ map[string]eng.Value, _ []eng.Value, _ *eng.Registry) ([]eng.Value, error) {
				if args[0].VType.Matches(eng.TInteger) {
					n, _ := eng.AsInteger(args[0])
					return []eng.Value{eng.NewInteger(-n)}, nil
				}
				f, _ := eng.AsDecimal(args[0])
				return []eng.Value{eng.NewDecimal(-f)}, nil
			},
			Returns: []*eng.Type{eng.TNumber},
		}},
	})
	r.RegisterNativeFunc(eng.NativeFunc{
		Name: "concatq", ForwardArgs: true,
		Signatures: []eng.NativeSig{{
			Args: []*eng.Type{eng.TString, eng.TString},
			Handler: func(args []eng.Value, _ map[string]eng.Value, _ []eng.Value, _ *eng.Registry) ([]eng.Value, error) {
				a, _ := eng.AsString(args[0])
				b, _ := eng.AsString(args[1])
				return []eng.Value{eng.NewString(b + a)}, nil
			},
			Returns: []*eng.Type{eng.TString},
		}},
	})
	r.RegisterNativeFunc(eng.NativeFunc{
		Name: "describeq", ForwardArgs: true,
		Signatures: []eng.NativeSig{
			{
				Args: []*eng.Type{eng.TInteger},
				Handler: func(args []eng.Value, _ map[string]eng.Value, _ []eng.Value, _ *eng.Registry) ([]eng.Value, error) {
					n, _ := eng.AsInteger(args[0])
					return []eng.Value{eng.NewString("int:" + strconv.FormatInt(n, 10))}, nil
				},
				Returns: []*eng.Type{eng.TString},
			},
			{
				Args: []*eng.Type{eng.TString},
				Handler: func(args []eng.Value, _ map[string]eng.Value, _ []eng.Value, _ *eng.Registry) ([]eng.Value, error) {
					s, _ := eng.AsString(args[0])
					return []eng.Value{eng.NewString("str:" + s)}, nil
				},
				Returns: []*eng.Type{eng.TString},
			},
		},
	})
	r.RegisterNativeFunc(eng.NativeFunc{
		Name: "tagq", ForwardArgs: true,
		Signatures: []eng.NativeSig{
			{Args: []*eng.Type{eng.TAny}, Handler: func(_ []eng.Value, _ map[string]eng.Value, _ []eng.Value, _ *eng.Registry) ([]eng.Value, error) {
				return []eng.Value{eng.NewString("any")}, nil
			}, Returns: []*eng.Type{eng.TString}},
			{Args: []*eng.Type{eng.TInteger}, Handler: func(_ []eng.Value, _ map[string]eng.Value, _ []eng.Value, _ *eng.Registry) ([]eng.Value, error) {
				return []eng.Value{eng.NewString("specific")}, nil
			}, Returns: []*eng.Type{eng.TString}},
		},
	})
	r.RegisterNativeFunc(eng.NativeFunc{
		Name: "factq", ForwardArgs: true,
		Signatures: []eng.NativeSig{
			{
				Args: []*eng.Type{eng.TInteger}, Patterns: map[int]eng.Value{0: eng.NewInteger(0)},
				Handler: func(_ []eng.Value, _ map[string]eng.Value, _ []eng.Value, _ *eng.Registry) ([]eng.Value, error) {
					return []eng.Value{eng.NewInteger(1)}, nil
				},
				Returns: []*eng.Type{eng.TInteger},
			},
			{
				Args: []*eng.Type{eng.TInteger},
				Handler: func(args []eng.Value, _ map[string]eng.Value, _ []eng.Value, _ *eng.Registry) ([]eng.Value, error) {
					n, _ := eng.AsInteger(args[0])
					return []eng.Value{eng.NewInteger(n)}, nil
				},
				Returns: []*eng.Type{eng.TInteger},
			},
		},
	})
	r.RegisterNativeFunc(eng.NativeFunc{
		Name: "codeq", ForwardArgs: true,
		Signatures: []eng.NativeSig{
			{
				Args: []*eng.Type{eng.TInteger}, Patterns: map[int]eng.Value{0: eng.NewInteger(99)},
				Handler: func(_ []eng.Value, _ map[string]eng.Value, _ []eng.Value, _ *eng.Registry) ([]eng.Value, error) {
					return []eng.Value{eng.NewString("ninety-nine")}, nil
				},
				Returns: []*eng.Type{eng.TString},
			},
			{
				Args: []*eng.Type{eng.TInteger},
				Handler: func(_ []eng.Value, _ map[string]eng.Value, _ []eng.Value, _ *eng.Registry) ([]eng.Value, error) {
					return []eng.Value{eng.NewString("general")}, nil
				},
				Returns: []*eng.Type{eng.TString},
			},
		},
	})
	r.RegisterNativeFunc(eng.NativeFunc{
		Name: "routeq", ForwardArgs: true,
		Signatures: []eng.NativeSig{
			{
				Args: []*eng.Type{eng.TString}, Patterns: map[int]eng.Value{0: eng.NewString("admin")},
				Handler: func(_ []eng.Value, _ map[string]eng.Value, _ []eng.Value, _ *eng.Registry) ([]eng.Value, error) {
					return []eng.Value{eng.NewString("matched-admin")}, nil
				},
				Returns: []*eng.Type{eng.TString},
			},
			{
				Args: []*eng.Type{eng.TString},
				Handler: func(_ []eng.Value, _ map[string]eng.Value, _ []eng.Value, _ *eng.Registry) ([]eng.Value, error) {
					return []eng.Value{eng.NewString("other")}, nil
				},
				Returns: []*eng.Type{eng.TString},
			},
		},
	})
	r.RegisterNativeFunc(eng.NativeFunc{
		Name: "tripq", ForwardArgs: true,
		Signatures: []eng.NativeSig{{
			Args: []*eng.Type{eng.TInteger, eng.TInteger, eng.TInteger},
			Handler: func(args []eng.Value, _ map[string]eng.Value, _ []eng.Value, _ *eng.Registry) ([]eng.Value, error) {
				a, _ := eng.AsInteger(args[0])
				b, _ := eng.AsInteger(args[1])
				c, _ := eng.AsInteger(args[2])
				return []eng.Value{eng.NewString(fmt.Sprintf("%d,%d,%d", a, b, c))}, nil
			},
			Returns: []*eng.Type{eng.TString},
		}},
	})
	r.RegisterNativeFunc(eng.NativeFunc{
		Name: "pairq", ForwardArgs: true,
		Signatures: []eng.NativeSig{{
			Args:       []*eng.Type{eng.TInteger, eng.TInteger},
			BarrierPos: 1,
			Handler: func(args []eng.Value, _ map[string]eng.Value, _ []eng.Value, _ *eng.Registry) ([]eng.Value, error) {
				a, _ := eng.AsInteger(args[0])
				b, _ := eng.AsInteger(args[1])
				return []eng.Value{eng.NewString(fmt.Sprintf("%d:%d", a, b))}, nil
			},
			Returns: []*eng.Type{eng.TString},
		}},
	})

	// ── Barrier / arity fixtures (for barrier.tsv) ────────────────
	// nilq — a 0-arg word. Exercises 0-arity sigs and the `/0`
	// argCount filter (the fallback-section match path).
	r.RegisterNativeFunc(eng.NativeFunc{
		Name: "nilq",
		Signatures: []eng.NativeSig{{
			Args: []*eng.Type{},
			Handler: func(_ []eng.Value, _ map[string]eng.Value, _ []eng.Value, _ *eng.Registry) ([]eng.Value, error) {
				return []eng.Value{eng.NewString("nil")}, nil
			},
			Returns: []*eng.Type{eng.TString},
		}},
	})

	// flexq — two overloads of different arity, [Integer] and
	// [Integer, Integer], both forward-eligible (BarrierPos = N). The
	// 1-arg sig is tried first, so a bare `flexq` always picks it; the
	// `/N` argCount modifier (flexq/1, flexq/2) selects the overload
	// explicitly, and `/1f`, `/2s` etc. combine arity selection with a
	// forced forward/stack boundary.
	r.RegisterNativeFunc(eng.NativeFunc{
		Name: "flexq", ForwardArgs: true,
		Signatures: []eng.NativeSig{
			{
				Args: []*eng.Type{eng.TInteger},
				Handler: func(args []eng.Value, _ map[string]eng.Value, _ []eng.Value, _ *eng.Registry) ([]eng.Value, error) {
					a, _ := eng.AsInteger(args[0])
					return []eng.Value{eng.NewString(fmt.Sprintf("one:%d", a))}, nil
				},
				Returns: []*eng.Type{eng.TString},
			},
			{
				Args: []*eng.Type{eng.TInteger, eng.TInteger},
				Handler: func(args []eng.Value, _ map[string]eng.Value, _ []eng.Value, _ *eng.Registry) ([]eng.Value, error) {
					a, _ := eng.AsInteger(args[0])
					b, _ := eng.AsInteger(args[1])
					return []eng.Value{eng.NewString(fmt.Sprintf("two:%d,%d", a, b))}, nil
				},
				Returns: []*eng.Type{eng.TString},
			},
		},
	})

	// Fixed-arity Integer formatters with an intrinsic barrier at the
	// position named by the numeric suffix (the un-suffixed "main"
	// series — pairq=2/B1, tripq=3/B3, quadq=4/B2, quintq=5/B3,
	// hexq=6/B3, septq=7/B4 — plus tri1q/tri2q and quad1q/quad3q for
	// the off-centre boundaries). Each handler renders its args in
	// signature order, comma-separated, so a row's output reveals
	// exactly which source token bound to which sig position.
	// Combined with /s, /f and /N the rows reach every boundary
	// position 0..N for arities 1..7.
	intArgsFmt := func(args []eng.Value, _ map[string]eng.Value, _ []eng.Value, _ *eng.Registry) ([]eng.Value, error) {
		parts := make([]string, len(args))
		for i, a := range args {
			n, _ := eng.AsInteger(a)
			parts[i] = strconv.FormatInt(n, 10)
		}
		return []eng.Value{eng.NewString(strings.Join(parts, ","))}, nil
	}
	intArity := func(name string, n, barrier int) {
		args := make([]*eng.Type, n)
		for i := range args {
			args[i] = eng.TInteger
		}
		r.RegisterNativeFunc(eng.NativeFunc{
			Name: name, ForwardArgs: true,
			Signatures: []eng.NativeSig{{
				Args: args, BarrierPos: barrier,
				Handler: intArgsFmt,
				Returns: []*eng.Type{eng.TString},
			}},
		})
	}
	intArity("tri1q", 3, 1)
	intArity("tri2q", 3, 2)
	intArity("quad1q", 4, 1)
	intArity("quadq", 4, 2)
	intArity("quad3q", 4, 3)
	intArity("quintq", 5, 3)
	intArity("hexq", 6, 3)
	intArity("septq", 7, 4)

	r.RegisterNativeFunc(eng.NativeFunc{
		Name: "lengthq", ForwardArgs: true,
		Signatures: []eng.NativeSig{{
			Args: []*eng.Type{eng.TList},
			Handler: func(args []eng.Value, _ map[string]eng.Value, _ []eng.Value, _ *eng.Registry) ([]eng.Value, error) {
				lst, _ := eng.AsList(args[0])
				return []eng.Value{eng.NewInteger(int64(lst.Len()))}, nil
			},
			Returns: []*eng.Type{eng.TInteger},
		}},
	})
	r.RegisterNativeFunc(eng.NativeFunc{
		Name: "firstq", ForwardArgs: true,
		Signatures: []eng.NativeSig{{
			Args: []*eng.Type{eng.TList},
			Handler: func(args []eng.Value, _ map[string]eng.Value, _ []eng.Value, _ *eng.Registry) ([]eng.Value, error) {
				lst, _ := eng.AsList(args[0])
				if lst.Len() == 0 {
					return []eng.Value{eng.NewNone()}, nil
				}
				return []eng.Value{lst.Get(0)}, nil
			},
			Returns: []*eng.Type{eng.TAny},
		}},
	})
	r.RegisterNativeFunc(eng.NativeFunc{
		Name: "replayq", ForwardArgs: true,
		Signatures: []eng.NativeSig{{
			Args:       []*eng.Type{eng.TList},
			NoEvalArgs: map[int]bool{0: true},
			Handler: func(args []eng.Value, _ map[string]eng.Value, _ []eng.Value, _ *eng.Registry) ([]eng.Value, error) {
				_lst, _ := eng.AsList(args[0])
				body := _lst.Slice()
				specReplayCounter++
				id := fmt.Sprintf("__replayq_%d", specReplayCounter)
				out := make([]eng.Value, 0, len(body)+2)
				out = append(out, eng.NewMark(id, body...))
				out = append(out, body...)
				out = append(out, eng.NewMove(id, "replayq"))
				return out, nil
			},
		}},
	})

	r.Defs.Push("pi", eng.NewInteger(3))
	r.Defs.Push("tau", eng.NewInteger(6))
	r.Defs.Push("greeting", eng.NewString("hello"))
}

// TestSpec runs aql/eng/spec/*.tsv against the engine kernel — a fresh
// eng.Registry populated with eng.RegisterCoreWords plus the spec-runner
// fixtures registered by registerSpecWords above. The shared TSV
// scaffolding (file walk, row parsing, ERROR handling, value rendering)
// lives in test/go/specrunner.
func TestSpec(t *testing.T) {
	specDir := filepath.Join("..", "..", "..", "eng", "spec")
	specrunner.RunDir(t, specDir, func(input string) ([]eng.Value, error) {
		values, err := parser.Parse(input)
		if err != nil {
			return nil, err
		}
		r, err := eng.NewRegistry()
		if err != nil {
			return nil, err
		}
		registerSpecWords(r)
		r.InitRootContext()
		return eng.NewTop(r).Run(values)
	})
}
