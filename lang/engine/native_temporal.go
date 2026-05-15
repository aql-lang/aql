package engine

import (
	"fmt"
	"time"

	"github.com/aql-lang/aql/eng"
)

// The Scalar/Time type family is owned by the lang/engine package
// post Step 8. The kernel (eng) no longer declares these types in
// its builtinDecls or carries their constructors / format Behaviors.
// They live in lang/engine because date-arithmetic handlers in
// native_math.go reference them at package-init time — colocating
// the declarations with the handlers avoids the cross-package
// import cycle that blocked an earlier nativemod-based migration.
//
// FixedIDs 1000-1008 come from the documented
// lang/internal/nativemod/time range (1000-1999) reused here; the
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
