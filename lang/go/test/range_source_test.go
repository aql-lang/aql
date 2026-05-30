package test

import (
	"testing"
)

// range parses and runs through the real parser, and composes with
// other array words just like iota.
func TestRangeSource(t *testing.T) {
	cases := []struct {
		expr string
		want string
	}{
		{`range 2 6`, "[2,3,4,5]"},
		{`range 0 10 3`, "[0,3,6,9]"},
		{`range 5 0 -1`, "[5,4,3,2,1]"},
		{`range 0 5 1`, "[0,1,2,3,4]"}, // == iota 5
		{`iota 5`, "[0,1,2,3,4]"},
		// composes with each, same as iota
		{`range 1 4 each [dup mul]`, "[1,4,9]"},
		// reshape a range into a grid
		{`range 0 6 reshape [2,3]`, "[[0,1,2],[3,4,5]]"},
	}
	for _, tc := range cases {
		t.Run(tc.expr, func(t *testing.T) {
			result, err := runNativeSteps(t, nil, []string{tc.expr})
			if err != nil {
				t.Fatalf("%q: unexpected error: %v", tc.expr, err)
			}
			assertResult(t, result, tc.want)
		})
	}
}

// A type literal in an arg slot must not panic (Panic Prevention rule).
func TestRangeTypeLiteralNoPanic(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("range with type-literal arg panicked: %v", r)
		}
	}()
	_, _ = runNativeSteps(t, nil, []string{`range Integer 6`})
}
