package modules

import (
	"fmt"
	"time"

	"github.com/aql-lang/aql/lang/go/native"
)

// BuildTimeModule creates the "aql:time" native module.
func BuildTimeModule(parent *native.Registry) (native.ModuleDesc, error) {
	subReg, err := native.DefaultRegistry()
	if err != nil {
		return native.ModuleDesc{}, err
	}

	for _, n := range TimeNatives {
		subReg.RegisterNativeFunc(n)
	}

	exports := native.NewOrderedMap()

	// Construction — numeric and IANA-zone only. ISO 8601 date /
	// datetime / instant / time-of-day / duration string parsing
	// was removed as a feature; construct dates from `now-local`,
	// `today`, `today-utc`, `unix`, `unix-ms`, `unix-ns`, the
	// numeric duration constructors (`years`, `months`, `days`,
	// `hours`, `minutes`, `seconds`, …), or `cal-dur` directly.
	exports.Set("tz", makeTimeFnDef("time-tz", []native.FnParam{{Type: native.TString}}, []*native.Type{native.TTimezone}, subReg))
	exports.Set("unix", makeTimeFnDef("time-unix", []native.FnParam{{Type: native.TInteger}}, []*native.Type{native.TInstant}, subReg))
	exports.Set("unix-ms", makeTimeFnDef("time-unix-ms", []native.FnParam{{Type: native.TInteger}}, []*native.Type{native.TInstant}, subReg))
	exports.Set("unix-ns", makeTimeFnDef("time-unix-ns", []native.FnParam{{Type: native.TInteger}}, []*native.Type{native.TInstant}, subReg))

	// Current time
	exports.Set("now-local", makeTimeFnDef("time-now-local", []native.FnParam{}, []*native.Type{native.TDateTime}, subReg))
	exports.Set("today", makeTimeFnDef("time-today", []native.FnParam{}, []*native.Type{native.TDate}, subReg))
	exports.Set("today-utc", makeTimeFnDef("time-today-utc", []native.FnParam{}, []*native.Type{native.TDate}, subReg))

	// Extraction (Date -> Integer)
	for _, name := range []string{"year", "month", "day", "weekday", "year-day", "iso-week", "quarter", "days-in-month", "days-in-year"} {
		exports.Set(name, makeTimeFnDef(name, []native.FnParam{{Type: native.TDate}}, []*native.Type{native.TInteger}, subReg))
	}
	for _, name := range []string{"weekday-name", "month-name"} {
		exports.Set(name, makeTimeFnDef(name, []native.FnParam{{Type: native.TDate}}, []*native.Type{native.TString}, subReg))
	}
	exports.Set("is-leap-year", makeTimeFnDef("is-leap-year", []native.FnParam{{Type: native.TDate}}, []*native.Type{native.TBoolean}, subReg))

	// Extraction from Instant
	exports.Set("to-unix", makeTimeFnDef("to-unix", []native.FnParam{{Type: native.TInstant}}, []*native.Type{native.TInteger}, subReg))
	exports.Set("to-unix-ms", makeTimeFnDef("to-unix-ms", []native.FnParam{{Type: native.TInstant}}, []*native.Type{native.TInteger}, subReg))

	// Comparison (Date Date -> Boolean)
	for _, name := range []string{"is-before", "is-after", "is-equal"} {
		exports.Set(name, makeTimeFnDef(name, []native.FnParam{{Type: native.TDate}, {Type: native.TDate}}, []*native.Type{native.TBoolean}, subReg))
	}

	// Formatting
	exports.Set("to-string", makeTimeFnDef("to-string", []native.FnParam{{Type: native.TDate}}, []*native.Type{native.TString}, subReg))
	// Params match the inner native's Args (sig order: top-first).
	// stack `date layout`: Params[0]=layout (top, String), Params[1]=date (deeper, Date).
	exports.Set("format", makeTimeFnDef("format", []native.FnParam{{Type: native.TString}, {Type: native.TDate}}, []*native.Type{native.TString}, subReg))
	exports.Set("to-iso", makeTimeFnDef("to-iso", []native.FnParam{{Type: native.TDate}}, []*native.Type{native.TString}, subReg))

	// Duration construction
	for _, name := range []string{"years", "months", "weeks", "days"} {
		exports.Set(name, makeTimeFnDef(name, []native.FnParam{{Type: native.TInteger}}, []*native.Type{native.TCalDuration}, subReg))
	}
	for _, name := range []string{"hours", "minutes", "seconds", "ms", "us", "ns"} {
		exports.Set(name, makeTimeFnDef(name, []native.FnParam{{Type: native.TNumber}}, []*native.Type{native.TClkDuration}, subReg))
	}
	exports.Set("cal-dur", makeTimeFnDef("cal-dur", []native.FnParam{{Type: native.TInteger}, {Type: native.TInteger}, {Type: native.TInteger}}, []*native.Type{native.TCalDuration}, subReg))
	// ISO 8601 duration string parsing (`"P1Y2M3D" duration`)
	// removed as a feature. Use cal-dur or the years/months/days
	// constructors directly.

	// Duration extraction
	for _, name := range []string{"total-hours", "total-minutes", "total-seconds", "total-ms"} {
		exports.Set(name, makeTimeFnDef(name, []native.FnParam{{Type: native.TClkDuration}}, []*native.Type{native.TDecimal}, subReg))
	}
	for _, name := range []string{"dur-years", "dur-months", "dur-days"} {
		exports.Set(name, makeTimeFnDef(name, []native.FnParam{{Type: native.TCalDuration}}, []*native.Type{native.TInteger}, subReg))
	}
	exports.Set("dur-sign", makeTimeFnDef("dur-sign", []native.FnParam{{Type: native.TCalDuration}}, []*native.Type{native.TInteger}, subReg))

	// Arithmetic
	exports.Set("until", makeTimeFnDef("until", []native.FnParam{{Type: native.TDate}, {Type: native.TDate}}, []*native.Type{native.TCalDuration}, subReg))
	exports.Set("since", makeTimeFnDef("since", []native.FnParam{{Type: native.TDate}, {Type: native.TDate}}, []*native.Type{native.TCalDuration}, subReg))
	exports.Set("diff", makeTimeFnDef("diff", []native.FnParam{{Type: native.TInstant}, {Type: native.TInstant}}, []*native.Type{native.TClkDuration}, subReg))
	exports.Set("elapsed", makeTimeFnDef("elapsed", []native.FnParam{{Type: native.TInstant}}, []*native.Type{native.TClkDuration}, subReg))

	// Comparison extended
	exports.Set("compare", makeTimeFnDef("time-compare", []native.FnParam{{Type: native.TDate}, {Type: native.TDate}}, []*native.Type{native.TInteger}, subReg))
	exports.Set("is-between", makeTimeFnDef("is-between", []native.FnParam{{Type: native.TDate}, {Type: native.TDate}, {Type: native.TDate}}, []*native.Type{native.TBoolean}, subReg))
	exports.Set("earliest", makeTimeFnDef("earliest", []native.FnParam{{Type: native.TDate}, {Type: native.TDate}}, []*native.Type{native.TDate}, subReg))
	exports.Set("latest", makeTimeFnDef("latest", []native.FnParam{{Type: native.TDate}, {Type: native.TDate}}, []*native.Type{native.TDate}, subReg))

	// Conversion
	exports.Set("to-date", makeTimeFnDef("to-date", []native.FnParam{{Type: native.TDateTime}}, []*native.Type{native.TDate}, subReg))
	exports.Set("to-time-of-day", makeTimeFnDef("to-time-of-day", []native.FnParam{{Type: native.TDateTime}}, []*native.Type{native.TTimeOfDay}, subReg))
	exports.Set("to-datetime", makeTimeFnDef("to-datetime", []native.FnParam{{Type: native.TDate}}, []*native.Type{native.TDateTime}, subReg))
	// Wrapper Params match the inner native Args (sig order, top-first).
	exports.Set("to-instant", makeTimeFnDef("to-instant", []native.FnParam{{Type: native.TTimezone}, {Type: native.TDateTime}}, []*native.Type{native.TInstant}, subReg))
	exports.Set("to-local", makeTimeFnDef("to-local", []native.FnParam{{Type: native.TTimezone}, {Type: native.TInstant}}, []*native.Type{native.TDateTime}, subReg))
	exports.Set("to-utc", makeTimeFnDef("to-utc", []native.FnParam{{Type: native.TInstant}}, []*native.Type{native.TDateTime}, subReg))

	// Rounding
	exports.Set("start-of", makeTimeFnDef("start-of", []native.FnParam{{Type: native.TString}, {Type: native.TDate}}, []*native.Type{native.TDate}, subReg))
	exports.Set("end-of", makeTimeFnDef("end-of", []native.FnParam{{Type: native.TString}, {Type: native.TDate}}, []*native.Type{native.TDate}, subReg))

	// Timezone
	exports.Set("tz-utc", makeTimeFnDef("tz-utc", []native.FnParam{}, []*native.Type{native.TTimezone}, subReg))
	exports.Set("tz-local", makeTimeFnDef("tz-local", []native.FnParam{}, []*native.Type{native.TTimezone}, subReg))
	exports.Set("tz-name", makeTimeFnDef("tz-name", []native.FnParam{{Type: native.TTimezone}}, []*native.Type{native.TString}, subReg))
	exports.Set("tz-offset", makeTimeFnDef("tz-offset", []native.FnParam{{Type: native.TTimezone}, {Type: native.TInstant}}, []*native.Type{native.TString}, subReg))
	exports.Set("is-dst", makeTimeFnDef("is-dst", []native.FnParam{{Type: native.TTimezone}, {Type: native.TInstant}}, []*native.Type{native.TBoolean}, subReg))

	// Parsing — removed as a feature. parse-date / parse-datetime
	// (layout-based) and auto-date (auto-format-detecting) are gone.

	// Wrapper Params match inner native Args (sig order, top-first):
	// stack `date n add-days` → top=n (Integer)=sig[0], deeper=date (Date)=sig[1].
	for _, name := range []string{"add-days", "add-months", "add-years"} {
		exports.Set(name, makeTimeFnDef(name, []native.FnParam{{Type: native.TInteger}, {Type: native.TDate}}, []*native.Type{native.TDate}, subReg))
	}

	modID := parent.Modules.NextID()
	desc := native.ModuleDesc{
		ID:      modID,
		Exports: map[string]*native.OrderedMap{"time": exports},
	}
	return desc, nil
}

// makeTimeFnDef creates a FnDef value with the given params, returns, and word name.
func makeTimeFnDef(wordName string, params []native.FnParam, returns []*native.Type, subReg *native.Registry) native.Value {
	fnDef := native.FnDefInfo{
		Name: wordName,
		Sigs: []native.FnSig{
			{
				Params:  params,
				Returns: returns,
				Body:    []native.Value{native.NewWord(wordName)}, BarrierPos: -1,
			},
		},
		Registry: subReg,
	}
	return native.NewFnDef(fnDef)
}

// --- helpers ---

// extractTime returns the time.Time from a Date, DateTime, or Instant value.
func extractTime(v native.Value) time.Time {
	if tp, ok := v.Data.(native.TimePayload); ok {
		if t, ok := tp.T.(time.Time); ok {
			return t
		}
	}
	return time.Time{}
}

// dateDiffCalDuration computes the CalDuration between two dates (from → to).
func dateDiffCalDuration(from, to time.Time) native.CalDurationData {
	years := to.Year() - from.Year()
	months := int(to.Month()) - int(from.Month())
	days := to.Day() - from.Day()
	if days < 0 {
		months--
		// Days in the previous month of 'to'
		prev := time.Date(to.Year(), to.Month(), 0, 0, 0, 0, 0, time.UTC)
		days += prev.Day()
	}
	if months < 0 {
		years--
		months += 12
	}
	return native.CalDurationData{Years: years, Months: months, Days: days}
}

// ISO 8601 duration parsing (parseISO8601Duration) and auto-date
// layout tables (autoDateLayouts) removed at the parser-hand-off
// step. AQL no longer parses dates / times / durations from
// strings; construct via numeric helpers (cal-dur, years, months,
// days, hours, …) or via current-time sources (today, now-local,
// unix, unix-ms, unix-ns).

// --- builders for repeating shapes ---

// dateToIntNative builds a NativeFunc with a single [TDate] -> [TInteger]
// signature whose handler applies fn to the extracted time.Time.
func dateToIntNative(name string, fn func(time.Time) int64) native.NativeFunc {
	return native.NativeFunc{
		Name: name,

		Signatures: []native.NativeSig{{
			Args: []*native.Type{native.TDate},
			Handler: func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
				return []native.Value{native.NewInteger(fn(extractTime(args[0])))}, nil
			},
			Returns: []*native.Type{native.TInteger}, BarrierPos: -1,
		}},
	}
}

// dateToStringNative builds a NativeFunc with a single [TDate] -> [TString]
// signature whose handler applies fn to the extracted time.Time.
func dateToStringNative(name string, fn func(time.Time) string) native.NativeFunc {
	return native.NativeFunc{
		Name: name,

		Signatures: []native.NativeSig{{
			Args: []*native.Type{native.TDate},
			Handler: func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
				return []native.Value{native.NewString(fn(extractTime(args[0])))}, nil
			},
			Returns: []*native.Type{native.TString}, BarrierPos: -1,
		}},
	}
}

// intToCalDurationNative builds a NativeFunc with [TInteger] -> [TCalDuration]
// where the handler turns the integer into a CalDurationData via fn.
func intToCalDurationNative(name string, returnType *native.Type, fn func(int) (int, int, int)) native.NativeFunc {
	return native.NativeFunc{
		Name: name,

		Signatures: []native.NativeSig{{
			Args: []*native.Type{native.TInteger},
			Handler: func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
				n, err := args[0].AsConcreteInteger()
				if err != nil {
					return nil, err
				}
				y, m, d := fn(int(n))
				return []native.Value{native.NewCalDuration(y, m, d)}, nil
			},
			Returns: []*native.Type{returnType}, BarrierPos: -1,
		}},
	}
}

// numToClkDurationNative builds a NativeFunc with [TNumber] -> [TClkDuration]
// where the handler scales the numeric input by `unit`.
func numToClkDurationNative(name string, unit time.Duration) native.NativeFunc {
	return native.NativeFunc{
		Name: name,

		Signatures: []native.NativeSig{{
			Args: []*native.Type{native.TNumber},
			Handler: func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
				n, err := native.AsNumber(args[0])
				if err != nil {
					return nil, err
				}
				return []native.Value{native.NewClkDuration(time.Duration(n * float64(unit)))}, nil
			},
			Returns: []*native.Type{native.TClkDuration}, BarrierPos: -1,
		}},
	}
}

// clkDurationToDecimalNative builds a NativeFunc with
// [TClkDuration] -> [TDecimal] for a total-* extraction. `returnType`
// is exposed because total-ms historically declared TInteger even
// though the value pushed is Decimal.
func clkDurationToDecimalNative(name string, returnType *native.Type, fn func(time.Duration) float64) native.NativeFunc {
	return native.NativeFunc{
		Name: name,

		Signatures: []native.NativeSig{{
			Args: []*native.Type{native.TClkDuration},
			Handler: func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
				d, _ := native.AsClkDuration(args[0])
				return []native.Value{native.NewDecimal(fn(d))}, nil
			},
			Returns: []*native.Type{returnType}, BarrierPos: -1,
		}},
	}
}

// calDurationToIntNative builds a NativeFunc with
// [TCalDuration] -> [TInteger] for dur-years/dur-months/dur-days.
func calDurationToIntNative(name string, fn func(native.CalDurationData) int64) native.NativeFunc {
	return native.NativeFunc{
		Name: name,

		Signatures: []native.NativeSig{{
			Args: []*native.Type{native.TCalDuration},
			Handler: func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
				cd, _ := native.AsCalDuration(args[0])
				return []native.Value{native.NewInteger(fn(cd))}, nil
			},
			Returns: []*native.Type{native.TInteger}, BarrierPos: -1,
		}},
	}
}

// addDateNative builds the add-days/add-months/add-years words. The
// fn signature mirrors time.Time.AddDate's parameters so each word
// just supplies (years, months, days) shifts driven by the integer arg.
func addDateNative(name string, build func(n int) (years, months, days int)) native.NativeFunc {
	return native.NativeFunc{
		Name: name,

		Signatures: []native.NativeSig{{
			Args: []*native.Type{native.TInteger, native.TDate},
			Handler: func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
				n, err := args[0].AsConcreteInteger()
				if err != nil {
					return nil, err
				}
				t := extractTime(args[1])
				y, m, d := build(int(n))
				return []native.Value{native.NewDate(t.AddDate(y, m, d))}, nil
			},
			Returns: []*native.Type{native.TAny}, BarrierPos: -1,
		}},
	}
}

// --- Word definitions ---

// TimeNatives is the consolidated NativeFunc slice for the time
// module's Go-implemented words. Replaces the per-word
// register* functions and the master registerAllTimeWords aggregator.
var TimeNatives = func() []native.NativeFunc {
	out := []native.NativeFunc{
		// --- Construction ---
		// ISO 8601 string parsers (time-date, time-datetime,
		// time-instant, time-time-of-day) were removed as a feature.
		// Construct via numeric helpers below or via the
		// extraction/conversion words.
		{
			Name: "time-tz",

			Signatures: []native.NativeSig{{
				Args: []*native.Type{native.TString},
				Handler: func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
					s, err := args[0].AsConcreteString()
					if err != nil {
						return nil, err
					}
					loc, err := time.LoadLocation(s)
					if err != nil {
						return nil, fmt.Errorf("tz: unknown timezone: %q", s)
					}
					return []native.Value{native.NewTimezone(loc)}, nil
				},
				Returns: []*native.Type{native.TTimezone}, BarrierPos: -1,
			}},
		},
		{
			Name: "time-unix",

			Signatures: []native.NativeSig{{
				Args: []*native.Type{native.TInteger},
				Handler: func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
					n, err := args[0].AsConcreteInteger()
					if err != nil {
						return nil, err
					}
					return []native.Value{native.NewInstant(time.Unix(n, 0))}, nil
				},
				Returns: []*native.Type{native.TInstant}, BarrierPos: -1,
			}},
		},
		{
			Name: "time-unix-ms",

			Signatures: []native.NativeSig{{
				Args: []*native.Type{native.TInteger},
				Handler: func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
					n, err := args[0].AsConcreteInteger()
					if err != nil {
						return nil, err
					}
					return []native.Value{native.NewInstant(time.UnixMilli(n))}, nil
				},
				Returns: []*native.Type{native.TInstant}, BarrierPos: -1,
			}},
		},
		{
			Name: "time-unix-ns",

			Signatures: []native.NativeSig{{
				Args: []*native.Type{native.TInteger},
				Handler: func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
					n, err := args[0].AsConcreteInteger()
					if err != nil {
						return nil, err
					}
					return []native.Value{native.NewInstant(time.Unix(0, n))}, nil
				},
				Returns: []*native.Type{native.TInstant}, BarrierPos: -1,
			}},
		},
		// --- Current time (stack-only zero-arg words) ---
		{
			Name: "time-now-local",

			Signatures: []native.NativeSig{{
				Args: []*native.Type{},
				Handler: func(_ []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
					return []native.Value{native.NewDateTime(time.Now())}, nil
				},
				Returns: []*native.Type{native.TDateTime}, BarrierPos: 0,
			}},
		},
		{
			Name: "time-today",

			Signatures: []native.NativeSig{{
				Args: []*native.Type{},
				Handler: func(_ []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
					now := time.Now()
					d := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
					return []native.Value{native.NewDate(d)}, nil
				},
				Returns: []*native.Type{native.TDate}, BarrierPos: 0,
			}},
		},
		{
			Name: "time-today-utc",

			Signatures: []native.NativeSig{{
				Args: []*native.Type{},
				Handler: func(_ []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
					now := time.Now().UTC()
					d := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
					return []native.Value{native.NewDate(d)}, nil
				},
				Returns: []*native.Type{native.TDate}, BarrierPos: 0,
			}},
		},
		// --- Extraction (Date -> Integer) ---
		dateToIntNative("year", func(t time.Time) int64 { return int64(t.Year()) }),
		dateToIntNative("month", func(t time.Time) int64 { return int64(t.Month()) }),
		dateToIntNative("day", func(t time.Time) int64 { return int64(t.Day()) }),
		dateToIntNative("weekday", func(t time.Time) int64 {
			wd := t.Weekday()
			iso := int64(wd)
			if wd == time.Sunday {
				iso = 7
			}
			return iso
		}),
		dateToIntNative("year-day", func(t time.Time) int64 { return int64(t.YearDay()) }),
		dateToStringNative("weekday-name", func(t time.Time) string { return t.Weekday().String() }),
		dateToStringNative("month-name", func(t time.Time) string { return t.Month().String() }),
		dateToIntNative("iso-week", func(t time.Time) int64 {
			_, week := t.ISOWeek()
			return int64(week)
		}),
		dateToIntNative("quarter", func(t time.Time) int64 {
			m := t.Month()
			return int64((int(m) + 2) / 3)
		}),
		dateToIntNative("days-in-month", func(t time.Time) int64 {
			last := time.Date(t.Year(), t.Month()+1, 0, 0, 0, 0, 0, t.Location())
			return int64(last.Day())
		}),
		dateToIntNative("days-in-year", func(t time.Time) int64 {
			y := t.Year()
			start := time.Date(y, 1, 1, 0, 0, 0, 0, time.UTC)
			end := time.Date(y+1, 1, 1, 0, 0, 0, 0, time.UTC)
			return int64(end.Sub(start).Hours() / 24)
		}),
		{
			Name: "is-leap-year",

			Signatures: []native.NativeSig{{
				Args: []*native.Type{native.TDate},
				Handler: func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
					y := extractTime(args[0]).Year()
					leap := y%4 == 0 && (y%100 != 0 || y%400 == 0)
					return []native.Value{native.NewBoolean(leap)}, nil
				},
				Returns: []*native.Type{native.TBoolean}, BarrierPos: -1,
			}},
		},
		// to-unix / to-unix-ms (Instant -> Integer)
		{
			Name: "to-unix",

			Signatures: []native.NativeSig{{
				Args: []*native.Type{native.TInstant},
				Handler: func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
					return []native.Value{native.NewInteger(extractTime(args[0]).Unix())}, nil
				},
				Returns: []*native.Type{native.TInteger}, BarrierPos: -1,
			}},
		},
		{
			Name: "to-unix-ms",

			Signatures: []native.NativeSig{{
				Args: []*native.Type{native.TInstant},
				Handler: func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
					return []native.Value{native.NewInteger(extractTime(args[0]).UnixMilli())}, nil
				},
				Returns: []*native.Type{native.TInteger}, BarrierPos: -1,
			}},
		},
		// --- Comparison (Date Date -> Boolean) ---
		{
			Name: "is-before",

			Signatures: []native.NativeSig{{
				Args: []*native.Type{native.TDate, native.TDate},
				Handler: func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
					return []native.Value{native.NewBoolean(extractTime(args[1]).Before(extractTime(args[0])))}, nil
				},
				Returns: []*native.Type{native.TBoolean}, BarrierPos: -1,
			}},
		},
		{
			Name: "is-after",

			Signatures: []native.NativeSig{{
				Args: []*native.Type{native.TDate, native.TDate},
				Handler: func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
					return []native.Value{native.NewBoolean(extractTime(args[1]).After(extractTime(args[0])))}, nil
				},
				Returns: []*native.Type{native.TBoolean}, BarrierPos: -1,
			}},
		},
		{
			Name: "is-equal",

			Signatures: []native.NativeSig{{
				Args: []*native.Type{native.TDate, native.TDate},
				Handler: func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
					return []native.Value{native.NewBoolean(extractTime(args[0]).Equal(extractTime(args[1])))}, nil
				},
				Returns: []*native.Type{native.TBoolean}, BarrierPos: -1,
			}},
		},
		// --- Formatting ---
		{
			Name: "to-string",

			Signatures: []native.NativeSig{{
				Args: []*native.Type{native.TDate},
				Handler: func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
					return []native.Value{native.NewString(extractTime(args[0]).Format("2006-01-02"))}, nil
				},
				Returns: []*native.Type{native.TString}, BarrierPos: -1,
			}},
		},
		{
			Name: "format",

			Signatures: []native.NativeSig{{
				Args: []*native.Type{native.TString, native.TDate},
				Handler: func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
					layout, err := args[0].AsConcreteString()
					if err != nil {
						return nil, err
					}
					return []native.Value{native.NewString(extractTime(args[1]).Format(layout))}, nil
				},
				Returns: []*native.Type{native.TString}, BarrierPos: -1,
			}},
		},
		{
			Name: "to-iso",

			Signatures: []native.NativeSig{{
				Args: []*native.Type{native.TDate},
				Handler: func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
					return []native.Value{native.NewString(extractTime(args[0]).Format("2006-01-02"))}, nil
				},
				Returns: []*native.Type{native.TString}, BarrierPos: -1,
			}},
		},
		// --- Legacy arithmetic ---
		addDateNative("add-days", func(n int) (int, int, int) { return 0, 0, n }),
		addDateNative("add-months", func(n int) (int, int, int) { return 0, n, 0 }),
		addDateNative("add-years", func(n int) (int, int, int) { return n, 0, 0 }),
		// --- Duration construction (Integer -> CalDuration) ---
		intToCalDurationNative("years", native.TCalDuration, func(n int) (int, int, int) { return n, 0, 0 }),
		intToCalDurationNative("months", native.TCalDuration, func(n int) (int, int, int) { return 0, n, 0 }),
		intToCalDurationNative("weeks", native.TClkDuration, func(n int) (int, int, int) { return 0, 0, n * 7 }),
		intToCalDurationNative("days", native.TClkDuration, func(n int) (int, int, int) { return 0, 0, n }),
		// --- Duration construction (Number -> ClkDuration) ---
		numToClkDurationNative("hours", time.Hour),
		numToClkDurationNative("minutes", time.Minute),
		numToClkDurationNative("seconds", time.Second),
		numToClkDurationNative("ms", time.Millisecond),
		numToClkDurationNative("us", time.Microsecond),
		numToClkDurationNative("ns", time.Nanosecond),
		{
			// cal-dur 1 6 15 → args[0]=15 (nearest), args[1]=6, args[2]=1 (deepest)
			Name: "cal-dur",

			Signatures: []native.NativeSig{{
				Args: []*native.Type{native.TInteger, native.TInteger, native.TInteger},
				Handler: func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
					y, err := args[2].AsConcreteInteger()
					if err != nil {
						return nil, err
					}
					m, err := args[1].AsConcreteInteger()
					if err != nil {
						return nil, err
					}
					d, err := args[0].AsConcreteInteger()
					if err != nil {
						return nil, err
					}
					return []native.Value{native.NewCalDuration(int(y), int(m), int(d))}, nil
				},
				Returns: []*native.Type{native.TCalDuration}, BarrierPos: -1,
			}},
		},
		// time-duration (ISO 8601 P1Y2M3D parser) removed as a feature.
		// --- Duration extraction ---
		clkDurationToDecimalNative("total-hours", native.TDecimal, func(d time.Duration) float64 { return d.Hours() }),
		clkDurationToDecimalNative("total-minutes", native.TDecimal, func(d time.Duration) float64 { return d.Minutes() }),
		clkDurationToDecimalNative("total-seconds", native.TDecimal, func(d time.Duration) float64 { return d.Seconds() }),
		// total-ms: handler pushes Decimal but historical Returns is TInteger.
		clkDurationToDecimalNative("total-ms", native.TInteger, func(d time.Duration) float64 { return float64(d.Milliseconds()) }),
		calDurationToIntNative("dur-years", func(cd native.CalDurationData) int64 { return int64(cd.Years) }),
		calDurationToIntNative("dur-months", func(cd native.CalDurationData) int64 { return int64(cd.Months) }),
		calDurationToIntNative("dur-days", func(cd native.CalDurationData) int64 { return int64(cd.Days) }),
		// dur-sign — two overloads (CalDuration and ClkDuration), both
		// return -1/0/1 integers. Historically two separate r.Register
		// calls; here unified into one NativeFunc with two signatures.
		{
			Name: "dur-sign",

			Signatures: []native.NativeSig{
				{
					Args: []*native.Type{native.TCalDuration},
					Handler: func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
						cd, _ := native.AsCalDuration(args[0])
						total := cd.Years*365 + cd.Months*30 + cd.Days
						switch {
						case total < 0:
							return []native.Value{native.NewInteger(-1)}, nil
						case total > 0:
							return []native.Value{native.NewInteger(1)}, nil
						default:
							return []native.Value{native.NewInteger(0)}, nil
						}
					},
					Returns: []*native.Type{native.TInteger}, BarrierPos: -1,
				},
				{
					Args: []*native.Type{native.TClkDuration},
					Handler: func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
						d, _ := native.AsClkDuration(args[0])
						switch {
						case d < 0:
							return []native.Value{native.NewInteger(-1)}, nil
						case d > 0:
							return []native.Value{native.NewInteger(1)}, nil
						default:
							return []native.Value{native.NewInteger(0)}, nil
						}
					},
					Returns: []*native.Type{native.TInteger}, BarrierPos: -1,
				},
			},
		},
		// --- Arithmetic ---
		{
			// d1 d2 until → args[0]=d2 (nearest), args[1]=d1
			Name: "until",

			Signatures: []native.NativeSig{{
				Args: []*native.Type{native.TDate, native.TDate},
				Handler: func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
					from := extractTime(args[1])
					to := extractTime(args[0])
					cd := dateDiffCalDuration(from, to)
					return []native.Value{native.NewCalDuration(cd.Years, cd.Months, cd.Days)}, nil
				},
				Returns: []*native.Type{native.TClkDuration}, BarrierPos: -1,
			}},
		},
		{
			// d1 d2 since → args[0]=d2 (nearest), args[1]=d1
			Name: "since",

			Signatures: []native.NativeSig{{
				Args: []*native.Type{native.TDate, native.TDate},
				Handler: func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
					from := extractTime(args[0])
					to := extractTime(args[1])
					cd := dateDiffCalDuration(from, to)
					return []native.Value{native.NewCalDuration(cd.Years, cd.Months, cd.Days)}, nil
				},
				Returns: []*native.Type{native.TClkDuration}, BarrierPos: -1,
			}},
		},
		{
			// ins1 ins2 diff → args[0]=ins2 (nearest), args[1]=ins1
			Name: "diff",

			Signatures: []native.NativeSig{{
				Args: []*native.Type{native.TInstant, native.TInstant},
				Handler: func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
					t1 := extractTime(args[1])
					t2 := extractTime(args[0])
					return []native.Value{native.NewClkDuration(t2.Sub(t1))}, nil
				},
				Returns: []*native.Type{native.TClkDuration}, BarrierPos: -1,
			}},
		},
		{
			Name: "elapsed",

			Signatures: []native.NativeSig{{
				Args: []*native.Type{native.TInstant},
				Handler: func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
					start := extractTime(args[0])
					return []native.Value{native.NewClkDuration(time.Since(start))}, nil
				},
				Returns: []*native.Type{native.TClkDuration}, BarrierPos: -1,
			}},
		},
		// --- Comparison extended ---
		{
			Name: "time-compare",

			Signatures: []native.NativeSig{{
				Args: []*native.Type{native.TDate, native.TDate},
				Handler: func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
					t1 := extractTime(args[1])
					t2 := extractTime(args[0])
					switch {
					case t1.Before(t2):
						return []native.Value{native.NewInteger(-1)}, nil
					case t1.After(t2):
						return []native.Value{native.NewInteger(1)}, nil
					default:
						return []native.Value{native.NewInteger(0)}, nil
					}
				},
				Returns: []*native.Type{native.TInteger}, BarrierPos: -1,
			}},
		},
		{
			// d start end is-between → args[0]=end (nearest), args[1]=start, args[2]=d (deepest)
			Name: "is-between",

			Signatures: []native.NativeSig{{
				Args: []*native.Type{native.TDate, native.TDate, native.TDate},
				Handler: func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
					d := extractTime(args[2])
					start := extractTime(args[1])
					end := extractTime(args[0])
					return []native.Value{native.NewBoolean(!d.Before(start) && !d.After(end))}, nil
				},
				Returns: []*native.Type{native.TBoolean}, BarrierPos: -1,
			}},
		},
		{
			Name: "earliest",

			Signatures: []native.NativeSig{{
				Args: []*native.Type{native.TDate, native.TDate},
				Handler: func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
					t1 := extractTime(args[1])
					t2 := extractTime(args[0])
					if t1.Before(t2) {
						return []native.Value{native.NewDate(t1)}, nil
					}
					return []native.Value{native.NewDate(t2)}, nil
				},
				Returns: []*native.Type{native.TAny}, BarrierPos: -1,
			}},
		},
		{
			Name: "latest",

			Signatures: []native.NativeSig{{
				Args: []*native.Type{native.TDate, native.TDate},
				Handler: func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
					t1 := extractTime(args[1])
					t2 := extractTime(args[0])
					if t1.After(t2) {
						return []native.Value{native.NewDate(t1)}, nil
					}
					return []native.Value{native.NewDate(t2)}, nil
				},
				Returns: []*native.Type{native.TAny}, BarrierPos: -1,
			}},
		},
		// --- Conversion ---
		// to-date — DateTime overload + Instant overload (UTC).
		{
			Name: "to-date",

			Signatures: []native.NativeSig{
				{
					Args: []*native.Type{native.TDateTime},
					Handler: func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
						t := extractTime(args[0])
						return []native.Value{native.NewDate(time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location()))}, nil
					},
					Returns: []*native.Type{native.TDate}, BarrierPos: -1,
				},
				{
					Args: []*native.Type{native.TInstant},
					Handler: func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
						t := extractTime(args[0])
						return []native.Value{native.NewDate(time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC))}, nil
					},
					Returns: []*native.Type{native.TDate}, BarrierPos: -1,
				},
			},
		},
		// to-time-of-day — DateTime + Instant overloads (identical body).
		{
			Name: "to-time-of-day",

			Signatures: []native.NativeSig{
				{
					Args: []*native.Type{native.TDateTime},
					Handler: func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
						t := extractTime(args[0])
						d := time.Duration(t.Hour())*time.Hour + time.Duration(t.Minute())*time.Minute +
							time.Duration(t.Second())*time.Second + time.Duration(t.Nanosecond())
						return []native.Value{native.NewTimeOfDay(d)}, nil
					},
					Returns: []*native.Type{native.TTimeOfDay}, BarrierPos: -1,
				},
				{
					Args: []*native.Type{native.TInstant},
					Handler: func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
						t := extractTime(args[0])
						d := time.Duration(t.Hour())*time.Hour + time.Duration(t.Minute())*time.Minute +
							time.Duration(t.Second())*time.Second + time.Duration(t.Nanosecond())
						return []native.Value{native.NewTimeOfDay(d)}, nil
					},
					Returns: []*native.Type{native.TTimeOfDay}, BarrierPos: -1,
				},
			},
		},
		{
			Name: "to-datetime",

			Signatures: []native.NativeSig{{
				Args: []*native.Type{native.TDate},
				Handler: func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
					t := extractTime(args[0])
					return []native.Value{native.NewDateTime(t)}, nil
				},
				Returns: []*native.Type{native.TDateTime}, BarrierPos: -1,
			}},
		},
		{
			// dt tz to-instant → args[0]=tz (nearest), args[1]=dt
			Name: "to-instant",

			Signatures: []native.NativeSig{{
				Args: []*native.Type{native.TTimezone, native.TDateTime},
				Handler: func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
					dt := extractTime(args[1])
					loc := native.AsTimezone(args[0])
					if loc == nil {
						loc = time.UTC
					}
					t := time.Date(dt.Year(), dt.Month(), dt.Day(), dt.Hour(), dt.Minute(), dt.Second(), dt.Nanosecond(), loc)
					return []native.Value{native.NewInstant(t)}, nil
				},
				Returns: []*native.Type{native.TInstant}, BarrierPos: -1,
			}},
		},
		{
			// ins tz to-local → args[0]=tz (nearest), args[1]=ins
			Name: "to-local",

			Signatures: []native.NativeSig{{
				Args: []*native.Type{native.TTimezone, native.TInstant},
				Handler: func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
					t := extractTime(args[1])
					loc := native.AsTimezone(args[0])
					if loc == nil {
						loc = time.UTC
					}
					return []native.Value{native.NewDateTime(t.In(loc))}, nil
				},
				Returns: []*native.Type{native.TDateTime}, BarrierPos: -1,
			}},
		},
		{
			Name: "to-utc",

			Signatures: []native.NativeSig{{
				Args: []*native.Type{native.TInstant},
				Handler: func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
					t := extractTime(args[0])
					return []native.Value{native.NewDateTime(t.UTC())}, nil
				},
				Returns: []*native.Type{native.TDateTime}, BarrierPos: -1,
			}},
		},
		// --- Rounding ---
		{
			Name: "start-of",

			Signatures: []native.NativeSig{{
				Args: []*native.Type{native.TString, native.TDate},
				Handler: func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
					unit, err := args[0].AsConcreteString()
					if err != nil {
						return nil, err
					}
					t := extractTime(args[1])
					var result time.Time
					switch unit {
					case "year":
						result = time.Date(t.Year(), 1, 1, 0, 0, 0, 0, t.Location())
					case "quarter":
						q := (int(t.Month()) + 2) / 3
						m := time.Month((q-1)*3 + 1)
						result = time.Date(t.Year(), m, 1, 0, 0, 0, 0, t.Location())
					case "month":
						result = time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, t.Location())
					case "week":
						wd := t.Weekday()
						if wd == time.Sunday {
							wd = 7
						}
						result = t.AddDate(0, 0, -int(wd-time.Monday))
						result = time.Date(result.Year(), result.Month(), result.Day(), 0, 0, 0, 0, t.Location())
					case "day":
						result = time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
					default:
						return nil, fmt.Errorf("start-of: unknown unit %q", unit)
					}
					return []native.Value{native.NewDate(result)}, nil
				},
				Returns: []*native.Type{native.TAny}, BarrierPos: -1,
			}},
		},
		{
			Name: "end-of",

			Signatures: []native.NativeSig{{
				Args: []*native.Type{native.TString, native.TDate},
				Handler: func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
					unit, err := args[0].AsConcreteString()
					if err != nil {
						return nil, err
					}
					t := extractTime(args[1])
					var result time.Time
					switch unit {
					case "year":
						result = time.Date(t.Year(), 12, 31, 0, 0, 0, 0, t.Location())
					case "quarter":
						q := (int(t.Month()) + 2) / 3
						endMonth := time.Month(q * 3)
						last := time.Date(t.Year(), endMonth+1, 0, 0, 0, 0, 0, t.Location())
						result = last
					case "month":
						result = time.Date(t.Year(), t.Month()+1, 0, 0, 0, 0, 0, t.Location())
					case "week":
						wd := t.Weekday()
						if wd == time.Sunday {
							wd = 7
						}
						daysToSunday := 7 - int(wd)
						result = t.AddDate(0, 0, daysToSunday)
						result = time.Date(result.Year(), result.Month(), result.Day(), 0, 0, 0, 0, t.Location())
					case "day":
						result = time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
					default:
						return nil, fmt.Errorf("end-of: unknown unit %q", unit)
					}
					return []native.Value{native.NewDate(result)}, nil
				},
				Returns: []*native.Type{native.TAny}, BarrierPos: -1,
			}},
		},
		// --- Timezone ---
		{
			Name: "tz-utc",

			Signatures: []native.NativeSig{{
				Args: []*native.Type{},
				Handler: func(_ []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
					return []native.Value{native.NewTimezone(time.UTC)}, nil
				},
				Returns: []*native.Type{native.TTimezone}, BarrierPos: 0,
			}},
		},
		{
			Name: "tz-local",

			Signatures: []native.NativeSig{{
				Args: []*native.Type{},
				Handler: func(_ []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
					return []native.Value{native.NewTimezone(time.Local)}, nil
				},
				Returns: []*native.Type{native.TTimezone}, BarrierPos: 0,
			}},
		},
		{
			Name: "tz-name",

			Signatures: []native.NativeSig{{
				Args: []*native.Type{native.TTimezone},
				Handler: func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
					loc := native.AsTimezone(args[0])
					if loc == nil {
						return []native.Value{native.NewString("UTC")}, nil
					}
					return []native.Value{native.NewString(loc.String())}, nil
				},
				Returns: []*native.Type{native.TString}, BarrierPos: -1,
			}},
		},
		{
			// ins tz tz-offset → args[0]=tz (nearest), args[1]=ins
			Name: "tz-offset",

			Signatures: []native.NativeSig{{
				Args: []*native.Type{native.TTimezone, native.TInstant},
				Handler: func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
					t := extractTime(args[1])
					loc := native.AsTimezone(args[0])
					if loc == nil {
						loc = time.UTC
					}
					_, offset := t.In(loc).Zone()
					h := offset / 3600
					m := (offset % 3600) / 60
					if m < 0 {
						m = -m
					}
					sign := "+"
					if h < 0 {
						sign = "-"
						h = -h
					}
					return []native.Value{native.NewString(fmt.Sprintf("%s%02d:%02d", sign, h, m))}, nil
				},
				Returns: []*native.Type{native.TClkDuration}, BarrierPos: -1,
			}},
		},
		{
			// ins tz is-dst → args[0]=tz (nearest), args[1]=ins
			Name: "is-dst",

			Signatures: []native.NativeSig{{
				Args: []*native.Type{native.TTimezone, native.TInstant},
				Handler: func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
					t := extractTime(args[1])
					loc := native.AsTimezone(args[0])
					if loc == nil {
						loc = time.UTC
					}
					name, _ := t.In(loc).Zone()
					// Heuristic: standard zone names don't end in "DT" or "ST",
					// but DST zones typically contain "DT" (e.g. EDT, CDT, PDT).
					// A more robust check: compare January offset vs current offset.
					jan := time.Date(t.Year(), 1, 1, 0, 0, 0, 0, loc)
					jul := time.Date(t.Year(), 7, 1, 0, 0, 0, 0, loc)
					_, janOff := jan.Zone()
					_, julOff := jul.Zone()
					_, curOff := t.In(loc).Zone()
					_ = name
					// If offsets differ between Jan and Jul, the larger offset is DST.
					if janOff == julOff {
						return []native.Value{native.NewBoolean(false)}, nil
					}
					stdOff := janOff
					if julOff < janOff {
						stdOff = julOff // Southern hemisphere
					}
					return []native.Value{native.NewBoolean(curOff != stdOff)}, nil
				},
				Returns: []*native.Type{native.TBoolean}, BarrierPos: -1,
			}},
		},
		// --- Parsing ---
		// parse-date, parse-datetime, auto-date removed as a feature.
		// Date construction is via numeric helpers / current-time
		// sources (today, now-local, unix, unix-ms, unix-ns), not
		// string parsing.
	}

	return out
}()
