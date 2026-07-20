package native

import (
	"strings"
	"testing"
	"time"
)

// The affine temporal arithmetic handlers (native_math.go). The
// end-to-end word behaviour is pinned by lang/spec/module-time.tsv §23;
// these Go tests reach what the frozen-clock UTC spec harness cannot:
// the civil-day Date subtraction across a DST transition, the pre-epoch
// arm of civilDayNumber, and the checked ClockDuration overflow
// (PR #292 review findings).

func affineTT(t *testing.T) (*Registry, TemporalModuleTypes) {
	t.Helper()
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	return r, MintTemporalModuleTypes(r)
}

func TestCivilDayNumber(t *testing.T) {
	if d := civilDayNumber(time.Date(1970, 1, 1, 0, 0, 0, 0, time.UTC)); d != 0 {
		t.Errorf("epoch day = %d, want 0", d)
	}
	if d := civilDayNumber(time.Date(1970, 1, 2, 12, 30, 0, 0, time.UTC)); d != 1 {
		t.Errorf("day after epoch (mid-day) = %d, want 1", d)
	}
	// The pre-epoch arm: floor semantics keep day arithmetic exact.
	if d := civilDayNumber(time.Date(1969, 12, 31, 23, 0, 0, 0, time.UTC)); d != -1 {
		t.Errorf("day before epoch = %d, want -1", d)
	}
}

func TestDateSubCivilDaysAcrossDST(t *testing.T) {
	r, tt := affineTT(t)
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Skipf("tz database unavailable: %v", err)
	}
	// 2024-03-10 is the US spring-forward date: the local midnights of
	// Mar 10 and Mar 11 are 23 hours apart. A duration/24h computation
	// truncates that to 0; the civil-day difference is exactly 1.
	d0 := NewDate(time.Date(2024, 3, 10, 0, 0, 0, 0, loc))
	d1 := NewDate(time.Date(2024, 3, 11, 0, 0, 0, 0, loc))
	h := dateSubHandler(tt)
	out, herr := h([]Value{d1, d0}, nil, nil, r)
	if herr != nil {
		t.Fatal(herr)
	}
	cd, ok := AsCalendarDuration(out[0])
	if !ok || cd.Days != 1 || cd.Years != 0 || cd.Months != 0 {
		t.Errorf("DST-spanning Date sub = %+v, want 1 day", cd)
	}
	// And the reverse direction is exactly -1, not a truncated 0.
	out, _ = h([]Value{d0, d1}, nil, nil, r)
	if cd, _ := AsCalendarDuration(out[0]); cd.Days != -1 {
		t.Errorf("reverse DST-spanning Date sub = %+v, want -1 day", cd)
	}
}

func TestClockDurArithChecked(t *testing.T) {
	r, tt := affineTT(t)
	addH := clockDurArith(tt, false)
	subH := clockDurArith(tt, true)
	// Plain arithmetic stays exact.
	out, err := addH([]Value{tt.NewClockDuration(2 * time.Hour), tt.NewClockDuration(30 * time.Minute)}, nil, nil, r)
	if err != nil {
		t.Fatal(err)
	}
	if d, _ := AsClockDuration(out[0]); d != 2*time.Hour+30*time.Minute {
		t.Errorf("clock add = %v", d)
	}
	// Overflow raises instead of wrapping (WAT Exhibit K policy).
	maxD := tt.NewClockDuration(time.Duration(1<<63 - 1))
	if _, err := addH([]Value{maxD, tt.NewClockDuration(time.Hour)}, nil, nil, r); err == nil ||
		!strings.Contains(err.Error(), "overflow") {
		t.Errorf("clock add overflow: got %v, want overflow error", err)
	}
	minD := tt.NewClockDuration(time.Duration(-1 << 63))
	if _, err := subH([]Value{minD, tt.NewClockDuration(time.Hour)}, nil, nil, r); err == nil ||
		!strings.Contains(err.Error(), "overflow") {
		t.Errorf("clock sub overflow: got %v, want overflow error", err)
	}
}
