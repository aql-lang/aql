package eng

// unifyListFamily owns unification when either side is in the List
// family (List, TypedList, Table) or is a bare List type literal.
//
// Canonicalization: if exactly one side is a type literal, normalize so
// `lit` is the type literal and `other` is the concrete side. If both
// sides are in the family, sort by shape rank so the more-general side
// comes first. This collapses the mirrored "aTyped vs concrete" and
// "concrete vs bTyped" arms in the prior implementation into one path.
func unifyListFamily(a Value, sa ValueShape, b Value, sb ValueShape) (Value, bool) {
	// If one side is the bare List type literal (`List`), it unifies
	// with any List-family value except a table.
	aLit := sa == ShapeTypeLiteral && denotedType(a).Equal(TList)
	bLit := sb == ShapeTypeLiteral && denotedType(b).Equal(TList)
	if aLit {
		if sb == ShapeTable {
			return Value{}, false
		}
		if IsListShape(sb) || bLit {
			return b, true
		}
		return Value{}, false
	}
	if bLit {
		if sa == ShapeTable {
			return Value{}, false
		}
		if IsListShape(sa) {
			return a, true
		}
		return Value{}, false
	}

	// At this point both sides must be in the List family for
	// unification to succeed.
	if !IsListShape(sa) || !IsListShape(sb) {
		return Value{}, false
	}

	// Table is exclusive — only unifies with another table.
	if sa == ShapeTable || sb == ShapeTable {
		if sa != sb {
			return Value{}, false
		}
		aTT, _ := AsTableType(a)
		bTT, _ := AsTableType(b)
		unified, ok := unifyRecordTypes(aTT.Record, bTT.Record)
		if !ok {
			return Value{}, false
		}
		uRec, _ := AsRecordType(unified)
		return NewTableType(uRec), true
	}

	// Both typed lists → unify child types.
	if sa == ShapeTypedList && sb == ShapeTypedList {
		aCT, _ := AsChildType(a)
		bCT, _ := AsChildType(b)
		unified, ok := Unify(aCT.Child, bCT.Child)
		if !ok {
			return Value{}, false
		}
		return NewTypedList(unified), true
	}

	// One side typed, the other concrete → each element must unify
	// with the child type. Canonicalize: typed on left.
	if sa == ShapeTypedList || sb == ShapeTypedList {
		var typed, concrete Value
		if sa == ShapeTypedList {
			typed, concrete = a, b
		} else {
			typed, concrete = b, a
		}
		ct, _ := AsChildType(typed)
		lst, _ := AsList(concrete)
		return unifyTypedListWithConcrete(ct.Child, lst.Slice())
	}

	// Both concrete lists → element-by-element.
	aLst, _ := AsList(a)
	bLst, _ := AsList(b)
	aElems := aLst.Slice()
	bElems := bLst.Slice()
	if len(aElems) != len(bElems) {
		return Value{}, false
	}
	result := make([]Value, len(aElems))
	for i := range aElems {
		unified, ok := Unify(aElems[i], bElems[i])
		if !ok {
			return Value{}, false
		}
		result[i] = unified
	}
	return NewList(result), true
}

// unifyTypedListWithConcrete unifies a child type constraint against
// each element of a concrete list. Every element must unify.
func unifyTypedListWithConcrete(childType Value, elems []Value) (Value, bool) {
	result := make([]Value, len(elems))
	for i, elem := range elems {
		unified, ok := Unify(childType, elem)
		if !ok {
			return Value{}, false
		}
		result[i] = unified
	}
	return NewList(result), true
}
