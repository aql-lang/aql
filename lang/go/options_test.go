package lang

import "testing"

func TestParseOptionsNesting(t *testing.T) {
	// Colon chains nest (the documented --options syntax).
	m, err := ParseOptions("tape:initial:65536,tape:grows:9")
	if err != nil {
		t.Fatalf("ParseOptions: %v", err)
	}
	tape, ok := m["tape"].(map[string]any)
	if !ok {
		t.Fatalf("tape not a map: %#v", m["tape"])
	}
	// jsonic numbers arrive as int or int64; coerce before comparing.
	if got := asI64(tape["initial"]); got != 65536 {
		t.Errorf("initial = %v, want 65536", tape["initial"])
	}
	if got := asI64(tape["grows"]); got != 9 {
		t.Errorf("grows = %v, want 9", tape["grows"])
	}
}

func asI64(v any) int64 {
	switch n := v.(type) {
	case int:
		return int64(n)
	case int64:
		return n
	case float64:
		return int64(n)
	default:
		return -1
	}
}

func TestApplyOptionsTape(t *testing.T) {
	var o Options
	m, err := ParseOptions("tape:initial:4096,tape:grows:5,tape:factor:3.0")
	if err != nil {
		t.Fatalf("ParseOptions: %v", err)
	}
	if err := ApplyOptions(&o, m); err != nil {
		t.Fatalf("ApplyOptions: %v", err)
	}
	if o.Tape.InitialSize != 4096 {
		t.Errorf("InitialSize = %d, want 4096", o.Tape.InitialSize)
	}
	if o.Tape.MaxGrows != 5 {
		t.Errorf("MaxGrows = %d, want 5", o.Tape.MaxGrows)
	}
	if o.Tape.GrowthFactor != 3.0 {
		t.Errorf("GrowthFactor = %v, want 3.0", o.Tape.GrowthFactor)
	}
}

// Strictness: unknown keys and malformed values must error (fail loud),
// never be silently dropped.
func TestApplyOptionsRejectsBadInput(t *testing.T) {
	cases := []string{
		"tape:nope:10",     // misspelled key in a known section
		"bogus:1",          // unknown top-level section
		"tape:initial:1.5", // non-integer where an int is required
		"tape:factor:fast", // non-number factor (parses to nested map)
	}
	for _, src := range cases {
		var o Options
		m, err := ParseOptions(src)
		if err != nil {
			continue // a parse-level rejection is also "fails loud"
		}
		if err := ApplyOptions(&o, m); err == nil {
			t.Errorf("ApplyOptions(%q) = nil error, want a rejection", src)
		}
	}
}

func TestParseOptionsTopLevelMustBeMap(t *testing.T) {
	if _, err := ParseOptions("[1,2,3]"); err == nil {
		t.Error("a non-map option blob should be rejected")
	}
}
