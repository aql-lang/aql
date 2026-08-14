package compiler

import "testing"

// TestInlineCtxBoundaryLatch pins the inline context-boundary region
// mechanics (NUR054): the region latches the open-unit depth at entry, so
// InInlineCtxBoundary holds only while recording INLINE within the innermost
// region — a unit opened inside the region un-marks it until it closes, and
// nil receivers / empty stacks are safe no-ops.
func TestInlineCtxBoundaryLatch(t *testing.T) {
	var nilES *EmitState
	nilES.PushInlineCtxBoundary()
	nilES.PopInlineCtxBoundary()
	if nilES.InInlineCtxBoundary() {
		t.Fatal("nil EmitState must not report an inline region")
	}

	es := &EmitState{}
	es.PopInlineCtxBoundary() // empty-stack pop is a no-op
	if es.InInlineCtxBoundary() {
		t.Fatal("no region open: InInlineCtxBoundary must be false")
	}
	es.PushInlineCtxBoundary()
	if !es.InInlineCtxBoundary() {
		t.Fatal("region open at unit depth 0: must be inline")
	}
	// A unit opened inside the region breaks the latch equality — the unit's
	// body is bracketed by the VM's own context frame at run time.
	es.openUnitRecs = append(es.openUnitRecs, 0)
	if es.InInlineCtxBoundary() {
		t.Fatal("unit opened inside the region: must NOT be inline")
	}
	// A region entered INSIDE that unit is inline again at ITS depth.
	es.PushInlineCtxBoundary()
	if !es.InInlineCtxBoundary() {
		t.Fatal("inner region at unit depth 1: must be inline")
	}
	es.PopInlineCtxBoundary()
	es.openUnitRecs = es.openUnitRecs[:0]
	if !es.InInlineCtxBoundary() {
		t.Fatal("unit closed, outer region still open: must be inline again")
	}
	es.PopInlineCtxBoundary()
	if es.InInlineCtxBoundary() {
		t.Fatal("all regions closed: must not be inline")
	}
}
