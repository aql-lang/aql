package native

import (
	"strings"
	"testing"

	eng "github.com/aql-lang/aql/eng/go"
	"github.com/aql-lang/aql/eng/go/parser"
)

// weakRunErr runs source and returns the error (nil on success), for
// negative-path assertions that must not panic.
func weakRunErr(t *testing.T, src string) error {
	t.Helper()
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	toks, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	_, runErr := NewTop(r).Run(toks)
	return runErr
}

func weakReg(t *testing.T) *Registry {
	t.Helper()
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	return r
}

// ── direct handler arms (deterministic coverage of every branch) ─────

func TestSetWeakFlexMapHandlerArms(t *testing.T) {
	r := weakReg(t)
	w := eng.NewWeakFlexMap()
	// Success: scalar stores strongly, node returns.
	out, err := setWeakFlexMapHandler([]Value{eng.NewAtom("k"), eng.NewInteger(1), w}, nil, nil, r)
	if err != nil || len(out) != 1 || !eng.IsWeakFlexMap(out[0]) {
		t.Fatalf("success arm: %v", err)
	}
	// Refusal: immutable map.
	_, err = setWeakFlexMapHandler([]Value{eng.NewAtom("k"), eng.NewMap(eng.NewOrderedMap()), w}, nil, nil, r)
	if err == nil || !strings.Contains(err.Error(), "weak_value_error") {
		t.Fatalf("refusal arm: %v", err)
	}
	// Wrong container payload (a bare literal reaching the handler).
	_, err = setWeakFlexMapHandler([]Value{eng.NewAtom("k"), eng.NewInteger(1), eng.NewTypeLiteral(TWeakFlexMap)}, nil, nil, r)
	if err == nil || !strings.Contains(err.Error(), "expected a WeakFlexMap") {
		t.Fatalf("container arm: %v", err)
	}
}

func TestSetWeakFlexListHandlerArms(t *testing.T) {
	r := weakReg(t)
	w := eng.NewWeakFlexList()
	wd, _ := AsWeakFlexList(w)
	if refusal := wd.Append(eng.NewInteger(1)); refusal != nil {
		t.Fatal("fixture append refused")
	}
	// Success.
	out, err := setWeakFlexListHandler([]Value{eng.NewInteger(0), eng.NewInteger(9), w}, nil, nil, r)
	if err != nil || !eng.IsWeakFlexList(out[0]) {
		t.Fatalf("success arm: %v", err)
	}
	// Bounds.
	_, err = setWeakFlexListHandler([]Value{eng.NewInteger(5), eng.NewInteger(9), w}, nil, nil, r)
	if err == nil || !strings.Contains(err.Error(), "out of bounds for WeakFlexList") {
		t.Fatalf("bounds arm: %v", err)
	}
	// Non-integer index (unreachable via the sig; the guard is direct).
	_, err = setWeakFlexListHandler([]Value{eng.NewString("x"), eng.NewInteger(9), w}, nil, nil, r)
	if err == nil || !strings.Contains(err.Error(), "concrete integer") {
		t.Fatalf("index arm: %v", err)
	}
	// Refusal at index.
	_, err = setWeakFlexListHandler([]Value{eng.NewInteger(0), eng.NewList(nil), w}, nil, nil, r)
	if err == nil || !strings.Contains(err.Error(), "weak_value_error") {
		t.Fatalf("refusal arm: %v", err)
	}
	// Wrong container.
	_, err = setWeakFlexListHandler([]Value{eng.NewInteger(0), eng.NewInteger(1), eng.NewTypeLiteral(TWeakFlexList)}, nil, nil, r)
	if err == nil || !strings.Contains(err.Error(), "expected a WeakFlexList") {
		t.Fatalf("container arm: %v", err)
	}
}

func TestSetWeakFlexXmlHandlerArms(t *testing.T) {
	r := weakReg(t)
	w := eng.NewWeakFlexXml("a")
	out, err := setWeakFlexXmlHandler([]Value{eng.NewAtom("k"), eng.NewString("v"), w}, nil, nil, r)
	if err != nil || !eng.IsWeakFlexXml(out[0]) {
		t.Fatalf("success arm: %v", err)
	}
	_, err = setWeakFlexXmlHandler([]Value{eng.NewAtom("k"), eng.NewString("v"), eng.NewTypeLiteral(TWeakFlexXml)}, nil, nil, r)
	if err == nil || !strings.Contains(err.Error(), "expected a WeakFlexXml") {
		t.Fatalf("container arm: %v", err)
	}
}

func TestAppendWeakHandlersArms(t *testing.T) {
	r := weakReg(t)
	w := eng.NewWeakFlexList()
	out, err := appendWeakElemHandler([]Value{eng.NewInteger(1), w}, nil, nil, r)
	if err != nil || !eng.IsWeakFlexList(out[0]) {
		t.Fatalf("elem success: %v", err)
	}
	_, err = appendWeakElemHandler([]Value{eng.NewList(nil), w}, nil, nil, r)
	if err == nil || !strings.Contains(err.Error(), "weak_value_error") {
		t.Fatalf("elem refusal: %v", err)
	}
	_, err = appendWeakElemHandler([]Value{eng.NewInteger(1), eng.NewTypeLiteral(TWeakFlexList)}, nil, nil, r)
	if err == nil || !strings.Contains(err.Error(), "expected a WeakFlexList") {
		t.Fatalf("elem container: %v", err)
	}
	x := eng.NewWeakFlexXml("t")
	out, err = appendWeakXmlChildHandler([]Value{eng.NewString("hi"), x}, nil, nil, r)
	if err != nil || !eng.IsWeakFlexXml(out[0]) {
		t.Fatalf("xml success: %v", err)
	}
	_, err = appendWeakXmlChildHandler([]Value{eng.NewMap(eng.NewOrderedMap()), x}, nil, nil, r)
	if err == nil || !strings.Contains(err.Error(), "weak_value_error") {
		t.Fatalf("xml refusal: %v", err)
	}
	_, err = appendWeakXmlChildHandler([]Value{eng.NewString("hi"), eng.NewTypeLiteral(TWeakFlexXml)}, nil, nil, r)
	if err == nil || !strings.Contains(err.Error(), "expected a WeakFlexXml") {
		t.Fatalf("xml container: %v", err)
	}
}

// ── no-panic battery: bare weak type literals through the words ──────

func TestWeakTypeLiteralNoPanic(t *testing.T) {
	programs := []string{
		"set n/q 1 WeakFlexMap",
		"set 0 1 WeakFlexList",
		"set n/q 1 WeakFlexXml",
		"append 1 WeakFlexList",
		"append 1 WeakFlexXml",
		"make WeakFlexMap 42",
		"make WeakFlexList 42",
		"make WeakFlexXml 42",
	}
	for _, src := range programs {
		func() {
			defer func() {
				if p := recover(); p != nil {
					t.Fatalf("panic on %q: %v", src, p)
				}
			}()
			if err := weakRunErr(t, src); err == nil {
				t.Fatalf("expected an error for %q", src)
			}
		}()
	}
}

// ── diagnostic quality: the designed rendering, end to end ───────────

func TestWeakRefusalDiagnosticRendering(t *testing.T) {
	err := weakRunErr(t, "def w (make WeakFlexMap { }) set k/q {a:1} w")
	if err == nil {
		t.Fatal("expected refusal")
	}
	msg := err.Error()
	for _, want := range []string{
		"[aql/weak_value_error]",
		"cannot store an immutable Map in a WeakFlexMap",
		"= note:",
		"no independent identity",
		"= help:",
		"flex the map first",
	} {
		if !strings.Contains(msg, want) {
			t.Fatalf("diagnostic missing %q:\n%s", want, msg)
		}
	}
}
