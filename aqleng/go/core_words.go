package aqleng

import (
	"fmt"
	"strings"
)

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
//   Stack manipulation (Forth-style; all stack-only):
//     dup        — duplicate top                  ( a — a a )
//     swap       — exchange top two               ( a b — b a )
//     drop       — remove top                     ( a — )
//     over       — copy second-from-top to top    ( a b — a b a )
//     rot        — rotate top three               ( a b c — b c a )
//     nip        — drop second-from-top           ( a b — b )
//     tuck       — copy top under second          ( a b — b a b )
//     2dup       — duplicate top pair             ( a b — a b a b )
//     2swap      — swap top two pairs             ( a b c d — c d a b )
//     2drop      — remove top two                 ( a b — )
//     2over      — copy second pair to top        ( a b c d — a b c d a b )
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
				if info, ok := args[1].Data.(FnDefInfo); ok && len(info.Sigs) == 1 {
					installCoreFnDef(reg, w.Name, info.Sigs[0])
					return []Value{}, nil
				}
				reg.PushDef(w.Name, args[1])
				return []Value{}, nil
			},
			Returns: []Type{},
		}},
	})
}

// registerCoreFn installs `fn [params] [returns] [body]`. Each param
// is a Word of the form `name:TypeName`; the bare aqleng tokenizer
// is whitespace-only so a typed param arrives as a single Word and
// the handler splits on `:` to recover the (name, type) pair.
func registerCoreFn(r *Registry) {
	r.RegisterNativeFunc(NativeFunc{
		Name:              "fn",
		ForwardPrecedence: true,
		Signatures: []NativeSig{{
			Args:       []Type{TList, TList, TList},
			NoEvalArgs: map[int]bool{0: true, 1: true, 2: true},
			Handler: func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
				paramsList := args[0].AsList()
				returnsList := args[1].AsList()
				body := args[2].AsList()

				params := make([]FnParam, paramsList.Len())
				for i := 0; i < paramsList.Len(); i++ {
					p, err := parseCoreFnParam(paramsList.Get(i))
					if err != nil {
						return nil, err
					}
					params[i] = p
				}
				returns := make([]Type, returnsList.Len())
				for i := 0; i < returnsList.Len(); i++ {
					t, err := parseCoreFnReturn(returnsList.Get(i))
					if err != nil {
						return nil, err
					}
					returns[i] = t
				}

				info := FnDefInfo{
					Sigs: []FnSig{{
						Params:  params,
						Returns: returns,
						Body:    body.Slice(),
					}},
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
func registerCoreStack(r *Registry) {
	// dup ( a — a a )
	r.RegisterNativeFunc(NativeFunc{
		Name: "dup",
		Signatures: []NativeSig{{
			Args: []Type{TAny},
			Handler: func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
				return []Value{args[0], args[0]}, nil
			},
		}},
	})
	// swap ( a b — b a ).
	// Under the unified §1.4 rule args[0] is the top and args[1] is
	// next-deeper. Returning [args[0], args[1]] in source order puts
	// the old top at the deeper position and the old second-from-top
	// at the top — i.e. they swap.
	r.RegisterNativeFunc(NativeFunc{
		Name: "swap",
		Signatures: []NativeSig{{
			Args: []Type{TAny, TAny},
			Handler: func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
				return []Value{args[0], args[1]}, nil
			},
		}},
	})
	// drop ( a — )
	r.RegisterNativeFunc(NativeFunc{
		Name: "drop",
		Signatures: []NativeSig{{
			Args: []Type{TAny},
			Handler: func(_ []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
				return nil, nil
			},
		}},
	})
	// over ( a b — a b a ).
	// args[0]=top=b, args[1]=deeper=a. Output sequence puts a at the
	// deepest restore position, then b, then a again on top.
	r.RegisterNativeFunc(NativeFunc{
		Name: "over",
		Signatures: []NativeSig{{
			Args: []Type{TAny, TAny},
			Handler: func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
				return []Value{args[1], args[0], args[1]}, nil
			},
		}},
	})
	// rot ( a b c — b c a ).
	// args[0]=top=c, args[1]=b, args[2]=deepest=a.
	r.RegisterNativeFunc(NativeFunc{
		Name: "rot",
		Signatures: []NativeSig{{
			Args: []Type{TAny, TAny, TAny},
			Handler: func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
				return []Value{args[1], args[0], args[2]}, nil
			},
		}},
	})
	// nip ( a b — b ).
	// args[0]=top=b is kept; args[1]=a is discarded.
	r.RegisterNativeFunc(NativeFunc{
		Name: "nip",
		Signatures: []NativeSig{{
			Args: []Type{TAny, TAny},
			Handler: func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
				return []Value{args[0]}, nil
			},
		}},
	})
	// tuck ( a b — b a b ).
	// args[0]=top=b, args[1]=a. Output: b at deepest, a in middle,
	// b at top — copies the top under the second.
	r.RegisterNativeFunc(NativeFunc{
		Name: "tuck",
		Signatures: []NativeSig{{
			Args: []Type{TAny, TAny},
			Handler: func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
				return []Value{args[0], args[1], args[0]}, nil
			},
		}},
	})
	// 2dup ( a b — a b a b ).
	// args[0]=top=b, args[1]=a.
	r.RegisterNativeFunc(NativeFunc{
		Name: "2dup",
		Signatures: []NativeSig{{
			Args: []Type{TAny, TAny},
			Handler: func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
				return []Value{args[1], args[0], args[1], args[0]}, nil
			},
		}},
	})
	// 2swap ( a b c d — c d a b ).
	// args[0]=top=d, args[1]=c, args[2]=b, args[3]=deepest=a.
	r.RegisterNativeFunc(NativeFunc{
		Name: "2swap",
		Signatures: []NativeSig{{
			Args: []Type{TAny, TAny, TAny, TAny},
			Handler: func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
				return []Value{args[1], args[0], args[3], args[2]}, nil
			},
		}},
	})
	// 2drop ( a b — )
	r.RegisterNativeFunc(NativeFunc{
		Name: "2drop",
		Signatures: []NativeSig{{
			Args: []Type{TAny, TAny},
			Handler: func(_ []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
				return nil, nil
			},
		}},
	})
	// 2over ( a b c d — a b c d a b ).
	// Copies the second pair (a b) on top of the first pair (c d).
	r.RegisterNativeFunc(NativeFunc{
		Name: "2over",
		Signatures: []NativeSig{{
			Args: []Type{TAny, TAny, TAny, TAny},
			Handler: func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
				return []Value{args[3], args[2], args[1], args[0], args[3], args[2]}, nil
			},
		}},
	})
}

// installCoreFnDef wires a single FnSig into the registry as a
// synthesised native: when invoked, it binds each named param onto
// the def stack, pushes the matched args onto the args stack so the
// body can read them via the `args` word, runs the body in a fresh
// sub-engine, and pops both stacks on return.
//
// Mirrors the param-binding portion of the production engine's
// InstallFnDef in aql/internal/engine/core_helpers.go without pulling
// in the production engine's `__pa` / paren-marker machinery (which
// belongs to the full parser, not the bare aqleng core).
func installCoreFnDef(r *Registry, name string, sig FnSig) {
	argTypes := make([]Type, len(sig.Params))
	for i, p := range sig.Params {
		argTypes[i] = p.Type
	}
	bodyCopy := append([]Value{}, sig.Body...)
	r.RegisterNativeFunc(NativeFunc{
		Name:              name,
		ForwardPrecedence: true,
		Signatures: []NativeSig{{
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
		}},
	})
}

// parseCoreFnParam splits a `name:TypeName` Word into an FnParam.
func parseCoreFnParam(v Value) (FnParam, error) {
	if !v.IsWord() {
		return FnParam{}, fmt.Errorf("fn: expected param Word, got %s", v.String())
	}
	w, _ := v.AsWord()
	idx := strings.Index(w.Name, ":")
	if idx < 0 {
		return FnParam{}, fmt.Errorf("fn: param %q missing ':TypeName' suffix", w.Name)
	}
	name := w.Name[:idx]
	typeName := w.Name[idx+1:]
	t, err := parseCoreTypeName(typeName)
	if err != nil {
		return FnParam{}, err
	}
	return FnParam{Name: name, Type: t}, nil
}

// parseCoreFnReturn parses a single Word as a return type name.
func parseCoreFnReturn(v Value) (Type, error) {
	if !v.IsWord() {
		return Type{}, fmt.Errorf("fn: expected return-type Word, got %s", v.String())
	}
	w, _ := v.AsWord()
	return parseCoreTypeName(w.Name)
}

// parseCoreTypeName resolves a type name (e.g. "Integer", "String")
// to its Type. Returns an error for names not in TypeNameTable.
func parseCoreTypeName(name string) (Type, error) {
	tn, ok := TypeNameTable()[name]
	if !ok {
		return Type{}, fmt.Errorf("fn: unknown type %q", name)
	}
	return tn, nil
}
