package engine

import "fmt"

func registerTypeDef(r *Registry) {
	typeHandler := func(args []Value) ([]Value, error) {
		name := defName(args[0])
		body := args[1]

		// Validate that the body is a type-like value.
		if !isTypeValue(body) {
			return nil, fmt.Errorf("type: body must be a type value (record, disjunct, type literal, typed list, or typed map), got %s", body.String())
		}

		installDef(r, name, body)
		return nil, nil
	}

	r.Register("type",
		Signature{
			Args:    []Type{TWord, TAny},
			Handler: typeHandler,
		},
		Signature{
			Args:    []Type{TString, TAny},
			Handler: typeHandler,
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
	return false
}
