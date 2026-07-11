// Spec-runner test for the engine kernel — runs the shared corpus at
// aql/eng/spec/*.tsv (sibling of eng/go/ and eng/ts/, so Go and TypeScript
// ports run the same .tsv files). Each row is parsed with the AQL parser
// (eng/go/parser) and run against a fresh eng.Registry pre-populated with
// kernel-only spec-runner fixtures (q-suffixed plus minimal copies of
// the words eng/spec rows exercise — def, fn, dup, …). After the
// eng→lang migration eng itself ships no word registrations; engspec
// keeps its own fixtures so the kernel can still be tested in
// isolation against the same .tsv corpus.
//
// The "q" suffix on most fixtures marks them as SPEC-RUNNER FIXTURES,
// distinct from production AQL words of the same root name. Language-
// fundamental keywords (def, fn, quote, args, refine, typeof,
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

	"github.com/aql-lang/aql/eng/go"
	"github.com/aql-lang/aql/eng/go/parser"
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
	toFloat := func(v eng.Value) float64 {
		if v.Parent.ConformsTo(eng.TInteger) {
			n, _ := eng.AsInteger(v)
			return float64(n)
		}
		f, _ := eng.AsFloat(v)
		return f
	}
	numericBinary := func(intOp func(a, b int64) int64, floatOp func(a, b float64) float64) eng.Handler {
		return func(args []eng.Value, _ map[string]eng.Value, _ []eng.Value, _ *eng.Registry) ([]eng.Value, error) {
			if args[0].Parent.ConformsTo(eng.TInteger) && args[1].Parent.ConformsTo(eng.TInteger) {
				a, _ := eng.AsInteger(args[0])
				b, _ := eng.AsInteger(args[1])
				return []eng.Value{eng.NewInteger(intOp(a, b))}, nil
			}
			return []eng.Value{eng.NewFloat(floatOp(toFloat(args[0]), toFloat(args[1])))}, nil
		}
	}
	numberPair := []*eng.Type{eng.TNumber, eng.TNumber}

	r.RegisterNativeFunc(eng.NativeFunc{
		Name: "addq",
		Signatures: []eng.Signature{{
			Args:    numberPair,
			Impl:    eng.Go(numericBinary(func(a, b int64) int64 { return b + a }, func(a, b float64) float64 { return b + a })),
			Returns: []*eng.Type{eng.TNumber}, BarrierPos: -1,
		}},
	})
	r.RegisterNativeFunc(eng.NativeFunc{
		Name: "subq",
		Signatures: []eng.Signature{{
			Args:    numberPair,
			Impl:    eng.Go(numericBinary(func(a, b int64) int64 { return b - a }, func(a, b float64) float64 { return b - a })),
			Returns: []*eng.Type{eng.TNumber}, BarrierPos: -1,
		}},
	})
	r.RegisterNativeFunc(eng.NativeFunc{
		Name: "mulq",
		Signatures: []eng.Signature{{
			Args:    numberPair,
			Impl:    eng.Go(numericBinary(func(a, b int64) int64 { return b * a }, func(a, b float64) float64 { return b * a })),
			Returns: []*eng.Type{eng.TNumber}, BarrierPos: -1,
		}},
	})
	r.RegisterNativeFunc(eng.NativeFunc{
		Name: "negq",
		Signatures: []eng.Signature{{
			Args: []*eng.Type{eng.TNumber}, BarrierPos: 1,
			Impl: eng.Go(func(args []eng.Value, _ map[string]eng.Value, _ []eng.Value, _ *eng.Registry) ([]eng.Value, error) {
				if args[0].Parent.ConformsTo(eng.TInteger) {
					n, _ := eng.AsInteger(args[0])
					return []eng.Value{eng.NewInteger(-n)}, nil
				}
				f, _ := eng.AsFloat(args[0])
				return []eng.Value{eng.NewFloat(-f)}, nil
			}),
			Returns: []*eng.Type{eng.TNumber},
		}},
	})
	r.RegisterNativeFunc(eng.NativeFunc{
		Name: "concatq",
		Signatures: []eng.Signature{{
			Args: []*eng.Type{eng.TString, eng.TString},
			Impl: eng.Go(func(args []eng.Value, _ map[string]eng.Value, _ []eng.Value, _ *eng.Registry) ([]eng.Value, error) {
				a, _ := eng.AsString(args[0])
				b, _ := eng.AsString(args[1])
				return []eng.Value{eng.NewString(b + a)}, nil
			}),
			Returns: []*eng.Type{eng.TString}, BarrierPos: -1,
		}},
	})
	r.RegisterNativeFunc(eng.NativeFunc{
		Name: "describeq",
		Signatures: []eng.Signature{
			{
				Args: []*eng.Type{eng.TInteger},
				Impl: eng.Go(func(args []eng.Value, _ map[string]eng.Value, _ []eng.Value, _ *eng.Registry) ([]eng.Value, error) {
					n, _ := eng.AsInteger(args[0])
					return []eng.Value{eng.NewString("int:" + strconv.FormatInt(n, 10))}, nil
				}),
				Returns: []*eng.Type{eng.TString}, BarrierPos: -1,
			},
			{
				Args: []*eng.Type{eng.TString},
				Impl: eng.Go(func(args []eng.Value, _ map[string]eng.Value, _ []eng.Value, _ *eng.Registry) ([]eng.Value, error) {
					s, _ := eng.AsString(args[0])
					return []eng.Value{eng.NewString("str:" + s)}, nil
				}),
				Returns: []*eng.Type{eng.TString}, BarrierPos: -1,
			},
		},
	})
	r.RegisterNativeFunc(eng.NativeFunc{
		Name: "tagq",
		Signatures: []eng.Signature{
			{Args: []*eng.Type{eng.TAny}, Impl: eng.Go(func(_ []eng.Value, _ map[string]eng.Value, _ []eng.Value, _ *eng.Registry) ([]eng.Value, error) {
				return []eng.Value{eng.NewString("any")}, nil
			}), Returns: []*eng.Type{eng.TString}, BarrierPos: -1},
			{Args: []*eng.Type{eng.TInteger}, Impl: eng.Go(func(_ []eng.Value, _ map[string]eng.Value, _ []eng.Value, _ *eng.Registry) ([]eng.Value, error) {
				return []eng.Value{eng.NewString("specific")}, nil
			}), Returns: []*eng.Type{eng.TString}, BarrierPos: -1},
		},
	})
	r.RegisterNativeFunc(eng.NativeFunc{
		Name: "factq",
		Signatures: []eng.Signature{
			{
				Args: []*eng.Type{eng.TInteger}, Patterns: map[int]eng.Value{0: eng.NewInteger(0)},
				Impl: eng.Go(func(_ []eng.Value, _ map[string]eng.Value, _ []eng.Value, _ *eng.Registry) ([]eng.Value, error) {
					return []eng.Value{eng.NewInteger(1)}, nil
				}),
				Returns: []*eng.Type{eng.TInteger}, BarrierPos: -1,
			},
			{
				Args: []*eng.Type{eng.TInteger},
				Impl: eng.Go(func(args []eng.Value, _ map[string]eng.Value, _ []eng.Value, _ *eng.Registry) ([]eng.Value, error) {
					n, _ := eng.AsInteger(args[0])
					return []eng.Value{eng.NewInteger(n)}, nil
				}),
				Returns: []*eng.Type{eng.TInteger}, BarrierPos: -1,
			},
		},
	})
	r.RegisterNativeFunc(eng.NativeFunc{
		Name: "codeq",
		Signatures: []eng.Signature{
			{
				Args: []*eng.Type{eng.TInteger}, Patterns: map[int]eng.Value{0: eng.NewInteger(99)},
				Impl: eng.Go(func(_ []eng.Value, _ map[string]eng.Value, _ []eng.Value, _ *eng.Registry) ([]eng.Value, error) {
					return []eng.Value{eng.NewString("ninety-nine")}, nil
				}),
				Returns: []*eng.Type{eng.TString}, BarrierPos: -1,
			},
			{
				Args: []*eng.Type{eng.TInteger},
				Impl: eng.Go(func(_ []eng.Value, _ map[string]eng.Value, _ []eng.Value, _ *eng.Registry) ([]eng.Value, error) {
					return []eng.Value{eng.NewString("general")}, nil
				}),
				Returns: []*eng.Type{eng.TString}, BarrierPos: -1,
			},
		},
	})
	r.RegisterNativeFunc(eng.NativeFunc{
		Name: "routeq",
		Signatures: []eng.Signature{
			{
				Args: []*eng.Type{eng.TString}, Patterns: map[int]eng.Value{0: eng.NewString("admin")},
				Impl: eng.Go(func(_ []eng.Value, _ map[string]eng.Value, _ []eng.Value, _ *eng.Registry) ([]eng.Value, error) {
					return []eng.Value{eng.NewString("matched-admin")}, nil
				}),
				Returns: []*eng.Type{eng.TString}, BarrierPos: -1,
			},
			{
				Args: []*eng.Type{eng.TString},
				Impl: eng.Go(func(_ []eng.Value, _ map[string]eng.Value, _ []eng.Value, _ *eng.Registry) ([]eng.Value, error) {
					return []eng.Value{eng.NewString("other")}, nil
				}),
				Returns: []*eng.Type{eng.TString}, BarrierPos: -1,
			},
		},
	})
	r.RegisterNativeFunc(eng.NativeFunc{
		Name: "tripq",
		Signatures: []eng.Signature{{
			Args: []*eng.Type{eng.TInteger, eng.TInteger, eng.TInteger},
			Impl: eng.Go(func(args []eng.Value, _ map[string]eng.Value, _ []eng.Value, _ *eng.Registry) ([]eng.Value, error) {
				a, _ := eng.AsInteger(args[0])
				b, _ := eng.AsInteger(args[1])
				c, _ := eng.AsInteger(args[2])
				return []eng.Value{eng.NewString(fmt.Sprintf("%d,%d,%d", a, b, c))}, nil
			}),
			Returns: []*eng.Type{eng.TString}, BarrierPos: -1,
		}},
	})
	r.RegisterNativeFunc(eng.NativeFunc{
		Name: "pairq",
		Signatures: []eng.Signature{{
			Args:       []*eng.Type{eng.TInteger, eng.TInteger},
			BarrierPos: 1,
			Impl: eng.Go(func(args []eng.Value, _ map[string]eng.Value, _ []eng.Value, _ *eng.Registry) ([]eng.Value, error) {
				a, _ := eng.AsInteger(args[0])
				b, _ := eng.AsInteger(args[1])
				return []eng.Value{eng.NewString(fmt.Sprintf("%d:%d", a, b))}, nil
			}),
			Returns: []*eng.Type{eng.TString},
		}},
	})

	// ── Barrier / arity fixtures (for barrier.tsv) ────────────────
	// nilq — a 0-arg word. Exercises 0-arity sigs and the `/0`
	// argCount filter (the fallback-section match path).
	r.RegisterNativeFunc(eng.NativeFunc{
		Name: "nilq",
		Signatures: []eng.Signature{{
			Args: []*eng.Type{},
			Impl: eng.Go(func(_ []eng.Value, _ map[string]eng.Value, _ []eng.Value, _ *eng.Registry) ([]eng.Value, error) {
				return []eng.Value{eng.NewString("nil")}, nil
			}),
			Returns: []*eng.Type{eng.TString}, BarrierPos: 0,
		}},
	})

	// flexq — two overloads of different arity, [Integer] and
	// [Integer, Integer], both forward-eligible (BarrierPos = N). The
	// 1-arg sig is tried first, so a bare `flexq` always picks it; the
	// `/N` argCount modifier (flexq/1, flexq/2) selects the overload
	// explicitly, and `/1f`, `/2s` etc. combine arity selection with a
	// forced forward/stack boundary.
	r.RegisterNativeFunc(eng.NativeFunc{
		Name: "flexq",
		Signatures: []eng.Signature{
			{
				Args: []*eng.Type{eng.TInteger},
				Impl: eng.Go(func(args []eng.Value, _ map[string]eng.Value, _ []eng.Value, _ *eng.Registry) ([]eng.Value, error) {
					a, _ := eng.AsInteger(args[0])
					return []eng.Value{eng.NewString(fmt.Sprintf("one:%d", a))}, nil
				}),
				Returns: []*eng.Type{eng.TString}, BarrierPos: -1,
			},
			{
				Args: []*eng.Type{eng.TInteger, eng.TInteger},
				Impl: eng.Go(func(args []eng.Value, _ map[string]eng.Value, _ []eng.Value, _ *eng.Registry) ([]eng.Value, error) {
					a, _ := eng.AsInteger(args[0])
					b, _ := eng.AsInteger(args[1])
					return []eng.Value{eng.NewString(fmt.Sprintf("two:%d,%d", a, b))}, nil
				}),
				Returns: []*eng.Type{eng.TString}, BarrierPos: -1,
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
			Name: name,
			Signatures: []eng.Signature{{
				Args: args, BarrierPos: barrier,
				Impl:    eng.Go(intArgsFmt),
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
		Name: "lengthq",
		Signatures: []eng.Signature{{
			Args: []*eng.Type{eng.TList},
			Impl: eng.Go(func(args []eng.Value, _ map[string]eng.Value, _ []eng.Value, _ *eng.Registry) ([]eng.Value, error) {
				lst, _ := eng.AsList(args[0])
				return []eng.Value{eng.NewInteger(int64(lst.Len()))}, nil
			}),
			Returns: []*eng.Type{eng.TInteger}, BarrierPos: -1,
		}},
	})
	r.RegisterNativeFunc(eng.NativeFunc{
		Name: "firstq",
		Signatures: []eng.Signature{{
			Args: []*eng.Type{eng.TList},
			Impl: eng.Go(func(args []eng.Value, _ map[string]eng.Value, _ []eng.Value, _ *eng.Registry) ([]eng.Value, error) {
				lst, _ := eng.AsList(args[0])
				if lst.Len() == 0 {
					return []eng.Value{eng.NewNone()}, nil
				}
				return []eng.Value{lst.Get(0)}, nil
			}),
			Returns: []*eng.Type{eng.TAny}, BarrierPos: -1,
		}},
	})
	r.RegisterNativeFunc(eng.NativeFunc{
		Name: "replayq",
		Signatures: []eng.Signature{{
			Args:       []*eng.Type{eng.TList},
			NoEvalArgs: map[int]bool{0: true},
			Impl: eng.Go(func(args []eng.Value, _ map[string]eng.Value, _ []eng.Value, _ *eng.Registry) ([]eng.Value, error) {
				_lst, _ := eng.AsList(args[0])
				body := _lst.Slice()
				specReplayCounter++
				id := fmt.Sprintf("__replayq_%d", specReplayCounter)
				out := make([]eng.Value, 0, len(body)+2)
				out = append(out, eng.NewMark(id, body...))
				out = append(out, body...)
				out = append(out, eng.NewMove(id, "replayq"))
				return out, nil
			}), BarrierPos: -1,
		}},
	})

	r.Defs.Push("pi", eng.NewInteger(3))
	r.Defs.Push("tau", eng.NewInteger(6))
	r.Defs.Push("greeting", eng.NewString("hello"))

	// break / continue — the production words live in lang
	// (lang/go/engine/native_control.go); for engspec we register
	// kernel-side stubs that signal Registry.FlowCtrl so the
	// interp.tsv "break outside loop" rows exercise the Run-loop
	// dispatch (which IS kernel territory) without dragging the
	// whole lang word set into the engspec setup.
	r.RegisterNativeFunc(eng.NativeFunc{
		Name: "break",
		Signatures: []eng.Signature{{
			Impl: eng.Go(func(_ []eng.Value, _ map[string]eng.Value, _ []eng.Value, r *eng.Registry) ([]eng.Value, error) {
				r.FlowCtrl = eng.FlowBreak
				return nil, nil
			}),
			Returns: []*eng.Type{}, BarrierPos: 0,
		}},
	})
	r.RegisterNativeFunc(eng.NativeFunc{
		Name: "continue",
		Signatures: []eng.Signature{{
			Impl: eng.Go(func(_ []eng.Value, _ map[string]eng.Value, _ []eng.Value, r *eng.Registry) ([]eng.Value, error) {
				r.FlowCtrl = eng.FlowContinue
				return nil, nil
			}),
			Returns: []*eng.Type{}, BarrierPos: 0,
		}},
	})

	// not / and / or — production registrations live in
	// lang/go/engine/native_boolean.go. The eng kernel exposes only the
	// CoerceBoolean primitive; engspec wires it into bare not/and/or
	// names so the existing eng/spec tsvs (forth.tsv, inspect.tsv,
	// types.tsv, …) keep exercising the dispatch path.
	registerEngSpecBoolean(r)
	registerEngSpecTypeOps(r)
	registerEngSpecDo(r)
	registerEngSpecFnSig(r)
	registerEngSpecObjectRecord(r)
	registerEngSpecStorage(r)
	registerEngSpecInspect(r)
	registerEngSpecMake(r)
	registerEngSpecTypeWords(r)
	registerEngSpecDefinition(r)
	registerEngSpecStack(r)
}

// registerEngSpecDefinition installs def / fn / quote / args as
// spec-runner fixtures. Production registrations live in
// lang/go/engine/native_definition.go; engspec delegates to the same
// algorithm helpers in eng (registerCoreDef etc. retained in
// eng/go/core_words.go as in-package test infrastructure are not
// reachable from this package — so engspec wires its own NativeFunc
// dispatch through the public eng API).
func registerEngSpecDefinition(r *eng.Registry) {
	// def name body — plain and typed forms. The production version in
	// lang handles richer FnDef installation; engspec ships the bare
	// "push body onto def stack after name validation" semantics, which
	// is enough for the eng/spec tsv rows that exercise simple bindings.
	plainDef := func(args []eng.Value, _ map[string]eng.Value, _ []eng.Value, reg *eng.Registry) ([]eng.Value, error) {
		name, _ := args[0].AsConcreteAtom()
		// `def` is the universal binder (TYPE-UNIFORM Phase 2): a
		// capitalised name is a TYPE binding — delegate to InstallType,
		// the same path the `refine` word uses.
		if eng.IsCapitalisedName(name) {
			return nil, eng.InstallType(reg, name, args[1])
		}
		if err := eng.ValidateWordName(name); err != nil {
			return nil, err
		}
		reg.Check.RecordDef(name, eng.SrcPos{})
		if info, ok := args[1].Data.(eng.FnDefInfo); ok {
			eng.InstallFnDef(reg, name, info)
			return nil, nil
		}
		reg.Defs.Push(name, args[1])
		return nil, nil
	}
	typedDef := func(args []eng.Value, _ map[string]eng.Value, _ []eng.Value, reg *eng.Registry) ([]eng.Value, error) {
		nameMap, _ := eng.AsMap(args[0])
		if nameMap == nil || nameMap.Len() != 1 {
			return nil, &eng.AqlError{Code: "type_error", Detail: "def: typed-name map must have exactly one key"}
		}
		name := nameMap.Keys()[0]
		if err := eng.ValidateWordName(name); err != nil {
			return nil, err
		}
		if reg.Defs.IsType(name) {
			return nil, &eng.AqlError{Code: "type_error", Detail: "def " + name + ": name clash — already a type"}
		}
		constraint, _ := nameMap.Get(name)
		if resolved, _, _ := reg.ResolveTypedNameValue(constraint); resolved.Data != nil || resolved.Parent != nil {
			constraint = resolved
		}
		body := args[1]
		if !eng.IsValueOfType(body, constraint) {
			return nil, &eng.AqlError{
				Code:   "type_error",
				Detail: "def " + name + ": value " + body.String() + " does not satisfy declared type " + constraint.String(),
			}
		}
		if info, ok := body.Data.(eng.FnDefInfo); ok {
			eng.InstallFnDef(reg, name, info)
			return nil, nil
		}
		reg.Defs.Push(name, body)
		return nil, nil
	}
	// `def name word BODY` — the Forth-style splice binder. `word` is a
	// KEYWORD SLOT (quoted, pattern-matched), so def captures it structurally
	// instead of stranding on it under the strict forward-barrier. Mirrors
	// the lang layer's synthesized keyword form (registerDefKeywordForms);
	// the eng fixture spells it directly since it has no lang registry.
	defWord := func(args []eng.Value, _ map[string]eng.Value, _ []eng.Value, reg *eng.Registry) ([]eng.Value, error) {
		name, _ := args[0].AsConcreteAtom()
		if err := eng.ValidateWordName(name); err != nil {
			return nil, err
		}
		reg.Check.RecordDef(name, eng.SrcPos{})
		reg.Defs.Push(name, eng.NewSplice(args[2]))
		return nil, nil
	}
	// `def name fn [triples]` — the fn KEYWORD SLOT. Captures `fn`
	// structurally and binds the FnDef by name, mirroring the lang keyword
	// form. Without it, `def name fn […]` would strand on `fn` under strict,
	// and paren-wrapping (`def name (fn […])`) is WRONG for a fn with a
	// 0-arg overload — the group auto-invokes the nullary sig instead of
	// yielding the FnDef value.
	defFn := func(args []eng.Value, _ map[string]eng.Value, _ []eng.Value, reg *eng.Registry) ([]eng.Value, error) {
		name, _ := args[0].AsConcreteAtom()
		if err := eng.ValidateWordName(name); err != nil {
			return nil, err
		}
		if !eng.IsConcrete(args[2]) {
			return nil, &eng.AqlError{Code: "type_error", Detail: "def fn: body must be a concrete list"}
		}
		lst, _ := eng.AsList(args[2])
		elems := lst.Slice()
		if len(elems) == 0 || len(elems)%3 != 0 {
			return nil, &eng.AqlError{Code: "fn_invalid_spec", Detail: "fn: list length must be a non-zero multiple of 3"}
		}
		fnDef, err := eng.ParseFnDef(reg, elems)
		if err != nil {
			return nil, err
		}
		reg.Check.RecordDef(name, eng.SrcPos{})
		eng.InstallFnDef(reg, name, fnDef)
		return nil, nil
	}
	r.RegisterNativeFunc(eng.NativeFunc{
		Name: "def",

		Signatures: []eng.Signature{
			{
				Args:       []*eng.Type{eng.TAtom, eng.TAtom, eng.TAny},
				QuoteArgs:  map[int]bool{0: true, 1: true},
				Patterns:   map[int]eng.Value{1: eng.NewAtom("word")},
				NoEvalArgs: map[int]bool{2: true},
				Impl:       eng.Go(defWord, eng.RunInCheck()),
				Returns:    []*eng.Type{}, BarrierPos: -1,
			},
			{
				Args:       []*eng.Type{eng.TAtom, eng.TAtom, eng.TList},
				QuoteArgs:  map[int]bool{0: true, 1: true},
				Patterns:   map[int]eng.Value{1: eng.NewAtom("fn")},
				NoEvalArgs: map[int]bool{2: true},
				Impl:       eng.Go(defFn, eng.RunInCheck()),
				Returns:    []*eng.Type{}, BarrierPos: -1,
			},
			{
				Args:          []*eng.Type{eng.TMap, eng.TAny},
				NoEvalMapArgs: map[int]bool{0: true},
				Impl:          eng.Go(typedDef),
				Returns:       []*eng.Type{}, BarrierPos: -1,
			},
			{
				Args:      []*eng.Type{eng.TAtom, eng.TAny},
				QuoteArgs: map[int]bool{0: true},
				Impl:      eng.Go(plainDef, eng.RunInCheck()),
				Returns:   []*eng.Type{}, BarrierPos: -1,
			},
		},
	})

	// word VALUE — wrap the (unevaluated) arg in an __SP splice marker (the
	// kernel splice mechanism). `def name word value` binds the marker so a
	// reference splices its payload. Production registration lives in
	// lang/go/native/natives.go; engspec ships its own fixture so eng/spec
	// rows (forth.tsv etc.) can express splices after the def-list flip.
	r.RegisterNativeFunc(eng.NativeFunc{
		Name: "word",
		Signatures: []eng.Signature{{
			Args:       []*eng.Type{eng.TAny},
			NoEvalArgs: map[int]bool{0: true},
			Impl: eng.Go(func(args []eng.Value, _ map[string]eng.Value, _ []eng.Value, _ *eng.Registry) ([]eng.Value, error) {
				return []eng.Value{eng.NewSplice(args[0])}, nil
			}),
			Returns: []*eng.Type{eng.TAny}, BarrierPos: -1,
		}},
	})

	// fn [triples] — uses ParseFnDef from eng to build the FnDef.
	r.RegisterNativeFunc(eng.NativeFunc{
		Name: "fn",

		Signatures: []eng.Signature{{
			Args:       []*eng.Type{eng.TList},
			NoEvalArgs: map[int]bool{0: true},
			Impl: eng.Go(func(args []eng.Value, _ map[string]eng.Value, _ []eng.Value, reg *eng.Registry) ([]eng.Value, error) {
				if !eng.IsConcrete(args[0]) {
					return nil, &eng.AqlError{Code: "type_error", Detail: "fn: argument must be a concrete list"}
				}
				lst, _ := eng.AsList(args[0])
				elems := lst.Slice()
				if len(elems) == 0 || len(elems)%3 != 0 {
					return nil, &eng.AqlError{Code: "fn_invalid_spec", Detail: "fn: list length must be a non-zero multiple of 3"}
				}
				fnDef, err := eng.ParseFnDef(reg, elems)
				if err != nil {
					return nil, err
				}
				return []eng.Value{eng.NewFunction(fnDef)}, nil
			}, eng.RunInCheck()),
			Returns:    []*eng.Type{eng.TFunction},
			BarrierPos: -1,
		}},
	})

	// quote VALUE — atom-quoted and any-value forms.
	r.RegisterNativeFunc(eng.NativeFunc{
		Name: "quote",

		Signatures: []eng.Signature{
			{
				Args:      []*eng.Type{eng.TAtom},
				QuoteArgs: map[int]bool{0: true},
				Impl: eng.Go(func(args []eng.Value, _ map[string]eng.Value, _ []eng.Value, _ *eng.Registry) ([]eng.Value, error) {
					return []eng.Value{args[0]}, nil
				}),
				Returns: []*eng.Type{eng.TAtom}, BarrierPos: -1,
			},
			{
				Args:       []*eng.Type{eng.TAny},
				NoEvalArgs: map[int]bool{0: true},
				Impl: eng.Go(func(args []eng.Value, _ map[string]eng.Value, _ []eng.Value, _ *eng.Registry) ([]eng.Value, error) {
					v := args[0]
					if v.Parent.Equal(eng.TList) && v.Data != nil {
						v.Quoted = true
					}
					return []eng.Value{v}, nil
				}),
				Returns: []*eng.Type{eng.TAny}, BarrierPos: -1,
			},
		},
	})

	// args — return the current fn-call's args frame as a List.
	r.RegisterNativeFunc(eng.NativeFunc{
		Name: "args",
		Signatures: []eng.Signature{{
			Impl: eng.Go(func(_ []eng.Value, _ map[string]eng.Value, _ []eng.Value, reg *eng.Registry) ([]eng.Value, error) {
				v, ok, err := reg.Args.Top()
				if err != nil {
					return nil, err
				}
				if !ok {
					return []eng.Value{eng.NewList(nil)}, nil
				}
				return []eng.Value{v}, nil
			}),
			Returns: []*eng.Type{eng.TList}, BarrierPos: 0,
		}},
	})

	// __pa — engine-internal args-frame cleanup marker emitted by
	// eng.InstallFnDef's expansion. Pops the top args frame from the
	// per-fn-call argsStack. The production version lives at
	// lang/go/engine/native_definition.go::popArgsHandler.
	r.RegisterNativeFunc(eng.NativeFunc{
		Name: "__pa",

		Signatures: []eng.Signature{{
			Impl: eng.Go(func(_ []eng.Value, _ map[string]eng.Value, _ []eng.Value, reg *eng.Registry) ([]eng.Value, error) {
				if _, err := reg.Args.Pop(); err != nil {
					return nil, err
				}
				return nil, nil
			}),
			Returns: []*eng.Type{}, BarrierPos: -1,
		}},
	})

	// undef NAME — pop the named binding from the def stack. Also
	// emitted by eng.InstallFnDef's body expansion to clean up
	// per-call parameter bindings.
	r.RegisterNativeFunc(eng.NativeFunc{
		Name: "undef",

		Signatures: []eng.Signature{{
			Args:      []*eng.Type{eng.TAtom},
			QuoteArgs: map[int]bool{0: true},
			Impl: eng.Go(func(args []eng.Value, _ map[string]eng.Value, _ []eng.Value, reg *eng.Registry) ([]eng.Value, error) {
				name, _ := args[0].AsConcreteAtom()
				// Universal unbinder: a capitalised name pops the type
				// binding from the single store, mirroring `def`.
				if eng.IsCapitalisedName(name) {
					entry, ok := reg.Defs.PopEntry(name)
					if !ok {
						return nil, &eng.AqlError{Code: "type_error", Detail: "undef " + name + ": no such type binding"}
					}
					if entry.TypeDef != nil {
						reg.Types.Retire(entry.TypeDef)
					}
					return nil, nil
				}
				eng.UninstallDef(reg, name)
				return nil, nil
			}),
			Returns: []*eng.Type{}, BarrierPos: -1,
		}},
	})
}

// registerEngSpecStack installs the Forth-style stack manipulators —
// dup / swap / drop / over / rot / nip / tuck / dup2 / swap2 / drop2
// / over2 — as spec-runner fixtures. Production registrations live
// in lang/go/engine/native_stack.go.
//
// All ops are stack-only (every sig sets `BarrierPos: 0`) —
// args[0] is the top of stack, args[1] the next-deeper, etc. The
// handler returns the splice-form replacement for the consumed
// args, in source order.
func registerEngSpecStack(r *eng.Registry) {
	op := func(name string, argCount int, fn func(args []eng.Value) []eng.Value) {
		args := make([]*eng.Type, argCount)
		for i := range args {
			args[i] = eng.TAny
		}
		r.RegisterNativeFunc(eng.NativeFunc{
			Name: name,
			Signatures: []eng.Signature{{
				Args: args,
				Impl: eng.Go(func(args []eng.Value, _ map[string]eng.Value, _ []eng.Value, _ *eng.Registry) ([]eng.Value, error) {
					return fn(args), nil
				}),
				Returns: []*eng.Type{}, BarrierPos: 0,
			}},
		})
	}
	op("dup", 1, func(a []eng.Value) []eng.Value { return []eng.Value{a[0], a[0]} })
	op("drop", 1, func(_ []eng.Value) []eng.Value { return nil })
	op("swap", 2, func(a []eng.Value) []eng.Value { return []eng.Value{a[0], a[1]} })
	op("over", 2, func(a []eng.Value) []eng.Value { return []eng.Value{a[1], a[0], a[1]} })
	op("rot", 3, func(a []eng.Value) []eng.Value { return []eng.Value{a[1], a[0], a[2]} })
	op("nip", 2, func(a []eng.Value) []eng.Value { return []eng.Value{a[0]} })
	op("tuck", 2, func(a []eng.Value) []eng.Value { return []eng.Value{a[0], a[1], a[0]} })
	op("dup2", 2, func(a []eng.Value) []eng.Value { return []eng.Value{a[1], a[0], a[1], a[0]} })
	op("swap2", 4, func(a []eng.Value) []eng.Value { return []eng.Value{a[1], a[0], a[3], a[2]} })
	op("drop2", 2, func(_ []eng.Value) []eng.Value { return nil })
	op("over2", 4, func(a []eng.Value) []eng.Value { return []eng.Value{a[3], a[2], a[1], a[0], a[3], a[2]} })
}

// registerEngSpecTypeWords installs `type` / `untype` / `typeof` /
// `pathof` / `is` / `enum` as spec-runner fixtures using the eng-
// exported algorithm primitives (InstallType, TypeOf, PathOf,
// IsValueOfType, NewEnum). Production registrations live in
// lang/go/engine/native_type.go.
func registerEngSpecTypeWords(r *eng.Registry) {
	r.RegisterNativeFunc(eng.NativeFunc{
		Name: "typeof",

		Signatures: []eng.Signature{{
			Args: []*eng.Type{eng.TAny},
			Impl: eng.Go(func(args []eng.Value, _ map[string]eng.Value, _ []eng.Value, _ *eng.Registry) ([]eng.Value, error) {
				return []eng.Value{eng.TypeOf(args[0])}, nil
			}),
			Returns: []*eng.Type{eng.TType}, BarrierPos: -1,
		}},
	})
	r.RegisterNativeFunc(eng.NativeFunc{
		Name: "pathof",

		Signatures: []eng.Signature{{
			Args:     []*eng.Type{eng.TAny},
			TypeArgs: map[int]bool{0: true},
			Impl: eng.Go(func(args []eng.Value, _ map[string]eng.Value, _ []eng.Value, _ *eng.Registry) ([]eng.Value, error) {
				return []eng.Value{eng.PathOf(args[0])}, nil
			}),
			Returns: []*eng.Type{eng.TList}, BarrierPos: -1,
		}},
	})
	r.RegisterNativeFunc(eng.NativeFunc{
		Name: "is",

		Signatures: []eng.Signature{{
			Args:       []*eng.Type{eng.TAny, eng.TAny},
			BarrierPos: 1,
			Impl: eng.Go(func(args []eng.Value, _ map[string]eng.Value, _ []eng.Value, _ *eng.Registry) ([]eng.Value, error) {
				return []eng.Value{eng.NewBoolean(eng.IsValueOfType(args[1], args[0]))}, nil
			}),
			Returns: []*eng.Type{eng.TBoolean},
		}},
	})
	r.RegisterNativeFunc(eng.NativeFunc{
		Name: "enum",

		Signatures: []eng.Signature{{
			Args:       []*eng.Type{eng.TList},
			NoEvalArgs: map[int]bool{0: true},
			Impl: eng.Go(func(args []eng.Value, _ map[string]eng.Value, _ []eng.Value, _ *eng.Registry) ([]eng.Value, error) {
				list := args[0]
				if !eng.IsConcrete(list) {
					return nil, &eng.AqlError{Code: "type_error", Detail: "enum: argument must be a concrete list"}
				}
				var childType eng.Value
				hasChild := false
				if eng.IsTypedList(list) {
					ci, _ := eng.AsChildType(list)
					childType = ci.Child
					hasChild = childType.Parent != nil
				}
				elems, _ := eng.AsList(list)
				alts := make([]eng.Value, 0, elems.Len())
				for i := 0; i < elems.Len(); i++ {
					e := elems.Get(i)
					if eng.IsWord(e) {
						w, _ := eng.AsWord(e)
						e = eng.NewAtom(w.Name)
					}
					if hasChild && !eng.IsValueOfType(e, childType) {
						return nil, &eng.AqlError{
							Code:   "type_error",
							Detail: "enum: element " + e.String() + " does not satisfy child type " + childType.String(),
						}
					}
					alts = append(alts, e)
				}
				return []eng.Value{eng.NewEnum(alts)}, nil
			}),
			Returns: []*eng.Type{eng.TEnum}, BarrierPos: -1,
		}},
	})
}

// registerEngSpecMake installs `make` as a spec-runner fixture
// using the eng-exported algorithm handlers. Production
// registration lives in lang/go/engine/native_make.go.
func registerEngSpecMake(r *eng.Registry) {
	r.RegisterNativeFunc(eng.NativeFunc{
		Name: "make",

		Signatures: []eng.Signature{
			{Args: []*eng.Type{eng.TScalar, eng.TMap, eng.TAny}, TypeArgs: map[int]bool{0: true}, Impl: eng.Go(eng.MakeScalarOptsHandler), ReturnsFn: eng.ReturnsFreshInstance(0), BarrierPos: -1},
			{Args: []*eng.Type{eng.TIdeal, eng.TMap}, TypeArgs: map[int]bool{0: true}, Impl: eng.Go(eng.MakeObjHandler), ReturnsFn: eng.ReturnsFreshInstance(0), BarrierPos: -1},
			{Args: []*eng.Type{eng.TScalar, eng.TAny}, TypeArgs: map[int]bool{0: true}, Impl: eng.Go(eng.MakeScalarHandler), ReturnsFn: eng.ReturnsFreshInstance(0), BarrierPos: -1},
			{Args: []*eng.Type{eng.TAny, eng.TAny, eng.TMap}, Impl: eng.Go(eng.MakeWithOpts), Returns: []*eng.Type{eng.TAny}, BarrierPos: -1},
			{Args: []*eng.Type{eng.TAny, eng.TAny}, Impl: eng.Go(eng.MakeHandler), Returns: []*eng.Type{eng.TAny}, BarrierPos: -1},
		},
	})
}

// registerEngSpecStorage installs the kernel-container `get` and
// `set` signatures (Node / Object / Array / None) as spec-runner
// fixtures. The production registration in
// lang/go/engine/native_storage.go adds Store-side sigs on top of
// these; engspec mirrors only the kernel slice so eng/spec rows
// that inspect `get` / `set` see the expected shape.
func registerEngSpecStorage(r *eng.Registry) {
	setObjectH := func(args []eng.Value, _ map[string]eng.Value, _ []eng.Value, _ *eng.Registry) ([]eng.Value, error) {
		container := args[2]
		if !eng.IsConcrete(container) {
			return nil, fmt.Errorf("set: cannot set field on type literal")
		}
		key := eng.StoreKey(args[0])
		oi, ok := container.Data.(eng.ClassInstanceInfo)
		if !ok {
			return nil, fmt.Errorf("set: expected an Object instance, got %s", container.Parent.String())
		}
		oi.Fields.Set(key, args[1])
		return nil, nil
	}
	getNodeH := func(args []eng.Value, _ map[string]eng.Value, _ []eng.Value, _ *eng.Registry) ([]eng.Value, error) {
		key := args[0]
		container := args[1]
		if !eng.IsConcrete(container) {
			return nil, fmt.Errorf("get: cannot access property on type literal")
		}
		if key.Parent.ConformsTo(eng.TInteger) {
			idx, _ := eng.AsInteger(key)
			if list, _ := eng.AsList(container); !list.IsNil() && container.Parent.ConformsTo(eng.TList) {
				i := int(idx)
				if i < 0 || i >= list.Len() {
					return []eng.Value{eng.NewTypeLiteral(eng.TNone)}, nil
				}
				return []eng.Value{list.Get(i)}, nil
			}
		}
		k := eng.GetKey(key)
		if m, _ := eng.AsMap(container); m != nil {
			val, ok := m.Get(k)
			if !ok {
				return []eng.Value{eng.NewTypeLiteral(eng.TNone)}, nil
			}
			return []eng.Value{val}, nil
		}
		return []eng.Value{eng.NewTypeLiteral(eng.TNone)}, nil
	}
	getObjectH := func(args []eng.Value, _ map[string]eng.Value, _ []eng.Value, _ *eng.Registry) ([]eng.Value, error) {
		key := args[0]
		container := args[1]
		if !eng.IsConcrete(container) {
			return nil, fmt.Errorf("get: cannot access property on type literal")
		}
		k := eng.GetKey(key)
		if m, err := eng.AsMutableMap(container); err == nil {
			val, found := m.Get(k)
			if !found {
				return []eng.Value{eng.NewTypeLiteral(eng.TNone)}, nil
			}
			return []eng.Value{val}, nil
		}
		oi, _ := eng.AsClassInstance(container)
		val, ok := oi.GetField(k)
		if !ok {
			return []eng.Value{eng.NewTypeLiteral(eng.TNone)}, nil
		}
		return []eng.Value{val}, nil
	}
	getNoneH := func(_ []eng.Value, _ map[string]eng.Value, _ []eng.Value, _ *eng.Registry) ([]eng.Value, error) {
		return []eng.Value{eng.NewTypeLiteral(eng.TNone)}, nil
	}
	r.RegisterNativeFunc(eng.NativeFunc{
		Name: "set",

		Signatures: []eng.Signature{
			{Args: []*eng.Type{eng.TString, eng.TAny, eng.TClass}, Impl: eng.Go(setObjectH), Returns: []*eng.Type{}, BarrierPos: -1},
			{Args: []*eng.Type{eng.TAtom, eng.TAny, eng.TClass}, QuoteArgs: map[int]bool{0: true}, Impl: eng.Go(setObjectH), Returns: []*eng.Type{}, BarrierPos: -1},
		},
	})
	r.RegisterNativeFunc(eng.NativeFunc{
		Name: "get",

		Signatures: []eng.Signature{
			{Args: []*eng.Type{eng.TAtom, eng.TNode}, QuoteArgs: map[int]bool{0: true}, BarrierPos: 1, Impl: eng.Go(getNodeH), Returns: []*eng.Type{eng.TAny}},
			{Args: []*eng.Type{eng.TString, eng.TNode}, BarrierPos: 1, Impl: eng.Go(getNodeH), Returns: []*eng.Type{eng.TAny}},
			{Args: []*eng.Type{eng.TInteger, eng.TNode}, BarrierPos: 1, Impl: eng.Go(getNodeH), Returns: []*eng.Type{eng.TAny}},
			{Args: []*eng.Type{eng.TAtom, eng.TClass}, QuoteArgs: map[int]bool{0: true}, BarrierPos: 1, Impl: eng.Go(getObjectH), Returns: []*eng.Type{eng.TAny}},
			{Args: []*eng.Type{eng.TString, eng.TClass}, BarrierPos: 1, Impl: eng.Go(getObjectH), Returns: []*eng.Type{eng.TAny}},
			{Args: []*eng.Type{eng.TInteger, eng.TClass}, BarrierPos: 1, Impl: eng.Go(getObjectH), Returns: []*eng.Type{eng.TAny}},
			{Args: []*eng.Type{eng.TAny, eng.TNone}, BarrierPos: 1, Impl: eng.Go(getNoneH), Returns: []*eng.Type{eng.TNone}},
		},
	})
}

// registerEngSpecObjectRecord installs `record` and `object` as
// spec-runner fixtures so the eng/spec/inspect.tsv rows around
// record / object inspection can run against the kernel. Production
// registrations live in lang/go/engine/native_object_record.go.
func registerEngSpecObjectRecord(r *eng.Registry) {
	recordH := func(args []eng.Value, _ map[string]eng.Value, _ []eng.Value, r *eng.Registry) ([]eng.Value, error) {
		list := args[0]
		if !list.Parent.Equal(eng.TList) {
			return nil, fmt.Errorf("record: argument must be a list")
		}
		if !eng.IsConcrete(list) {
			return nil, fmt.Errorf("record: argument must be a concrete list, got type literal")
		}
		elems, _ := eng.AsList(list)
		if elems.Len() == 0 {
			return nil, fmt.Errorf("record: list must have at least one field")
		}
		fields := eng.NewOrderedMap()
		for _, elem := range elems.Slice() {
			if !elem.Parent.Equal(eng.TMap) {
				return nil, fmt.Errorf("record: each element must be a pair (map), got %s", elem.String())
			}
			m, err := eng.AsMutableMap(elem)
			if err != nil {
				return nil, fmt.Errorf("record: each element must be a concrete pair, got %s", elem.String())
			}
			for _, key := range m.Keys() {
				val, _ := m.Get(key)
				val = eng.ResolveFieldType(r, val)
				fields.Set(key, val)
			}
		}
		return []eng.Value{eng.NewRecordType(fields)}, nil
	}
	parseObjectFields := func(fieldsMap *eng.OrderedMap, r *eng.Registry) *eng.OrderedMap {
		fields := eng.NewOrderedMap()
		for _, key := range fieldsMap.Keys() {
			val, _ := fieldsMap.Get(key)
			val = eng.ResolveFieldType(r, val)
			fields.Set(key, val)
		}
		return fields
	}
	objectWithParentH := func(args []eng.Value, _ map[string]eng.Value, _ []eng.Value, r *eng.Registry) ([]eng.Value, error) {
		fieldsVal := args[0]
		parentVal := args[1]
		if !fieldsVal.Parent.Equal(eng.TMap) {
			return nil, fmt.Errorf("object: first argument must be a map of field definitions, got %s", fieldsVal.String())
		}
		m, err := eng.AsMutableMap(fieldsVal)
		if err != nil {
			return nil, fmt.Errorf("object: first argument must be a concrete map, got %s", fieldsVal.String())
		}
		if !eng.IsClassType(parentVal) {
			return nil, fmt.Errorf("object: parent must be an object type, got %s", parentVal.String())
		}
		parentInfo, _ := eng.AsClassType(parentVal)
		fields := parseObjectFields(m, r)
		parentAllFields := parentInfo.AllFields()
		for _, key := range fields.Keys() {
			childConstraint, _ := fields.Get(key)
			parentConstraint, exists := parentAllFields.Get(key)
			if !exists {
				continue
			}
			if _, ok := eng.Unify(parentConstraint, childConstraint); !ok {
				return nil, fmt.Errorf("object: field %q in child type cannot expand parent type %s (child: %s, parent: %s)",
					key, parentInfo.Name, childConstraint.String(), parentConstraint.String())
			}
		}
		id := eng.GenerateObjectTypeID()
		info := eng.ClassTypeInfo{Fields: fields, Parent: &parentInfo, ID: id, Name: ""}
		parentDef := parentInfo.Type
		if parentDef == nil {
			parentDef = eng.TClass
		}
		def := r.Types.MintType(id, parentDef)
		return []eng.Value{eng.NewClassType(def, info)}, nil
	}
	// refine — the uniform type constructor.
	// Mirrors lang/go/native/native_type.go::refineHandler; dispatches
	// to the record/object handler functions above. eng/spec exercises
	// only the Object and Record bases.
	refineCtorH := func(args []eng.Value, _ map[string]eng.Value, _ []eng.Value, reg *eng.Registry) ([]eng.Value, error) {
		base := args[0]
		arg := args[1]
		if eng.IsClassType(base) {
			return objectWithParentH([]eng.Value{arg, base}, nil, nil, reg)
		}
		if eng.IsBareTypeNode(base) && base.Equal(eng.TRecord) {
			if !arg.Parent.Equal(eng.TList) {
				return nil, fmt.Errorf("refine Record: a record takes a list of field pairs")
			}
			return recordH([]eng.Value{arg}, nil, nil, reg)
		}
		return nil, fmt.Errorf("refine: base must be Record or an object type, got %s", base.String())
	}
	// refine BaseType — the 1-arg bare-subtype form. Returns the base
	// type unchanged; the paired `def` mints a fresh subtype of it.
	refineBareCtorH := func(args []eng.Value, _ map[string]eng.Value, _ []eng.Value, _ *eng.Registry) ([]eng.Value, error) {
		base := args[0]
		if !eng.IsTypeBody(base) {
			return nil, fmt.Errorf("refine: argument must be a type, got %s", base.String())
		}
		return []eng.Value{base}, nil
	}
	r.RegisterNativeFunc(eng.NativeFunc{
		Name: "refine",

		Signatures: []eng.Signature{
			{
				Args:       []*eng.Type{eng.TAny, eng.TNode},
				Impl:       eng.Go(refineCtorH, eng.RunInCheck()),
				Returns:    []*eng.Type{eng.TType},
				BarrierPos: -1,
			},
			{
				Args:       []*eng.Type{eng.TAny},
				Impl:       eng.Go(refineBareCtorH, eng.RunInCheck()),
				Returns:    []*eng.Type{eng.TType},
				BarrierPos: -1,
			},
		},
	})
}

// registerEngSpecInspect installs `inspect` as a spec-runner fixture
// that mirrors lang/go/engine/native_inspect.go. The production
// registration lives in lang; engspec keeps a copy so the
// eng/spec/inspect.tsv rows continue to exercise the kernel-only
// dispatch and value-introspection paths.
func registerEngSpecInspect(r *eng.Registry) {
	atomH := func(args []eng.Value, _ map[string]eng.Value, _ []eng.Value, r *eng.Registry) ([]eng.Value, error) {
		name, _ := args[0].AsConcreteAtom()
		if tv, ok := r.TopTypeBody(name); ok {
			return []eng.Value{buildTypeInspection(name, tv)}, nil
		}
		if top, ok := r.Defs.Top(name); ok {
			if eng.IsTypeBody(top) && !top.Parent.Equal(eng.TFnDef) && !top.Parent.Equal(eng.TFunction) {
				return []eng.Value{buildTypeInspection(name, top)}, nil
			}
		}
		return []eng.Value{buildInspection(r, name)}, nil
	}
	typeH := func(args []eng.Value, _ map[string]eng.Value, _ []eng.Value, _ *eng.Registry) ([]eng.Value, error) {
		return []eng.Value{buildTypeInspection("", args[0])}, nil
	}
	r.RegisterNativeFunc(eng.NativeFunc{
		Name: "inspect",

		Signatures: []eng.Signature{
			{Args: []*eng.Type{eng.TAtom}, QuoteArgs: map[int]bool{0: true}, Impl: eng.Go(atomH), Returns: []*eng.Type{eng.TInspect}, BarrierPos: -1},
			{Args: []*eng.Type{eng.TAny}, Impl: eng.Go(typeH), Returns: []*eng.Type{eng.TInspect}, BarrierPos: -1},
		},
	})
}

// buildInspection constructs a word-inspection map for the named word.
func buildInspection(r *eng.Registry, name string) eng.Value {
	result := eng.NewOrderedMap()
	result.Set("name", eng.NewString(name))

	fn := r.Lookup(name)
	if fn == nil {
		if r.Defs.Has(name) {
			result.Set("kind", eng.NewAtom("defined"))
			if v, ok := r.Defs.Top(name); ok {
				result.Set("value", v)
			}
			result.Set("signatures", eng.NewList(nil))
			return eng.NewValueRaw(eng.TInspect, eng.MapPayload{M: result})
		}
		result.Set("kind", eng.NewAtom("unknown"))
		result.Set("signatures", eng.NewList(nil))
		return eng.NewValueRaw(eng.TInspect, eng.MapPayload{M: result})
	}

	isDefined := false
	for _, s := range fn.OwnSigs() {
		if len(s.Body()) > 0 {
			isDefined = true
			break
		}
	}
	if isDefined {
		result.Set("kind", eng.NewAtom("defined"))
	} else {
		result.Set("kind", eng.NewAtom("native"))
	}

	var sigMaps []eng.Value
	for _, sig := range fn.Signatures {
		sm := eng.NewOrderedMap()
		var argVals []eng.Value
		for _, argType := range sig.Args {
			argVals = append(argVals, eng.NewString(argType.Leaf()))
		}
		if argVals == nil {
			argVals = []eng.Value{}
		}
		sm.Set("args", eng.NewList(argVals))
		sigMaps = append(sigMaps, eng.NewMap(sm))
	}
	if sigMaps == nil {
		sigMaps = []eng.Value{}
	}
	result.Set("signatures", eng.NewList(sigMaps))

	return eng.NewValueRaw(eng.TInspect, eng.MapPayload{M: result})
}

// buildTypeInspection constructs a type-inspection map for a type value.
func buildTypeInspection(name string, tv eng.Value) eng.Value {
	result := eng.NewOrderedMap()

	if name != "" {
		result.Set("name", eng.NewString(name))
	}

	if eng.IsBareTypeNode(tv) || eng.IsTypeBody(tv) || eng.IsRecordShape(tv) {
		result.Set("type", eng.NewString("Type"))
		result.Set("struct", eng.NewString(eng.TypeNameOf(tv)))
	} else {
		result.Set("type", eng.NewString(tv.Parent.Leaf()))
	}

	switch {
	case eng.IsRecordType(tv):
		result.Set("kind", eng.NewAtom("record"))
		rt, _ := eng.AsRecordType(tv)
		fields := eng.NewOrderedMap()
		for _, k := range rt.Fields.Keys() {
			v, _ := rt.Fields.Get(k)
			fields.Set(k, eng.NewString(eng.TypeNameOf(v)))
		}
		result.Set("fields", eng.NewMap(fields))

	case eng.IsRecordShape(tv):
		result.Set("kind", eng.NewAtom("record"))
		m, _ := eng.AsMap(tv)
		fields := eng.NewOrderedMap()
		for _, k := range m.Keys() {
			v, _ := m.Get(k)
			fields.Set(k, eng.NewString(eng.TypeNameOf(v)))
		}
		result.Set("fields", eng.NewMap(fields))

	case eng.IsClassType(tv):
		result.Set("kind", eng.NewAtom("object"))
		oi, _ := eng.AsClassType(tv)
		if oi.Parent != nil {
			result.Set("parent", eng.NewString(oi.Parent.Name))
		}
		af := oi.AllFields()
		fields := eng.NewOrderedMap()
		for _, k := range af.Keys() {
			v, _ := af.Get(k)
			fields.Set(k, eng.NewString(eng.TypeNameOf(v)))
		}
		result.Set("fields", eng.NewMap(fields))

	case eng.IsTableType(tv):
		result.Set("kind", eng.NewAtom("table"))
		tt, _ := eng.AsTableType(tv)
		fields := eng.NewOrderedMap()
		for _, k := range tt.Record.Fields.Keys() {
			v, _ := tt.Record.Fields.Get(k)
			fields.Set(k, eng.NewString(eng.TypeNameOf(v)))
		}
		result.Set("fields", eng.NewMap(fields))

	case eng.IsDisjunct(tv):
		result.Set("kind", eng.NewAtom("disjunct"))
		di, _ := eng.AsDisjunct(tv)
		alts := make([]eng.Value, len(di.Alternatives))
		for i, alt := range di.Alternatives {
			alts[i] = eng.NewString(eng.TypePathOf(alt))
		}
		result.Set("alternatives", eng.NewList(alts))

	case eng.IsTypedList(tv):
		result.Set("kind", eng.NewAtom("typed_list"))
		ci, _ := eng.AsChildType(tv)
		result.Set("child", eng.NewString(eng.TypePathOf(ci.Child)))

	case eng.IsTypedMap(tv):
		result.Set("kind", eng.NewAtom("typed_map"))
		ci, _ := eng.AsChildType(tv)
		result.Set("child", eng.NewString(eng.TypePathOf(ci.Child)))

	case eng.ValueType(tv).Equal(eng.TFnUndef):
		result.Set("kind", eng.NewAtom("function_shape"))
		uInfo, _ := tv.Data.(eng.FnUndefInfo)
		sigs := make([]eng.Value, 0, len(uInfo.Sigs))
		for _, spec := range uInfo.Sigs {
			sig := eng.NewOrderedMap()
			params := make([]eng.Value, len(spec.Params))
			for i, p := range spec.Params {
				params[i] = eng.NewString(p.Type.Leaf())
			}
			sig.Set("params", eng.NewList(params))
			rets := make([]eng.Value, len(spec.Returns))
			for i, ret := range spec.Returns {
				rets[i] = eng.NewString(ret.Leaf())
			}
			sig.Set("returns", eng.NewList(rets))
			sigs = append(sigs, eng.NewMap(sig))
		}
		result.Set("signatures", eng.NewList(sigs))

	case tv.IsDepScalar():
		result.Set("kind", eng.NewAtom("dependent_scalar"))
		info, _ := tv.AsDepScalar()
		result.Set("leaf", eng.NewString(tv.Parent.Name()))
		if info.Lo != nil {
			lo := eng.NewOrderedMap()
			lo.Set("kind", eng.NewString(eng.BoundToKind(info.Lo, true).String()))
			lo.Set("value", info.Lo.Value)
			result.Set("lo", eng.NewMap(lo))
		}
		if info.Hi != nil {
			hi := eng.NewOrderedMap()
			hi.Set("kind", eng.NewString(eng.BoundToKind(info.Hi, false).String()))
			hi.Set("value", info.Hi.Value)
			result.Set("hi", eng.NewMap(hi))
		}

	default:
		result.Set("kind", eng.NewAtom("literal"))
	}

	return eng.NewValueRaw(eng.TInspect, eng.MapPayload{M: result})
}

// registerEngSpecFnSig installs `fnsig` as a spec-runner fixture so
// the eng/spec/types.tsv rows around FnSig type-shape matching can
// run against the kernel alone. The production fnsig registration
// lives in lang/go/engine/native_definition.go.
func registerEngSpecFnSig(r *eng.Registry) {
	r.RegisterNativeFunc(eng.NativeFunc{
		Name: "fnsig",

		Signatures: []eng.Signature{{
			Args:       []*eng.Type{eng.TList},
			NoEvalArgs: map[int]bool{0: true},
			Impl: eng.Go(func(args []eng.Value, _ map[string]eng.Value, _ []eng.Value, reg *eng.Registry) ([]eng.Value, error) {
				if !eng.IsConcrete(args[0]) {
					return nil, &eng.AqlError{
						Code:   "fnsig_invalid_spec",
						Detail: "fnsig: argument must be a concrete list",
					}
				}
				lst, _ := eng.AsList(args[0])
				spec := lst.Slice()
				if len(spec) == 0 || len(spec)%2 != 0 {
					return nil, &eng.AqlError{
						Code:   "fnsig_invalid_spec",
						Detail: "fnsig: list length must be a non-zero multiple of 2 (input output pairs); use `fn` for the with-body form",
					}
				}
				info, err := eng.ParseFnUndefSpec(reg, spec)
				if err != nil {
					return nil, err
				}
				return []eng.Value{eng.NewFnUndef(info)}, nil
			}),
			Returns: []*eng.Type{eng.TFnUndef}, BarrierPos: -1,
		}},
	})
}

// registerEngSpecBoolean installs not/and/or as spec-runner fixtures
// using eng.CoerceBoolean. The production words with the same names
// live in lang/go/engine/native_boolean.go.
func registerEngSpecBoolean(r *eng.Registry) {
	notH := func(args []eng.Value, _ map[string]eng.Value, _ []eng.Value, _ *eng.Registry) ([]eng.Value, error) {
		return []eng.Value{eng.NewBoolean(!eng.CoerceBoolean(args[0]))}, nil
	}
	andH := func(args []eng.Value, _ map[string]eng.Value, _ []eng.Value, _ *eng.Registry) ([]eng.Value, error) {
		if !eng.CoerceBoolean(args[1]) {
			return []eng.Value{args[1]}, nil
		}
		return []eng.Value{args[0]}, nil
	}
	orH := func(args []eng.Value, _ map[string]eng.Value, _ []eng.Value, _ *eng.Registry) ([]eng.Value, error) {
		if eng.CoerceBoolean(args[1]) {
			return []eng.Value{args[1]}, nil
		}
		return []eng.Value{args[0]}, nil
	}
	r.RegisterNativeFunc(eng.NativeFunc{
		Name: "not",

		Signatures: []eng.Signature{
			{Args: []*eng.Type{eng.TBoolean}, Impl: eng.Go(notH), Returns: []*eng.Type{eng.TBoolean}, BarrierPos: -1},
			{Args: []*eng.Type{eng.TAny}, Impl: eng.Go(notH), Returns: []*eng.Type{eng.TBoolean}, BarrierPos: -1},
		},
	})
	r.RegisterNativeFunc(eng.NativeFunc{
		Name: "and",

		Signatures: []eng.Signature{
			{Args: []*eng.Type{eng.TBoolean, eng.TBoolean}, Impl: eng.Go(andH), Returns: []*eng.Type{eng.TBoolean}, BarrierPos: -1},
			{Args: []*eng.Type{eng.TAny, eng.TAny}, Impl: eng.Go(andH), Returns: []*eng.Type{eng.TAny}, BarrierPos: -1},
		},
	})
	r.RegisterNativeFunc(eng.NativeFunc{
		Name: "or",

		Signatures: []eng.Signature{
			{Args: []*eng.Type{eng.TBoolean, eng.TBoolean}, BarrierPos: 1, Impl: eng.Go(orH), Returns: []*eng.Type{eng.TBoolean}},
			{Args: []*eng.Type{eng.TAny, eng.TAny}, BarrierPos: 1, Impl: eng.Go(orH), Returns: []*eng.Type{eng.TAny}},
		},
	})
}

// registerEngSpecDo installs the `do` word as a spec-runner fixture.
// The production registration lives in
// lang/go/engine/native_control.go; engspec ships a minimal version
// that runs a list body or evaluates embedded lists in a map literal
// against a sub-engine — enough surface for the eng/spec/do.tsv rows
// to exercise the kernel's sub-engine semantics.
func registerEngSpecDo(r *eng.Registry) {
	listH := func(args []eng.Value, _ map[string]eng.Value, _ []eng.Value, r *eng.Registry) ([]eng.Value, error) {
		if !eng.IsConcrete(args[0]) {
			return nil, &eng.AqlError{
				Code:   "type_error",
				Detail: "do: argument must be a concrete list, got type literal",
			}
		}
		lst, _ := eng.AsList(args[0])
		sub := eng.New(r)
		input := append([]eng.Value{}, lst.Slice()...)
		result, err := sub.Run(input)
		if err != nil {
			return []eng.Value{eng.NewError(err)}, nil
		}
		return result, nil
	}
	mapH := func(args []eng.Value, _ map[string]eng.Value, _ []eng.Value, r *eng.Registry) ([]eng.Value, error) {
		result, err := doEvalMapValue(r, args[0])
		if err != nil {
			return nil, err
		}
		return []eng.Value{result}, nil
	}
	r.RegisterNativeFunc(eng.NativeFunc{
		Name: "do",

		Signatures: []eng.Signature{
			{Args: []*eng.Type{eng.TList}, NoEvalArgs: map[int]bool{0: true}, Impl: eng.Go(listH), Returns: []*eng.Type{eng.TAny}, BarrierPos: -1},
			{Args: []*eng.Type{eng.TMap}, Impl: eng.Go(mapH), Returns: []*eng.Type{eng.TAny}, BarrierPos: -1},
		},
	})
}

// doEvalMapValue recursively evaluates list values within a map.
// Records, options, table types, typed lists / maps are left
// untouched — only plain concrete lists and maps are walked.
func doEvalMapValue(r *eng.Registry, v eng.Value) (eng.Value, error) {
	if v.Parent.Equal(eng.TList) && v.Data != nil && !eng.IsTypedList(v) && !eng.IsTableType(v) {
		lst, _ := eng.AsList(v)
		sub := eng.New(r)
		input := make([]eng.Value, lst.Len())
		copy(input, lst.Slice())
		results, err := sub.Run(input)
		if err != nil {
			return eng.Value{}, err
		}
		if len(results) == 1 {
			return results[0], nil
		}
		return eng.NewList(results), nil
	}
	if v.Parent.Equal(eng.TMap) && v.Data != nil && !eng.IsTypedMap(v) && !eng.IsRecordType(v) && !eng.IsOptionsType(v) {
		m, err := eng.AsMap(v)
		if err != nil || m == nil {
			return v, nil
		}
		out := eng.NewOrderedMap()
		for _, key := range m.Keys() {
			val, _ := m.Get(key)
			evaluated, err := doEvalMapValue(r, val)
			if err != nil {
				return eng.Value{}, err
			}
			out.Set(key, evaluated)
		}
		return eng.NewMap(out), nil
	}
	return v, nil
}

// registerEngSpecTypeOps installs tor/tand as spec-runner fixtures
// using the eng-exported algorithm handlers. Production registrations
// live in lang/go/engine/native_type.go.
func registerEngSpecTypeOps(r *eng.Registry) {
	r.RegisterNativeFunc(eng.NativeFunc{
		Name: "tor",

		Signatures: []eng.Signature{{
			Args:       []*eng.Type{eng.TAny, eng.TAny},
			BarrierPos: 1,
			Impl:       eng.Go(eng.TorHandler),
			ReturnsFn:  eng.TorReturnsFn,
		}},
	})
	r.RegisterNativeFunc(eng.NativeFunc{
		Name: "tand",

		Signatures: []eng.Signature{{
			Args:       []*eng.Type{eng.TAny, eng.TAny},
			BarrierPos: 1,
			Impl:       eng.Go(eng.TandHandler),
			Returns:    []*eng.Type{eng.TAny},
		}},
	})
}

// TestSpec runs aql/eng/spec/*.tsv against the engine kernel — a fresh
// eng.Registry populated by registerSpecWords, which installs only
// the spec-runner fixtures eng/spec needs (q-suffixed probes plus
// minimal in-test copies of the dispatching words). The shared TSV
// scaffolding (file walk, row parsing, ERROR handling, value
// rendering) lives in test/go/specrunner.
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
