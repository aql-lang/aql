package eng

import "fmt"

// unifyListFamily owns unification when either side is in the List
// family (List, TypedList, Table) or is a bare List type literal.
//
// Canonicalization: if exactly one side is a type literal, normalize so
// `lit` is the type literal and `other` is the concrete side. If both
// sides are in the family, sort by shape rank so the more-general side
// comes first. This collapses the mirrored "aTyped vs concrete" and
// "concrete vs bTyped" arms in the prior implementation into one path.
func unifyListFamily(a Value, sa ValueShape, b Value, sb ValueShape) (Value, *UnifyError) {
	// If one side is the bare List type literal (`List`), it unifies
	// with any List-family value except a table.
	aLit := sa == ShapeTypeLiteral && denotedType(a).Equal(TList)
	bLit := sb == ShapeTypeLiteral && denotedType(b).Equal(TList)
	if aLit {
		if sb == ShapeTable {
			return Value{}, unifyFail("List type literal does not unify with Table", a, b)
		}
		if IsListShape(sb) || bLit {
			return b, nil
		}
		return Value{}, unifyFail("List type literal needs a list-family right-hand side", a, b)
	}
	if bLit {
		if sa == ShapeTable {
			return Value{}, unifyFail("List type literal does not unify with Table", a, b)
		}
		if IsListShape(sa) {
			return a, nil
		}
		return Value{}, unifyFail("List type literal needs a list-family left-hand side", a, b)
	}

	// At this point both sides must be in the List family for
	// unification to succeed.
	if !IsListShape(sa) || !IsListShape(sb) {
		return Value{}, unifyFail("list family requires list-shaped values on both sides", a, b)
	}

	// Table is exclusive — only unifies with another table.
	if sa == ShapeTable || sb == ShapeTable {
		if sa != sb {
			return Value{}, unifyFail("Table only unifies with Table", a, b)
		}
		aTT, _ := AsTableType(a)
		bTT, _ := AsTableType(b)
		unified, err := unifyRecordTypes(aTT.Record, bTT.Record)
		if err != nil {
			return Value{}, err
		}
		uRec, _ := AsRecordType(unified)
		return NewTableType(uRec), nil
	}

	// Both typed lists → unify child types.
	if sa == ShapeTypedList && sb == ShapeTypedList {
		aCT, _ := AsChildType(a)
		bCT, _ := AsChildType(b)
		unified, err := unifyInner(aCT.Child, bCT.Child)
		if err != nil {
			return Value{}, err.withPath("child")
		}
		return NewTypedList(unified), nil
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
		return Value{}, unifyFail(
			fmt.Sprintf("list length mismatch: %d vs %d", len(aElems), len(bElems)), a, b)
	}
	result := make([]Value, len(aElems))
	for i := range aElems {
		unified, err := unifyInner(aElems[i], bElems[i])
		if err != nil {
			return Value{}, err.withPath(fmt.Sprintf("[%d]", i))
		}
		result[i] = unified
	}
	return NewList(result), nil
}

// unifyTypedListWithConcrete unifies a child type constraint against
// each element of a concrete list. Every element must unify.
func unifyTypedListWithConcrete(childType Value, elems []Value) (Value, *UnifyError) {
	result := make([]Value, len(elems))
	for i, elem := range elems {
		unified, err := unifyInner(childType, elem)
		if err != nil {
			return Value{}, err.withPath(fmt.Sprintf("[%d]", i))
		}
		result[i] = unified
	}
	return NewList(result), nil
}
