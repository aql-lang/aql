package aqleng

import (
	"strings"
)

// registerCoreType installs the language-fundamental type words:
//
//   type NAME body    — bind a type name (NAME must start [A-Z]) to a
//                       type body (a type literal, disjunct, typed
//                       list/map, or any value satisfying IsTypeBody).
//                       Pushes onto the type stack — `type Foo Integer;
//                       type Foo String` shadows; `untype Foo` pops.
//
//   untype NAME       — pop the most-recent binding for NAME from the
//                       type stack. Errors if no binding exists.
//
//   typeof v          — return the leaf-or-second-part of v's VType
//                       (for non-concrete types, the metatype). Returns
//                       an Atom: e.g. typeof 5 → atom(Integer),
//                       typeof "x" → atom(String), typeof Integer →
//                       atom(Type) (the metatype of a type literal).
//
// Mirrors the production aql `type` / `untype` / `typeof` (see
// aql/internal/engine/native_type.go); these are the foundational
// type-system surface that the richer words (record, table, object,
// make, is, guard, …) build on. The richer words are NOT in aqleng's
// core — they layer on top in the production engine.
func registerCoreType(r *Registry) {
	registerCoreTypeWord(r)
	registerCoreUntypeWord(r)
	registerCoreTypeof(r)
}

// registerCoreTypeWord installs `type NAME body`. NAME arrives as a
// Word (the bare aqleng tokenizer never quotes; the production parser's
// /q machinery captures the name as an Atom — both forms route here).
//
// The body is unquoted (NoEvalArgs[1] = true) because type bodies are
// literal type expressions, not code to evaluate.
func registerCoreTypeWord(r *Registry) {
	r.RegisterNativeFunc(NativeFunc{
		Name:              "type",
		ForwardPrecedence: true,
		Signatures: []NativeSig{{
			Args:       []Type{TWord, TAny},
			NoEvalArgs: map[int]bool{1: true},
			Handler: func(args []Value, _ map[string]Value, _ []Value, reg *Registry) ([]Value, error) {
				w, _ := args[0].AsWord()
				body := args[1]
				if err := installType(reg, w.Name, body); err != nil {
					return nil, err
				}
				return nil, nil
			},
			Returns: []Type{},
		}},
	})
}

// registerCoreUntypeWord installs `untype NAME`. Name arrives as a Word.
func registerCoreUntypeWord(r *Registry) {
	r.RegisterNativeFunc(NativeFunc{
		Name:              "untype",
		ForwardPrecedence: true,
		Signatures: []NativeSig{{
			Args: []Type{TWord},
			Handler: func(args []Value, _ map[string]Value, _ []Value, reg *Registry) ([]Value, error) {
				w, _ := args[0].AsWord()
				if !IsCapitalisedName(w.Name) {
					return nil, &AqlError{
						Code:   "type_error",
						Detail: "untype " + w.Name + ": type names must start with a capital letter",
					}
				}
				if !reg.PopType(w.Name) {
					return nil, &AqlError{
						Code:   "type_error",
						Detail: "untype " + w.Name + ": no such type binding",
					}
				}
				return nil, nil
			},
			Returns: []Type{},
		}},
	})
}

// registerCoreTypeof installs `typeof v`. Returns an Atom carrying the
// "leaf" type-name part of v's VType:
//
//   typeof 5            → atom(Integer)        (Scalar/Number/Integer leaf)
//   typeof 5.0          → atom(Decimal)
//   typeof "x"          → atom(String)
//   typeof true         → atom(Boolean)
//   typeof null         → atom(None)
//   typeof [ 1 2 ]      → atom(List)
//   typeof Integer      → atom(Type)           (metatype of a type literal)
//
// The "leaf" rule: take parts[1] if present, else parts[0]. So a
// type-literal (Data==nil, non-Word) reports its metatype rather than
// the type itself. This matches the production aql typeof.
func registerCoreTypeof(r *Registry) {
	r.RegisterNativeFunc(NativeFunc{
		Name:              "typeof",
		ForwardPrecedence: true,
		Signatures: []NativeSig{{
			Args: []Type{TAny},
			Handler: func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
				v := args[0]
				parts := v.VType.Parts
				if v.Data == nil && !v.VType.Matches(TWord) {
					parts = MetatypeFor(v.VType).Parts
				}
				// Trim a trailing numeric/disambiguator part (e.g. /-1).
				if len(parts) > 0 {
					last := parts[len(parts)-1]
					if len(last) > 0 && last[0] >= '0' && last[0] <= '9' {
						parts = parts[:len(parts)-1]
					}
					if len(last) > 1 && last[0] == '-' && last[1] >= '0' && last[1] <= '9' {
						parts = parts[:len(parts)-1]
					}
				}
				if len(parts) == 0 {
					return []Value{NewAtom("")}, nil
				}
				result := parts[0]
				if len(parts) > 1 {
					result = parts[1]
				}
				return []Value{NewAtom(result)}, nil
			},
			Returns: []Type{TAtom},
		}},
	})
}

// installType validates a (name, body) pair and pushes the body onto
// the type stack. Mirrors production aql validateAndInstallType minus
// the ObjectType naming (ObjectType bodies are constructed by the
// production-only `object` word, which isn't in aqleng's core).
func installType(r *Registry, name string, body Value) error {
	if !IsTypeBody(body) {
		return &AqlError{
			Code:   "type_error",
			Detail: "type: body must be a type value, got " + body.String(),
		}
	}
	if !IsCapitalisedName(name) {
		return &AqlError{
			Code:   "type_error",
			Detail: "type " + name + ": type names must start with a capital letter",
		}
	}
	if !r.HasType(name) {
		if err := ValidateTypeNameParts(name, r.KnownTypeParts); err != nil {
			return err
		}
	}
	if r.Lookup(name) != nil {
		return &AqlError{
			Code:   "type_error",
			Detail: "type " + name + ": name clash — already a registered function",
		}
	}
	if r.HasDef(name) {
		return &AqlError{
			Code:   "type_error",
			Detail: "type " + name + ": name clash — already a def'd value",
		}
	}
	r.PushType(name, body)
	for _, p := range strings.Split(name, "/") {
		r.KnownTypeParts[p] = true
	}
	return nil
}
