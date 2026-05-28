package native

import (
	"testing"

	eng "github.com/aql-lang/aql/eng/go"
)

// User-defined refinement subtypes must dispatch like system types — a
// fn parameter declared `[f:Foo]` (where Foo is `def Foo refine Object
// {...}`) accepts ONLY Foo instances and REJECTS unrelated values, just
// as `[f:Integer]` does.
//
// **End-to-end dispatch tests live in lang/spec/user-types.tsv** —
// run via test/go/langspec. They exercise the actual parser, dispatcher,
// and rendering pipeline through AQL source code (the right level for
// a *language* test).
//
// The Go tests below are unit-level: they probe internal Value-shape
// invariants and the `DefEntry.TypeDef` flag path that the TSV runner
// can't reach. Keep them tight — anything user-observable belongs in
// the TSV.

// Documents the architectural distinction the fix relies on:
//
//   - The codebase HAS a dedicated "this is a type" flag:
//     DefEntry.TypeDef *Type (see eng/go/deftable.go). It's set
//     non-nil exactly when a capitalised def installs a type
//     binding. r.LookupTypeName(name) consults it directly and
//     returns the lattice node.
//
//   - Before the fix, ResolveSigType only used that flag as a
//     narrow special-case for predicate-fn bodies, and reached
//     ResolveDefType for everything else. ResolveDefType then
//     INFERRED type-ness from body payload shape: `Data == nil`
//     (lattice-only body), IsRecordType, IsOptionsType — anything
//     else fell through to `return TAny`. ObjectType, Disjunct,
//     DepScalar and others silently became wildcards.
//
//   - After the fix, ResolveSigType consults DefEntry.TypeDef
//     first via r.LookupTypeName, and per-kind Behavior wrappers
//     (disjunctUnifier, depScalarUnifier, bareRefineUnifier) drive
//     the actual Match the dispatcher consults. Payload-shape
//     inspection is no longer the discriminator for "is this a
//     type"; the dedicated flag is.
//
// Confirms the shape data the fix relies on: ObjectType bodies carry
// their payload in Data, and the lattice node lives at body.Parent.
func TestUserTypeBindingShape_ObjectRefinement(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatalf("registry: %v", err)
	}
	runAQL(t, r, []Value{
		NewWord("def"), NewWord("Foo"),
		NewOpenParen(),
		NewWord("refine"), NewWord("Object"), NewMap(NewOrderedMap()),
		NewCloseParen(),
	})

	// 1. The dedicated flag: r.LookupTypeName returns the lattice
	//    node directly. This is the authoritative path.
	def := r.LookupTypeName("Foo")
	if def == nil {
		t.Fatal("LookupTypeName(Foo) returned nil — DefEntry.TypeDef flag not set")
	}
	if def.Leaf() != "Foo" {
		t.Errorf("LookupTypeName(Foo).Leaf() = %q, want %q", def.Leaf(), "Foo")
	}

	// 2. The body in r.TopTypeBody is a POPULATED body — its Data
	//    carries the ObjectTypeInfo payload; the lattice node is at
	//    body.Parent.
	body, ok := r.TopTypeBody("Foo")
	if !ok {
		t.Fatal("Foo not installed as a type")
	}
	if !IsConcrete(body) {
		t.Error("ObjectType body should have Data != nil")
	}
	if body.Parent == nil || body.Parent.Leaf() != "Foo" {
		t.Errorf("body.Parent should be Foo lattice node, got %v", body.Parent)
	}

	// 3. ResolveSigType (the dispatcher's entry point) consults the
	//    dedicated flag and returns the lattice node, NOT TAny.
	tp, _, err := eng.ResolveSigType(r, NewWord("Foo"))
	if err != nil {
		t.Fatalf("ResolveSigType: %v", err)
	}
	if tp.Equal(TAny) {
		t.Error("ResolveSigType returned TAny for Foo — user subtype silently wildcard")
	}
	if tp.Leaf() != "Foo" {
		t.Errorf("ResolveSigType leaf = %q, want %q", tp.Leaf(), "Foo")
	}
}

// Catalogue what each refinement shape produces as a body and confirms
// ResolveSigType resolves each to the user-named lattice node. This is
// the structural invariant that anchors the dispatch-matrix tests in
// lang/spec/user-types.tsv — if ResolveSigType ever started returning
// TAny again for any of these shapes, the TSV happy-path rows would
// still pass (TAny accepts everything) but the TSV negative-path rows
// would fail. This Go test catches the regression at the resolution
// layer where it originates.
func TestResolveSigType_AllUserTypeKinds(t *testing.T) {
	cases := []struct {
		name    string
		defTail []Value
	}{
		{
			name: "refine Object {}",
			defTail: []Value{
				NewWord("refine"), NewWord("Object"), NewMap(NewOrderedMap()),
			},
		},
		{
			name: "refine Integer (bare scalar)",
			defTail: []Value{
				NewWord("refine"), NewWord("Integer"),
			},
		},
		{
			name: "Disjunct (Integer tor none)",
			defTail: []Value{
				NewWord("Integer"), NewWord("tor"), NewWord("none"),
			},
		},
		{
			name: "DepScalar (Integer gt 10)",
			defTail: []Value{
				NewWord("Integer"), NewWord("gt"), NewInteger(10),
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r, _ := DefaultRegistry()
			input := append([]Value{
				NewWord("def"), NewWord("X"),
				NewOpenParen(),
			}, tc.defTail...)
			input = append(input, NewCloseParen())
			runAQL(t, r, input)

			// The dedicated flag must be set.
			def := r.LookupTypeName("X")
			if def == nil {
				t.Fatalf("LookupTypeName(X) = nil — DefEntry.TypeDef flag not set for %s", tc.name)
			}

			// And ResolveSigType must use it — not fall through to TAny.
			tp, _, err := eng.ResolveSigType(r, NewWord("X"))
			if err != nil {
				t.Fatalf("ResolveSigType: %v", err)
			}
			if tp == nil {
				t.Fatal("ResolveSigType returned nil")
			}
			if tp.Equal(TAny) {
				t.Errorf("ResolveSigType returned TAny for %s — user subtype silently wildcard", tc.name)
			}
			if tp.Leaf() != "X" {
				t.Errorf("ResolveSigType leaf = %q, want %q", tp.Leaf(), "X")
			}
		})
	}
}

func TestProbeIsHandlerPath_DEBUG(t *testing.T) {
	r, _ := DefaultRegistry()
	runAQL(t, r, []Value{
		NewWord("def"), NewWord("Pos"),
		NewWord("refine"), NewWord("Integer"),
	})

	// What's in Defs.Top("Pos")?
	body, ok := r.TopTypeBody("Pos")
	if !ok {
		t.Fatal("Pos not installed")
	}
	t.Logf("body for Pos: Data==nil=%v Behavior type=%T leaf=%s",
		IsBareTypeNode(body), body.Behavior, body.Leaf())

	// What's the lattice node from LookupTypeName?
	def := r.LookupTypeName("Pos")
	t.Logf("lattice def: Behavior type=%T", def.Behavior)

	// Same Behavior? Or different?
	t.Logf("body.Behavior == def.Behavior: %v",
		body.Behavior == def.Behavior)

	// Direct membership check via v.Is(t) on the lattice node.
	t.Logf("42.Is(def-lattice-node) = %v", NewInteger(42).Is(def))

	// And on &body (what isHandler does).
	bNode := body
	t.Logf("42.Is(&body) = %v", NewInteger(42).Is(&bNode))
}

func TestProbeFooInstance_DEBUG(t *testing.T) {
	r, _ := DefaultRegistry()
	runAQL(t, r, []Value{
		NewWord("def"), NewWord("Foo"),
		NewOpenParen(),
		NewWord("refine"), NewWord("Object"), NewMap(NewOrderedMap()),
		NewCloseParen(),
	})

	result := runAQL(t, r, []Value{
		NewOpenParen(),
		NewWord("make"), NewWord("Foo"), NewMap(NewOrderedMap()),
		NewCloseParen(),
	})
	inst := result[0]
	t.Logf("instance: Parent.ID=%q Parent.Name=%q Behavior=%T",
		inst.Parent.ID, inst.Parent.Name, inst.Parent.Behavior)

	// What does Foo type literal look like (via Defs.Top, i.e. what
	// isHandler sees in args[0])?
	body, _ := r.TopTypeBody("Foo")
	t.Logf("body: ID=%q Name=%q Behavior=%T", body.ID, body.Name, body.Behavior)

	// And LookupTypeName (what dispatch sees).
	def := r.LookupTypeName("Foo")
	t.Logf("def lattice: ID=%q Name=%q Behavior=%T", def.ID, def.Name, def.Behavior)

	// Are they the SAME lattice node by ID?
	t.Logf("inst.Parent.ID == def.ID: %v", inst.Parent.ID == def.ID)
	t.Logf("inst.Parent.ID == body.ID: %v", inst.Parent.ID == body.ID)

	// Is the membership check working at the lattice node level?
	t.Logf("inst.Is(def): %v", inst.Is(def))
	bNode := body
	t.Logf("inst.Is(&body): %v", inst.Is(&bNode))
	t.Logf("inst.Parent.Matches(def): %v", inst.Parent.Matches(def))
	t.Logf("inst.Parent.Matches(&body): %v", inst.Parent.Matches(&bNode))
}

func TestProbeMaybe_DEBUG(t *testing.T) {
	r, _ := DefaultRegistry()
	runAQL(t, r, []Value{
		NewWord("def"), NewWord("Maybe"),
		NewOpenParen(),
		NewWord("Integer"), NewWord("tor"), NewWord("none"),
		NewCloseParen(),
	})
	body, _ := r.TopTypeBody("Maybe")
	t.Logf("body: Data==nil=%v Behavior=%T ID=%q Name=%q Parent.Leaf=%s",
		IsBareTypeNode(body), body.Behavior, body.ID, body.Name, body.Parent.Leaf())
	def := r.LookupTypeName("Maybe")
	t.Logf("def: Behavior=%T", def.Behavior)
	t.Logf("body.Behavior == def.Behavior: %v", body.Behavior == def.Behavior)

	bNode := body
	t.Logf("42.Is(&body): %v", NewInteger(42).Is(&bNode))
	t.Logf("42.Is(def):   %v", NewInteger(42).Is(def))
}
