package eng

// RegisterCoreWords installs the language-fundamental words into the
// registry. These are part of the aqleng language proper — the
// minimal native subset needed to express the AQL language design's
// core constructs:
//
//	Binding / quotation:
//	  def    — name binding (simple values, list bodies, fn definitions)
//	  fn     — function literal builder (typed params, return types, body)
//	  quote  — explicit data capture (suppresses word evaluation)
//	  args   — per-fn-call positional argument frame
//
//	Boolean / logical connectives:
//	  not    — boolean negation                  ( a — !a )
//	  and    — short-circuit conjunction         ( a b — a∧b ); returns the first falsy or last truthy operand
//	  or     — short-circuit disjunction         ( a b — a∨b ); returns the first truthy or last falsy operand
//
//	Type-level connectives (run-time disjunct/intersect builders):
//	  tor    — type-level union (disjunct)       ( T U — T|U )
//	  tand   — type-level intersection           ( T U — T∩U )
//
//	Type system:
//	  type / untype  — push / pop a named type binding
//	  typeof         — the type of a value, as a Type literal
//	  pathof         — a type's ancestry path (a List of Type)
//	  is             — type-test predicate (`v is T`)
//	  enum           — fixed-enumeration (Enum) type builder
//	  record         — RecordType from a list of single-pair maps
//	  object         — ObjectType (nominal, inheritance-aware)
//	  make           — universal typed-value constructor
//	  inspect        — machine-readable introspection (word / type → Map)
//
//	State / control:
//	  do             — run a list body as a sub-program
//	  get / set      — read / write a field on a Map / List / Record / Object
//
//	Stack manipulation (Forth-style; all stack-only):
//	  dup        — duplicate top                  ( a — a a )
//	  swap       — exchange top two               ( a b — b a )
//	  drop       — remove top                     ( a — )
//	  over       — copy second-from-top to top    ( a b — a b a )
//	  rot        — rotate top three               ( a b c — b c a )
//	  nip        — drop second-from-top           ( a b — b )
//	  tuck       — copy top under second          ( a b — b a b )
//	  dup2       — duplicate top pair             ( a b — a b a b )
//	  swap2      — swap top two pairs             ( a b c d — c d a b )
//	  drop2      — remove top two                 ( a b — )
//	  over2      — copy second pair to top        ( a b c d — a b c d a b )
//
// `end` is NOT registered here — it's a structural keyword handled
// directly by the engine's stepEnd path in engine.go.
//
// `if`, `for`, `each`, `fold`, the higher-arity stack ops (`depth`,
// `pick`, `roll` which need FullStack), and the rest of the production
// word set are reserved for future addition.
//
// These implementations are deliberately minimal: they cover the
// dispatch / value / type-lattice core that every consumer of
// aqleng (including the production aql package) builds on. The
// production aql engine in lang/internal/engine layers richer
// behaviour (paren markers, /q forward capture, ObjectType bindings,
// etc.) on top of these primitives — but the primitives themselves
// live here so any aqleng test suite can exercise them directly.
func RegisterCoreWords(r *Registry) {
	registerCoreDef(r)
	registerCoreFn(r)
	registerCoreFnSig(r)
	registerCoreQuote(r)
	registerCoreArgs(r)
	registerCoreStack(r)
	registerCoreBoolean(r)
	registerCoreTypeOps(r)
	registerCoreType(r)
	registerCoreObjectRecord(r)
	registerCoreInspect(r)
	registerCoreDo(r)
	registerCoreMake(r)
	registerCoreStorage(r)
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

// RegisterCoreFnSig installs the `fnsig` core word — the type-only
// counterpart to `fn`. Exported as a separate entry point so the
// production aql package (which has its own `fn` registration) can
// install just this addition.
func RegisterCoreFnSig(r *Registry) {
	registerCoreFnSig(r)
}

// RegisterCoreMake installs the `make` core word — the universal
// constructor for typed values (scalars, objects, records, paths,
// arrays). Exported as a separate entry point so the production lang
// package can install just this without taking the rest of
// RegisterCoreWords.
func RegisterCoreMake(r *Registry) {
	registerCoreMake(r)
}

// RegisterCoreStorage installs the `get` and `set` core words — the
// universal container-access pair. Exported as a separate entry
// point so the production lang package can install just these
// without taking the rest of RegisterCoreWords.
func RegisterCoreStorage(r *Registry) {
	registerCoreStorage(r)
}

// registerCoreDef installs `def NAME body`. NAME may arrive as either
// a Word (`def x 1`) or as a typed-name implicit map (`def x:Integer
// 1`, which the parser builds as `{x:Integer}`). The typed form
// validates the body against the declared type before installing.
//
// If body is a single-sig FnDefInfo (i.e. produced by `fn […]`), def
// installs a synthesised native that binds the params and runs the
// body. Otherwise body is pushed as a simple value onto the def
// stack under NAME.
func registerCoreDef(r *Registry) {
	plainHandler := func(args []Value, _ map[string]Value, _ []Value, reg *Registry) ([]Value, error) {
		w, _ := args[0].AsWord()
		if err := ValidateWordName(w.Name); err != nil {
			return nil, err
		}
		if info, ok := args[1].Data.(FnDefInfo); ok {
			sigs := ExpandOptionalSigs(w.Name, info.Sigs)
			installCoreFnDef(reg, w.Name, sigs...)
			return []Value{}, nil
		}
		reg.PushDef(w.Name, args[1])
		return []Value{}, nil
	}
	typedHandler := func(args []Value, _ map[string]Value, _ []Value, reg *Registry) ([]Value, error) {
		nameMap := args[0].AsMap()
		if nameMap == nil || nameMap.Len() == 0 {
			return nil, &AqlError{Code: "type_error", Detail: "def: typed-name map must have exactly one key"}
		}
		if nameMap.Len() != 1 {
			return nil, &AqlError{Code: "type_error", Detail: "def: typed-name map must have exactly one key"}
		}
		name := nameMap.Keys()[0]
		if err := ValidateWordName(name); err != nil {
			return nil, err
		}
		if reg.HasType(name) {
			return nil, &AqlError{Code: "type_error", Detail: "def " + name + ": name clash — already a type"}
		}
		constraint, _ := nameMap.Get(name)
		// A bare-Word constraint (e.g. `def x:Color …` where Color was
		// installed by `type Color enum […]`) must be resolved through
		// the type stack before validation. Mirrors the production
		// aql defTypedHandler in
		// lang/internal/engine/native_definition.go.
		if resolved, _, _ := reg.ResolveTypedNameValue(constraint); resolved.Data != nil || resolved.VType.ID != "" {
			constraint = resolved
		}
		body := args[1]
		if !IsValueOfType(body, constraint) {
			return nil, &AqlError{
				Code:   "type_error",
				Detail: "def " + name + ": value " + body.String() + " does not satisfy declared type " + constraint.String(),
			}
		}
		// Installation path mirrors the plain def: FnDefInfo bodies
		// register typed signatures, everything else pushes onto the
		// def stack as a simple value.
		if info, ok := body.Data.(FnDefInfo); ok {
			sigs := ExpandOptionalSigs(name, info.Sigs)
			installCoreFnDef(reg, name, sigs...)
			return []Value{}, nil
		}
		reg.PushDef(name, body)
		return []Value{}, nil
	}
	r.RegisterNativeFunc(NativeFunc{
		Name:              "def",
		ForwardPrecedence: true,
		Signatures: []NativeSig{
			{
				// Typed-name binding: def name:Type body. Sorts first
				// because TMap is more specific than TWord at the same
				// depth (higher inherent score).
				Args:          []Type{TMap, TAny},
				NoEvalArgs:    map[int]bool{1: true},
				NoEvalMapArgs: map[int]bool{0: true},
				Handler:       typedHandler,
				Returns:       []Type{},
			},
			{
				Args:       []Type{TWord, TAny},
				NoEvalArgs: map[int]bool{1: true},
				Handler:    plainHandler,
				Returns:    []Type{},
			},
		},
	})
}

// registerCoreFn installs `fn [triples-list]`. The single list arg
// contains `[input-sig, output-sig, body]` repeated as many times as
// there are overloads — same shape the production aql `fn` accepts.
// A single-sig def therefore wraps the three lists in an outer list:
//
//	def double fn [ [a:Integer] [Integer] [a a addq] ]
//
// Multi-sig is the same shape with N triples flat in the outer list:
//
//	def f fn [
//	  [n:Integer]            [Integer] [n addq n]
//	  [n:Integer m:Integer]  [Integer] [n m mulq]
//	]
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
//
// `fn` ALWAYS produces a Function value: the list is a series of
// [input, output, body] triples (length must be a multiple of 3).
// For the shape-only "function-type" form (input/output pairs, no
// body), use the separate `fnsig` word — it produces an FnSig
// value usable as a type constraint.
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
						Detail: "fn: argument must be a concrete list",
					}
				}
				spec := args[0].AsList().Slice()
				if len(spec) == 0 || len(spec)%3 != 0 {
					return nil, &AqlError{
						Code:   "fn_invalid_spec",
						Detail: "fn: list length must be a non-zero multiple of 3 (input output body triples); use `fnsig` for the type-only form",
					}
				}
				info, err := ParseFnDef(reg, spec)
				if err != nil {
					return nil, err
				}
				return []Value{NewFunction(info)}, nil
			},
			Returns: []Type{TFunction},
		}},
	})
}

// registerCoreFnSig installs `fnsig [input output …]` — produces a
// function-SHAPE type literal (FnSig) from input/output sig pairs.
//
// `fnsig` is the type-only counterpart to `fn`: same shape grammar,
// no body. The list length must be a non-zero multiple of 2 (each
// pair is one signature). The result is an FnSig value usable as a
// type constraint, e.g. `def f:fnsig [[Integer] [String]] impl`
// asserts that `impl` is a function whose signatures cover the
// shape `Integer → String`.
//
// FnSig is structural: any function value whose registered
// signatures satisfy every pair in the FnSig matches. See
// eng/go/fnsig.go::FnUndefMatchesFnDef.
func registerCoreFnSig(r *Registry) {
	r.RegisterNativeFunc(NativeFunc{
		Name:              "fnsig",
		ForwardPrecedence: true,
		Signatures: []NativeSig{{
			Args:       []Type{TList},
			NoEvalArgs: map[int]bool{0: true},
			Handler: func(args []Value, _ map[string]Value, _ []Value, reg *Registry) ([]Value, error) {
				if args[0].Data == nil {
					return nil, &AqlError{
						Code:   "fnsig_invalid_spec",
						Detail: "fnsig: argument must be a concrete list",
					}
				}
				spec := args[0].AsList().Slice()
				if len(spec) == 0 || len(spec)%2 != 0 {
					return nil, &AqlError{
						Code:   "fnsig_invalid_spec",
						Detail: "fnsig: list length must be a non-zero multiple of 2 (input output pairs); use `fn` for the with-body form",
					}
				}
				info, err := ParseFnUndefSpec(reg, spec)
				if err != nil {
					return nil, err
				}
				return []Value{NewFnUndef(info)}, nil
			},
			Returns: []Type{TFnUndef},
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
// lang/internal/engine/native_stack.go — handlers are byte-identical.
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
// InstallFnDef in lang/internal/engine/core_helpers.go without pulling
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
// fn_params.go. Both core_words.go and lang/internal/engine now call
// ParseFnParams / ParseFnReturns / ResolveTypeName directly.)
