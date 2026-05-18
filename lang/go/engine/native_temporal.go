package engine

import (
	"fmt"
	"time"

	"github.com/aql-lang/aql/eng/go"
)

// The Scalar/Time type family is owned by the lang/go/engine package
// post Step 8. The kernel (eng) no longer declares these types in
// its builtinDecls or carries their constructors / format Behaviors.
// They live in lang/go/engine because date-arithmetic handlers in
// native_math.go reference them at package-init time — colocating
// the declarations with the handlers avoids the cross-package
// import cycle that blocked an earlier nativemod-based migration.
//
// FixedIDs 1000-1008 come from the documented
// lang/go/internal/nativemod/time range (1000-1999) reused here; the
// types could later move to nativemod if the date-arithmetic
// handlers also move (a separate restructure).
//
// Registration uses var-initialiser form so any other var that
// references these types — notably mathNatives' date-arithmetic
// signatures — sees a non-nil pointer at slice-init time. Go's
// init order resolves dependencies before declaration order.
//
// Parents (Scalar/Time and Scalar/Time/Duration) register first so
// children's parent paths resolve.
var (
	TTime        = registerTemporalType("Scalar/Time", 1000, nil)
	TDate        = registerTemporalType("Scalar/Time/Date", 1001, dateFormatBehavior{})
	TDateTime    = registerTemporalType("Scalar/Time/DateTime", 1002, dateTimeFormatBehavior{})
	TInstant     = registerTemporalType("Scalar/Time/Instant", 1003, instantFormatBehavior{})
	TTimeOfDay   = registerTemporalType("Scalar/Time/TimeOfDay", 1004, timeOfDayFormatBehavior{})
	TDuration    = registerTemporalType("Scalar/Time/Duration", 1005, nil)
	TCalDuration = registerTemporalType("Scalar/Time/Duration/CalDuration", 1006, calDurationFormatBehavior{})
	TClkDuration = registerTemporalType("Scalar/Time/Duration/ClkDuration", 1007, clkDurationFormatBehavior{})
	TTimezone    = registerTemporalType("Scalar/Time/Timezone", 1008, timezoneFormatBehavior{})
)

func registerTemporalType(path string, fixedID int, behavior eng.TypeBehavior) *eng.Type {
	t, err := eng.Builtin.RegisterExternalBuiltin(path, fixedID, behavior)
	if err != nil {
		// lint:allow-panic — init-time builtin registration; see
		// registerTimerType in native_misc.go for rationale.
		panic(fmt.Sprintf("native_temporal: register %s: %v", path, err))
	}
	return t
}

// New* constructors for Scalar/Time/* — moved from eng/value.go at
// Step 8 since the kernel no longer carries constructors for types
// it doesn't own.

func NewDate(t time.Time) Value {
	return eng.NewValueRaw(TDate, eng.TimePayload{T: t})
}

func NewDateTime(t time.Time) Value {
	return eng.NewValueRaw(TDateTime, eng.TimePayload{T: t})
}

func NewInstant(t time.Time) Value {
	return eng.NewValueRaw(TInstant, eng.TimePayload{T: t.UTC()})
}

func NewTimeOfDay(d time.Duration) Value {
	return eng.NewValueRaw(TTimeOfDay, eng.DurationPayload{D: d})
}

func NewCalDuration(years, months, days int) Value {
	return eng.NewValueRaw(TCalDuration, eng.CalDurationData{Years: years, Months: months, Days: days})
}

func NewClkDuration(d time.Duration) Value {
	return eng.NewValueRaw(TClkDuration, eng.DurationPayload{D: d})
}

func NewTimezone(loc *time.Location) Value {
	return eng.NewValueRaw(TTimezone, eng.TimezonePayload{Loc: loc})
}

// As* accessors for the time-family types. Moved from
// eng/value.go at Step 6/7 — the kernel no longer carries methods
// for types it doesn't own. The implementations are identical to
// the previous methods: assert the kernel-owned payload variant
// (TimePayload / DurationPayload / TimezonePayload / CalDurationData)
// and return the inner Go value, or the zero value on mismatch.

// AsDate extracts the time.Time from a Date value.
func AsDate(v Value) time.Time {
	if tp, ok := v.Data.(eng.TimePayload); ok {
		if t, ok := tp.T.(time.Time); ok {
			return t
		}
	}
	return time.Time{}
}

// AsDateTime extracts the time.Time from a DateTime value.
func AsDateTime(v Value) time.Time {
	if tp, ok := v.Data.(eng.TimePayload); ok {
		if t, ok := tp.T.(time.Time); ok {
			return t
		}
	}
	return time.Time{}
}

// AsInstant extracts the time.Time from an Instant value.
func AsInstant(v Value) time.Time {
	if tp, ok := v.Data.(eng.TimePayload); ok {
		if t, ok := tp.T.(time.Time); ok {
			return t
		}
	}
	return time.Time{}
}

// AsTimeOfDay extracts the time.Duration offset for a TimeOfDay value.
func AsTimeOfDay(v Value) time.Duration {
	if dp, ok := v.Data.(eng.DurationPayload); ok {
		if d, ok := dp.D.(time.Duration); ok {
			return d
		}
	}
	return 0
}

// AsCalDuration extracts the CalDurationData payload.
func AsCalDuration(v Value) (eng.CalDurationData, bool) {
	if d, ok := v.Data.(eng.CalDurationData); ok {
		return d, true
	}
	return eng.CalDurationData{}, false
}

// AsClkDuration extracts the time.Duration payload for a ClkDuration value.
func AsClkDuration(v Value) (time.Duration, bool) {
	if dp, ok := v.Data.(eng.DurationPayload); ok {
		if d, ok := dp.D.(time.Duration); ok {
			return d, true
		}
	}
	return 0, false
}

// AsTimezone extracts the *time.Location for a Timezone value.
func AsTimezone(v Value) *time.Location {
	if tp, ok := v.Data.(eng.TimezonePayload); ok {
		if loc, ok := tp.Loc.(*time.Location); ok {
			return loc
		}
	}
	return nil
}

// Format Behaviors for the time-family types. Moved from
// eng/coretype_format_behaviors.go at Step 8.

type dateFormatBehavior struct{}

func (dateFormatBehavior) Match(v Value, t *Type) bool { return eng.DefaultBehavior.Match(v, t) }
func (dateFormatBehavior) Equal(a, b Value) bool       { return eng.DefaultBehavior.Equal(a, b) }
func (dateFormatBehavior) Format(v Value) string {
	if tp, ok := v.Data.(eng.TimePayload); ok {
		if t, ok := tp.T.(time.Time); ok {
			return t.Format("2006-01-02")
		}
	}
	return "Date(nil)"
}

// Compare orders Date values chronologically (earlier < later).
// Implements eng.Comparer so `lt`/`gt`/`sort` work on Dates via the
// canonical CompareValues lattice dispatch.
func (dateFormatBehavior) Compare(a, b Value) (int, error) {
	return compareTimePayloads(a, b), nil
}

type dateTimeFormatBehavior struct{}

func (dateTimeFormatBehavior) Match(v Value, t *Type) bool { return eng.DefaultBehavior.Match(v, t) }
func (dateTimeFormatBehavior) Equal(a, b Value) bool       { return eng.DefaultBehavior.Equal(a, b) }
func (dateTimeFormatBehavior) Format(v Value) string {
	if tp, ok := v.Data.(eng.TimePayload); ok {
		if t, ok := tp.T.(time.Time); ok {
			return t.Format("2006-01-02T15:04:05.999999999")
		}
	}
	return "DateTime(nil)"
}

func (dateTimeFormatBehavior) Compare(a, b Value) (int, error) {
	return compareTimePayloads(a, b), nil
}

type instantFormatBehavior struct{}

func (instantFormatBehavior) Match(v Value, t *Type) bool { return eng.DefaultBehavior.Match(v, t) }
func (instantFormatBehavior) Equal(a, b Value) bool       { return eng.DefaultBehavior.Equal(a, b) }
func (instantFormatBehavior) Format(v Value) string {
	if tp, ok := v.Data.(eng.TimePayload); ok {
		if t, ok := tp.T.(time.Time); ok {
			return t.Format(time.RFC3339Nano)
		}
	}
	return "Instant(nil)"
}

func (instantFormatBehavior) Compare(a, b Value) (int, error) {
	return compareTimePayloads(a, b), nil
}

type timeOfDayFormatBehavior struct{}

func (timeOfDayFormatBehavior) Match(v Value, t *Type) bool { return eng.DefaultBehavior.Match(v, t) }
func (timeOfDayFormatBehavior) Equal(a, b Value) bool       { return eng.DefaultBehavior.Equal(a, b) }
func (timeOfDayFormatBehavior) Format(v Value) string {
	dp, ok := v.Data.(eng.DurationPayload)
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

func (timeOfDayFormatBehavior) Compare(a, b Value) (int, error) {
	return compareDurationPayloads(a, b), nil
}

// compareTimePayloads returns -1/0/1 for two values whose Data is an
// eng.TimePayload wrapping a time.Time. Non-Time payloads compare as
// equal — the matching dispatch already filters by VType, so this
// only fires on well-formed temporal values.
func compareTimePayloads(a, b Value) int {
	ta, _ := timeFromValue(a)
	tb, _ := timeFromValue(b)
	switch {
	case ta.Before(tb):
		return -1
	case ta.After(tb):
		return 1
	default:
		return 0
	}
}

func compareDurationPayloads(a, b Value) int {
	da := durationFromValue(a)
	db := durationFromValue(b)
	switch {
	case da < db:
		return -1
	case da > db:
		return 1
	default:
		return 0
	}
}

func timeFromValue(v Value) (time.Time, bool) {
	tp, ok := v.Data.(eng.TimePayload)
	if !ok {
		return time.Time{}, false
	}
	t, ok := tp.T.(time.Time)
	return t, ok
}

func durationFromValue(v Value) time.Duration {
	dp, ok := v.Data.(eng.DurationPayload)
	if !ok {
		return 0
	}
	d, ok := dp.D.(time.Duration)
	if !ok {
		return 0
	}
	return d
}

type calDurationFormatBehavior struct{}

func (calDurationFormatBehavior) Match(v Value, t *Type) bool {
	return eng.DefaultBehavior.Match(v, t)
}
func (calDurationFormatBehavior) Equal(a, b Value) bool { return eng.DefaultBehavior.Equal(a, b) }
func (calDurationFormatBehavior) Format(v Value) string {
	if cd, ok := v.Data.(eng.CalDurationData); ok {
		return fmt.Sprintf("P%dY%dM%dD", cd.Years, cd.Months, cd.Days)
	}
	return "CalDuration(nil)"
}

type clkDurationFormatBehavior struct{}

func (clkDurationFormatBehavior) Match(v Value, t *Type) bool {
	return eng.DefaultBehavior.Match(v, t)
}
func (clkDurationFormatBehavior) Equal(a, b Value) bool { return eng.DefaultBehavior.Equal(a, b) }
func (clkDurationFormatBehavior) Format(v Value) string {
	if dp, ok := v.Data.(eng.DurationPayload); ok {
		if d, ok := dp.D.(time.Duration); ok {
			return d.String()
		}
	}
	return "ClkDuration(nil)"
}

func (clkDurationFormatBehavior) Compare(a, b Value) (int, error) {
	return compareDurationPayloads(a, b), nil
}

type timezoneFormatBehavior struct{}

func (timezoneFormatBehavior) Match(v Value, t *Type) bool { return eng.DefaultBehavior.Match(v, t) }
func (timezoneFormatBehavior) Equal(a, b Value) bool       { return eng.DefaultBehavior.Equal(a, b) }
func (timezoneFormatBehavior) Format(v Value) string {
	if tp, ok := v.Data.(eng.TimezonePayload); ok {
		if loc, ok := tp.Loc.(*time.Location); ok {
			return loc.String()
		}
	}
	return "Timezone(nil)"
}
