package core

// Stage-5 coverage: stamp_report.go. The stamp-attribution log is pure
// in-memory state (arming, recording, snapshotting, resetting), so every
// arm is drivable directly: nil receivers, unarmed registries, and an
// armed registry's full record/report/reset cycle.

import "testing"

func stage5Registry(t *testing.T) *Registry {
	t.Helper()
	r, err := NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	return r
}

// TestStampLogNilAndDirect pins the *stampLog primitives: a nil log
// swallows record/reset silently; a real log appends and clears.
func TestStampLogNilAndDirect(t *testing.T) {
	var nilLog *stampLog
	nilLog.record(StampEvent{Name: "x"}) // must not panic
	nilLog.reset()                       // must not panic

	l := &stampLog{}
	l.record(StampEvent{Name: "a", Stamped: true})
	l.record(StampEvent{Name: "b", Reason: "capture"})
	if len(l.events) != 2 {
		t.Fatalf("record appended %d events, want 2", len(l.events))
	}
	l.reset()
	if l.events != nil {
		t.Errorf("reset must drop the events, got %v", l.events)
	}
}

// TestStampRegistryNilReceiver pins the nil-registry guards on the three
// Registry-level entry points.
func TestStampRegistryNilReceiver(t *testing.T) {
	var r *Registry
	if got := r.StampEvents(); got != nil {
		t.Errorf("nil registry StampEvents = %v, want nil", got)
	}
	r.RecordStampEvent(StampEvent{Name: "x"}) // must not panic
	r.ResetStampLog()                         // must not panic
}

// TestStampUnarmedRegistry pins the unarmed shape: a fresh registry has no
// stamp log, so the report reads nil and record/reset stay inert.
func TestStampUnarmedRegistry(t *testing.T) {
	r := stage5Registry(t)
	if got := r.StampEvents(); got != nil {
		t.Errorf("unarmed StampEvents = %v, want nil", got)
	}
	// Recording while unarmed routes to a nil log — silent by design.
	// The empty name still takes the anonymous-rename arm first.
	r.RecordStampEvent(StampEvent{})
	r.ResetStampLog()
	if got := r.StampEvents(); got != nil {
		t.Errorf("unarmed StampEvents after record/reset = %v, want nil", got)
	}
}

// TestStampArmedFlow pins the armed cycle: EnableRuntimeStamping arms the
// log, RecordStampEvent appends (renaming anonymous fns), StampEvents
// returns an independent copy, and ResetStampLog drops the attempts while
// staying armed.
func TestStampArmedFlow(t *testing.T) {
	r := stage5Registry(t)
	r.EnableRuntimeStamping()

	r.RecordStampEvent(StampEvent{Name: "handler", Stamped: true})
	r.RecordStampEvent(StampEvent{Reason: "captures locals"}) // anonymous

	evs := r.StampEvents()
	if len(evs) != 2 {
		t.Fatalf("StampEvents len = %d, want 2", len(evs))
	}
	if evs[0].Name != "handler" || !evs[0].Stamped {
		t.Errorf("first event = %+v, want named+stamped", evs[0])
	}
	if evs[1].Name != "(anonymous fn)" {
		t.Errorf("anonymous event name = %q, want %q", evs[1].Name, "(anonymous fn)")
	}
	if evs[1].Reason != "captures locals" {
		t.Errorf("anonymous event reason = %q", evs[1].Reason)
	}

	// The report is a COPY: mutating it must not touch the log.
	evs[0].Name = "clobbered"
	if again := r.StampEvents(); again[0].Name != "handler" {
		t.Errorf("StampEvents must return a copy; log shows %q", again[0].Name)
	}

	// Reset drops the attempts but leaves stamping armed.
	r.ResetStampLog()
	if got := r.StampEvents(); len(got) != 0 {
		t.Errorf("after reset StampEvents len = %d, want 0", len(got))
	}
	if !r.RuntimeStampingEnabled() {
		t.Error("reset must not disarm runtime stamping")
	}
	r.RecordStampEvent(StampEvent{Name: "later"})
	if got := r.StampEvents(); len(got) != 1 || got[0].Name != "later" {
		t.Errorf("post-reset record = %v", got)
	}
}
