package eng

import (
	"errors"
	"testing"
)

func TestArgsStackPushPopTop(t *testing.T) {
	as := NewArgsStack()

	// Empty state.
	if v, ok, err := as.Top(); err != nil || ok || v.Data != nil {
		t.Errorf("empty Top = (%v, %v, %v), want (zero, false, nil)", v, ok, err)
	}
	if popped, err := as.Pop(); err != nil || popped {
		t.Errorf("empty Pop = (%v, %v), want (false, nil)", popped, err)
	}

	// Push two entries.
	if err := as.Push(NewInteger(1)); err != nil {
		t.Fatalf("Push 1: %v", err)
	}
	if err := as.Push(NewInteger(2)); err != nil {
		t.Fatalf("Push 2: %v", err)
	}

	// Top sees the last push.
	v, ok, err := as.Top()
	if err != nil || !ok {
		t.Fatalf("Top after pushes = (%v, %v, %v)", v, ok, err)
	}
	n, _ := AsInteger(v)
	if n != 2 {
		t.Errorf("Top = %d, want 2", n)
	}

	// Pop returns true once per entry, then false.
	if popped, err := as.Pop(); err != nil || !popped {
		t.Errorf("first Pop = (%v, %v), want (true, nil)", popped, err)
	}
	if popped, err := as.Pop(); err != nil || !popped {
		t.Errorf("second Pop = (%v, %v), want (true, nil)", popped, err)
	}
	if popped, err := as.Pop(); err != nil || popped {
		t.Errorf("third Pop = (%v, %v), want (false, nil)", popped, err)
	}
}

// TestArgsStackNilReceiver verifies that every method surfaces a
// non-nil error when called on a nil *ArgsStack — the methods no
// longer silently no-op. Empty-stack flow control is preserved via
// the bool returns of Pop / Top.
func TestArgsStackNilReceiver(t *testing.T) {
	var as *ArgsStack

	if err := as.Push(NewInteger(1)); !errors.Is(err, errArgsStackNil) {
		t.Errorf("nil Push err = %v, want errArgsStackNil", err)
	}
	popped, err := as.Pop()
	if popped {
		t.Errorf("nil Pop popped = true, want false")
	}
	if !errors.Is(err, errArgsStackNil) {
		t.Errorf("nil Pop err = %v, want errArgsStackNil", err)
	}
	v, ok, err := as.Top()
	if ok || v.Data != nil {
		t.Errorf("nil Top = (%v, %v, ...), want (zero, false, ...)", v, ok)
	}
	if !errors.Is(err, errArgsStackNil) {
		t.Errorf("nil Top err = %v, want errArgsStackNil", err)
	}
}
