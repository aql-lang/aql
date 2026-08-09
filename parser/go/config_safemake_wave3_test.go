package parser

import (
	"errors"
	"reflect"
	"testing"

	jsonic "github.com/tabnas/jsonic/go"

	core "github.com/boru-lang/boru/core/go"
)

// --- ParseConfig (the CLI --options backing parser) ---

func TestParseConfigWave3Valid(t *testing.T) {
	m, err := ParseConfig("a:1,b:c:2")
	if err != nil {
		t.Fatalf("ParseConfig(a:1,b:c:2): %v", err)
	}
	if got, ok := m["a"].(float64); !ok || got != 1 {
		t.Errorf("a = %v (%T), want 1", m["a"], m["a"])
	}
	inner, ok := m["b"].(map[string]any)
	if !ok {
		t.Fatalf("b = %v, want a nested map (colon chains nest)", m["b"])
	}
	if got, ok := inner["c"].(float64); !ok || got != 2 {
		t.Errorf("b.c = %v (%T), want 2", inner["c"], inner["c"])
	}
}

func TestParseConfigWave3NonMapTopLevel(t *testing.T) {
	for _, tc := range []struct {
		src, want string
	}{
		{src: "[1,2,3]", want: "options must be a map of key:value pairs, got list"},
		{src: "just-a-string", want: "options must be a map of key:value pairs, got string"},
		{src: "null", want: "options must be a map of key:value pairs, got scalar"},
	} {
		_, err := ParseConfig(tc.src)
		if err == nil || err.Error() != tc.want {
			t.Errorf("ParseConfig(%q): want %q, got %v", tc.src, tc.want, err)
		}
	}
}

func TestParseConfigWave3InvalidSyntax(t *testing.T) {
	for _, src := range []string{`"`, "a:'unclosed", ":::"} {
		_, err := ParseConfig(src)
		if err == nil || err.Error() != configSyntaxMessage {
			t.Errorf("ParseConfig(%q): want exact outer message %q, got %v", src, configSyntaxMessage, err)
			continue
		}
		if errors.Unwrap(err) == nil {
			t.Errorf("ParseConfig(%q): stable outer error lost the jsonic cause", src)
		}
	}
}

// --- SafeParse / SafeParseData / GuardMake ---

func TestSafeParseWave3(t *testing.T) {
	v, err := SafeParse("a:1")
	if err != nil {
		t.Fatalf("SafeParse(a:1): %v", err)
	}
	// Objects parse into the insertion-ordered OrderedMap now; flatten to a
	// plain map to check the value.
	m, ok := jsonic.Plainify(v).(map[string]any)
	if !ok || m["a"] != float64(1) {
		t.Errorf("SafeParse(a:1) = %#v, want {a:1}", v)
	}
}

// TestSafeParseDataWave3 pins the numeric round-trip contract: numbers come
// back wrapped so ConvertParsedNumber can preserve the int/float split.
func TestSafeParseDataWave3(t *testing.T) {
	// "42.0" must stay a Float (the bare SafeParse would collapse it).
	v, err := SafeParseData("42.0")
	if err != nil {
		t.Fatalf("SafeParseData(42.0): %v", err)
	}
	ev, ok, err := ConvertParsedNumber(v)
	if err != nil || !ok {
		t.Fatalf("ConvertParsedNumber(42.0): ok=%v err=%v", ok, err)
	}
	if f, ferr := core.AsFloat(ev); ferr != nil || f != 42.0 {
		t.Errorf("ConvertParsedNumber(42.0) = %v, want Float 42.0", ev)
	}
	// "42" stays an Integer.
	v, err = SafeParseData("42")
	if err != nil {
		t.Fatalf("SafeParseData(42): %v", err)
	}
	ev, ok, err = ConvertParsedNumber(v)
	if err != nil || !ok {
		t.Fatalf("ConvertParsedNumber(42): ok=%v err=%v", ok, err)
	}
	if n, nerr := core.AsInteger(ev); nerr != nil || n != 42 {
		t.Errorf("ConvertParsedNumber(42) = %v, want Integer 42", ev)
	}
	// The public data seam must retain the same complete numeric token
	// boundaries as Parse and LexTokens. These are exactly the stock-Go-jsonic
	// fallback classes that otherwise arrive as an unwrapped float or text and
	// make ConvertParsedNumber silently decline them.
	for _, tc := range []struct {
		src, wantCanon, wantCode string
		wantRow, wantCol         int
	}{
		{src: "1.e2", wantCanon: "100.0"},
		{src: "1e400", wantCode: "float_overflow"},
		{src: "1.0e400", wantCode: "float_overflow"},
		{src: "0x8000000000000000", wantCode: "integer_overflow", wantRow: 1, wantCol: 1},
		{src: "+.1e400", wantCode: "syntax_error", wantRow: 1, wantCol: 1},
		{src: "1_.2", wantCode: "syntax_error", wantRow: 1, wantCol: 1},
		{src: "1.2_", wantCode: "syntax_error", wantRow: 1, wantCol: 1},
		{src: "1e_2", wantCode: "syntax_error", wantRow: 1, wantCol: 1},
	} {
		v, parseErr := SafeParseData(tc.src)
		if parseErr != nil {
			t.Errorf("SafeParseData(%q): %v", tc.src, parseErr)
			continue
		}
		got, claimed, convertErr := ConvertParsedNumber(v)
		if !claimed {
			t.Errorf("ConvertParsedNumber(SafeParseData(%q)): declined wrapped number", tc.src)
			continue
		}
		if tc.wantCode == "" {
			if convertErr != nil || core.CanonValue(got) != tc.wantCanon {
				t.Errorf("ConvertParsedNumber(SafeParseData(%q)) = %q, %v; want %q", tc.src, core.CanonValue(got), convertErr, tc.wantCanon)
			}
			continue
		}
		var be *core.BoruError
		if !errors.As(convertErr, &be) || be.Code != tc.wantCode || be.Src != tc.src || be.Row != tc.wantRow || be.Col != tc.wantCol {
			t.Errorf("ConvertParsedNumber(SafeParseData(%q)) error = %#v; want %s at %d:%d over full token", tc.src, convertErr, tc.wantCode, tc.wantRow, tc.wantCol)
		}
	}
	// A malformed separator errors through the same seam.
	v, err = SafeParseData("1__0")
	if err != nil {
		t.Fatalf("SafeParseData(1__0): %v", err)
	}
	if _, ok, cerr := ConvertParsedNumber(v); ok && cerr == nil {
		t.Errorf("ConvertParsedNumber(1__0): expected an error, got none")
	}
	// Jsonic's Go lexer counts source columns in Unicode code points. The
	// public data seam must retain that location on NumberVal diagnostics.
	v, err = SafeParseData("[🙂,1_]")
	if err != nil {
		t.Fatalf("SafeParseData(unicode list): %v", err)
	}
	items, ok := v.([]any)
	if !ok || len(items) != 2 {
		t.Fatalf("SafeParseData(unicode list) = %#v, want two items", v)
	}
	_, ok, cerr := ConvertParsedNumber(items[1])
	be, isBoru := cerr.(*core.BoruError)
	if !ok || !isBoru || be.Row != 1 || be.Col != 4 {
		t.Errorf("unicode numeric diagnostic = %#v (ok=%v), want 1:4", cerr, ok)
	}
	// A non-number input declines (ok=false, no error).
	if _, ok, cerr := ConvertParsedNumber("not-a-number"); ok || cerr != nil {
		t.Errorf("ConvertParsedNumber(string): want ok=false err=nil, got ok=%v err=%v", ok, cerr)
	}
}

// TestSafeParseDataLenientText pins the jsonic-superset side of the data
// seam: a digit-led run that is not a complete Boru numeric spelling stays
// lenient text, exactly as the stock data grammar decodes it. The data-mode
// matcher must never split a run — `1.x` is one string, never [1, '.x'].
func TestSafeParseDataLenientText(t *testing.T) {
	for _, tc := range []struct {
		src  string
		want any
	}{
		{src: "{v: 1.2.3}", want: map[string]any{"v": "1.2.3"}},
		{src: "1.x", want: "1.x"},
		{src: "{p: 1.2-beta}", want: map[string]any{"p": "1.2-beta"}},
		{src: "1..2", want: "1..2"},
		{src: "{a: 1.5suffix}", want: map[string]any{"a": "1.5suffix"}},
		// An empty exponent after a dot declines rather than splitting.
		{src: "{a: 1.e}", want: map[string]any{"a": "1.e"}},
		// A signed leading-dot run followed by text declines whole.
		{src: "+.5x", want: "+.5x"},
		// Language operators (`=`, `;`) are plain text in a data grammar.
		{src: "{a: 1=2}", want: map[string]any{"a": "1=2"}},
		{src: "1;2", want: "1;2"},
	} {
		v, err := SafeParseData(tc.src)
		if err != nil {
			t.Errorf("SafeParseData(%q): %v", tc.src, err)
			continue
		}
		if got := jsonic.Plainify(v); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("SafeParseData(%q) = %#v, want %#v", tc.src, got, tc.want)
		}
	}
}

func TestGuardMakeWave3(t *testing.T) {
	got := GuardMake(func() int { return 7 })
	if got != 7 {
		t.Errorf("GuardMake: got %d, want 7", got)
	}
}
