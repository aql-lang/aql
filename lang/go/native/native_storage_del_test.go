package native

import "testing"

// Coverage for `del` (native_storage.go): the Map copy-returning form,
// the FlexMap in-place form, key skipping, elem-constraint retention,
// and the reject arms of both handlers (driven directly with crafted
// args, in the native_storage_seam9_test.go style).

func TestDelMapCopyReturning(t *testing.T) {
	// Present key: the copy lacks it, the receiver is untouched.
	v := w9Top(t, `def m {a:1 b:2} (m del "a") size m size`)
	if got, _ := v.AsConcreteInteger(); got != 2 {
		t.Fatalf("receiver mutated: size %d, want 2", got)
	}
	v = w9Top(t, `({a:1 b:2} del "a") size`)
	if got, _ := v.AsConcreteInteger(); got != 1 {
		t.Fatalf("copy size %d, want 1", got)
	}

	// Missing key: a no-op copy.
	v = w9Top(t, `({a:1} del "zz") size`)
	if got, _ := v.AsConcreteInteger(); got != 1 {
		t.Fatalf("no-op copy size %d, want 1", got)
	}

	// Atom-key form quotes the bare word.
	v = w9Top(t, `({a:1 b:2} del a) has ("a")`)
	if b, _ := v.AsConcreteBoolean(); b {
		t.Fatalf("atom-key del left the key present")
	}
}

func TestDelMapKeepsElemConstraint(t *testing.T) {
	// A typed map's element tag survives the deletion copy: a
	// conforming write onto the del result stays accepted…
	if _, err := w9Run(t, w9Reg(t), `def t:{:Integer} {a:1 b:2} ((t del "a") set c/q 3)`); err != nil {
		t.Fatalf("conforming write on del result rejected: %v", err)
	}
	// …and a non-conforming write is still rejected.
	if _, err := w9Run(t, w9Reg(t), `def t:{:Integer} {a:1 b:2} ((t del "a") set c/q "wrong")`); err == nil {
		t.Fatalf("non-conforming write on del result unexpectedly accepted")
	}
}

func TestDelFlexMapInPlace(t *testing.T) {
	// In place: the alias observes the removal; del returns the node.
	v := w9Top(t, `def f (flex {a:1 b:2}) def g f (f del "a") drop g size`)
	if got, _ := v.AsConcreteInteger(); got != 1 {
		t.Fatalf("alias size %d, want 1", got)
	}
	v = w9Top(t, `def f (flex {a:1}) ((f del "a") eq f)`)
	if b, _ := v.AsConcreteBoolean(); !b {
		t.Fatalf("del did not return the receiver node")
	}

	// Missing key: no-op, node identity kept.
	v = w9Top(t, `def f (flex {a:1}) ((f del "zz") eq f)`)
	if b, _ := v.AsConcreteBoolean(); !b {
		t.Fatalf("missing-key del did not return the receiver node")
	}

	// Chaining, atom keys.
	v = w9Top(t, `(flex {a:1 b:2 c:3}) del a del b size`)
	if got, _ := v.AsConcreteInteger(); got != 1 {
		t.Fatalf("chained del size %d, want 1", got)
	}
}

func TestDelContainerRejects(t *testing.T) {
	r := w9Reg(t)

	// delMapHandler: non-concrete map receiver.
	_, err := delMapHandler([]Value{NewString("k"), NewTypeLiteral(TMap)}, nil, nil, r)
	w9wantErr(t, err, "")

	// delFlexMapHandler: non-FlexMap receiver.
	_, err = delFlexMapHandler([]Value{NewString("k"), NewTypeLiteral(TFlexMap)}, nil, nil, r)
	w9wantErr(t, err, "expected a FlexMap")
}
