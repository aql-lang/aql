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
		// installDef would interpret a FnUndef body as the existing
		// "targeted-sig undef" command (`def name fn [spec]` removes
		// matching sigs). For the `type Mapper fn [[Integer] [Integer]]`
		// surface form we want the FnUndef to BIND as a structural
		// function-shape type, so push it directly to DefStacks here.
		if body.VType.Equal(TFnUndef) {
			r.DefStacks[name] = append(r.DefStacks[name], body)
		} else {
			installDef(r, name, body)
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
