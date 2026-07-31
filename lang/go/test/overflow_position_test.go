package test

import (
	"strings"
	"testing"

	"github.com/boru-lang/boru/lang/go"
)

// Integer-overflow errors must be located in the source (WAT Exhibit K,
// Phase 0). Runtime overflow (add/sub/mul/pow) is positioned by the
// engine's stampErrPos at the operator token; a literal that is out of
// range is positioned by the parser at the literal itself.
func TestIntegerOverflowErrorHasPosition(t *testing.T) {
	cases := []struct {
		name string
		src  string
		pos  string // expected "--> row:col" fragment
	}{
		{"runtime pow", "2 63 pow", "--> 1:6"},
		{"runtime mul", "4000000000 4000000000 mul", "--> 1:23"},
		{"runtime add", "9223372036854775807 add 1", "--> 1:21"},
		{"literal standalone", "9223372036854775808", "--> 1:1"},
		{"literal in expr", "add 1 9223372036854775808", "--> 1:7"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			a, err := lang.New()
			if err != nil {
				t.Fatalf("new: %v", err)
			}
			_, runErr := a.Run(c.src)
			if runErr == nil {
				t.Fatalf("Run(%q): expected an integer_overflow error, got nil", c.src)
			}
			msg := runErr.Error()
			if !strings.Contains(msg, "integer_overflow") {
				t.Errorf("Run(%q): expected integer_overflow, got: %s", c.src, msg)
			}
			if !strings.Contains(msg, c.pos) {
				t.Errorf("Run(%q): expected position %q in error, got: %s", c.src, c.pos, msg)
			}
		})
	}
}
