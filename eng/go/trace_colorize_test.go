package eng

import (
	"strings"
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// stripANSI removes the color escapes so assertions read plainly.
func stripANSI(s string) string {
	var b strings.Builder
	inEsc := false
	for _, r := range s {
		if r == '\033' {
			inEsc = true
			continue
		}
		if inEsc {
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
				inEsc = false
			}
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// TraceColorize must render the PAYLOAD value, not the sealed payload
// struct. The String/Integer arms used to apply %q/%d to v.Data — the
// wrapper struct — after Data became a sealed interface, so a traced 5
// rendered as "{5}" and "hi" as `{"hi"}`.
func TestTraceColorizeRendersPayloads(t *testing.T) {
	cases := []struct {
		name string
		v    core.Value
		want string
	}{
		{"string", core.NewString("hi"), `"hi"`},
		{"integer", core.NewInteger(5), "5"},
		{"negative_integer", core.NewInteger(-42), "-42"},
		{"bool_true", core.NewBoolean(true), "true"},
		{"bool_false", core.NewBoolean(false), "false"},
		{"atom", core.NewAtom("sym"), "sym"},
		{"type_literal", core.NewTypeLiteral(core.TInteger), "Integer"},
		{"list", core.NewList([]core.Value{core.NewInteger(1), core.NewString("a")}), `[1 "a"]`},
	}
	for _, c := range cases {
		got := stripANSI(core.TraceColorize(c.v))
		if got != c.want {
			t.Errorf("%s: TraceColorize = %q, want %q", c.name, got, c.want)
		}
		// Negative: the sealed-payload struct rendering must never
		// reappear (a "{" wrapping a scalar is the bug signature).
		if c.name == "string" || c.name == "integer" {
			if strings.Contains(got, "{") {
				t.Errorf("%s: TraceColorize = %q renders the payload STRUCT, not the value", c.name, got)
			}
		}
	}
}
