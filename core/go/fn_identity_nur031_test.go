package core

import "testing"

// NUR031: a function's `deq` is CONTENT equality — parameters, returns and
// body — while `eq` stays reference identity per NUR011. Before this,
// `f/r deq f/r` was false: a function was not deq to ITSELF, which made
// ADR-015's round-trip unsatisfiable for the kind, since a value that is
// not deq to itself cannot be deq to anything re-parsed from its canon.

func nur031Fn(t *testing.T, r *Registry, name string, body []Value) Value {
	t.Helper()
	return NewFunction(FnDefInfo{Name: name, Registry: r, Signatures: []Signature{{
		Params:  []FnParam{{Name: "n", Type: TInteger}},
		Returns: []*Type{TInteger},
		Impl:    Boru(body),
	}}})
}

func TestNUR031FnDeqIsReflexive(t *testing.T) {
	r, err := NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	f := nur031Fn(t, r, "f", []Value{NewInteger(1)})
	if !DeepEqual(f, f) {
		t.Fatal("a function must be deq to itself — ADR-015's prerequisite")
	}
}

// Content equality ignores the BINDING NAME, which is what made two
// references to one function compare unequal. The name is how a function
// was reached, not part of what it is.
func TestNUR031FnDeqIgnoresTheBindingName(t *testing.T) {
	r, _ := NewRegistry()
	f := nur031Fn(t, r, "f", []Value{NewInteger(1)})
	renamed, _ := f.Data.(FnDefInfo)
	renamed.Name = "a"
	if !DeepEqual(f, NewFunction(renamed)) {
		t.Fatal("the same function under another name must be deq")
	}
}

// …but it does NOT ignore the body. This is the negative that the earlier
// Impl-walking attempt got wrong: after InstallFnDef a signature's Impl is
// a Go handler, so walking it found no body and every installed function
// read as identical — a false POSITIVE. canonFnDef still sees the body.
func TestNUR031FnDeqDiscriminatesTheBody(t *testing.T) {
	r, _ := NewRegistry()
	f := nur031Fn(t, r, "f", []Value{NewInteger(1)})
	h := nur031Fn(t, r, "h", []Value{NewInteger(2)})
	if DeepEqual(f, h) {
		t.Fatal("different bodies must NOT be deq")
	}
}

// The DEFINING MODULE is part of the content: boru resolves a function's
// free words in the module that defined it, so two identical bodies from
// different modules do not mean the same thing.
func TestNUR031FnDeqSeparatesDefiningModules(t *testing.T) {
	r1, _ := NewRegistry()
	r2, _ := NewRegistry()
	body := []Value{NewInteger(1)}
	if DeepEqual(nur031Fn(t, r1, "f", body), nur031Fn(t, r2, "f", body)) {
		t.Fatal("identical bodies in different modules must NOT be deq")
	}
}

// eq stays REFERENCE identity: two independently-written functions with the
// same content are deq but not eq. That distinction is the reason content
// addressing answers only half of this record.
func TestNUR031EqStaysReferenceIdentity(t *testing.T) {
	r, _ := NewRegistry()
	body := []Value{NewInteger(1)}
	f := nur031Fn(t, r, "f", body)
	g := nur031Fn(t, r, "g", body)
	if !DeepEqual(f, g) {
		t.Fatal("identical content is deq")
	}
	if ExactEqual(f, g) {
		t.Fatal("…but two independently-written functions are not eq")
	}
	if !ExactEqual(f, f) {
		t.Fatal("a function is eq to itself")
	}
}

// A zero-signature payload has no array to key on, so it is never
// reference-equal — answering true would make every empty fn value eq.
func TestNUR031EmptySignaturesAreNotReferenceEqual(t *testing.T) {
	a := FnDefInfo{Name: "a"}
	if sameFnIdentity(a, a) {
		t.Fatal("a zero-signature payload has no reference identity")
	}
}

// A function against a NON-function: the fn arms claim the pair as soon as
// either side is one, so they must answer for the mixed case too rather
// than falling through to an arm that reads a FnDefInfo as something else.
func TestNUR031FnAgainstNonFn(t *testing.T) {
	r, _ := NewRegistry()
	f := nur031Fn(t, r, "f", []Value{NewInteger(1)})
	other := NewInteger(1)
	if ExactEqual(f, other) || ExactEqual(other, f) {
		t.Fatal("a function is not eq to a non-function, in either order")
	}
	if DeepEqual(f, other) || DeepEqual(other, f) {
		t.Fatal("…nor deq to one")
	}
}
