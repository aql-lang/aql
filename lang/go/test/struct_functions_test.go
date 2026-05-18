package test

import (
	"testing"

	"github.com/aql-lang/aql/eng/go/parser"
	"github.com/aql-lang/aql/lang/go/engine"
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
	m, _ := engine.AsMap(result[0])
	a, _ := m.Get("a")
	b, _ := m.Get("b")
	c, _ := m.Get("c")
	ai1, _ := engine.AsInteger(a)
	bi1, _ := engine.AsInteger(b)
	ci1, _ := engine.AsInteger(c)
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
	m, _ := engine.AsMap(result[0])
	inner, _ := m.Get("a")
	im, _ := engine.AsMap(inner)
	x, _ := im.Get("x")
	y, _ := im.Get("y")
	z, _ := im.Get("z")
	xi1, _ := engine.AsInteger(x)
	yi1, _ := engine.AsInteger(y)
	zi1, _ := engine.AsInteger(z)
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
	_lst, _ := engine.AsList(result[0])
	list := _lst.Slice()
	if len(list) != 3 {
		t.Fatalf("expected 3 elements, got %d", len(list))
	}
	l0s, _ := engine.AsString(list[0])
	l1s, _ := engine.AsString(list[1])
	l2s, _ := engine.AsString(list[2])
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
	_lst, _ := engine.AsList(result[0])
	list := _lst.Slice()
	if len(list) != 4 {
		t.Fatalf("expected 4 elements, got %d", len(list))
	}
	l3s, _ := engine.AsString(list[3])
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
	_lst, _ := engine.AsList(result[0])
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
	_lst, _ := engine.AsList(result[0])
	list := _lst.Slice()
	l2push, _ := engine.AsString(list[2])
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
	_lst, _ := engine.AsList(result[0])
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
	_lst, _ := engine.AsList(result[0])
	list := _lst.Slice()
	if len(list) != 2 {
		t.Errorf("expected list of 2, got %d", len(list))
	}
	r1pop, _ := engine.AsString(result[1])
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
	_lst, _ := engine.AsList(result[0])
	list := _lst.Slice()
	l0unshift, _ := engine.AsString(list[0])
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
	_lst, _ := engine.AsList(result[0])
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
	_lst, _ := engine.AsList(result[0])
	list := _lst.Slice()
	if len(list) != 2 {
		t.Errorf("expected list of 2, got %d", len(list))
	}
	r1shift, _ := engine.AsString(result[1])
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
	r0i1, _ := engine.AsInteger(result[0])
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
	r0s1, _ := engine.AsString(result[0])
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
	m, _ := engine.AsMap(result[0])
	a, _ := m.Get("a")
	b, _ := m.Get("b")
	ai2, _ := engine.AsInteger(a)
	bi2, _ := engine.AsInteger(b)
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
	m, _ := engine.AsMap(result[0])
	a, _ := m.Get("a")
	b, _ := m.Get("b")
	ai5, _ := engine.AsInteger(a)
	bi5, _ := engine.AsInteger(b)
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
	m, _ := engine.AsMap(result[0])
	a, _ := m.Get("a")
	b, _ := m.Get("b")
	ai3, _ := engine.AsInteger(a)
	bi3, _ := engine.AsInteger(b)
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
	m, _ := engine.AsMap(result[0])
	v, ok := m.Get("greeting")
	if !ok {
		t.Fatal("expected key 'greeting' in result")
	}
	vs2, _ := engine.AsString(v)
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
	m, _ := engine.AsMap(result[0])
	name, ok := m.Get("name")
	if !ok {
		t.Fatal("expected key 'name' in result")
	}
	nameS, _ := engine.AsString(name)
	if nameS != "$STRING" {
		t.Errorf("expected $STRING, got %s", nameS)
	}
	age, ok := m.Get("age")
	if !ok {
		t.Fatal("expected key 'age' in result")
	}
	ageS, _ := engine.AsString(age)
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
	_lst, _ := engine.AsList(result[0])
	leaves := _lst.Slice()
	if len(leaves) != 2 {
		t.Fatalf("expected 2 leaves, got %d", len(leaves))
	}

	paths := make(map[string]string)
	for _, leaf := range leaves {
		m, _ := engine.AsMap(leaf)
		p, _ := m.Get("path")
		v, _ := m.Get("value")
		ps1, _ := engine.AsString(p)
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
	_lst, _ := engine.AsList(result[0])
	leaves := _lst.Slice()
	if len(leaves) != 3 {
		t.Fatalf("expected 3 leaves, got %d", len(leaves))
	}

	paths := make(map[string]bool)
	for _, leaf := range leaves {
		m, _ := engine.AsMap(leaf)
		p, _ := m.Get("path")
		ps2, _ := engine.AsString(p)
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
	fnDef := engine.FnDefInfo{
		Sigs: []engine.FnSig{{
			Params:  []engine.FnParam{{Name: "m", Type: engine.TMap}},
			Returns: []*engine.Type{engine.TAny},
			Body:    []engine.Value{engine.NewWord("m"), engine.NewWord("get"), engine.NewWord("value")},
		}},
	}
	om := engine.NewOrderedMap()
	om.Set("a", engine.NewInteger(1))
	om.Set("b", engine.NewInteger(2))

	reg, err := engine.DefaultRegistry(native.Register)
	if err != nil {
		t.Fatal(err)
	}
	reg.SetParseFunc(parser.Parse)
	native.Register(reg)

	// Push the fn as a Quoted function value so it doesn't auto-execute
	// before walk can consume it from the stack.
	fnVal := engine.NewFunction(fnDef)
	fnVal.Quoted = true

	eng := engine.NewTop(reg)
	result, err := eng.Run([]engine.Value{
		engine.NewMap(om), fnVal, engine.NewWord("walk"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	m, _ := engine.AsMap(result[0])
	a, _ := m.Get("a")
	b, _ := m.Get("b")
	ai6, _ := engine.AsInteger(a)
	bi6, _ := engine.AsInteger(b)
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
	fnDef := engine.FnDefInfo{
		Sigs: []engine.FnSig{{
			Params:  []engine.FnParam{{Name: "m", Type: engine.TMap}},
			Returns: []*engine.Type{engine.TAny},
			Body:    []engine.Value{engine.NewWord("m"), engine.NewWord("get"), engine.NewWord("value")},
		}},
	}
	inner := engine.NewOrderedMap()
	inner.Set("x", engine.NewInteger(1))
	inner.Set("y", engine.NewInteger(2))
	om := engine.NewOrderedMap()
	om.Set("a", engine.NewMap(inner))
	om.Set("b", engine.NewInteger(3))

	reg, err := engine.DefaultRegistry(native.Register)
	if err != nil {
		t.Fatal(err)
	}
	reg.SetParseFunc(parser.Parse)
	native.Register(reg)

	fnVal := engine.NewFunction(fnDef)
	fnVal.Quoted = true

	eng := engine.NewTop(reg)
	result, err := eng.Run([]engine.Value{
		engine.NewMap(om), fnVal, engine.NewWord("walk"),
	})
	if err != nil {
		t.Fatal(err)
	}
	rm, _ := engine.AsMap(result[0])
	aVal, _ := rm.Get("a")
	aim, _ := engine.AsMap(aVal)
	x, _ := aim.Get("x")
	y, _ := aim.Get("y")
	b, _ := rm.Get("b")
	xi2, _ := engine.AsInteger(x)
	yi2, _ := engine.AsInteger(y)
	bi7, _ := engine.AsInteger(b)
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
	fnDef := engine.FnDefInfo{
		Sigs: []engine.FnSig{{
			Params:  []engine.FnParam{{Name: "m", Type: engine.TMap}},
			Returns: []*engine.Type{engine.TAny},
			Body:    []engine.Value{engine.NewInteger(99)},
		}},
	}
	om := engine.NewOrderedMap()
	om.Set("a", engine.NewInteger(1))
	om.Set("b", engine.NewInteger(2))

	reg, err := engine.DefaultRegistry(native.Register)
	if err != nil {
		t.Fatal(err)
	}
	reg.SetParseFunc(parser.Parse)
	native.Register(reg)

	fnVal := engine.NewFunction(fnDef)
	fnVal.Quoted = true

	eng := engine.NewTop(reg)
	result, err := eng.Run([]engine.Value{
		engine.NewMap(om), fnVal, engine.NewWord("walk"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	r0i2, _ := engine.AsInteger(result[0])
	if r0i2 != 99 {
		t.Errorf("expected 99, got %v", result[0])
	}
}

func TestWalkBeforeReturnPath(t *testing.T) {
	// AQL: {a:1 b:2} (fn [[m:Map] [Any] [m.path]]) walk
	// Before callback returns the path string for every node.
	// The root path is "" (empty string), which replaces the root map.
	// Since a string is not a node, descent stops — result is "".
	fnDef := engine.FnDefInfo{
		Sigs: []engine.FnSig{{
			Params:  []engine.FnParam{{Name: "m", Type: engine.TMap}},
			Returns: []*engine.Type{engine.TAny},
			Body:    []engine.Value{engine.NewWord("m"), engine.NewWord("get"), engine.NewWord("path")},
		}},
	}
	om := engine.NewOrderedMap()
	om.Set("a", engine.NewInteger(1))
	om.Set("b", engine.NewInteger(2))

	reg, err := engine.DefaultRegistry(native.Register)
	if err != nil {
		t.Fatal(err)
	}
	reg.SetParseFunc(parser.Parse)
	native.Register(reg)

	fnVal := engine.NewFunction(fnDef)
	fnVal.Quoted = true

	eng := engine.NewTop(reg)
	result, err := eng.Run([]engine.Value{
		engine.NewMap(om), fnVal, engine.NewWord("walk"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	r0s2, _ := engine.AsString(result[0])
	if r0s2 != "" {
		t.Errorf("expected empty string (root path), got %q", r0s2)
	}
}

// --- walk with before AND after callbacks ---

func TestWalkBeforeAfterIdentity(t *testing.T) {
	// AQL: {a:1 b:2} (fn [[m:Map] [Any] [m.value]]) (fn [[m:Map] [Any] [m.value]]) walk
	// Both before and after return m.value (identity) — tree is preserved.
	identityBody := []engine.Value{engine.NewWord("m"), engine.NewWord("get"), engine.NewWord("value")}
	fnDef1 := engine.FnDefInfo{
		Sigs: []engine.FnSig{{
			Params:  []engine.FnParam{{Name: "m", Type: engine.TMap}},
			Returns: []*engine.Type{engine.TAny},
			Body:    identityBody,
		}},
	}
	fnDef2 := engine.FnDefInfo{
		Sigs: []engine.FnSig{{
			Params:  []engine.FnParam{{Name: "m", Type: engine.TMap}},
			Returns: []*engine.Type{engine.TAny},
			Body:    identityBody,
		}},
	}
	om := engine.NewOrderedMap()
	om.Set("a", engine.NewInteger(1))
	om.Set("b", engine.NewInteger(2))

	reg, err := engine.DefaultRegistry(native.Register)
	if err != nil {
		t.Fatal(err)
	}
	reg.SetParseFunc(parser.Parse)
	native.Register(reg)

	fnVal1 := engine.NewFunction(fnDef1)
	fnVal1.Quoted = true
	fnVal2 := engine.NewFunction(fnDef2)
	fnVal2.Quoted = true

	eng := engine.NewTop(reg)
	result, err := eng.Run([]engine.Value{
		engine.NewMap(om), fnVal1, fnVal2, engine.NewWord("walk"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	m, _ := engine.AsMap(result[0])
	a, _ := m.Get("a")
	b, _ := m.Get("b")
	ai7, _ := engine.AsInteger(a)
	bi8, _ := engine.AsInteger(b)
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
	fnDef1 := engine.FnDefInfo{
		Sigs: []engine.FnSig{{
			Params:  []engine.FnParam{{Name: "m", Type: engine.TMap}},
			Returns: []*engine.Type{engine.TAny},
			Body:    []engine.Value{engine.NewWord("m"), engine.NewWord("get"), engine.NewWord("value")},
		}},
	}
	fnDef2 := engine.FnDefInfo{
		Sigs: []engine.FnSig{{
			Params:  []engine.FnParam{{Name: "m", Type: engine.TMap}},
			Returns: []*engine.Type{engine.TAny},
			Body:    []engine.Value{engine.NewInteger(99)},
		}},
	}
	om := engine.NewOrderedMap()
	om.Set("a", engine.NewInteger(1))
	om.Set("b", engine.NewInteger(2))

	reg, err := engine.DefaultRegistry(native.Register)
	if err != nil {
		t.Fatal(err)
	}
	reg.SetParseFunc(parser.Parse)
	native.Register(reg)

	fnVal1 := engine.NewFunction(fnDef1)
	fnVal1.Quoted = true
	fnVal2 := engine.NewFunction(fnDef2)
	fnVal2.Quoted = true

	eng := engine.NewTop(reg)
	result, err := eng.Run([]engine.Value{
		engine.NewMap(om), fnVal1, fnVal2, engine.NewWord("walk"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	r0i3, _ := engine.AsInteger(result[0])
	if r0i3 != 99 {
		t.Errorf("expected 99 (after replaces all), got %v", result[0])
	}
}

func TestWalkBeforeAfterNested(t *testing.T) {
	// AQL: {a:{x:1 y:2} b:3}
	//        (fn [[m:Map] [Any] [m.value]])
	//        (fn [[m:Map] [Any] [m.value]]) walk
	// Both callbacks are identity — nested tree preserved through full traversal.
	identityBody := []engine.Value{engine.NewWord("m"), engine.NewWord("get"), engine.NewWord("value")}
	fnDef1 := engine.FnDefInfo{
		Sigs: []engine.FnSig{{
			Params:  []engine.FnParam{{Name: "m", Type: engine.TMap}},
			Returns: []*engine.Type{engine.TAny},
			Body:    identityBody,
		}},
	}
	fnDef2 := engine.FnDefInfo{
		Sigs: []engine.FnSig{{
			Params:  []engine.FnParam{{Name: "m", Type: engine.TMap}},
			Returns: []*engine.Type{engine.TAny},
			Body:    identityBody,
		}},
	}
	innerMap := engine.NewOrderedMap()
	innerMap.Set("x", engine.NewInteger(1))
	innerMap.Set("y", engine.NewInteger(2))
	om := engine.NewOrderedMap()
	om.Set("a", engine.NewMap(innerMap))
	om.Set("b", engine.NewInteger(3))

	reg, err := engine.DefaultRegistry(native.Register)
	if err != nil {
		t.Fatal(err)
	}
	reg.SetParseFunc(parser.Parse)
	native.Register(reg)

	fnVal1 := engine.NewFunction(fnDef1)
	fnVal1.Quoted = true
	fnVal2 := engine.NewFunction(fnDef2)
	fnVal2.Quoted = true

	eng := engine.NewTop(reg)
	result, err := eng.Run([]engine.Value{
		engine.NewMap(om), fnVal1, fnVal2, engine.NewWord("walk"),
	})
	if err != nil {
		t.Fatal(err)
	}
	m, _ := engine.AsMap(result[0])
	inner, _ := m.Get("a")
	im, _ := engine.AsMap(inner)
	x, _ := im.Get("x")
	y, _ := im.Get("y")
	b, _ := m.Get("b")
	xi3, _ := engine.AsInteger(x)
	yi3, _ := engine.AsInteger(y)
	bi9, _ := engine.AsInteger(b)
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
