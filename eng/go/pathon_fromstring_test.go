package eng

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// TestNewPathonFromString pins the exported host-side Pathon constructor
// against the `make Pathon "<s>"` parse it wraps: POSIX relative and
// absolute forms, and the Windows drive form.
func TestNewPathonFromString(t *testing.T) {
	cases := []struct {
		in, want string
		abs      bool
	}{
		{"a/b.txt", "a/b.txt", false},
		{"/usr/bin", "/usr/bin", true},
		{"C:/Users/foo", `C:\Users\foo`, true},
		{"", "", false},
	}
	for _, c := range cases {
		v := core.NewPathonFromString(c.in)
		if !core.IsPathon(v) {
			t.Fatalf("NewPathonFromString(%q) is not a Pathon: %v", c.in, v)
		}
		info, err := core.AsPathon(v)
		if err != nil {
			t.Fatalf("AsPathon(%q): %v", c.in, err)
		}
		if info.String() != c.want || info.Abs != c.abs {
			t.Errorf("NewPathonFromString(%q) = %q abs=%v, want %q abs=%v",
				c.in, info.String(), info.Abs, c.want, c.abs)
		}
	}
	// Parity with the `make Pathon` string arm.
	made, err := core.MakePathon(core.NewString("x/y"), false)
	if err != nil {
		t.Fatal(err)
	}
	direct := core.NewPathonFromString("x/y")
	if made[0].String() != direct.String() {
		t.Errorf("make-parity: %s vs %s", made[0], direct)
	}
}
