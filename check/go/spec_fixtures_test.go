package check_test

// The check corpus's fixture vocabulary.
//
// A bare core.NewRegistry() carries NO words at all — the base language
// vocabulary lives in basic/go, and the corpus-wide fixture set lives in
// test/specfix, neither of which check may import (basic is a sibling that
// would add a production edge; specfix REQUIRES check, so importing it
// closes a cycle). So the corpus declares its own.
//
// This set is deliberately minimal. It is not a copy of specfix's
// vocabulary: it carries only the shapes check's own analysis needs to be
// driven through — a monomorphic word, an overloaded word (dispatch
// selection), a word over a type union (carrier joins), and a predicate
// (guard narrowing). Words are named with a `q` suffix, the repo's
// convention marking a spec fixture rather than a production word.

import (
	core "github.com/boru-lang/boru/core/go"
)

// goImpl adapts a plain function to the native-func signature.
func goImpl(f func(args []core.Value) []core.Value) *core.GoImpl {
	return core.Go(func(args []core.Value, _ map[string]core.Value, _ []core.Value, _ *core.Registry) ([]core.Value, error) {
		return f(args), nil
	})
}

// registerCheckFixtures installs the corpus vocabulary on r.
func registerCheckFixtures(r *core.Registry) {
	numberPair := []*core.Type{core.TNumber, core.TNumber}

	// addq — monomorphic over Number: the simplest dispatch shape, and the
	// baseline every row builds on.
	r.RegisterNativeFunc(core.NativeFunc{
		Name: "addq",
		Signatures: []core.Signature{{
			Args:       numberPair,
			Returns:    []*core.Type{core.TNumber},
			BarrierPos: -1,
			Impl: goImpl(func(args []core.Value) []core.Value {
				a, _ := core.AsInteger(args[0])
				b, _ := core.AsInteger(args[1])
				return []core.Value{core.NewInteger(a + b)}
			}),
		}},
	})

	// twoq — two overloads on distinct argument types. This is the shape
	// that drives overload selection, and with a non-concrete operand the
	// dynamic-reachable-overload counting in carrier.go.
	r.RegisterNativeFunc(core.NativeFunc{
		Name: "twoq",
		Signatures: []core.Signature{
			{
				Args: []*core.Type{core.TInteger}, Returns: []*core.Type{core.TInteger},
				BarrierPos: 1,
				Impl:       goImpl(func(args []core.Value) []core.Value { return []core.Value{args[0]} }),
			},
			{
				Args: []*core.Type{core.TString}, Returns: []*core.Type{core.TString},
				BarrierPos: 1,
				Impl:       goImpl(func(args []core.Value) []core.Value { return []core.Value{args[0]} }),
			},
		},
	})

	// strq — monomorphic over String, for rows that need a second concrete
	// leaf type to join against.
	r.RegisterNativeFunc(core.NativeFunc{
		Name: "strq",
		Signatures: []core.Signature{{
			Args: []*core.Type{core.TString}, Returns: []*core.Type{core.TString},
			BarrierPos: 1,
			Impl:       goImpl(func(args []core.Value) []core.Value { return []core.Value{args[0]} }),
		}},
	})

	// anyq — Integer -> Any. The point of this word is its RETURN: Any is
	// not a concrete leaf, so its result is a non-concrete operand. Feeding
	// that into an overloaded word is what drives the dynamic-reachable
	// overload counting and the "is any operand non-concrete" family in
	// carrier.go, none of which a fully concrete row can reach.
	r.RegisterNativeFunc(core.NativeFunc{
		Name: "anyq",
		Signatures: []core.Signature{{
			Args: []*core.Type{core.TInteger}, Returns: []*core.Type{core.TAny},
			BarrierPos: 1,
			Impl:       goImpl(func(args []core.Value) []core.Value { return []core.Value{args[0]} }),
		}},
	})

	// defq — `defq name [triples]` installs a user fn: the list is walked
	// by core.ParseFnDef in [input-sig, output-sig, body] triples and
	// installed with core.InstallFnDef, mirroring the shape of specfix's
	// `def name fn [...]` fixture (test/specfix/words.go) minus the keyword
	// slot. This is the word that opens the fn-body analysis surface:
	// installing a boru-bodied fn is what drives check_fnbody.go's
	// construction pass and the AnalyseFnBody family in carrier.go, none of
	// which a native-only vocabulary can reach. RunInCheck makes the
	// definition fire during the check pass itself.
	r.RegisterNativeFunc(core.NativeFunc{
		Name: "defq",
		Signatures: []core.Signature{{
			Args:       []*core.Type{core.TAtom, core.TList},
			QuoteArgs:  map[int]bool{0: true},
			NoEvalArgs: map[int]bool{1: true},
			Returns:    []*core.Type{}, BarrierPos: -1,
			Impl: core.Go(func(args []core.Value, _ map[string]core.Value, _ []core.Value, reg *core.Registry) ([]core.Value, error) {
				name, _ := args[0].AsConcreteAtom()
				if err := core.ValidateWordName(name); err != nil {
					return nil, err
				}
				if !core.IsConcrete(args[1]) {
					return nil, &core.BoruError{Code: "type_error", Detail: "defq: body must be a concrete list"}
				}
				lst, _ := core.AsList(args[1])
				elems := lst.Slice()
				if len(elems) == 0 || len(elems)%3 != 0 {
					return nil, &core.BoruError{Code: "fn_invalid_spec", Detail: "defq: list length must be a non-zero multiple of 3"}
				}
				fnDef, err := core.ParseFnDef(reg, elems)
				if err != nil {
					return nil, err
				}
				reg.Check.RecordDef(name, core.SrcPos{})
				core.InstallFnDef(reg, name, fnDef)
				return nil, nil
			}, core.RunInCheck()),
		}},
	})

	// predq — Integer -> Boolean, the predicate shape guard narrowing and
	// the fn-predicate overload hazard analysis look for.
	r.RegisterNativeFunc(core.NativeFunc{
		Name: "predq",
		Signatures: []core.Signature{{
			Args: []*core.Type{core.TInteger}, Returns: []*core.Type{core.TBoolean},
			BarrierPos: 1,
			Impl: goImpl(func(args []core.Value) []core.Value {
				n, _ := core.AsInteger(args[0])
				return []core.Value{core.NewBoolean(n > 0)}
			}),
		}},
	})
}
