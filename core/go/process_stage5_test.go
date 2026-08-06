package core

// Stage-5 coverage tests for process.go — the BEAM-style process runtime
// (pid table, name registry, bounded mailboxes, shutdown). White-box
// tests: they drive Send/PopFront directly and synchronize through the
// runtime's own wake discipline (Send's broadcast, waitWithWake's 100ms
// self-wake, and explicit cond broadcasts) — never bare sleeps as
// assertions. Every goroutine started here is joined before the test
// returns, and every runtime is shut down.

import (
	"bytes"
	"errors"
	"testing"
	"time"
)

func TestStage5SyncOutput(t *testing.T) {
	rt := NewProcessRuntime()
	defer rt.Shutdown()

	if got := rt.SyncOutput(nil); got != nil {
		t.Errorf("SyncOutput(nil) = %v, want nil", got)
	}

	buf := &bytes.Buffer{}
	first := rt.SyncOutput(buf)
	sw, ok := first.(*SyncWriter)
	if !ok || sw.Unwrap() != buf {
		t.Fatalf("SyncOutput(buf) = %T, want a *SyncWriter around buf", first)
	}
	if again := rt.SyncOutput(buf); again != first {
		t.Errorf("SyncOutput must cache one wrapper per writer: got %p, want %p", again, first)
	}
	if same := rt.SyncOutput(sw); same != first {
		t.Errorf("SyncOutput(*SyncWriter) must return it unchanged, got %v", same)
	}
}

func TestStage5ShutdownIdempotentAndGates(t *testing.T) {
	rt := NewProcessRuntime()
	if rt.Down() {
		t.Fatal("a fresh runtime must not be down")
	}
	p := NewProcess(rt, 4, OverflowBlock)
	defer p.Close()
	if err := rt.Insert(p); err != nil {
		t.Fatalf("Insert: %v", err)
	}

	rt.Shutdown()
	if !rt.Down() {
		t.Fatal("Down must report true after Shutdown")
	}
	rt.Shutdown() // idempotent second call takes the early-return path

	if err := rt.Insert(NewProcess(rt, 4, OverflowBlock)); !errors.Is(err, ErrRuntimeDown) {
		t.Errorf("Insert after shutdown = %v, want ErrRuntimeDown", err)
	}
	if err := rt.RegisterName("stage5late", p); !errors.Is(err, ErrRuntimeDown) {
		t.Errorf("RegisterName after shutdown = %v, want ErrRuntimeDown", err)
	}
}

func TestStage5PidTableAndNames(t *testing.T) {
	rt := NewProcessRuntime()
	defer rt.Shutdown()

	p1 := NewProcess(rt, 4, OverflowBlock)
	defer p1.Close()
	if err := rt.Insert(p1); err != nil {
		t.Fatalf("Insert: %v", err)
	}
	if got, ok := rt.LookupPid(p1.ID); !ok || got != p1 {
		t.Fatalf("LookupPid(%q) = %v/%v, want p1", p1.ID, got, ok)
	}
	if _, ok := rt.LookupPid("P_stage5_missing"); ok {
		t.Error("LookupPid of an unknown id must miss")
	}

	if err := rt.RegisterName("stage5svc", p1); err != nil {
		t.Fatalf("RegisterName: %v", err)
	}
	if p1.Name() != "stage5svc" {
		t.Errorf("Name = %q, want stage5svc", p1.Name())
	}
	if got, ok := rt.Whereis("stage5svc"); !ok || got != p1 {
		t.Errorf("Whereis = %v/%v, want p1", got, ok)
	}
	if _, ok := rt.Whereis("stage5nobody"); ok {
		t.Error("Whereis of an unbound name must miss")
	}

	// Rebinding the name to p2 strips it from p1 (which keeps running).
	p2 := NewProcess(rt, 4, OverflowBlock)
	defer p2.Close()
	if err := rt.Insert(p2); err != nil {
		t.Fatalf("Insert p2: %v", err)
	}
	if err := rt.RegisterName("stage5svc", p2); err != nil {
		t.Fatalf("RegisterName rebind: %v", err)
	}
	if p1.Name() != "" || p2.Name() != "stage5svc" {
		t.Errorf("rebind left p1 %q / p2 %q, want \"\" / stage5svc", p1.Name(), p2.Name())
	}
	// Re-registering the same holder is a no-op rebind.
	if err := rt.RegisterName("stage5svc", p2); err != nil || p2.Name() != "stage5svc" {
		t.Errorf("self-rebind: err %v, name %q", err, p2.Name())
	}

	// Remove drops the pid and the registered name.
	rt.Remove(p2)
	if _, ok := rt.LookupPid(p2.ID); ok {
		t.Error("Remove must drop the pid table entry")
	}
	if _, ok := rt.Whereis("stage5svc"); ok {
		t.Error("Remove must drop the registered name")
	}
	if p2.Name() != "" {
		t.Errorf("Remove must clear the process's name, got %q", p2.Name())
	}
	rt.Remove(p1) // p1 lost its name above: the unnamed removal path

	// UnregisterName: a bound name goes away; unknown names are a no-op.
	p3 := NewProcess(rt, 4, OverflowBlock)
	defer p3.Close()
	if err := rt.RegisterName("stage5tmp", p3); err != nil {
		t.Fatalf("RegisterName p3: %v", err)
	}
	rt.UnregisterName("stage5tmp")
	if _, ok := rt.Whereis("stage5tmp"); ok || p3.Name() != "" {
		t.Error("UnregisterName must drop the binding and clear the name")
	}
	rt.UnregisterName("stage5neverbound")
}

func TestStage5NewProcessLifecycle(t *testing.T) {
	rt := NewProcessRuntime()
	defer rt.Shutdown()

	p := NewProcess(rt, 0, "") // "" defaults to block; bound<=0 is unbounded
	if p.ID == "" || p.overflow != OverflowBlock || p.bound != 0 {
		t.Fatalf("NewProcess defaults: id %q overflow %q bound %d", p.ID, p.overflow, p.bound)
	}
	if p.Runtime() != rt {
		t.Error("Runtime must return the owning runtime")
	}
	select {
	case <-p.Done():
		t.Fatal("Done must not be closed before Close")
	default:
	}
	if p.Closed() {
		t.Fatal("Closed must be false before Close")
	}

	p.Close()
	if !p.Closed() {
		t.Error("Closed must be true after Close")
	}
	select {
	case <-p.Done():
	default:
		t.Error("Done must be closed after Close")
	}
	p.Close() // idempotent second close takes the early-return path

	// Sends to a closed process are silently dropped (fire-and-forget).
	if err := p.Send(NewString("late")); err != nil {
		t.Errorf("Send to closed = %v, want nil", err)
	}
	if p.MailboxDepth() != 0 {
		t.Errorf("a dropped send must not enqueue, depth %d", p.MailboxDepth())
	}
}

func TestStage5SendOverflowFailAndDrop(t *testing.T) {
	rt := NewProcessRuntime()
	defer rt.Shutdown()

	fail := NewProcess(rt, 1, OverflowFail)
	defer fail.Close()
	if err := fail.Send(NewInteger(1)); err != nil {
		t.Fatalf("first send: %v", err)
	}
	if err := fail.Send(NewInteger(2)); !errors.Is(err, ErrMailboxOverload) {
		t.Errorf("overfull fail send = %v, want ErrMailboxOverload", err)
	}

	drop := NewProcess(rt, 1, OverflowDrop)
	defer drop.Close()
	if err := drop.Send(NewInteger(1)); err != nil {
		t.Fatalf("first send: %v", err)
	}
	if err := drop.Send(NewInteger(2)); err != nil {
		t.Errorf("overfull drop send = %v, want nil", err)
	}
	if drop.MailboxDepth() != 1 {
		t.Errorf("drop policy must discard the NEW message, depth %d", drop.MailboxDepth())
	}
	msg, ok, err := drop.PopFront(0, true)
	if err != nil || !ok {
		t.Fatalf("PopFront: %v/%v", ok, err)
	}
	if n, _ := AsInteger(msg); n != 1 {
		t.Errorf("drop policy must keep the OLD message, got %v", msg)
	}
}

func TestStage5SendBlockedOnDownRuntime(t *testing.T) {
	rt := NewProcessRuntime()
	p := NewProcess(rt, 1, OverflowBlock)
	defer p.Close()
	if err := p.Send(NewInteger(1)); err != nil {
		t.Fatalf("fill: %v", err)
	}
	rt.Shutdown()
	// Full mailbox, block policy, down runtime: no progress is possible.
	if err := p.Send(NewInteger(2)); !errors.Is(err, ErrRuntimeDown) {
		t.Errorf("blocked send on a down runtime = %v, want ErrRuntimeDown", err)
	}
}

func TestStage5PopFrontImmediatePaths(t *testing.T) {
	rt := NewProcessRuntime()
	defer rt.Shutdown()

	p := NewProcess(rt, 4, OverflowBlock)
	defer p.Close()
	for _, s := range []string{"a", "b"} {
		if err := p.Send(NewString(s)); err != nil {
			t.Fatalf("Send %q: %v", s, err)
		}
	}
	// Consume-front discipline: the OLDEST message pops first.
	msg, ok, err := p.PopFront(0, true)
	if err != nil || !ok {
		t.Fatalf("PopFront: %v/%v", ok, err)
	}
	if s, _ := AsString(msg); s != "a" {
		t.Errorf("PopFront = %v, want the front message a", msg)
	}
	if p.MailboxDepth() != 1 {
		t.Errorf("depth after pop = %d, want 1", p.MailboxDepth())
	}

	// Empty mailbox with an already-expired deadline: the `after` clause
	// path returns ok=false with no error.
	empty := NewProcess(rt, 4, OverflowBlock)
	defer empty.Close()
	if _, ok, err := empty.PopFront(0, true); ok || err != nil {
		t.Errorf("PopFront(0, deadline) on empty = %v/%v, want false/nil", ok, err)
	}
}

func TestStage5PopFrontDeadlineTimerExpires(t *testing.T) {
	rt := NewProcessRuntime()
	defer rt.Shutdown()

	p := NewProcess(rt, 4, OverflowBlock)
	defer p.Close()
	start := time.Now()
	_, ok, err := p.PopFront(30*time.Millisecond, true)
	if ok || err != nil {
		t.Fatalf("PopFront(30ms) on empty = %v/%v, want false/nil", ok, err)
	}
	// The deadline check guarantees no early return; the wake came from
	// the deadline timer's broadcast.
	if elapsed := time.Since(start); elapsed < 30*time.Millisecond {
		t.Errorf("PopFront returned after %v, before its 30ms deadline", elapsed)
	}
}

func TestStage5PopFrontDownRuntime(t *testing.T) {
	rt := NewProcessRuntime()
	p := NewProcess(rt, 4, OverflowBlock)
	defer p.Close()
	rt.Shutdown()
	if _, _, err := p.PopFront(0, false); !errors.Is(err, ErrRuntimeDown) {
		t.Errorf("PopFront on a down runtime = %v, want ErrRuntimeDown", err)
	}
	// The deadline variant reports the same shutdown error (checked
	// before any waiting, so the one-second timeout never elapses).
	if _, _, err := p.PopFront(time.Second, true); !errors.Is(err, ErrRuntimeDown) {
		t.Errorf("PopFront(deadline) on a down runtime = %v, want ErrRuntimeDown", err)
	}
}

// TestStage5PopFrontBlocksUntilSend parks a receiver in the no-deadline
// wait loop, lets its 100ms self-wake lap at least once, then wakes it
// for real with a Send (which broadcasts). The sleep only biases the
// schedule; the assertions hold under any interleaving.
func TestStage5PopFrontBlocksUntilSend(t *testing.T) {
	rt := NewProcessRuntime()
	defer rt.Shutdown()

	p := NewProcess(rt, 4, OverflowBlock)
	defer p.Close()

	type res struct {
		msg Value
		ok  bool
		err error
	}
	entered := make(chan struct{})
	done := make(chan res, 1)
	go func() {
		close(entered)
		m, ok, err := p.PopFront(0, false)
		done <- res{m, ok, err}
	}()

	<-entered
	time.Sleep(150 * time.Millisecond) // let the receiver park and self-wake once
	if err := p.Send(NewString("wake")); err != nil {
		t.Fatalf("Send: %v", err)
	}

	select {
	case r := <-done:
		if r.err != nil || !r.ok {
			t.Fatalf("PopFront = %v/%v, want a message", r.ok, r.err)
		}
		if s, _ := AsString(r.msg); s != "wake" {
			t.Errorf("PopFront = %v, want wake", r.msg)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("receiver never woke after Send")
	}
}

// TestStage5SendBlocksUntilSpace parks a sender on a full block-policy
// mailbox, then frees a slot and nudges the condition. The sender's
// 100ms self-wake is the backstop, so the assertions hold under any
// interleaving; the sleep only biases the schedule toward parking.
func TestStage5SendBlocksUntilSpace(t *testing.T) {
	rt := NewProcessRuntime()
	defer rt.Shutdown()

	p := NewProcess(rt, 1, OverflowBlock)
	defer p.Close()
	if err := p.Send(NewString("first")); err != nil {
		t.Fatalf("fill: %v", err)
	}

	entered := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		close(entered)
		done <- p.Send(NewString("second"))
	}()

	<-entered
	time.Sleep(50 * time.Millisecond) // let the sender observe full and park
	msg, ok, err := p.PopFront(0, true)
	if err != nil || !ok {
		t.Fatalf("PopFront: %v/%v", ok, err)
	}
	if s, _ := AsString(msg); s != "first" {
		t.Fatalf("PopFront = %v, want first", msg)
	}
	// PopFront does not signal; nudge the parked sender rather than
	// waiting out its self-wake. Harmless if it has not parked yet.
	p.mu.Lock()
	p.cond.Broadcast()
	p.mu.Unlock()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("blocked send = %v, want nil once space freed", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("sender never woke after space freed")
	}
	msg, ok, err = p.PopFront(0, true)
	if err != nil || !ok {
		t.Fatalf("PopFront second: %v/%v", ok, err)
	}
	if s, _ := AsString(msg); s != "second" {
		t.Errorf("PopFront = %v, want the blocked send's message, got %v", msg, msg)
	}
}

// TestStage5WaitWithWakeSelfTimer drives waitWithWake directly: nothing
// else touches this process, so only its own 100ms timer can end the
// wait — the periodic-wake callback is the deterministic wake path.
func TestStage5WaitWithWakeSelfTimer(t *testing.T) {
	rt := NewProcessRuntime()
	defer rt.Shutdown()
	p := NewProcess(rt, 4, OverflowBlock)
	defer p.Close()

	start := time.Now()
	p.mu.Lock()
	p.waitWithWake()
	p.mu.Unlock()
	if elapsed := time.Since(start); elapsed < 100*time.Millisecond {
		t.Errorf("waitWithWake returned after %v, before its 100ms self-wake", elapsed)
	}
}
