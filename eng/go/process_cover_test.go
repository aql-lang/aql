package eng

import (
	"bytes"
	"errors"
	"testing"
)

// ADR-008 coverage for the small ProcessRuntime / Process accessor and
// guard arms not exercised by the behavioural suite in process_test.go.

func TestSyncOutputNilAndIdempotent(t *testing.T) {
	rt := NewProcessRuntime()
	defer rt.Shutdown()

	if got := rt.SyncOutput(nil); got != nil {
		t.Errorf("SyncOutput(nil) = %v, want nil", got)
	}
	var buf bytes.Buffer
	first := rt.SyncOutput(&buf)
	sw, ok := first.(*SyncWriter)
	if !ok {
		t.Fatalf("SyncOutput = %T, want *SyncWriter", first)
	}
	// Wrapping an already-synchronised writer must not double-wrap.
	if again := rt.SyncOutput(sw); again != first {
		t.Errorf("SyncOutput(SyncWriter) = %v, want the same wrapper back", again)
	}
}

func TestShutdownIdempotent(t *testing.T) {
	rt := NewProcessRuntime()
	rt.Shutdown()
	rt.Shutdown() // second call takes the already-down early return
	if !rt.Down() {
		t.Error("runtime should report down after Shutdown")
	}
}

func TestLookupPid(t *testing.T) {
	rt := NewProcessRuntime()
	defer rt.Shutdown()
	p := NewProcess(rt, 0, "")
	if err := rt.Insert(p); err != nil {
		t.Fatal(err)
	}
	got, ok := rt.LookupPid(p.ID)
	if !ok || got != p {
		t.Errorf("LookupPid(%q) = %v, %v; want the spawned process", p.ID, got, ok)
	}
	if _, ok := rt.LookupPid("P_nope"); ok {
		t.Error("LookupPid of an unknown id must miss")
	}
}

func TestRegisterNameAfterShutdown(t *testing.T) {
	rt := NewProcessRuntime()
	p := NewProcess(rt, 0, "")
	if err := rt.Insert(p); err != nil {
		t.Fatal(err)
	}
	rt.Shutdown()
	if err := rt.RegisterName("late", p); !errors.Is(err, ErrRuntimeDown) {
		t.Errorf("RegisterName after Shutdown = %v, want ErrRuntimeDown", err)
	}
}

func TestRegisterNameRebindStripsPrevious(t *testing.T) {
	rt := NewProcessRuntime()
	defer rt.Shutdown()
	a := NewProcess(rt, 0, "")
	b := NewProcess(rt, 0, "")
	if err := rt.Insert(a); err != nil {
		t.Fatal(err)
	}
	if err := rt.Insert(b); err != nil {
		t.Fatal(err)
	}
	if err := rt.RegisterName("worker", a); err != nil {
		t.Fatal(err)
	}
	// Rebinding moves the name: the previous holder keeps running but
	// loses it (the RegisterName prev-strip arm).
	if err := rt.RegisterName("worker", b); err != nil {
		t.Fatal(err)
	}
	if got, ok := rt.Whereis("worker"); !ok || got != b {
		t.Errorf("worker resolves to %v, want the rebound process", got)
	}
	if a.Name() != "" {
		t.Errorf("previous holder kept name %q, want stripped", a.Name())
	}
}

func TestNewProcessDefaultsAndAccessors(t *testing.T) {
	rt := NewProcessRuntime()
	defer rt.Shutdown()
	p := NewProcess(rt, 0, "") // overflow "" defaults to OverflowBlock
	if p.Runtime() != rt {
		t.Error("Runtime() must return the owning runtime")
	}
	select {
	case <-p.Done():
		t.Fatal("Done() closed before Close")
	default:
	}
	if p.Closed() {
		t.Error("Closed() true before Close")
	}
	p.Close()
	p.Close() // idempotent second close takes the early return
	if !p.Closed() {
		t.Error("Closed() false after Close")
	}
	select {
	case <-p.Done():
	default:
		t.Error("Done() must be closed after Close")
	}
}

func TestSendBlockOverflowRuntimeDownWake(t *testing.T) {
	rt := NewProcessRuntime()
	p := NewProcess(rt, 1, OverflowBlock)
	if err := rt.Insert(p); err != nil {
		t.Fatal(err)
	}
	if err := p.Send(NewInteger(1)); err != nil {
		t.Fatal(err)
	}
	// Second send blocks on the full mailbox; the runtime shutdown must
	// wake it with ErrRuntimeDown (the OverflowBlock down-check arm).
	done := make(chan error, 1)
	go func() { done <- p.Send(NewInteger(2)) }()
	rt.Shutdown()
	if err := <-done; !errors.Is(err, ErrRuntimeDown) {
		t.Errorf("blocked send after Shutdown = %v, want runtime-down/closed", err)
	}
}
