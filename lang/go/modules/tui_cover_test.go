package modules

import (
	"errors"
	"strings"
	"testing"

	eng "github.com/aql-lang/aql/eng/go"
	"github.com/aql-lang/aql/lang/go/native"
	"github.com/aql-lang/aql/lang/go/policy"
	"github.com/aql-lang/aql/lang/go/tuikit"
)

// Direct-handler cover tests for tui.go, driving the arms the engine
// path cannot reach on a terminal-less CI (net_socket_cover_test.go
// pattern): crafted args, error-code assertions, seam swaps.

func tcReg(t *testing.T) *native.Registry {
	t.Helper()
	reg, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	return reg
}

func tcErrContains(t *testing.T, err error, want string) {
	t.Helper()
	if err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("error = %v, want substring %q", err, want)
	}
}

func tcCode(t *testing.T, err error) string {
	t.Helper()
	var ae *eng.AqlError
	if !errors.As(err, &ae) {
		t.Fatalf("not an AqlError: %v", err)
	}
	return ae.Code
}

// tcTerm builds an opened Terminal value over a fresh VirtualBackend.
func tcTerm(t *testing.T, cols, rows int) (native.Value, *tuikit.VirtualBackend) {
	t.Helper()
	vb := tuikit.NewVirtualBackend(cols, rows)
	info, err := vb.Open(tuikit.OpenOpts{})
	if err != nil {
		t.Fatal(err)
	}
	return newTerminalValue(vb, info), vb
}

// --- type registration ---

func TestTuiRegisterTypeDuplicateRecorded(t *testing.T) {
	prev := native.SwapTypeInitErrs(nil)
	t.Cleanup(func() { native.SwapTypeInitErrs(prev) })
	registerTuiType("Ideal/Terminal", 5011, terminalBehavior{})
	errs := native.TypeInitError()
	if errs == nil || !strings.Contains(errs.Error(), "tui: register") {
		t.Fatalf("duplicate registration not recorded: %v", errs)
	}
}

// --- behavior ---

func TestTuiTerminalBehaviorEqualFormat(t *testing.T) {
	a, _ := tcTerm(t, 1, 1)
	b, _ := tcTerm(t, 1, 1)
	beh := terminalBehavior{}
	if !beh.Equal(a, a) {
		t.Fatal("same handle not Equal")
	}
	if beh.Equal(a, b) {
		t.Fatal("distinct handles Equal")
	}
	if beh.Equal(a, native.NewInteger(1)) {
		t.Fatal("Terminal Equal Integer")
	}
	if got := beh.Format(a); !strings.HasPrefix(got, "Terminal<TM_") || strings.Contains(got, "closed") {
		t.Fatalf("open Format = %q", got)
	}
	ts, _ := asTerminal(a)
	_ = ts.close()
	if got := beh.Format(a); !strings.Contains(got, " closed>") {
		t.Fatalf("closed Format = %q", got)
	}
	if got := beh.Format(native.NewInteger(1)); got != "Terminal" {
		t.Fatalf("non-Terminal Format = %q", got)
	}
	if _, ok := asTerminal(native.NewInteger(1)); ok {
		t.Fatal("asTerminal accepted an Integer")
	}
}

// --- registration seam ---

func TestTuiRegisterHostValidation(t *testing.T) {
	reg := tcReg(t)
	open := func() (tuikit.Backend, error) { return tuikit.NewVirtualBackend(1, 1), nil }
	if err := RegisterHostTui(reg, TuiSpec{Name: "", Open: open}); err == nil ||
		!strings.Contains(err.Error(), "name must not be empty") {
		t.Fatalf("empty name = %v", err)
	}
	if err := RegisterHostTui(reg, TuiSpec{Name: "x", Open: nil}); err == nil ||
		!strings.Contains(err.Error(), "open must not be nil") {
		t.Fatalf("nil open = %v", err)
	}
}

// --- policy ---

func TestTuiPolicyArms(t *testing.T) {
	if err := checkTuiPolicy(nil, "open"); err != nil {
		t.Fatalf("nil registry: %v", err)
	}
	pol, err := policy.Load("sandbox")
	if err != nil {
		t.Fatal(err)
	}
	reg, err := native.DefaultRegistryWithPolicy(pol)
	if err != nil {
		t.Fatal(err)
	}
	_, oErr := tuiOpenHandler(nil, nil, nil, reg)
	tcErrContains(t, oErr, "terminal")
	var denied *policy.Denied
	if !errors.As(oErr, &denied) || denied.Code != policy.CodeCapabilityNotInstalled {
		t.Fatalf("sandbox open = %v", oErr)
	}
	// an installed scope consults Check and proceeds (to no_backend here)
	trusted, err := policy.Load("trusted")
	if err != nil {
		t.Fatal(err)
	}
	treg, err := native.DefaultRegistryWithPolicy(trusted)
	if err != nil {
		t.Fatal(err)
	}
	_, tErr := tuiOpenHandler(nil, nil, nil, treg)
	if got := tcCode(t, tErr); got != "no_backend" {
		t.Fatalf("trusted open code = %q (%v)", got, tErr)
	}
}

// --- open arms ---

func TestTuiOpenHandlerArms(t *testing.T) {
	reg := tcReg(t)
	vb := tuikit.NewVirtualBackend(1, 1)
	if err := RegisterHostTui(reg, TuiSpec{Name: "v", Open: func() (tuikit.Backend, error) { return vb, nil }}); err != nil {
		t.Fatal(err)
	}
	_, err := tuiOpenHandler([]native.Value{native.NewInteger(5)}, nil, nil, reg)
	tcErrContains(t, err, "options must be a Map")

	m := native.NewOrderedMap()
	m.Set("mouse", native.NewString("yes"))
	_, err = tuiOpenHandler([]native.Value{native.NewMap(m)}, nil, nil, reg)
	tcErrContains(t, err, "mouse: must be a Boolean")

	m2 := native.NewOrderedMap()
	m2.Set("title", native.NewInteger(5))
	_, err = tuiOpenHandler([]native.Value{native.NewMap(m2)}, nil, nil, reg)
	tcErrContains(t, err, "title: must be a String")

	vb.OpenErr = errors.New("backend refused")
	_, err = tuiOpenHandler(nil, nil, nil, reg)
	if got := tcCode(t, err); got != "terminal" {
		t.Fatalf("backend.Open failure code = %q (%v)", got, err)
	}

	failReg := tcReg(t)
	if err := RegisterHostTui(failReg, TuiSpec{Name: "f", Open: func() (tuikit.Backend, error) {
		return nil, errors.New("no surface")
	}}); err != nil {
		t.Fatal(err)
	}
	_, err = tuiOpenHandler(nil, nil, nil, failReg)
	tcErrContains(t, err, "no surface")
}

// --- per-word guard + closed + backend-failure arms ---

func TestTuiHandlerGuardArms(t *testing.T) {
	reg := tcReg(t)
	bad := []native.Value{native.NewInteger(1)}
	for name, h := range map[string]native.Handler{
		"close":      tuiCloseHandler,
		"dims":       tuiDimsHandler,
		"read-event": tuiReadEventHandler,
		"clear":      tuiClearHandler,
		"show":       tuiShowHandler,
		"bell":       tuiBellHandler,
	} {
		_, err := h(bad, nil, nil, reg)
		if got := tcCode(t, err); got != "tui_error" {
			t.Fatalf("%s guard code = %q (%v)", name, got, err)
		}
	}
	_, err := tuiPrintAtHandler([]native.Value{native.NewInteger(1), native.NewInteger(0), native.NewInteger(0), native.NewString("x")}, nil, nil, reg)
	if got := tcCode(t, err); got != "tui_error" {
		t.Fatalf("print-at guard code = %q (%v)", got, err)
	}
	_, err = tuiTitleHandler([]native.Value{native.NewInteger(1), native.NewString("s")}, nil, nil, reg)
	if got := tcCode(t, err); got != "tui_error" {
		t.Fatalf("title guard code = %q (%v)", got, err)
	}
}

func TestTuiBackendFailureArms(t *testing.T) {
	reg := tcReg(t)
	term, vb := tcTerm(t, 2, 2)

	vb.SizeErr = errors.New("nope")
	_, err := tuiDimsHandler([]native.Value{term}, nil, nil, reg)
	if got := tcCode(t, err); got != "terminal" {
		t.Fatalf("dims failure code = %q", got)
	}
	vb.SizeErr = nil

	vb.PresentErr = errors.New("nope")
	_, err = tuiShowHandler([]native.Value{term}, nil, nil, reg)
	if got := tcCode(t, err); got != "terminal" {
		t.Fatalf("show failure code = %q", got)
	}
	vb.PresentErr = nil

	vb.TitleErr = errors.New("nope")
	_, err = tuiTitleHandler([]native.Value{term, native.NewString("s")}, nil, nil, reg)
	if got := tcCode(t, err); got != "terminal" {
		t.Fatalf("title failure code = %q", got)
	}
	vb.TitleErr = nil

	vb.BellErr = errors.New("nope")
	_, err = tuiBellHandler([]native.Value{term}, nil, nil, reg)
	if got := tcCode(t, err); got != "terminal" {
		t.Fatalf("bell failure code = %q", got)
	}
	vb.BellErr = nil

	vb.CloseErr = errors.New("nope")
	_, err = tuiCloseHandler([]native.Value{term}, nil, nil, reg)
	if got := tcCode(t, err); got != "terminal" {
		t.Fatalf("close failure code = %q", got)
	}
	// the failed close latched the termState; the backend close is owed
	vb.CloseErr = nil
}

// --- read-event arms ---

func TestTuiReadEventArms(t *testing.T) {
	reg := tcReg(t)
	term, vb := tcTerm(t, 2, 2)

	badWithin := native.NewOrderedMap()
	badWithin.Set("within", native.NewInteger(-1))
	_, err := tuiReadEventHandler([]native.Value{term, native.NewMap(badWithin)}, nil, nil, reg)
	tcErrContains(t, err, "within: must be a non-negative Integer")

	// blocking (no deadline) path with a queued event
	vb.Inject(tuikit.Event{Tag: "paste", Text: "clip"})
	out, err := tuiReadEventHandler([]native.Value{term}, nil, nil, reg)
	if err != nil {
		t.Fatal(err)
	}
	mp, _ := native.AsMap(out[0])
	got, _ := mp.Get("text")
	if s, sErr := got.AsConcreteString(); sErr != nil || s != "clip" {
		t.Fatalf("paste event = %v", out[0])
	}

	// end-of-input: the backend closes underneath an open handle
	_ = vb.Close()
	_, err = tuiReadEventHandler([]native.Value{term}, nil, nil, reg)
	if got := tcCode(t, err); got != "closed" {
		t.Fatalf("blocking end-of-input code = %q", got)
	}
	within := native.NewOrderedMap()
	within.Set("within", native.NewInteger(1000))
	_, err = tuiReadEventHandler([]native.Value{term, native.NewMap(within)}, nil, nil, reg)
	if got := tcCode(t, err); got != "closed" {
		t.Fatalf("deadline end-of-input code = %q", got)
	}
}

// --- print-at / title argument arms ---

func TestTuiPrintAtArgumentArms(t *testing.T) {
	reg := tcReg(t)
	term, _ := tcTerm(t, 2, 2)
	i := native.NewInteger
	s := native.NewString

	_, err := tuiPrintAtHandler([]native.Value{term, s("x"), i(0), s("t")}, nil, nil, reg)
	tcErrContains(t, err, "x must be an Integer")
	_, err = tuiPrintAtHandler([]native.Value{term, i(0), s("y"), s("t")}, nil, nil, reg)
	tcErrContains(t, err, "y must be an Integer")
	_, err = tuiPrintAtHandler([]native.Value{term, i(0), i(0), i(7)}, nil, nil, reg)
	tcErrContains(t, err, "text must be a String")
	_, err = tuiPrintAtHandler([]native.Value{term, i(0), i(0), s("t"), i(7)}, nil, nil, reg)
	tcErrContains(t, err, "style must be a Map")

	_, err = tuiTitleHandler([]native.Value{term, i(7)}, nil, nil, reg)
	tcErrContains(t, err, "title: expected a String")
}

// --- style parsing arms ---

func TestTuiStyleFromMapArms(t *testing.T) {
	reg := tcReg(t)
	for key, val := range map[string]native.Value{
		"fg":        native.NewInteger(1),
		"bg":        native.NewInteger(1),
		"bold":      native.NewString("y"),
		"italic":    native.NewString("y"),
		"underline": native.NewString("y"),
		"reverse":   native.NewString("y"),
	} {
		m := native.NewOrderedMap()
		m.Set(key, val)
		_, err := styleFromMap(native.NewMap(m), "print-at", reg)
		tcErrContains(t, err, key+": must be a")
	}
}

// --- event projection ---

func TestTuiEventToMapAllTags(t *testing.T) {
	cases := map[string]tuikit.Event{
		"resize": {Tag: "resize", Cols: 80, Rows: 24},
		"mouse":  {Tag: "mouse", Kind: "press", X: 3, Y: 4},
		"paste":  {Tag: "paste", Text: "p"},
		"focus":  {Tag: "focus", Gained: true},
	}
	for tag, ev := range cases {
		mp, _ := native.AsMap(eventToMap(ev))
		got, ok := mp.Get("tag")
		s, sErr := got.AsConcreteString()
		if !ok || sErr != nil || s != tag {
			t.Fatalf("%s tag = %v", tag, got)
		}
	}
	mp, _ := native.AsMap(eventToMap(tuikit.Event{Tag: "resize", Cols: 80, Rows: 24}))
	cols, _ := mp.Get("cols")
	if n, err := cols.AsConcreteInteger(); err != nil || n != 80 {
		t.Fatalf("resize cols = %v (%v)", cols, err)
	}
}

// --- returns fn ---

func TestTuiNoReturns(t *testing.T) {
	if got := tuiNoReturns(nil, nil); got != nil {
		t.Fatalf("tuiNoReturns = %v", got)
	}
}
