package native

import (
	"fmt"
	"testing"
)

// w8FailSpanRegistry swaps the span sub-registry constructor to a failing
// one for the duration of the test, restoring it afterwards.
func w8FailSpanRegistry(t *testing.T) {
	t.Helper()
	orig := newSubRegistry
	newSubRegistry = func(providers ...func(*Registry)) (*Registry, error) {
		return nil, fmt.Errorf("w8: forced span sub-registry failure")
	}
	t.Cleanup(func() { newSubRegistry = orig })
}

func TestW8BuildSpanInstanceRegistryError(t *testing.T) {
	w8FailSpanRegistry(t)
	lsr := NewLogSinkRegistry()
	if _, err := buildSpanInstance(&spanState{attrs: NewOrderedMap()}, lsr); err == nil {
		t.Fatal("buildSpanInstance must surface the sub-registry error")
	}
}

func TestW8LogSpanStartRegistryError(t *testing.T) {
	w8FailSpanRegistry(t)
	r := w8Reg(t)
	lsr := NewLogSinkRegistry()
	h := logSpanNative(lsr).Signatures[0].DispatchHandler()
	attrs := NewMap(NewOrderedMap())
	if _, err := h([]Value{NewString("s"), attrs}, nil, nil, r); err == nil {
		t.Fatal("Log.span must surface the sub-registry error")
	}
}

func TestW8LogCurrentSpanRegistryError(t *testing.T) {
	w8FailSpanRegistry(t)
	r := w8Reg(t)
	lsr := NewLogSinkRegistry()
	// Push an active span so current-span reaches buildSpanInstance.
	lsr.startSpan(r, "s", nil)
	h := logCurrentSpanNative(lsr).Signatures[0].DispatchHandler()
	if _, err := h(nil, nil, nil, r); err == nil {
		t.Fatal("Log.current-span must surface the sub-registry error")
	}
}

func TestW8SpanShapeReturnsRegistryError(t *testing.T) {
	w8FailSpanRegistry(t)
	r := w8Reg(t)
	lsr := NewLogSinkRegistry()
	// The check-mode shape falls back to a bare Map carrier on failure.
	out := spanShapeReturns(lsr)(nil, r)
	if len(out) != 1 {
		t.Fatalf("spanShapeReturns should still return one shape value, got %d", len(out))
	}
}
