package native

import (
	"testing"

	eng "github.com/boru-lang/boru/eng/go"
)

// TestTemporalMatchDelegates keeps the temporal behaviors' Match and
// Equal delegate one-liners covered after the `is` one-predicate
// shortcut re-routed the paths that used to exercise them: each is a
// plain kernel-default delegate, so a value of the family matches its
// own node and a foreign node stays out, and equality follows the
// kernel's value equality.
func TestTemporalMatchDelegates(t *testing.T) {
	if !(timeOfDayFormatBehavior{}).Match(eng.NewCarrier(TTime), TTime) {
		t.Fatal("TimeOfDay delegate match failed")
	}
	if (clkDurationFormatBehavior{}).Match(eng.NewCarrier(TTime), TDate) {
		t.Fatal("ClockDuration delegate must reject a non-conforming node")
	}
	if !(timezoneFormatBehavior{}).Match(eng.NewCarrier(TDate), TTime) {
		t.Fatal("Timezone delegate match failed")
	}
	one, two := eng.NewInteger(1), eng.NewInteger(2)
	if !(timeOfDayFormatBehavior{}).Equal(one, one) || (timeOfDayFormatBehavior{}).Equal(one, two) {
		t.Fatal("timeOfDayFormatBehavior.Equal delegates to the kernel default")
	}
	if !(clkDurationFormatBehavior{}).Equal(one, one) || (clkDurationFormatBehavior{}).Equal(one, two) {
		t.Fatal("clkDurationFormatBehavior.Equal delegates to the kernel default")
	}
	if !(timezoneFormatBehavior{}).Equal(one, one) || (timezoneFormatBehavior{}).Equal(one, two) {
		t.Fatal("timezoneFormatBehavior.Equal delegates to the kernel default")
	}
}
