package eng

import (
	"strings"
	"testing"
)

// Golden tests for the structured diagnostic renderer
// (design/DIAGNOSTICS.0.md). The plain rendering of a legacy-shaped
// error is pinned byte-for-byte against the historical Error() format;
// the structured layer (spans / notes / suggestions) and the color
// palette get their own goldens, plain and colored, so every renderer
// branch is exercised both ways.

const drSource = "def x 5\nbadword x\nprint x"

func drBaseError() *BoruError {
	return &BoruError{
		Code:       "signature_error",
		Detail:     "no matching signature for badword",
		Row:        2,
		Col:        1,
		Src:        "badword",
		FullSource: drSource,
	}
}

func TestRenderLegacyByteIdentity(t *testing.T) {
	e := drBaseError()
	e.Hint = "stack: 5"
	want := strings.Join([]string{
		"[boru/signature_error]: no matching signature for badword",
		"  --> 2:1",
		"  1 | def x 5",
		"  2 | badword x",
		"      ^^^^^^^ no matching signature for badword",
		"  3 | print x",
		"  = stack: 5",
	}, "\n")
	if got := e.Error(); got != want {
		t.Fatalf("legacy plain rendering drifted.\ngot:\n%s\nwant:\n%s", got, want)
	}
	if got := e.Render(RenderOpts{}); got != e.Error() {
		t.Fatal("Error() must be exactly Render(RenderOpts{})")
	}
}

func TestRenderPositionVariants(t *testing.T) {
	// Unknown position: honest marker, no excerpt.
	e := &BoruError{Code: "type_error", Detail: "boom"}
	want := "[boru/type_error]: boom\n  --> source position unknown"
	if got := e.Error(); got != want {
		t.Fatalf("unknown-position rendering: got %q want %q", got, want)
	}

	// File-qualified position with an unknown column (falls back to 1).
	e = &BoruError{Code: "type_error", Detail: "boom", Row: 3, File: "/tmp/mod.boru"}
	if got := e.Error(); !strings.Contains(got, "--> /tmp/mod.boru:3:1") {
		t.Fatalf("file-qualified position missing: %q", got)
	}

	// Position known but no source text: header only, no excerpt.
	if strings.Contains(e.Error(), " | ") {
		t.Fatalf("no-source error must not render an excerpt: %q", e.Error())
	}
}

func TestRenderSecondarySpans(t *testing.T) {
	e := drBaseError()
	e.Spans = []DiagSpan{
		// Same-source span: own arrow header + one-line excerpt with --- label.
		{Pos: SrcPos{Row: 1, Col: 5, Src: "x"}, Label: "`x` was defined here"},
		// Cross-file span: its own file and source text.
		{Pos: SrcPos{Row: 1, Col: 1, Src: "export"}, Label: "exported from the module", File: "/lib/m.boru", Source: "export 'X' {}"},
		// Position-less span: degrades to a note (no guessed locations).
		{Label: "a related constraint applies"},
	}
	got := e.Error()
	want := strings.Join([]string{
		"[boru/signature_error]: no matching signature for badword",
		"  --> 2:1",
		"  1 | def x 5",
		"  2 | badword x",
		"      ^^^^^^^ no matching signature for badword",
		"  3 | print x",
		"  --> 1:5",
		"  1 | def x 5",
		"          - `x` was defined here",
		"  --> /lib/m.boru:1:1",
		"  1 | export 'X' {}",
		"      ------ exported from the module",
		"  = note: a related constraint applies",
	}, "\n")
	if got != want {
		t.Fatalf("secondary spans rendering drifted.\ngot:\n%s\nwant:\n%s", got, want)
	}
}

func TestRenderSpanWithoutSourceOrLabel(t *testing.T) {
	// A located span with NO source text anywhere renders its label as a
	// note; with no label either, only the arrow header appears.
	e := &BoruError{Code: "x", Detail: "d", Row: 1, Col: 1, Src: "d"}
	e.Spans = []DiagSpan{
		{Pos: SrcPos{Row: 4, Col: 2, Src: "y"}, Label: "over here"},
		{Pos: SrcPos{Row: 9, Col: 1, Src: "z"}},
	}
	got := e.Error()
	if !strings.Contains(got, "--> 4:2") || !strings.Contains(got, "= note: over here") {
		t.Fatalf("sourceless labeled span must render header + note: %q", got)
	}
	if !strings.Contains(got, "--> 9:1") || strings.Contains(got, "note: \n") {
		t.Fatalf("sourceless unlabeled span must render only its header: %q", got)
	}
}

func TestRenderNotesAndSuggestions(t *testing.T) {
	repl := "(badword x)"
	e := drBaseError()
	e.Hint = "legacy hint stays first"
	e.Notes = []string{"candidate `(Integer)` needs 1 argument, but 2 were supplied"}
	e.Suggestions = []DiagSuggestion{
		{Message: "group the call in parens", Replacement: &repl},
		{Message: "see `boru describe badword`"},
	}
	got := e.Error()
	want := strings.Join([]string{
		"[boru/signature_error]: no matching signature for badword",
		"  --> 2:1",
		"  1 | def x 5",
		"  2 | badword x",
		"      ^^^^^^^ no matching signature for badword",
		"  3 | print x",
		"  = legacy hint stays first",
		"  = note: candidate `(Integer)` needs 1 argument, but 2 were supplied",
		"  = help: group the call in parens",
		"          (badword x)",
		"  = help: see `boru describe badword`",
	}, "\n")
	if got != want {
		t.Fatalf("notes/suggestions rendering drifted.\ngot:\n%s\nwant:\n%s", got, want)
	}
}

func TestRenderColorGolden(t *testing.T) {
	repl := "(f 1)"
	e := &BoruError{
		Code: "type_error", Detail: "boom", Row: 1, Col: 1, Src: "f",
		FullSource:  "f 1",
		Hint:        "h",
		Spans:       []DiagSpan{{Pos: SrcPos{Row: 1, Col: 3, Src: "1"}, Label: "the operand"}},
		Notes:       []string{"n"},
		Suggestions: []DiagSuggestion{{Message: "s", Replacement: &repl}},
	}
	got := e.Render(RenderOpts{Color: true})
	want := strings.Join([]string{
		cBold + cRed + "[boru/type_error]:" + cReset + " boom",
		cBlue + "  --> 1:1" + cReset,
		cBlue + "  1 |" + cReset + " f 1",
		"      " + cBold + cRed + "^ boom" + cReset,
		cBlue + "  --> 1:3" + cReset,
		cBlue + "  1 |" + cReset + " f 1",
		"        " + cBlue + "- the operand" + cReset,
		"  " + cBold + "=" + cReset + " h",
		"  " + cBold + cBlue + "= note:" + cReset + " n",
		"  " + cBold + cCyan + "= help:" + cReset + " s",
		"          " + cGreen + "(f 1)" + cReset,
	}, "\n")
	if got != want {
		t.Fatalf("color rendering drifted.\ngot:\n%q\nwant:\n%q", got, want)
	}
	// The plain rendering of the same error carries zero ANSI bytes.
	if strings.Contains(e.Error(), "\x1b[") {
		t.Fatal("plain rendering must never contain ANSI escapes")
	}
}

func TestRenderSiteEdges(t *testing.T) {
	// Empty caption: no trailing space after the underline.
	out := renderSite("one two", "one", "", 1, 1, RenderOpts{}, primarySiteStyle)
	if strings.Contains(out, "^^^ ") {
		t.Fatalf("empty caption must not leave a trailing space: %q", out)
	}
	// Empty sub still draws one caret.
	out = renderSite("one", "", "m", 1, 1, RenderOpts{}, primarySiteStyle)
	if !strings.Contains(out, "^ m") || strings.Contains(out, "^^") {
		t.Fatalf("empty sub must draw exactly one caret: %q", out)
	}
}

func TestRenderCheckDiagnosticBlock(t *testing.T) {
	repl := "upper"
	d := CheckDiagnostic{
		Code: "undefined_word", Detail: "undefined word: uppr",
		Word: "uppr", Row: 1, Col: 8, Severity: SeverityError,
		Src:         "uppr",
		Notes:       []string{"nothing registered or defined has this name"},
		Suggestions: []DiagSuggestion{{Message: "did you mean `upper`?", Replacement: &repl}},
	}
	got := RenderCheckDiagnostic(d, "print (uppr [1 2])", "", RenderOpts{})
	want := strings.Join([]string{
		"  1 | print (uppr [1 2])",
		"             ^^^^",
		"  = note: nothing registered or defined has this name",
		"  = help: did you mean `upper`?",
		"          upper",
	}, "\n")
	if got != want {
		t.Fatalf("check block rendering drifted.\ngot:\n%s\nwant:\n%s", got, want)
	}

	// Src falls back to Word for the caret width.
	d2 := CheckDiagnostic{Code: "x", Detail: "d", Word: "ab", Row: 1, Col: 1}
	if out := RenderCheckDiagnostic(d2, "ab", "", RenderOpts{}); !strings.Contains(out, "^^") {
		t.Fatalf("caret width must fall back to Word: %q", out)
	}

	// No position, no notes, no suggestions: nothing to show.
	if out := RenderCheckDiagnostic(CheckDiagnostic{Code: "x", Detail: "d"}, "src", "", RenderOpts{}); out != "" {
		t.Fatalf("empty rich block must render as empty, got %q", out)
	}

	// Notes render without a leading newline when there is no excerpt.
	d3 := CheckDiagnostic{Code: "x", Detail: "d", Notes: []string{"n"}}
	if out := RenderCheckDiagnostic(d3, "", "", RenderOpts{}); out != "  = note: n" {
		t.Fatalf("excerpt-less note block drifted: %q", out)
	}

	// Suggestion without a replacement, after a note.
	d4 := CheckDiagnostic{Code: "x", Detail: "d", Notes: []string{"n"}, Suggestions: []DiagSuggestion{{Message: "s"}}}
	if out := RenderCheckDiagnostic(d4, "", "", RenderOpts{}); out != "  = note: n\n  = help: s" {
		t.Fatalf("note+help block drifted: %q", out)
	}

	// Severity styles: warning and default both color their carets.
	for _, sev := range []CheckSeverity{SeverityWarning, ""} {
		dd := CheckDiagnostic{Code: "x", Detail: "d", Row: 1, Col: 1, Src: "a", Severity: sev}
		if out := RenderCheckDiagnostic(dd, "a", "", RenderOpts{Color: true}); !strings.Contains(out, "^") {
			t.Fatalf("severity %q must still underline: %q", sev, out)
		}
	}
}

func TestPaintIdentity(t *testing.T) {
	o := RenderOpts{Color: true}
	if o.paint(cRed, "") != "" {
		t.Fatal("painting an empty string must stay empty")
	}
	if (RenderOpts{}).paint(cRed, "x") != "x" {
		t.Fatal("plain opts must not paint")
	}
	if o.paint(cRed, "x") != cRed+"x"+cReset {
		t.Fatal("color opts must wrap in the style and reset")
	}
}
