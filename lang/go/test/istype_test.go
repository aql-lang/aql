package test

import (
	"testing"
)

// Tests for the istype native function.

func TestIstype_TypeLiteral_String(t *testing.T) {
	result, err := runNativeSteps(t, nil, []string{`istype String`})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "true")
}

func TestIstype_TypeLiteral_Integer(t *testing.T) {
	result, err := runNativeSteps(t, nil, []string{`istype Integer`})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "true")
}

func TestIstype_TypeLiteral_Map(t *testing.T) {
	result, err := runNativeSteps(t, nil, []string{`istype Map`})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "true")
}

func TestIstype_TypeLiteral_List(t *testing.T) {
	result, err := runNativeSteps(t, nil, []string{`istype List`})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "true")
}

func TestIstype_TypeLiteral_Boolean(t *testing.T) {
	result, err := runNativeSteps(t, nil, []string{`istype Boolean`})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "true")
}

func TestIstype_TypeLiteral_Number(t *testing.T) {
	result, err := runNativeSteps(t, nil, []string{`istype Number`})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "true")
}

func TestIstype_TypeLiteral_Decimal(t *testing.T) {
	result, err := runNativeSteps(t, nil, []string{`istype Decimal`})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "true")
}

func TestIstype_TypeLiteral_Atom(t *testing.T) {
	result, err := runNativeSteps(t, nil, []string{`istype Atom`})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "true")
}

func TestIstype_TypeLiteral_Any(t *testing.T) {
	result, err := runNativeSteps(t, nil, []string{`istype Any`})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "true")
}

// --- Concrete values are not types ---

func TestIstype_Integer(t *testing.T) {
	result, err := runNativeSteps(t, nil, []string{`istype 99`})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "false")
}

func TestIstype_String(t *testing.T) {
	result, err := runNativeSteps(t, nil, []string{`istype "hello"`})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "false")
}

func TestIstype_Boolean(t *testing.T) {
	result, err := runNativeSteps(t, nil, []string{`istype true`})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "false")
}

func TestIstype_ConcreteList(t *testing.T) {
	result, err := runNativeSteps(t, nil, []string{`istype [99,88]`})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "false")
}

func TestIstype_ConcreteMap(t *testing.T) {
	result, err := runNativeSteps(t, nil, []string{`istype {a:{b:77}}`})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "false")
}

func TestIstype_None(t *testing.T) {
	result, err := runNativeSteps(t, nil, []string{`istype None`})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "false")
}

// --- Nodes containing type leaves ---

func TestIstype_ListWithTypLeaf(t *testing.T) {
	// [a:{b:66,c:String}] — the map value for key c is a type literal
	result, err := runNativeSteps(t, nil, []string{`istype [a:{b:66,c:String}]`})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "true")
}

func TestIstype_MapWithTypeLeaf(t *testing.T) {
	result, err := runNativeSteps(t, nil, []string{`istype {a:String}`})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "true")
}

func TestIstype_NestedMapWithTypeLeaf(t *testing.T) {
	result, err := runNativeSteps(t, nil, []string{`istype {a:{b:{c:Integer}}}`})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "true")
}

func TestIstype_ListWithMixedValues(t *testing.T) {
	// List with integers and a type literal
	result, err := runNativeSteps(t, nil, []string{`istype [1,2,Integer]`})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "true")
}

func TestIstype_DeeplyNestedType(t *testing.T) {
	result, err := runNativeSteps(t, nil, []string{`istype {a:{b:[1,{c:Boolean}]}}`})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "true")
}

func TestIstype_DeeplyNestedNoType(t *testing.T) {
	result, err := runNativeSteps(t, nil, []string{`istype {a:{b:[1,{c:2}]}}`})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "false")
}

// --- Record and Options types ---

func TestIstype_RecordType(t *testing.T) {
	result, err := runNativeSteps(t, nil, []string{
		`maketype Record [{a:String} {b:Integer}]; istype`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "true")
}
