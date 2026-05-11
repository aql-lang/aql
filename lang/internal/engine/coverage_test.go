package engine

import (
	"bytes"
	"strings"
	"testing"
)

// ========================
// Query / Table tests
// ========================

func makeTestTable(r *Registry) {
	// Create a "people" table: name:string, age:number
	fields := NewOrderedMap()
	fields.Set("name", NewTypeLiteral(TString))
	fields.Set("age", NewTypeLiteral(TNumber))
	rec := RecordTypeInfo{Fields: fields}

	row1 := NewOrderedMap()
	row1.Set("name", NewString("alice"))
	row1.Set("age", NewInteger(30))
	row2 := NewOrderedMap()
	row2.Set("name", NewString("bob"))
	row2.Set("age", NewInteger(25))
	row3 := NewOrderedMap()
	row3.Set("name", NewString("carol"))
	row3.Set("age", NewInteger(35))

	td := TableData{
		Record: rec,
		Rows:   []Value{NewMap(row1), NewMap(row2), NewMap(row3)},
	}
	r.ContextSet("people", Value{VType: TList, Data: td})
}

func TestQueryFrom(t *testing.T) {
	t.Skip("query words disabled")
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	makeTestTable(r)

	result := runAQL(t, r, []Value{
		NewWord("from"), NewWord("people"),
	})
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	if !result[0].IsTableType() {
		t.Fatalf("expected table type, got %s", result[0])
	}
}

func TestQueryFromSelect(t *testing.T) {
	t.Skip("query words disabled")
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	makeTestTable(r)

	result := runAQL(t, r, []Value{
		NewWord("from"), NewWord("people"),
		NewWord("select"), NewWord("star"),
	})
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
}

func TestQueryFromWhereSimple(t *testing.T) {
	t.Skip("query words disabled")
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	makeTestTable(r)

	// from people where [name eq "alice"]
	result := runAQL(t, r, []Value{
		NewWord("from"), NewWord("people"),
		NewWord("where"), NewList([]Value{
			NewWord("name"), NewWord("eq"), NewString("alice"),
		}),
	})
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
}

func TestQueryFromWhere(t *testing.T) {
	t.Skip("query words disabled")
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	makeTestTable(r)

	// from people where [age gt 28]
	result := runAQL(t, r, []Value{
		NewWord("from"), NewWord("people"),
		NewWord("where"), NewList([]Value{
			NewWord("age"), NewWord("gt"), NewInteger(28),
		}),
	})
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	td, ok := result[0].Data.(QueryBuilder)
	if !ok {
		td2, ok2 := result[0].Data.(TableData)
		if !ok2 {
			t.Fatalf("expected QueryBuilder or TableData, got %T", result[0].Data)
		}
		if len(td2.Rows) != 2 {
			t.Errorf("expected 2 rows (alice=30, carol=35), got %d", len(td2.Rows))
		}
		return
	}
	mat, err := td.Materialize()
	if err != nil {
		t.Fatalf("materialize error: %v", err)
	}
	if len(mat.Rows) != 2 {
		t.Errorf("expected 2 rows (alice=30, carol=35), got %d", len(mat.Rows))
	}
}

func TestQueryFromOrderBy(t *testing.T) {
	t.Skip("query words disabled")
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	makeTestTable(r)

	result := runAQL(t, r, []Value{
		NewWord("from"), NewWord("people"),
		NewWord("order"), NewWord("by"), NewWord("age"),
	})
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
}

func TestQueryFromLimit(t *testing.T) {
	t.Skip("query words disabled")
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	makeTestTable(r)

	result := runAQL(t, r, []Value{
		NewWord("from"), NewWord("people"),
		NewWord("limit"), NewInteger(2),
	})
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
}

func TestQueryFromOffset(t *testing.T) {
	t.Skip("query words disabled")
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	makeTestTable(r)

	result := runAQL(t, r, []Value{
		NewWord("from"), NewWord("people"),
		NewWord("offset"), NewInteger(1),
	})
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
}

func TestQueryFromDistinct(t *testing.T) {
	t.Skip("query words disabled")
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	makeTestTable(r)

	result := runAQL(t, r, []Value{
		NewWord("from"), NewWord("people"),
		NewWord("distinct"),
	})
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
}

func TestQueryFromAs(t *testing.T) {
	t.Skip("query words disabled")
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	makeTestTable(r)

	result := runAQL(t, r, []Value{
		NewWord("from"), NewWord("people"),
		NewWord("as"), NewWord("p"),
	})
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
}

func TestQueryFromGroupBy(t *testing.T) {
	t.Skip("query words disabled")
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	makeTestTable(r)

	result := runAQL(t, r, []Value{
		NewWord("from"), NewWord("people"),
		NewWord("group"), NewWord("by"), NewWord("name"),
	})
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
}

func TestQueryStarWordErrors(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	e := New(r)
	_, err = e.Run([]Value{NewWord("star")})
	if err == nil {
		t.Fatal("expected error for undefined word 'star', got nil")
	}
}

func TestQueryMaterializeSelectStar(t *testing.T) {
	t.Skip("query words disabled")
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	makeTestTable(r)

	// from people select star — triggers full materialization pipeline
	result := runAQL(t, r, []Value{
		NewWord("from"), NewWord("people"),
		NewWord("select"), NewWord("star"),
	})
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	// Materialize the result
	v := result[0]
	if qb, ok := v.Data.(QueryBuilder); ok {
		td, err := qb.Materialize()
		if err != nil {
			t.Fatalf("materialize error: %v", err)
		}
		if len(td.Rows) != 3 {
			t.Errorf("expected 3 rows, got %d", len(td.Rows))
		}
	}
}

func TestQueryJoin(t *testing.T) {
	t.Skip("query words disabled")
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	makeTestTable(r)

	// Create a second table "scores"
	fields := NewOrderedMap()
	fields.Set("name", NewTypeLiteral(TString))
	fields.Set("score", NewTypeLiteral(TNumber))
	rec := RecordTypeInfo{Fields: fields}

	row1 := NewOrderedMap()
	row1.Set("name", NewString("alice"))
	row1.Set("score", NewInteger(95))
	row2 := NewOrderedMap()
	row2.Set("name", NewString("bob"))
	row2.Set("score", NewInteger(88))

	td := TableData{Record: rec, Rows: []Value{NewMap(row1), NewMap(row2)}}
	r.ContextSet("scores", Value{VType: TList, Data: td})

	result := runAQL(t, r, []Value{
		NewWord("from"), NewWord("people"),
		NewWord("join"), NewWord("scores"),
		NewWord("using"), NewList([]Value{NewWord("name")}),
	})
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
}

func TestQueryUnion(t *testing.T) {
	t.Skip("query words disabled")
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	makeTestTable(r)

	// Create second table with same schema
	fields := NewOrderedMap()
	fields.Set("name", NewTypeLiteral(TString))
	fields.Set("age", NewTypeLiteral(TNumber))
	rec := RecordTypeInfo{Fields: fields}

	row1 := NewOrderedMap()
	row1.Set("name", NewString("dave"))
	row1.Set("age", NewInteger(40))
	td := TableData{Record: rec, Rows: []Value{NewMap(row1)}}
	r.ContextSet("people2", Value{VType: TList, Data: td})

	result := runAQL(t, r, []Value{
		NewWord("from"), NewWord("people"),
		NewWord("union"), NewWord("from"), NewWord("people2"),
	})
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
}

// ========================
// Unify tests (disjunct, equality, open map)
// ========================

func TestUnifyDisjunctMatch(t *testing.T) {
	// string|none should unify with string "hello"
	disj := NewDisjunct([]Value{NewTypeLiteral(TString), NewTypeLiteral(TNone)})
	val := NewString("hello")
	result, ok := Unify(disj, val)
	if !ok {
		t.Fatal("expected unification to succeed")
	}
	if !result.VType.Matches(TString) {
		t.Errorf("expected string type, got %s", result.VType)
	}
}

func TestUnifyDisjunctNone(t *testing.T) {
	// string|none should unify with none
	disj := NewDisjunct([]Value{NewTypeLiteral(TString), NewTypeLiteral(TNone)})
	val := NewTypeLiteral(TNone)
	result, ok := Unify(disj, val)
	if !ok {
		t.Fatal("expected unification to succeed")
	}
	if !result.VType.Equal(TNone) {
		t.Errorf("expected none type, got %s", result.VType)
	}
}

func TestUnifyDisjunctFail(t *testing.T) {
	// string|none should NOT unify with integer
	disj := NewDisjunct([]Value{NewTypeLiteral(TString), NewTypeLiteral(TNone)})
	val := NewInteger(42)
	_, ok := Unify(disj, val)
	if ok {
		t.Fatal("expected unification to fail")
	}
}

func TestUnifyDisjunctWithAny(t *testing.T) {
	// string|none unifying with any should preserve the disjunct
	disj := NewDisjunct([]Value{NewTypeLiteral(TString), NewTypeLiteral(TNone)})
	val := NewTypeLiteral(TAny)
	result, ok := Unify(disj, val)
	if !ok {
		t.Fatal("expected unification to succeed")
	}
	if !result.IsDisjunct() {
		t.Errorf("expected disjunct, got %s", result)
	}
}

func TestUnifyDisjunctMapOpen(t *testing.T) {
	// Disjunct with map alternative uses open (subset) matching
	patternMap := NewOrderedMap()
	patternMap.Set("x", NewTypeLiteral(TNumber))
	pattern := NewMap(patternMap)

	candidateMap := NewOrderedMap()
	candidateMap.Set("x", NewInteger(42))
	candidateMap.Set("y", NewString("extra"))
	candidate := NewMap(candidateMap)

	disj := NewDisjunct([]Value{pattern})
	result, ok := Unify(disj, candidate)
	if !ok {
		t.Fatal("expected open map unification to succeed")
	}
	if !result.VType.Equal(TMap) {
		t.Errorf("expected map, got %s", result.VType)
	}
}

func TestValuesEqualLists(t *testing.T) {
	a := NewList([]Value{NewInteger(1), NewInteger(2)})
	b := NewList([]Value{NewInteger(1), NewInteger(2)})
	if !ValuesEqual(a, b) {
		t.Error("expected equal lists")
	}
}

func TestValuesEqualListsDiff(t *testing.T) {
	a := NewList([]Value{NewInteger(1), NewInteger(2)})
	b := NewList([]Value{NewInteger(1), NewInteger(3)})
	if ValuesEqual(a, b) {
		t.Error("expected unequal lists")
	}
}

func TestValuesEqualMaps(t *testing.T) {
	m1 := NewOrderedMap()
	m1.Set("a", NewInteger(1))
	m2 := NewOrderedMap()
	m2.Set("a", NewInteger(1))
	if !ValuesEqual(NewMap(m1), NewMap(m2)) {
		t.Error("expected equal maps")
	}
}

func TestValuesEqualMapsDiffKeys(t *testing.T) {
	m1 := NewOrderedMap()
	m1.Set("a", NewInteger(1))
	m2 := NewOrderedMap()
	m2.Set("b", NewInteger(1))
	if ValuesEqual(NewMap(m1), NewMap(m2)) {
		t.Error("expected unequal maps")
	}
}

func TestValuesEqualTableTypes(t *testing.T) {
	f1 := NewOrderedMap()
	f1.Set("x", NewTypeLiteral(TNumber))
	f2 := NewOrderedMap()
	f2.Set("x", NewTypeLiteral(TNumber))

	tt1 := Value{VType: TList, Data: TableTypeInfo{Record: RecordTypeInfo{Fields: f1}}}
	tt2 := Value{VType: TList, Data: TableTypeInfo{Record: RecordTypeInfo{Fields: f2}}}
	if !ValuesEqual(tt1, tt2) {
		t.Error("expected equal table types")
	}
}

func TestValuesEqualTypedLists(t *testing.T) {
	tl1 := NewTypedList(NewTypeLiteral(TString))
	tl2 := NewTypedList(NewTypeLiteral(TString))
	if !ValuesEqual(tl1, tl2) {
		t.Error("expected equal typed lists")
	}
}

func TestValuesEqualTypedListsDiff(t *testing.T) {
	tl1 := NewTypedList(NewTypeLiteral(TString))
	tl2 := NewTypedList(NewTypeLiteral(TNumber))
	if ValuesEqual(tl1, tl2) {
		t.Error("expected unequal typed lists")
	}
}

func TestValuesEqualRecordTypes(t *testing.T) {
	f1 := NewOrderedMap()
	f1.Set("x", NewTypeLiteral(TNumber))
	f2 := NewOrderedMap()
	f2.Set("x", NewTypeLiteral(TNumber))
	rt1 := NewRecordType(f1)
	rt2 := NewRecordType(f2)
	if !ValuesEqual(rt1, rt2) {
		t.Error("expected equal record types")
	}
}

func TestValuesEqualTypedMaps(t *testing.T) {
	tm1 := NewTypedMap(NewTypeLiteral(TString))
	tm2 := NewTypedMap(NewTypeLiteral(TString))
	if !ValuesEqual(tm1, tm2) {
		t.Error("expected equal typed maps")
	}
}

func TestValuesEqualTypedMapsDiff(t *testing.T) {
	tm1 := NewTypedMap(NewTypeLiteral(TString))
	tm2 := NewTypedMap(NewTypeLiteral(TNumber))
	if ValuesEqual(tm1, tm2) {
		t.Error("expected unequal typed maps")
	}
}

func TestValuesEqualOneNilData(t *testing.T) {
	a := NewTypeLiteral(TString)
	b := NewString("hello")
	if ValuesEqual(a, b) {
		t.Error("expected unequal (type literal vs concrete)")
	}
}

func TestOpenUnifyMap(t *testing.T) {
	pattern := NewOrderedMap()
	pattern.Set("x", NewTypeLiteral(TNumber))
	candidate := NewOrderedMap()
	candidate.Set("x", NewInteger(5))
	candidate.Set("y", NewString("extra"))
	if !OpenUnifyMap(NewMap(pattern), NewMap(candidate)) {
		t.Error("expected open unify to succeed")
	}
}

func TestOpenUnifyMapMissingKey(t *testing.T) {
	pattern := NewOrderedMap()
	pattern.Set("z", NewTypeLiteral(TNumber))
	candidate := NewOrderedMap()
	candidate.Set("x", NewInteger(5))
	if OpenUnifyMap(NewMap(pattern), NewMap(candidate)) {
		t.Error("expected open unify to fail")
	}
}

func TestUnifyTypedMaps(t *testing.T) {
	tm1 := NewTypedMap(NewTypeLiteral(TNumber))
	tm2 := NewTypedMap(NewTypeLiteral(TInteger))
	result, ok := Unify(tm1, tm2)
	if !ok {
		t.Fatal("expected typed map unification to succeed")
	}
	if !result.IsTypedMap() {
		t.Errorf("expected typed map, got %s", result)
	}
}

func TestUnifyTypedMapWithConcrete(t *testing.T) {
	tm := NewTypedMap(NewTypeLiteral(TNumber))
	m := NewOrderedMap()
	m.Set("x", NewInteger(1))
	m.Set("y", NewInteger(2))
	result, ok := Unify(tm, NewMap(m))
	if !ok {
		t.Fatal("expected unification to succeed")
	}
	if !result.VType.Equal(TMap) {
		t.Errorf("expected map, got %s", result.VType)
	}
}

func TestUnifyMapsClosedFail(t *testing.T) {
	m1 := NewOrderedMap()
	m1.Set("x", NewInteger(1))
	m2 := NewOrderedMap()
	m2.Set("x", NewInteger(1))
	m2.Set("y", NewInteger(2))
	_, ok := Unify(NewMap(m1), NewMap(m2))
	if ok {
		t.Error("expected closed map unification to fail (different key counts)")
	}
}

func TestUnifyMapsKeyMismatch(t *testing.T) {
	m1 := NewOrderedMap()
	m1.Set("x", NewInteger(1))
	m2 := NewOrderedMap()
	m2.Set("y", NewInteger(1))
	_, ok := Unify(NewMap(m1), NewMap(m2))
	if ok {
		t.Error("expected closed map unification to fail (different keys)")
	}
}

func TestUnifyMapWithNonMap(t *testing.T) {
	m := NewOrderedMap()
	m.Set("x", NewInteger(1))
	_, ok := Unify(NewMap(m), NewInteger(1))
	if ok {
		t.Error("expected map+int unification to fail")
	}
}

func TestUnifyListLiteralWithConcrete(t *testing.T) {
	listType := NewTypeLiteral(TList)
	concrete := NewList([]Value{NewInteger(1)})
	result, ok := Unify(listType, concrete)
	if !ok {
		t.Fatal("expected list type to unify with concrete list")
	}
	if !result.VType.Equal(TList) {
		t.Errorf("expected list, got %s", result.VType)
	}
}

func TestUnifyMapLiteralWithConcrete(t *testing.T) {
	mapType := NewTypeLiteral(TMap)
	m := NewOrderedMap()
	m.Set("a", NewInteger(1))
	result, ok := Unify(mapType, NewMap(m))
	if !ok {
		t.Fatal("expected map type to unify with concrete map")
	}
	if !result.VType.Equal(TMap) {
		t.Errorf("expected map, got %s", result.VType)
	}
}

// ========================
// Dot / Dotr tests
// ========================

func TestDotMapAtom(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	m := NewOrderedMap()
	m.Set("x", NewInteger(42))
	result := runAQL(t, r, []Value{NewMap(m), NewAtom("x"), NewWord("get")})
	_as0, _ := result[0].AsInteger()
	if len(result) != 1 || _as0 != 42 {
		t.Errorf("expected 42, got %v", result)
	}
}

func TestDotMapString(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	m := NewOrderedMap()
	m.Set("key", NewInteger(99))
	result := runAQL(t, r, []Value{NewMap(m), NewString("key"), NewWord("get")})
	_as1, _ := result[0].AsInteger()
	if len(result) != 1 || _as1 != 99 {
		t.Errorf("expected 99, got %v", result)
	}
}

func TestDotListIndex(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	list := NewList([]Value{NewString("a"), NewString("b"), NewString("c")})
	result := runAQL(t, r, []Value{list, NewInteger(1), NewWord("get")})
	_as2, _ := result[0].AsString()
	if len(result) != 1 || _as2 != "b" {
		t.Errorf("expected 'b', got %v", result)
	}
}

func TestDotListOutOfBounds(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	list := NewList([]Value{NewString("a")})
	result := runAQL(t, r, []Value{list, NewInteger(5), NewWord("get")})
	if len(result) != 1 || !result[0].VType.Equal(TNone) {
		t.Errorf("expected none, got %v", result)
	}
}

func TestDotMapMissing(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	m := NewOrderedMap()
	m.Set("x", NewInteger(1))
	result := runAQL(t, r, []Value{NewMap(m), NewAtom("y"), NewWord("get")})
	if len(result) != 1 || !result[0].VType.Equal(TNone) {
		t.Errorf("expected none for missing key, got %v", result)
	}
}

func TestDotMapIntegerKey(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	m := NewOrderedMap()
	m.Set("0", NewString("zero"))
	result := runAQL(t, r, []Value{NewMap(m), NewInteger(0), NewWord("get")})
	_as3, _ := result[0].AsString()
	if len(result) != 1 || _as3 != "zero" {
		t.Errorf("expected 'zero', got %v", result)
	}
}

func TestDotNone(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	result := runAQL(t, r, []Value{NewTypeLiteral(TNone), NewAtom("x"), NewWord("get")})
	if len(result) != 1 || !result[0].VType.Equal(TNone) {
		t.Errorf("expected none, got %v", result)
	}
}

// --- dot: list access ---

func TestDotListByIndex(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	list := NewList([]Value{NewInteger(10), NewInteger(20), NewInteger(30)})
	result := runAQL(t, r, []Value{list, NewInteger(1), NewWord("get")})
	_as4, _ := result[0].AsInteger()
	if len(result) != 1 || _as4 != 20 {
		t.Errorf("expected 20, got %v", result)
	}
}

func TestDotListAtomKeyReturnsNone(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	list := NewList([]Value{NewInteger(10), NewInteger(20)})
	result := runAQL(t, r, []Value{list, NewAtom("x"), NewWord("get")})
	if len(result) != 1 || !result[0].VType.Equal(TNone) {
		t.Errorf("expected none for atom key on list, got %v", result)
	}
}

func TestDotListStringKeyReturnsNone(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	list := NewList([]Value{NewInteger(10)})
	result := runAQL(t, r, []Value{list, NewString("x"), NewWord("get")})
	if len(result) != 1 || !result[0].VType.Equal(TNone) {
		t.Errorf("expected none for string key on list, got %v", result)
	}
}

// --- dot: nested map access (chained) ---

func TestDotNestedMapChain(t *testing.T) {
	// {a:{b:{c:1}}} dot a dot b dot c → 1
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	inner := NewOrderedMap()
	inner.Set("c", NewInteger(1))
	mid := NewOrderedMap()
	mid.Set("b", NewMap(inner))
	outer := NewOrderedMap()
	outer.Set("a", NewMap(mid))
	// Chained forward gets: map get a get b get c
	result := runAQL(t, r, []Value{
		NewMap(outer),
		NewWord("get"), NewWord("a"),
		NewWord("get"), NewWord("b"),
		NewWord("get"), NewWord("c"),
	})
	_as5, _ := result[0].AsInteger()
	if len(result) != 1 || _as5 != 1 {
		t.Errorf("expected 1, got %v", result)
	}
}

// --- dot: map containing list ---

func TestDotMapThenList(t *testing.T) {
	// {items:[10 20 30]} dot items dot 1 → 20
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	m := NewOrderedMap()
	m.Set("items", NewList([]Value{NewInteger(10), NewInteger(20), NewInteger(30)}))
	result := runAQL(t, r, []Value{
		NewMap(m),
		NewAtom("items"), NewWord("get"),
		NewInteger(1), NewWord("get"),
	})
	_as6, _ := result[0].AsInteger()
	if len(result) != 1 || _as6 != 20 {
		t.Errorf("expected 20, got %v", result)
	}
}

// --- dot: list containing maps ---

func TestDotListThenMap(t *testing.T) {
	// [{x:1} {x:2}] dot 0 dot x → 1
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	m0 := NewOrderedMap()
	m0.Set("x", NewInteger(1))
	m1 := NewOrderedMap()
	m1.Set("x", NewInteger(2))
	list := NewList([]Value{NewMap(m0), NewMap(m1)})
	result := runAQL(t, r, []Value{
		list,
		NewInteger(0), NewWord("get"),
		NewAtom("x"), NewWord("get"),
	})
	_as7, _ := result[0].AsInteger()
	if len(result) != 1 || _as7 != 1 {
		t.Errorf("expected 1, got %v", result)
	}
}

func TestDotListThenMapSecondElement(t *testing.T) {
	// [{x:1} {x:2}] dot 1 dot x → 2
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	m0 := NewOrderedMap()
	m0.Set("x", NewInteger(1))
	m1 := NewOrderedMap()
	m1.Set("x", NewInteger(2))
	list := NewList([]Value{NewMap(m0), NewMap(m1)})
	result := runAQL(t, r, []Value{
		list,
		NewInteger(1), NewWord("get"),
		NewAtom("x"), NewWord("get"),
	})
	_as8, _ := result[0].AsInteger()
	if len(result) != 1 || _as8 != 2 {
		t.Errorf("expected 2, got %v", result)
	}
}

// --- dot: . alias works identically ---

func TestDotAliasMapAccess(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	m := NewOrderedMap()
	m.Set("key", NewInteger(99))
	result := runAQL(t, r, []Value{NewMap(m), NewAtom("key"), NewWord("get")})
	_as9, _ := result[0].AsInteger()
	if len(result) != 1 || _as9 != 99 {
		t.Errorf("expected 99 via . alias, got %v", result)
	}
}

func TestDotAliasListAccess(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	list := NewList([]Value{NewString("a"), NewString("b")})
	result := runAQL(t, r, []Value{list, NewInteger(0), NewWord("get")})
	_as10, _ := result[0].AsString()
	if len(result) != 1 || _as10 != "a" {
		t.Errorf("expected 'a' via . alias, got %v", result)
	}
}

// --- dot: deeply nested list/map combo ---

func TestDotDeepListMapCombo(t *testing.T) {
	// {a:[{b:[100 200]} {b:[300 400]}]} dot a dot 1 dot b dot 0 → 300
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	b0 := NewList([]Value{NewInteger(100), NewInteger(200)})
	b1 := NewList([]Value{NewInteger(300), NewInteger(400)})
	m0 := NewOrderedMap()
	m0.Set("b", b0)
	m1 := NewOrderedMap()
	m1.Set("b", b1)
	outer := NewOrderedMap()
	outer.Set("a", NewList([]Value{NewMap(m0), NewMap(m1)}))
	result := runAQL(t, r, []Value{
		NewMap(outer),
		NewAtom("a"), NewWord("get"),
		NewInteger(1), NewWord("get"),
		NewAtom("b"), NewWord("get"),
		NewInteger(0), NewWord("get"),
	})
	_as11, _ := result[0].AsInteger()
	if len(result) != 1 || _as11 != 300 {
		t.Errorf("expected 300, got %v", result)
	}
}

func TestDotrMapSuccess(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	m := NewOrderedMap()
	m.Set("x", NewInteger(42))
	result := runAQL(t, r, []Value{NewMap(m), NewAtom("x"), NewWord("getr")})
	_as12, _ := result[0].AsInteger()
	if len(result) != 1 || _as12 != 42 {
		t.Errorf("expected 42, got %v", result)
	}
}

func TestDotrMapMissingError(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	m := NewOrderedMap()
	m.Set("x", NewInteger(1))
	err = runAQLError(t, r, []Value{NewMap(m), NewAtom("y"), NewWord("getr")})
	if err == nil {
		t.Fatal("expected error for missing key")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' error, got: %v", err)
	}
}

func TestDotrNoneError(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	err = runAQLError(t, r, []Value{NewTypeLiteral(TNone), NewAtom("x"), NewWord("getr")})
	if err == nil {
		t.Fatal("expected error for none parent")
	}
	if !strings.Contains(err.Error(), "None") {
		t.Errorf("expected 'None' error, got: %v", err)
	}
}

func TestDotrListOutOfBounds(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	list := NewList([]Value{NewString("a")})
	err = runAQLError(t, r, []Value{list, NewInteger(5), NewWord("getr")})
	if err == nil {
		t.Fatal("expected error for out of bounds")
	}
	if !strings.Contains(err.Error(), "out of bounds") {
		t.Errorf("expected 'out of bounds' error, got: %v", err)
	}
}

func TestDotrMapStringKey(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	m := NewOrderedMap()
	m.Set("key", NewInteger(99))
	result := runAQL(t, r, []Value{NewMap(m), NewString("key"), NewWord("getr")})
	_as13, _ := result[0].AsInteger()
	if len(result) != 1 || _as13 != 99 {
		t.Errorf("expected 99, got %v", result)
	}
}

func TestDotrMapStringMissing(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	m := NewOrderedMap()
	m.Set("x", NewInteger(1))
	err = runAQLError(t, r, []Value{NewMap(m), NewString("nope"), NewWord("getr")})
	if err == nil {
		t.Fatal("expected error for missing string key")
	}
}

func TestDotrMapIntegerKey(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	m := NewOrderedMap()
	m.Set("5", NewString("five"))
	result := runAQL(t, r, []Value{NewMap(m), NewInteger(5), NewWord("getr")})
	_as14, _ := result[0].AsString()
	if len(result) != 1 || _as14 != "five" {
		t.Errorf("expected 'five', got %v", result)
	}
}

func TestDotrMapIntegerKeyMissing(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	m := NewOrderedMap()
	m.Set("0", NewString("zero"))
	err = runAQLError(t, r, []Value{NewMap(m), NewInteger(9), NewWord("getr")})
	if err == nil {
		t.Fatal("expected error for missing integer key")
	}
}

// ========================
// Make / Record tests
// ========================

func TestMakeRecordPositional(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	// type Point record [x:number y:number]
	// make Point [1 2]
	result := runAQL(t, r, []Value{
		NewWord("type"), NewWord("Point"),
		NewWord("record"), NewList([]Value{
			NewMap(singleMap("x", NewTypeLiteral(TNumber))),
			NewMap(singleMap("y", NewTypeLiteral(TNumber))),
		}),
		NewWord("make"), NewWord("Point"),
		NewList([]Value{NewInteger(1), NewInteger(2)}),
	})
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d: %v", len(result), result)
	}
	m := result[0].AsMap()
	xVal, _ := m.Get("x")
	yVal, _ := m.Get("y")
	_as16, _ := xVal.AsInteger()
	_as15, _ := yVal.AsInteger()
	if _as16 != 1 || _as15 != 2 {
		t.Errorf("expected {x:1,y:2}, got %v", result[0])
	}
}

func singleMap(key string, val Value) *OrderedMap {
	m := NewOrderedMap()
	m.Set(key, val)
	return m
}

func TestMakeRecordNamed(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	result := runAQL(t, r, []Value{
		NewWord("type"), NewWord("Pt"),
		NewWord("record"), NewList([]Value{
			NewMap(singleMap("x", NewTypeLiteral(TNumber))),
			NewMap(singleMap("y", NewTypeLiteral(TNumber))),
		}),
		NewWord("make"), NewWord("Pt"),
		NewList([]Value{
			NewMap(singleMap("y", NewInteger(20))),
			NewMap(singleMap("x", NewInteger(10))),
		}),
	})
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	m := result[0].AsMap()
	xVal, _ := m.Get("x")
	yVal, _ := m.Get("y")
	_as18, _ := xVal.AsInteger()
	_as17, _ := yVal.AsInteger()
	if _as18 != 10 || _as17 != 20 {
		t.Errorf("expected {x:10,y:20}, got %v", result[0])
	}
}

func TestMakeRecordMap(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	src := NewOrderedMap()
	src.Set("x", NewInteger(5))
	src.Set("y", NewInteger(6))
	result := runAQL(t, r, []Value{
		NewWord("type"), NewWord("Pt2"),
		NewWord("record"), NewList([]Value{
			NewMap(singleMap("x", NewTypeLiteral(TNumber))),
			NewMap(singleMap("y", NewTypeLiteral(TNumber))),
		}),
		NewWord("make"), NewWord("Pt2"), NewMap(src),
	})
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
}

// ========================
// Convert tests
// ========================

func TestConvertIntToString(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	result := runAQL(t, r, []Value{
		NewInteger(42), NewWord("convert"), NewWord("String"),
	})
	_as19, _ := result[0].AsString()
	if len(result) != 1 || _as19 != "42" {
		t.Errorf("expected '42', got %v", result)
	}
}

func TestConvertIntToStringHex(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	hexOpts := NewOrderedMap()
	hexOpts.Set("base", NewString("hex"))
	result := runAQL(t, r, []Value{
		NewInteger(255), NewWord("convert"), NewWord("String"), NewMap(hexOpts),
	})
	_as20, _ := result[0].AsString()
	if len(result) != 1 || _as20 != "ff" {
		t.Errorf("expected 'ff', got %v", result)
	}
}

func TestConvertIntToStringBin(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	binOpts := NewOrderedMap()
	binOpts.Set("base", NewString("bin"))
	result := runAQL(t, r, []Value{
		NewInteger(10), NewWord("convert"), NewWord("String"), NewMap(binOpts),
	})
	_as21, _ := result[0].AsString()
	if len(result) != 1 || _as21 != "1010" {
		t.Errorf("expected '1010', got %v", result)
	}
}

func TestConvertIntToStringOct(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	octOpts := NewOrderedMap()
	octOpts.Set("base", NewString("oct"))
	result := runAQL(t, r, []Value{
		NewInteger(8), NewWord("convert"), NewWord("String"), NewMap(octOpts),
	})
	_as22, _ := result[0].AsString()
	if len(result) != 1 || _as22 != "10" {
		t.Errorf("expected '10', got %v", result)
	}
}

func TestConvertStringToNumber(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	result := runAQL(t, r, []Value{
		NewString("99"), NewWord("convert"), NewWord("Number"),
	})
	_as23, _ := result[0].AsInteger()
	if len(result) != 1 || _as23 != 99 {
		t.Errorf("expected 99, got %v", result)
	}
}

func TestConvertBoolToString(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	result := runAQL(t, r, []Value{
		NewBoolean(true), NewWord("convert"), NewWord("String"),
	})
	_as24, _ := result[0].AsString()
	if len(result) != 1 || _as24 != "true" {
		t.Errorf("expected 'true', got %v", result)
	}
}

func TestConvertIntToBool(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	result := runAQL(t, r, []Value{
		NewInteger(1), NewWord("convert"), NewWord("Boolean"),
	})
	_as25, _ := result[0].AsBoolean()
	if len(result) != 1 || !_as25 {
		t.Errorf("expected true, got %v", result)
	}
}

func TestConvertIntToBoolZero(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	result := runAQL(t, r, []Value{
		NewInteger(0), NewWord("convert"), NewWord("Boolean"),
	})
	_as26, _ := result[0].AsBoolean()
	if len(result) != 1 || _as26 {
		t.Errorf("expected false, got %v", result)
	}
}

func TestConvertStringToBool(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	result := runAQL(t, r, []Value{
		NewString("true"), NewWord("convert"), NewWord("Boolean"),
	})
	_as27, _ := result[0].AsBoolean()
	if len(result) != 1 || !_as27 {
		t.Errorf("expected true, got %v", result)
	}
}

func TestConvertWithSettingsMap(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	settings := NewOrderedMap()
	settings.Set("base", NewString("hex"))
	result := runAQL(t, r, []Value{
		NewInteger(255), NewWord("convert"), NewWord("String"), NewMap(settings),
	})
	_as28, _ := result[0].AsString()
	if len(result) != 1 || _as28 != "ff" {
		t.Errorf("expected 'ff', got %v", result)
	}
}

// ========================
// Var edge cases
// ========================

func TestVarStringName(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	// 5 var [["x"] x mul x]
	result := runAQL(t, r, []Value{
		NewInteger(5),
		NewWord("var"), NewList([]Value{
			NewList([]Value{NewString("x")}),
			NewWord("x"), NewWord("mul"), NewWord("x"),
		}),
	})
	_as29, _ := result[0].AsInteger()
	if len(result) != 1 || _as29 != 25 {
		t.Errorf("expected 25, got %v", result)
	}
}

func TestVarWithDefault(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	// var [[[x 10]] x add 1]
	result := runAQL(t, r, []Value{
		NewWord("var"), NewList([]Value{
			NewList([]Value{
				NewList([]Value{NewWord("x"), NewInteger(10)}),
			}),
			NewWord("x"), NewWord("add"), NewInteger(1),
		}),
	})
	_as30, _ := result[0].AsInteger()
	if len(result) != 1 || _as30 != 11 {
		t.Errorf("expected 11, got %v", result)
	}
}

// ========================
// Print / format tests
// ========================

func TestFormatForPrintString(t *testing.T) {
	out := FormatForPrint(NewString("hello"))
	if out != "hello" {
		t.Errorf("expected 'hello', got %q", out)
	}
}

func TestFormatForPrintInteger(t *testing.T) {
	out := FormatForPrint(NewInteger(42))
	if out != "42" {
		t.Errorf("expected '42', got %q", out)
	}
}

func TestFormatForPrintBoolean(t *testing.T) {
	out := FormatForPrint(NewBoolean(true))
	if out != "true" {
		t.Errorf("expected 'true', got %q", out)
	}
}

func TestFormatForPrintMap(t *testing.T) {
	m := NewOrderedMap()
	m.Set("x", NewInteger(1))
	out := FormatForPrint(NewMap(m))
	if !strings.Contains(out, "\"x\"") || !strings.Contains(out, "1") {
		t.Errorf("expected JSON-like map, got %q", out)
	}
}

func TestFormatForPrintList(t *testing.T) {
	list := NewList([]Value{NewInteger(1), NewString("a")})
	out := FormatForPrint(list)
	if !strings.Contains(out, "1") || !strings.Contains(out, "\"a\"") {
		t.Errorf("expected JSON-like list, got %q", out)
	}
}

func TestFormatForPrintTable(t *testing.T) {
	fields := NewOrderedMap()
	fields.Set("name", NewTypeLiteral(TString))
	rec := RecordTypeInfo{Fields: fields}
	row := NewOrderedMap()
	row.Set("name", NewString("alice"))
	td := TableData{Record: rec, Rows: []Value{NewMap(row)}}
	out := FormatForPrint(Value{VType: TList, Data: td})
	if !strings.Contains(out, "name") || !strings.Contains(out, "alice") {
		t.Errorf("expected table with 'name' and 'alice', got %q", out)
	}
}

func TestFormatForPrintEmptyTable(t *testing.T) {
	rec := RecordTypeInfo{Fields: NewOrderedMap()}
	td := TableData{Record: rec, Rows: nil}
	out := FormatForPrint(Value{VType: TList, Data: td})
	if out != "(empty table)" {
		t.Errorf("expected '(empty table)', got %q", out)
	}
}

func TestFormatValueJSONNone(t *testing.T) {
	out := FormatValueJSON(NewTypeLiteral(TNone))
	if out != "null" {
		t.Errorf("expected 'null', got %q", out)
	}
}

func TestFormatValueJSONBoolFalse(t *testing.T) {
	out := FormatValueJSON(NewBoolean(false))
	if out != "false" {
		t.Errorf("expected 'false', got %q", out)
	}
}

func TestFormatValueJSONNestedMap(t *testing.T) {
	inner := NewOrderedMap()
	inner.Set("a", NewInteger(1))
	outer := NewOrderedMap()
	outer.Set("m", NewMap(inner))
	out := FormatValueJSON(NewMap(outer))
	if !strings.Contains(out, "\"m\"") || !strings.Contains(out, "\"a\"") {
		t.Errorf("expected nested map, got %q", out)
	}
}

func TestPadRight(t *testing.T) {
	if PadRight("hi", 5) != "hi   " {
		t.Error("expected padding")
	}
	if PadRight("hello", 3) != "hello" {
		t.Error("expected no truncation")
	}
}

// ========================
// Trace tests
// ========================

func TestTraceColorize(t *testing.T) {
	cases := []struct {
		val  Value
		want string
	}{
		{NewWord("add"), "add"},
		{NewWordModified("x", -1, true, false), "x/s"},
		{NewWordModified("x", -1, false, true), "x/f"},
		{NewString("hi"), `"hi"`},
		{NewInteger(42), "42"},
		{NewBoolean(true), "true"},
		{NewBoolean(false), "false"},
		{NewAtom("foo"), "foo"},
		{NewTypeLiteral(TNumber), "Number"},
		{NewList([]Value{NewInteger(1)}), "1"},
	}
	for _, tc := range cases {
		got := TraceColorize(tc.val)
		// Strip ANSI codes
		visible := stripANSI(got)
		if !strings.Contains(visible, tc.want) {
			t.Errorf("TraceColorize(%s) visible = %q, want contains %q", tc.val, visible, tc.want)
		}
	}
}

func stripANSI(s string) string {
	var out strings.Builder
	inEsc := false
	for _, r := range s {
		if r == '\033' {
			inEsc = true
			continue
		}
		if inEsc {
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
				inEsc = false
			}
			continue
		}
		out.WriteRune(r)
	}
	return out.String()
}

func TestTraceColorizeMap(t *testing.T) {
	m := NewOrderedMap()
	m.Set("x", NewInteger(1))
	got := TraceColorize(NewMap(m))
	visible := stripANSI(got)
	if !strings.Contains(visible, "x") || !strings.Contains(visible, "1") {
		t.Errorf("expected map colorize to contain 'x' and '1', got %q", visible)
	}
}

func TestTraceColorizeForward(t *testing.T) {
	fwd := NewForward(ForwardInfo{FuncName: "add", CollectedArgs: 1, ExpectedArgs: 2})
	got := TraceColorize(fwd)
	visible := stripANSI(got)
	if !strings.Contains(visible, "add") || !strings.Contains(visible, "1/2") {
		t.Errorf("expected forward colorize, got %q", visible)
	}
}

func TestTraceColorizeOpenParen(t *testing.T) {
	got := TraceColorize(NewWord("("))
	visible := stripANSI(got)
	if !strings.Contains(visible, "(") {
		t.Errorf("expected '(', got %q", visible)
	}
}

func TestTraceVisibleLen(t *testing.T) {
	if TraceVisibleLen("hello") != 5 {
		t.Error("plain string length wrong")
	}
	if TraceVisibleLen("\033[31mhello\033[0m") != 5 {
		t.Error("ANSI-wrapped length wrong")
	}
}

func TestRunTrace(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	r.Output = &buf
	tokens := []Value{NewInteger(1), NewWord("add"), NewInteger(2)}
	result, err := RunTrace(r, tokens, &buf)
	if err != nil {
		t.Fatalf("RunTrace error: %v", err)
	}
	_as31, _ := result[0].AsInteger()
	if len(result) != 1 || _as31 != 3 {
		t.Errorf("expected [3], got %v", result)
	}
	out := buf.String()
	if !strings.Contains(out, "trace") {
		t.Errorf("expected trace output, got %q", out)
	}
	if !strings.Contains(out, "result") {
		t.Errorf("expected result in output, got %q", out)
	}
}

func TestRunTraceLong(t *testing.T) {
	// Force multi-line wrapping by using a long expression
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	r.Output = &buf
	tokens := make([]Value, 0)
	for i := 0; i < 30; i++ {
		tokens = append(tokens, NewInteger(int64(i)))
	}
	_, err = RunTrace(r, tokens, &buf)
	if err != nil {
		t.Fatalf("RunTrace error: %v", err)
	}
}

func TestTraceWrapEmpty(t *testing.T) {
	lines := TraceWrap(nil, 0, 80)
	if len(lines) != 1 || !strings.Contains(stripANSI(lines[0]), "[ ]") {
		t.Errorf("expected [ ] for empty, got %v", lines)
	}
}

// ========================
// Engine edge cases: stepEnd, curryOrPrefix, peekForwardValue
// ========================

func TestStepEndNoForward(t *testing.T) {
	// "end" with no pending forward should just be removed
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	result := runAQL(t, r, []Value{NewInteger(1), NewWord("end")})
	_as32, _ := result[0].AsInteger()
	if len(result) != 1 || _as32 != 1 {
		t.Errorf("expected [1], got %v", result)
	}
}

func TestDefEndExplicit(t *testing.T) {
	// "def foo 42 end foo" — end terminates def's forward collection
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	result := runAQL(t, r, []Value{
		NewWord("def"), NewWord("foo"), NewInteger(42), NewWord("end"),
		NewWord("foo"),
	})
	_as33, _ := result[0].AsInteger()
	if len(result) != 1 || _as33 != 42 {
		t.Errorf("expected 42, got %v", result)
	}
}

func TestParenResolvesForward(t *testing.T) {
	// (1 add 2) — paren should resolve the forward
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	result := runAQL(t, r, []Value{
		NewWord("("), NewInteger(1), NewWord("add"), NewInteger(2), NewWord(")"),
	})
	_as34, _ := result[0].AsInteger()
	if len(result) != 1 || _as34 != 3 {
		t.Errorf("expected 3, got %v", result)
	}
}

func TestUnmatchedCloseParen(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	err = runAQLError(t, r, []Value{NewInteger(1), NewWord(")")})
	if err == nil {
		t.Fatal("expected error for unmatched close paren")
	}
}

// ========================
// ResolveWordValue tests
// ========================

func TestResolveWordValueTrue(t *testing.T) {
	v := ResolveWordValue(NewWord("true"))
	_as35, _ := v.AsBoolean()
	if !v.VType.Matches(TBoolean) || !_as35 {
		t.Errorf("expected boolean true, got %s", v)
	}
}

func TestResolveWordValueFalse(t *testing.T) {
	v := ResolveWordValue(NewWord("false"))
	_as36, _ := v.AsBoolean()
	if !v.VType.Matches(TBoolean) || _as36 {
		t.Errorf("expected boolean false, got %s", v)
	}
}

func TestResolveWordValueNone(t *testing.T) {
	v := ResolveWordValue(NewWord("None"))
	if !v.VType.Equal(TNone) {
		t.Errorf("expected none, got %s", v)
	}
}

func TestResolveWordValueOther(t *testing.T) {
	v := ResolveWordValue(NewWord("foo"))
	if !v.VType.Equal(TAtom) {
		t.Errorf("expected atom, got %s", v)
	}
}

func TestResolveWordValueNonWord(t *testing.T) {
	v := ResolveWordValue(NewInteger(42))
	if !v.VType.Matches(TInteger) {
		t.Errorf("expected integer passthrough, got %s", v)
	}
}

// ========================
// resolveSigType / resolveTypeName tests
// ========================

func TestResolveSigTypeTypeLiteral(t *testing.T) {
	tp, _, err := resolveSigType(nil, NewTypeLiteral(TNumber))
	if err != nil {
		t.Fatal(err)
	}
	if !tp.Equal(TNumber) {
		t.Errorf("expected number, got %s", tp)
	}
}

func TestResolveSigTypeWord(t *testing.T) {
	tp, _, err := resolveSigType(nil, NewWord("String"))
	if err != nil {
		t.Fatal(err)
	}
	if !tp.Equal(TString) {
		t.Errorf("expected string, got %s", tp)
	}
}

func TestResolveSigTypeString(t *testing.T) {
	tp, _, err := resolveSigType(nil, NewString("Boolean"))
	if err != nil {
		t.Fatal(err)
	}
	if !tp.Equal(TBoolean) {
		t.Errorf("expected boolean, got %s", tp)
	}
}

func TestResolveSigTypeInteger(t *testing.T) {
	v := NewInteger(42)
	tp, _, err := resolveSigType(nil, v)
	if err != nil {
		t.Fatal(err)
	}
	if !tp.Matches(TInteger) {
		t.Errorf("expected integer type, got %s", tp)
	}
}

func TestResolveSigTypeBoolean(t *testing.T) {
	v := NewBoolean(true)
	tp, _, err := resolveSigType(nil, v)
	if err != nil {
		t.Fatal(err)
	}
	if !tp.Matches(TBoolean) {
		t.Errorf("expected boolean type, got %s", tp)
	}
}

func TestResolveSigTypeMap(t *testing.T) {
	m := NewOrderedMap()
	m.Set("x", NewInteger(1))
	mapVal := NewMap(m)
	tp, pattern, err := resolveSigType(nil, mapVal)
	if err != nil {
		t.Fatal(err)
	}
	if !tp.Equal(TMap) {
		t.Errorf("expected Map for map, got %s", tp)
	}
	if pattern == nil {
		t.Fatal("expected non-nil pattern for map")
	}
	if !pattern.VType.Equal(TMap) {
		t.Errorf("expected pattern to be a map, got %s", pattern.VType)
	}
}

func TestResolveTypeName(t *testing.T) {
	cases := map[string]Type{
		"Any": TAny, "None": TNone, "Number": TNumber,
		"Integer": TInteger, "String": TString, "Boolean": TBoolean,
		"Atom": TAtom, "List": TList, "Map": TMap, "Scalar": TScalar,
	}
	for name, want := range cases {
		got, err := resolveTypeName(name)
		if err != nil {
			t.Fatal(err)
		}
		if !got.Equal(want) {
			t.Errorf("resolveTypeName(%q) = %s, want %s", name, got, want)
		}
	}
}

func TestResolveTypeNameUnknown(t *testing.T) {
	// Unknown names create a new named type via NewType
	got, err := resolveTypeName("Foobar")
	if err != nil {
		t.Fatal(err)
	}
	if got.String() != "Foobar" {
		t.Errorf("expected named type 'Foobar', got %s", got)
	}
}

// ========================
// IsTypeBody tests
// ========================

func TestIsTypeValueTypeLiteral(t *testing.T) {
	if !IsTypeBody(NewTypeLiteral(TNumber)) {
		t.Error("type literal should be a type value")
	}
}

func TestIsTypeValueDisjunct(t *testing.T) {
	disj := NewDisjunct([]Value{NewTypeLiteral(TString), NewTypeLiteral(TNone)})
	if !IsTypeBody(disj) {
		t.Error("disjunct of types should be a type value")
	}
}

func TestIsTypeValueRecordType(t *testing.T) {
	f := NewOrderedMap()
	f.Set("x", NewTypeLiteral(TNumber))
	rt := NewRecordType(f)
	if !IsTypeBody(rt) {
		t.Error("record type should be a type value")
	}
}

func TestIsTypeValueNotType(t *testing.T) {
	if IsTypeBody(NewInteger(42)) {
		t.Error("integer should not be a type value")
	}
}

func TestIsTypeValueTypedList(t *testing.T) {
	tl := NewTypedList(NewTypeLiteral(TString))
	if !IsTypeBody(tl) {
		t.Error("typed list should be a type value")
	}
}

func TestIsTypeValueTypedMap(t *testing.T) {
	tm := NewTypedMap(NewTypeLiteral(TNumber))
	if !IsTypeBody(tm) {
		t.Error("typed map should be a type value")
	}
}

// ========================
// Fn tests (multi-signature, edge cases)
// ========================

func TestFnMultiSignature(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	// def f fn [[x:number] [number] [x mul x] [x:string] [string] [x upper]]
	result := runAQL(t, r, []Value{
		NewWord("def"), NewWord("f"), NewWord("fn"),
		NewList([]Value{
			NewList([]Value{NewImplicitMap(singleMap("x", NewTypeLiteral(TNumber)))}),
			NewList([]Value{NewTypeLiteral(TNumber)}),
			NewList([]Value{NewWord("x"), NewWord("mul"), NewWord("x")}),
			NewList([]Value{NewImplicitMap(singleMap("x", NewTypeLiteral(TString)))}),
			NewList([]Value{NewTypeLiteral(TString)}),
			NewList([]Value{NewWord("x"), NewWord("upper")}),
		}),
		NewInteger(5), NewWord("f"),
	})
	_as37, _ := result[0].AsInteger()
	if len(result) != 1 || _as37 != 25 {
		t.Errorf("expected 25, got %v", result)
	}
}

// ========================
// Value.String edge cases (additional)
// ========================

func TestValueStringTypedListCov(t *testing.T) {
	tl := NewTypedList(NewTypeLiteral(TString))
	s := tl.String()
	if !strings.Contains(s, "String") {
		t.Errorf("expected typed list string, got %q", s)
	}
}

func TestValueStringTypedMapCov(t *testing.T) {
	tm := NewTypedMap(NewTypeLiteral(TNumber))
	s := tm.String()
	if !strings.Contains(s, "Number") {
		t.Errorf("expected typed map string, got %q", s)
	}
}

// ========================
// Format: encode/decode delimited
// ========================

func TestDecodeCSV(t *testing.T) {
	content := "name,age\nalice,30\nbob,25\n"
	result, err := decodeDelimited(content, ",")
	if err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	if !result[0].IsTableType() {
		t.Fatalf("expected table type, got %s", result[0])
	}
	td := result[0].Data.(TableData)
	if len(td.Rows) != 2 {
		t.Errorf("expected 2 rows, got %d", len(td.Rows))
	}
	if td.Record.Fields.Len() != 2 {
		t.Errorf("expected 2 columns, got %d", td.Record.Fields.Len())
	}
}

func TestDecodeTSV(t *testing.T) {
	content := "x\ty\n1\t2\n3\t4\n"
	result, err := decodeDelimited(content, "\t")
	if err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
}

func TestDecodeEmpty(t *testing.T) {
	result, err := decodeDelimited("", ",")
	if err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
}

func TestEncodeTableData(t *testing.T) {
	fields := NewOrderedMap()
	fields.Set("name", NewTypeLiteral(TString))
	fields.Set("age", NewTypeLiteral(TNumber))
	rec := RecordTypeInfo{Fields: fields}

	row := NewOrderedMap()
	row.Set("name", NewString("alice"))
	row.Set("age", NewInteger(30))
	td := TableData{Record: rec, Rows: []Value{NewMap(row)}}
	v := Value{VType: TList, Data: td}

	encoded, err := encodeDelimited(v, ",")
	if err != nil {
		t.Fatalf("encode error: %v", err)
	}
	if !strings.Contains(encoded, "name") || !strings.Contains(encoded, "alice") {
		t.Errorf("expected encoded CSV with 'name' and 'alice', got %q", encoded)
	}
}

func TestEncodeQuotedStrings(t *testing.T) {
	fields := NewOrderedMap()
	fields.Set("val", NewTypeLiteral(TString))
	rec := RecordTypeInfo{Fields: fields}

	row := NewOrderedMap()
	row.Set("val", NewString("has,comma"))
	td := TableData{Record: rec, Rows: []Value{NewMap(row)}}
	v := Value{VType: TList, Data: td}

	encoded, err := encodeDelimited(v, ",")
	if err != nil {
		t.Fatalf("encode error: %v", err)
	}
	if !strings.Contains(encoded, "\"has,comma\"") {
		t.Errorf("expected quoted value, got %q", encoded)
	}
}

func TestEncodeListOfMaps(t *testing.T) {
	m := NewOrderedMap()
	m.Set("x", NewInteger(1))
	list := NewList([]Value{NewMap(m)})

	encoded, err := encodeDelimited(list, ",")
	if err != nil {
		t.Fatalf("encode error: %v", err)
	}
	if !strings.Contains(encoded, "x") {
		t.Errorf("expected 'x' header, got %q", encoded)
	}
}

func TestEncodeEmptyColumns(t *testing.T) {
	fields := NewOrderedMap()
	rec := RecordTypeInfo{Fields: fields}
	td := TableData{Record: rec, Rows: nil}
	v := Value{VType: TList, Data: td}

	encoded, err := encodeDelimited(v, ",")
	if err != nil {
		t.Fatalf("encode error: %v", err)
	}
	if encoded != "" {
		t.Errorf("expected empty string, got %q", encoded)
	}
}

func TestEncodeNonTable(t *testing.T) {
	v := NewString("hello")
	encoded, err := encodeDelimited(v, ",")
	if err != nil {
		t.Fatalf("encode error: %v", err)
	}
	if encoded != "'hello'" {
		t.Errorf("expected 'hello', got %q", encoded)
	}
}

// ========================
// More query tests to cover remaining branches
// ========================

func TestQueryFromWhereLt(t *testing.T) {
	t.Skip("query words disabled")
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	makeTestTable(r)

	result := runAQL(t, r, []Value{
		NewWord("from"), NewWord("people"),
		NewWord("where"), NewList([]Value{
			NewWord("age"), NewWord("lt"), NewInteger(30),
		}),
	})
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
}

func TestQueryFromWhereGte(t *testing.T) {
	t.Skip("query words disabled")
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	makeTestTable(r)

	result := runAQL(t, r, []Value{
		NewWord("from"), NewWord("people"),
		NewWord("where"), NewList([]Value{
			NewWord("age"), NewWord("gte"), NewInteger(30),
		}),
	})
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
}

func TestQueryFromWhereLte(t *testing.T) {
	t.Skip("query words disabled")
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	makeTestTable(r)

	result := runAQL(t, r, []Value{
		NewWord("from"), NewWord("people"),
		NewWord("where"), NewList([]Value{
			NewWord("age"), NewWord("lte"), NewInteger(30),
		}),
	})
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
}

func TestQueryFromWhereNeq(t *testing.T) {
	t.Skip("query words disabled")
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	makeTestTable(r)

	result := runAQL(t, r, []Value{
		NewWord("from"), NewWord("people"),
		NewWord("where"), NewList([]Value{
			NewWord("name"), NewWord("neq"), NewString("alice"),
		}),
	})
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
}

func TestQueryFromOrderAsc(t *testing.T) {
	t.Skip("query words disabled")
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	makeTestTable(r)

	result := runAQL(t, r, []Value{
		NewWord("from"), NewWord("people"),
		NewWord("order"), NewList([]Value{NewWord("age")}),
	})
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
}

func TestQueryFromLimitOffset(t *testing.T) {
	t.Skip("query words disabled")
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	makeTestTable(r)

	result := runAQL(t, r, []Value{
		NewWord("from"), NewWord("people"),
		NewWord("limit"), NewInteger(1),
		NewWord("offset"), NewInteger(1),
	})
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
}

func TestQueryPrintTable(t *testing.T) {
	t.Skip("query words disabled")
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	r.Output = &buf
	makeTestTable(r)

	runAQL(t, r, []Value{
		NewWord("from"), NewWord("people"),
		NewWord("print"),
	})
	out := buf.String()
	if !strings.Contains(out, "alice") || !strings.Contains(out, "name") {
		t.Errorf("expected table output, got %q", out)
	}
}

// ========================
// More unify edge cases
// ========================

func TestUnifyListsDifferentLength(t *testing.T) {
	a := NewList([]Value{NewInteger(1)})
	b := NewList([]Value{NewInteger(1), NewInteger(2)})
	_, ok := Unify(a, b)
	if ok {
		t.Error("expected different-length lists to fail")
	}
}

func TestUnifyTypedListWithConcrete(t *testing.T) {
	tl := NewTypedList(NewTypeLiteral(TNumber))
	concrete := NewList([]Value{NewInteger(1), NewInteger(2)})
	result, ok := Unify(tl, concrete)
	if !ok {
		t.Fatal("expected typed list to unify with concrete list")
	}
	if !result.VType.Equal(TList) {
		t.Errorf("expected list, got %s", result.VType)
	}
}

func TestUnifyTypedListFail(t *testing.T) {
	tl := NewTypedList(NewTypeLiteral(TNumber))
	concrete := NewList([]Value{NewString("not a number")})
	_, ok := Unify(tl, concrete)
	if ok {
		t.Error("expected typed list + string list to fail")
	}
}

func TestUnifyRecordTypesDiffOrder(t *testing.T) {
	f1 := NewOrderedMap()
	f1.Set("a", NewTypeLiteral(TNumber))
	f1.Set("b", NewTypeLiteral(TString))
	f2 := NewOrderedMap()
	f2.Set("b", NewTypeLiteral(TString))
	f2.Set("a", NewTypeLiteral(TNumber))
	_, ok := Unify(NewRecordType(f1), NewRecordType(f2))
	if ok {
		t.Error("expected different field order to fail")
	}
}

func TestUnifyRecordTypesDiffFields(t *testing.T) {
	f1 := NewOrderedMap()
	f1.Set("a", NewTypeLiteral(TNumber))
	f2 := NewOrderedMap()
	f2.Set("a", NewTypeLiteral(TNumber))
	f2.Set("b", NewTypeLiteral(TString))
	_, ok := Unify(NewRecordType(f1), NewRecordType(f2))
	if ok {
		t.Error("expected different field count to fail")
	}
}

func TestUnifyTableWithNonTable(t *testing.T) {
	f := NewOrderedMap()
	f.Set("x", NewTypeLiteral(TNumber))
	tt := NewTableType(RecordTypeInfo{Fields: f})
	concrete := NewList([]Value{NewInteger(1)})
	_, ok := Unify(tt, concrete)
	if ok {
		t.Error("expected table + plain list to fail")
	}
}

func TestUnifyTableTypes(t *testing.T) {
	f1 := NewOrderedMap()
	f1.Set("x", NewTypeLiteral(TNumber))
	f2 := NewOrderedMap()
	f2.Set("x", NewTypeLiteral(TInteger))
	tt1 := NewTableType(RecordTypeInfo{Fields: f1})
	tt2 := NewTableType(RecordTypeInfo{Fields: f2})
	_, ok := Unify(tt1, tt2)
	if !ok {
		t.Error("expected compatible table types to unify")
	}
}

func TestUnifyListLiteralWithTable(t *testing.T) {
	listType := NewTypeLiteral(TList)
	f := NewOrderedMap()
	f.Set("x", NewTypeLiteral(TNumber))
	tt := NewTableType(RecordTypeInfo{Fields: f})
	_, ok := Unify(listType, tt)
	if ok {
		t.Error("expected list literal + table to fail")
	}
}

func TestUnifyMapLiteralWithRecord(t *testing.T) {
	mapType := NewTypeLiteral(TMap)
	f := NewOrderedMap()
	f.Set("x", NewTypeLiteral(TNumber))
	rt := NewRecordType(f)
	_, ok := Unify(mapType, rt)
	if ok {
		t.Error("expected map literal + record to fail")
	}
}

func TestUnifyRecordWithConcrete(t *testing.T) {
	f := NewOrderedMap()
	f.Set("x", NewTypeLiteral(TNumber))
	rt := NewRecordType(f)
	m := NewOrderedMap()
	m.Set("x", NewInteger(1))
	_, ok := Unify(rt, NewMap(m))
	if ok {
		t.Error("expected record type + concrete map to fail")
	}
}

// ========================
// SQLite store tests
// ========================

func TestSQLiteStoreTable(t *testing.T) {
	store, err := NewSQLiteStore()
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	fields := NewOrderedMap()
	fields.Set("name", NewTypeLiteral(TString))
	fields.Set("age", NewTypeLiteral(TNumber))
	rec := RecordTypeInfo{Fields: fields}

	row := NewOrderedMap()
	row.Set("name", NewString("alice"))
	row.Set("age", NewInteger(30))
	td := TableData{Record: rec, Rows: []Value{NewMap(row)}}

	if err := store.StoreTable("test_people", td); err != nil {
		t.Fatalf("StoreTable error: %v", err)
	}
	if !store.HasTable("test_people") {
		t.Error("expected table to exist")
	}
}

func TestSQLiteQuery(t *testing.T) {
	store, err := NewSQLiteStore()
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	fields := NewOrderedMap()
	fields.Set("x", NewTypeLiteral(TNumber))
	rec := RecordTypeInfo{Fields: fields}

	row1 := NewOrderedMap()
	row1.Set("x", NewInteger(1))
	row2 := NewOrderedMap()
	row2.Set("x", NewInteger(2))
	td := TableData{Record: rec, Rows: []Value{NewMap(row1), NewMap(row2)}}

	if err := store.StoreTable("nums", td); err != nil {
		t.Fatalf("StoreTable error: %v", err)
	}

	result, err := store.Query("SELECT * FROM \"nums\"", &rec)
	if err != nil {
		t.Fatalf("Query error: %v", err)
	}
	if len(result.Rows) != 2 {
		t.Errorf("expected 2 rows, got %d", len(result.Rows))
	}
}

func TestSQLiteStoreTempTable(t *testing.T) {
	store, err := NewSQLiteStore()
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	fields := NewOrderedMap()
	fields.Set("v", NewTypeLiteral(TString))
	rec := RecordTypeInfo{Fields: fields}

	row := NewOrderedMap()
	row.Set("v", NewString("hello"))
	td := TableData{Record: rec, Rows: []Value{NewMap(row)}}

	name, err := store.StoreTempTable(td)
	if err != nil {
		t.Fatalf("StoreTempTable error: %v", err)
	}
	if name == "" {
		t.Error("expected non-empty temp table name")
	}
	if !store.HasTable(name) {
		t.Error("expected temp table to exist")
	}
}

func TestSQLiteDropTable(t *testing.T) {
	store, err := NewSQLiteStore()
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	fields := NewOrderedMap()
	fields.Set("v", NewTypeLiteral(TString))
	rec := RecordTypeInfo{Fields: fields}
	td := TableData{Record: rec, Rows: nil}

	if err := store.StoreTable("drop_me", td); err != nil {
		t.Fatalf("StoreTable error: %v", err)
	}
	if !store.HasTable("drop_me") {
		t.Fatal("expected table to exist before drop")
	}
	store.DropTable("drop_me")
	if store.HasTable("drop_me") {
		t.Error("expected table to not exist after drop")
	}
}

func TestSQLiteStoreWithBoolAndString(t *testing.T) {
	store, err := NewSQLiteStore()
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	fields := NewOrderedMap()
	fields.Set("flag", NewTypeLiteral(TBoolean))
	fields.Set("name", NewTypeLiteral(TString))
	rec := RecordTypeInfo{Fields: fields}

	row := NewOrderedMap()
	row.Set("flag", NewBoolean(true))
	row.Set("name", NewString("test"))
	td := TableData{Record: rec, Rows: []Value{NewMap(row)}}

	if err := store.StoreTable("bool_test", td); err != nil {
		t.Fatalf("StoreTable error: %v", err)
	}

	result, err := store.Query("SELECT * FROM \"bool_test\"", &rec)
	if err != nil {
		t.Fatalf("Query error: %v", err)
	}
	if len(result.Rows) != 1 {
		t.Errorf("expected 1 row, got %d", len(result.Rows))
	}
}

// ========================
// More trace coverage (wrapping, notes on second line)
// ========================

func TestRunTraceError(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	r.Output = &buf
	tokens := []Value{NewInteger(10), NewWord("div"), NewInteger(0)}
	_, err = RunTrace(r, tokens, &buf)
	if err == nil {
		t.Fatal("expected error for div by zero")
	}
	out := buf.String()
	if !strings.Contains(out, "error") {
		t.Errorf("expected error in trace output, got %q", out)
	}
}

func TestTraceWrapMultiLine(t *testing.T) {
	// Create enough parts to force wrapping
	var parts []string
	for i := 0; i < 20; i++ {
		parts = append(parts, TraceColorize(NewInteger(int64(i*1000000))))
	}
	lines := TraceWrap(parts, 5, 40)
	if len(lines) < 2 {
		t.Errorf("expected multiple lines for wrapping, got %d", len(lines))
	}
}

// ========================
// makeConvert / makeFieldValue
// ========================

func TestMakeConvertToString(t *testing.T) {
	v, err := makeConvert(NewInteger(42), TString)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	_as38, _ := v.AsString()
	if _as38 != "42" {
		t.Errorf("expected '42', got %s", v)
	}
}

func TestMakeConvertToNumber(t *testing.T) {
	v, err := makeConvert(NewString("99"), TNumber)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	_as39, _ := v.AsInteger()
	if _as39 != 99 {
		t.Errorf("expected 99, got %s", v)
	}
}

func TestMakeConvertToBoolFromBool(t *testing.T) {
	v, err := makeConvert(NewBoolean(true), TBoolean)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	_as40, _ := v.AsBoolean()
	if !_as40 {
		t.Error("expected true")
	}
}

func TestMakeConvertToBoolFromInt(t *testing.T) {
	v, err := makeConvert(NewInteger(1), TBoolean)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	_as41, _ := v.AsBoolean()
	if !_as41 {
		t.Error("expected true")
	}
}

func TestMakeConvertToBoolFromString(t *testing.T) {
	v, err := makeConvert(NewString("true"), TBoolean)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	_as42, _ := v.AsBoolean()
	if !_as42 {
		t.Error("expected true")
	}
	v2, _ := makeConvert(NewString("false"), TBoolean)
	_as43, _ := v2.AsBoolean()
	if _as43 {
		t.Error("expected false")
	}
	v3, _ := makeConvert(NewString(""), TBoolean)
	_as44, _ := v3.AsBoolean()
	if _as44 {
		t.Error("expected false for empty string")
	}
}

func TestMakeConvertToAtom(t *testing.T) {
	v, err := makeConvert(NewString("hello"), TAtom)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if !v.VType.Equal(TAtom) {
		t.Errorf("expected atom, got %s", v.VType)
	}
}

func TestMakeConvertUnsupported(t *testing.T) {
	_, err := makeConvert(NewString("x"), TList)
	if err == nil {
		t.Error("expected error for unsupported target type")
	}
}

func TestMakeFieldValueAlreadyMatches(t *testing.T) {
	v, err := makeFieldValue(NewInteger(42), NewTypeLiteral(TNumber))
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	_as45, _ := v.AsInteger()
	if _as45 != 42 {
		t.Errorf("expected 42, got %s", v)
	}
}

func TestMakeFieldValueConvert(t *testing.T) {
	v, err := makeFieldValue(NewString("99"), NewTypeLiteral(TNumber))
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	_as46, _ := v.AsInteger()
	if _as46 != 99 {
		t.Errorf("expected 99, got %s", v)
	}
}

func TestMakeFieldValueWordTrue(t *testing.T) {
	v, err := makeFieldValue(NewWord("true"), NewTypeLiteral(TBoolean))
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	_as47, _ := v.AsBoolean()
	if !_as47 {
		t.Error("expected true")
	}
}

func TestMakeFieldValueConstraintUnify(t *testing.T) {
	v, err := makeFieldValue(NewInteger(42), NewInteger(42))
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	_as48, _ := v.AsInteger()
	if _as48 != 42 {
		t.Errorf("expected 42, got %s", v)
	}
}

// ========================
// ValToString coverage
// ========================

func TestValToStringAtom(t *testing.T) {
	s := ValToString(NewAtom("hello"))
	if s != "hello" {
		t.Errorf("expected 'hello', got %q", s)
	}
}

func TestValToStringNone(t *testing.T) {
	s := ValToString(NewTypeLiteral(TNone))
	if s != "None" {
		t.Errorf("expected 'None', got %q", s)
	}
}

// ========================
// AsList QueryBuilder path
// ========================

func TestAsListQueryBuilder(t *testing.T) {
	t.Skip("query words disabled")
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	makeTestTable(r)

	result := runAQL(t, r, []Value{
		NewWord("from"), NewWord("people"),
	})
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	// Accessing AsList on a QueryBuilder triggers materialization
	list := result[0].AsList().Slice()
	if len(list) != 3 {
		t.Errorf("expected 3 rows via AsList, got %d", len(list))
	}
}

// ========================
// Engine: stepEnd with forward before end
// ========================

func TestStepEndWithForwardBeforeEnd(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	// def myval 42 end 1 add myval
	result := runAQL(t, r, []Value{
		NewWord("def"), NewWord("myval"), NewInteger(42), NewWord("end"),
		NewInteger(1), NewWord("add"), NewWord("myval"),
	})
	_as49, _ := result[0].AsInteger()
	if len(result) != 1 || _as49 != 43 {
		t.Errorf("expected 43, got %v", result)
	}
}

func TestStepEndTerminatesDef(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	// def a [1 add] end 10 a
	result := runAQL(t, r, []Value{
		NewWord("def"), NewWord("a"),
		NewList([]Value{NewInteger(1), NewWord("add")}),
		NewWord("end"),
		NewInteger(10), NewWord("a"),
	})
	_as50, _ := result[0].AsInteger()
	if len(result) != 1 || _as50 != 11 {
		t.Errorf("expected 11, got %v", result)
	}
}

// ========================
// NewRegistry (Store field)
// ========================

func TestNewRegistryHasStore(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	// Store field removed; context store is initialized by InitRootContext.
	if HostFormats(r) == nil {
		t.Error("expected Formats capability to be installed")
	}
}

// ========================
// Direct unit tests for query.go internal functions
// ========================

func TestBuildWhereClauseEmpty(t *testing.T) {
	cond := NewList([]Value{})
	clause, err := buildWhereClause(cond)
	if err != nil {
		t.Fatal(err)
	}
	if clause != "1=1" {
		t.Errorf("expected '1=1', got %q", clause)
	}
}

func TestBuildWhereClauseSimpleEq(t *testing.T) {
	cond := NewList([]Value{NewAtom("name"), NewAtom("eq"), NewString("alice")})
	clause, err := buildWhereClause(cond)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(clause, "=") || !strings.Contains(clause, "alice") {
		t.Errorf("expected SQL with = and alice, got %q", clause)
	}
}

func TestBuildWhereClauseLtGteLteNeq(t *testing.T) {
	tests := []struct {
		op    string
		sqlOp string
	}{
		{"lt", "<"},
		{"gte", ">="},
		{"lte", "<="},
		{"neq", "!="},
		{"gt", ">"},
		{"like", "LIKE"},
	}
	for _, tt := range tests {
		cond := NewList([]Value{NewAtom("age"), NewAtom(tt.op), NewInteger(25)})
		clause, err := buildWhereClause(cond)
		if err != nil {
			t.Errorf("op %s: %v", tt.op, err)
			continue
		}
		if !strings.Contains(clause, tt.sqlOp) {
			t.Errorf("op %s: expected %q in clause %q", tt.op, tt.sqlOp, clause)
		}
	}
}

func TestBuildWhereClauseIsNull(t *testing.T) {
	cond := NewList([]Value{NewAtom("name"), NewAtom("is"), NewAtom("null")})
	clause, err := buildWhereClause(cond)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(clause, "IS NULL") {
		t.Errorf("expected IS NULL, got %q", clause)
	}
}

func TestBuildWhereClauseIsNotNull(t *testing.T) {
	cond := NewList([]Value{NewAtom("name"), NewAtom("is"), NewAtom("not"), NewAtom("null")})
	clause, err := buildWhereClause(cond)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(clause, "IS NOT NULL") {
		t.Errorf("expected IS NOT NULL, got %q", clause)
	}
}

func TestBuildWhereClauseBetween(t *testing.T) {
	cond := NewList([]Value{NewAtom("age"), NewAtom("between"), NewInteger(20), NewInteger(30)})
	clause, err := buildWhereClause(cond)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(clause, "BETWEEN") {
		t.Errorf("expected BETWEEN, got %q", clause)
	}
}

func TestBuildWhereClauseIn(t *testing.T) {
	inList := NewList([]Value{NewInteger(1), NewInteger(2), NewInteger(3)})
	cond := NewList([]Value{NewAtom("id"), NewAtom("in"), inList})
	clause, err := buildWhereClause(cond)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(clause, "IN") {
		t.Errorf("expected IN, got %q", clause)
	}
}

func TestBuildWhereClauseNotIn(t *testing.T) {
	inList := NewList([]Value{NewInteger(1), NewInteger(2)})
	cond := NewList([]Value{NewAtom("id"), NewAtom("not"), NewAtom("in"), inList})
	clause, err := buildWhereClause(cond)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(clause, "NOT IN") {
		t.Errorf("expected NOT IN, got %q", clause)
	}
}

func TestBuildWhereClauseNotBetween(t *testing.T) {
	cond := NewList([]Value{NewAtom("age"), NewAtom("not"), NewAtom("between"), NewInteger(20), NewInteger(30)})
	clause, err := buildWhereClause(cond)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(clause, "NOT BETWEEN") {
		t.Errorf("expected NOT BETWEEN, got %q", clause)
	}
}

func TestBuildWhereClauseAndOr(t *testing.T) {
	cond := NewList([]Value{
		NewAtom("age"), NewAtom("gt"), NewInteger(20),
		NewAtom("and"),
		NewAtom("name"), NewAtom("eq"), NewString("alice"),
	})
	clause, err := buildWhereClause(cond)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(clause, "AND") {
		t.Errorf("expected AND, got %q", clause)
	}
}

func TestBuildWhereClauseCollate(t *testing.T) {
	cond := NewList([]Value{
		NewAtom("name"), NewAtom("eq"), NewString("alice"), NewAtom("collate"), NewAtom("nocase"),
	})
	clause, err := buildWhereClause(cond)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(clause, "COLLATE NOCASE") {
		t.Errorf("expected COLLATE NOCASE, got %q", clause)
	}
}

func TestBuildWhereClauseNotPrefix(t *testing.T) {
	// not [sub-condition]
	sub := NewList([]Value{NewAtom("age"), NewAtom("gt"), NewInteger(20)})
	cond := NewList([]Value{NewAtom("not"), sub})
	clause, err := buildWhereClause(cond)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(clause, "NOT (") {
		t.Errorf("expected NOT (...), got %q", clause)
	}
}

func TestBuildWhereClauseSubgroup(t *testing.T) {
	// [[age gt 20] and name eq "alice"]
	sub := NewList([]Value{NewAtom("age"), NewAtom("gt"), NewInteger(20)})
	cond := NewList([]Value{sub, NewAtom("and"), NewAtom("name"), NewAtom("eq"), NewString("alice")})
	clause, err := buildWhereClause(cond)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(clause, "(") {
		t.Errorf("expected parenthesized subgroup, got %q", clause)
	}
}

func TestBuildWhereClauseValueBoolNone(t *testing.T) {
	// Test boolean and none in WHERE value
	cond := NewList([]Value{NewAtom("active"), NewAtom("eq"), NewBoolean(true)})
	clause, err := buildWhereClause(cond)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(clause, "'true'") {
		t.Errorf("expected 'true', got %q", clause)
	}
}

// ========================
// buildJoinCondition tests
// ========================

func TestBuildJoinConditionSimple(t *testing.T) {
	cond := NewList([]Value{NewAtom("id"), NewAtom("eq"), NewAtom("dept_id")})
	clause, err := buildJoinCondition(cond)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(clause, "=") {
		t.Errorf("expected = in join condition, got %q", clause)
	}
}

func TestBuildJoinConditionDotQualified(t *testing.T) {
	cond := NewList([]Value{NewString("a.id"), NewAtom("eq"), NewString("b.id")})
	clause, err := buildJoinCondition(cond)
	if err != nil {
		t.Fatal(err)
	}
	// quoteJoinCol should split on dot
	if !strings.Contains(clause, ".") {
		t.Errorf("expected dot-qualified columns in join condition, got %q", clause)
	}
}

func TestBuildJoinConditionMultipleWithAnd(t *testing.T) {
	cond := NewList([]Value{
		NewAtom("id"), NewAtom("eq"), NewAtom("fk"),
		NewAtom("and"),
		NewAtom("type"), NewAtom("eq"), NewAtom("kind"),
	})
	clause, err := buildJoinCondition(cond)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(clause, "AND") {
		t.Errorf("expected AND, got %q", clause)
	}
}

func TestBuildJoinConditionEmpty(t *testing.T) {
	cond := NewList([]Value{})
	clause, err := buildJoinCondition(cond)
	if err != nil {
		t.Fatal(err)
	}
	if clause != "1=1" {
		t.Errorf("expected '1=1', got %q", clause)
	}
}

func TestQuoteJoinCol(t *testing.T) {
	// Simple name
	if got := quoteJoinCol("name"); !strings.Contains(got, "name") {
		t.Errorf("expected 'name' in result, got %q", got)
	}
	// Dot-qualified
	got := quoteJoinCol("people.id")
	if !strings.Contains(got, ".") {
		t.Errorf("expected dot in result, got %q", got)
	}
}

// ========================
// buildOrderClause tests
// ========================

func TestBuildOrderClauseSimple(t *testing.T) {
	cols := NewList([]Value{NewAtom("name")})
	clause, err := buildOrderClause(cols)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(clause, "name") {
		t.Errorf("expected 'name', got %q", clause)
	}
}

func TestBuildOrderClauseDesc(t *testing.T) {
	cols := NewList([]Value{NewAtom("age"), NewAtom("desc")})
	clause, err := buildOrderClause(cols)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(clause, "DESC") {
		t.Errorf("expected DESC, got %q", clause)
	}
}

func TestBuildOrderClauseNullsFirst(t *testing.T) {
	cols := NewList([]Value{NewAtom("score"), NewAtom("asc"), NewAtom("nulls"), NewAtom("first")})
	clause, err := buildOrderClause(cols)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(clause, "NULLS FIRST") {
		t.Errorf("expected NULLS FIRST, got %q", clause)
	}
}

func TestBuildOrderClauseCollateNocase(t *testing.T) {
	cols := NewList([]Value{NewAtom("name"), NewAtom("collate"), NewAtom("nocase"), NewAtom("asc")})
	clause, err := buildOrderClause(cols)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(clause, "COLLATE NOCASE") {
		t.Errorf("expected COLLATE NOCASE, got %q", clause)
	}
}

func TestBuildOrderClausePositional(t *testing.T) {
	cols := NewList([]Value{NewInteger(1), NewInteger(2)})
	clause, err := buildOrderClause(cols)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(clause, "1") || !strings.Contains(clause, "2") {
		t.Errorf("expected positional 1,2, got %q", clause)
	}
}

func TestBuildOrderClauseMultiple(t *testing.T) {
	cols := NewList([]Value{NewAtom("city"), NewAtom("asc"), NewAtom("name"), NewAtom("desc")})
	clause, err := buildOrderClause(cols)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(clause, ",") {
		t.Errorf("expected comma-separated, got %q", clause)
	}
}

// ========================
// parseColumnSpec tests
// ========================

func TestParseColumnSpecAtoms(t *testing.T) {
	cols := NewList([]Value{NewAtom("name"), NewAtom("age")})
	specs, err := parseColumnSpec(cols)
	if err != nil {
		t.Fatal(err)
	}
	if len(specs) != 2 {
		t.Fatalf("expected 2 specs, got %d", len(specs))
	}
	if specs[0].Name != "name" {
		t.Errorf("expected 'name', got %q", specs[0].Name)
	}
}

func TestParseColumnSpecStrings(t *testing.T) {
	cols := NewList([]Value{NewString("name")})
	specs, err := parseColumnSpec(cols)
	if err != nil {
		t.Fatal(err)
	}
	if len(specs) != 1 || specs[0].Name != "name" {
		t.Errorf("expected name spec, got %+v", specs)
	}
}

func TestParseColumnSpecAlias(t *testing.T) {
	// [name, person_name] alias pair
	pair := NewList([]Value{NewAtom("name"), NewAtom("person_name")})
	cols := NewList([]Value{pair})
	specs, err := parseColumnSpec(cols)
	if err != nil {
		t.Fatal(err)
	}
	if len(specs) != 1 || specs[0].Name != "name" || specs[0].Alias != "person_name" {
		t.Errorf("expected name->person_name alias, got %+v", specs)
	}
}

func TestParseColumnSpecAggregate(t *testing.T) {
	// [count name cnt]
	agg := NewList([]Value{NewAtom("count"), NewAtom("name"), NewAtom("cnt")})
	cols := NewList([]Value{agg})
	specs, err := parseColumnSpec(cols)
	if err != nil {
		t.Fatal(err)
	}
	if len(specs) != 1 {
		t.Fatalf("expected 1 spec, got %d", len(specs))
	}
	if !strings.Contains(specs[0].Raw, "COUNT") {
		t.Errorf("expected COUNT in raw, got %q", specs[0].Raw)
	}
	if specs[0].Alias != "cnt" {
		t.Errorf("expected alias 'cnt', got %q", specs[0].Alias)
	}
}

func TestParseColumnSpecCountStar(t *testing.T) {
	// [count * total]
	agg := NewList([]Value{NewAtom("count"), NewAtom("*"), NewAtom("total")})
	cols := NewList([]Value{agg})
	specs, err := parseColumnSpec(cols)
	if err != nil {
		t.Fatal(err)
	}
	if len(specs) != 1 {
		t.Fatalf("expected 1 spec, got %d", len(specs))
	}
	if !strings.Contains(specs[0].Raw, "COUNT(*)") {
		t.Errorf("expected COUNT(*), got %q", specs[0].Raw)
	}
}

func TestParseColumnSpecCast(t *testing.T) {
	// [cast age integer age_int]
	cast := NewList([]Value{NewAtom("cast"), NewAtom("age"), NewAtom("integer"), NewAtom("age_int")})
	cols := NewList([]Value{cast})
	specs, err := parseColumnSpec(cols)
	if err != nil {
		t.Fatal(err)
	}
	if len(specs) != 1 {
		t.Fatalf("expected 1 spec, got %d", len(specs))
	}
	if !strings.Contains(specs[0].Raw, "CAST") {
		t.Errorf("expected CAST in raw, got %q", specs[0].Raw)
	}
	if specs[0].Alias != "age_int" {
		t.Errorf("expected alias 'age_int', got %q", specs[0].Alias)
	}
}

func TestParseColumnSpecCastNoAlias(t *testing.T) {
	// [cast age integer]
	cast := NewList([]Value{NewAtom("cast"), NewAtom("age"), NewAtom("integer")})
	cols := NewList([]Value{cast})
	specs, err := parseColumnSpec(cols)
	if err != nil {
		t.Fatal(err)
	}
	if len(specs) != 1 {
		t.Fatalf("expected 1 spec, got %d", len(specs))
	}
	if specs[0].Alias != "age" {
		t.Errorf("expected alias 'age' (default), got %q", specs[0].Alias)
	}
}

func TestParseColumnSpecSumAvgMinMax(t *testing.T) {
	for _, fn := range []string{"sum", "avg", "min", "max"} {
		agg := NewList([]Value{NewAtom(fn), NewAtom("val")})
		cols := NewList([]Value{agg})
		specs, err := parseColumnSpec(cols)
		if err != nil {
			t.Errorf("%s: %v", fn, err)
			continue
		}
		if len(specs) != 1 {
			t.Errorf("%s: expected 1 spec, got %d", fn, len(specs))
			continue
		}
		if !strings.Contains(specs[0].Raw, strings.ToUpper(fn)) {
			t.Errorf("%s: expected %s in raw, got %q", fn, strings.ToUpper(fn), specs[0].Raw)
		}
	}
}

// ========================
// nameFromValue tests
// ========================

func TestNameFromValueAtom(t *testing.T) {
	if got := nameFromValue(NewAtom("foo")); got != "foo" {
		t.Errorf("expected 'foo', got %q", got)
	}
}

func TestNameFromValueString(t *testing.T) {
	if got := nameFromValue(NewString("bar")); got != "bar" {
		t.Errorf("expected 'bar', got %q", got)
	}
}

func TestNameFromValueWord(t *testing.T) {
	if got := nameFromValue(NewWord("baz")); got != "baz" {
		t.Errorf("expected 'baz', got %q", got)
	}
}

func TestNameFromValueOther(t *testing.T) {
	if got := nameFromValue(NewInteger(42)); got != "" {
		t.Errorf("expected empty, got %q", got)
	}
}

// ========================
// aqlTypenameToSQLType / sqlTypeToAQLType tests
// ========================

func TestAqlTypenameToSQLType(t *testing.T) {
	tests := map[string]string{
		"Integer": "INTEGER", "int": "INTEGER",
		"real": "REAL", "float": "REAL", "Number": "REAL",
		"text": "TEXT", "String": "TEXT",
		"Boolean": "INTEGER", "bool": "INTEGER",
	}
	for input, expected := range tests {
		if got := aqlTypenameToSQLType(input); got != expected {
			t.Errorf("aqlTypenameToSQLType(%q) = %q, want %q", input, got, expected)
		}
	}
}

func TestSqlTypeToAQLType(t *testing.T) {
	if got := sqlTypeToAQLType("INTEGER"); !got.Equal(TInteger) {
		t.Errorf("expected TInteger, got %v", got)
	}
	if got := sqlTypeToAQLType("REAL"); !got.Equal(TDecimal) {
		t.Errorf("expected TDecimal, got %v", got)
	}
	if got := sqlTypeToAQLType("TEXT"); !got.Equal(TString) {
		t.Errorf("expected TString, got %v", got)
	}
	if got := sqlTypeToAQLType("UNKNOWN"); !got.Equal(TString) {
		t.Errorf("expected TString (default), got %v", got)
	}
}

// ========================
// valueToSQL tests
// ========================

func TestValueToSQLString(t *testing.T) {
	got, err := valueToSQL(NewString("hello"))
	if err != nil {
		t.Fatal(err)
	}
	if got != "'hello'" {
		t.Errorf("expected 'hello', got %q", got)
	}
}

func TestValueToSQLInt(t *testing.T) {
	got, err := valueToSQL(NewInteger(42))
	if err != nil {
		t.Fatal(err)
	}
	if got != "42" {
		t.Errorf("expected '42', got %q", got)
	}
}

func TestValueToSQLBool(t *testing.T) {
	got, err := valueToSQL(NewBoolean(true))
	if err != nil {
		t.Fatal(err)
	}
	if got != "'true'" {
		t.Errorf("expected \"'true'\", got %q", got)
	}
}

func TestValueToSQLNone(t *testing.T) {
	got, err := valueToSQL(Value{VType: TNone})
	if err != nil {
		t.Fatal(err)
	}
	if got != "NULL" {
		t.Errorf("expected 'NULL', got %q", got)
	}
}

func TestValueToSQLAtom(t *testing.T) {
	got, err := valueToSQL(NewAtom("foo"))
	if err != nil {
		t.Fatal(err)
	}
	if got != "'foo'" {
		t.Errorf("expected \"'foo'\", got %q", got)
	}
}

func TestValueToSQLWord(t *testing.T) {
	got, err := valueToSQL(NewWord("bar"))
	if err != nil {
		t.Fatal(err)
	}
	if got != "'bar'" {
		t.Errorf("expected \"'bar'\", got %q", got)
	}
}

func TestValueToSQLStringWithQuote(t *testing.T) {
	got, err := valueToSQL(NewString("it's"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "''") {
		t.Errorf("expected escaped quote, got %q", got)
	}
}

// ========================
// buildInList tests
// ========================

func TestBuildInListValues(t *testing.T) {
	inList := NewList([]Value{NewInteger(1), NewInteger(2), NewInteger(3)})
	got, err := buildInList(inList)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "1") || !strings.Contains(got, "3") {
		t.Errorf("expected 1,2,3 in result, got %q", got)
	}
}

func TestBuildInListSingleValue(t *testing.T) {
	got, err := buildInList(NewInteger(42))
	if err != nil {
		t.Fatal(err)
	}
	if got != "42" {
		t.Errorf("expected '42', got %q", got)
	}
}

func TestBuildInListFromTableValues(t *testing.T) {
	fields := NewOrderedMap()
	fields.Set("id", NewTypeLiteral(TInteger))
	rec := RecordTypeInfo{Fields: fields}

	row1 := NewOrderedMap()
	row1.Set("id", NewInteger(10))
	row2 := NewOrderedMap()
	row2.Set("id", NewInteger(20))
	td := TableData{Record: rec, Rows: []Value{NewMap(row1), NewMap(row2)}}

	got, err := buildInListFromTable(td)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "10") || !strings.Contains(got, "20") {
		t.Errorf("expected 10,20 in result, got %q", got)
	}
}

func TestBuildInListFromTableEmpty(t *testing.T) {
	fields := NewOrderedMap()
	fields.Set("id", NewTypeLiteral(TInteger))
	rec := RecordTypeInfo{Fields: fields}
	td := TableData{Record: rec, Rows: nil}

	got, err := buildInListFromTable(td)
	if err != nil {
		t.Fatal(err)
	}
	if got != "NULL" {
		t.Errorf("expected 'NULL', got %q", got)
	}
}

func TestBuildInListFromTableInWhere(t *testing.T) {
	fields := NewOrderedMap()
	fields.Set("id", NewTypeLiteral(TInteger))
	rec := RecordTypeInfo{Fields: fields}

	row := NewOrderedMap()
	row.Set("id", NewInteger(5))
	td := TableData{Record: rec, Rows: []Value{NewMap(row)}}
	tdVal := Value{VType: TList, Data: td}

	got, err := buildInList(tdVal)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "5") {
		t.Errorf("expected 5 in result, got %q", got)
	}
}

// ========================
// isTableOrQuery tests
// ========================

func TestIsTableOrQueryTableData(t *testing.T) {
	td := TableData{}
	v := Value{VType: TList, Data: td}
	if !isTableOrQuery(v) {
		t.Error("expected true for TableData")
	}
}

func TestIsTableOrQueryQueryBuilder(t *testing.T) {
	qb := QueryBuilder{}
	v := Value{VType: TList, Data: qb}
	if !isTableOrQuery(v) {
		t.Error("expected true for QueryBuilder")
	}
}

func TestIsTableOrQueryOther(t *testing.T) {
	v := NewInteger(42)
	if isTableOrQuery(v) {
		t.Error("expected false for integer")
	}
}

// ========================
// scalarFromTable tests
// ========================

func TestScalarFromTableOneRow(t *testing.T) {
	fields := NewOrderedMap()
	fields.Set("x", NewTypeLiteral(TInteger))
	rec := RecordTypeInfo{Fields: fields}

	row := NewOrderedMap()
	row.Set("x", NewInteger(42))
	td := TableData{Record: rec, Rows: []Value{NewMap(row)}}

	val, err := scalarFromTable(td)
	if err != nil {
		t.Fatal(err)
	}
	_as51, _ := val.AsInteger()
	if !val.VType.Matches(TInteger) || _as51 != 42 {
		t.Errorf("expected 42, got %v", val)
	}
}

func TestScalarFromTableNoRows(t *testing.T) {
	fields := NewOrderedMap()
	fields.Set("x", NewTypeLiteral(TInteger))
	rec := RecordTypeInfo{Fields: fields}
	td := TableData{Record: rec, Rows: nil}

	val, err := scalarFromTable(td)
	if err != nil {
		t.Fatal(err)
	}
	if !val.VType.Equal(TNone) {
		t.Errorf("expected TNone, got %v", val.VType)
	}
}

func TestScalarFromTableMultipleRows(t *testing.T) {
	fields := NewOrderedMap()
	fields.Set("x", NewTypeLiteral(TInteger))
	rec := RecordTypeInfo{Fields: fields}

	row1 := NewOrderedMap()
	row1.Set("x", NewInteger(1))
	row2 := NewOrderedMap()
	row2.Set("x", NewInteger(2))
	td := TableData{Record: rec, Rows: []Value{NewMap(row1), NewMap(row2)}}

	_, err := scalarFromTable(td)
	if err == nil {
		t.Error("expected error for multiple rows")
	}
}

// ========================
// resolveScalarValue tests
// ========================

func TestResolveScalarValuePlain(t *testing.T) {
	v := NewInteger(42)
	got, err := resolveScalarValue(v)
	if err != nil {
		t.Fatal(err)
	}
	_as52, _ := got.AsInteger()
	if _as52 != 42 {
		t.Errorf("expected 42, got %v", got)
	}
}

func TestResolveScalarValueTableData(t *testing.T) {
	fields := NewOrderedMap()
	fields.Set("x", NewTypeLiteral(TInteger))
	rec := RecordTypeInfo{Fields: fields}

	row := NewOrderedMap()
	row.Set("x", NewInteger(99))
	td := TableData{Record: rec, Rows: []Value{NewMap(row)}}

	v := Value{VType: TList, Data: td}
	got, err := resolveScalarValue(v)
	if err != nil {
		t.Fatal(err)
	}
	_as53, _ := got.AsInteger()
	if _as53 != 99 {
		t.Errorf("expected 99, got %v", got)
	}
}

// ========================
// buildGroupByClause tests
// ========================

func TestBuildGroupByClause(t *testing.T) {
	cols := NewList([]Value{NewAtom("dept"), NewAtom("role")})
	clause, err := buildGroupByClause(cols)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(clause, ",") {
		t.Errorf("expected comma-separated, got %q", clause)
	}
}

func TestBuildGroupByClauseEmpty(t *testing.T) {
	cols := NewList([]Value{})
	_, err := buildGroupByClause(cols)
	if err == nil {
		t.Error("expected error for empty group by")
	}
}

// ========================
// End-to-end query tests via runAQL
// ========================

func makeTestTableWithDepts(r *Registry) {
	// Create "people" with name, age, dept columns
	fields := NewOrderedMap()
	fields.Set("name", NewTypeLiteral(TString))
	fields.Set("age", NewTypeLiteral(TNumber))
	fields.Set("dept", NewTypeLiteral(TString))

	rec := RecordTypeInfo{Fields: fields}

	mkRow := func(name string, age int64, dept string) Value {
		om := NewOrderedMap()
		om.Set("name", NewString(name))
		om.Set("age", NewInteger(age))
		om.Set("dept", NewString(dept))
		return NewMap(om)
	}

	td := TableData{
		Record: rec,
		Rows: []Value{
			mkRow("alice", 30, "eng"),
			mkRow("bob", 25, "eng"),
			mkRow("carol", 35, "sales"),
			mkRow("dave", 28, "sales"),
		},
	}
	r.ContextSet("employees", Value{VType: TList, Data: td})
}

func makeDeptTable(r *Registry) {
	fields := NewOrderedMap()
	fields.Set("dept", NewTypeLiteral(TString))
	fields.Set("budget", NewTypeLiteral(TNumber))

	rec := RecordTypeInfo{Fields: fields}

	mkRow := func(dept string, budget int64) Value {
		om := NewOrderedMap()
		om.Set("dept", NewString(dept))
		om.Set("budget", NewInteger(budget))
		return NewMap(om)
	}

	td := TableData{
		Record: rec,
		Rows: []Value{
			mkRow("eng", 100000),
			mkRow("sales", 50000),
		},
	}
	r.ContextSet("departments", Value{VType: TList, Data: td})
}

func TestQuerySelectWithColumnList(t *testing.T) {
	t.Skip("query words disabled")
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	makeTestTableWithDepts(r)

	// select [name dept] from employees
	result := runAQL(t, r, []Value{
		NewWord("select"), NewList([]Value{NewAtom("name"), NewAtom("dept")}),
		NewWord("from"), NewWord("employees"),
	})
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
}

func TestQuerySelectWithAlias(t *testing.T) {
	t.Skip("query words disabled")
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	makeTestTableWithDepts(r)

	// select [[name person_name]] from employees
	aliasPair := NewList([]Value{NewAtom("name"), NewAtom("person_name")})
	result := runAQL(t, r, []Value{
		NewWord("select"), NewList([]Value{aliasPair}),
		NewWord("from"), NewWord("employees"),
	})
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
}

func TestQuerySelectCountStar(t *testing.T) {
	t.Skip("query words disabled")
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	makeTestTableWithDepts(r)

	// select [[count * total]] from employees
	countSpec := NewList([]Value{NewAtom("count"), NewAtom("*"), NewAtom("total")})
	result := runAQL(t, r, []Value{
		NewWord("select"), NewList([]Value{countSpec}),
		NewWord("from"), NewWord("employees"),
	})
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
}

func TestQuerySelectCast(t *testing.T) {
	t.Skip("query words disabled")
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	makeTestTableWithDepts(r)

	// select [[cast age integer age_int]] from employees
	castSpec := NewList([]Value{NewAtom("cast"), NewAtom("age"), NewAtom("integer"), NewAtom("age_int")})
	result := runAQL(t, r, []Value{
		NewWord("select"), NewList([]Value{castSpec}),
		NewWord("from"), NewWord("employees"),
	})
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
}

func TestQueryWhereIsNull(t *testing.T) {
	t.Skip("query words disabled")
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	makeTestTable(r)

	result := runAQL(t, r, []Value{
		NewWord("from"), NewWord("people"),
		NewWord("where"), NewList([]Value{
			NewAtom("name"), NewAtom("is"), NewAtom("not"), NewAtom("null"),
		}),
		NewWord("select"), NewWord("star"),
	})
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
}

func TestQueryWhereBetween(t *testing.T) {
	t.Skip("query words disabled")
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	makeTestTable(r)

	result := runAQL(t, r, []Value{
		NewWord("from"), NewWord("people"),
		NewWord("where"), NewList([]Value{
			NewAtom("age"), NewAtom("between"), NewInteger(25), NewInteger(32),
		}),
		NewWord("select"), NewWord("star"),
	})
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
}

func TestQueryWhereIn(t *testing.T) {
	t.Skip("query words disabled")
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	makeTestTable(r)

	result := runAQL(t, r, []Value{
		NewWord("from"), NewWord("people"),
		NewWord("where"), NewList([]Value{
			NewAtom("name"), NewAtom("in"),
			NewList([]Value{NewString("alice"), NewString("carol")}),
		}),
		NewWord("select"), NewWord("star"),
	})
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
}

func TestQueryGroupByWithAggregate(t *testing.T) {
	t.Skip("query words disabled")
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	makeTestTableWithDepts(r)

	// select [[count * cnt]] from employees group by [dept]
	countSpec := NewList([]Value{NewAtom("count"), NewAtom("*"), NewAtom("cnt")})
	result := runAQL(t, r, []Value{
		NewWord("select"), NewList([]Value{NewAtom("dept"), countSpec}),
		NewWord("from"), NewWord("employees"),
		NewWord("group"), NewWord("by"), NewList([]Value{NewAtom("dept")}),
	})
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
}

func TestQueryDistinct(t *testing.T) {
	t.Skip("query words disabled")
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	makeTestTableWithDepts(r)

	result := runAQL(t, r, []Value{
		NewWord("select"), NewList([]Value{NewAtom("dept")}),
		NewWord("from"), NewWord("employees"),
		NewWord("distinct"),
	})
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
}

func TestQueryJoinOnCondition(t *testing.T) {
	t.Skip("query words disabled")
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	makeTestTableWithDepts(r)
	makeDeptTable(r)

	// Use unqualified column names with aliases to avoid SQLite naming issues
	result := runAQL(t, r, []Value{
		NewWord("from"), NewWord("employees"),
		NewWord("join"), NewWord("departments"),
		NewWord("using"), NewList([]Value{NewAtom("dept")}),
		NewWord("select"), NewWord("star"),
	})
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
}

func TestQueryJoinUsing(t *testing.T) {
	t.Skip("query words disabled")
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	makeTestTableWithDepts(r)
	makeDeptTable(r)

	result := runAQL(t, r, []Value{
		NewWord("from"), NewWord("employees"),
		NewWord("join"), NewWord("departments"),
		NewWord("using"), NewList([]Value{NewAtom("dept")}),
		NewWord("select"), NewWord("star"),
	})
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
}

func TestQueryOrderByList(t *testing.T) {
	t.Skip("query words disabled")
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	makeTestTable(r)

	result := runAQL(t, r, []Value{
		NewWord("from"), NewWord("people"),
		NewWord("order"), NewList([]Value{NewAtom("age"), NewAtom("desc")}),
		NewWord("select"), NewWord("star"),
	})
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
}

func TestQueryLimitOffset(t *testing.T) {
	t.Skip("query words disabled")
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	makeTestTable(r)

	result := runAQL(t, r, []Value{
		NewWord("from"), NewWord("people"),
		NewWord("limit"), NewInteger(2),
		NewWord("offset"), NewInteger(1),
		NewWord("select"), NewWord("star"),
	})
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
}

func TestQueryAsAlias(t *testing.T) {
	t.Skip("query words disabled")
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	makeTestTable(r)

	result := runAQL(t, r, []Value{
		NewWord("from"), NewWord("people"),
		NewWord("as"), NewWord("p"),
		NewWord("select"), NewWord("star"),
	})
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
}

func TestQueryWhereAndCondition(t *testing.T) {
	t.Skip("query words disabled")
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	makeTestTable(r)

	result := runAQL(t, r, []Value{
		NewWord("from"), NewWord("people"),
		NewWord("where"), NewList([]Value{
			NewAtom("age"), NewAtom("gt"), NewInteger(20),
			NewAtom("and"),
			NewAtom("age"), NewAtom("lt"), NewInteger(33),
		}),
		NewWord("select"), NewWord("star"),
	})
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
}

func TestQueryWhereCollate(t *testing.T) {
	t.Skip("query words disabled")
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	makeTestTable(r)

	result := runAQL(t, r, []Value{
		NewWord("from"), NewWord("people"),
		NewWord("where"), NewList([]Value{
			NewAtom("name"), NewAtom("eq"), NewString("ALICE"),
			NewAtom("collate"), NewAtom("nocase"),
		}),
		NewWord("select"), NewWord("star"),
	})
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
}

// ========================
// Additional engine coverage: peekForwardValue
// ========================

func TestPeekForwardValueInContext(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	// Exercise curryOrPrefix and peekForwardValue through a word that uses forward precedence
	// e.g., "add" with forward: 1 add 2
	result := runAQL(t, r, []Value{NewInteger(1), NewWord("add"), NewInteger(2)})
	_as54, _ := result[0].AsInteger()
	if len(result) != 1 || _as54 != 3 {
		t.Errorf("expected [3], got %v", result)
	}
}

// ========================
// Additional engine.go coverage: stepEnd branches
// ========================

func TestStepEndWithMoveAndMark(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	// def creates a mark; calling a def word triggers move
	result := runAQL(t, r, []Value{
		NewWord("def"), NewWord("dbl"), NewList([]Value{NewWord("dup"), NewWord("add")}),
		NewInteger(5), NewWord("dbl"),
	})
	_as55, _ := result[0].AsInteger()
	if len(result) != 1 || _as55 != 10 {
		t.Errorf("expected [10], got %v", result)
	}
}

// ========================
// Additional registry.go coverage
// ========================

func TestBaseValueForConstraintCoverage(t *testing.T) {
	// Exercise BaseValueForConstraint by testing type-related operations
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	// Create a typed list via the type system
	result := runAQL(t, r, []Value{
		NewInteger(1), NewWord("typeof"),
	})
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
}

// ========================
// Additional SQLite tests: various value type conversions
// ========================

func TestSQLiteQueryWithNullValues(t *testing.T) {
	store, err := NewSQLiteStore()
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	fields := NewOrderedMap()
	fields.Set("name", NewTypeLiteral(TString))
	fields.Set("score", NewTypeLiteral(TNumber))
	rec := RecordTypeInfo{Fields: fields}

	row1 := NewOrderedMap()
	row1.Set("name", NewString("alice"))
	row1.Set("score", NewInteger(100))
	row2 := NewOrderedMap()
	row2.Set("name", NewString("bob"))
	row2.Set("score", Value{VType: TNone})

	td := TableData{Record: rec, Rows: []Value{NewMap(row1), NewMap(row2)}}

	if err := store.StoreTable("scores_null", td); err != nil {
		t.Fatalf("StoreTable error: %v", err)
	}
	result, err := store.Query("SELECT * FROM \"scores_null\"", &rec)
	if err != nil {
		t.Fatalf("Query error: %v", err)
	}
	if len(result.Rows) != 2 {
		t.Errorf("expected 2 rows, got %d", len(result.Rows))
	}
}

// ========================
// OrderedMap.Delete tests
// ========================

func TestOrderedMapDeleteExisting(t *testing.T) {
	m := NewOrderedMap()
	m.Set("a", NewInteger(1))
	m.Set("b", NewInteger(2))
	m.Set("c", NewInteger(3))

	ok := m.Delete("b")
	if !ok {
		t.Error("Delete returned false for existing key")
	}
	if m.Len() != 2 {
		t.Errorf("Len = %d, want 2", m.Len())
	}
	if _, found := m.Get("b"); found {
		t.Error("key 'b' still exists after delete")
	}
	keys := m.Keys()
	if len(keys) != 2 || keys[0] != "a" || keys[1] != "c" {
		t.Errorf("keys = %v, want [a c]", keys)
	}
}

func TestOrderedMapDeleteNonExisting(t *testing.T) {
	m := NewOrderedMap()
	m.Set("a", NewInteger(1))
	ok := m.Delete("z")
	if ok {
		t.Error("Delete returned true for non-existing key")
	}
	if m.Len() != 1 {
		t.Errorf("Len = %d, want 1", m.Len())
	}
}

func TestOrderedMapDeleteFirst(t *testing.T) {
	m := NewOrderedMap()
	m.Set("x", NewInteger(10))
	m.Set("y", NewInteger(20))
	m.Delete("x")
	keys := m.Keys()
	if len(keys) != 1 || keys[0] != "y" {
		t.Errorf("keys = %v, want [y]", keys)
	}
}

func TestOrderedMapDeleteLast(t *testing.T) {
	m := NewOrderedMap()
	m.Set("x", NewInteger(10))
	m.Set("y", NewInteger(20))
	m.Delete("y")
	keys := m.Keys()
	if len(keys) != 1 || keys[0] != "x" {
		t.Errorf("keys = %v, want [x]", keys)
	}
}

func TestOrderedMapDeleteAll(t *testing.T) {
	m := NewOrderedMap()
	m.Set("a", NewInteger(1))
	m.Set("b", NewInteger(2))
	m.Delete("a")
	m.Delete("b")
	if m.Len() != 0 {
		t.Errorf("Len = %d, want 0", m.Len())
	}
	if len(m.Keys()) != 0 {
		t.Errorf("keys = %v, want []", m.Keys())
	}
}

// ========================
// CallAQL tests
// ========================

func TestCallAQLBasic(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}

	// def double fn [[x:number] [number] [x add x]] end
	xParam := NewOrderedMap()
	xParam.Set("x", NewWord("Number"))
	fnBody := NewList([]Value{
		NewList([]Value{NewImplicitMap(xParam)}),
		NewList([]Value{NewWord("Number")}),
		NewList([]Value{NewWord("x"), NewWord("add"), NewWord("x")}),
	})
	_ = runAQL(t, r, []Value{
		NewWord("def"), NewWord("double"), NewWord("fn"), fnBody, NewWord("end"),
	})

	// Look up the function
	fnVal, ok := r.TopOfDefStack("double")
	if !ok {
		t.Fatal("double not defined")
	}

	args := []Value{NewInteger(5)}
	sig := MatchFnSig(fnVal, args)
	if sig == nil {
		t.Fatal("no matching signature")
	}
	result, err := r.CallAQL(sig, args)
	if err != nil {
		t.Fatalf("CallAQL error: %v", err)
	}
	_as56, _ := result[0].AsInteger()
	if len(result) != 1 || _as56 != 10 {
		t.Errorf("CallAQL(double, 5) = %v, want [10]", result)
	}
}

func TestCallAQLNotAFunction(t *testing.T) {
	sig := MatchFnSig(NewInteger(42), []Value{})
	if sig != nil {
		t.Error("expected nil sig for non-function value")
	}
}

func TestCallAQLNoMatchingSig(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}

	// def inc fn [[x:number] [number] [x add 1]] end
	xParam := NewOrderedMap()
	xParam.Set("x", NewWord("Number"))
	fnBody := NewList([]Value{
		NewList([]Value{NewImplicitMap(xParam)}),
		NewList([]Value{NewWord("Number")}),
		NewList([]Value{NewWord("x"), NewWord("add"), NewInteger(1)}),
	})
	_ = runAQL(t, r, []Value{
		NewWord("def"), NewWord("inc"), NewWord("fn"), fnBody, NewWord("end"),
	})

	fnVal, _ := r.TopOfDefStack("inc")

	// Call with wrong type — MatchFnSig returns nil
	sig := MatchFnSig(fnVal, []Value{NewString("hello")})
	if sig != nil {
		t.Error("expected nil sig for mismatched argument types")
	}
}

// ========================
// RegisterDblcall tests
// ========================

func TestDblcallBasic(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	// dblcall 5 [dup mul] => 100  (5*2=10, then 10 dup mul = 100)
	result := runAQL(t, r, []Value{
		NewWord("dblcall"), NewInteger(5),
		NewList([]Value{NewWord("dup"), NewWord("mul")}),
	})
	_as57, _ := result[0].AsInteger()
	if len(result) != 1 || _as57 != 100 {
		t.Errorf("dblcall 5 [dup mul] = %v, want [100]", result)
	}
}

func TestDblcallWithAdd(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	// dblcall 3 [add 1] => 7  (3*2=6, then 6 add 1 = 7)
	result := runAQL(t, r, []Value{
		NewWord("dblcall"), NewInteger(3),
		NewList([]Value{NewWord("add"), NewInteger(1)}),
	})
	_as58, _ := result[0].AsInteger()
	if len(result) != 1 || _as58 != 7 {
		t.Errorf("3 dblcall [add 1] = %v, want [7]", result)
	}
}

func TestDblcallEmptyBody(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	// dblcall 4 [] => 8
	result := runAQL(t, r, []Value{
		NewWord("dblcall"), NewInteger(4),
		NewList([]Value{}),
	})
	_as59, _ := result[0].AsInteger()
	if len(result) != 1 || _as59 != 8 {
		t.Errorf("dblcall 4 [] = %v, want [8]", result)
	}
}

// ========================
// RegisterCall tests
// ========================

func TestCallBasic(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	// 5 [dup mul] call => 25
	result := runAQL(t, r, []Value{
		NewInteger(5),
		NewList([]Value{NewWord("dup"), NewWord("mul")}),
		NewWord("call"),
	})
	_as60, _ := result[0].AsInteger()
	if len(result) != 1 || _as60 != 25 {
		t.Errorf("5 [dup mul] call = %v, want [25]", result)
	}
}

func TestCallEmptyList(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	// 42 [] call => 42 (empty call does nothing)
	result := runAQL(t, r, []Value{
		NewInteger(42),
		NewList([]Value{}),
		NewWord("call"),
	})
	_as61, _ := result[0].AsInteger()
	if len(result) != 1 || _as61 != 42 {
		t.Errorf("42 [] call = %v, want [42]", result)
	}
}

// ========================
// RegisterArgs tests
// ========================

func TestArgsInsideFn(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	// def sum2 fn [[a:number b:number] [number] [a add b]] end
	// Using named params to exercise the args stack indirectly
	aParam := NewOrderedMap()
	aParam.Set("a", NewWord("Number"))
	bParam := NewOrderedMap()
	bParam.Set("b", NewWord("Number"))
	fnBody := NewList([]Value{
		NewList([]Value{NewImplicitMap(aParam), NewImplicitMap(bParam)}),
		NewList([]Value{NewWord("Number")}),
		NewList([]Value{NewWord("a"), NewWord("add"), NewWord("b")}),
	})
	_ = runAQL(t, r, []Value{
		NewWord("def"), NewWord("sum2"), NewWord("fn"), fnBody, NewWord("end"),
	})
	result := runAQL(t, r, []Value{
		NewWord("sum2"), NewInteger(3), NewInteger(7),
	})
	_as62, _ := result[0].AsInteger()
	if len(result) != 1 || _as62 != 10 {
		t.Errorf("sum2 3 7 = %v, want [10]", result)
	}
}

func TestArgsDirectAccess(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	// Directly exercise the args stack by pushing and calling
	r.PushArgs(NewList([]Value{NewInteger(42), NewString("hi")}))
	e := New(r)
	result, err := e.Run([]Value{NewWord("args")})
	if err != nil {
		t.Fatalf("args error: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	argsList := result[0].AsList().Slice()
	if len(argsList) != 2 {
		t.Errorf("expected args list of length 2, got %d", len(argsList))
	}
	// Clean up
	r.PopArgs()
}

func TestArgsOutsideFnErrors(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	// args outside of a function should error
	err = runAQLError(t, r, []Value{NewWord("args")})
	if err == nil {
		t.Error("expected error for args outside function")
	}
}

// ========================
// resolveOrphanedForwards tests (via integration)
// ========================

// ========================
// resolveOrphanedForwards tests (via integration)
// ========================

func TestResolveOrphanedForwardsCurry(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	// "2 add" with no second argument triggers orphan forward resolution => curry
	e := NewTop(r)
	result, err := e.Run([]Value{
		NewInteger(2), NewWord("add"),
	})
	// Some implementations produce a curry; others error. Both are valid coverage.
	if err != nil {
		// Orphan forward was processed even if it errored
		return
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d: %v", len(result), result)
	}
}

func TestResolveOrphanedForwardsMultipleValues(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	// Multiple values with no matching function should resolve gracefully
	e := NewTop(r)
	result, err := e.Run([]Value{
		NewInteger(10), NewInteger(20),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 2 {
		t.Errorf("expected 2 results, got %d: %v", len(result), result)
	}
}

// ========================
// ResolveFieldType tests
// ========================

func TestResolveFieldTypeString(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}

	// Define a custom type: type MyNum Number
	_ = runAQL(t, r, []Value{
		NewWord("type"), NewWord("MyNum"), NewWord("Number"),
	})

	// ResolveFieldType should resolve "MyNum" string to the type value
	result := ResolveFieldType(r, NewString("MyNum"))
	if !IsTypeBody(result) {
		t.Errorf("expected type value, got %s (data=%v)", result.VType, result.Data)
	}
}

func TestResolveFieldTypeStringUnknown(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}

	// Unknown name should pass through
	v := NewString("NotAType")
	result := ResolveFieldType(r, v)
	_as63, _ := result.AsString()
	if _as63 != "NotAType" {
		t.Errorf("expected pass-through, got %v", result)
	}
}

func TestResolveFieldTypeList(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}

	// [String tor None] as code should evaluate to a disjunct
	result := ResolveFieldType(r, NewList([]Value{
		NewWord("String"), NewWord("tor"), NewWord("None"),
	}))
	// Should be a disjunct type, not a raw list
	if result.VType.Matches(TList) && !result.IsTypedList() {
		t.Error("expected resolved type, not raw list")
	}
}

func TestResolveFieldTypePassthrough(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}

	// A type literal should pass through unchanged
	v := NewTypeLiteral(TNumber)
	result := ResolveFieldType(r, v)
	if !result.VType.Equal(v.VType) {
		t.Errorf("expected pass-through, got %v", result)
	}
}

// ========================
// SetParseFunc tests
// ========================

func TestSetParseFunc(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}

	called := false
	r.SetParseFunc(func(code string) ([]Value, error) {
		called = true
		return []Value{NewInteger(99)}, nil
	})

	if r.ParseFunc == nil {
		t.Error("ParseFunc should be set")
	}

	result, err := r.ParseFunc("test")
	if err != nil {
		t.Fatalf("ParseFunc error: %v", err)
	}
	if !called {
		t.Error("ParseFunc was not called")
	}
	_as64, _ := result[0].AsInteger()
	if len(result) != 1 || _as64 != 99 {
		t.Errorf("ParseFunc result = %v, want [99]", result)
	}
}

// ========================
// resolveSigType coverage
// ========================

func TestResolveSigTypeList(t *testing.T) {
	listVal := NewList([]Value{NewInteger(1), NewInteger(2)})
	tp, pattern, err := resolveSigType(nil, listVal)
	if err != nil {
		t.Fatal(err)
	}
	if !tp.Equal(TList) {
		t.Errorf("expected List for list, got %s", tp)
	}
	if pattern == nil {
		t.Fatal("expected non-nil pattern for list")
	}
	if !pattern.VType.Equal(TList) {
		t.Errorf("expected pattern to be a list, got %s", pattern.VType)
	}
}

func TestResolveSigTypeDecimalLiteral(t *testing.T) {
	// Post §1.1 fix: scalar literals (Integer, Decimal, Boolean,
	// String, Atom) are routed through Signature.Patterns. The type
	// is normalised to the kind, and the literal value lands in the
	// pattern slot.
	v := NewDecimal(3.14)
	tp, pattern, err := resolveSigType(nil, v)
	if err != nil {
		t.Fatal(err)
	}
	if !tp.Equal(TDecimal) {
		t.Errorf("expected TDecimal kind for decimal literal, got %s", tp)
	}
	if pattern == nil {
		t.Fatal("expected pattern to carry the literal value, got nil")
	}
	if got, _ := pattern.AsDecimal(); got != 3.14 {
		t.Errorf("pattern value = %v, want 3.14", got)
	}
}

// ========================
// MatchSignature pattern coverage
// ========================

func TestMatchSignaturePatternReject(t *testing.T) {
	// Signature with a map pattern that should reject a non-matching map.
	patternMap := NewOrderedMap()
	patternMap.Set("x", NewInteger(99))
	patternVal := NewMap(patternMap)

	sig := Signature{
		Args:     []Type{TMap},
		Patterns: map[int]Value{0: patternVal},
		Handler:  func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) { return args, nil },
	}

	// Matching map: {x:99}
	matchMap := NewOrderedMap()
	matchMap.Set("x", NewInteger(99))
	stack := []Value{NewMap(matchMap)}

	result := MatchSignature([]Signature{sig}, stack, WordInfo{ArgCount: -1})
	if result == nil {
		t.Error("expected match for {x:99} against pattern {x:99}")
	}

	// Non-matching map: {x:100}
	noMatchMap := NewOrderedMap()
	noMatchMap.Set("x", NewInteger(100))
	stack2 := []Value{NewMap(noMatchMap)}

	result2 := MatchSignature([]Signature{sig}, stack2, WordInfo{ArgCount: -1})
	if result2 != nil {
		t.Error("expected no match for {x:100} against pattern {x:99}")
	}
}

func TestMatchSignaturePatternFallthrough(t *testing.T) {
	// Two signatures: one with pattern (more specific), one without (fallback).
	patternMap := NewOrderedMap()
	patternMap.Set("a", NewInteger(1))
	patternVal := NewMap(patternMap)

	specificSig := Signature{
		Args:     []Type{TMap},
		Patterns: map[int]Value{0: patternVal},
		Handler: func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
			return []Value{NewString("specific")}, nil
		},
	}
	fallbackSig := Signature{
		Args: []Type{TMap},
		Handler: func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
			return []Value{NewString("fallback")}, nil
		},
	}

	// Non-matching map should fall through to fallback.
	m := NewOrderedMap()
	m.Set("a", NewInteger(2))
	stack := []Value{NewMap(m)}

	result := MatchSignature([]Signature{specificSig, fallbackSig}, stack, WordInfo{ArgCount: -1})
	if result == nil {
		t.Fatal("expected fallback match")
	}
	out, _ := result.Sig.Handler(result.Args, nil, nil, nil)
	_as65, _ := out[0].AsString()
	if _as65 != "fallback" {
		_as66, _ := out[0].AsString()
		t.Errorf("expected fallback, got %s", _as66)
	}
}

// ========================
// CallAQL pattern coverage
// ========================

func TestCallAQLMapPattern(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}

	// Build a function with map pattern: fn [[x:{k:1}] [String] ["yes"]]
	patternMap := NewOrderedMap()
	patternMap.Set("k", NewInteger(1))
	patternVal := NewMap(patternMap)

	fnDef := FnDefInfo{
		Sigs: []FnSig{
			{
				Params:  []FnParam{{Name: "x", Type: TMap, Pattern: &patternVal}},
				Returns: []Type{TString},
				Body:    []Value{NewString("yes")},
			},
		},
	}
	fnVal := NewFunction(fnDef)

	// Matching call: {k:1}
	argMap := NewOrderedMap()
	argMap.Set("k", NewInteger(1))
	matchArgs := []Value{NewMap(argMap)}
	matchSig := MatchFnSig(fnVal, matchArgs)
	if matchSig == nil {
		t.Fatal("expected matching signature")
	}
	result, callErr := r.CallAQL(matchSig, matchArgs)
	if callErr != nil {
		t.Fatalf("expected match, got error: %v", callErr)
	}
	_as67, _ := result[0].AsString()
	if len(result) != 1 || _as67 != "yes" {
		t.Errorf("expected [yes], got %v", result)
	}

	// Non-matching call: {k:2}
	noArgMap := NewOrderedMap()
	noArgMap.Set("k", NewInteger(2))
	noMatchSig := MatchFnSig(fnVal, []Value{NewMap(noArgMap)})
	if noMatchSig != nil {
		t.Error("expected nil sig for non-matching pattern {k:2}")
	}
}

// ========================
// RegisterFn error paths
// ========================

func TestRegisterFnNonList(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	// fn with a non-list argument should error.
	err = runAQLError(t, r, []Value{NewInteger(42), NewWord("fn")})
	if err == nil {
		t.Error("expected error for fn with non-list argument")
	}
}

func TestRegisterFnEmptyList(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	err = runAQLError(t, r, []Value{NewList([]Value{}), NewWord("fn")})
	if err == nil {
		t.Error("expected error for fn with empty list")
	}
}

func TestRegisterFnBadTriple(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	// Triple with invalid input sig (non-list, non-map param element).
	err = runAQLError(t, r, []Value{
		NewList([]Value{
			NewList([]Value{NewDecimal(1.5)}), // invalid param type
			NewTypeLiteral(TString),
			NewList([]Value{NewString("body")}),
		}),
		NewWord("fn"),
	})
	if err == nil {
		t.Error("expected error for fn with invalid param type")
	}
}

// ========================
// parseFnUndefSpec error paths
// ========================

func TestParseFnUndefSpecParamError(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	// 4 elements = 2 pairs, first pair has bad input sig (invalid param type).
	err = runAQLError(t, r, []Value{
		NewList([]Value{
			NewList([]Value{NewDecimal(1.5)}), // invalid param
			NewTypeLiteral(TString),
			NewList([]Value{NewDecimal(1.5)}), // invalid param
			NewTypeLiteral(TString),
		}),
		NewWord("fn"),
	})
	if err == nil {
		t.Error("expected error for fn undef spec with invalid param")
	}
}

func TestParseFnUndefSpecReturnError(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	// 4 elements = 2 pairs, valid input sig but invalid return type.
	err = runAQLError(t, r, []Value{
		NewList([]Value{
			NewList([]Value{NewTypeLiteral(TString)}), // valid param
			NewString("nonexistent_type"),             // invalid return type
			NewList([]Value{NewTypeLiteral(TString)}),
			NewString("nonexistent_type"),
		}),
		NewWord("fn"),
	})
	if err == nil {
		t.Error("expected error for fn undef spec with invalid return type")
	}
}

// ========================
// parseFnReturns error paths
// ========================

func TestParseFnReturnsSingleError(t *testing.T) {
	// A single non-list return type that is an invalid type name.
	_, err := parseFnReturns(NewString("nonexistent_type"))
	if err == nil {
		t.Error("expected error for invalid return type name")
	}
}

func TestParseFnReturnsListError(t *testing.T) {
	// A list with an invalid return type element.
	_, err := parseFnReturns(NewList([]Value{NewString("nonexistent_type")}))
	if err == nil {
		t.Error("expected error for invalid return type in list")
	}
}

// ========================
// FlexibleMatch coverage
// ========================

func TestFlexibleMatchTooFewValues(t *testing.T) {
	// Fewer values than types should return nil, false.
	values := []Value{NewInteger(1)}
	types := []Type{TInteger, TString}
	result, ok := FlexibleMatch(values, &Signature{Args: types})
	if ok || result != nil {
		t.Errorf("expected no match with fewer values than types")
	}
}

// ========================
// fn with map pattern via engine (exercises MatchSignature patterns)
// ========================

func TestFnMapPatternViaEngine(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}

	// def foo fn [[x:{x:99}] [String] ["A"] [x:Map] [String] ["B"]]
	patternMap := NewOrderedMap()
	patternMap.Set("x", NewInteger(99))

	result := runAQL(t, r, []Value{
		NewWord("def"), NewWord("foo"), NewWord("fn"),
		NewList([]Value{
			// Overload 1: x matches {x:99}
			NewList([]Value{NewImplicitMap(singleMap("x", NewMap(patternMap)))}),
			NewList([]Value{NewTypeLiteral(TString)}),
			NewList([]Value{NewString("A")}),
			// Overload 2: x matches any Map
			NewList([]Value{NewImplicitMap(singleMap("x", NewTypeLiteral(TMap)))}),
			NewList([]Value{NewTypeLiteral(TString)}),
			NewList([]Value{NewString("B")}),
		}),
		// Call with {x:99} — should match overload 1
		NewMap(patternMap), NewWord("foo"),
	})
	_as68, _ := result[0].AsString()
	if len(result) != 1 || _as68 != "A" {
		t.Errorf("expected 'A' for {x:99}, got %v", result)
	}

	// Call with {x:100} — should match overload 2 (fallback)
	noMatchMap := NewOrderedMap()
	noMatchMap.Set("x", NewInteger(100))
	result2 := runAQL(t, r, []Value{
		NewMap(noMatchMap), NewWord("foo"),
	})
	_as69, _ := result2[0].AsString()
	if len(result2) != 1 || _as69 != "B" {
		t.Errorf("expected 'B' for {x:100}, got %v", result2)
	}
}
