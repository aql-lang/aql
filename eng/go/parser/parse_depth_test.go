package parser

import (
	"strings"
	"testing"
)

// Deeply nested source must be REJECTED with a clean evaluation_limit error
// rather than overflowing the Go stack during conversion (a fatal, unrecoverable
// crash). These tests pin the maxParseNestingDepth guard added to Parse.

// deepList builds d-deep nested word-context lists: `[[[ … ]]]`.
func deepList(d int) string {
	return strings.Repeat("[", d) + "1" + strings.Repeat("]", d)
}

// deepParens builds d-deep nested paren groups: `(( … ))`.
func deepParens(d int) string {
	return strings.Repeat("(", d) + "1" + strings.Repeat(")", d)
}

// deepMap builds d-deep nested data-context maps: `{a:{a:{ … }}}`.
func deepMap(d int) string {
	return "{" + strings.Repeat("a:{", d) + "x:1" + strings.Repeat("}", d) + "}"
}

func TestParseRejectsDeeplyNestedSource(t *testing.T) {
	cases := []struct {
		name string
		src  string
	}{
		{"lists", deepList(maxParseNestingDepth + 50)},
		{"parens", deepParens(maxParseNestingDepth + 50)},
		{"maps", deepMap(maxParseNestingDepth + 50)},
		// The depth that crashed the process before the guard (Finding 1).
		{"crash-repro", strings.Repeat("if true [", 200000) + "1" + strings.Repeat("]", 200000)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			func() {
				defer func() {
					if r := recover(); r != nil {
						t.Fatalf("%s: Parse panicked instead of erroring cleanly: %v", tc.name, r)
					}
				}()
				_, err := Parse(tc.src)
				if err == nil {
					t.Fatalf("%s: deeply nested source parsed without error; the depth guard did not fire", tc.name)
				}
				if !strings.Contains(err.Error(), "evaluation_limit") &&
					!strings.Contains(err.Error(), "nesting") {
					t.Fatalf("%s: error %q is not the nesting-depth diagnostic", tc.name, err)
				}
			}()
		})
	}
}

func TestParseAcceptsModeratelyNestedSource(t *testing.T) {
	// Negative control: nesting well under the ceiling must parse cleanly, so
	// the guard cannot be regressing legitimate (even unusually deep) programs.
	for _, tc := range []struct {
		name string
		src  string
	}{
		{"lists", deepList(500)},
		{"parens", deepParens(500)},
		{"maps", deepMap(500)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := Parse(tc.src); err != nil {
				t.Fatalf("%s: moderately nested source (depth 500) was rejected: %v", tc.name, err)
			}
		})
	}
}
