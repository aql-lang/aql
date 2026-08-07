package parser

import (
	"errors"
	"strings"
	"testing"

	core "github.com/boru-lang/boru/core/go"
	jsonic "github.com/tabnas/jsonic/go"
)

// Parse failures must surface as boru-voice [boru/syntax_error]s — same
// renderer, same position discipline, zero ANSI — never as raw library
// output (which used to leak an always-on color palette, an
// `--internal:` block, and the library docs link into the REPL and the
// wasm DOM).

// parseErrOf runs Parse on bad source and returns the translated error.
func parseErrOf(t *testing.T, src string) *core.BoruError {
	t.Helper()
	_, err := Parse(src)
	if err == nil {
		t.Fatalf("Parse(%q) must fail", src)
	}
	var ae *core.BoruError
	if !errors.As(err, &ae) {
		t.Fatalf("Parse(%q) must yield a *BoruError, got %T: %v", src, err, err)
	}
	return ae
}

func TestParseErrorTranslation(t *testing.T) {
	cases := []struct {
		src        string
		wantDetail string // substring of the boru-voice detail
		wantExtra  string // substring of a note/help line ("" = none asserted)
	}{
		{"(fn x]", "unexpected `]` — nothing valid can appear here", "check for a missing bracket, quote, or value"},
		{"def s 'abc", "this string is never closed", "add the closing quote"},
		{`def s "a\u{zzzz}b"`, "invalid unicode escape", "does not encode a valid unicode code point"},
		{`def s "a\x{zz}b"`, "invalid ascii escape", "does not encode a valid ASCII character"},
	}
	for _, c := range cases {
		t.Run(c.src, func(t *testing.T) {
			ae := parseErrOf(t, c.src)
			if ae.Code != "syntax_error" {
				t.Fatalf("code = %q, want syntax_error", ae.Code)
			}
			if ae.Row <= 0 {
				t.Fatalf("translated parse error must keep the source position: %v", ae)
			}
			rendered := ae.Error()
			if !strings.Contains(rendered, c.wantDetail) {
				t.Fatalf("missing detail %q in:\n%s", c.wantDetail, rendered)
			}
			if c.wantExtra != "" && !strings.Contains(rendered, c.wantExtra) {
				t.Fatalf("missing note/help %q in:\n%s", c.wantExtra, rendered)
			}
			if strings.Contains(rendered, "\x1b[") {
				t.Fatalf("parse error must be ANSI-free:\n%q", rendered)
			}
			if strings.Contains(rendered, "tabnas") || strings.Contains(rendered, "--internal") {
				t.Fatalf("library internals must not leak:\n%s", rendered)
			}
		})
	}
}

func TestParseErrorSyntheticCodes(t *testing.T) {
	// Codes a real Parse cannot produce here still translate sensibly.
	mk := func(code, detail, src string) *jsonic.JsonicError {
		return &jsonic.JsonicError{Code: code, Detail: detail, Row: 1, Col: 1, Src: src}
	}
	cases := []struct {
		te   *jsonic.JsonicError
		want string
	}{
		{mk("unterminated_comment", "unterminated comment: /*", "/*"), "this comment is never closed"},
		{mk("end_of_source", "unexpected end of source", ""), "the source ends in the middle of an expression"},
		{mk("unprintable", "unprintable character: \x01", "\x01"), "unprintable character inside a string"},
		{mk("internal", "internal error: boom", "boom"), "parser-internal failure"},
		{mk("some_future_code", "novel detail", "x"), "novel detail"},
	}
	for _, c := range cases {
		t.Run(c.te.Code, func(t *testing.T) {
			err := translateParseError(c.te, "x")
			var ae *core.BoruError
			if !errors.As(err, &ae) {
				t.Fatalf("want *BoruError, got %T", err)
			}
			if ae.Code != "syntax_error" {
				t.Fatalf("code = %q, want syntax_error", ae.Code)
			}
			if !strings.Contains(ae.Error(), c.want) {
				t.Fatalf("missing %q in:\n%s", c.want, ae.Error())
			}
		})
	}
}

func TestParseErrorNonJsonicFallback(t *testing.T) {
	// A non-library error keeps the historical wrap so no failure is
	// ever silently reshaped.
	err := translateParseError(errors.New("boom"), "src")
	if err == nil || err.Error() != "parse error: boom" {
		t.Fatalf("non-jsonic errors keep the parse error wrap, got %v", err)
	}
	var ae *core.BoruError
	if errors.As(err, &ae) {
		t.Fatal("non-jsonic errors must not be reshaped into BoruError")
	}
}
