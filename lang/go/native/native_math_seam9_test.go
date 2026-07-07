package native

import (
	"testing"
	"time"
)

// Seam-9 coverage (W9_nativeC) for native_math.go: date/clock arithmetic
// handlers and the big-integer pow negative-exponent guard.

func w9ClockDur(d time.Duration) Value { return NewValueRaw(TTime, DurationPayload{D: d}) }

func TestW9DateTimeClockArith(t *testing.T) {
	base := time.Unix(0, 0).UTC()
	dur := w9ClockDur(90 * time.Second)

	// DateTime - ClockDuration: the neg branch negates the duration
	// (369.10,371.4).
	sub := dateTimeClkArith(true)
	out, err := sub([]Value{NewDateTime(base), dur}, nil, nil, nil)
	if err != nil || len(out) != 1 {
		t.Fatalf("dateTimeClkArith(neg): %v / %v", out, err)
	}
	got := AsDateTime(out[0])
	if !got.Equal(base.Add(-90 * time.Second)) {
		t.Fatalf("DateTime - 90s = %v, want %v", got, base.Add(-90*time.Second))
	}
}

func TestW9AddDateClock(t *testing.T) {
	base := time.Unix(0, 0).UTC()
	dur := w9ClockDur(3600 * time.Second)

	// Date + ClockDuration → DateTime (390.99,394.2).
	out, err := addDateClkHandler([]Value{NewDate(base), dur}, nil, nil, nil)
	if err != nil || len(out) != 1 {
		t.Fatalf("addDateClkHandler: %v / %v", out, err)
	}
	if !AsDateTime(out[0]).Equal(base.Add(3600 * time.Second)) {
		t.Fatalf("Date + 1h = %v", AsDateTime(out[0]))
	}
}
