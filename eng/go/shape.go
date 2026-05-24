package eng

// ValueShape is the kernel's single classifier for value shapes that
// participate in unification, equality, and structural dispatch. Every
// branch in the unifier and the equality default switches on a Shape
// rather than re-running its own ad hoc combination of IsRecordType /
// IsTypedMap / IsOptionsType / Parent.Equal(TMap) / Data==nil checks.
//
// The ordering is significant: smaller-valued shapes are "more general"
// (Any, None, Never) and larger-valued shapes are "more specific"
// (Record, Options, Table). Family handlers exploit this by sorting the
// pair so the more-general side is first — collapsing mirrored
// asymmetric arms into one canonical case.
type ValueShape int

const (
	ShapeUnknown     ValueShape = iota
	ShapeNever                  // bottom type, only unifies with itself
	ShapeNone                   // none value or none type literal
	ShapeAny                    // any value or any type literal
	ShapeCarrier                // Data==nil, Carrier=true — abstract value of a type
	ShapeTypeLiteral            // bare type literal (Data==nil, not carrier, not none/never/any)
	ShapeDisjunct               // DisjunctInfo
	ShapeFnUndef                // FnUndefInfo — structural fn-shape constraint
	ShapeFnDef                  // FnDefInfo on TFnDef
	ShapeFunction               // FnDefInfo on TFunction
	ShapeDepScalar              // DepScalarInfo — refined scalar with bounds
	ShapeScalar                 // concrete scalar leaf (Integer/Decimal/String/Boolean/Atom/...)
	ShapeList                   // plain list — Parent=TList, Data=ListPayload
	ShapeTypedList              // typed list — Parent=TList, Data=ChildTypeInfo
	ShapeTable                  // table — Parent=TList, Data=TableTypeInfo/TableData/Materializer
	ShapeMap                    // plain map — Parent=TMap, Data=MapPayload
	ShapeTypedMap               // typed map — Parent=TMap, Data=ChildTypeInfo
	ShapeRecord                 // record type — Parent=TMap, Data=RecordTypeInfo
	ShapeOptions                // options type — Parent=TMap, Data=OptionsTypeInfo
	ShapeObjectInstance         // object instance — Data=ObjectInstanceInfo
	ShapeObjectType             // object type — Data=ObjectTypeInfo
)

// Shape classifies v into exactly one ValueShape. The classification
// is total — every Value maps to one shape, with ShapeUnknown reserved
// for values the kernel hasn't been taught yet (extension payloads
// outside the structural taxonomy).
func Shape(v Value) ValueShape {
	// Special root types first — they're identified by lattice node,
	// not by payload shape.
	t := v.Parent
	if v.Data == nil && !v.Carrier && v.ID != "" {
		t = &v
	}
	if t.Equal(TNever) {
		return ShapeNever
	}
	if t.Equal(TNone) {
		return ShapeNone
	}
	if t.Equal(TAny) {
		return ShapeAny
	}

	// Payload-shaped values come next — they win over plain Data==nil
	// classification because a Disjunct/DepScalar/FnUndef payload is
	// the discriminator, not the Parent.
	if IsDisjunct(v) {
		return ShapeDisjunct
	}
	if v.IsDepScalar() {
		return ShapeDepScalar
	}
	if _, ok := v.Data.(FnUndefInfo); ok {
		return ShapeFnUndef
	}
	if _, ok := v.Data.(FnDefInfo); ok {
		if t.Equal(TFunction) {
			return ShapeFunction
		}
		return ShapeFnDef
	}
	if _, ok := v.Data.(ObjectInstanceInfo); ok {
		return ShapeObjectInstance
	}
	if _, ok := v.Data.(ObjectTypeInfo); ok {
		return ShapeObjectType
	}

	// Map family — every map-Parent value falls into one of these.
	if t.Equal(TMap) {
		switch {
		case v.Data == nil:
			if v.Carrier {
				return ShapeCarrier
			}
			return ShapeTypeLiteral
		case IsRecordType(v):
			return ShapeRecord
		case IsOptionsType(v):
			return ShapeOptions
		case IsTypedMap(v):
			return ShapeTypedMap
		default:
			return ShapeMap
		}
	}

	// List family.
	if t.Equal(TList) {
		switch {
		case v.Data == nil:
			if v.Carrier {
				return ShapeCarrier
			}
			return ShapeTypeLiteral
		case IsTableType(v):
			return ShapeTable
		case IsTypedList(v):
			return ShapeTypedList
		default:
			return ShapeList
		}
	}

	// Bare type literal vs carrier vs concrete scalar.
	if v.Data == nil {
		if v.Carrier {
			return ShapeCarrier
		}
		return ShapeTypeLiteral
	}
	return ShapeScalar
}

// IsMapShape reports whether s is in the Map family
// (Map, TypedMap, Record, Options).
func IsMapShape(s ValueShape) bool {
	switch s {
	case ShapeMap, ShapeTypedMap, ShapeRecord, ShapeOptions:
		return true
	}
	return false
}

// IsListShape reports whether s is in the List family
// (List, TypedList, Table).
func IsListShape(s ValueShape) bool {
	switch s {
	case ShapeList, ShapeTypedList, ShapeTable:
		return true
	}
	return false
}

// IsConcreteShape reports whether s is a concrete value (Data!=nil and
// not a bare type literal / carrier / disjunct / fn-undef).
func IsConcreteShape(s ValueShape) bool {
	switch s {
	case ShapeScalar, ShapeDepScalar,
		ShapeList, ShapeTypedList, ShapeTable,
		ShapeMap, ShapeTypedMap, ShapeRecord, ShapeOptions,
		ShapeFnDef, ShapeFunction,
		ShapeObjectInstance, ShapeObjectType:
		return true
	}
	return false
}
