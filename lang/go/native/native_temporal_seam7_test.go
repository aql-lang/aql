package native

import (
	"strings"
	"testing"
	"time"
)

// Seam-7 coverage for native_temporal.go and native_temporal_timeout.go
// (design/TEST-SEAMS.10.md). All times are fixed literals — no real
// clock is read.

var seam7FixedTime = time.Date(2020, 1, 2, 3, 4, 5, 0, time.UTC)

func TestSeam7TemporalAccessorFallbacks(t *testing.T) {
	// Valid payloads round-trip.
	if got := AsDate(NewDate(seam7FixedTime)); !got.Equal(seam7FixedTime) {
		t.Fatalf("AsDate valid: %v", got)
	}
	if got := AsDateTime(NewDateTime(seam7FixedTime)); !got.Equal(seam7FixedTime) {
		t.Fatalf("AsDateTime valid: %v", got)
	}
	if got := AsInstant(NewInstant(seam7FixedTime)); !got.Equal(seam7FixedTime.UTC()) {
		t.Fatalf("AsInstant valid: %v", got)
	}
	tod := NewValueRaw(TTime, DurationPayload{D: 90 * time.Minute})
	if got := AsTimeOfDay(tod); got != 90*time.Minute {
		t.Fatalf("AsTimeOfDay valid: %v", got)
	}

	// Wrong payloads fall back to the zero value.
	notTemporal := NewInteger(1)
	if !AsDate(notTemporal).IsZero() {
		t.Fatal("AsDate fallback must be zero")
	}
	if !AsDateTime(notTemporal).IsZero() {
		t.Fatal("AsDateTime fallback must be zero")
	}
	if !AsInstant(notTemporal).IsZero() {
		t.Fatal("AsInstant fallback must be zero")
	}
	if AsTimeOfDay(notTemporal) != 0 {
		t.Fatal("AsTimeOfDay fallback must be 0")
	}

	// CalendarDuration / ClockDuration / Timezone accessors.
	cd := NewValueRaw(TTime, CalDurationData{Years: 1, Months: 2, Days: 3})
	if d, ok := AsCalendarDuration(cd); !ok || d.Years != 1 {
		t.Fatalf("AsCalendarDuration valid: %v / %v", d, ok)
	}
	if _, ok := AsCalendarDuration(notTemporal); ok {
		t.Fatal("AsCalendarDuration fallback must be false")
	}
	clk := NewValueRaw(TTime, DurationPayload{D: 5 * time.Second})
	if d, ok := AsClockDuration(clk); !ok || d != 5*time.Second {
		t.Fatalf("AsClockDuration valid: %v / %v", d, ok)
	}
	if _, ok := AsClockDuration(notTemporal); ok {
		t.Fatal("AsClockDuration fallback must be false")
	}
	tz := NewValueRaw(TTime, TimezonePayload{Loc: time.UTC})
	if AsTimezone(tz) == nil {
		t.Fatal("AsTimezone valid must be non-nil")
	}
	if AsTimezone(notTemporal) != nil {
		t.Fatal("AsTimezone fallback must be nil")
	}
}

func TestSeam7TemporalFormatBehaviors(t *testing.T) {
	bad := NewInteger(1)

	// Each Format renders a valid payload and falls back to "T(nil)".
	if got := (dateFormatBehavior{}).Format(NewDate(seam7FixedTime)); got != "2020-01-02" {
		t.Fatalf("date format: %q", got)
	}
	if got := (dateFormatBehavior{}).Format(bad); got != "Date(nil)" {
		t.Fatalf("date nil: %q", got)
	}
	if got := (dateTimeFormatBehavior{}).Format(NewDateTime(seam7FixedTime)); !strings.HasPrefix(got, "2020-01-02T") {
		t.Fatalf("datetime format: %q", got)
	}
	if got := (dateTimeFormatBehavior{}).Format(bad); got != "DateTime(nil)" {
		t.Fatalf("datetime nil: %q", got)
	}
	if got := (instantFormatBehavior{}).Format(NewInstant(seam7FixedTime)); !strings.HasPrefix(got, "2020-01-02T") {
		t.Fatalf("instant format: %q", got)
	}
	if got := (instantFormatBehavior{}).Format(bad); got != "Instant(nil)" {
		t.Fatalf("instant nil: %q", got)
	}

	// TimeOfDay: whole seconds, with-nanoseconds, and the two nil arms.
	todWhole := NewValueRaw(TTime, DurationPayload{D: 3*time.Hour + 30*time.Minute})
	if got := (timeOfDayFormatBehavior{}).Format(todWhole); got != "03:30:00" {
		t.Fatalf("tod whole: %q", got)
	}
	todNS := NewValueRaw(TTime, DurationPayload{D: time.Hour + 500*time.Nanosecond})
	if got := (timeOfDayFormatBehavior{}).Format(todNS); !strings.Contains(got, ".") {
		t.Fatalf("tod ns: %q", got)
	}
	if got := (timeOfDayFormatBehavior{}).Format(bad); got != "TimeOfDay(nil)" {
		t.Fatalf("tod nil (non-duration payload): %q", got)
	}
	todBadInner := NewValueRaw(TTime, DurationPayload{D: "not-a-duration"})
	if got := (timeOfDayFormatBehavior{}).Format(todBadInner); got != "TimeOfDay(nil)" {
		t.Fatalf("tod nil (bad inner): %q", got)
	}

	// CalendarDuration / ClockDuration / Timezone formats + nil arms.
	cd := NewValueRaw(TTime, CalDurationData{Years: 1, Months: 2, Days: 3})
	if got := (calDurationFormatBehavior{}).Format(cd); got != "P1Y2M3D" {
		t.Fatalf("cal format: %q", got)
	}
	if got := (calDurationFormatBehavior{}).Format(bad); got != "CalendarDuration(nil)" {
		t.Fatalf("cal nil: %q", got)
	}
	clk := NewValueRaw(TTime, DurationPayload{D: 90 * time.Second})
	if got := (clkDurationFormatBehavior{}).Format(clk); got == "" {
		t.Fatalf("clk format empty")
	}
	if got := (clkDurationFormatBehavior{}).Format(bad); got != "ClockDuration(nil)" {
		t.Fatalf("clk nil: %q", got)
	}
	tz := NewValueRaw(TTime, TimezonePayload{Loc: time.UTC})
	if got := (timezoneFormatBehavior{}).Format(tz); got != "UTC" {
		t.Fatalf("tz format: %q", got)
	}
	if got := (timezoneFormatBehavior{}).Format(bad); got != "Timezone(nil)" {
		t.Fatalf("tz nil: %q", got)
	}
}

func TestSeam7TemporalBehaviorMethods(t *testing.T) {
	a := NewDate(seam7FixedTime)
	b := NewDate(seam7FixedTime.Add(24 * time.Hour))

	// Match / Equal delegate to the default behavior.
	if !(dateTimeFormatBehavior{}).Match(NewDateTime(seam7FixedTime), TDateTime) {
		t.Fatal("datetime Match")
	}
	if !(dateTimeFormatBehavior{}).Equal(a, a) {
		t.Fatal("datetime Equal identical")
	}
	if !(instantFormatBehavior{}).Match(NewInstant(seam7FixedTime), TInstant) {
		t.Fatal("instant Match")
	}
	if (instantFormatBehavior{}).Equal(a, b) {
		t.Fatal("instant Equal distinct must be false")
	}
	if !(timeOfDayFormatBehavior{}).Match(a, TTime) {
		t.Fatal("tod Match")
	}

	// Per-leaf Comparers order chronologically.
	if c, _ := (dateFormatBehavior{}).Compare(a, b); c != -1 {
		t.Fatalf("date compare a<b: %d", c)
	}
	if c, _ := (dateTimeFormatBehavior{}).Compare(b, a); c != 1 {
		t.Fatalf("datetime compare b>a: %d", c)
	}
	if c, _ := (instantFormatBehavior{}).Compare(a, a); c != 0 {
		t.Fatalf("instant compare equal: %d", c)
	}
	tod1 := NewValueRaw(TTime, DurationPayload{D: time.Hour})
	if c, _ := (timeOfDayFormatBehavior{}).Compare(tod1, tod1); c != 0 {
		t.Fatalf("tod compare equal: %d", c)
	}
	if c, _ := (clkDurationFormatBehavior{}).Compare(tod1, tod1); c != 0 {
		t.Fatalf("clk compare equal: %d", c)
	}

	// timeCompareBehavior: chronological across leaves + ErrNoComparer.
	tc := timeCompareBehavior{}
	if !tc.Match(a, TTime) {
		t.Fatal("timeCompare Match")
	}
	if !tc.Equal(a, a) {
		t.Fatal("timeCompare Equal")
	}
	if tc.Format(a) == "" {
		t.Fatal("timeCompare Format empty")
	}
	if c, err := tc.Compare(b, a); err != nil || c != 1 {
		t.Fatalf("timeCompare after: %d / %v", c, err)
	}
	if c, err := tc.Compare(a, b); err != nil || c != -1 {
		t.Fatalf("timeCompare before: %d / %v", c, err)
	}
	if _, err := tc.Compare(NewInteger(1), a); err == nil {
		t.Fatal("timeCompare non-time must decline with ErrNoComparer")
	}

	// durationFromValue fallbacks (non-payload and bad inner type).
	if durationFromValue(NewInteger(1)) != 0 {
		t.Fatal("durationFromValue non-payload must be 0")
	}
	if durationFromValue(NewValueRaw(TTime, DurationPayload{D: "x"})) != 0 {
		t.Fatal("durationFromValue bad inner must be 0")
	}
}

func TestSeam7RegisterTemporalTypeDuplicate(t *testing.T) {
	saved := typeInitErrs
	t.Cleanup(func() { typeInitErrs = saved })
	typeInitErrs = nil
	// Re-registering an existing FixedID records an init error rather than
	// panicking (ADR-005).
	registerTemporalType("Scalar/Time", 1000, timeCompareBehavior{})
	if err := TypeInitError(); err == nil ||
		!strings.Contains(err.Error(), "register Scalar/Time") {
		t.Fatalf("expected recorded init error, got %v", err)
	}
}

func TestSeam7RunTimerCallback(t *testing.T) {
	r := seam5Reg(t)

	// List callback, concrete: runs the elements (side effects discarded).
	RunTimerCallback(r, NewList([]Value{NewInteger(1), NewWord("dup")}), true)

	// List callback, non-concrete: early return, no panic.
	RunTimerCallback(r, NewTypeLiteral(TList), true)

	// Atom callback: resolves to a word name.
	RunTimerCallback(r, NewAtom("dup"), false)

	// String callback: also yields a word name (the else branch).
	RunTimerCallback(r, NewString("dup"), false)
}
