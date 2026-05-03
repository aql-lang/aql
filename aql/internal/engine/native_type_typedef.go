package engine

import (
	"fmt"
	"strings"
)

func RegisterTypeDef(r *Registry) {
	validateAndInstall := func(name string, body Value) error {
		if !isTypeValue(body) {
			return fmt.Errorf("type: body must be a type value (record, disjunct, type literal, typed list, or typed map), got %s", body.String())
		}
		if err := ValidateTypeNameParts(name, r.KnownTypeParts); err != nil {
			return err
		}
		// Refuse a type definition whose name already names a callable
		// or a def'd value. Type and def share a single source-level
		// namespace (the same Word resolves both), so allowing both
		// to bind the same name would silently change behaviour
		// depending on context.
		if r.Lookup(name) != nil {
			return fmt.Errorf("type %s: name clash — already a registered function", name)
		}
		if len(r.DefStacks[name]) > 0 {
			return fmt.Errorf("type %s: name clash — already a def'd value", name)
		}
		if _, ok := r.Types[name]; ok {
			return fmt.Errorf("type %s: already defined as a type", name)
		}
		// Type-defining functions (FnUndef = structural sig pattern,
		// FnDef/Function = predicate) live ONLY in r.Types: they are
		// not independently callable and only participate in type
		// operations (`def n:T v`, `v is T`, `inspect T`). Routing
		// them through installDef would either register Bbd as a
		// free-standing callable word (FnDef path) or trigger the
		// targeted-sig undef machinery (FnUndef path). Other type
		// kinds (literals, records, disjuncts, typed list/map,
		// ObjectType, DepScalar, …) keep the legacy installDef path
		// so they continue to round-trip through DefStacks for
		// auto-eval lookup. They're also mirrored into r.Types so
		// type ops can resolve every named type uniformly.
		if body.VType.Equal(TFnDef) || body.VType.Equal(TFunction) || body.VType.Equal(TFnUndef) {
			r.Types[name] = body
		} else {
			installDef(r, name, body)
			r.Types[name] = body
		}
		// Register the new name parts as known.
		for _, p := range strings.Split(name, "/") {
			r.KnownTypeParts[p] = true
		}
		return nil
	}

	// Forward handler: "type foo number" → args=[foo(name), number(body)]
	// Forward precedence handles all orderings without infix signatures.
	typeHandler := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		name := defName(args[0])
		body := args[1]
		if err := validateAndInstall(name, body); err != nil {
			return nil, err
		}
		return nil, nil
	}

	r.RegisterNativeFunc(NativeFunc{
		Name:              "type",
		ForwardPrecedence: true,
		Signatures: []NativeSig{
			{
				Args:           []Type{TString, TAny},
				Handler:        typeHandler,
				Returns:        []Type{},
				RunInCheckMode: true,
			},
			{
				Args:           []Type{TAtom, TAny},
				QuoteArgs:      map[int]bool{0: true},
				Handler:        typeHandler,
				Returns:        []Type{},
				RunInCheckMode: true,
			},
		},
	})
}

// isTypeValue reports whether a value is a valid type definition body.
func isTypeValue(v Value) bool {
	// Type literal (Data==nil): number, string, boolean, any, etc.
	if v.Data == nil {
		return true
	}
	// Record type
	if v.IsRecordType() {
		return true
	}
	// Options type
	if v.IsOptionsType() {
		return true
	}
	// Table type
	if v.IsTableType() {
		return true
	}
	// Disjunct
	if v.IsDisjunct() {
		return true
	}
	// Typed list [:type]
	if v.IsTypedList() {
		return true
	}
	// Typed map {:type}
	if v.IsTypedMap() {
		return true
	}
	// Object type
	if v.IsObjectType() {
		return true
	}
	// Dependent scalar type (Integer gt 10, String lt "z", …)
	if v.IsDepScalar() {
		return true
	}
	// Function-signature type: a FnUndef carrying input + output sig
	// patterns and no body. Produced by `fn [[input] [output]]`. Used
	// as a structural function shape — `def n:Mapper f` requires f to
	// be a function whose signatures match the FnUndef pattern.
	if v.VType.Equal(TFnUndef) {
		return true
	}
	// Predicate type: a FnDef / Function whose body returns a Boolean.
	// Produced by `fn [x:Any Any [body]]`. Used as a *dependent* type
	// — `def n:Bbd value` calls the predicate against `value` and
	// installs the def iff the predicate returns true. The fn's
	// signature must take exactly one argument and return a Boolean
	// (or a value that converts to Boolean); enforcement happens in
	// the typed-def handler when it actually calls the predicate.
	if v.VType.Equal(TFnDef) || v.VType.Equal(TFunction) {
		return true
	}
	return false
}
