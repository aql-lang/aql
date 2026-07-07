package native

import (
	"testing"
)

// Seam-9 coverage (W9_nativeC) for tabnas.go (OptsToMap / jsonRoundTrip)
// and native_misc.go (write / import handler error propagation).

func TestW9OptsToMapReject(t *testing.T) {
	// A non-map value yields nil (161.9,163.3).
	if m := OptsToMap(NewInteger(1)); m != nil {
		t.Fatalf("OptsToMap(Integer) = %v, want nil", m)
	}
}

func TestW9JsonRoundTripUnmarshalError(t *testing.T) {
	saved := jsonMarshal
	t.Cleanup(func() { jsonMarshal = saved })
	// Return bytes that marshal "cleanly" but do not parse back (193.48,195.3).
	jsonMarshal = func(any) ([]byte, error) { return []byte("{not-json"), nil }
	if _, err := jsonRoundTrip(map[string]any{"a": 1}); err == nil {
		t.Fatalf("jsonRoundTrip must surface the Unmarshal error")
	}
}

func TestW9WriteHandlersFileOpsError(t *testing.T) {
	newR := func() *Registry {
		r := seam5Reg(t)
		w9SetFileOps(t, r, w9FailMkdirOps{}) // WriteFile errors
		return r
	}
	emptyOpts := NewMap(NewOrderedMap())

	// writeAnyHandler (340.16,342.3).
	if _, err := writeAnyHandler([]Value{NewString("f"), NewInteger(1)}, nil, nil, newR()); err == nil {
		t.Fatalf("writeAnyHandler with failing FS must error")
	}
	// writeOptsHandler (307.16,309.3).
	if _, err := writeOptsHandler([]Value{NewString("f"), NewString("data"), emptyOpts}, nil, nil, newR()); err == nil {
		t.Fatalf("writeOptsHandler with failing FS must error")
	}
	// writeAnyOptsHandler (358.16,360.3).
	if _, err := writeAnyOptsHandler([]Value{NewString("f"), NewInteger(1), emptyOpts}, nil, nil, newR()); err == nil {
		t.Fatalf("writeAnyOptsHandler with failing FS must error")
	}
}

func TestW9ImportFileHandlerMissingFile(t *testing.T) {
	r := seam5Reg(t)
	// A nonexistent file path fails loadFileModule (599.17,601.4).
	if _, err := importFileHandler([]Value{NewString("/nonexistent/w9-xyz-999.aql")}, nil, nil, r); err == nil {
		t.Fatalf("importFileHandler(missing file) must error")
	}
}
