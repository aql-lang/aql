package modules

import (
	"strings"
	"testing"

	"github.com/boru-lang/boru/lang/go/native"
	parser "github.com/boru-lang/boru/parser/go"
)

// Wave-6 agent-D coverage for the boru:debug module internals: every
// per-argument concrete-value rejection arm (signature dispatch shields
// them from wrong-typed values, so the handlers are invoked directly, the
// wave-3/wave-4 precedent), the parser-not-configured and body-error arms
// of the body-running words, and the small pure helpers (typeLeavesToList,
// lastOrNone, isBreakAt, sizeOfValue/shapeCensus map walks, dashboard
// render fallbacks).

// seam6dDebugNative returns the named inner native from the full debug set.
func seam6dDebugNative(t *testing.T, name string) native.NativeFunc {
	t.Helper()
	all := append(debugNatives(), stepNatives()...)
	all = append(all, dashboardNatives()...)
	return findNativeWave3(t, all, name)
}

// TestSeam6DDebugArgRejections drives the per-argument AsConcrete* /
// RequireConcreteList rejection arm of every debug word: a wrong-typed or
// non-concrete value at any position errors loudly, never panics.
func TestSeam6DDebugArgRejections(t *testing.T) {
	r, _ := debugRegistry(t)
	i1 := native.NewInteger(1)
	s1 := native.NewString("x")
	bt := native.NewBoolean(true)
	listLit := native.NewTypeLiteral(native.TList)
	body := native.NewList([]native.Value{native.NewInteger(1)})

	cases := []struct {
		word string
		args []native.Value
	}{
		{"debug-label", []native.Value{i1, i1}},
		{"debug-assert", []native.Value{s1, s1}},
		{"debug-assert", []native.Value{bt, i1}},
		{"debug-todo", []native.Value{i1}},
		{"debug-parse", []native.Value{i1}},
		{"debug-deps", []native.Value{listLit}},
		{"debug-explain", []native.Value{i1}},
		{"debug-sig", []native.Value{i1}},
		{"debug-body", []native.Value{i1}},
		{"debug-watch", []native.Value{i1}},
		{"debug-bench", []native.Value{body, s1}},
		{"debug-trace", []native.Value{listLit}},
		{"debug-profile", []native.Value{listLit}},
		{"debug-disasm", []native.Value{listLit}},
		{"debug-step", []native.Value{listLit}},
		{"debug-break-when", []native.Value{i1}},
		{"debug-run-stepped", []native.Value{i1}},
		{"debug-dashboard", []native.Value{listLit}},
		{"debug-widget", []native.Value{i1, s1}},
		{"debug-widget", []native.Value{s1, i1}},
	}
	for _, c := range cases {
		n := seam6dDebugNative(t, c.word)
		if _, err := w4Handler(t, n, 0, r, c.args...); err == nil {
			t.Errorf("%s with %v: expected rejection, got nil", c.word, c.args)
		}
	}
}

// TestSeam6DDebugParserNotConfigured pins the parser-not-configured arms of
// Debug.parse and Debug.run-stepped on a registry without a ParseFunc.
func TestSeam6DDebugParserNotConfigured(t *testing.T) {
	r, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	for _, word := range []string{"debug-parse", "debug-run-stepped"} {
		n := seam6dDebugNative(t, word)
		_, herr := w4Handler(t, n, 0, r, native.NewString("1 add 2"))
		if herr == nil || !strings.Contains(herr.Error(), "parser not configured") {
			t.Errorf("%s without parser: got %v, want 'parser not configured'", word, herr)
		}
	}
}

// TestSeam6DDebugRunSteppedParseError pins Debug.run-stepped's parse-error
// arm (its sibling Debug.parse arm is pinned in debug_test.go).
func TestSeam6DDebugRunSteppedParseError(t *testing.T) {
	r, _ := debugRegistry(t)
	n := seam6dDebugNative(t, "debug-run-stepped")
	_, err := w4Handler(t, n, 0, r, native.NewString("("))
	if err == nil || !strings.Contains(err.Error(), "parse_error") {
		t.Errorf("run-stepped of malformed source: got %v, want parse_error", err)
	}
}

// TestSeam6DDebugBodyErrorArms drives the body-execution error arm of every
// body-running debug word with a body whose word is undefined.
func TestSeam6DDebugBodyErrorArms(t *testing.T) {
	bad := native.NewList([]native.Value{native.NewWord("seam6d-undefined-word")})
	one := native.NewInteger(1)
	cases := []struct {
		word string
		args []native.Value
	}{
		{"debug-time", []native.Value{bad}},
		{"debug-bench", []native.Value{bad, one}},
		{"debug-trace", []native.Value{bad}},
		{"debug-profile", []native.Value{bad}},
		{"debug-step", []native.Value{bad}},
	}
	for _, c := range cases {
		r, _ := debugRegistry(t)
		n := seam6dDebugNative(t, c.word)
		if _, err := w4Handler(t, n, 0, r, c.args...); err == nil ||
			!strings.Contains(err.Error(), "undefined_word") {
			t.Errorf("%s with an erroring body: got %v, want undefined_word", c.word, err)
		}
	}
}

// TestSeam6DDebugBodyUnknownWord pins Debug.body's no-such-word arm.
func TestSeam6DDebugBodyUnknownWord(t *testing.T) {
	r, _ := debugRegistry(t)
	n := seam6dDebugNative(t, "debug-body")
	_, err := w4Handler(t, n, 0, r, native.NewString("seam6d-nope"))
	if err == nil || !strings.Contains(err.Error(), "no such word") {
		t.Errorf("Debug.body of unknown word: got %v, want 'no such word'", err)
	}
}

// TestSeam6DDebugWatchUnbound pins Debug.watch's unbound-name fallback: it
// prints and returns None rather than erroring.
func TestSeam6DDebugWatchUnbound(t *testing.T) {
	r, buf := debugRegistry(t)
	n := seam6dDebugNative(t, "debug-watch")
	out, err := w4Handler(t, n, 0, r, native.NewString("seam6d-unbound"))
	if err != nil {
		t.Fatalf("watch of unbound name: %v", err)
	}
	if len(out) != 1 || !out[0].Is(native.TNone) {
		t.Errorf("watch of unbound name = %v, want None", out)
	}
	if !strings.Contains(buf.String(), "seam6d-unbound = ") {
		t.Errorf("watch must print the binding line, output = %q", buf.String())
	}
}

// TestSeam6DDebugDepsRepeatedWords pins the dedup arm of Debug.deps: a word
// referenced twice appears once.
func TestSeam6DDebugDepsRepeatedWords(t *testing.T) {
	r, _ := debugRegistry(t)
	n := seam6dDebugNative(t, "debug-deps")
	body := native.NewList([]native.Value{native.NewWord("add"), native.NewWord("add")})
	out, err := w4Handler(t, n, 0, r, body)
	if err != nil {
		t.Fatalf("deps: %v", err)
	}
	lst, lerr := native.AsList(out[0])
	if lerr != nil || lst.Len() != 1 {
		t.Errorf("deps of [add add] = %v, want exactly [add]", out[0])
	}
}

// TestSeam6DDebugDisasmArms pins Debug.disasm's registry-construction and
// compile-failure arms.
func TestSeam6DDebugDisasmArms(t *testing.T) {
	r, _ := debugRegistry(t)
	n := seam6dDebugNative(t, "debug-disasm")

	// Compile failure: the body's word is undefined in the fresh sub-registry.
	bad := native.NewList([]native.Value{native.NewWord("seam6d-undefined-word")})
	if _, err := w4Handler(t, n, 0, r, bad); err == nil ||
		!strings.Contains(err.Error(), "Debug.disasm") {
		t.Errorf("disasm of uncompilable body: got %v, want Debug.disasm error", err)
	}

	// Registry-construction failure through the seam.
	seam6dFailRegistryAfter(t, 0)
	good := native.NewList([]native.Value{native.NewInteger(1)})
	if _, err := w4Handler(t, n, 0, r, good); err == nil ||
		!strings.Contains(err.Error(), "seam6d") {
		t.Errorf("disasm under failing registry seam: got %v", err)
	}
}

// TestSeam6DDebugProfileHappy runs Debug.profile end-to-end so the
// per-word count rows are actually built and sorted.
func TestSeam6DDebugProfileHappy(t *testing.T) {
	r, _ := debugRegistry(t)
	res := runDebug(t, r, "[1 2 add] Debug.profile")
	lst, err := native.AsList(res[len(res)-1])
	if err != nil || lst.IsNil() || lst.Len() == 0 {
		t.Fatalf("profile rows = %v (err %v), want non-empty list", res, err)
	}
	row, merr := native.AsMap(lst.Get(0))
	if merr != nil || row == nil {
		t.Fatalf("profile row not a map: %v", lst.Get(0))
	}
	if _, ok := row.Get("word"); !ok {
		t.Error("profile row missing 'word'")
	}
	if _, ok := row.Get("steps"); !ok {
		t.Error("profile row missing 'steps'")
	}
}

// TestSeam6DDebugSizeofShapeMaps drives the map branches of sizeOfValue and
// shapeCensus.walk (with a string leaf inside the map).
func TestSeam6DDebugSizeofShapeMaps(t *testing.T) {
	r, _ := debugRegistry(t)
	res := runDebug(t, r, "{a: 1, b: 'xy'} Debug.sizeof")
	nbytes, err := res[len(res)-1].AsConcreteInteger()
	if err != nil || nbytes <= 0 {
		t.Errorf("sizeof map = %v (err %v), want > 0", res[len(res)-1], err)
	}

	res = runDebug(t, r, "{a: 'xy'} Debug.shape")
	m, merr := native.AsMap(res[len(res)-1])
	if merr != nil || m == nil {
		t.Fatalf("shape result not a map: %v", res[len(res)-1])
	}
	maps, _ := m.Get("maps")
	if n, _ := maps.AsConcreteInteger(); n != 1 {
		t.Errorf("shape maps = %v, want 1", maps)
	}
	strs, _ := m.Get("strings")
	if n, _ := strs.AsConcreteInteger(); n != 1 {
		t.Errorf("shape strings = %v, want 1", strs)
	}
}

// TestSeam6DDebugSmallHelpers pins the pure helpers directly: the nil-type
// skip of typeLeavesToList, runCounted's non-concrete-body rejection,
// lastOrNone's empty fallback, and isBreakAt's bounds guard.
func TestSeam6DDebugSmallHelpers(t *testing.T) {
	lst := typeLeavesToList([]*native.Type{nil, native.TInteger})
	got, err := native.AsList(lst)
	if err != nil || got.Len() != 1 {
		t.Errorf("typeLeavesToList([nil Integer]) = %v, want one leaf", lst)
	}

	r, _ := debugRegistry(t)
	if _, _, cerr := runCounted(r, native.NewTypeLiteral(native.TList), "seam6d"); cerr == nil {
		t.Error("runCounted of a List type literal must be rejected")
	}

	if v := lastOrNone(nil); !v.Is(native.TNone) {
		t.Errorf("lastOrNone(nil) = %v, want None", v)
	}

	if isBreakAt(nil, 0) {
		t.Error("isBreakAt(nil, 0) must be false")
	}
	if isBreakAt([]native.Value{native.NewInteger(1)}, -1) {
		t.Error("isBreakAt(_, -1) must be false")
	}
}

// TestSeam6DDashboardRenderArms pins the dashboard render fallbacks: a
// non-map widget, an empty sample, a missing parser, a failing sample, and
// a sample that leaves nothing on the stack.
func TestSeam6DDashboardRenderArms(t *testing.T) {
	r, _ := debugRegistry(t)

	// Non-map widget → untitled panel with an empty sample.
	out := renderDashboard(r, []native.Value{native.NewInteger(42)})
	if !strings.Contains(out, "(untitled)") || !strings.Contains(out, "(empty)") {
		t.Errorf("non-map widget rendered %q, want untitled/empty", out)
	}

	// No parser installed.
	bare, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	if got := sampleResult(bare, "1 add 2"); got != "(no parser)" {
		t.Errorf("sampleResult without parser = %q, want (no parser)", got)
	}

	// Sample that errors at run time.
	if got := sampleResult(r, "seam6d-undefined-word"); !strings.Contains(got, "(error:") {
		t.Errorf("sampleResult of erroring source = %q, want (error: …)", got)
	}

	// Sample that leaves nothing on the stack renders None.
	if got := sampleResult(r, "def seam6dx 1"); got != native.FormatForPrint(native.NewNone()) {
		t.Errorf("sampleResult of empty-result source = %q, want None render", got)
	}
}

// TestSeam6DDashboardWidgetHappy pins the widget + dashboard happy path
// end-to-end (parse, run, snapshot) through the module surface.
func TestSeam6DDashboardWidgetHappy(t *testing.T) {
	r, buf := debugRegistry(t)
	r.SetParseFunc(parser.Parse)
	res := runDebug(t, r, `[(Debug.widget "t1" "1 add 2")] Debug.dashboard`)
	s, err := res[len(res)-1].AsConcreteString()
	if err != nil {
		t.Fatalf("dashboard result not a string: %v", err)
	}
	if !strings.Contains(s, "t1") || !strings.Contains(s, "3") {
		t.Errorf("dashboard snapshot %q must contain the title and the sample result", s)
	}
	if !strings.Contains(buf.String(), "t1") {
		t.Errorf("dashboard must print the snapshot, output = %q", buf.String())
	}
}
