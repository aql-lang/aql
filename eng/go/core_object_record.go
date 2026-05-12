package eng

import "fmt"

// RegisterCoreObjectRecord installs the two structural type-constructor
// words — `record` and `object` — and is wired into RegisterCoreWords.
// Exported so callers can install just these without taking the rest of
// RegisterCoreWords.
//
//	record [ {a:Integer} {b:String} … ]   build a RecordType from a
//	                                       list of single-pair maps;
//	                                       the explicit, named-shape
//	                                       counterpart of the implicit
//	                                       `{a:Integer}` literal.
//	object {a:String b:Integer …}          build an anonymous ObjectType
//	                                       (nominal, inheritance-aware);
//	object {b:Integer …} ParentType        build an ObjectType that
//	                                       extends ParentType — child
//	                                       fields must unify with the
//	                                       parent's same-named fields.
//
// The RecordType / ObjectType *values* these produce are type bodies:
// name one with `type Foo record […]` / `type Foo object {…}`, then
// instantiate with `make Foo data` (the kernel `make` validates that
// `data` carries every declared field with a conforming value).
//
// Both run under CheckMode (RunInCheckMode) — downstream `make NAME`
// needs the constructed type even during static analysis.
func RegisterCoreObjectRecord(r *Registry) {
	registerCoreObjectRecord(r)
}

func registerCoreObjectRecord(r *Registry) {
	r.RegisterNativeFunc(NativeFunc{
		Name:        "record",
		ForwardArgs: true,
		Signatures: []NativeSig{{
			Args:           []*Type{TList},
			Handler:        recordHandler,
			Returns:        []*Type{TRecord},
			RunInCheckMode: true,
		}},
	})
	r.RegisterNativeFunc(NativeFunc{
		Name:        "object",
		ForwardArgs: true,
		Signatures: []NativeSig{
			{
				Args:           []*Type{TMap, TObject},
				Handler:        objectWithParentHandler,
				Returns:        []*Type{TObjectType},
				RunInCheckMode: true,
			},
			{
				Args:           []*Type{TMap},
				Handler:        objectHandler,
				Returns:        []*Type{TObjectType},
				RunInCheckMode: true,
			},
		},
	})
}

// ---- record ----

func recordHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	list := args[0]
	if !list.VType.Equal(TList) {
		return nil, fmt.Errorf("record: argument must be a list")
	}
	if list.Data == nil {
		return nil, fmt.Errorf("record: argument must be a concrete list, got type literal")
	}
	elems := list.AsList()
	if elems.Len() == 0 {
		return nil, fmt.Errorf("record: list must have at least one field")
	}
	fields := NewOrderedMap()
	for _, elem := range elems.Slice() {
		if !elem.VType.Equal(TMap) {
			return nil, fmt.Errorf("record: each element must be a pair (map), got %s", elem.String())
		}
		m, ok := elem.Data.(*OrderedMap)
		if !ok {
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

// ---- object ----

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
	m, ok := fieldsVal.Data.(*OrderedMap)
	if !ok {
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
	m, ok := fieldsVal.Data.(*OrderedMap)
	if !ok {
		return nil, fmt.Errorf("object: first argument must be a concrete map, got %s", fieldsVal.String())
	}

	if !parentVal.IsObjectType() {
		return nil, fmt.Errorf("object: parent must be an object type, got %s", parentVal.String())
	}
	parentInfo, _ := parentVal.AsObjectType()

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
