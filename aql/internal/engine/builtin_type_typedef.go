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

	// All-suffix handler: "type foo number" → args=[foo(name), number(body)]
	typeSuffixHandler := func(args []Value) ([]Value, error) {
		name := defName(args[0])
		body := args[1]
		if err := validateAndInstall(name, body); err != nil {
			return nil, err
		}
		return nil, nil
	}

	// Infix handler: "number type foo" → args=[number(body), foo(name)]
	typeInfixHandler := func(args []Value) ([]Value, error) {
		body := args[0]
		name := defName(args[1])
		if err := validateAndInstall(name, body); err != nil {
			return nil, err
		}
		return nil, nil
	}

	r.Register("type",
		Signature{
			Args:    []Type{TWord, TAny},
			Handler: typeSuffixHandler,
		},
		Signature{
			Args:    []Type{TString, TAny},
			Handler: typeSuffixHandler,
		},
		Signature{
			Args:    []Type{TAny, TWord},
			Handler: typeInfixHandler,
		},
		Signature{
			Args:    []Type{TAny, TString},
			Handler: typeInfixHandler,
		},
	)
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
