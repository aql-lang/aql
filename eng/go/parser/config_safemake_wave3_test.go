package parser

import (
	"strings"
	"testing"

	"github.com/aql-lang/aql/eng/go"
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
	for _, src := range []string{"[1,2,3]", "just-a-string"} {
		_, err := ParseConfig(src)
		if err == nil || !strings.Contains(err.Error(), "must be a map") {
			t.Errorf("ParseConfig(%q): expected the non-map error, got %v", src, err)
		}
	}
}

func TestParseConfigWave3InvalidSyntax(t *testing.T) {
	for _, src := range []string{`"`, "a:'unclosed", ":::"} {
		_, err := ParseConfig(src)
		if err == nil || !strings.Contains(err.Error(), "invalid options syntax") {
			t.Errorf("ParseConfig(%q): expected an invalid-syntax error, got %v", src, err)
		}
	}
}

// --- SafeParse / SafeParseData / GuardMake ---

func TestSafeParseWave3(t *testing.T) {
	v, err := SafeParse("a:1")
	if err != nil {
		t.Fatalf("SafeParse(a:1): %v", err)
	}
	m, ok := v.(map[string]any)
	if !ok || m["a"] != float64(1) {
		t.Errorf("SafeParse(a:1) = %#v, want map[a:1]", v)
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
	if f, ferr := eng.AsFloat(ev); ferr != nil || f != 42.0 {
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
	if n, nerr := eng.AsInteger(ev); nerr != nil || n != 42 {
		t.Errorf("ConvertParsedNumber(42) = %v, want Integer 42", ev)
	}
	// A malformed separator errors through the same seam.
	v, err = SafeParseData("1__0")
	if err != nil {
		t.Fatalf("SafeParseData(1__0): %v", err)
	}
	if _, ok, cerr := ConvertParsedNumber(v); ok && cerr == nil {
		t.Errorf("ConvertParsedNumber(1__0): expected an error, got none")
	}
	// A non-number input declines (ok=false, no error).
	if _, ok, cerr := ConvertParsedNumber("not-a-number"); ok || cerr != nil {
		t.Errorf("ConvertParsedNumber(string): want ok=false err=nil, got ok=%v err=%v", ok, cerr)
	}
}

func TestGuardMakeWave3(t *testing.T) {
	got := GuardMake(func() int { return 7 })
	if got != 7 {
		t.Errorf("GuardMake: got %d, want 7", got)
	}
}
