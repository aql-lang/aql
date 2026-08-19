package lang

import (
	"testing"

	"github.com/boru-lang/boru/lang/go/native"
)

// A fn handed BARE to a Function-typed parameter is USED — `boru check` must
// not report `unused_def` for it.
//
// The TFunction intercept in stepWord consumes the name as a VALUE rather than
// calling it, so nothing else on that path marked it. The result was a false
// positive on the single most common way a boru library takes a function:
// `Sort.quick mycmp xs`, `filter pred xs`, every comparator and predicate API
// in the ecosystem. The sibling ResolveRef path has recorded the use since the
// `export "X" { f: impl/v }` form was fixed; this brings the slot intercept
// into line with it.
//
// The negative half is the real contract: the note fires only on a SUCCESSFUL
// fn lookup, so a def that is genuinely never referenced must still warn.
func TestFunctionSlotArgIsNotUnused(t *testing.T) {
	cases := []struct {
		name       string
		src        string
		wantUnused bool
		why        string
	}{
		{
			name: "bare fn into a Function slot",
			src: "def mycmp fn [[b:Any a:Any] [Integer] [ a b cmp ]]\n" +
				"def use fn [[f:Function xs:List] [List] [ xs ]]\n" +
				"use mycmp [3 1 2]\n",
			wantUnused: false,
			why:        "mycmp IS used — it is the Function argument",
		},
		{
			name: "explicit /v reference",
			src: "def mycmp fn [[b:Any a:Any] [Integer] [ a b cmp ]]\n" +
				"def use fn [[f:Function xs:List] [List] [ xs ]]\n" +
				"use mycmp/v [3 1 2]\n",
			wantUnused: false,
			why:        "the /v spelling was already correct; it must stay correct",
		},
		{
			name: "a genuinely unused def still warns",
			src: "def never-called fn [[n:Integer] [Integer] [ n ]]\n" +
				"1 add 2\n",
			wantUnused: true,
			why:        "the note must fire only on a successful fn lookup at the slot",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a, err := New()
			if err != nil {
				t.Fatalf("New: %v", err)
			}
			res, err := a.Check(tc.src)
			if err != nil {
				t.Fatalf("check: %v", err)
			}
			got := false
			for _, d := range res.Diagnostics {
				if d.Code == "unused_def" {
					got = true
				}
				if d.Severity == native.SeverityError {
					t.Errorf("unexpected check error: [%s] %s", d.Code, d.Detail)
				}
			}
			if got != tc.wantUnused {
				t.Errorf("unused_def reported = %v, want %v — %s", got, tc.wantUnused, tc.why)
			}
		})
	}
}
