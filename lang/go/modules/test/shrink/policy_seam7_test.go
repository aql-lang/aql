package shrink

import "testing"

// TestTransparency_StringAllVariants covers every arm of the
// Transparency.String() switch, including the default "Unknown"
// fallthrough for an out-of-range value.
func TestTransparency_StringAllVariants(t *testing.T) {
	cases := []struct {
		in   Transparency
		want string
	}{
		{Transparent, "Transparent"},
		{Generator, "Generator"},
		{Frozen, "Frozen"},
		{Opaque, "Opaque"},
	}
	for _, c := range cases {
		if got := c.in.String(); got != c.want {
			t.Errorf("Transparency(%d).String() = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestClassify_ShortNameStartsWithGuard covers the startsWith
// length guard (len(s) < len(prefix)): a name shorter than every
// prefix in the table can't match any, so Classify falls through to
// Opaque. The default prefixes ("rand-", "time-", "fetch-") are all
// at least 5 chars, so a 2-char name exercises the guard.
func TestClassify_ShortNameStartsWithGuard(t *testing.T) {
	p := DefaultPolicy()
	if got := p.Classify("hi"); got != Opaque {
		t.Errorf("Classify(short unknown) = %s, want Opaque", got)
	}
	// Empty name is the extreme boundary.
	if got := p.Classify(""); got != Opaque {
		t.Errorf("Classify(\"\") = %s, want Opaque", got)
	}
}

// TestTransparency_StringUnknown is the negative/boundary case: a
// Transparency value outside the declared constants renders as
// "Unknown" (the switch default).
func TestTransparency_StringUnknown(t *testing.T) {
	if got := Transparency(99).String(); got != "Unknown" {
		t.Errorf("Transparency(99).String() = %q, want %q", got, "Unknown")
	}
	// -1 is also out of range on the low side.
	if got := Transparency(-1).String(); got != "Unknown" {
		t.Errorf("Transparency(-1).String() = %q, want %q", got, "Unknown")
	}
}
