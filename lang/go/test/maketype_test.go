package test

import (
	"testing"

	"github.com/aql-lang/aql/lang/go"
)

// Phase-1 prototype tests for the uniform type constructor — see
// lang/doc/design/TYPE-UNIFORM.0.md. `maketype` is the transitional
// name for what becomes `type` at the final cutover; it constructs a
// type from a base type plus an argument, and is paired with `def`:
//
//	def acct (maketype Object {x:Integer})
//
// Note: the prototype binds type names with lowercase identifiers
// because `def` still rejects capitalised names — folding the
// capitalised-name binder role into `def` is Phase 3. The new
// constructor mechanism is what Phase 1 proves out.
//
// These tests assert the new constructor works and that the existing
// `type`/`record`/`object`/`table` syntax is unaffected.

// maketype Record {fields} builds a record type; make instantiates it.
func TestMaketypeRecord(t *testing.T) {
	got := runOne(t, "def pt (maketype Record {x:Integer y:Integer})\nmake pt {x:3 y:4} .x")
	if len(got) != 1 || got[0] != int64(3) {
		t.Errorf("got %v, want [3]", got)
	}
}

// A maketype-built record renders as a record type.
func TestMaketypeRecordRenders(t *testing.T) {
	got := runOne(t, "def pt (maketype Record {x:Integer y:String})\npt")
	if len(got) != 1 || got[0] != "record{x:Integer,y:String}" {
		t.Errorf("got %v, want [record{x:Integer,y:String}]", got)
	}
}

// maketype Table <recordtype> builds a table type; make builds rows.
func TestMaketypeTable(t *testing.T) {
	got := runOne(t, "def row (maketype Record {n:String})\n"+
		"def tbl (maketype Table row)\n"+
		"make tbl [[\"a\"] [\"b\"] [\"c\"]] length")
	if len(got) != 1 || got[0] != int64(3) {
		t.Errorf("got %v, want [3]", got)
	}
}

// maketype Object {fields} builds an object type; make + .field works.
func TestMaketypeObject(t *testing.T) {
	got := runOne(t, "def acct (maketype Object {bal:Number})\nmake acct {bal:50} .bal")
	if len(got) != 1 || got[0] != int64(50) {
		t.Errorf("got %v, want [50]", got)
	}
}

// Inheritance: applying an existing object type extends it. The child
// instance carries both the parent's and its own fields.
func TestMaketypeObjectInheritance(t *testing.T) {
	got := runOne(t, "def animal (maketype Object {legs:Integer})\n"+
		"def dog (maketype animal {breed:String})\n"+
		"make dog {legs:4 breed:\"lab\"} .legs")
	if len(got) != 1 || got[0] != int64(4) {
		t.Errorf("got %v, want [4]", got)
	}
}

// A child instance satisfies the parent type via `is`.
func TestMaketypeObjectInheritanceIsCheck(t *testing.T) {
	// `is` yields a boolean; (*AQL).Run stringifies non-int/string
	// results, so the boolean surfaces as the string "true".
	got := runOne(t, "def animal (maketype Object {legs:Integer})\n"+
		"def dog (maketype animal {breed:String})\n"+
		"make dog {legs:4 breed:\"lab\"} is animal")
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
