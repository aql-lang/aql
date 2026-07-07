package native

import (
	"errors"
	"testing"
)

// Seam-9 coverage (W9_nativeC) for log_module.go: the built-in sink
// closures' nil-registry guards, renderJSON's marshal-failure fallback
// (via the jsonMarshal seam), asConcreteOrderedMap's reject arm, and
// checkLogInstall's allow tail.

func TestW9LogSinkNilRegistryGuards(t *testing.T) {
	lsr := newLogSinkRegistry()

	// console.emit with a nil registry short-circuits (242.38,244.5).
	if err := lsr.available["console"].emit(nil, LogRecord{}); err != nil {
		t.Fatalf("console.emit(nil) = %v", err)
	}
	// console.measure with a nil registry short-circuits (249.38,251.5).
	lsr.available["console"].measure(nil, Measurement{})

	// null.emit always discards (281.46,281.60).
	if err := lsr.available["null"].emit(nil, LogRecord{}); err != nil {
		t.Fatalf("null.emit = %v", err)
	}
}

func TestW9RenderJSONMarshalFallback(t *testing.T) {
	saved := jsonMarshal
	t.Cleanup(func() { jsonMarshal = saved })
	jsonMarshal = func(any) ([]byte, error) { return nil, errors.New("boom-marshal") }

	// A marshal failure falls back to renderText (499.16,501.3).
	out := renderJSON(LogRecord{Body: NewString("hi")})
	if out == "" {
		t.Fatalf("renderJSON fallback produced empty output")
	}
}

func TestW9AsConcreteOrderedMapReject(t *testing.T) {
	// A non-map value yields nil (604.28,606.3).
	if m := asConcreteOrderedMap(NewInteger(1)); m != nil {
		t.Fatalf("asConcreteOrderedMap(Integer) = %v, want nil", m)
	}
}

func TestW9CheckLogInstallAllows(t *testing.T) {
	r := seam5Reg(t)
	// The default registry's policy permits sink management, so the allow
	// tail returns nil (849.2,849.12).
	if err := checkLogInstall(r, "log", "console"); err != nil {
		t.Fatalf("checkLogInstall(default policy) = %v, want nil", err)
	}
}
