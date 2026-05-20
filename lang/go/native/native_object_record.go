package native

import "fmt"

// This file holds the record / object type-construction handlers.
// They are not registered as words of their own — the `maketype`
// constructor (native_type.go) dispatches to them:
//
//	maketype Record [a:Integer b:String …]   RecordType from a list
//	                                          of single-pair maps.
//	maketype Object {a:String b:Integer …}    anonymous ObjectType
//	                                          (nominal, inheritance-aware).
//	maketype <objtype> {b:Integer …}          ObjectType extending the
//	                                          parent — child fields must
//	                                          unify with the parent's
//	                                          same-named fields.
//
// Algorithms (ResolveFieldType, Unify, MintType, NewRecordType,
// NewObjectType, …) live in eng.

func recordHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	list := args[0]
	if !list.VType.Equal(TList) {
		return nil, r.AqlError("record_error", "record: argument must be a list", "record")
	}
	if list.Data == nil {
		return nil, r.AqlError("record_error", "record: argument must be a concrete list, got type literal", "record")
	}
	elems, _ := AsList(list)
	if elems.Len() == 0 {
		return nil, r.AqlError("record_error", "record: list must have at least one field", "record")
	}
	fields := NewOrderedMap()
	for _, elem := range elems.Slice() {
		if !elem.VType.Equal(TMap) {
			return nil, fmt.Errorf("record: each element must be a pair (map), got %s", elem.String())
		}
		m, err := AsMutableMap(elem)
		if err != nil {
			return nil, fmt.Errorf("record: each element must be a concrete pair, got %s", elem.String())
		}
		for _, key := range m.Keys() {
			val, _ := m.Get(key)
			val = ResolveFieldType(r, val)
			fields.Set(key, val)
		}
	}
	return []Value{NewRecordType(fields)}, nil
}

// parseObjectFields converts a map of field definitions into an
// OrderedMap of field name → type-constraint Value, resolving type
// references via r.
func parseObjectFields(fieldsMap *OrderedMap, r *Registry) *OrderedMap {
	fields := NewOrderedMap()
	for _, key := range fieldsMap.Keys() {
		val, _ := fieldsMap.Get(key)
		val = ResolveFieldType(r, val)
		fields.Set(key, val)
	}
	return fields
}

func objectHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	fieldsVal := args[0]
	if !fieldsVal.VType.Equal(TMap) {
		return nil, fmt.Errorf("object: argument must be a map of field definitions, got %s", fieldsVal.String())
	}
	m, err := AsMutableMap(fieldsVal)
	if err != nil {
		return nil, fmt.Errorf("object: argument must be a concrete map, got %s", fieldsVal.String())
	}
	fields := parseObjectFields(m, r)
	id := GenerateObjectTypeID()
	info := ObjectTypeInfo{
		Fields: fields,
		Parent: nil,
		ID:     id,
		Name:   "",
	}
	def := r.Types.MintType(id, TObject)
	return []Value{NewObjectType(def, info)}, nil
}

func objectWithParentHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	fieldsVal := args[0]
	parentVal := args[1]

	if !fieldsVal.VType.Equal(TMap) {
		return nil, fmt.Errorf("object: first argument must be a map of field definitions, got %s", fieldsVal.String())
	}
	m, err := AsMutableMap(fieldsVal)
	if err != nil {
		return nil, fmt.Errorf("object: first argument must be a concrete map, got %s", fieldsVal.String())
	}

	if !IsObjectType(parentVal) {
		return nil, fmt.Errorf("object: parent must be an object type, got %s", parentVal.String())
	}
	parentInfo, _ := AsObjectType(parentVal)

	fields := parseObjectFields(m, r)

	parentAllFields := parentInfo.AllFields()
	for _, key := range fields.Keys() {
		childConstraint, _ := fields.Get(key)
		parentConstraint, exists := parentAllFields.Get(key)
		if !exists {
			continue
		}
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
		Name:   "",
	}
	parentDef := parentInfo.Type
	if parentDef == nil {
		parentDef = TObject
	}
	def := r.Types.MintType(id, parentDef)
	return []Value{NewObjectType(def, info)}, nil
}
