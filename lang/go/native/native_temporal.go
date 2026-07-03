package native

import (
	"fmt"
	"time"

	"github.com/aql-lang/aql/eng/go"
)

// The Scalar/Time type family splits across two owners:
//
//   - CORE (global external builtins, wire-stable FixedIDs 1000-1003):
//     the family root Scalar/Time and the three instant-bearing
//     leaves Date / DateTime / Instant. These stay global because
//     cross-module producers construct them (IO mtime is an Instant),
//     they order chronologically through the Scalar/Time Comparer,
//     and their identities are baked into serialised Value IDs.
//
//   - aql:time-util (per-import module mints, no FixedIDs): TimeOfDay,
//     Duration and its CalendarDuration / ClockDuration leaves, and Timezone.
//     BuildTimeModule mints them per import via
//     MintTemporalModuleTypes below — the StreamKind pattern
//     (io_stream.go) — so a program that never imports the module
//     doesn't see the types at all. They are reachable as
//     `TimeUtil.CalendarDuration` etc. The temporal add/sub overloads ride
//     the module's word-extension exports (TemporalArithmeticExtensions,
//     native_math.go), and the minted types are what satisfy the
//     module-scope user-type rule for those extensions.
//
// Registration uses var-initialiser form so any other var that
// references the core types — notably signatures built at
// package-init time — sees a non-nil pointer at slice-init time.
// Go's init order resolves dependencies before declaration order.
var (
	TTime     = registerTemporalType("Scalar/Time", 1000, timeCompareBehavior{})
	TDate     = registerTemporalType("Scalar/Time/Date", 1001, dateFormatBehavior{})
	TDateTime = registerTemporalType("Scalar/Time/DateTime", 1002, dateTimeFormatBehavior{})
	TInstant  = registerTemporalType("Scalar/Time/Instant", 1003, instantFormatBehavior{})
)

// TemporalModuleTypes are the Scalar/Time leaves owned by
// aql:time-util, minted per import into the module's sub-registry by
// MintTemporalModuleTypes. Their values escape to the importer (a
// `TimeUtil.days 3` result is a CalendarDuration), so the mints draw IDs
// from the importing tree's counter — BuildTimeModule adopts the
// parent's sequence before minting (see eng TypeTable.mintID and the
// BuildIOModule / StreamKind precedent).
type TemporalModuleTypes struct {
	TimeOfDay        *Type
	Duration         *Type
	CalendarDuration *Type
	ClockDuration    *Type
	Timezone         *Type
	// The timer handle types (former global FixedIDs 4000-4001) —
	// their words (timeout / interval / await / cancel) are module
	// words already; the handles they return carry these mints.
	Timeout  *Type
	Interval *Type
}

// MintTemporalModuleTypes mints the module-owned temporal types into
// r's type table (r is aql:time-util's sub-registry) and returns the
// nodes. Parents mint first so the children's Parent chains resolve;
// the whole set hangs under the global Scalar/Time root, so the
// family Comparer and lattice ordering see them exactly where the
// former global registrations sat.
func MintTemporalModuleTypes(r *Registry) TemporalModuleTypes {
	timeOfDay := r.Types.MintTypeWithBehavior("TimeOfDay", TTime, timeOfDayFormatBehavior{})
	duration := r.Types.MintType("Duration", TTime)
	return TemporalModuleTypes{
		TimeOfDay:        timeOfDay,
		Duration:         duration,
		CalendarDuration: r.Types.MintTypeWithBehavior("CalendarDuration", duration, calDurationFormatBehavior{}),
		ClockDuration:    r.Types.MintTypeWithBehavior("ClockDuration", duration, clkDurationFormatBehavior{}),
		Timezone:         r.Types.MintTypeWithBehavior("Timezone", TTime, timezoneFormatBehavior{}),
		Timeout:          r.Types.MintTypeWithBehavior("Timeout", eng.TIdeal, timeoutFormatBehavior{}),
		Interval:         r.Types.MintTypeWithBehavior("Interval", eng.TIdeal, intervalFormatBehavior{}),
	}
}

func registerTemporalType(path string, fixedID int, behavior eng.TypeBehavior) *eng.Type {
	t, err := eng.Builtin.RegisterExternalBuiltin(path, fixedID, behavior)
	if err != nil {
		// Init-time registration error — recorded, not panicked.
		// See ADR-005 and typeinit.go.
		recordTypeInitErr(fmt.Errorf("native_temporal: register %s: %w", path, err))
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

// The module-owned constructors are methods on TemporalModuleTypes:
// the Parent tag comes from that import's mints, so every value a
// module instance produces carries the instance's own type identity.

func (tt TemporalModuleTypes) NewTimeOfDay(d time.Duration) Value {
	return eng.NewValueRaw(tt.TimeOfDay, eng.DurationPayload{D: d})
}

func (tt TemporalModuleTypes) NewCalendarDuration(years, months, days int) Value {
	return eng.NewValueRaw(tt.CalendarDuration, eng.CalDurationData{Years: years, Months: months, Days: days})
}

func (tt TemporalModuleTypes) NewClockDuration(d time.Duration) Value {
	return eng.NewValueRaw(tt.ClockDuration, eng.DurationPayload{D: d})
}

func (tt TemporalModuleTypes) NewTimezone(loc *time.Location) Value {
	return eng.NewValueRaw(tt.Timezone, eng.TimezonePayload{Loc: loc})
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

// AsCalendarDuration extracts the CalDurationData payload.
func AsCalendarDuration(v Value) (eng.CalDurationData, bool) {
	if d, ok := v.Data.(eng.CalDurationData); ok {
		return d, true
	}
	return eng.CalDurationData{}, false
}

// AsClockDuration extracts the time.Duration payload for a ClockDuration value.
func AsClockDuration(v Value) (time.Duration, bool) {
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
// equal — the matching dispatch already filters by Parent, so this
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

// timeCompareBehavior is the Comparer installed on the abstract
// Scalar/Time node. It is reached by the CompareValues LCA walk only
// for CROSS-leaf Time pairs (same-leaf pairs hit their own per-leaf
// Comparer first). It unifies the instant-bearing leaves — Date,
// DateTime, Instant — so they order chronologically against one another
// (a date and a datetime are both moments in time). Pairs it can't
// place chronologically (TimeOfDay and the Duration leaves, type
// literals — none of which carry a time.Time) decline with
// ErrNoComparer, so the cascade falls through to the lattice Rank, and
// the family-restricted ordering words reject them in favour of tcmp.
type timeCompareBehavior struct{}

func (timeCompareBehavior) Match(v Value, t *Type) bool { return eng.DefaultBehavior.Match(v, t) }
func (timeCompareBehavior) Equal(a, b Value) bool       { return eng.DefaultBehavior.Equal(a, b) }
func (timeCompareBehavior) Format(v Value) string       { return eng.DefaultBehavior.Format(v) }

func (timeCompareBehavior) Compare(a, b Value) (int, error) {
	ta, aok := timeFromValue(a)
	tb, bok := timeFromValue(b)
	if !aok || !bok {
		return 0, eng.ErrNoComparer
	}
	switch {
	case ta.Before(tb):
		return -1, nil
	case ta.After(tb):
		return 1, nil
	default:
		return 0, nil
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
	return "CalendarDuration(nil)"
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
	return "ClockDuration(nil)"
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
