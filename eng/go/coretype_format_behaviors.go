package eng

import (
	"fmt"
	"time"
)

// This file installs per-Type TypeBehavior overrides that pull the
// domain render arms out of Value.String's switch. Each Behavior
// overrides Format only; Match and Equal delegate to DefaultBehavior
// so signature matching and value equality are unchanged.
//
// The init() function runs after Builtin is constructed (package
// init order: var initialisers first, then init functions), so
// TDate/TDateTime/etc. exist by the time we reach init().
//
// Lifecycle note: these Behaviors are kernel-internal until Step 9
// of the migration, when TDate/TMatrix/etc. move out of eng/. At
// that point the Behaviors travel with their types to
// lang/internal/nativemod/* and this file goes away.

func init() {
	TInstant.Behavior = instantFormatBehavior{}
	TDateTime.Behavior = dateTimeFormatBehavior{}
	TDate.Behavior = dateFormatBehavior{}
	TTimeOfDay.Behavior = timeOfDayFormatBehavior{}
	TCalDuration.Behavior = calDurationFormatBehavior{}
	TClkDuration.Behavior = clkDurationFormatBehavior{}
	TTimezone.Behavior = timezoneFormatBehavior{}
	// TMatrix Behavior moved to lang/internal/nativemod/matrix.go (Step 8).
	TTimeout.Behavior = timeoutFormatBehavior{}
	TInterval.Behavior = intervalFormatBehavior{}
}

// instantFormatBehavior renders Instant values as RFC3339Nano.
type instantFormatBehavior struct{}

func (instantFormatBehavior) Match(v Value, t *Type) bool { return DefaultBehavior.Match(v, t) }
func (instantFormatBehavior) Equal(a, b Value) bool       { return DefaultBehavior.Equal(a, b) }
func (instantFormatBehavior) Format(v Value) string {
	if tp, ok := v.Data.(TimePayload); ok {
		if t, ok := tp.T.(time.Time); ok {
			return t.Format(time.RFC3339Nano)
		}
	}
	return "Instant(nil)"
}

// dateTimeFormatBehavior renders DateTime as "2006-01-02T15:04:05.999999999".
type dateTimeFormatBehavior struct{}

func (dateTimeFormatBehavior) Match(v Value, t *Type) bool { return DefaultBehavior.Match(v, t) }
func (dateTimeFormatBehavior) Equal(a, b Value) bool       { return DefaultBehavior.Equal(a, b) }
func (dateTimeFormatBehavior) Format(v Value) string {
	if tp, ok := v.Data.(TimePayload); ok {
		if t, ok := tp.T.(time.Time); ok {
			return t.Format("2006-01-02T15:04:05.999999999")
		}
	}
	return "DateTime(nil)"
}

// dateFormatBehavior renders Date as "2006-01-02".
type dateFormatBehavior struct{}

func (dateFormatBehavior) Match(v Value, t *Type) bool { return DefaultBehavior.Match(v, t) }
func (dateFormatBehavior) Equal(a, b Value) bool       { return DefaultBehavior.Equal(a, b) }
func (dateFormatBehavior) Format(v Value) string {
	if tp, ok := v.Data.(TimePayload); ok {
		if t, ok := tp.T.(time.Time); ok {
			return t.Format("2006-01-02")
		}
	}
	return "Date(nil)"
}

// timeOfDayFormatBehavior renders TimeOfDay as HH:MM:SS[.NS].
type timeOfDayFormatBehavior struct{}

func (timeOfDayFormatBehavior) Match(v Value, t *Type) bool { return DefaultBehavior.Match(v, t) }
func (timeOfDayFormatBehavior) Equal(a, b Value) bool       { return DefaultBehavior.Equal(a, b) }
func (timeOfDayFormatBehavior) Format(v Value) string {
	dp, ok := v.Data.(DurationPayload)
	if !ok {
		return "TimeOfDay(nil)"
	}
	d, ok := dp.D.(time.Duration)
	if !ok {
		return "TimeOfDay(nil)"
	}
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	s := int(d.Seconds()) % 60
	ns := d.Nanoseconds() % 1e9
	if ns != 0 {
		return fmt.Sprintf("%02d:%02d:%02d.%09d", h, m, s, ns)
	}
	return fmt.Sprintf("%02d:%02d:%02d", h, m, s)
}

// calDurationFormatBehavior renders CalDuration as PnYnMnD.
type calDurationFormatBehavior struct{}

func (calDurationFormatBehavior) Match(v Value, t *Type) bool { return DefaultBehavior.Match(v, t) }
func (calDurationFormatBehavior) Equal(a, b Value) bool       { return DefaultBehavior.Equal(a, b) }
func (calDurationFormatBehavior) Format(v Value) string {
	if cd, ok := v.Data.(CalDurationData); ok {
		return fmt.Sprintf("P%dY%dM%dD", cd.Years, cd.Months, cd.Days)
	}
	return "CalDuration(nil)"
}

// clkDurationFormatBehavior renders ClkDuration as time.Duration.String().
type clkDurationFormatBehavior struct{}

func (clkDurationFormatBehavior) Match(v Value, t *Type) bool { return DefaultBehavior.Match(v, t) }
func (clkDurationFormatBehavior) Equal(a, b Value) bool       { return DefaultBehavior.Equal(a, b) }
func (clkDurationFormatBehavior) Format(v Value) string {
	if dp, ok := v.Data.(DurationPayload); ok {
		if d, ok := dp.D.(time.Duration); ok {
			return d.String()
		}
	}
	return "ClkDuration(nil)"
}

// timezoneFormatBehavior renders Timezone as the location name.
type timezoneFormatBehavior struct{}

func (timezoneFormatBehavior) Match(v Value, t *Type) bool { return DefaultBehavior.Match(v, t) }
func (timezoneFormatBehavior) Equal(a, b Value) bool       { return DefaultBehavior.Equal(a, b) }
func (timezoneFormatBehavior) Format(v Value) string {
	if tp, ok := v.Data.(TimezonePayload); ok {
		if loc, ok := tp.Loc.(*time.Location); ok {
			return loc.String()
		}
	}
	return "Timezone(nil)"
}

// matrixFormatBehavior moved to lang/internal/nativemod/matrix.go (Step 8).

// timeoutFormatBehavior renders Timeout as Timeout(id,Nms).
type timeoutFormatBehavior struct{}

func (timeoutFormatBehavior) Match(v Value, t *Type) bool { return DefaultBehavior.Match(v, t) }
func (timeoutFormatBehavior) Equal(a, b Value) bool       { return DefaultBehavior.Equal(a, b) }
func (timeoutFormatBehavior) Format(v Value) string {
	if ti, ok := v.Data.(*TimeoutInfo); ok {
		return fmt.Sprintf("Timeout(%s,%dms)", ti.ID, ti.Ms)
	}
	return "Timeout(nil)"
}

// intervalFormatBehavior renders Interval as Interval(id,Nms).
type intervalFormatBehavior struct{}

func (intervalFormatBehavior) Match(v Value, t *Type) bool { return DefaultBehavior.Match(v, t) }
func (intervalFormatBehavior) Equal(a, b Value) bool       { return DefaultBehavior.Equal(a, b) }
func (intervalFormatBehavior) Format(v Value) string {
	if ii, ok := v.Data.(*IntervalInfo); ok {
		return fmt.Sprintf("Interval(%s,%dms)", ii.ID, ii.Ms)
	}
	return "Interval(nil)"
}
