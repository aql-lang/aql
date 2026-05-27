package policy

import "testing"

func TestGlobBasic(t *testing.T) {
	tests := []struct {
		pat, s string
		want   bool
	}{
		// exact
		{"add", "add", true},
		{"add", "sub", false},
		{"", "", true},
		{"", "x", false},
		{"x", "", false},
		// ? matches one non-sep char
		{"?dd", "add", true},
		{"a??", "abc", true},
		{"a??", "ab", false},
		{"a?b", "a/b", false},
		// * matches any non-sep run
		{"a*", "add", true},
		{"*", "anything", true},
		{"*", "with/slash", false},
		{"*.aql", "main.aql", true},
		{"*.aql", "main.go", false},
		// ** matches any run including separators
		{"**", "any/thing/at/all", true},
		{"/tmp/**", "/tmp/aql/output", true},
		{"/tmp/**", "/tmp", false},
		{"/tmp/**", "/tmp/x", true},
		{"**/foo", "a/b/c/foo", true},
		{"**/foo", "foo", true},
		{"**/foo", "afoo", false},
		// composed
		{"aql:*", "aql:math", true},
		{"aql:*", "aql:time", true},
		{"aql:math", "aql:time", false},
		{"read-*", "read-file", true},
		{"read-*", "read", false},
		{"read-*", "write-file", false},
	}
	for _, tt := range tests {
		got := Glob(tt.pat, tt.s)
		if got != tt.want {
			t.Errorf("Glob(%q, %q) = %v, want %v", tt.pat, tt.s, got, tt.want)
		}
	}
}

func TestGlobAny(t *testing.T) {
	pats := []string{"add", "sub", "mul"}
	if !GlobAny(pats, "add") {
		t.Error("expected add to match")
	}
	if GlobAny(pats, "div") {
		t.Error("expected div to not match")
	}
	if GlobAny(nil, "add") {
		t.Error("nil pats should not match")
	}
}
