package core

import "testing"

// TestTruncateFrameDefs pins the VM deopt island's def-cleanup duty in core:
// every def binding installed since the snapshot is popped — a name the
// snapshot never held goes entirely, a name it held keeps its earlier
// depth — and a registry with nothing new is left alone.
func TestTruncateFrameDefs(t *testing.T) {
	r := newTestRegistry(t)
	r.Defs.Push("x", NewInteger(1))
	snapshot := r.Defs.Snapshot()
	r.Defs.Push("x", NewInteger(2))
	r.Defs.Push("y", NewInteger(3))
	TruncateFrameDefs(r, snapshot)
	if d := r.Defs.Depth("x"); d != 1 {
		t.Errorf("x keeps its pre-island depth: got %d", d)
	}
	if v, ok := r.Defs.Top("x"); !ok || v.String() != "1" {
		t.Errorf("x holds its pre-island value: %v %v", v, ok)
	}
	if d := r.Defs.Depth("y"); d != 0 {
		t.Errorf("a def the island made is gone: depth %d", d)
	}
	TruncateFrameDefs(r, r.Defs.Snapshot())
	if d := r.Defs.Depth("x"); d != 1 {
		t.Errorf("nothing new, nothing popped: %d", d)
	}
}
