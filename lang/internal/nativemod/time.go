package nativemod

import (
	"fmt"
	"strings"
	"time"

	"github.com/aql-lang/aql/lang/engine"
)

// BuildTimeModule creates the "aql:time" native module.
func BuildTimeModule(parent *engine.Registry) (engine.ModuleDesc, error) {
	subReg, err := engine.DefaultRegistry()
	if err != nil {
		return engine.ModuleDesc{}, err
	}

	for _, n := range TimeNatives {
		subReg.RegisterNativeFunc(n)
	}

	exports := engine.NewOrderedMap()

	// Construction
	exports.Set("date", makeTimeFnDef("time-date", []engine.FnParam{{Type: engine.TString}}, []*engine.Type{engine.TDate}, subReg))
	exports.Set("datetime", makeTimeFnDef("time-datetime", []engine.FnParam{{Type: engine.TString}}, []*engine.Type{engine.TDateTime}, subReg))
	exports.Set("instant", makeTimeFnDef("time-instant", []engine.FnParam{{Type: engine.TString}}, []*engine.Type{engine.TInstant}, subReg))
	exports.Set("time-of-day", makeTimeFnDef("time-time-of-day", []engine.FnParam{{Type: engine.TString}}, []*engine.Type{engine.TTimeOfDay}, subReg))
	exports.Set("tz", makeTimeFnDef("time-tz", []engine.FnParam{{Type: engine.TString}}, []*engine.Type{engine.TTimezone}, subReg))
	exports.Set("unix", makeTimeFnDef("time-unix", []engine.FnParam{{Type: engine.TInteger}}, []*engine.Type{engine.TInstant}, subReg))
	exports.Set("unix-ms", makeTimeFnDef("time-unix-ms", []engine.FnParam{{Type: engine.TInteger}}, []*engine.Type{engine.TInstant}, subReg))
	exports.Set("unix-ns", makeTimeFnDef("time-unix-ns", []engine.FnParam{{Type: engine.TInteger}}, []*engine.Type{engine.TInstant}, subReg))

	// Current time
	exports.Set("now-local", makeTimeFnDef("time-now-local", []engine.FnParam{}, []*engine.Type{engine.TDateTime}, subReg))
	exports.Set("today", makeTimeFnDef("time-today", []engine.FnParam{}, []*engine.Type{engine.TDate}, subReg))
	exports.Set("today-utc", makeTimeFnDef("time-today-utc", []engine.FnParam{}, []*engine.Type{engine.TDate}, subReg))

	// Extraction (Date -> Integer)
	for _, name := range []string{"year", "month", "day", "weekday", "year-day", "iso-week", "quarter", "days-in-month", "days-in-year"} {
		exports.Set(name, makeTimeFnDef(name, []engine.FnParam{{Type: engine.TDate}}, []*engine.Type{engine.TInteger}, subReg))
	}
	for _, name := range []string{"weekday-name", "month-name"} {
		exports.Set(name, makeTimeFnDef(name, []engine.FnParam{{Type: engine.TDate}}, []*engine.Type{engine.TString}, subReg))
	}
	exports.Set("leap-year?", makeTimeFnDef("leap-year?", []engine.FnParam{{Type: engine.TDate}}, []*engine.Type{engine.TBoolean}, subReg))

	// Extraction from Instant
	exports.Set("to-unix", makeTimeFnDef("to-unix", []engine.FnParam{{Type: engine.TInstant}}, []*engine.Type{engine.TInteger}, subReg))
	exports.Set("to-unix-ms", makeTimeFnDef("to-unix-ms", []engine.FnParam{{Type: engine.TInstant}}, []*engine.Type{engine.TInteger}, subReg))

	// Comparison (Date Date -> Boolean)
	for _, name := range []string{"before?", "after?", "equal?"} {
		exports.Set(name, makeTimeFnDef(name, []engine.FnParam{{Type: engine.TDate}, {Type: engine.TDate}}, []*engine.Type{engine.TBoolean}, subReg))
	}

	// Formatting
	exports.Set("to-string", makeTimeFnDef("to-string", []engine.FnParam{{Type: engine.TDate}}, []*engine.Type{engine.TString}, subReg))
	exports.Set("format", makeTimeFnDef("format", []engine.FnParam{{Type: engine.TDate}, {Type: engine.TString}}, []*engine.Type{engine.TString}, subReg))
	exports.Set("to-iso", makeTimeFnDef("to-iso", []engine.FnParam{{Type: engine.TDate}}, []*engine.Type{engine.TString}, subReg))

	// Duration construction
	for _, name := range []string{"years", "months", "weeks", "days"} {
		exports.Set(name, makeTimeFnDef(name, []engine.FnParam{{Type: engine.TInteger}}, []*engine.Type{engine.TCalDuration}, subReg))
	}
	for _, name := range []string{"hours", "minutes", "seconds", "ms", "us", "ns"} {
		exports.Set(name, makeTimeFnDef(name, []engine.FnParam{{Type: engine.TNumber}}, []*engine.Type{engine.TClkDuration}, subReg))
	}
	exports.Set("cal-dur", makeTimeFnDef("cal-dur", []engine.FnParam{{Type: engine.TInteger}, {Type: engine.TInteger}, {Type: engine.TInteger}}, []*engine.Type{engine.TCalDuration}, subReg))
	exports.Set("duration", makeTimeFnDef("time-duration", []engine.FnParam{{Type: engine.TString}}, []*engine.Type{engine.TCalDuration}, subReg))

	// Duration extraction
	for _, name := range []string{"total-hours", "total-minutes", "total-seconds", "total-ms"} {
		exports.Set(name, makeTimeFnDef(name, []engine.FnParam{{Type: engine.TClkDuration}}, []*engine.Type{engine.TDecimal}, subReg))
	}
	for _, name := range []string{"dur-years", "dur-months", "dur-days"} {
		exports.Set(name, makeTimeFnDef(name, []engine.FnParam{{Type: engine.TCalDuration}}, []*engine.Type{engine.TInteger}, subReg))
	}
	exports.Set("dur-sign", makeTimeFnDef("dur-sign", []engine.FnParam{{Type: engine.TCalDuration}}, []*engine.Type{engine.TInteger}, subReg))

	// Arithmetic
	exports.Set("until", makeTimeFnDef("until", []engine.FnParam{{Type: engine.TDate}, {Type: engine.TDate}}, []*engine.Type{engine.TCalDuration}, subReg))
	exports.Set("since", makeTimeFnDef("since", []engine.FnParam{{Type: engine.TDate}, {Type: engine.TDate}}, []*engine.Type{engine.TCalDuration}, subReg))
	exports.Set("diff", makeTimeFnDef("diff", []engine.FnParam{{Type: engine.TInstant}, {Type: engine.TInstant}}, []*engine.Type{engine.TClkDuration}, subReg))
	exports.Set("elapsed", makeTimeFnDef("elapsed", []engine.FnParam{{Type: engine.TInstant}}, []*engine.Type{engine.TClkDuration}, subReg))

	// Comparison extended
	exports.Set("compare", makeTimeFnDef("time-compare", []engine.FnParam{{Type: engine.TDate}, {Type: engine.TDate}}, []*engine.Type{engine.TInteger}, subReg))
	exports.Set("between?", makeTimeFnDef("between?", []engine.FnParam{{Type: engine.TDate}, {Type: engine.TDate}, {Type: engine.TDate}}, []*engine.Type{engine.TBoolean}, subReg))
	exports.Set("earliest", makeTimeFnDef("earliest", []engine.FnParam{{Type: engine.TDate}, {Type: engine.TDate}}, []*engine.Type{engine.TDate}, subReg))
	exports.Set("latest", makeTimeFnDef("latest", []engine.FnParam{{Type: engine.TDate}, {Type: engine.TDate}}, []*engine.Type{engine.TDate}, subReg))

	// Conversion
	exports.Set("to-date", makeTimeFnDef("to-date", []engine.FnParam{{Type: engine.TDateTime}}, []*engine.Type{engine.TDate}, subReg))
	exports.Set("to-time-of-day", makeTimeFnDef("to-time-of-day", []engine.FnParam{{Type: engine.TDateTime}}, []*engine.Type{engine.TTimeOfDay}, subReg))
	exports.Set("to-datetime", makeTimeFnDef("to-datetime", []engine.FnParam{{Type: engine.TDate}}, []*engine.Type{engine.TDateTime}, subReg))
	exports.Set("to-instant", makeTimeFnDef("to-instant", []engine.FnParam{{Type: engine.TDateTime}, {Type: engine.TTimezone}}, []*engine.Type{engine.TInstant}, subReg))
	exports.Set("to-local", makeTimeFnDef("to-local", []engine.FnParam{{Type: engine.TInstant}, {Type: engine.TTimezone}}, []*engine.Type{engine.TDateTime}, subReg))
	exports.Set("to-utc", makeTimeFnDef("to-utc", []engine.FnParam{{Type: engine.TInstant}}, []*engine.Type{engine.TDateTime}, subReg))

	// Rounding
	exports.Set("start-of", makeTimeFnDef("start-of", []engine.FnParam{{Type: engine.TDate}, {Type: engine.TString}}, []*engine.Type{engine.TDate}, subReg))
	exports.Set("end-of", makeTimeFnDef("end-of", []engine.FnParam{{Type: engine.TDate}, {Type: engine.TString}}, []*engine.Type{engine.TDate}, subReg))

	// Timezone
	exports.Set("tz-utc", makeTimeFnDef("tz-utc", []engine.FnParam{}, []*engine.Type{engine.TTimezone}, subReg))
	exports.Set("tz-local", makeTimeFnDef("tz-local", []engine.FnParam{}, []*engine.Type{engine.TTimezone}, subReg))
	exports.Set("tz-name", makeTimeFnDef("tz-name", []engine.FnParam{{Type: engine.TTimezone}}, []*engine.Type{engine.TString}, subReg))
	exports.Set("tz-offset", makeTimeFnDef("tz-offset", []engine.FnParam{{Type: engine.TInstant}, {Type: engine.TTimezone}}, []*engine.Type{engine.TString}, subReg))
	exports.Set("dst?", makeTimeFnDef("dst?", []engine.FnParam{{Type: engine.TInstant}, {Type: engine.TTimezone}}, []*engine.Type{engine.TBoolean}, subReg))

	// Parsing
	exports.Set("parse-date", makeTimeFnDef("parse-date", []engine.FnParam{{Type: engine.TString}, {Type: engine.TString}}, []*engine.Type{engine.TDate}, subReg))
	exports.Set("parse-datetime", makeTimeFnDef("parse-datetime", []engine.FnParam{{Type: engine.TString}, {Type: engine.TString}}, []*engine.Type{engine.TDateTime}, subReg))
	exports.Set("auto-date", makeTimeFnDef("auto-date", []engine.FnParam{{Type: engine.TString}}, []*engine.Type{engine.TDate}, subReg))

	// Legacy arithmetic — FnDef params are in user-facing positional order
	// (deepest-first match): `date n add-days`. The underlying NativeFunc
	// sig is "data-last" (top-of-stack-first match): [TInteger, TDate].
	for _, name := range []string{"add-days", "add-months", "add-years"} {
		exports.Set(name, makeTimeFnDef(name, []engine.FnParam{{Type: engine.TDate}, {Type: engine.TInteger}}, []*engine.Type{engine.TDate}, subReg))
	}

	modID := parent.Modules.NextID()
	desc := engine.ModuleDesc{
		ID:      modID,
		Exports: map[string]*engine.OrderedMap{"time": exports},
	}
	return desc, nil
}

// makeTimeFnDef creates a FnDef value with the given params, returns, and word name.
func makeTimeFnDef(wordName string, params []engine.FnParam, returns []*engine.Type, subReg *engine.Registry) engine.Value {
	fnDef := engine.FnDefInfo{
		Name: wordName,
		Sigs: []engine.FnSig{
			{
				Params:  params,
				Returns: returns,
				Body:    []engine.Value{engine.NewWord(wordName)},
			},
		},
		Registry: subReg,
	}
	return engine.NewFnDef(fnDef)
}

// --- helpers ---

// extractTime returns the time.Time from a Date, DateTime, or Instant value.
func extractTime(v engine.Value) time.Time {
	if t, ok := v.Data.(time.Time); ok {
		return t
	}
	return time.Time{}
}

// dateDiffCalDuration computes the CalDuration between two dates (from → to).
func dateDiffCalDuration(from, to time.Time) engine.CalDurationData {
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
	return engine.CalDurationData{Years: years, Months: months, Days: days}
}

// parseISO8601Duration parses a subset of ISO 8601 durations: P[nY][nM][nD][T[nH][nM][nS]]
func parseISO8601Duration(s string) (engine.CalDurationData, time.Duration, bool, error) {
	if !strings.HasPrefix(s, "P") {
		return engine.CalDurationData{}, 0, false, fmt.Errorf("duration: must start with P: %q", s)
	}
	rest := s[1:]
	var years, months, days int
	var clk time.Duration
	isCal := true

	// Split on T
	parts := strings.SplitN(rest, "T", 2)
	datePart := parts[0]
	timePart := ""
	if len(parts) == 2 {
		timePart = parts[1]
		isCal = false
	}

	// Parse date components: nY, nM, nD
	for len(datePart) > 0 {
		i := 0
		for i < len(datePart) && (datePart[i] >= '0' && datePart[i] <= '9') {
			i++
		}
		if i == 0 || i >= len(datePart) {
			return engine.CalDurationData{}, 0, false, fmt.Errorf("duration: invalid date component in %q", s)
		}
		n := 0
		for _, c := range datePart[:i] {
			n = n*10 + int(c-'0')
		}
		switch datePart[i] {
		case 'Y':
			years = n
		case 'M':
			months = n
		case 'W':
			days += n * 7
		case 'D':
			days = n
		default:
			return engine.CalDurationData{}, 0, false, fmt.Errorf("duration: unknown date unit %c in %q", datePart[i], s)
		}
		datePart = datePart[i+1:]
	}

	// Parse time components: nH, nM, nS
	for len(timePart) > 0 {
		i := 0
		for i < len(timePart) && (timePart[i] >= '0' && timePart[i] <= '9' || timePart[i] == '.') {
			i++
		}
		if i == 0 || i >= len(timePart) {
			return engine.CalDurationData{}, 0, false, fmt.Errorf("duration: invalid time component in %q", s)
		}
		n := 0.0
		_, _ = fmt.Sscanf(timePart[:i], "%f", &n)
		switch timePart[i] {
		case 'H':
			clk += time.Duration(n * float64(time.Hour))
		case 'M':
			clk += time.Duration(n * float64(time.Minute))
		case 'S':
			clk += time.Duration(n * float64(time.Second))
		default:
			return engine.CalDurationData{}, 0, false, fmt.Errorf("duration: unknown time unit %c in %q", timePart[i], s)
		}
		timePart = timePart[i+1:]
	}

	if isCal && clk == 0 {
		return engine.CalDurationData{Years: years, Months: months, Days: days}, 0, true, nil
	}
	if years == 0 && months == 0 && days == 0 {
		return engine.CalDurationData{}, clk, false, nil
	}
	// Mixed: return as CalDuration (date part only; time part is lost)
	return engine.CalDurationData{Years: years, Months: months, Days: days}, clk, true, nil
}

// autoDateLayouts is a list of common date formats tried in order by auto-date.
var autoDateLayouts = []string{
	"2006-01-02",
	"2006-01-02T15:04:05",
	"2006-01-02 15:04:05",
	time.RFC3339,
	"01/02/2006",       // US
	"02/01/2006",       // European
	"Jan 2, 2006",      // English
	"January 2, 2006",  // Full English
	"2 Jan 2006",       // European English
	"2006/01/02",       // ISO with slashes
	"02-Jan-2006",      // Dash with abbrev month
	time.RFC1123,       // RFC 1123
	time.RFC822,        // RFC 822
	"Mon, 02 Jan 2006", // RFC 2822 date part
}

// --- builders for repeating shapes ---

// dateToIntNative builds a NativeFunc with a single [TDate] -> [TInteger]
// signature whose handler applies fn to the extracted time.Time.
func dateToIntNative(name string, fn func(time.Time) int64) engine.NativeFunc {
	return engine.NativeFunc{
		Name:        name,
		ForwardArgs: true,
		Signatures: []engine.NativeSig{{
			Args: []*engine.Type{engine.TDate},
			Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
				return []engine.Value{engine.NewInteger(fn(extractTime(args[0])))}, nil
			},
			Returns: []*engine.Type{engine.TInteger},
		}},
	}
}

// dateToStringNative builds a NativeFunc with a single [TDate] -> [TString]
// signature whose handler applies fn to the extracted time.Time.
func dateToStringNative(name string, fn func(time.Time) string) engine.NativeFunc {
	return engine.NativeFunc{
		Name:        name,
		ForwardArgs: true,
		Signatures: []engine.NativeSig{{
			Args: []*engine.Type{engine.TDate},
			Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
				return []engine.Value{engine.NewString(fn(extractTime(args[0])))}, nil
			},
			Returns: []*engine.Type{engine.TString},
		}},
	}
}

// intToCalDurationNative builds a NativeFunc with [TInteger] -> [TCalDuration]
// where the handler turns the integer into a CalDurationData via fn.
func intToCalDurationNative(name string, returnType *engine.Type, fn func(int) (int, int, int)) engine.NativeFunc {
	return engine.NativeFunc{
		Name:        name,
		ForwardArgs: true,
		Signatures: []engine.NativeSig{{
			Args: []*engine.Type{engine.TInteger},
			Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
				n, err := args[0].AsConcreteInteger()
				if err != nil {
					return nil, err
				}
				y, m, d := fn(int(n))
				return []engine.Value{engine.NewCalDuration(y, m, d)}, nil
			},
			Returns: []*engine.Type{returnType},
		}},
	}
}

// numToClkDurationNative builds a NativeFunc with [TNumber] -> [TClkDuration]
// where the handler scales the numeric input by `unit`.
func numToClkDurationNative(name string, unit time.Duration) engine.NativeFunc {
	return engine.NativeFunc{
		Name:        name,
		ForwardArgs: true,
		Signatures: []engine.NativeSig{{
			Args: []*engine.Type{engine.TNumber},
			Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
				n, err := args[0].AsNumber()
				if err != nil {
					return nil, err
				}
				return []engine.Value{engine.NewClkDuration(time.Duration(n * float64(unit)))}, nil
			},
			Returns: []*engine.Type{engine.TClkDuration},
		}},
	}
}

// clkDurationToDecimalNative builds a NativeFunc with
// [TClkDuration] -> [TDecimal] for a total-* extraction. `returnType`
// is exposed because total-ms historically declared TInteger even
// though the value pushed is Decimal.
func clkDurationToDecimalNative(name string, returnType *engine.Type, fn func(time.Duration) float64) engine.NativeFunc {
	return engine.NativeFunc{
		Name:        name,
		ForwardArgs: true,
		Signatures: []engine.NativeSig{{
			Args: []*engine.Type{engine.TClkDuration},
			Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
				d, _ := args[0].AsClkDuration()
				return []engine.Value{engine.NewDecimal(fn(d))}, nil
			},
			Returns: []*engine.Type{returnType},
		}},
	}
}

// calDurationToIntNative builds a NativeFunc with
// [TCalDuration] -> [TInteger] for dur-years/dur-months/dur-days.
func calDurationToIntNative(name string, fn func(engine.CalDurationData) int64) engine.NativeFunc {
	return engine.NativeFunc{
		Name:        name,
		ForwardArgs: true,
		Signatures: []engine.NativeSig{{
			Args: []*engine.Type{engine.TCalDuration},
			Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
				cd, _ := args[0].AsCalDuration()
				return []engine.Value{engine.NewInteger(fn(cd))}, nil
			},
			Returns: []*engine.Type{engine.TInteger},
		}},
	}
}

// addDateNative builds the add-days/add-months/add-years words. The
// fn signature mirrors time.Time.AddDate's parameters so each word
// just supplies (years, months, days) shifts driven by the integer arg.
func addDateNative(name string, build func(n int) (years, months, days int)) engine.NativeFunc {
	return engine.NativeFunc{
		Name:        name,
		ForwardArgs: true,
		Signatures: []engine.NativeSig{{
			Args: []*engine.Type{engine.TInteger, engine.TDate},
			Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
				n, err := args[0].AsConcreteInteger()
				if err != nil {
					return nil, err
				}
				t := extractTime(args[1])
				y, m, d := build(int(n))
				return []engine.Value{engine.NewDate(t.AddDate(y, m, d))}, nil
			},
			Returns: []*engine.Type{engine.TAny},
		}},
	}
}

// --- Word definitions ---

// TimeNatives is the consolidated NativeFunc slice for the time
// module's Go-implemented words. Replaces the per-word
// register* functions and the master registerAllTimeWords aggregator.
var TimeNatives = func() []engine.NativeFunc {
	out := []engine.NativeFunc{
		// --- Construction ---
		{
			Name:        "time-date",
			ForwardArgs: true,
			Signatures: []engine.NativeSig{{
				Args: []*engine.Type{engine.TString},
				Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
					s, err := args[0].AsConcreteString()
					if err != nil {
						return nil, err
					}
					t, err := time.Parse("2006-01-02", s)
					if err != nil {
						return nil, fmt.Errorf("date: invalid ISO 8601 date string: %q", s)
					}
					return []engine.Value{engine.NewDate(t)}, nil
				},
				Returns: []*engine.Type{engine.TDate},
			}},
		},
		{
			Name:        "time-datetime",
			ForwardArgs: true,
			Signatures: []engine.NativeSig{{
				Args: []*engine.Type{engine.TString},
				Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
					s, err := args[0].AsConcreteString()
					if err != nil {
						return nil, err
					}
					for _, layout := range []string{
						"2006-01-02T15:04:05.999999999",
						"2006-01-02T15:04:05",
						"2006-01-02 15:04:05",
					} {
						if t, e := time.Parse(layout, s); e == nil {
							return []engine.Value{engine.NewDateTime(t)}, nil
						}
					}
					return nil, fmt.Errorf("datetime: invalid datetime string: %q", s)
				},
				Returns: []*engine.Type{engine.TDateTime},
			}},
		},
		{
			Name:        "time-instant",
			ForwardArgs: true,
			Signatures: []engine.NativeSig{{
				Args: []*engine.Type{engine.TString},
				Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
					s, err := args[0].AsConcreteString()
					if err != nil {
						return nil, err
					}
					for _, layout := range []string{
						time.RFC3339Nano,
						time.RFC3339,
						"2006-01-02T15:04:05Z",
						"2006-01-02T15:04:05-07:00",
					} {
						if t, e := time.Parse(layout, s); e == nil {
							return []engine.Value{engine.NewInstant(t)}, nil
						}
					}
					return nil, fmt.Errorf("instant: invalid ISO 8601 instant string: %q", s)
				},
				Returns: []*engine.Type{engine.TInstant},
			}},
		},
		{
			Name:        "time-time-of-day",
			ForwardArgs: true,
			Signatures: []engine.NativeSig{{
				Args: []*engine.Type{engine.TString},
				Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
					s, err := args[0].AsConcreteString()
					if err != nil {
						return nil, err
					}
					for _, layout := range []string{"15:04:05.999999999", "15:04:05", "15:04"} {
						if t, e := time.Parse(layout, s); e == nil {
							d := time.Duration(t.Hour())*time.Hour +
								time.Duration(t.Minute())*time.Minute +
								time.Duration(t.Second())*time.Second +
								time.Duration(t.Nanosecond())
							return []engine.Value{engine.NewTimeOfDay(d)}, nil
						}
					}
					return nil, fmt.Errorf("time-of-day: invalid time string: %q", s)
				},
				Returns: []*engine.Type{engine.TTimeOfDay},
			}},
		},
		{
			Name:        "time-tz",
			ForwardArgs: true,
			Signatures: []engine.NativeSig{{
				Args: []*engine.Type{engine.TString},
				Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
					s, err := args[0].AsConcreteString()
					if err != nil {
						return nil, err
					}
					loc, err := time.LoadLocation(s)
					if err != nil {
						return nil, fmt.Errorf("tz: unknown timezone: %q", s)
					}
					return []engine.Value{engine.NewTimezone(loc)}, nil
				},
				Returns: []*engine.Type{engine.TTimezone},
			}},
		},
		{
			Name:        "time-unix",
			ForwardArgs: true,
			Signatures: []engine.NativeSig{{
				Args: []*engine.Type{engine.TInteger},
				Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
					n, err := args[0].AsConcreteInteger()
					if err != nil {
						return nil, err
					}
					return []engine.Value{engine.NewInstant(time.Unix(n, 0))}, nil
				},
				Returns: []*engine.Type{engine.TInstant},
			}},
		},
		{
			Name:        "time-unix-ms",
			ForwardArgs: true,
			Signatures: []engine.NativeSig{{
				Args: []*engine.Type{engine.TInteger},
				Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
					n, err := args[0].AsConcreteInteger()
					if err != nil {
						return nil, err
					}
					return []engine.Value{engine.NewInstant(time.UnixMilli(n))}, nil
				},
				Returns: []*engine.Type{engine.TInstant},
			}},
		},
		{
			Name:        "time-unix-ns",
			ForwardArgs: true,
			Signatures: []engine.NativeSig{{
				Args: []*engine.Type{engine.TInteger},
				Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
					n, err := args[0].AsConcreteInteger()
					if err != nil {
						return nil, err
					}
					return []engine.Value{engine.NewInstant(time.Unix(0, n))}, nil
				},
				Returns: []*engine.Type{engine.TInstant},
			}},
		},
		// --- Current time (stack-only zero-arg words) ---
		{
			Name:        "time-now-local",
			ForwardArgs: false,
			Signatures: []engine.NativeSig{{
				Args: []*engine.Type{},
				Handler: func(_ []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
					return []engine.Value{engine.NewDateTime(time.Now())}, nil
				},
				Returns: []*engine.Type{engine.TDateTime},
			}},
		},
		{
			Name:        "time-today",
			ForwardArgs: false,
			Signatures: []engine.NativeSig{{
				Args: []*engine.Type{},
				Handler: func(_ []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
					now := time.Now()
					d := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
					return []engine.Value{engine.NewDate(d)}, nil
				},
				Returns: []*engine.Type{engine.TDate},
			}},
		},
		{
			Name:        "time-today-utc",
			ForwardArgs: false,
			Signatures: []engine.NativeSig{{
				Args: []*engine.Type{},
				Handler: func(_ []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
					now := time.Now().UTC()
					d := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
					return []engine.Value{engine.NewDate(d)}, nil
				},
				Returns: []*engine.Type{engine.TDate},
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
			Name:        "leap-year?",
			ForwardArgs: true,
			Signatures: []engine.NativeSig{{
				Args: []*engine.Type{engine.TDate},
				Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
					y := extractTime(args[0]).Year()
					leap := y%4 == 0 && (y%100 != 0 || y%400 == 0)
					return []engine.Value{engine.NewBoolean(leap)}, nil
				},
				Returns: []*engine.Type{engine.TBoolean},
			}},
		},
		// to-unix / to-unix-ms (Instant -> Integer)
		{
			Name:        "to-unix",
			ForwardArgs: true,
			Signatures: []engine.NativeSig{{
				Args: []*engine.Type{engine.TInstant},
				Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
					return []engine.Value{engine.NewInteger(extractTime(args[0]).Unix())}, nil
				},
				Returns: []*engine.Type{engine.TInteger},
			}},
		},
		{
			Name:        "to-unix-ms",
			ForwardArgs: true,
			Signatures: []engine.NativeSig{{
				Args: []*engine.Type{engine.TInstant},
				Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
					return []engine.Value{engine.NewInteger(extractTime(args[0]).UnixMilli())}, nil
				},
				Returns: []*engine.Type{engine.TInteger},
			}},
		},
		// --- Comparison (Date Date -> Boolean) ---
		{
			Name:        "before?",
			ForwardArgs: true,
			Signatures: []engine.NativeSig{{
				Args: []*engine.Type{engine.TDate, engine.TDate},
				Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
					return []engine.Value{engine.NewBoolean(extractTime(args[1]).Before(extractTime(args[0])))}, nil
				},
				Returns: []*engine.Type{engine.TBoolean},
			}},
		},
		{
			Name:        "after?",
			ForwardArgs: true,
			Signatures: []engine.NativeSig{{
				Args: []*engine.Type{engine.TDate, engine.TDate},
				Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
					return []engine.Value{engine.NewBoolean(extractTime(args[1]).After(extractTime(args[0])))}, nil
				},
				Returns: []*engine.Type{engine.TBoolean},
			}},
		},
		{
			Name:        "equal?",
			ForwardArgs: true,
			Signatures: []engine.NativeSig{{
				Args: []*engine.Type{engine.TDate, engine.TDate},
				Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
					return []engine.Value{engine.NewBoolean(extractTime(args[0]).Equal(extractTime(args[1])))}, nil
				},
				Returns: []*engine.Type{engine.TBoolean},
			}},
		},
		// --- Formatting ---
		{
			Name:        "to-string",
			ForwardArgs: true,
			Signatures: []engine.NativeSig{{
				Args: []*engine.Type{engine.TDate},
				Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
					return []engine.Value{engine.NewString(extractTime(args[0]).Format("2006-01-02"))}, nil
				},
				Returns: []*engine.Type{engine.TString},
			}},
		},
		{
			Name:        "format",
			ForwardArgs: true,
			Signatures: []engine.NativeSig{{
				Args: []*engine.Type{engine.TString, engine.TDate},
				Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
					layout, err := args[0].AsConcreteString()
					if err != nil {
						return nil, err
					}
					return []engine.Value{engine.NewString(extractTime(args[1]).Format(layout))}, nil
				},
				Returns: []*engine.Type{engine.TString},
			}},
		},
		{
			Name:        "to-iso",
			ForwardArgs: true,
			Signatures: []engine.NativeSig{{
				Args: []*engine.Type{engine.TDate},
				Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
					return []engine.Value{engine.NewString(extractTime(args[0]).Format("2006-01-02"))}, nil
				},
				Returns: []*engine.Type{engine.TString},
			}},
		},
		// --- Legacy arithmetic ---
		addDateNative("add-days", func(n int) (int, int, int) { return 0, 0, n }),
		addDateNative("add-months", func(n int) (int, int, int) { return 0, n, 0 }),
		addDateNative("add-years", func(n int) (int, int, int) { return n, 0, 0 }),
		// --- Duration construction (Integer -> CalDuration) ---
		intToCalDurationNative("years", engine.TCalDuration, func(n int) (int, int, int) { return n, 0, 0 }),
		intToCalDurationNative("months", engine.TCalDuration, func(n int) (int, int, int) { return 0, n, 0 }),
		intToCalDurationNative("weeks", engine.TClkDuration, func(n int) (int, int, int) { return 0, 0, n * 7 }),
		intToCalDurationNative("days", engine.TClkDuration, func(n int) (int, int, int) { return 0, 0, n }),
		// --- Duration construction (Number -> ClkDuration) ---
		numToClkDurationNative("hours", time.Hour),
		numToClkDurationNative("minutes", time.Minute),
		numToClkDurationNative("seconds", time.Second),
		numToClkDurationNative("ms", time.Millisecond),
		numToClkDurationNative("us", time.Microsecond),
		numToClkDurationNative("ns", time.Nanosecond),
		{
			// cal-dur 1 6 15 → args[0]=15 (nearest), args[1]=6, args[2]=1 (deepest)
			Name:        "cal-dur",
			ForwardArgs: true,
			Signatures: []engine.NativeSig{{
				Args: []*engine.Type{engine.TInteger, engine.TInteger, engine.TInteger},
				Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
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
					return []engine.Value{engine.NewCalDuration(int(y), int(m), int(d))}, nil
				},
				Returns: []*engine.Type{engine.TCalDuration},
			}},
		},
		{
			Name:        "time-duration",
			ForwardArgs: true,
			Signatures: []engine.NativeSig{{
				Args: []*engine.Type{engine.TString},
				Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
					s, err := args[0].AsConcreteString()
					if err != nil {
						return nil, err
					}
					cd, clk, isCal, err := parseISO8601Duration(s)
					if err != nil {
						return nil, err
					}
					if isCal {
						return []engine.Value{engine.NewCalDuration(cd.Years, cd.Months, cd.Days)}, nil
					}
					return []engine.Value{engine.NewClkDuration(clk)}, nil
				},
				Returns: []*engine.Type{engine.TClkDuration},
			}},
		},
		// --- Duration extraction ---
		clkDurationToDecimalNative("total-hours", engine.TDecimal, func(d time.Duration) float64 { return d.Hours() }),
		clkDurationToDecimalNative("total-minutes", engine.TDecimal, func(d time.Duration) float64 { return d.Minutes() }),
		clkDurationToDecimalNative("total-seconds", engine.TDecimal, func(d time.Duration) float64 { return d.Seconds() }),
		// total-ms: handler pushes Decimal but historical Returns is TInteger.
		clkDurationToDecimalNative("total-ms", engine.TInteger, func(d time.Duration) float64 { return float64(d.Milliseconds()) }),
		calDurationToIntNative("dur-years", func(cd engine.CalDurationData) int64 { return int64(cd.Years) }),
		calDurationToIntNative("dur-months", func(cd engine.CalDurationData) int64 { return int64(cd.Months) }),
		calDurationToIntNative("dur-days", func(cd engine.CalDurationData) int64 { return int64(cd.Days) }),
		// dur-sign — two overloads (CalDuration and ClkDuration), both
		// return -1/0/1 integers. Historically two separate r.Register
		// calls; here unified into one NativeFunc with two signatures.
		{
			Name:        "dur-sign",
			ForwardArgs: true,
			Signatures: []engine.NativeSig{
				{
					Args: []*engine.Type{engine.TCalDuration},
					Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
						cd, _ := args[0].AsCalDuration()
						total := cd.Years*365 + cd.Months*30 + cd.Days
						switch {
						case total < 0:
							return []engine.Value{engine.NewInteger(-1)}, nil
						case total > 0:
							return []engine.Value{engine.NewInteger(1)}, nil
						default:
							return []engine.Value{engine.NewInteger(0)}, nil
						}
					},
					Returns: []*engine.Type{engine.TInteger},
				},
				{
					Args: []*engine.Type{engine.TClkDuration},
					Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
						d, _ := args[0].AsClkDuration()
						switch {
						case d < 0:
							return []engine.Value{engine.NewInteger(-1)}, nil
						case d > 0:
							return []engine.Value{engine.NewInteger(1)}, nil
						default:
							return []engine.Value{engine.NewInteger(0)}, nil
						}
					},
					Returns: []*engine.Type{engine.TInteger},
				},
			},
		},
		// --- Arithmetic ---
		{
			// d1 d2 until → args[0]=d2 (nearest), args[1]=d1
			Name:        "until",
			ForwardArgs: true,
			Signatures: []engine.NativeSig{{
				Args: []*engine.Type{engine.TDate, engine.TDate},
				Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
					from := extractTime(args[1])
					to := extractTime(args[0])
					cd := dateDiffCalDuration(from, to)
					return []engine.Value{engine.NewCalDuration(cd.Years, cd.Months, cd.Days)}, nil
				},
				Returns: []*engine.Type{engine.TClkDuration},
			}},
		},
		{
			// d1 d2 since → args[0]=d2 (nearest), args[1]=d1
			Name:        "since",
			ForwardArgs: true,
			Signatures: []engine.NativeSig{{
				Args: []*engine.Type{engine.TDate, engine.TDate},
				Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
					from := extractTime(args[0])
					to := extractTime(args[1])
					cd := dateDiffCalDuration(from, to)
					return []engine.Value{engine.NewCalDuration(cd.Years, cd.Months, cd.Days)}, nil
				},
				Returns: []*engine.Type{engine.TClkDuration},
			}},
		},
		{
			// ins1 ins2 diff → args[0]=ins2 (nearest), args[1]=ins1
			Name:        "diff",
			ForwardArgs: true,
			Signatures: []engine.NativeSig{{
				Args: []*engine.Type{engine.TInstant, engine.TInstant},
				Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
					t1 := extractTime(args[1])
					t2 := extractTime(args[0])
					return []engine.Value{engine.NewClkDuration(t2.Sub(t1))}, nil
				},
				Returns: []*engine.Type{engine.TClkDuration},
			}},
		},
		{
			Name:        "elapsed",
			ForwardArgs: true,
			Signatures: []engine.NativeSig{{
				Args: []*engine.Type{engine.TInstant},
				Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
					start := extractTime(args[0])
					return []engine.Value{engine.NewClkDuration(time.Since(start))}, nil
				},
				Returns: []*engine.Type{engine.TClkDuration},
			}},
		},
		// --- Comparison extended ---
		{
			Name:        "time-compare",
			ForwardArgs: true,
			Signatures: []engine.NativeSig{{
				Args: []*engine.Type{engine.TDate, engine.TDate},
				Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
					t1 := extractTime(args[1])
					t2 := extractTime(args[0])
					switch {
					case t1.Before(t2):
						return []engine.Value{engine.NewInteger(-1)}, nil
					case t1.After(t2):
						return []engine.Value{engine.NewInteger(1)}, nil
					default:
						return []engine.Value{engine.NewInteger(0)}, nil
					}
				},
				Returns: []*engine.Type{engine.TInteger},
			}},
		},
		{
			// d start end between? → args[0]=end (nearest), args[1]=start, args[2]=d (deepest)
			Name:        "between?",
			ForwardArgs: true,
			Signatures: []engine.NativeSig{{
				Args: []*engine.Type{engine.TDate, engine.TDate, engine.TDate},
				Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
					d := extractTime(args[2])
					start := extractTime(args[1])
					end := extractTime(args[0])
					return []engine.Value{engine.NewBoolean(!d.Before(start) && !d.After(end))}, nil
				},
				Returns: []*engine.Type{engine.TBoolean},
			}},
		},
		{
			Name:        "earliest",
			ForwardArgs: true,
			Signatures: []engine.NativeSig{{
				Args: []*engine.Type{engine.TDate, engine.TDate},
				Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
					t1 := extractTime(args[1])
					t2 := extractTime(args[0])
					if t1.Before(t2) {
						return []engine.Value{engine.NewDate(t1)}, nil
					}
					return []engine.Value{engine.NewDate(t2)}, nil
				},
				Returns: []*engine.Type{engine.TAny},
			}},
		},
		{
			Name:        "latest",
			ForwardArgs: true,
			Signatures: []engine.NativeSig{{
				Args: []*engine.Type{engine.TDate, engine.TDate},
				Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
					t1 := extractTime(args[1])
					t2 := extractTime(args[0])
					if t1.After(t2) {
						return []engine.Value{engine.NewDate(t1)}, nil
					}
					return []engine.Value{engine.NewDate(t2)}, nil
				},
				Returns: []*engine.Type{engine.TAny},
			}},
		},
		// --- Conversion ---
		// to-date — DateTime overload + Instant overload (UTC).
		{
			Name:        "to-date",
			ForwardArgs: true,
			Signatures: []engine.NativeSig{
				{
					Args: []*engine.Type{engine.TDateTime},
					Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
						t := extractTime(args[0])
						return []engine.Value{engine.NewDate(time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location()))}, nil
					},
					Returns: []*engine.Type{engine.TDate},
				},
				{
					Args: []*engine.Type{engine.TInstant},
					Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
						t := extractTime(args[0])
						return []engine.Value{engine.NewDate(time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC))}, nil
					},
					Returns: []*engine.Type{engine.TDate},
				},
			},
		},
		// to-time-of-day — DateTime + Instant overloads (identical body).
		{
			Name:        "to-time-of-day",
			ForwardArgs: true,
			Signatures: []engine.NativeSig{
				{
					Args: []*engine.Type{engine.TDateTime},
					Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
						t := extractTime(args[0])
						d := time.Duration(t.Hour())*time.Hour + time.Duration(t.Minute())*time.Minute +
							time.Duration(t.Second())*time.Second + time.Duration(t.Nanosecond())
						return []engine.Value{engine.NewTimeOfDay(d)}, nil
					},
					Returns: []*engine.Type{engine.TTimeOfDay},
				},
				{
					Args: []*engine.Type{engine.TInstant},
					Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
						t := extractTime(args[0])
						d := time.Duration(t.Hour())*time.Hour + time.Duration(t.Minute())*time.Minute +
							time.Duration(t.Second())*time.Second + time.Duration(t.Nanosecond())
						return []engine.Value{engine.NewTimeOfDay(d)}, nil
					},
					Returns: []*engine.Type{engine.TTimeOfDay},
				},
			},
		},
		{
			Name:        "to-datetime",
			ForwardArgs: true,
			Signatures: []engine.NativeSig{{
				Args: []*engine.Type{engine.TDate},
				Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
					t := extractTime(args[0])
					return []engine.Value{engine.NewDateTime(t)}, nil
				},
				Returns: []*engine.Type{engine.TDateTime},
			}},
		},
		{
			// dt tz to-instant → args[0]=tz (nearest), args[1]=dt
			Name:        "to-instant",
			ForwardArgs: true,
			Signatures: []engine.NativeSig{{
				Args: []*engine.Type{engine.TTimezone, engine.TDateTime},
				Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
					dt := extractTime(args[1])
					loc := args[0].AsTimezone()
					if loc == nil {
						loc = time.UTC
					}
					t := time.Date(dt.Year(), dt.Month(), dt.Day(), dt.Hour(), dt.Minute(), dt.Second(), dt.Nanosecond(), loc)
					return []engine.Value{engine.NewInstant(t)}, nil
				},
				Returns: []*engine.Type{engine.TInstant},
			}},
		},
		{
			// ins tz to-local → args[0]=tz (nearest), args[1]=ins
			Name:        "to-local",
			ForwardArgs: true,
			Signatures: []engine.NativeSig{{
				Args: []*engine.Type{engine.TTimezone, engine.TInstant},
				Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
					t := extractTime(args[1])
					loc := args[0].AsTimezone()
					if loc == nil {
						loc = time.UTC
					}
					return []engine.Value{engine.NewDateTime(t.In(loc))}, nil
				},
				Returns: []*engine.Type{engine.TDateTime},
			}},
		},
		{
			Name:        "to-utc",
			ForwardArgs: true,
			Signatures: []engine.NativeSig{{
				Args: []*engine.Type{engine.TInstant},
				Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
					t := extractTime(args[0])
					return []engine.Value{engine.NewDateTime(t.UTC())}, nil
				},
				Returns: []*engine.Type{engine.TDateTime},
			}},
		},
		// --- Rounding ---
		{
			Name:        "start-of",
			ForwardArgs: true,
			Signatures: []engine.NativeSig{{
				Args: []*engine.Type{engine.TString, engine.TDate},
				Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
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
					return []engine.Value{engine.NewDate(result)}, nil
				},
				Returns: []*engine.Type{engine.TAny},
			}},
		},
		{
			Name:        "end-of",
			ForwardArgs: true,
			Signatures: []engine.NativeSig{{
				Args: []*engine.Type{engine.TString, engine.TDate},
				Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
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
					return []engine.Value{engine.NewDate(result)}, nil
				},
				Returns: []*engine.Type{engine.TAny},
			}},
		},
		// --- Timezone ---
		{
			Name:        "tz-utc",
			ForwardArgs: false,
			Signatures: []engine.NativeSig{{
				Args: []*engine.Type{},
				Handler: func(_ []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
					return []engine.Value{engine.NewTimezone(time.UTC)}, nil
				},
				Returns: []*engine.Type{engine.TTimezone},
			}},
		},
		{
			Name:        "tz-local",
			ForwardArgs: false,
			Signatures: []engine.NativeSig{{
				Args: []*engine.Type{},
				Handler: func(_ []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
					return []engine.Value{engine.NewTimezone(time.Local)}, nil
				},
				Returns: []*engine.Type{engine.TTimezone},
			}},
		},
		{
			Name:        "tz-name",
			ForwardArgs: true,
			Signatures: []engine.NativeSig{{
				Args: []*engine.Type{engine.TTimezone},
				Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
					loc := args[0].AsTimezone()
					if loc == nil {
						return []engine.Value{engine.NewString("UTC")}, nil
					}
					return []engine.Value{engine.NewString(loc.String())}, nil
				},
				Returns: []*engine.Type{engine.TString},
			}},
		},
		{
			// ins tz tz-offset → args[0]=tz (nearest), args[1]=ins
			Name:        "tz-offset",
			ForwardArgs: true,
			Signatures: []engine.NativeSig{{
				Args: []*engine.Type{engine.TTimezone, engine.TInstant},
				Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
					t := extractTime(args[1])
					loc := args[0].AsTimezone()
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
					return []engine.Value{engine.NewString(fmt.Sprintf("%s%02d:%02d", sign, h, m))}, nil
				},
				Returns: []*engine.Type{engine.TClkDuration},
			}},
		},
		{
			// ins tz dst? → args[0]=tz (nearest), args[1]=ins
			Name:        "dst?",
			ForwardArgs: true,
			Signatures: []engine.NativeSig{{
				Args: []*engine.Type{engine.TTimezone, engine.TInstant},
				Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
					t := extractTime(args[1])
					loc := args[0].AsTimezone()
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
						return []engine.Value{engine.NewBoolean(false)}, nil
					}
					stdOff := janOff
					if julOff < janOff {
						stdOff = julOff // Southern hemisphere
					}
					return []engine.Value{engine.NewBoolean(curOff != stdOff)}, nil
				},
				Returns: []*engine.Type{engine.TBoolean},
			}},
		},
		// --- Parsing ---
		{
			// "15/03/2024" "02/01/2006" parse-date → args[0]=layout (nearest), args[1]=str
			Name:        "parse-date",
			ForwardArgs: true,
			Signatures: []engine.NativeSig{{
				Args: []*engine.Type{engine.TString, engine.TString},
				Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
					s, err := args[1].AsConcreteString()
					if err != nil {
						return nil, err
					}
					layout, err := args[0].AsConcreteString()
					if err != nil {
						return nil, err
					}
					t, err := time.Parse(layout, s)
					if err != nil {
						return nil, fmt.Errorf("parse-date: %w", err)
					}
					d := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
					return []engine.Value{engine.NewDate(d)}, nil
				},
				Returns: []*engine.Type{engine.TDate},
			}},
		},
		{
			Name:        "parse-datetime",
			ForwardArgs: true,
			Signatures: []engine.NativeSig{{
				Args: []*engine.Type{engine.TString, engine.TString},
				Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
					s, err := args[1].AsConcreteString()
					if err != nil {
						return nil, err
					}
					layout, err := args[0].AsConcreteString()
					if err != nil {
						return nil, err
					}
					t, err := time.Parse(layout, s)
					if err != nil {
						return nil, fmt.Errorf("parse-datetime: %w", err)
					}
					return []engine.Value{engine.NewDateTime(t)}, nil
				},
				Returns: []*engine.Type{engine.TDateTime},
			}},
		},
		{
			Name:        "auto-date",
			ForwardArgs: true,
			Signatures: []engine.NativeSig{{
				Args: []*engine.Type{engine.TString},
				Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
					s, err := args[0].AsConcreteString()
					if err != nil {
						return nil, err
					}
					for _, layout := range autoDateLayouts {
						if t, e := time.Parse(layout, s); e == nil {
							d := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
							return []engine.Value{engine.NewDate(d)}, nil
						}
					}
					return nil, fmt.Errorf("auto-date: unable to parse %q", s)
				},
				Returns: []*engine.Type{engine.TDateTime},
			}},
		},
	}

	return out
}()
