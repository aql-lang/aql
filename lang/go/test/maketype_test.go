package test

import (
	"testing"

	"github.com/aql-lang/aql/lang/go"
)

// Tests for the uniform type constructor — see
// lang/doc/design/TYPE-UNIFORM.0.md. `maketype` is the transitional
// name for what becomes `type` at the final cutover; it constructs a
// type from a base type plus an argument, and is paired with `def`:
//
//	def Acct (maketype Object {x:Integer})
//
// Phase 2 made `def` the universal binder, so type names here use the
// canonical capitalised form — a capitalised `def` is a type binding.
//
// These tests assert the new constructor works and that the existing
// `type`/`record`/`object`/`table` syntax is unaffected.

// maketype Record {fields} builds a record type; make instantiates it.
func TestMaketypeRecord(t *testing.T) {
	got := runOne(t, "def Pt (maketype Record {x:Integer y:Integer})\nmake Pt {x:3 y:4} .x")
	if len(got) != 1 || got[0] != int64(3) {
		t.Errorf("got %v, want [3]", got)
	}
}

// A maketype-built record renders as a record type.
func TestMaketypeRecordRenders(t *testing.T) {
	got := runOne(t, "def Pt (maketype Record {x:Integer y:String})\nPt")
	if len(got) != 1 || got[0] != "record{x:Integer,y:String}" {
		t.Errorf("got %v, want [record{x:Integer,y:String}]", got)
	}
}

// maketype Table <recordtype> builds a table type; make builds rows.
func TestMaketypeTable(t *testing.T) {
	got := runOne(t, "def Row (maketype Record {n:String})\n"+
		"def Tbl (maketype Table Row)\n"+
		"make Tbl [[\"a\"] [\"b\"] [\"c\"]] length")
	if len(got) != 1 || got[0] != int64(3) {
		t.Errorf("got %v, want [3]", got)
	}
}

// maketype Object {fields} builds an object type; make + .field works.
func TestMaketypeObject(t *testing.T) {
	got := runOne(t, "def Acct (maketype Object {bal:Number})\nmake Acct {bal:50} .bal")
	if len(got) != 1 || got[0] != int64(50) {
		t.Errorf("got %v, want [50]", got)
	}
}

// Inheritance: applying an existing object type extends it. The child
// instance carries both the parent's and its own fields.
func TestMaketypeObjectInheritance(t *testing.T) {
	got := runOne(t, "def Animal (maketype Object {legs:Integer})\n"+
		"def Dog (maketype Animal {breed:String})\n"+
		"make Dog {legs:4 breed:\"lab\"} .legs")
	if len(got) != 1 || got[0] != int64(4) {
		t.Errorf("got %v, want [4]", got)
	}
}

// A child instance satisfies the parent type via `is`.
func TestMaketypeObjectInheritanceIsCheck(t *testing.T) {
	// `is` yields a boolean; (*AQL).Run stringifies non-int/string
	// results, so the boolean surfaces as the string "true".
	got := runOne(t, "def Animal (maketype Object {legs:Integer})\n"+
		"def Dog (maketype Animal {breed:String})\n"+
		"make Dog {legs:4 breed:\"lab\"} is Animal")
	if len(got) != 1 || got[0] != "true" {
		t.Errorf("got %v, want [true]", got)
	}
}

// The legacy `type`/`record` syntax is unaffected by the new word.
func TestMaketypeLegacySyntaxUnaffected(t *testing.T) {
	got := runOne(t, "type Foo record [a:Integer]\nmake Foo {a:9} .a")
	if len(got) != 1 || got[0] != int64(9) {
		t.Errorf("got %v, want [9]", got)
	}
}

// maketype with a base that is not a structural type is rejected.
func TestMaketypeBadBase(t *testing.T) {
	a, err := lang.New()
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	if _, err := a.Run("maketype Integer {x:1}"); err == nil {
		t.Fatal("expected error for `maketype Integer {x:1}`, got nil")
	}
}
