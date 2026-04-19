package engine

import (
	"fmt"
	"strings"
)

func registerTypeDef(r *Registry) {
	validateAndInstall := func(name string, body Value) error {
		if !isTypeValue(body) {
			return fmt.Errorf("type: body must be a type value (record, disjunct, type literal, typed list, or typed map), got %s", body.String())
		}
		if err := ValidateTypeNameParts(name, r.KnownTypeParts); err != nil {
			return err
		}
		installDef(r, name, body)
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
				Args:    []Type{TString, TAny},
				Handler: typeHandler,
				Returns: []Type{},
			},
			{
				Args:      []Type{TAtom, TAny},
				QuoteArgs: map[int]bool{0: true},
				Handler:   typeHandler,
				Returns:   []Type{},
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
	return false
}
