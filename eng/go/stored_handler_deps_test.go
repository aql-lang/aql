package eng

import "testing"

// TestStoredHandlerDepsBranches exercises the module-level dep capture behind the
// PR #243 comment-#2 fix: storedHandlerDeps records exactly the body words bound
// as a user def at the store site (so a later NotifyNameRebound can poison a stale
// ref). It walks every guard — nil / reg-less receiver, an empty-name word, a
// repeat, and a word that is NOT a user def (a kernel native) — plus storedFnDeps
// over a fn value with and without an own sig.
func TestStoredHandlerDepsBranches(t *testing.T) {
	var nilES *EmitState
	if nilES.storedHandlerDeps([]Value{NewWord("x")}) != nil {
		t.Fatal("nil EmitState must return nil deps")
	}
	if (&EmitState{}).storedHandlerDeps([]Value{NewWord("x")}) != nil {
		t.Fatal("reg-less EmitState must return nil deps")
	}

	r := runUnitReg(t)
	r.Defs.Push("helper", NewInteger(1)) // a user def the body reads
	es := NewEmitState()
	es.reg = r
	// helper → added; NewInteger → not a word (skipped); zzznotdef → not a def
	// (ok=false, skipped); helper → already in deps (repeat branch); "" → empty
	// name (skipped).
	body := []Value{NewWord("helper"), NewInteger(5), NewWord("zzznotdef"), NewWord("helper"), NewWord("")}
	deps := es.storedHandlerDeps(body)
	if deps == nil || !deps["helper"] || len(deps) != 1 {
		t.Fatalf("deps = %v, want exactly {helper}", deps)
	}
	// A body reading no user def yields nil.
	if d := es.storedHandlerDeps([]Value{NewWord("zzznotdef")}); d != nil {
		t.Fatalf("deps = %v, want nil (no user-def reads)", d)
	}

	// storedFnDeps: no own sig → nil; an own AQL body sig → its body's deps.
	if es.storedFnDeps(FnDefInfo{}) != nil {
		t.Fatal("storedFnDeps with no own sig must be nil")
	}
	fd := FnDefInfo{Signatures: []Signature{{Impl: &AQLImpl{Body: []Value{NewWord("helper")}}}}}
	if d := es.storedFnDeps(fd); d == nil || !d["helper"] {
		t.Fatalf("storedFnDeps = %v, want {helper}", d)
	}
}

// TestNotifyNameReboundBranches pins the poison propagation: a rebind of a name an
// already-created stored ref reads flips that ref's poisoned flag (→ Finalize skips
// the stamp → interpreter fallback), while a ref that does NOT read the name is
// left alone. The nil / inactive receiver guards are no-ops (a plain check pass
// must not poison anything).
func TestNotifyNameReboundBranches(t *testing.T) {
	var nilES *EmitState
	nilES.NotifyNameRebound("x") // nil receiver → no panic, no-op

	inactive := NewEmitState()
	inactive.Compilable = false // !active()
	inactive.storedFnRefs = []*CompiledFnRef{{depNames: map[string]bool{"x": true}}}
	inactive.NotifyNameRebound("x")
	if inactive.storedFnRefs[0].poisoned {
		t.Error("an inactive recorder must not poison")
	}

	es := NewEmitState()
	reads := &CompiledFnRef{depNames: map[string]bool{"helper": true}}
	other := &CompiledFnRef{depNames: map[string]bool{"kv": true}}
	es.storedFnRefs = []*CompiledFnRef{reads, other}
	es.NotifyNameRebound("helper")
	if !reads.poisoned {
		t.Error("a ref reading the rebound name must be poisoned")
	}
	if other.poisoned {
		t.Error("a ref not reading the rebound name must stay unpoisoned")
	}
}
