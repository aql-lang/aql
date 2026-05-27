package test

import (
	"testing"

	"github.com/aql-lang/aql/eng/go/parser"
	"github.com/aql-lang/aql/lang/go/native"
)

// --- merge ---

func TestMergeMaps(t *testing.T) {
	result, err := runNativeSteps(t, nil, []string{
		`merge {a:1 b:2} {b:3 c:4}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	m, _ := native.AsMap(result[0])
	a, _ := m.Get("a")
	b, _ := m.Get("b")
	c, _ := m.Get("c")
	ai1, _ := native.AsInteger(a)
	bi1, _ := native.AsInteger(b)
	ci1, _ := native.AsInteger(c)
	if ai1 != 1 {
		t.Errorf("expected a=1, got %d", ai1)
	}
	if bi1 != 3 {
		t.Errorf("expected b=3 (overridden), got %d", bi1)
	}
	if ci1 != 4 {
		t.Errorf("expected c=4, got %d", ci1)
	}
}

func TestMergeNested(t *testing.T) {
	result, err := runNativeSteps(t, nil, []string{
		`merge {a:{x:1 y:2}} {a:{y:3 z:4}}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	m, _ := native.AsMap(result[0])
	inner, _ := m.Get("a")
	im, _ := native.AsMap(inner)
	x, _ := im.Get("x")
	y, _ := im.Get("y")
	z, _ := im.Get("z")
	xi1, _ := native.AsInteger(x)
	yi1, _ := native.AsInteger(y)
	zi1, _ := native.AsInteger(z)
	if xi1 != 1 {
		t.Errorf("expected x=1, got %d", xi1)
	}
	if yi1 != 3 {
		t.Errorf("expected y=3 (overridden), got %d", yi1)
	}
	if zi1 != 4 {
		t.Errorf("expected z=4, got %d", zi1)
	}
}

// --- merge list+map ---

func TestMergeListMap(t *testing.T) {
	result, err := runNativeSteps(t, nil, []string{
		`["a","b","c"] merge {1:"d"}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	_lst, _ := native.AsList(result[0])
	list := _lst.Slice()
	if len(list) != 3 {
		t.Fatalf("expected 3 elements, got %d", len(list))
	}
	l0s, _ := native.AsString(list[0])
	l1s, _ := native.AsString(list[1])
	l2s, _ := native.AsString(list[2])
	if l0s != "a" {
		t.Errorf("expected [0]=a, got %s", l0s)
	}
	if l1s != "d" {
		t.Errorf("expected [1]=d, got %s", l1s)
	}
	if l2s != "c" {
		t.Errorf("expected [2]=c, got %s", l2s)
	}
}

func TestMergeMapList(t *testing.T) {
	result, err := runNativeSteps(t, nil, []string{
		`{3:"d"} merge ["a","b","c"]`,
	})
	if err != nil {
		t.Fatal(err)
	}
	_lst, _ := native.AsList(result[0])
	list := _lst.Slice()
	if len(list) != 4 {
		t.Fatalf("expected 4 elements, got %d", len(list))
	}
	l3s, _ := native.AsString(list[3])
	if l3s != "d" {
		t.Errorf("expected [3]=d, got %s", l3s)
	}
}

func TestMergeMapListIgnoreNonInt(t *testing.T) {
	result, err := runNativeSteps(t, nil, []string{
		`{x:"X",y:"Y"} merge ["a","b"]`,
	})
	if err != nil {
		t.Fatal(err)
	}
	_lst, _ := native.AsList(result[0])
	list := _lst.Slice()
	if len(list) != 2 {
		t.Fatalf("expected 2 elements, got %d", len(list))
	}
}

// --- push/pop/shift/unshift ---

func TestPush(t *testing.T) {
	result, err := runNativeSteps(t, nil, []string{
		`push "c" ["a","b"]`,
	})
	if err != nil {
		t.Fatal(err)
	}
	_lst, _ := native.AsList(result[0])
	list := _lst.Slice()
	l2push, _ := native.AsString(list[2])
	if len(list) != 3 || l2push != "c" {
		t.Errorf("expected [a,b,c], got %v", result[0].String())
	}
}

func TestPushSingleElement(t *testing.T) {
	result, err := runNativeSteps(t, nil, []string{
		`push ["c","d"] ["a","b"]`,
	})
	if err != nil {
		t.Fatal(err)
	}
	_lst, _ := native.AsList(result[0])
	list := _lst.Slice()
	if len(list) != 3 {
		t.Errorf("expected 3 elements (list added as single element), got %d", len(list))
	}
}

func TestPop(t *testing.T) {
	result, err := runNativeSteps(t, nil, []string{
		`pop ["a","b","c"]`,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 2 {
		t.Fatalf("expected 2 results (list + popped), got %d", len(result))
	}
	_lst, _ := native.AsList(result[0])
	list := _lst.Slice()
	if len(list) != 2 {
		t.Errorf("expected list of 2, got %d", len(list))
	}
	r1pop, _ := native.AsString(result[1])
	if r1pop != "c" {
		t.Errorf("expected popped 'c', got %s", r1pop)
	}
}

func TestUnshift(t *testing.T) {
	result, err := runNativeSteps(t, nil, []string{
		`unshift "c" ["a","b"]`,
	})
	if err != nil {
		t.Fatal(err)
	}
	_lst, _ := native.AsList(result[0])
	list := _lst.Slice()
	l0unshift, _ := native.AsString(list[0])
	if len(list) != 3 || l0unshift != "c" {
		t.Errorf("expected [c,a,b], got %v", result[0].String())
	}
}

func TestUnshiftSingleElement(t *testing.T) {
	result, err := runNativeSteps(t, nil, []string{
		`unshift ["c","d"] ["a","b"]`,
	})
	if err != nil {
		t.Fatal(err)
	}
	_lst, _ := native.AsList(result[0])
	list := _lst.Slice()
	if len(list) != 3 {
		t.Errorf("expected 3 elements (list added as single element), got %d", len(list))
	}
}

func TestShift(t *testing.T) {
	result, err := runNativeSteps(t, nil, []string{
		`shift ["a","b","c"]`,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 2 {
		t.Fatalf("expected 2 results, got %d", len(result))
	}
	_lst, _ := native.AsList(result[0])
	list := _lst.Slice()
	if len(list) != 2 {
		t.Errorf("expected list of 2, got %d", len(list))
	}
	r1shift, _ := native.AsString(result[1])
	if r1shift != "a" {
		t.Errorf("expected shifted 'a', got %s", r1shift)
	}
}

// --- getpath ---

func TestGetpathSimple(t *testing.T) {
	result, err := runNativeSteps(t, nil, []string{
		`getpath "a.b" {a:{b:42}}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	r0i1, _ := native.AsInteger(result[0])
	if r0i1 != 42 {
		t.Errorf("expected 42, got %d", r0i1)
	}
}

func TestGetpathTopLevel(t *testing.T) {
	result, err := runNativeSteps(t, nil, []string{
		`getpath "name" {name:"Alice"}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	r0s1, _ := native.AsString(result[0])
	if r0s1 != "Alice" {
		t.Errorf("expected Alice, got %s", r0s1)
	}
}

// --- setpath ---

func TestSetpathSimple(t *testing.T) {
	result, err := runNativeSteps(t, nil, []string{
		`setpath {a:1} "b" 99`,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	m, _ := native.AsMap(result[0])
	a, _ := m.Get("a")
	b, _ := m.Get("b")
	ai2, _ := native.AsInteger(a)
	bi2, _ := native.AsInteger(b)
	if ai2 != 1 {
		t.Errorf("expected a=1, got %d", ai2)
	}
	if bi2 != 99 {
		t.Errorf("expected b=99, got %d", bi2)
	}
}

func TestSetpathNewKey(t *testing.T) {
	result, err := runNativeSteps(t, nil, []string{
		`setpath {a:1} "b" 2`,
	})
	if err != nil {
		t.Fatal(err)
	}
	m, _ := native.AsMap(result[0])
	a, _ := m.Get("a")
	b, _ := m.Get("b")
	ai5, _ := native.AsInteger(a)
	bi5, _ := native.AsInteger(b)
	if ai5 != 1 {
		t.Errorf("expected a=1, got %d", ai5)
	}
	if bi5 != 2 {
		t.Errorf("expected b=2, got %d", bi5)
	}
}

// --- clone ---

func TestCloneMap(t *testing.T) {
	result, err := runNativeSteps(t, nil, []string{
		`{a:1 b:2} clone`,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	m, _ := native.AsMap(result[0])
	a, _ := m.Get("a")
	b, _ := m.Get("b")
	ai3, _ := native.AsInteger(a)
	bi3, _ := native.AsInteger(b)
	if ai3 != 1 {
		t.Errorf("expected a=1, got %d", ai3)
	}
	if bi3 != 2 {
		t.Errorf("expected b=2, got %d", bi3)
	}
}

// --- inject ---

func TestInjectPaths(t *testing.T) {
	bt := string(rune(96)) // backtick character
	input := `inject {greeting:"` + bt + `name` + bt + `"} {name:"Alice"}`
	result, err := runNativeSteps(t, nil, []string{input})
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	m, _ := native.AsMap(result[0])
	v, ok := m.Get("greeting")
	if !ok {
		t.Fatal("expected key 'greeting' in result")
	}
	vs2, _ := native.AsString(v)
	if vs2 != "Alice" {
		t.Errorf("expected Alice, got %s", vs2)
	}
}

// --- validate ---

func TestValidateReturnsSpec(t *testing.T) {
	result, err := runNativeSteps(t, nil, []string{
		`validate {name:"$STRING" age:"$NUMBER"} {name:"Alice" age:30}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	m, _ := native.AsMap(result[0])
	name, ok := m.Get("name")
	if !ok {
		t.Fatal("expected key 'name' in result")
	}
	nameS, _ := native.AsString(name)
	if nameS != "$STRING" {
		t.Errorf("expected $STRING, got %s", nameS)
	}
	age, ok := m.Get("age")
	if !ok {
		t.Fatal("expected key 'age' in result")
	}
	ageS, _ := native.AsString(age)
	if ageS != "$NUMBER" {
		t.Errorf("expected $NUMBER, got %s", ageS)
	}
}

// --- walk ---

func TestWalkFlat(t *testing.T) {
	result, err := runNativeSteps(t, nil, []string{
		`{a:1 b:"hello"} walk`,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	_lst, _ := native.AsList(result[0])
	leaves := _lst.Slice()
	if len(leaves) != 2 {
		t.Fatalf("expected 2 leaves, got %d", len(leaves))
	}

	paths := make(map[string]string)
	for _, leaf := range leaves {
		m, _ := native.AsMap(leaf)
		p, _ := m.Get("path")
		v, _ := m.Get("value")
		ps1, _ := native.AsString(p)
		paths[ps1] = v.String()
	}
	if _, ok := paths["a"]; !ok {
		t.Error("missing path 'a'")
	}
	if _, ok := paths["b"]; !ok {
		t.Error("missing path 'b'")
	}
}

func TestWalkNested(t *testing.T) {
	result, err := runNativeSteps(t, nil, []string{
		`{a:{x:1 y:2} b:3} walk`,
	})
	if err != nil {
		t.Fatal(err)
	}
	_lst, _ := native.AsList(result[0])
	leaves := _lst.Slice()
	if len(leaves) != 3 {
		t.Fatalf("expected 3 leaves, got %d", len(leaves))
	}

	paths := make(map[string]bool)
	for _, leaf := range leaves {
		m, _ := native.AsMap(leaf)
		p, _ := m.Get("path")
		ps2, _ := native.AsString(p)
		paths[ps2] = true
	}
	for _, want := range []string{"a.x", "a.y", "b"} {
		if !paths[want] {
			t.Errorf("missing path %q", want)
		}
	}
}

// --- walk with before callback ---

func TestWalkBeforeIdentity(t *testing.T) {
	// Before callback returns m.value (identity) — tree is preserved unchanged.
	// walk is stack-only [TAny, TFunction] so it needs both values on the
	// stack. We pass the fn as a Go-constructed TFunction value directly
	// in the engine stack to prevent auto-execution.
	fnDef := native.FnDefInfo{
		Sigs: []native.FnSig{{
			Params:  []native.FnParam{{Name: "m", Type: native.TMap}},
			Returns: []*native.Type{native.TAny},
			Body:    []native.Value{native.NewWord("m"), native.NewWord("get"), native.NewWord("value")}, BarrierPos: -1,
		}},
	}
	om := native.NewOrderedMap()
	om.Set("a", native.NewInteger(1))
	om.Set("b", native.NewInteger(2))

	reg, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	reg.SetParseFunc(parser.Parse)
	native.Register(reg)

	// Push the fn as a Quoted function value so it doesn't auto-execute
	// before walk can consume it from the stack.
	fnVal := native.NewFunction(fnDef)
	fnVal.Quoted = true

	eng := native.NewTop(reg)
	result, err := eng.Run([]native.Value{
		native.NewMap(om), fnVal, native.NewWord("walk"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	m, _ := native.AsMap(result[0])
	a, _ := m.Get("a")
	b, _ := m.Get("b")
	ai6, _ := native.AsInteger(a)
	bi6, _ := native.AsInteger(b)
	if ai6 != 1 {
		t.Errorf("expected a=1, got %v", a)
	}
	if bi6 != 2 {
		t.Errorf("expected b=2, got %v", b)
	}
}

func TestWalkBeforeIdentityNested(t *testing.T) {
	// Identity before callback on a nested structure — entire tree preserved.
	// walk is stack-only [TAny, TFunction], so we push the fn as a Quoted
	// value to prevent auto-execution before walk consumes it.
	fnDef := native.FnDefInfo{
		Sigs: []native.FnSig{{
			Params:  []native.FnParam{{Name: "m", Type: native.TMap}},
			Returns: []*native.Type{native.TAny},
			Body:    []native.Value{native.NewWord("m"), native.NewWord("get"), native.NewWord("value")}, BarrierPos: -1,
		}},
	}
	inner := native.NewOrderedMap()
	inner.Set("x", native.NewInteger(1))
	inner.Set("y", native.NewInteger(2))
	om := native.NewOrderedMap()
	om.Set("a", native.NewMap(inner))
	om.Set("b", native.NewInteger(3))

	reg, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	reg.SetParseFunc(parser.Parse)
	native.Register(reg)

	fnVal := native.NewFunction(fnDef)
	fnVal.Quoted = true

	eng := native.NewTop(reg)
	result, err := eng.Run([]native.Value{
		native.NewMap(om), fnVal, native.NewWord("walk"),
	})
	if err != nil {
		t.Fatal(err)
	}
	rm, _ := native.AsMap(result[0])
	aVal, _ := rm.Get("a")
	aim, _ := native.AsMap(aVal)
	x, _ := aim.Get("x")
	y, _ := aim.Get("y")
	b, _ := rm.Get("b")
	xi2, _ := native.AsInteger(x)
	yi2, _ := native.AsInteger(y)
	bi7, _ := native.AsInteger(b)
	if xi2 != 1 {
		t.Errorf("expected x=1, got %v", x)
	}
	if yi2 != 2 {
		t.Errorf("expected y=2, got %v", y)
	}
	if bi7 != 3 {
		t.Errorf("expected b=3, got %v", b)
	}
}

func TestWalkBeforeReplace(t *testing.T) {
	// AQL: {a:1 b:2} (fn [[m:Map] [Any] [99]]) walk
	// Before callback replaces the root node with 99 (a non-node value).
	// Since 99 is not a map/list, walk does NOT descend into children.
	// This demonstrates that the before callback controls traversal:
	// replacing a node with a scalar stops descent into that subtree.
	fnDef := native.FnDefInfo{
		Sigs: []native.FnSig{{
			Params:  []native.FnParam{{Name: "m", Type: native.TMap}},
			Returns: []*native.Type{native.TAny},
			Body:    []native.Value{native.NewInteger(99)}, BarrierPos: -1,
		}},
	}
	om := native.NewOrderedMap()
	om.Set("a", native.NewInteger(1))
	om.Set("b", native.NewInteger(2))

	reg, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	reg.SetParseFunc(parser.Parse)
	native.Register(reg)

	fnVal := native.NewFunction(fnDef)
	fnVal.Quoted = true

	eng := native.NewTop(reg)
	result, err := eng.Run([]native.Value{
		native.NewMap(om), fnVal, native.NewWord("walk"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	r0i2, _ := native.AsInteger(result[0])
	if r0i2 != 99 {
		t.Errorf("expected 99, got %v", result[0])
	}
}

func TestWalkBeforeReturnPath(t *testing.T) {
	// AQL: {a:1 b:2} (fn [[m:Map] [Any] [m.path]]) walk
	// Before callback returns the path string for every node.
	// The root path is "" (empty string), which replaces the root map.
	// Since a string is not a node, descent stops — result is "".
	fnDef := native.FnDefInfo{
		Sigs: []native.FnSig{{
			Params:  []native.FnParam{{Name: "m", Type: native.TMap}},
			Returns: []*native.Type{native.TAny},
			Body:    []native.Value{native.NewWord("m"), native.NewWord("get"), native.NewWord("path")}, BarrierPos: -1,
		}},
	}
	om := native.NewOrderedMap()
	om.Set("a", native.NewInteger(1))
	om.Set("b", native.NewInteger(2))

	reg, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	reg.SetParseFunc(parser.Parse)
	native.Register(reg)

	fnVal := native.NewFunction(fnDef)
	fnVal.Quoted = true

	eng := native.NewTop(reg)
	result, err := eng.Run([]native.Value{
		native.NewMap(om), fnVal, native.NewWord("walk"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	r0s2, _ := native.AsString(result[0])
	if r0s2 != "" {
		t.Errorf("expected empty string (root path), got %q", r0s2)
	}
}

// --- walk with before AND after callbacks ---

func TestWalkBeforeAfterIdentity(t *testing.T) {
	// AQL: {a:1 b:2} (fn [[m:Map] [Any] [m.value]]) (fn [[m:Map] [Any] [m.value]]) walk
	// Both before and after return m.value (identity) — tree is preserved.
	identityBody := []native.Value{native.NewWord("m"), native.NewWord("get"), native.NewWord("value")}
	fnDef1 := native.FnDefInfo{
		Sigs: []native.FnSig{{
			Params:  []native.FnParam{{Name: "m", Type: native.TMap}},
			Returns: []*native.Type{native.TAny},
			Body:    identityBody, BarrierPos: -1,
		}},
	}
	fnDef2 := native.FnDefInfo{
		Sigs: []native.FnSig{{
			Params:  []native.FnParam{{Name: "m", Type: native.TMap}},
			Returns: []*native.Type{native.TAny},
			Body:    identityBody, BarrierPos: -1,
		}},
	}
	om := native.NewOrderedMap()
	om.Set("a", native.NewInteger(1))
	om.Set("b", native.NewInteger(2))

	reg, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	reg.SetParseFunc(parser.Parse)
	native.Register(reg)

	fnVal1 := native.NewFunction(fnDef1)
	fnVal1.Quoted = true
	fnVal2 := native.NewFunction(fnDef2)
	fnVal2.Quoted = true

	eng := native.NewTop(reg)
	result, err := eng.Run([]native.Value{
		native.NewMap(om), fnVal1, fnVal2, native.NewWord("walk"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	m, _ := native.AsMap(result[0])
	a, _ := m.Get("a")
	b, _ := m.Get("b")
	ai7, _ := native.AsInteger(a)
	bi8, _ := native.AsInteger(b)
	if ai7 != 1 {
		t.Errorf("expected a=1, got %v", a)
	}
	if bi8 != 2 {
		t.Errorf("expected b=2, got %v", b)
	}
}

func TestWalkBeforeAfterPostOrder(t *testing.T) {
	// AQL: {a:1 b:2} (fn [[m:Map] [Any] [m.value]]) (fn [[m:Map] [Any] [99]]) walk
	// Before callback is identity (allows descent), after callback replaces
	// every node with 99 (post-order). Processing order:
	//   1. before(root) → {a:1 b:2} (identity, descent proceeds)
	//   2. before(a=1) → 1 (identity)
	//   3. after(a=1) → 99
	//   4. before(b=2) → 2 (identity)
	//   5. after(b=2) → 99
	//   6. after(root={a:99 b:99}) → 99
	// Final result: 99
	fnDef1 := native.FnDefInfo{
		Sigs: []native.FnSig{{
			Params:  []native.FnParam{{Name: "m", Type: native.TMap}},
			Returns: []*native.Type{native.TAny},
			Body:    []native.Value{native.NewWord("m"), native.NewWord("get"), native.NewWord("value")}, BarrierPos: -1,
		}},
	}
	fnDef2 := native.FnDefInfo{
		Sigs: []native.FnSig{{
			Params:  []native.FnParam{{Name: "m", Type: native.TMap}},
			Returns: []*native.Type{native.TAny},
			Body:    []native.Value{native.NewInteger(99)}, BarrierPos: -1,
		}},
	}
	om := native.NewOrderedMap()
	om.Set("a", native.NewInteger(1))
	om.Set("b", native.NewInteger(2))

	reg, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	reg.SetParseFunc(parser.Parse)
	native.Register(reg)

	fnVal1 := native.NewFunction(fnDef1)
	fnVal1.Quoted = true
	fnVal2 := native.NewFunction(fnDef2)
	fnVal2.Quoted = true

	eng := native.NewTop(reg)
	result, err := eng.Run([]native.Value{
		native.NewMap(om), fnVal1, fnVal2, native.NewWord("walk"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	r0i3, _ := native.AsInteger(result[0])
	if r0i3 != 99 {
		t.Errorf("expected 99 (after replaces all), got %v", result[0])
	}
}

func TestWalkBeforeAfterNested(t *testing.T) {
	// AQL: {a:{x:1 y:2} b:3}
	//        (fn [[m:Map] [Any] [m.value]])
	//        (fn [[m:Map] [Any] [m.value]]) walk
	// Both callbacks are identity — nested tree preserved through full traversal.
	identityBody := []native.Value{native.NewWord("m"), native.NewWord("get"), native.NewWord("value")}
	fnDef1 := native.FnDefInfo{
		Sigs: []native.FnSig{{
			Params:  []native.FnParam{{Name: "m", Type: native.TMap}},
			Returns: []*native.Type{native.TAny},
			Body:    identityBody, BarrierPos: -1,
		}},
	}
	fnDef2 := native.FnDefInfo{
		Sigs: []native.FnSig{{
			Params:  []native.FnParam{{Name: "m", Type: native.TMap}},
			Returns: []*native.Type{native.TAny},
			Body:    identityBody, BarrierPos: -1,
		}},
	}
	innerMap := native.NewOrderedMap()
	innerMap.Set("x", native.NewInteger(1))
	innerMap.Set("y", native.NewInteger(2))
	om := native.NewOrderedMap()
	om.Set("a", native.NewMap(innerMap))
	om.Set("b", native.NewInteger(3))

	reg, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	reg.SetParseFunc(parser.Parse)
	native.Register(reg)

	fnVal1 := native.NewFunction(fnDef1)
	fnVal1.Quoted = true
	fnVal2 := native.NewFunction(fnDef2)
	fnVal2.Quoted = true

	eng := native.NewTop(reg)
	result, err := eng.Run([]native.Value{
		native.NewMap(om), fnVal1, fnVal2, native.NewWord("walk"),
	})
	if err != nil {
		t.Fatal(err)
	}
	m, _ := native.AsMap(result[0])
	inner, _ := m.Get("a")
	im, _ := native.AsMap(inner)
	x, _ := im.Get("x")
	y, _ := im.Get("y")
	b, _ := m.Get("b")
	xi3, _ := native.AsInteger(x)
	yi3, _ := native.AsInteger(y)
	bi9, _ := native.AsInteger(b)
	if xi3 != 1 {
		t.Errorf("expected x=1, got %v", x)
	}
	if yi3 != 2 {
		t.Errorf("expected y=2, got %v", y)
	}
	if bi9 != 3 {
		t.Errorf("expected b=3, got %v", b)
	}
}
