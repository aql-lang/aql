package pathutil

import (
	"os"
	"path/filepath"
	"testing"
)

func TestExpandTilde(t *testing.T) {
	home := filepath.FromSlash("/home/u")
	cases := []struct {
		in, home, want string
	}{
		{"~", home, home},
		{"~/.boru", home, filepath.Join(home, ".boru")},
		{filepath.Join("~", "a", "b"), home, filepath.Join(home, "a", "b")},
		{"~/", home, home},
		{"/abs/path", home, "/abs/path"}, // no tilde
		{"rel/path", home, "rel/path"},   // no tilde
		{"~user/x", home, "~user/x"},     // other-user home, untouched
		{"~/.boru", "", "~/.boru"},       // unknown home, untouched
		{"", home, ""},                   // empty
		{"a~b", home, "a~b"},             // tilde not leading
	}
	for _, c := range cases {
		if got := ExpandTilde(c.in, c.home); got != c.want {
			t.Errorf("ExpandTilde(%q, %q) = %q, want %q", c.in, c.home, got, c.want)
		}
	}
}

func TestExpand(t *testing.T) {
	// Non-tilde inputs are OS-independent.
	if got := Expand("/abs"); got != "/abs" {
		t.Errorf("Expand(/abs) = %q, want unchanged", got)
	}
	if got := Expand(""); got != "" {
		t.Errorf("Expand(\"\") = %q, want empty", got)
	}
	// The tilde case resolves against the real OS home (read-only).
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skipf("no home dir reported: %v", err)
	}
	if got := Expand("~/sub"); got != filepath.Join(home, "sub") {
		t.Errorf("Expand(~/sub) = %q, want under %q", got, home)
	}
}
