package core

import "testing"

// ApplyBindTwin is the runtime half of §6.5's regime: one placed OpBindTwin
// re-performing one recorded transition. Each kind's arm — and the
// carrier-class skip that pairs a computed def's twin with its Push-mode
// OpBindGlobal — is pinned here directly, against the same DefTable
// surface the sandbox harness proved at corpus scale.

func TestApplyBindTwinPushKinds(t *testing.T) {
	// Nil registry: inert, like every registry entry point.
	ApplyBindTwin(nil, BindTransition{Kind: BindDef, Name: "x"}, DefEntry{Body: NewInteger(1)})

	r := newTestRegistry(t)

	// A concrete def replays the captured value.
	ApplyBindTwin(r, BindTransition{Kind: BindDef, Name: "c"}, DefEntry{Body: NewInteger(7)})
	if v, ok := r.Defs.Top("c"); !ok || v.String() != "7" {
		t.Fatalf("concrete def twin left %v/%v, want 7", v, ok)
	}

	// A bare type node bound as a VALUE (`def x None`) is self-representing
	// and replays too.
	ApplyBindTwin(r, BindTransition{Kind: BindDef, Name: "n"}, DefEntry{Body: NewTypeLiteral(TNone)})
	if _, ok := r.Defs.Top("n"); !ok {
		t.Fatal("bare-node def twin installed nothing")
	}

	// THE CARRIER-CLASS SKIP: a captured CARRIER is lowerDynBind's
	// needGlobal class — its Push-mode OpBindGlobal owns the install, so
	// the twin must leave the name untouched.
	ApplyBindTwin(r, BindTransition{Kind: BindDef, Name: "k"}, DefEntry{Body: NewDynamicCarrier(TAny)})
	if _, ok := r.Defs.Top("k"); ok {
		t.Fatal("carrier-class def twin must skip — its OpBindGlobal pushes the runtime value")
	}

	// A MINTED type install re-pushes its node (the mint survived the
	// rollback); an ADOPTED alias re-adopts without claiming the mint.
	minted := r.Types.MintType("TwinMint", TInteger)
	ApplyBindTwin(r, BindTransition{Kind: BindTypeInstall, Name: "TwinMint"},
		DefEntry{Body: NewTypeLiteral(TInteger), TypeDef: minted, Minted: true})
	if e, ok := r.Defs.TopEntry("TwinMint"); !ok || e.TypeDef != minted || !e.Minted {
		t.Fatalf("minted type twin left %+v/%v", e, ok)
	}
	ApplyBindTwin(r, BindTransition{Kind: BindTypeInstall, Name: "TwinAlias"},
		DefEntry{Body: NewTypeLiteral(TInteger), TypeDef: TInteger})
	if e, ok := r.Defs.TopEntry("TwinAlias"); !ok || e.TypeDef != TInteger || e.Minted {
		t.Fatalf("adopted type twin left %+v/%v", e, ok)
	}
}

func TestApplyBindTwinUndef(t *testing.T) {
	r := newTestRegistry(t)

	// A value undef pops the live entry; a missing name is a no-op (the
	// interpreter's undef would have failed the same row at check time).
	r.Defs.Push("u", NewInteger(1))
	ApplyBindTwin(r, BindTransition{Kind: BindUndef, Name: "u"}, DefEntry{})
	if _, ok := r.Defs.Top("u"); ok {
		t.Fatal("undef twin left the binding")
	}
	ApplyBindTwin(r, BindTransition{Kind: BindUndef, Name: "absent"}, DefEntry{})

	// A capitalised undef of a binding that MINTED its node re-retires it —
	// the rollback re-admitted the retirement for exactly this twin.
	minted := r.Types.MintType("TwinGone", TInteger)
	r.Defs.PushType("TwinGone", minted, NewTypeLiteral(TInteger))
	ApplyBindTwin(r, BindTransition{Kind: BindUndef, Name: "TwinGone"}, DefEntry{})
	if r.Defs.IsType("TwinGone") || r.Types.LookupByID(minted.ID) != nil {
		t.Fatal("minted-type undef twin must pop the binding AND retire the node")
	}

	// An ADOPTED binding's undef pops without retiring the node it adopted —
	// that identity belongs to whatever minted it.
	owned := r.Types.MintType("TwinOwned", TInteger)
	r.Defs.PushTypeAdopted("TwinKeep", owned, NewTypeLiteral(TInteger))
	ApplyBindTwin(r, BindTransition{Kind: BindUndef, Name: "TwinKeep"}, DefEntry{})
	if r.Defs.IsType("TwinKeep") {
		t.Fatal("adopted-alias undef twin left the binding")
	}
	if r.Types.LookupByID(owned.ID) != owned {
		t.Fatal("adopted-alias undef twin retired a node it did not mint")
	}
}

func TestApplyBindTwinDefReplace(t *testing.T) {
	r := newTestRegistry(t)
	r.Defs.Push("rp", NewInteger(1))
	ApplyBindTwin(r, BindTransition{Kind: BindDefReplace, Name: "rp"}, DefEntry{Body: NewInteger(2)})
	if v, ok := r.Defs.Top("rp"); !ok || v.String() != "2" || r.Defs.Depth("rp") != 1 {
		t.Fatalf("replace twin left %v/%v at depth %d, want 2 at depth 1", v, ok, r.Defs.Depth("rp"))
	}

	// A COMPUTED replacement is the pop half only — its Push-mode
	// OpBindGlobal pushes the runtime value right after, netting zero.
	ApplyBindTwin(r, BindTransition{Kind: BindDefReplace, Name: "rp"}, DefEntry{Body: NewDynamicCarrier(TAny)})
	if _, ok := r.Defs.Top("rp"); ok {
		t.Fatal("computed replace twin must pop and leave the push to OpBindGlobal")
	}
}

func TestApplyBindTwinSigUndef(t *testing.T) {
	r := newTestRegistry(t)

	// ID identity, mid-stack: the captured removal takes out ITS entry, not
	// the top.
	oldFn := NewInteger(10)
	oldFn.ID = "twin-old"
	newFn := NewInteger(20)
	newFn.ID = "twin-new"
	r.Defs.Push("f", oldFn)
	r.Defs.Push("f", newFn)
	ApplyBindTwin(r, BindTransition{Kind: BindSigUndef, Name: "f"}, DefEntry{Body: oldFn})
	if r.Defs.Depth("f") != 1 {
		t.Fatalf("sig-undef twin left depth %d, want 1", r.Defs.Depth("f"))
	}
	if v, _ := r.Defs.Top("f"); v.ID != "twin-new" {
		t.Fatalf("sig-undef twin removed the wrong entry: top is %s", v.ID)
	}

	// No ID on the capture: value equality locates the entry, most-recent
	// first.
	r.Defs.Push("g", NewInteger(1))
	r.Defs.Push("g", NewInteger(2))
	ApplyBindTwin(r, BindTransition{Kind: BindSigUndef, Name: "g"}, DefEntry{Body: NewInteger(1)})
	if r.Defs.Depth("g") != 1 {
		t.Fatalf("value-equal sig-undef left depth %d, want 1", r.Defs.Depth("g"))
	}
	if v, _ := r.Defs.Top("g"); v.String() != "2" {
		t.Fatalf("value-equal sig-undef removed the wrong entry: top is %v", v)
	}

	// A twin that cannot find its entry removes NOTHING — never a guess by
	// position. Both miss shapes: an ID nothing carries, and an unequal
	// value.
	missing := NewInteger(3)
	missing.ID = "twin-none"
	ApplyBindTwin(r, BindTransition{Kind: BindSigUndef, Name: "g"}, DefEntry{Body: missing})
	ApplyBindTwin(r, BindTransition{Kind: BindSigUndef, Name: "g"}, DefEntry{Body: NewInteger(9)})
	if r.Defs.Depth("g") != 1 {
		t.Fatalf("an unmatched sig-undef twin must be a no-op: depth %d", r.Defs.Depth("g"))
	}
}

// RestoreBindingsForReplay is RestoreBindings minus the module-ledger
// rollback: the run that follows REPLAYS the pass's bindings, so imports
// stay done — rolling the ledger back would re-import (and re-run module
// body effects) on the next request.
func TestRestoreBindingsForReplayKeepsModuleLedger(t *testing.T) {
	r := newTestRegistry(t)
	if r.Modules == nil {
		t.Skip("registry has no module ledger")
	}
	minted := r.Types.MintType("ReplayT", TInteger)
	r.Defs.PushType("ReplayT", minted, NewTypeLiteral(TInteger))
	snap := r.SnapshotBindings()

	// The pass: a def, an undef's retirement, an import.
	r.Defs.Push("passed", NewInteger(1))
	r.Defs.Pop("ReplayT")
	r.Types.Retire(minted)
	r.Modules.Loaded["imported"] = ModuleDesc{Ref: "imported"}

	r.RestoreBindingsForReplay(snap)

	if _, ok := r.Defs.Top("passed"); ok {
		t.Error("the def was not rolled back for its twin to replay")
	}
	if r.Types.LookupByID(minted.ID) != minted {
		t.Error("the retirement was not re-admitted for the undef twin to re-apply")
	}
	if _, ok := r.Modules.Loaded["imported"]; !ok {
		t.Error("the module ledger must stay PASS-FINAL — rolling it back re-imports")
	}

	// Zero-value and nil safety, like every sandbox entry point.
	r.Defs.Push("held", NewInteger(2))
	r.RestoreBindingsForReplay(BindingSandbox{})
	if _, ok := r.Defs.Top("held"); !ok {
		t.Error("the zero sandbox must restore nothing")
	}
	var nilReg *Registry
	nilReg.RestoreBindingsForReplay(BindingSandbox{valid: true})
}
