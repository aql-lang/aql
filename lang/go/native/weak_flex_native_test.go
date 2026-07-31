package native

import (
	"strings"
	"testing"

	eng "github.com/boru-lang/boru/eng/go"
	"github.com/boru-lang/boru/eng/go/parser"
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
		"[boru/weak_value_error]",
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

// ── check-mode mirrors + typed-write plumbing (Codex round) ──────────

// TestWeakReturnsFnsArityGuards pins the no-panic contract for a
// ReturnsFn invoked with a mismatched arg slice (ADR-005): each falls
// back to a bare carrier of its declared return.
func TestWeakReturnsFnsArityGuards(t *testing.T) {
	r := mirrorReg(t)
	for _, tc := range []struct {
		name string
		fn   func([]Value, *Registry) []Value
		want *Type
	}{
		{"set map", weakSetMapReturns, TWeakFlexMap},
		{"set list", weakSetListReturns, TWeakFlexList},
		{"append list", weakAppendListReturns, TWeakFlexList},
		{"append xml", weakAppendXmlReturns, TWeakFlexXml},
	} {
		out := tc.fn(nil, r)
		if len(out) != 1 || !out[0].Parent.Equal(tc.want) {
			t.Errorf("%s: guard residual = %v, want one carrier of the declared weak type", tc.name, out)
		}
	}
	if len(r.Check.Diagnostics) != 0 {
		t.Errorf("guard arms emitted diagnostics: %+v", r.Check.Diagnostics)
	}
}

// countWeakDiags counts weak_value_error diagnostics on r.
func countWeakDiags(r *Registry) int {
	n := 0
	for _, d := range r.Check.Diagnostics {
		if d.Code == "weak_value_error" {
			n++
		}
	}
	return n
}

// TestWeakValueMirrorBranches walks every arm of the check-time weak-
// domain mirror: inert outside check mode, silent on dynamic values and
// valid handles, flagging on a concrete refusal, a bare type literal,
// and a None-typed carrier (single inhabitant — statically none).
func TestWeakValueMirrorBranches(t *testing.T) {
	// Outside check mode the mirror is inert.
	plain := weakReg(t)
	weakValueMirror(plain, eng.NewMap(eng.NewOrderedMap()), "set", "WeakFlexMap")
	if len(plain.Check.Diagnostics) != 0 {
		t.Fatalf("non-check registry collected diagnostics: %+v", plain.Check.Diagnostics)
	}

	r := mirrorReg(t)
	wm := eng.NewWeakFlexMap()
	wl := eng.NewWeakFlexList()
	wx := eng.NewWeakFlexXml("a")

	// Dynamic value: proves nothing, stays silent.
	weakSetMapReturns([]Value{eng.NewAtom("k"), NewCarrier(TMap), wm}, r)
	if n := countWeakDiags(r); n != 0 {
		t.Fatalf("dynamic value flagged: %d", n)
	}
	// Valid handle: silent.
	flexOM := eng.NewOrderedMap()
	weakSetMapReturns([]Value{eng.NewAtom("k"), eng.NewFlexMap(flexOM), wm}, r)
	if n := countWeakDiags(r); n != 0 {
		t.Fatalf("valid handle flagged: %d", n)
	}
	// Concrete refusal through each ReturnsFn (distinct containers keep
	// the dedupe key distinct).
	weakSetMapReturns([]Value{eng.NewAtom("k"), eng.NewMap(eng.NewOrderedMap()), wm}, r)
	weakSetListReturns([]Value{eng.NewInteger(0), eng.NewList([]Value{eng.NewInteger(1)}), wl}, r)
	weakAppendListReturns([]Value{eng.NewMap(eng.NewOrderedMap()), wl}, r)
	weakAppendXmlReturns([]Value{eng.NewList([]Value{eng.NewInteger(1)}), wx}, r)
	if n := countWeakDiags(r); n != 4 {
		t.Fatalf("refusals flagged = %d, want 4: %+v", n, r.Check.Diagnostics)
	}
	// A bare type literal is statically refused too.
	weakSetMapReturns([]Value{eng.NewAtom("k"), eng.NewTypeLiteral(TInteger), wm}, r)
	if n := countWeakDiags(r); n != 5 {
		t.Fatalf("type literal not flagged: %d", n)
	}
	// A None-typed carrier is provably none (single inhabitant).
	weakSetMapReturns([]Value{eng.NewAtom("k"), NewCarrier(eng.TNone), wm}, r)
	if n := countWeakDiags(r); n != 6 {
		t.Fatalf("None carrier not flagged: %d", n)
	}
}

// TestMakeNodeReturnsTagCarry pins the check residual's element-tag
// carry through make — mirroring exactly what the runtime carries per
// target: weak targets keep header AND typed-literal tags, flex targets
// keep the header tag only (FlexDeepCopy), other targets keep none.
func TestMakeNodeReturnsTagCarry(t *testing.T) {
	r := mirrorReg(t)
	fn := makeNodeReturns()
	intLit := eng.NewTypeLiteral(TInteger)

	om := eng.NewOrderedMap()
	om.Set("a", eng.NewInteger(1))
	headerMap := eng.NewMap(om)
	headerMap.SetElemConstraint(intLit)
	literalMap := eng.NewValueRaw(TMap, eng.ChildTypeInfo{
		Child:   intLit,
		Entries: []eng.ChildEntry{{Key: "a", Value: eng.NewInteger(1)}},
	})

	for _, tc := range []struct {
		name    string
		target  *Type
		src     Value
		carried bool
	}{
		{"weak map, header source", TWeakFlexMap, headerMap, true},
		{"weak map, typed literal", TWeakFlexMap, literalMap, true},
		{"weak list, header source", TWeakFlexList, headerMap, true},
		{"flex map, header source", TFlexMap, headerMap, true},
		{"flex map, typed literal (runtime drops it)", TFlexMap, literalMap, false},
		{"plain Map target (no tag home)", TMap, headerMap, false},
	} {
		out := fn([]Value{eng.NewTypeLiteral(tc.target), tc.src}, r)
		if len(out) != 1 {
			t.Fatalf("%s: residual = %v", tc.name, out)
		}
		_, ok := out[0].ElemConstraint()
		if ok != tc.carried {
			t.Errorf("%s: carried=%v, want %v", tc.name, ok, tc.carried)
		}
	}
	// Arity and non-bare-target guards pass through untouched.
	if out := fn([]Value{eng.NewTypeLiteral(TWeakFlexMap)}, r); len(out) != 1 {
		t.Errorf("short-args residual = %v", out)
	}
	if out := fn([]Value{eng.NewMap(om), headerMap}, r); len(out) != 1 {
		t.Errorf("concrete-target residual = %v", out)
	}
}

// TestWeakHandlersEnforceElemTag pins the runtime d2AdoptTyped wiring
// in the weak set/append handlers: a tagged weak container refuses a
// non-conforming write with type_error and accepts a conforming one.
func TestWeakHandlersEnforceElemTag(t *testing.T) {
	r := weakReg(t)
	intLit := eng.NewTypeLiteral(TInteger)

	wm := eng.NewWeakFlexMap()
	wm.SetElemConstraint(intLit)
	if _, err := setWeakFlexMapHandler([]Value{eng.NewAtom("k"), eng.NewString("x"), wm}, nil, nil, r); err == nil || !strings.Contains(err.Error(), "type_error") {
		t.Fatalf("map: non-conforming write accepted: %v", err)
	}
	if _, err := setWeakFlexMapHandler([]Value{eng.NewAtom("k"), eng.NewInteger(1), wm}, nil, nil, r); err != nil {
		t.Fatalf("map: conforming write refused: %v", err)
	}

	wl := eng.NewWeakFlexList()
	wl.SetElemConstraint(intLit)
	if _, err := appendWeakElemHandler([]Value{eng.NewString("x"), wl}, nil, nil, r); err == nil || !strings.Contains(err.Error(), "type_error") {
		t.Fatalf("list append: non-conforming write accepted: %v", err)
	}
	if _, err := appendWeakElemHandler([]Value{eng.NewInteger(1), wl}, nil, nil, r); err != nil {
		t.Fatalf("list append: conforming write refused: %v", err)
	}
	if _, err := setWeakFlexListHandler([]Value{eng.NewInteger(0), eng.NewString("x"), wl}, nil, nil, r); err == nil || !strings.Contains(err.Error(), "type_error") {
		t.Fatalf("list set: non-conforming write accepted: %v", err)
	}
	if _, err := setWeakFlexListHandler([]Value{eng.NewInteger(0), eng.NewInteger(2), wl}, nil, nil, r); err != nil {
		t.Fatalf("list set: conforming write refused: %v", err)
	}
}
