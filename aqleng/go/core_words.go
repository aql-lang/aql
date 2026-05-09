package aqleng

// RegisterCoreWords installs the language-fundamental words into the
// registry. These are part of the aqleng language proper — the
// minimal native subset needed to express the AQL language design's
// core constructs:
//
//   Binding / quotation:
//     def    — name binding (simple values, list bodies, fn definitions)
//     fn     — function literal builder (typed params, return types, body)
//     quote  — explicit data capture (suppresses word evaluation)
//     args   — per-fn-call positional argument frame
//
//   Boolean / logical connectives:
//     not    — boolean negation                  ( a — !a )
//     and    — short-circuit conjunction         ( a b — a∧b ); returns the first falsy or last truthy operand
//     or     — short-circuit disjunction         ( a b — a∨b ); returns the first truthy or last falsy operand
//
//   Type-level connectives (run-time disjunct/intersect builders):
//     tor    — type-level union (disjunct)       ( T U — T|U )
//     tand   — type-level intersection           ( T U — T∩U )
//
//   Stack manipulation (Forth-style; all stack-only):
//     dup        — duplicate top                  ( a — a a )
//     swap       — exchange top two               ( a b — b a )
//     drop       — remove top                     ( a — )
//     over       — copy second-from-top to top    ( a b — a b a )
//     rot        — rotate top three               ( a b c — b c a )
//     nip        — drop second-from-top           ( a b — b )
//     tuck       — copy top under second          ( a b — b a b )
//     dup2       — duplicate top pair             ( a b — a b a b )
//     swap2      — swap top two pairs             ( a b c d — c d a b )
//     drop2      — remove top two                 ( a b — )
//     over2      — copy second pair to top        ( a b c d — a b c d a b )
//
// `end` is NOT registered here — it's a structural keyword handled
// directly by the engine's stepEnd path in engine.go.
//
// `if`, `for`, `type*`, `do`, `each`, `fold`, the higher-arity stack
// ops (`depth`, `pick`, `roll` which need FullStack), and the rest
// of the production word set are reserved for future addition.
//
// These implementations are deliberately minimal: they cover the
// dispatch / value / type-lattice core that every consumer of
// aqleng (including the production aql package) builds on. The
// production aql engine in aql/internal/engine layers richer
// behaviour (paren markers, /q forward capture, ObjectType bindings,
// etc.) on top of these primitives — but the primitives themselves
// live here so any aqleng test suite can exercise them directly.
func RegisterCoreWords(r *Registry) {
	registerCoreDef(r)
	registerCoreFn(r)
	registerCoreQuote(r)
	registerCoreArgs(r)
	registerCoreStack(r)
	registerCoreBoolean(r)
	registerCoreTypeOps(r)
}

// RegisterCoreBoolean installs the boolean / logical-connective core
// words: not, and, or. They are exported as a separate entry point
// so consumers (e.g. the production aql package) can install just
// these without taking the rest of RegisterCoreWords.
//
// The handlers route through CoerceBoolean (in core_helpers.go) for
// non-boolean inputs; `and`/`or` short-circuit and return the
// operand that decided the result rather than a pure boolean. So
// `0 or 5` returns `5`, and `1 and 2` returns `2` — matching Lisp /
// Python truthy-coalescing semantics.
func RegisterCoreBoolean(r *Registry) {
	registerCoreBoolean(r)
}

// RegisterCoreTypeOps installs the type-level connective core words:
// tor (disjunct union) and tand (intersection). Exported as a
// separate entry point so consumers can install just these.
func RegisterCoreTypeOps(r *Registry) {
	registerCoreTypeOps(r)
}

// registerCoreDef installs `def NAME body`. NAME must arrive as a
// Word (the tokenizer's `/q` machinery in the production parser does
// this for `def`'s first slot — the bare aqleng tokenizer just leaves
// the name as a Word, which is what the [TWord] sig matches).
//
// If body is a single-sig FnDefInfo (i.e. produced by `fn […] […] […]`),
// def installs a synthesised native that binds the params and runs
// the body. Otherwise body is pushed as a simple value onto the def
// stack under NAME.
func registerCoreDef(r *Registry) {
	r.RegisterNativeFunc(NativeFunc{
		Name:              "def",
		ForwardPrecedence: true,
		Signatures: []NativeSig{{
			Args:       []Type{TWord, TAny},
			NoEvalArgs: map[int]bool{1: true},
			Handler: func(args []Value, _ map[string]Value, _ []Value, reg *Registry) ([]Value, error) {
				w, _ := args[0].AsWord()
				if err := ValidateWordName(w.Name); err != nil {
					return nil, err
				}
				if info, ok := args[1].Data.(FnDefInfo); ok {
					// Expand optional-param combinations into a flat
					// list of sigs. Each reduced sig's body calls back
					// into <name> with omitted params filled by their
					// type's BaseValue, so the full sig handles the
					// real work after defaults are plugged in.
					sigs := ExpandOptionalSigs(w.Name, info.Sigs)
					installCoreFnDef(reg, w.Name, sigs...)
					return []Value{}, nil
				}
				reg.PushDef(w.Name, args[1])
				return []Value{}, nil
			},
			Returns: []Type{},
		}},
	})
}

// registerCoreFn installs `fn [triples-list]`. The single list arg
// contains `[input-sig, output-sig, body]` repeated as many times as
// there are overloads — same shape the production aql `fn` accepts.
// A single-sig def therefore wraps the three lists in an outer list:
//
//   def double fn [ [a:Integer] [Integer] [a a addq] ]
//
// Multi-sig is the same shape with N triples flat in the outer list:
//
//   def f fn [
//     [n:Integer]            [Integer] [n addq n]
//     [n:Integer m:Integer]  [Integer] [n m mulq]
//   ]
//
// Routes through ParseFnDef in fn_def.go — same code path the
// production aql `def`/`fn` use. The shared parser handles:
//
//   - signature triples [input-sig output-sig body], repeated
//   - `?` marker for optional params, `|` for BarrierPos
//   - `{name:Type}` implicit-pair maps (with optional `?` suffix)
//   - bare type-name Words and type literals
//   - paren-expr type slots and disjuncts containing None
//   - concrete-value patterns (Integer/Boolean/String literals)
//   - return-by-value output sigs (`[42 "ok"]`) appended to the body
//
// At def-time each FnSig is fed through ExpandOptionalSigs so every
// present/omitted combination of optional params becomes its own
// callable overload, with omitted params filled by their type's
// BaseValue.
//
// Multi-sig dispatch then happens at call time via matchSignature,
// which picks the best (highest-arity, most-specific) match per call
// site.
func registerCoreFn(r *Registry) {
	r.RegisterNativeFunc(NativeFunc{
		Name:              "fn",
		ForwardPrecedence: true,
		Signatures: []NativeSig{{
			Args:       []Type{TList},
			NoEvalArgs: map[int]bool{0: true},
			Handler: func(args []Value, _ map[string]Value, _ []Value, reg *Registry) ([]Value, error) {
				if args[0].Data == nil {
					return nil, &AqlError{
						Code:   "fn_invalid_spec",
						Detail: "fn: argument must be a concrete list of triples",
					}
				}
				spec := args[0].AsList().Slice()
				info, err := ParseFnDef(reg, spec)
				if err != nil {
					return nil, err
				}
				return []Value{NewFnDef(info)}, nil
			},
			Returns: []Type{TFunction},
		}},
	})
}

// registerCoreQuote installs `quote VALUE`. Two overloads:
//   - [TWord]: convert a Word → Atom of the same name. This makes
//     `quote dup` produce atom(dup) even when dup is registered.
//   - [TAny]: catch-all passthrough. Lists arrive raw (NoEvalArgs)
//     and are flagged Quoted=true so downstream consumers (def,
//     auto-eval, etc.) treat them as data instead of code.
func registerCoreQuote(r *Registry) {
	r.RegisterNativeFunc(NativeFunc{
		Name:              "quote",
		ForwardPrecedence: true,
		Signatures: []NativeSig{
			{
				Args: []Type{TWord},
				Handler: func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
					w, _ := args[0].AsWord()
					return []Value{NewAtom(w.Name)}, nil
				},
				Returns: []Type{TAtom},
			},
			{
				Args:       []Type{TAny},
				NoEvalArgs: map[int]bool{0: true},
				Handler: func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
					v := args[0]
					if v.VType.Equal(TList) && v.Data != nil {
						v.Quoted = true
					}
					return []Value{v}, nil
				},
				Returns: []Type{TAny},
			},
		},
	})
}

// registerCoreArgs installs `args` (zero-arg). Returns the current
// fn-call's argument frame as a List. Outside any fn call, returns
// the empty list — consistent with "no args available" rather than
// raising.
//
// Also installs the engine-internal marker `__pa` (pop args). The
// engine emits `__pa` at the tail of fn-body expansions in
// execFnDefSig (engine.go) and InstallFnDef (core_helpers.go) — its
// only job is to pop the args frame that the call-site pushed via
// PushArgs. Without it, an FnDef literal that lands at a non-forward
// position (e.g. paren-wrapped, then auto-called) reaches stepWord
// with an unregistered word and fails with `undefined_word`.
func registerCoreArgs(r *Registry) {
	r.RegisterNativeFunc(NativeFunc{
		Name: "args",
		Signatures: []NativeSig{{
			Args: []Type{},
			Handler: func(_ []Value, _ map[string]Value, _ []Value, reg *Registry) ([]Value, error) {
				if top, ok := reg.TopArgs(); ok {
					return []Value{top}, nil
				}
				return []Value{NewList(nil)}, nil
			},
			Returns: []Type{TList},
		}},
	})
	r.RegisterNativeFunc(NativeFunc{
		Name:              "__pa",
		ForwardPrecedence: true,
		Signatures: []NativeSig{{
			Args: []Type{},
			Handler: func(_ []Value, _ map[string]Value, _ []Value, reg *Registry) ([]Value, error) {
				reg.PopArgs()
				return nil, nil
			},
			Returns: []Type{},
		}},
	})
}

// registerCoreStack installs the Forth-style stack manipulation
// primitives. All are stack-only (BarrierPos = 0 by default; no
// ForwardPrecedence flag). Argument convention is the unified §1.4
// rule: args[0] is the top of stack, args[1] is the next-deeper,
// etc. Splice ordering: the returned []Value is laid back onto the
// stack in source order, so an N-arg word that returns the same N
// values produces the inputs unchanged (see swap for the worked
// example).
//
// Mirrors the canonical-Forth subset of the production engine's
// aql/internal/engine/native_stack.go — handlers are byte-identical.
// The full-stack-aware ops (depth, pick, roll) are deliberately
// omitted: they need FullStack signatures and the spec runner's
// dispatch path doesn't yet wire that up. They can be added later
// without breaking this surface.
//
// Each op has its own per-name installer (registerCoreDup, etc.) so
// engine_options.go's Words whitelist can pick out individual stack
// ops. registerCoreStack just chains them.
func registerCoreStack(r *Registry) {
	registerCoreDup(r)
	registerCoreSwap(r)
	registerCoreDrop(r)
	registerCoreOver(r)
	registerCoreRot(r)
	registerCoreNip(r)
	registerCoreTuck(r)
	registerCoreDup2(r)
	registerCoreSwap2(r)
	registerCoreDrop2(r)
	registerCoreOver2(r)
}

// registerCoreDup — `dup ( a — a a )`.
func registerCoreDup(r *Registry) {
	r.RegisterNativeFunc(NativeFunc{
		Name: "dup",
		Signatures: []NativeSig{{
			Args: []Type{TAny},
			Handler: func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
				return []Value{args[0], args[0]}, nil
			},
		}},
	})
}

// registerCoreSwap — `swap ( a b — b a )`.
// Under the unified §1.4 rule args[0] is the top and args[1] is
// next-deeper. Returning [args[0], args[1]] in source order puts
// the old top at the deeper position and the old second-from-top
// at the top — i.e. they swap.
func registerCoreSwap(r *Registry) {
	r.RegisterNativeFunc(NativeFunc{
		Name: "swap",
		Signatures: []NativeSig{{
			Args: []Type{TAny, TAny},
			Handler: func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
				return []Value{args[0], args[1]}, nil
			},
		}},
	})
}

// registerCoreDrop — `drop ( a — )`.
func registerCoreDrop(r *Registry) {
	r.RegisterNativeFunc(NativeFunc{
		Name: "drop",
		Signatures: []NativeSig{{
			Args: []Type{TAny},
			Handler: func(_ []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
				return nil, nil
			},
		}},
	})
}

// registerCoreOver — `over ( a b — a b a )`.
// args[0]=top=b, args[1]=deeper=a. Output sequence puts a at the
// deepest restore position, then b, then a again on top.
func registerCoreOver(r *Registry) {
	r.RegisterNativeFunc(NativeFunc{
		Name: "over",
		Signatures: []NativeSig{{
			Args: []Type{TAny, TAny},
			Handler: func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
				return []Value{args[1], args[0], args[1]}, nil
			},
		}},
	})
}

// registerCoreRot — `rot ( a b c — b c a )`.
// args[0]=top=c, args[1]=b, args[2]=deepest=a.
func registerCoreRot(r *Registry) {
	r.RegisterNativeFunc(NativeFunc{
		Name: "rot",
		Signatures: []NativeSig{{
			Args: []Type{TAny, TAny, TAny},
			Handler: func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
				return []Value{args[1], args[0], args[2]}, nil
			},
		}},
	})
}

// registerCoreNip — `nip ( a b — b )`.
// args[0]=top=b is kept; args[1]=a is discarded.
func registerCoreNip(r *Registry) {
	r.RegisterNativeFunc(NativeFunc{
		Name: "nip",
		Signatures: []NativeSig{{
			Args: []Type{TAny, TAny},
			Handler: func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
				return []Value{args[0]}, nil
			},
		}},
	})
}

// registerCoreTuck — `tuck ( a b — b a b )`.
// args[0]=top=b, args[1]=a. Output: b at deepest, a in middle,
// b at top — copies the top under the second.
func registerCoreTuck(r *Registry) {
	r.RegisterNativeFunc(NativeFunc{
		Name: "tuck",
		Signatures: []NativeSig{{
			Args: []Type{TAny, TAny},
			Handler: func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
				return []Value{args[0], args[1], args[0]}, nil
			},
		}},
	})
}

// registerCoreDup2 — `dup2 ( a b — a b a b )`.
// args[0]=top=b, args[1]=a.
func registerCoreDup2(r *Registry) {
	r.RegisterNativeFunc(NativeFunc{
		Name: "dup2",
		Signatures: []NativeSig{{
			Args: []Type{TAny, TAny},
			Handler: func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
				return []Value{args[1], args[0], args[1], args[0]}, nil
			},
		}},
	})
}

// registerCoreSwap2 — `swap2 ( a b c d — c d a b )`.
// args[0]=top=d, args[1]=c, args[2]=b, args[3]=deepest=a.
func registerCoreSwap2(r *Registry) {
	r.RegisterNativeFunc(NativeFunc{
		Name: "swap2",
		Signatures: []NativeSig{{
			Args: []Type{TAny, TAny, TAny, TAny},
			Handler: func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
				return []Value{args[1], args[0], args[3], args[2]}, nil
			},
		}},
	})
}

// registerCoreDrop2 — `drop2 ( a b — )`.
func registerCoreDrop2(r *Registry) {
	r.RegisterNativeFunc(NativeFunc{
		Name: "drop2",
		Signatures: []NativeSig{{
			Args: []Type{TAny, TAny},
			Handler: func(_ []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
				return nil, nil
			},
		}},
	})
}

// registerCoreOver2 — `over2 ( a b c d — a b c d a b )`.
// Copies the second pair (a b) on top of the first pair (c d).
func registerCoreOver2(r *Registry) {
	r.RegisterNativeFunc(NativeFunc{
		Name: "over2",
		Signatures: []NativeSig{{
			Args: []Type{TAny, TAny, TAny, TAny},
			Handler: func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
				return []Value{args[3], args[2], args[1], args[0], args[3], args[2]}, nil
			},
		}},
	})
}

// installCoreFnDef wires one or more FnSigs into the registry as a
// single native with multiple overloads. Each sig's handler binds
// the named params onto the def stack, pushes the matched args onto
// the args stack so the body can read them via the `args` word, runs
// the body in a fresh sub-engine, and pops both stacks on return.
//
// When called with the post-ExpandOptionalSigs result, the full sig
// + every reduced sig (one per non-empty subset of optional params)
// land here as separate overloads. The matcher then dispatches each
// call site to the sig matching its arity; reduced-sig bodies call
// back into `name` with defaults plugged in, which re-dispatches to
// the full sig.
//
// Mirrors the param-binding portion of the production engine's
// InstallFnDef in aql/internal/engine/core_helpers.go without pulling
// in the production engine's `__pa` / paren-marker machinery (which
// belongs to the full parser, not the bare aqleng core).
func installCoreFnDef(r *Registry, name string, sigs ...FnSig) {
	nativeSigs := make([]NativeSig, 0, len(sigs))
	for _, sigOrig := range sigs {
		sig := sigOrig // capture per-iteration
		argTypes := make([]Type, len(sig.Params))
		for i, p := range sig.Params {
			argTypes[i] = p.Type
		}
		bodyCopy := append([]Value{}, sig.Body...)
		nativeSigs = append(nativeSigs, NativeSig{
			Args: argTypes,
			Handler: func(args []Value, _ map[string]Value, _ []Value, reg *Registry) ([]Value, error) {
				for i, p := range sig.Params {
					reg.PushDef(p.Name, args[i])
				}
				argsCopy := append([]Value{}, args...)
				reg.PushArgs(NewList(argsCopy))
				defer func() {
					reg.PopArgs()
					for i := len(sig.Params) - 1; i >= 0; i-- {
						reg.PopDef(sig.Params[i].Name)
					}
				}()
				sub := New(reg)
				input := append([]Value{}, bodyCopy...)
				return sub.Run(input)
			},
		})
	}
	r.RegisterNativeFunc(NativeFunc{
		Name:              name,
		ForwardPrecedence: true,
		Signatures:        nativeSigs,
	})
}

// (parseCoreFnParam, parseCoreFnReturn, parseCoreTypeName were
// retired when the canonical fn-signature parser landed in
// fn_params.go. Both core_words.go and aql/internal/engine now call
// ParseFnParams / ParseFnReturns / ResolveTypeName directly.)
