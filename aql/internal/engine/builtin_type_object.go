package engine

import "fmt"

func registerObject(r *Registry) {
	// parseObjectFields converts a map of field definitions into an OrderedMap
	// of field name → type-constraint Value, resolving type references.
	parseObjectFields := func(fieldsMap *OrderedMap) *OrderedMap {
		fields := NewOrderedMap()
		for _, key := range fieldsMap.Keys() {
			val, _ := fieldsMap.Get(key)
			val = resolveFieldType(r, val)
			fields.Set(key, val)
		}
		return fields
	}

	// objectHandler creates an object type from a field map (no parent).
	// Syntax: object {a:String, b:Boolean}
	objectHandler := func(args []Value) ([]Value, error) {
		fieldsVal := args[0]
		if !fieldsVal.VType.Equal(TMap) {
			return nil, fmt.Errorf("object: argument must be a map of field definitions, got %s", fieldsVal.String())
		}
		m, ok := fieldsVal.Data.(*OrderedMap)
		if !ok {
			return nil, fmt.Errorf("object: argument must be a concrete map, got %s", fieldsVal.String())
		}
		fields := parseObjectFields(m)
		id := GenerateObjectTypeID()
		info := ObjectTypeInfo{
			Fields: fields,
			Parent: nil,
			ID:     id,
			Name:   "", // set by installDef when used with def
		}
		return []Value{NewObjectType(info)}, nil
	}

	// objectWithParentHandler creates an object type with a parent.
	// Syntax: object {d:Integer} Foo
	// The second argument must be an existing object type.
	objectWithParentHandler := func(args []Value) ([]Value, error) {
		fieldsVal := args[0]
		parentVal := args[1]

		if !fieldsVal.VType.Equal(TMap) {
			return nil, fmt.Errorf("object: first argument must be a map of field definitions, got %s", fieldsVal.String())
		}
		m, ok := fieldsVal.Data.(*OrderedMap)
		if !ok {
			return nil, fmt.Errorf("object: first argument must be a concrete map, got %s", fieldsVal.String())
		}

		if !parentVal.IsObjectType() {
			return nil, fmt.Errorf("object: parent must be an object type, got %s", parentVal.String())
		}
		parentInfo := parentVal.AsObjectType()

		fields := parseObjectFields(m)
		id := GenerateObjectTypeID()
		info := ObjectTypeInfo{
			Fields: fields,
			Parent: &parentInfo,
			ID:     id,
			Name:   "", // set by installDef when used with def
		}
		return []Value{NewObjectType(info)}, nil
	}

	r.Register("object",
		// 2-arg: map + parent object type
		Signature{
			Args:    []Type{TMap, TObject},
			Handler: objectWithParentHandler,
		},
		// 1-arg: map only (no parent)
		Signature{
			Args:    []Type{TMap},
			Handler: objectHandler,
		},
	)
}
