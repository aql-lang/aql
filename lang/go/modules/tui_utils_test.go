package modules

import (
	"strings"
	"testing"

	"github.com/boru-lang/boru/lang/go/native"
)

// The standalone terminal utilities (tui_utils.go): exact ANSI bytes
// per profile, the identity and round-trip laws, display-width
// semantics — negatives paired throughout.

func tuStr(t *testing.T, out []native.Value, err error) string {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
	s, sErr := out[len(out)-1].AsConcreteString()
	if sErr != nil {
		t.Fatal(sErr)
	}
	return s
}

func TestTuiColorizeProfiles(t *testing.T) {
	reg := tcReg(t)
	run := func(src string) string {
		t.Helper()
		out, err := runTuiStepsOn(t, reg, []string{`import "boru:tui"`, src})
		return tuStr(t, out, err)
	}
	// truecolor is the default: 24-bit foreground + reset
	if got := run(`Tui.colorize {fg: "#ff0000"} "hi"`); got != "\x1b[38;2;255;0;0mhi\x1b[0m" {
		t.Fatalf("truecolor = %q", got)
	}
	// the same style degrades through the renderer's tables
	if got := run(`Tui.colorize {fg: "#ff0000"} "hi" {profile: "256"}`); got != "\x1b[38;5;196mhi\x1b[0m" {
		t.Fatalf("256 = %q", got)
	}
	if got := run(`Tui.colorize {fg: "#ff0000"} "hi" {profile: "16"}`); got != "\x1b[91mhi\x1b[0m" {
		t.Fatalf("16 = %q", got)
	}
	// "none" keeps attributes and drops colors
	if got := run(`Tui.colorize {fg: "#ff0000" bold: true} "hi" {profile: "none"}`); got != "\x1b[1mhi\x1b[0m" {
		t.Fatalf("none = %q", got)
	}
	// the attribute-less style is the identity — no stray escapes
	if got := run(`Tui.colorize {} "hi"`); got != "hi" {
		t.Fatalf("identity = %q", got)
	}
	// a color-only style under "none" collapses to the identity too
	if got := run(`Tui.colorize {fg: "red"} "hi" {profile: "none"}`); got != "hi" {
		t.Fatalf("none identity = %q", got)
	}
	// an unparseable color string is skipped, exactly like the renderer
	if got := run(`Tui.colorize {fg: "no-such-color" bold: true} "hi"`); got != "\x1b[1mhi\x1b[0m" {
		t.Fatalf("bad color skipped = %q", got)
	}
}

func TestTuiUtilRoundTripAndWidth(t *testing.T) {
	reg := tcReg(t)
	steps := func(src string) []native.Value {
		t.Helper()
		out, err := runTuiStepsOn(t, reg, []string{`import "boru:tui"`, src})
		if err != nil {
			t.Fatal(err)
		}
		return out
	}
	// strip-ansi round-trips colorize
	out := steps(`Tui.strip-ansi (Tui.colorize {fg: "red" bold: true underline: true} "héllo")`)
	if s, _ := out[len(out)-1].AsConcreteString(); s != "héllo" {
		t.Fatalf("round trip = %q", s)
	}
	// text-width: display cells, not characters or bytes
	for _, c := range []struct {
		src  string
		want int64
	}{
		{`Tui.text-width "hi"`, 2},
		{`Tui.text-width ""`, 0},
		{`Tui.text-width "漢字"`, 4},
		{`Tui.text-width "é"`, 1},
		{`Tui.text-width (Tui.colorize {fg: "red"} "漢")`, 2},
	} {
		out := steps(c.src)
		n, err := out[len(out)-1].AsConcreteInteger()
		if err != nil || n != c.want {
			t.Fatalf("%s = %v (%v), want %d", c.src, n, err, c.want)
		}
	}
}

func TestTuiUtilGuards(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`Tui.colorize 5 "hi"`, "uncalled_function"},
		{`Tui.colorize {} 5`, "uncalled_function"},
		{`Tui.colorize {fg: 5} "hi"`, "fg: must be a String"},
		{`Tui.colorize {bold: "yes"} "hi"`, "bold: must be a Boolean"},
		{`Tui.colorize {} "hi" {profile: "grayscale"}`, `profile: must be "truecolor", "256", "16" or "none"`},
		{`Tui.colorize {} "hi" {profile: 5}`, "profile: must be a String"},
		{`Tui.strip-ansi 5`, "uncalled_function"},
		{`Tui.text-width 5`, "uncalled_function"},
	} {
		_, err := runTuiStepsOn(t, tcReg(t), []string{`import "boru:tui"`, c.src})
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Fatalf("%s = %v, want %q", c.src, err, c.want)
		}
	}
	// the direct-handler arms the engine path cannot reach: a type
	// literal in a sig-matched Map/String slot
	reg := tcReg(t)
	_, err := tuiColorizeHandler([]native.Value{native.NewTypeLiteral(native.TMap), native.NewString("x")}, nil, nil, reg)
	if err == nil || !strings.Contains(err.Error(), "style must be a Map") {
		t.Fatalf("type-literal style = %v", err)
	}
	_, err = tuiColorizeHandler([]native.Value{native.NewMap(native.NewOrderedMap()), native.NewTypeLiteral(native.TString)}, nil, nil, reg)
	if err == nil || !strings.Contains(err.Error(), "expected the text as a String") {
		t.Fatalf("type-literal text = %v", err)
	}
	_, err = tuiColorizeHandler([]native.Value{native.NewMap(native.NewOrderedMap()), native.NewString("x"), native.NewTypeLiteral(native.TMap)}, nil, nil, reg)
	if err == nil || !strings.Contains(err.Error(), "options must be a Map") {
		t.Fatalf("type-literal opts = %v", err)
	}
	_, err = tuiStripAnsiHandler([]native.Value{native.NewTypeLiteral(native.TString)}, nil, nil, reg)
	if err == nil || !strings.Contains(err.Error(), "expected a String") {
		t.Fatalf("type-literal strip = %v", err)
	}
	_, err = tuiTextWidthHandler([]native.Value{native.NewTypeLiteral(native.TString)}, nil, nil, reg)
	if err == nil || !strings.Contains(err.Error(), "expected a String") {
		t.Fatalf("type-literal width = %v", err)
	}
}
