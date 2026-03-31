package native

import (
	"github.com/metsitaba/voxgig-exp/aql/internal/engine"
)

// istypeFunc returns the "istype" native function definition.
// istype checks whether a value is a type literal, an Options instance,
// or a Node (List/Map) containing a leaf that is a type.
func istypeFunc() NativeFunc {
	return NativeFunc{
		Name:              "istype",
		ForwardPrecedence: true,
		SkipSafetyCheck:   true,
		Signatures: []NativeSig{
			{
				Args:    []engine.Type{engine.TAny},
				Handler: istypeHandler,
			},
		},
	}
}

func istypeHandler(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
	return []engine.Value{engine.NewBoolean(isTypeValue(args[0]))}, nil
}

// isTypeValue returns true if v is a type literal, an Options instance,
// or a Node that contains a leaf that is a type.
func isTypeValue(v engine.Value) bool {
	// Type literal: Data==nil with a real type (not None).
	if v.Data == nil && !v.VType.Equal(engine.TNone) {
		return true
	}

	// Options type, record type, typed list/map, table type, object type.
	if v.IsOptionsType() || v.IsRecordType() || v.IsTypedList() ||
		v.IsTypedMap() || v.IsTableType() || v.IsObjectType() {
		return true
	}

	// Concrete list: check each element recursively.
	if v.VType.Matches(engine.TList) && v.Data != nil {
		elems := v.AsList()
		if elems != nil {
			for _, elem := range elems {
				if isTypeValue(elem) {
					return true
				}
			}
		}
	}

	// Concrete map: check each value recursively.
	if v.VType.Matches(engine.TMap) && v.Data != nil {
		m := v.AsMap()
		if m != nil {
			for _, key := range m.Keys() {
				val, _ := m.Get(key)
				if isTypeValue(val) {
					return true
				}
			}
		}
	}

	return false
}
