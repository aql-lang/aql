package engine

import "fmt"

func RegisterObject(r *Registry) {
	// parseObjectFields converts a map of field definitions into an OrderedMap
	// of field name → type-constraint Value, resolving type references.
	parseObjectFields := func(fieldsMap *OrderedMap) *OrderedMap {
		fields := NewOrderedMap()
		for _, key := range fieldsMap.Keys() {
			val, _ := fieldsMap.Get(key)
			val = ResolveFieldType(r, val)
			fields.Set(key, val)
		}
		return fields
	}

	// objectHandler creates an object type from a field map (no parent).
	// Syntax: object {a:String, b:Boolean}
	objectHandler := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
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
			Name:   "", // set by InstallDef when used with def
		}
		return []Value{NewObjectType(info)}, nil
	}

	// objectWithParentHandler creates an object type with a parent.
	// Syntax: object {d:Integer} Foo
	// The second argument must be an existing object type.
	objectWithParentHandler := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
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
		parentInfo, _ := parentVal.AsObjectType()

		fields := parseObjectFields(m)

		// Validate field narrowing: child fields that override parent fields
		// must be narrower (unify with the parent constraint succeeds and
		// the result matches the child's constraint, not the parent's).
		parentAllFields := parentInfo.AllFields()
		for _, key := range fields.Keys() {
			childConstraint, _ := fields.Get(key)
			parentConstraint, exists := parentAllFields.Get(key)
			if !exists {
				continue // new field, no narrowing check needed
			}
			// The child constraint must unify with the parent constraint.
			// If unification fails, the child is expanding the type.
			_, ok := Unify(parentConstraint, childConstraint)
			if !ok {
				return nil, fmt.Errorf("object: field %q in child type cannot expand parent type %s (child: %s, parent: %s)",
					key, parentInfo.Name, childConstraint.String(), parentConstraint.String())
			}
		}

		id := GenerateObjectTypeID()
		info := ObjectTypeInfo{
			Fields: fields,
			Parent: &parentInfo,
			ID:     id,
			Name:   "", // set by InstallDef when used with def
		}
		return []Value{NewObjectType(info)}, nil
	}

	r.RegisterNativeFunc(NativeFunc{
		Name:              "object",
		ForwardPrecedence: true,
		Signatures: []NativeSig{
			// 2-arg: map + parent object type
			{
				Args:           []Type{TMap, TObject},
				Handler:        objectWithParentHandler,
				Returns:        []Type{TObjectType},
				RunInCheckMode: true,
			},
			// 1-arg: map only (no parent)
			{
				Args:           []Type{TMap},
				Handler:        objectHandler,
				Returns:        []Type{TObjectType},
				RunInCheckMode: true,
			},
		},
	})
}
