package compiler

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

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
	nilES.noteInlineCtxRead("id")
	if nilES.inlineCtxArg([]core.Value{core.NewInteger(1)}) {
		t.Fatal("nil EmitState must not match an inline-ctx arg")
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

// TestInlineCtxReadNoting pins noteInlineCtxRead / inlineCtxArg: only reads
// resolved INLINE inside a region are noted (an empty ID and an out-of-region
// read are not), and inlineCtxArg matches any arg carrying a noted ID.
func TestInlineCtxReadNoting(t *testing.T) {
	es := &EmitState{}
	handle := core.NewCarrier(core.TStore)

	// Out of region / empty ID: not noted.
	es.noteInlineCtxRead(handle.ID)
	es.PushInlineCtxBoundary()
	es.noteInlineCtxRead("")
	if es.inlineCtxArg([]core.Value{handle}) {
		t.Fatal("nothing noted yet: inlineCtxArg must decline")
	}

	es.noteInlineCtxRead(handle.ID)
	if !es.inlineCtxArg([]core.Value{core.NewInteger(1), handle}) {
		t.Fatal("noted handle among args: inlineCtxArg must match")
	}
	other := core.NewCarrier(core.TStore)
	if es.inlineCtxArg([]core.Value{other}) {
		t.Fatal("an un-noted store handle must not match")
	}
	// The note is provenance, not region state: it outlives the region (a
	// handle that escapes the region is still the region's layer).
	es.PopInlineCtxBoundary()
	if !es.inlineCtxArg([]core.Value{handle}) {
		t.Fatal("noted handle must still match after the region closes")
	}
}
