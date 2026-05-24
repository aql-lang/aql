package eng

import "fmt"

// Value equality — the Equal half of the TypeBehavior trio. Lives here
// (not in unify.go) so the kernel's Equal default sits next to
// defaultBehavior in typebehavior.go rather than buried inside the
// unification module.
//
// ValuesEqual is the public entry; valuesEqualDefault is the path
// defaultBehavior.Equal delegates to and the fall-through for
// per-type Behaviors that override only Format. listsEqual and
// mapsEqual are the structural helpers.

// ValuesEqual compares the data payloads of two values with the same type.
//
// Routes through Behavior.Equal for the same-Parent case so types
// with normalisation semantics (CalDuration, DepScalar in a future
// step, and plugin types) can supply their own equality. The
// cross-Parent case falls through to the default switch since
// equality across types is a matching-strategy concern, not a
// per-type concern.
func ValuesEqual(a, b Value) bool {
	// Pluggable equality: when both sides share a Parent with a
	// non-default Behavior, delegate. Type literals (Data==nil) are
	// excluded — bare type equality is a lattice-identity check, not
	// a per-type semantic compare.
	if a.Data != nil && b.Data != nil &&
		a.Parent != nil && a.Parent == b.Parent &&
		a.Parent.Behavior != nil && a.Parent.Behavior != DefaultBehavior {
		return a.Parent.Behavior.Equal(a, b)
	}
	return valuesEqualDefault(a, b)
}

// valuesEqualDefault is the kernel's default equality path,
// bypassing the Behavior dispatch in ValuesEqual. Used by
// DefaultBehavior.Equal and by Behavior implementations that
// override Format but want fall-through equality without
// triggering infinite re-entry.
func valuesEqualDefault(a, b Value) bool {
	// Two Data==nil values: carriers are abstract (conservatively
	// equal); two type literals are equal iff they are the same
	// lattice identity.
	if a.Data == nil && b.Data == nil {
		if a.Carrier || b.Carrier {
			return true
		}
		return a.Equal(&b)
	}
	// One is a type literal and the other is a concrete value — not equal.
	if a.Data == nil || b.Data == nil {
		return false
	}
	// Dependent scalar: route to payload comparison BEFORE the
	// Matches(TString)/Matches(TInteger)/... dispatch below. The
	// lattice override makes DepString.Matches(TString)=true, so
	// without this branch a DepScalar would fall into AsString and
	// silently compare zero-value payloads.
	if a.IsDepScalar() || b.IsDepScalar() {
		if !a.IsDepScalar() || !b.IsDepScalar() {
			return false
		}
		ai, err := a.AsDepScalar()
		if err != nil {
			return false
		}
		bi, err := b.AsDepScalar()
		if err != nil {
			return false
		}
		return depScalarsEqual(ai, bi)
	}
	switch {
	case a.Parent.Matches(TString):
		as, _ := AsString(a)
		bs, _ := AsString(b)
		return as == bs
	case a.Parent.Matches(TInteger):
		ai, _ := AsInteger(a)
		bi, _ := AsInteger(b)
		return ai == bi
	case a.Parent.Matches(TBoolean):
		ab, _ := AsBoolean(a)
		bb, _ := AsBoolean(b)
		return ab == bb
	case a.Parent.Equal(TList):
		aTT, aTbl := a.Data.(TableTypeInfo)
		bTT, bTbl := b.Data.(TableTypeInfo)
		if aTbl && bTbl {
			return mapsEqual(aTT.Record.Fields, bTT.Record.Fields)
		}
		if aTbl != bTbl {
			return false
		}
		aCT, aOk := a.Data.(ChildTypeInfo)
		bCT, bOk := b.Data.(ChildTypeInfo)
		if aOk && bOk {
			return aCT.Child.Parent.Equal(bCT.Child.Parent) && ValuesEqual(aCT.Child, bCT.Child)
		}
		if aOk != bOk {
			return false
		}
		aLst, _ := AsList(a)
		bLst, _ := AsList(b)
		return listsEqual(aLst.Slice(), bLst.Slice())
	case a.Parent.Equal(TMap):
		aRT, aRec := a.Data.(RecordTypeInfo)
		bRT, bRec := b.Data.(RecordTypeInfo)
		if aRec && bRec {
			return mapsEqual(aRT.Fields, bRT.Fields)
		}
		if aRec != bRec {
			return false
		}
		aOT, aOpt := a.Data.(OptionsTypeInfo)
		bOT, bOpt := b.Data.(OptionsTypeInfo)
		if aOpt && bOpt {
			return mapsEqual(aOT.Fields, bOT.Fields)
		}
		if aOpt != bOpt {
			return false
		}
		aCT, aOk := a.Data.(ChildTypeInfo)
		bCT, bOk := b.Data.(ChildTypeInfo)
		if aOk && bOk {
			return aCT.Child.Parent.Equal(bCT.Child.Parent) && ValuesEqual(aCT.Child, bCT.Child)
		}
		if aOk != bOk {
			return false
		}
		aMap, _ := AsMap(a)
		bMap, _ := AsMap(b)
		return mapsEqual(aMap, bMap)
	default:
		return fmt.Sprintf("%v", a.Data) == fmt.Sprintf("%v", b.Data)
	}
}

// listsEqual compares two list payloads element by element.
func listsEqual(a, b []Value) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !a[i].Parent.Equal(b[i].Parent) || !ValuesEqual(a[i], b[i]) {
			return false
		}
	}
	return true
}

// mapsEqual compares two map payloads by keys and values.
func mapsEqual(a, b ReadMap) bool {
	if a.Len() != b.Len() {
		return false
	}
	for _, k := range a.Keys() {
		aVal, _ := a.Get(k)
		bVal, ok := b.Get(k)
		if !ok {
			return false
		}
		if !aVal.Parent.Equal(bVal.Parent) || !ValuesEqual(aVal, bVal) {
			return false
		}
	}
	return true
}
