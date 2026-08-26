package core

import "testing"

// The pending-fn-body queue (NUR105): `fn` and `afn` note a constructed fn
// value here, and the checker drains the queue at end of pass.
//
// These live in core's own suite deliberately. The production callers are in
// basic (FnConstruct, the afn handler) and lang (the two drain sites), so
// core profiled by ITS OWN suite alone — cover-gate-core — never reaches
// them, and the queue would be dead code in that profile. That is the
// cover-gate-core inversion, and the answer is a core-side test that drives
// the seam directly, not a pragma: nothing here is unreachable, it is merely
// reached from above.

// A queue entry is only taken while a check pass is active — the guard is
// what keeps a plain interpreter run from accumulating fn values forever.
func TestNoteFnBodyPendingOnlyWhenChecking(t *testing.T) {
	NoteFnBodyPending(nil, FnDefInfo{Name: "nil-registry"}) // must not panic

	r, err := NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	NoteFnBodyPending(r, FnDefInfo{Name: "inactive"})
	if got := len(r.Check.PendingFnBodies); got != 0 {
		t.Fatalf("inactive check must not queue: got %d entries", got)
	}

	done := r.Check.Begin()
	defer done()
	NoteFnBodyPending(r, FnDefInfo{Name: "queued"})
	if got := len(r.Check.PendingFnBodies); got != 1 {
		t.Fatalf("active check must queue: got %d entries", got)
	}
	if got := r.Check.PendingFnBodies[0].Fn.Name; got != "queued" {
		t.Fatalf("queued the wrong fn: %q", got)
	}
	// The registry rides on the ENTRY, not on the drain's argument: a handler
	// lambda inside an imported module reads that module's own words, which
	// the importer's registry cannot see.
	if r.Check.PendingFnBodies[0].Reg != r {
		t.Fatal("entry must carry the registry the body was written in")
	}
}

// The drain empties the queue, and keeps draining while the analysis it runs
// queues more — a body analysis runs `fn`, so the queue can grow under it.
func TestRunPendingFnBodyChecksDrainsUntilEmpty(t *testing.T) {
	RunPendingFnBodyChecks(nil) // must not panic

	r, err := NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	NoteFnBodyPending(r, FnDefInfo{Name: "never"})
	RunPendingFnBodyChecks(r) // inactive: no drain, and no panic

	done := r.Check.Begin()
	defer done()

	saved := AnalysisImpl.FnConstructionPass
	defer func() { AnalysisImpl.FnConstructionPass = saved }()

	var seen []string
	AnalysisImpl.FnConstructionPass = func(reg *Registry, _ string, fn FnDefInfo) {
		seen = append(seen, fn.Name)
		// Re-entrant growth: analysing `outer` constructs `inner`, which
		// queues while the drain is walking the batch it came from. Ranging
		// over the live slice would miss it; taking the batch and looping
		// does not.
		if fn.Name == "outer" {
			NoteFnBodyPending(reg, FnDefInfo{Name: "inner"})
		}
	}

	NoteFnBodyPending(r, FnDefInfo{Name: "outer"})
	RunPendingFnBodyChecks(r)

	if len(seen) != 2 || seen[0] != "outer" || seen[1] != "inner" {
		t.Fatalf("drain must reach the re-entrantly queued body, in order: %v", seen)
	}
	if got := len(r.Check.PendingFnBodies); got != 0 {
		t.Fatalf("drain must leave the queue empty: got %d entries", got)
	}
}

// Begin resets the queue, and Clone deep-copies it — a clone that shared the
// backing array would let a speculative sub-pass's queue entries surface in
// the parent after the sub-pass was discarded.
func TestPendingFnBodiesResetAndCloned(t *testing.T) {
	r, err := NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	done := r.Check.Begin()
	defer done()

	NoteFnBodyPending(r, FnDefInfo{Name: "a"})
	cl := r.Check.Clone()
	if len(cl.PendingFnBodies) != 1 || cl.PendingFnBodies[0].Fn.Name != "a" {
		t.Fatalf("clone must carry the queue: %v", cl.PendingFnBodies)
	}
	cl.PendingFnBodies[0] = PendingFnBody{Reg: r, Fn: FnDefInfo{Name: "mutated"}}
	if r.Check.PendingFnBodies[0].Fn.Name != "a" {
		t.Fatal("clone must not share the queue's backing array")
	}

	r.Check.Begin()
	if got := len(r.Check.PendingFnBodies); got != 0 {
		t.Fatalf("Begin must reset the queue: got %d entries", got)
	}
}
