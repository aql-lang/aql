package eng

import (
	"errors"
	"math/big"
	"strings"
	"testing"

	"github.com/cockroachdb/apd/v3"
)

// Rendering and accessor coverage for value.go and print.go: every
// constructor round-trips through its accessor, every accessor rejects
// a foreign value, and the format paths never panic.

func TestKernelRenderTable(t *testing.T) {
	m := NewOrderedMap()
	m.Set("k", NewInteger(1))
	cases := []struct {
		name string
		v    Value
		want string // substring of v.String()
	}{
		{"word", NewWord("foo"), "word(foo)"},
		{"open-paren", NewOpenParen(), "("},
		{"close-paren", NewCloseParen(), ")"},
		{"end", NewEnd(), "end"},
		{"mark", NewMark("m1"), "mark(m1)"},
		{"move", NewMove("m1", "loop"), "move(m1,loop)"},
		{"paren-expr", NewParenExpr([]Value{NewInteger(1)}), "paren"},
		{"string", NewString("hi"), "'hi'"},
		{"atom", NewAtom("sym"), "sym"},
		{"integer", NewInteger(-3), "-3"},
		{"float", NewFloat(2.5), "2.5"},
		{"bool-true", NewBoolean(true), "true"},
		{"bool-false", NewBoolean(false), "false"},
		{"none-value", NewNone(), "none"},
		{"none-literal", NewTypeLiteral(TNone), "None"},
		{"type-literal", NewTypeLiteral(TInteger), "Integer"},
		{"list", NewList([]Value{NewInteger(1), NewInteger(2)}), "1"},
		{"map", NewMap(m), "k"},
		{"error", NewError(errors.New("boom")), "boom"},
		{"pathon", func() Value { out, _ := MakePathon(NewString("a/b"), true); return out[0] }(), "/a/b"},
		{"big-integer", NewBigInteger(big.NewInt(12345)), "12345"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := c.v.String()
			if !strings.Contains(got, c.want) {
				t.Errorf("String() = %q, want substring %q", got, c.want)
			}
		})
	}
}

func TestFormatForPrintTable(t *testing.T) {
	m := NewOrderedMap()
	m.Set("a", NewInteger(1))
	m.Set("s", NewString("x"))
	cases := []struct {
		name string
		v    Value
		want string
	}{
		{"none", NewNone(), "none"},
		{"none-type", NewTypeLiteral(TNone), "None"},
		{"type-literal", NewTypeLiteral(TString), "String"},
		{"string-unquoted", NewString("plain"), "plain"},
		{"integer", NewInteger(9), "9"},
		{"map-json", NewMap(m), `"a": 1`},
		{"list-json", NewList([]Value{NewInteger(1), NewString("z")}), `[1, "z"]`},
		{"error", NewError(errors.New("oops")), "oops"},
		{"nested", NewList([]Value{NewMap(m)}), `"s": "x"`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := FormatForPrint(c.v)
			if !strings.Contains(got, c.want) {
				t.Errorf("FormatForPrint = %q, want substring %q", got, c.want)
			}
		})
	}
}

func TestFormatValueJSONQuotesStrings(t *testing.T) {
	if got := FormatValueJSON(NewString("s")); got != `"s"` {
		t.Errorf("string JSON = %q", got)
	}
	if got := FormatValueJSON(NewInteger(3)); got != "3" {
		t.Errorf("int JSON = %q", got)
	}
	if got := FormatValueJSON(NewBoolean(true)); got != "true" {
		t.Errorf("bool JSON = %q", got)
	}
	// Nested compounds recurse.
	inner := NewOrderedMap()
	inner.Set("k", NewList([]Value{NewInteger(1)}))
	got := FormatValueJSON(NewMap(inner))
	if !strings.Contains(got, `"k": [1]`) {
		t.Errorf("nested JSON = %q", got)
	}
}

func TestFormatTableAndPadRight(t *testing.T) {
	if got := PadRight("ab", 5); got != "ab   " {
		t.Errorf("PadRight = %q", got)
	}
	if got := PadRight("abcdef", 3); got != "abcdef" {
		t.Errorf("PadRight over-width = %q", got)
	}

	rec := intStrRecord()
	row := NewOrderedMap()
	row.Set("a", NewInteger(1))
	row.Set("b", NewString("x"))
	td := TableData{Record: rec, Rows: []Value{NewMap(row)}}
	tbl := NewValueRaw(TTable, td)
	got := FormatForPrint(tbl)
	if !strings.Contains(got, "a") || !strings.Contains(got, "x") {
		t.Errorf("table render = %q, want headers and cells", got)
	}
	// Empty table renders headers only, without panicking.
	empty := NewValueRaw(TTable, TableData{Record: rec})
	if got := FormatForPrint(empty); got == "" {
		t.Error("empty table rendered as empty string")
	}
}

func TestPrintHandlers(t *testing.T) {
	var buf strings.Builder
	r := newTestRegistry(t)
	r.Output = &buf
	if _, err := PrintHandler([]Value{NewString("hello")}, nil, nil, r); err != nil {
		t.Fatalf("PrintHandler: %v", err)
	}
	if !strings.Contains(buf.String(), "hello") {
		t.Errorf("print output = %q", buf.String())
	}
	buf.Reset()
	if _, err := PrintstrHandler([]Value{NewString("raw")}, nil, nil, r); err != nil {
		t.Fatalf("PrintstrHandler: %v", err)
	}
	if !strings.Contains(buf.String(), "raw") {
		t.Errorf("printstr output = %q", buf.String())
	}
}

// --- accessor round-trips with negatives -----------------------------------

func TestBigNumberAccessors(t *testing.T) {
	bi := NewBigInteger(big.NewInt(99))
	n, err := AsBigInteger(bi)
	if err != nil || n.Int64() != 99 {
		t.Errorf("AsBigInteger = %v, %v", n, err)
	}
	if _, err := AsBigInteger(NewInteger(1)); err == nil {
		t.Error("AsBigInteger(1) should error")
	}

	d := apd.New(25, -1) // 2.5
	bd := NewBigDecimal(d)
	dv, err := AsBigDecimal(bd)
	if err != nil {
		t.Fatalf("AsBigDecimal: %v", err)
	}
	if dv.String() != d.String() {
		t.Errorf("AsBigDecimal = %v", dv)
	}
	if _, err := AsBigDecimal(NewFloat(2.5)); err == nil {
		t.Error("AsBigDecimal(2.5 float) should error")
	}

	if s := FormatBigInteger(big.NewInt(7)); !strings.Contains(s, "7") {
		t.Errorf("FormatBigInteger = %q", s)
	}
	if s := FormatBigDecimal(d); !strings.Contains(s, "2.5") {
		t.Errorf("FormatBigDecimal = %q", s)
	}

	// AsFloatApprox spans int, float, and the big leaves.
	for _, c := range []struct {
		v    Value
		want float64
	}{
		{NewInteger(2), 2},
		{NewFloat(1.5), 1.5},
		{bi, 99},
	} {
		f, err := AsFloatApprox(c.v)
		if err != nil {
			t.Errorf("AsFloatApprox(%v): %v", c.v, err)
			continue
		}
		if f != c.want {
			t.Errorf("AsFloatApprox(%v) = %v, want %v", c.v, f, c.want)
		}
	}
	if _, err := AsFloatApprox(NewString("x")); err == nil {
		t.Error("AsFloatApprox(string) should error")
	}
}

func TestMarkMoveAccessors(t *testing.T) {
	mk := NewMark("m9", NewInteger(1))
	info, err := AsMark(mk)
	if err != nil || info.ID != "m9" || len(info.Body) != 1 {
		t.Errorf("AsMark = %+v, %v", info, err)
	}
	if _, err := AsMark(NewInteger(1)); err == nil {
		t.Error("AsMark(1) should error")
	}

	mv := NewMove("m9", "test")
	mi, err := AsMove(mv)
	if err != nil || mi.To != "m9" || mi.Reason != "test" {
		t.Errorf("AsMove = %+v, %v", mi, err)
	}
	if _, err := AsMove(NewInteger(1)); err == nil {
		t.Error("AsMove(1) should error")
	}

	// Continuation-carrying moves keep their payloads.
	fc := &ForCont{}
	mvc := NewMoveCont("m9", "iter", fc)
	mic, _ := AsMove(mvc)
	if mic.Cont != fc {
		t.Error("NewMoveCont lost the ForCont")
	}
	ic := &IfCont{}
	mvi := NewMoveIf("m9", "if", ic)
	mii, _ := AsMove(mvi)
	if mii.IfCont != ic {
		t.Error("NewMoveIf lost the IfCont")
	}
}

func TestErrorValueAccessors(t *testing.T) {
	// Plain Go error: message only, no code.
	ev := NewError(errors.New("plain failure"))
	info, err := AsError(ev)
	if err != nil {
		t.Fatalf("AsError: %v", err)
	}
	if info.Message != "plain failure" || info.Code != "" {
		t.Errorf("plain error info = %+v", info)
	}
	// BoruError: code preserved for dispatch.
	ae := MakeBoruError("type_error", "typed failure", "w", "", "")
	av := NewError(ae)
	ainfo, _ := AsError(av)
	if ainfo.Code != "type_error" {
		t.Errorf("BoruError code lost: %+v", ainfo)
	}
	if !strings.Contains(ainfo.Message, "typed failure") {
		t.Errorf("BoruError message lost: %+v", ainfo)
	}
	// Negative.
	if _, err := AsError(NewInteger(1)); err == nil {
		t.Error("AsError(1) should error")
	}
	if !IsError(av) || IsError(NewInteger(1)) {
		t.Error("IsError misclassifies")
	}
}

func TestSpliceAndDispatchMod(t *testing.T) {
	sp := NewSplice(NewList([]Value{NewInteger(1)}))
	if !IsSplice(sp) {
		t.Error("NewSplice not recognised by IsSplice")
	}
	if IsSplice(NewInteger(1)) {
		t.Error("IsSplice(1) = true")
	}

	dm := NewDispatchMod(DispatchModInfo{Ref: true})
	got, ok := AsDispatchMod(dm)
	if !ok || !got.Ref || got.Quote {
		t.Errorf("AsDispatchMod = %+v, %v", got, ok)
	}
	if _, ok := AsDispatchMod(NewInteger(1)); ok {
		t.Error("AsDispatchMod(1) should refuse")
	}
}

func TestTypedListAndMapConstructors(t *testing.T) {
	child := NewTypeLiteral(TInteger)

	tl := NewTypedList(child)
	if !IsTypedList(tl) {
		t.Error("NewTypedList not a typed list")
	}
	tle := NewTypedListWithElements(child, []Value{NewInteger(1), NewInteger(2)})
	ci, err := AsChildType(tle)
	if err != nil {
		t.Fatalf("AsChildType: %v", err)
	}
	if len(ci.Elements) != 2 {
		t.Errorf("typed list elements = %d, want 2", len(ci.Elements))
	}

	tm := NewTypedMap(child)
	if !IsTypedMap(tm) {
		t.Error("NewTypedMap not a typed map")
	}
	tme := NewTypedMapWithEntries(child, []ChildEntry{{Key: "k", Value: NewInteger(3)}})
	mi, err := AsChildType(tme)
	if err != nil {
		t.Fatalf("AsChildType map: %v", err)
	}
	if len(mi.Entries) != 1 || mi.Entries[0].Key != "k" {
		t.Errorf("typed map entries = %+v", mi.Entries)
	}

	// Negatives.
	if IsTypedList(NewList(nil)) {
		t.Error("plain list misread as typed list")
	}
	if IsTypedMap(NewMap(NewOrderedMap())) {
		t.Error("plain map misread as typed map")
	}
	if _, err := AsChildType(NewInteger(1)); err == nil {
		t.Error("AsChildType(1) should error")
	}
}

func TestEvalAndImplicitContainers(t *testing.T) {
	el := NewEvalList([]Value{NewInteger(1)})
	if !el.Eval {
		t.Error("NewEvalList did not set Eval")
	}
	m := NewOrderedMap()
	m.Set("a", NewInteger(1))
	em := NewEvalMap(m)
	if !em.Eval {
		t.Error("NewEvalMap did not set Eval")
	}
	im := NewImplicitMap(m)
	if !IsImplicitMap(im) {
		t.Error("NewImplicitMap not implicit")
	}
	if IsImplicitMap(NewMap(NewOrderedMap())) {
		t.Error("plain map misread as implicit")
	}
	fm := NewFlexMap(m)
	if !IsFlexMap(fm) {
		t.Error("NewFlexMap not recognised")
	}
	if IsFlexMap(NewMap(m)) {
		t.Error("plain map misread as flex map")
	}
}

func TestAtomReferent(t *testing.T) {
	a := NewAtom("name")
	if _, ok := AtomReferent(a); ok {
		t.Error("fresh atom should have no referent")
	}
	withRef := SetAtomReferent(a, NewInteger(42))
	ref, ok := AtomReferent(withRef)
	if !ok {
		t.Fatal("referent lost")
	}
	if got, _ := AsInteger(ref); got != 42 {
		t.Errorf("referent = %v", ref)
	}
	// Non-atoms have no referent.
	if _, ok := AtomReferent(NewInteger(1)); ok {
		t.Error("integer claims an atom referent")
	}
}

func TestTypeNameAndPathOf(t *testing.T) {
	if got := TypeNameOf(NewInteger(1)); got != "Integer" {
		t.Errorf("TypeNameOf(1) = %q", got)
	}
	if got := TypePathOf(NewInteger(1)); !strings.Contains(got, "Integer") {
		t.Errorf("TypePathOf(1) = %q", got)
	}
	// A concrete string's leaf is the ProperString node (String's
	// concrete child), not the String family root.
	if got := TypeNameOf(NewString("s")); got != "ProperString" {
		t.Errorf("TypeNameOf(\"s\") = %q, want ProperString leaf", got)
	}
}

func TestEnumAndFnUndef(t *testing.T) {
	e := NewEnum([]Value{NewAtom("red"), NewAtom("blue")})
	if !IsDisjunct(e) {
		t.Error("enum should be a disjunct value")
	}
	di, err := AsDisjunct(e)
	if err != nil || len(di.Alternatives) != 2 {
		t.Errorf("AsDisjunct(enum) = %+v, %v", di, err)
	}

	fu := NewFnUndef(FnUndefInfo{Sigs: []FnSigSpec{{}}})
	if !fu.Parent.Equal(TFnUndef) {
		t.Errorf("NewFnUndef parent = %v", fu.Parent)
	}
}

func TestClassAndResourceTypeValues(t *testing.T) {
	ct := testClassType()
	cv := NewClassType(TClass, ct)
	got, err := AsClassType(cv)
	if err != nil || got.Name != ct.Name {
		t.Errorf("AsClassType = %+v, %v", got, err)
	}
	if _, err := AsClassType(NewInteger(1)); err == nil {
		t.Error("AsClassType(1) should error")
	}

	fields := NewOrderedMap()
	fields.Set("id", NewTypeLiteral(TInteger))
	rt := ResourceTypeInfo{Fields: fields, Name: "Ideal/Resource/RVal"}
	rv := NewResourceType(TResource, rt)
	if !IsResourceType(rv) {
		t.Error("NewResourceType not recognised")
	}
	back, err := AsResourceType(rv)
	if err != nil || back.Name != rt.Name {
		t.Errorf("AsResourceType = %+v, %v", back, err)
	}
}

func TestStoreWithPrototype(t *testing.T) {
	proto := &StoreInstanceInfo{Data: map[string]Value{"greet": NewString("hi")}}
	s := NewStoreWithPrototype(TStore, proto)
	si, err := AsStore(s)
	if err != nil {
		t.Fatalf("AsStore: %v", err)
	}
	if si.Prototype != proto {
		t.Error("prototype not attached")
	}
	if _, err := AsStore(NewInteger(1)); err == nil {
		t.Error("AsStore(1) should error")
	}
}

func TestTableTypeAccessors(t *testing.T) {
	tt := NewTableType(intStrRecord())
	if !IsTableType(tt) {
		t.Error("NewTableType not recognised")
	}
	info, err := AsTableType(tt)
	if err != nil || info.Record.Fields.Len() != 2 {
		t.Errorf("AsTableType = %+v, %v", info, err)
	}
	if _, err := AsTableType(NewInteger(1)); err == nil {
		t.Error("AsTableType(1) should error")
	}
	if IsTableType(NewInteger(1)) {
		t.Error("IsTableType(1) = true")
	}
}

func TestRecordTypeAccessors(t *testing.T) {
	rv := NewRecordType(intStrRecord().Fields)
	if !IsRecordType(rv) {
		t.Error("NewRecordType not recognised")
	}
	ri, err := AsRecordType(rv)
	if err != nil || ri.Fields.Len() != 2 {
		t.Errorf("AsRecordType = %+v, %v", ri, err)
	}
	if _, err := AsRecordType(NewInteger(1)); err == nil {
		t.Error("AsRecordType(1) should error")
	}
}

func TestWordUsurpConstructor(t *testing.T) {
	w := NewWordUsurp("target", true)
	wi, err := AsWord(w)
	if err != nil {
		t.Fatalf("AsWord: %v", err)
	}
	if !wi.ForceUsurp || !wi.ForceRef || wi.Name != "target" {
		t.Errorf("usurp word info = %+v", wi)
	}
	plain := NewWordUsurp("other", false)
	pi, _ := AsWord(plain)
	if pi.ForceRef {
		t.Error("non-ref usurp gained ForceRef")
	}
}

func TestMapFieldHelpers(t *testing.T) {
	m := NewOrderedMap()
	m.Set("s", NewString("v"))
	m.Set("i", NewInteger(3))
	m.Set("b", NewBoolean(true))
	m.Set("f", NewFloat(1.5))
	mv := NewMap(m)
	rm, err := AsMap(mv)
	if err != nil {
		t.Fatalf("AsMap: %v", err)
	}
	if s, ok := MapFieldString(rm, "s"); !ok || s != "v" {
		t.Errorf("MapFieldString = %q, %v", s, ok)
	}
	if n, ok := MapFieldInteger(rm, "i"); !ok || n != 3 {
		t.Errorf("MapFieldInteger = %d, %v", n, ok)
	}
	if b, ok := MapFieldBoolean(rm, "b"); !ok || !b {
		t.Errorf("MapFieldBoolean = %v, %v", b, ok)
	}
	if f, ok := MapFieldFloat(rm, "f"); !ok || f != 1.5 {
		t.Errorf("MapFieldFloat = %v, %v", f, ok)
	}
	// Negatives: absent key and wrong kind.
	if _, ok := MapFieldString(rm, "missing"); ok {
		t.Error("missing key found")
	}
	if _, ok := MapFieldInteger(rm, "s"); ok {
		t.Error("string field read as integer")
	}
}

func TestConcreteScalarAccessors(t *testing.T) {
	if s, err := NewString("x").AsConcreteString(); err != nil || s != "x" {
		t.Errorf("AsConcreteString = %q, %v", s, err)
	}
	if _, err := NewTypeLiteral(TString).AsConcreteString(); err == nil {
		t.Error("bare String literal read as concrete string")
	}
	if f, err := NewFloat(1.25).AsConcreteFloat(); err != nil || f != 1.25 {
		t.Errorf("AsConcreteFloat = %v, %v", f, err)
	}
	if _, err := NewTypeLiteral(TFloat).AsConcreteFloat(); err == nil {
		t.Error("bare Float literal read as concrete float")
	}
	if b, err := NewBoolean(true).AsConcreteBoolean(); err != nil || !b {
		t.Errorf("AsConcreteBoolean = %v, %v", b, err)
	}
	if _, err := NewTypeLiteral(TBoolean).AsConcreteBoolean(); err == nil {
		t.Error("bare Boolean literal read as concrete boolean")
	}
}

func TestReadListGetOkAndOrderedMapExtras(t *testing.T) {
	rl := NewReadList([]Value{NewInteger(1)})
	if v, ok := rl.GetOk(0); !ok || v.String() != "1" {
		t.Errorf("GetOk(0) = %v, %v", v, ok)
	}
	if _, ok := rl.GetOk(5); ok {
		t.Error("GetOk out of range returned ok")
	}
	if _, ok := rl.GetOk(-1); ok {
		t.Error("GetOk(-1) returned ok")
	}

	m := NewOrderedMap()
	m.Set("b", NewInteger(2))
	m.Set("a", NewInteger(1))
	keys := m.SortedKeys()
	if len(keys) != 2 || keys[0] != "a" {
		t.Errorf("SortedKeys = %v", keys)
	}
	m.Delete("a")
	if _, ok := m.Get("a"); ok {
		t.Error("Delete left the key behind")
	}
	if m.Len() != 1 {
		t.Errorf("Len after delete = %d", m.Len())
	}
}

func TestIsTypeValue(t *testing.T) {
	if !IsTypeValue(NewTypeLiteral(TInteger)) {
		t.Error("Integer literal should be a type value")
	}
	if !IsTypeValue(NewRecordType(intStrRecord().Fields)) {
		t.Error("record type should be a type value")
	}
	if IsTypeValue(NewInteger(1)) {
		t.Error("concrete integer misread as a type value")
	}
}

func TestNextMarkIDAndObjectTypeID(t *testing.T) {
	a, b := NextMarkID(), NextMarkID()
	if a == b {
		t.Errorf("NextMarkID not unique: %q vs %q", a, b)
	}
	x, y := GenerateObjectTypeID(), GenerateObjectTypeID()
	if x == y {
		t.Errorf("GenerateObjectTypeID not unique: %q vs %q", x, y)
	}
	if !strings.HasPrefix(x, "T_") {
		t.Errorf("object type ID %q missing T_ prefix", x)
	}
}

func TestFnDefOwnSigsAndFields(t *testing.T) {
	fd := FnDefInfo{Name: "f", Signatures: []Signature{
		{Params: []FnParam{{Name: "n", Type: TInteger}}, Returns: []*Type{TInteger}},
		{Fallback: true},
	}}
	own := fd.OwnSigs()
	for i := range own {
		if own[i].Fallback {
			t.Error("OwnSigs kept the synthesized fallback")
		}
	}
	first, ok := fd.FirstOwnSig()
	if !ok || first == nil || len(first.Params) != 1 {
		t.Errorf("FirstOwnSig = %+v, %v", first, ok)
	}
	empty := FnDefInfo{}
	if _, ok := empty.FirstOwnSig(); ok {
		t.Error("FirstOwnSig on empty def should report false")
	}
}
