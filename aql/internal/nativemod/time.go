package nativemod

import (
	"fmt"
	"strings"
	"time"

	"github.com/metsitaba/voxgig-exp/aql/internal/engine"
)

// BuildTimeModule creates the "aql:time" native module.
func BuildTimeModule(parent *engine.Registry) (engine.ModuleDesc, error) {
	subReg, err := engine.DefaultRegistry()
	if err != nil {
		return engine.ModuleDesc{}, err
	}

	registerAllTimeWords(subReg)

	exports := engine.NewOrderedMap()

	// Construction
	exports.Set("date", makeTimeFnDef("time-date", []engine.FnParam{{Type: engine.TString}}, []engine.Type{engine.TDate}, subReg))
	exports.Set("datetime", makeTimeFnDef("time-datetime", []engine.FnParam{{Type: engine.TString}}, []engine.Type{engine.TDateTime}, subReg))
	exports.Set("instant", makeTimeFnDef("time-instant", []engine.FnParam{{Type: engine.TString}}, []engine.Type{engine.TInstant}, subReg))
	exports.Set("time-of-day", makeTimeFnDef("time-time-of-day", []engine.FnParam{{Type: engine.TString}}, []engine.Type{engine.TTimeOfDay}, subReg))
	exports.Set("tz", makeTimeFnDef("time-tz", []engine.FnParam{{Type: engine.TString}}, []engine.Type{engine.TTimezone}, subReg))
	exports.Set("unix", makeTimeFnDef("time-unix", []engine.FnParam{{Type: engine.TInteger}}, []engine.Type{engine.TInstant}, subReg))
	exports.Set("unix-ms", makeTimeFnDef("time-unix-ms", []engine.FnParam{{Type: engine.TInteger}}, []engine.Type{engine.TInstant}, subReg))
	exports.Set("unix-ns", makeTimeFnDef("time-unix-ns", []engine.FnParam{{Type: engine.TInteger}}, []engine.Type{engine.TInstant}, subReg))

	// Current time
	exports.Set("now-local", makeTimeFnDef("time-now-local", []engine.FnParam{}, []engine.Type{engine.TDateTime}, subReg))
	exports.Set("today", makeTimeFnDef("time-today", []engine.FnParam{}, []engine.Type{engine.TDate}, subReg))
	exports.Set("today-utc", makeTimeFnDef("time-today-utc", []engine.FnParam{}, []engine.Type{engine.TDate}, subReg))

	// Extraction (Date -> Integer)
	for _, name := range []string{"year", "month", "day", "weekday", "year-day", "iso-week", "quarter", "days-in-month", "days-in-year"} {
		exports.Set(name, makeTimeFnDef(name, []engine.FnParam{{Type: engine.TDate}}, []engine.Type{engine.TInteger}, subReg))
	}
	for _, name := range []string{"weekday-name", "month-name"} {
		exports.Set(name, makeTimeFnDef(name, []engine.FnParam{{Type: engine.TDate}}, []engine.Type{engine.TString}, subReg))
	}
	exports.Set("leap-year?", makeTimeFnDef("leap-year?", []engine.FnParam{{Type: engine.TDate}}, []engine.Type{engine.TBoolean}, subReg))

	// Extraction from Instant
	exports.Set("to-unix", makeTimeFnDef("to-unix", []engine.FnParam{{Type: engine.TInstant}}, []engine.Type{engine.TInteger}, subReg))
	exports.Set("to-unix-ms", makeTimeFnDef("to-unix-ms", []engine.FnParam{{Type: engine.TInstant}}, []engine.Type{engine.TInteger}, subReg))

	// Comparison (Date Date -> Boolean)
	for _, name := range []string{"before?", "after?", "equal?"} {
		exports.Set(name, makeTimeFnDef(name, []engine.FnParam{{Type: engine.TDate}, {Type: engine.TDate}}, []engine.Type{engine.TBoolean}, subReg))
	}

	// Formatting
	exports.Set("to-string", makeTimeFnDef("to-string", []engine.FnParam{{Type: engine.TDate}}, []engine.Type{engine.TString}, subReg))
	exports.Set("format", makeTimeFnDef("format", []engine.FnParam{{Type: engine.TDate}, {Type: engine.TString}}, []engine.Type{engine.TString}, subReg))
	exports.Set("to-iso", makeTimeFnDef("to-iso", []engine.FnParam{{Type: engine.TDate}}, []engine.Type{engine.TString}, subReg))

	// Duration construction
	for _, name := range []string{"years", "months", "weeks", "days"} {
		exports.Set(name, makeTimeFnDef(name, []engine.FnParam{{Type: engine.TInteger}}, []engine.Type{engine.TCalDuration}, subReg))
	}
	for _, name := range []string{"hours", "minutes", "seconds", "ms", "us", "ns"} {
		exports.Set(name, makeTimeFnDef(name, []engine.FnParam{{Type: engine.TNumber}}, []engine.Type{engine.TClkDuration}, subReg))
	}
	exports.Set("cal-dur", makeTimeFnDef("cal-dur", []engine.FnParam{{Type: engine.TInteger}, {Type: engine.TInteger}, {Type: engine.TInteger}}, []engine.Type{engine.TCalDuration}, subReg))
	exports.Set("duration", makeTimeFnDef("time-duration", []engine.FnParam{{Type: engine.TString}}, []engine.Type{engine.TCalDuration}, subReg))

	// Duration extraction
	for _, name := range []string{"total-hours", "total-minutes", "total-seconds", "total-ms"} {
		exports.Set(name, makeTimeFnDef(name, []engine.FnParam{{Type: engine.TClkDuration}}, []engine.Type{engine.TDecimal}, subReg))
	}
	for _, name := range []string{"dur-years", "dur-months", "dur-days"} {
		exports.Set(name, makeTimeFnDef(name, []engine.FnParam{{Type: engine.TCalDuration}}, []engine.Type{engine.TInteger}, subReg))
	}
	exports.Set("dur-sign", makeTimeFnDef("dur-sign", []engine.FnParam{{Type: engine.TCalDuration}}, []engine.Type{engine.TInteger}, subReg))

	// Arithmetic
	exports.Set("until", makeTimeFnDef("until", []engine.FnParam{{Type: engine.TDate}, {Type: engine.TDate}}, []engine.Type{engine.TCalDuration}, subReg))
	exports.Set("since", makeTimeFnDef("since", []engine.FnParam{{Type: engine.TDate}, {Type: engine.TDate}}, []engine.Type{engine.TCalDuration}, subReg))
	exports.Set("diff", makeTimeFnDef("diff", []engine.FnParam{{Type: engine.TInstant}, {Type: engine.TInstant}}, []engine.Type{engine.TClkDuration}, subReg))
	exports.Set("elapsed", makeTimeFnDef("elapsed", []engine.FnParam{{Type: engine.TInstant}}, []engine.Type{engine.TClkDuration}, subReg))

	// Comparison extended
	exports.Set("compare", makeTimeFnDef("time-compare", []engine.FnParam{{Type: engine.TDate}, {Type: engine.TDate}}, []engine.Type{engine.TInteger}, subReg))
	exports.Set("between?", makeTimeFnDef("between?", []engine.FnParam{{Type: engine.TDate}, {Type: engine.TDate}, {Type: engine.TDate}}, []engine.Type{engine.TBoolean}, subReg))
	exports.Set("earliest", makeTimeFnDef("earliest", []engine.FnParam{{Type: engine.TDate}, {Type: engine.TDate}}, []engine.Type{engine.TDate}, subReg))
	exports.Set("latest", makeTimeFnDef("latest", []engine.FnParam{{Type: engine.TDate}, {Type: engine.TDate}}, []engine.Type{engine.TDate}, subReg))

	// Conversion
	exports.Set("to-date", makeTimeFnDef("to-date", []engine.FnParam{{Type: engine.TDateTime}}, []engine.Type{engine.TDate}, subReg))
	exports.Set("to-time-of-day", makeTimeFnDef("to-time-of-day", []engine.FnParam{{Type: engine.TDateTime}}, []engine.Type{engine.TTimeOfDay}, subReg))
	exports.Set("to-datetime", makeTimeFnDef("to-datetime", []engine.FnParam{{Type: engine.TDate}}, []engine.Type{engine.TDateTime}, subReg))
	exports.Set("to-instant", makeTimeFnDef("to-instant", []engine.FnParam{{Type: engine.TDateTime}, {Type: engine.TTimezone}}, []engine.Type{engine.TInstant}, subReg))
	exports.Set("to-local", makeTimeFnDef("to-local", []engine.FnParam{{Type: engine.TInstant}, {Type: engine.TTimezone}}, []engine.Type{engine.TDateTime}, subReg))
	exports.Set("to-utc", makeTimeFnDef("to-utc", []engine.FnParam{{Type: engine.TInstant}}, []engine.Type{engine.TDateTime}, subReg))

	// Rounding
	exports.Set("start-of", makeTimeFnDef("start-of", []engine.FnParam{{Type: engine.TDate}, {Type: engine.TString}}, []engine.Type{engine.TDate}, subReg))
	exports.Set("end-of", makeTimeFnDef("end-of", []engine.FnParam{{Type: engine.TDate}, {Type: engine.TString}}, []engine.Type{engine.TDate}, subReg))

	// Timezone
	exports.Set("tz-utc", makeTimeFnDef("tz-utc", []engine.FnParam{}, []engine.Type{engine.TTimezone}, subReg))
	exports.Set("tz-local", makeTimeFnDef("tz-local", []engine.FnParam{}, []engine.Type{engine.TTimezone}, subReg))
	exports.Set("tz-name", makeTimeFnDef("tz-name", []engine.FnParam{{Type: engine.TTimezone}}, []engine.Type{engine.TString}, subReg))
	exports.Set("tz-offset", makeTimeFnDef("tz-offset", []engine.FnParam{{Type: engine.TInstant}, {Type: engine.TTimezone}}, []engine.Type{engine.TString}, subReg))
	exports.Set("dst?", makeTimeFnDef("dst?", []engine.FnParam{{Type: engine.TInstant}, {Type: engine.TTimezone}}, []engine.Type{engine.TBoolean}, subReg))

	// Parsing
	exports.Set("parse-date", makeTimeFnDef("parse-date", []engine.FnParam{{Type: engine.TString}, {Type: engine.TString}}, []engine.Type{engine.TDate}, subReg))
	exports.Set("parse-datetime", makeTimeFnDef("parse-datetime", []engine.FnParam{{Type: engine.TString}, {Type: engine.TString}}, []engine.Type{engine.TDateTime}, subReg))
	exports.Set("auto-date", makeTimeFnDef("auto-date", []engine.FnParam{{Type: engine.TString}}, []engine.Type{engine.TDate}, subReg))

	// Legacy arithmetic (Date Integer -> Date)
	for _, name := range []string{"add-days", "add-months", "add-years"} {
		exports.Set(name, makeTimeFnDef(name, []engine.FnParam{{Type: engine.TDate}, {Type: engine.TInteger}}, []engine.Type{engine.TDate}, subReg))
	}

	modID := parent.NextModuleID()
	desc := engine.ModuleDesc{
		ID:      modID,
		Exports: map[string]*engine.OrderedMap{"time": exports},
	}
	return desc, nil
}

// makeTimeFnDef creates a FnDef value with the given params, returns, and word name.
func makeTimeFnDef(wordName string, params []engine.FnParam, returns []engine.Type, subReg *engine.Registry) engine.Value {
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

func registerAllTimeWords(r *engine.Registry) {
	// Construction
	registerTimeDate(r)
	registerTimeDateTime(r)
	registerTimeInstant(r)
	registerTimeTimeOfDay(r)
	registerTimeTz(r)
	registerTimeUnix(r)
	registerTimeUnixMs(r)
	registerTimeUnixNs(r)

	// Current time
	registerTimeNowLocal(r)
	registerTimeToday(r)
	registerTimeTodayUtc(r)

	// Extraction
	registerYear(r)
	registerMonth(r)
	registerDay(r)
	registerWeekday(r)
	registerYearDay(r)
	registerWeekdayName(r)
	registerMonthName(r)
	registerIsoWeek(r)
	registerQuarter(r)
	registerDaysInMonth(r)
	registerDaysInYear(r)
	registerLeapYear(r)
	registerToUnix(r)
	registerToUnixMs(r)

	// Comparison
	registerBefore(r)
	registerAfter(r)
	registerEqual(r)

	// Formatting
	registerToString(r)
	registerFormat(r)
	registerToIso(r)

	// Duration construction
	registerDurYears(r)
	registerDurMonths(r)
	registerDurWeeks(r)
	registerDurDays(r)
	registerDurHours(r)
	registerDurMinutes(r)
	registerDurSeconds(r)
	registerDurMs(r)
	registerDurUs(r)
	registerDurNs(r)
	registerCalDur(r)
	registerTimeDuration(r)

	// Duration extraction
	registerTotalHours(r)
	registerTotalMinutes(r)
	registerTotalSeconds(r)
	registerTotalMs(r)
	registerDurYearsExtract(r)
	registerDurMonthsExtract(r)
	registerDurDaysExtract(r)
	registerDurSign(r)

	// Arithmetic
	registerUntil(r)
	registerSince(r)
	registerDiff(r)
	registerElapsed(r)

	// Comparison extended
	registerTimeCompare(r)
	registerBetween(r)
	registerEarliest(r)
	registerLatest(r)

	// Conversion
	registerToDate(r)
	registerToTimeOfDay(r)
	registerToDateTime(r)
	registerToInstant(r)
	registerToLocal(r)
	registerToUtc(r)

	// Rounding
	registerStartOf(r)
	registerEndOf(r)

	// Timezone
	registerTzUtc(r)
	registerTzLocal(r)
	registerTzName(r)
	registerTzOffset(r)
	registerDst(r)

	// Parsing
	registerParseDate(r)
	registerParseDatetime(r)
	registerAutoDate(r)

	// Legacy arithmetic
	registerAddDays(r)
	registerAddMonths(r)
	registerAddYears(r)
}

// --- helpers ---

// extractTime returns the time.Time from a Date, DateTime, or Instant value.
func extractTime(v engine.Value) time.Time {
	if t, ok := v.Data.(time.Time); ok {
		return t
	}
	return time.Time{}
}

// --- Construction ---

func registerTimeDate(r *engine.Registry) {
	r.Register("time-date", engine.Signature{
		Args: []engine.Type{engine.TString},
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
		Returns: []engine.Type{engine.TDate},
	})
}

func registerTimeDateTime(r *engine.Registry) {
	r.Register("time-datetime", engine.Signature{
		Args: []engine.Type{engine.TString},
		Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
			s, err := args[0].AsConcreteString()
			if err != nil {
				return nil, err
			}
			// Try with and without fractional seconds.
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
		Returns: []engine.Type{engine.TDateTime},
	})
}

func registerTimeInstant(r *engine.Registry) {
	r.Register("time-instant", engine.Signature{
		Args: []engine.Type{engine.TString},
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
		Returns: []engine.Type{engine.TInstant},
	})
}

func registerTimeTimeOfDay(r *engine.Registry) {
	r.Register("time-time-of-day", engine.Signature{
		Args: []engine.Type{engine.TString},
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
		Returns: []engine.Type{engine.TTimeOfDay},
	})
}

func registerTimeTz(r *engine.Registry) {
	r.Register("time-tz", engine.Signature{
		Args: []engine.Type{engine.TString},
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
		Returns: []engine.Type{engine.TTimezone},
	})
}

func registerTimeUnix(r *engine.Registry) {
	r.Register("time-unix", engine.Signature{
		Args: []engine.Type{engine.TInteger},
		Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
			n, err := args[0].AsConcreteInteger()
			if err != nil {
				return nil, err
			}
			return []engine.Value{engine.NewInstant(time.Unix(n, 0))}, nil
		},
		Returns: []engine.Type{engine.TInstant},
	})
}

func registerTimeUnixMs(r *engine.Registry) {
	r.Register("time-unix-ms", engine.Signature{
		Args: []engine.Type{engine.TInteger},
		Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
			n, err := args[0].AsConcreteInteger()
			if err != nil {
				return nil, err
			}
			return []engine.Value{engine.NewInstant(time.UnixMilli(n))}, nil
		},
		Returns: []engine.Type{engine.TInstant},
	})
}

func registerTimeUnixNs(r *engine.Registry) {
	r.Register("time-unix-ns", engine.Signature{
		Args: []engine.Type{engine.TInteger},
		Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
			n, err := args[0].AsConcreteInteger()
			if err != nil {
				return nil, err
			}
			t := time.Unix(0, n)
			return []engine.Value{engine.NewInstant(t)}, nil
		},
		Returns: []engine.Type{engine.TInstant},
	})
}

// --- Current Time ---

func registerTimeNowLocal(r *engine.Registry) {
	r.RegisterStackOnly("time-now-local", engine.Signature{
		Args: []engine.Type{},
		Handler: func(_ []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
			return []engine.Value{engine.NewDateTime(time.Now())}, nil
		},
		Returns: []engine.Type{engine.TDateTime},
	})
}

func registerTimeToday(r *engine.Registry) {
	r.RegisterStackOnly("time-today", engine.Signature{
		Args: []engine.Type{},
		Handler: func(_ []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
			now := time.Now()
			d := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
			return []engine.Value{engine.NewDate(d)}, nil
		},
		Returns: []engine.Type{engine.TDate},
	})
}

func registerTimeTodayUtc(r *engine.Registry) {
	r.RegisterStackOnly("time-today-utc", engine.Signature{
		Args: []engine.Type{},
		Handler: func(_ []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
			now := time.Now().UTC()
			d := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
			return []engine.Value{engine.NewDate(d)}, nil
		},
		Returns: []engine.Type{engine.TDate},
	})
}

// --- Extraction ---

func registerYear(r *engine.Registry) {
	r.Register("year", engine.Signature{
		Args: []engine.Type{engine.TDate},
		Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
			return []engine.Value{engine.NewInteger(int64(extractTime(args[0]).Year()))}, nil
		},
		Returns: []engine.Type{engine.TInteger},
	})
}

func registerMonth(r *engine.Registry) {
	r.Register("month", engine.Signature{
		Args: []engine.Type{engine.TDate},
		Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
			return []engine.Value{engine.NewInteger(int64(extractTime(args[0]).Month()))}, nil
		},
		Returns: []engine.Type{engine.TInteger},
	})
}

func registerDay(r *engine.Registry) {
	r.Register("day", engine.Signature{
		Args: []engine.Type{engine.TDate},
		Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
			return []engine.Value{engine.NewInteger(int64(extractTime(args[0]).Day()))}, nil
		},
		Returns: []engine.Type{engine.TInteger},
	})
}

func registerWeekday(r *engine.Registry) {
	r.Register("weekday", engine.Signature{
		Args: []engine.Type{engine.TDate},
		Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
			wd := extractTime(args[0]).Weekday()
			iso := int64(wd)
			if wd == time.Sunday {
				iso = 7
			}
			return []engine.Value{engine.NewInteger(iso)}, nil
		},
		Returns: []engine.Type{engine.TInteger},
	})
}

func registerYearDay(r *engine.Registry) {
	r.Register("year-day", engine.Signature{
		Args: []engine.Type{engine.TDate},
		Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
			return []engine.Value{engine.NewInteger(int64(extractTime(args[0]).YearDay()))}, nil
		},
		Returns: []engine.Type{engine.TInteger},
	})
}

func registerWeekdayName(r *engine.Registry) {
	r.Register("weekday-name", engine.Signature{
		Args: []engine.Type{engine.TDate},
		Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
			return []engine.Value{engine.NewString(extractTime(args[0]).Weekday().String())}, nil
		},
		Returns: []engine.Type{engine.TString},
	})
}

func registerMonthName(r *engine.Registry) {
	r.Register("month-name", engine.Signature{
		Args: []engine.Type{engine.TDate},
		Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
			return []engine.Value{engine.NewString(extractTime(args[0]).Month().String())}, nil
		},
		Returns: []engine.Type{engine.TString},
	})
}

func registerIsoWeek(r *engine.Registry) {
	r.Register("iso-week", engine.Signature{
		Args: []engine.Type{engine.TDate},
		Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
			_, week := extractTime(args[0]).ISOWeek()
			return []engine.Value{engine.NewInteger(int64(week))}, nil
		},
		Returns: []engine.Type{engine.TInteger},
	})
}

func registerQuarter(r *engine.Registry) {
	r.Register("quarter", engine.Signature{
		Args: []engine.Type{engine.TDate},
		Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
			m := extractTime(args[0]).Month()
			q := (int(m) + 2) / 3
			return []engine.Value{engine.NewInteger(int64(q))}, nil
		},
		Returns: []engine.Type{engine.TInteger},
	})
}

func registerDaysInMonth(r *engine.Registry) {
	r.Register("days-in-month", engine.Signature{
		Args: []engine.Type{engine.TDate},
		Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
			t := extractTime(args[0])
			last := time.Date(t.Year(), t.Month()+1, 0, 0, 0, 0, 0, t.Location())
			return []engine.Value{engine.NewInteger(int64(last.Day()))}, nil
		},
		Returns: []engine.Type{engine.TInteger},
	})
}

func registerDaysInYear(r *engine.Registry) {
	r.Register("days-in-year", engine.Signature{
		Args: []engine.Type{engine.TDate},
		Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
			y := extractTime(args[0]).Year()
			start := time.Date(y, 1, 1, 0, 0, 0, 0, time.UTC)
			end := time.Date(y+1, 1, 1, 0, 0, 0, 0, time.UTC)
			days := int64(end.Sub(start).Hours() / 24)
			return []engine.Value{engine.NewInteger(days)}, nil
		},
		Returns: []engine.Type{engine.TInteger},
	})
}

func registerLeapYear(r *engine.Registry) {
	r.Register("leap-year?", engine.Signature{
		Args: []engine.Type{engine.TDate},
		Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
			y := extractTime(args[0]).Year()
			leap := y%4 == 0 && (y%100 != 0 || y%400 == 0)
			return []engine.Value{engine.NewBoolean(leap)}, nil
		},
		Returns: []engine.Type{engine.TBoolean},
	})
}

func registerToUnix(r *engine.Registry) {
	r.Register("to-unix", engine.Signature{
		Args: []engine.Type{engine.TInstant},
		Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
			return []engine.Value{engine.NewInteger(extractTime(args[0]).Unix())}, nil
		},
		Returns: []engine.Type{engine.TInteger},
	})
}

func registerToUnixMs(r *engine.Registry) {
	r.Register("to-unix-ms", engine.Signature{
		Args: []engine.Type{engine.TInstant},
		Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
			return []engine.Value{engine.NewInteger(extractTime(args[0]).UnixMilli())}, nil
		},
		Returns: []engine.Type{engine.TInteger},
	})
}

// --- Comparison ---

func registerBefore(r *engine.Registry) {
	r.Register("before?", engine.Signature{
		Args: []engine.Type{engine.TDate, engine.TDate},
		Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
			return []engine.Value{engine.NewBoolean(extractTime(args[1]).Before(extractTime(args[0])))}, nil
		},
		Returns: []engine.Type{engine.TBoolean},
	})
}

func registerAfter(r *engine.Registry) {
	r.Register("after?", engine.Signature{
		Args: []engine.Type{engine.TDate, engine.TDate},
		Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
			return []engine.Value{engine.NewBoolean(extractTime(args[1]).After(extractTime(args[0])))}, nil
		},
		Returns: []engine.Type{engine.TBoolean},
	})
}

func registerEqual(r *engine.Registry) {
	r.Register("equal?", engine.Signature{
		Args: []engine.Type{engine.TDate, engine.TDate},
		Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
			return []engine.Value{engine.NewBoolean(extractTime(args[0]).Equal(extractTime(args[1])))}, nil
		},
		Returns: []engine.Type{engine.TBoolean},
	})
}

// --- Formatting ---

func registerToString(r *engine.Registry) {
	r.Register("to-string", engine.Signature{
		Args: []engine.Type{engine.TDate},
		Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
			return []engine.Value{engine.NewString(extractTime(args[0]).Format("2006-01-02"))}, nil
		},
		Returns: []engine.Type{engine.TString},
	})
}

func registerFormat(r *engine.Registry) {
	r.Register("format", engine.Signature{
		Args: []engine.Type{engine.TDate, engine.TString},
		Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
			layout, err := args[1].AsConcreteString()
			if err != nil {
				return nil, err
			}
			return []engine.Value{engine.NewString(extractTime(args[0]).Format(layout))}, nil
		},
		Returns: []engine.Type{engine.TString},
	})
}

func registerToIso(r *engine.Registry) {
	r.Register("to-iso", engine.Signature{
		Args: []engine.Type{engine.TDate},
		Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
			return []engine.Value{engine.NewString(extractTime(args[0]).Format("2006-01-02"))}, nil
		},
		Returns: []engine.Type{engine.TString},
	})
}

// --- Legacy Arithmetic ---

func registerAddDays(r *engine.Registry) {
	r.Register("add-days", engine.Signature{
		Args: []engine.Type{engine.TDate, engine.TInteger},
		Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
			t := extractTime(args[0])
			n, err := args[1].AsConcreteInteger()
			if err != nil {
				return nil, err
			}
			return []engine.Value{engine.NewDate(t.AddDate(0, 0, int(n)))}, nil
		},
		Returns: []engine.Type{engine.TAny},
	})
}

func registerAddMonths(r *engine.Registry) {
	r.Register("add-months", engine.Signature{
		Args: []engine.Type{engine.TDate, engine.TInteger},
		Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
			t := extractTime(args[0])
			n, err := args[1].AsConcreteInteger()
			if err != nil {
				return nil, err
			}
			return []engine.Value{engine.NewDate(t.AddDate(0, int(n), 0))}, nil
		},
		Returns: []engine.Type{engine.TAny},
	})
}

func registerAddYears(r *engine.Registry) {
	r.Register("add-years", engine.Signature{
		Args: []engine.Type{engine.TDate, engine.TInteger},
		Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
			t := extractTime(args[0])
			n, err := args[1].AsConcreteInteger()
			if err != nil {
				return nil, err
			}
			return []engine.Value{engine.NewDate(t.AddDate(int(n), 0, 0))}, nil
		},
		Returns: []engine.Type{engine.TAny},
	})
}

// --- Duration Construction ---

func registerDurYears(r *engine.Registry) {
	r.Register("years", engine.Signature{
		Args: []engine.Type{engine.TInteger},
		Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
			n, err := args[0].AsConcreteInteger()
			if err != nil {
				return nil, err
			}
			return []engine.Value{engine.NewCalDuration(int(n), 0, 0)}, nil
		},
		Returns: []engine.Type{engine.TCalDuration},
	})
}

func registerDurMonths(r *engine.Registry) {
	r.Register("months", engine.Signature{
		Args: []engine.Type{engine.TInteger},
		Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
			n, err := args[0].AsConcreteInteger()
			if err != nil {
				return nil, err
			}
			return []engine.Value{engine.NewCalDuration(0, int(n), 0)}, nil
		},
		Returns: []engine.Type{engine.TCalDuration},
	})
}

func registerDurWeeks(r *engine.Registry) {
	r.Register("weeks", engine.Signature{
		Args: []engine.Type{engine.TInteger},
		Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
			n, err := args[0].AsConcreteInteger()
			if err != nil {
				return nil, err
			}
			return []engine.Value{engine.NewCalDuration(0, 0, int(n)*7)}, nil
		},
		Returns: []engine.Type{engine.TClkDuration},
	})
}

func registerDurDays(r *engine.Registry) {
	r.Register("days", engine.Signature{
		Args: []engine.Type{engine.TInteger},
		Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
			n, err := args[0].AsConcreteInteger()
			if err != nil {
				return nil, err
			}
			return []engine.Value{engine.NewCalDuration(0, 0, int(n))}, nil
		},
		Returns: []engine.Type{engine.TClkDuration},
	})
}

func registerDurHours(r *engine.Registry) {
	r.Register("hours", engine.Signature{
		Args: []engine.Type{engine.TNumber},
		Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
			n, err := args[0].AsNumber()
			if err != nil {
				return nil, err
			}
			return []engine.Value{engine.NewClkDuration(time.Duration(n * float64(time.Hour)))}, nil
		},
		Returns: []engine.Type{engine.TClkDuration},
	})
}

func registerDurMinutes(r *engine.Registry) {
	r.Register("minutes", engine.Signature{
		Args: []engine.Type{engine.TNumber},
		Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
			n, err := args[0].AsNumber()
			if err != nil {
				return nil, err
			}
			return []engine.Value{engine.NewClkDuration(time.Duration(n * float64(time.Minute)))}, nil
		},
		Returns: []engine.Type{engine.TClkDuration},
	})
}

func registerDurSeconds(r *engine.Registry) {
	r.Register("seconds", engine.Signature{
		Args: []engine.Type{engine.TNumber},
		Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
			n, err := args[0].AsNumber()
			if err != nil {
				return nil, err
			}
			return []engine.Value{engine.NewClkDuration(time.Duration(n * float64(time.Second)))}, nil
		},
		Returns: []engine.Type{engine.TClkDuration},
	})
}

func registerDurMs(r *engine.Registry) {
	r.Register("ms", engine.Signature{
		Args: []engine.Type{engine.TNumber},
		Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
			n, err := args[0].AsNumber()
			if err != nil {
				return nil, err
			}
			return []engine.Value{engine.NewClkDuration(time.Duration(n * float64(time.Millisecond)))}, nil
		},
		Returns: []engine.Type{engine.TClkDuration},
	})
}

func registerDurUs(r *engine.Registry) {
	r.Register("us", engine.Signature{
		Args: []engine.Type{engine.TNumber},
		Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
			n, err := args[0].AsNumber()
			if err != nil {
				return nil, err
			}
			return []engine.Value{engine.NewClkDuration(time.Duration(n * float64(time.Microsecond)))}, nil
		},
		Returns: []engine.Type{engine.TClkDuration},
	})
}

func registerDurNs(r *engine.Registry) {
	r.Register("ns", engine.Signature{
		Args: []engine.Type{engine.TNumber},
		Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
			n, err := args[0].AsNumber()
			if err != nil {
				return nil, err
			}
			return []engine.Value{engine.NewClkDuration(time.Duration(n))}, nil
		},
		Returns: []engine.Type{engine.TClkDuration},
	})
}

func registerCalDur(r *engine.Registry) {
	// cal-dur 1 6 15 → args[0]=15 (nearest), args[1]=6, args[2]=1 (deepest)
	r.Register("cal-dur", engine.Signature{
		Args: []engine.Type{engine.TInteger, engine.TInteger, engine.TInteger},
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
		Returns: []engine.Type{engine.TCalDuration},
	})
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
		fmt.Sscanf(timePart[:i], "%f", &n)
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

func registerTimeDuration(r *engine.Registry) {
	r.Register("time-duration", engine.Signature{
		Args: []engine.Type{engine.TString},
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
		Returns: []engine.Type{engine.TClkDuration},
	})
}

// --- Duration Extraction ---

func registerTotalHours(r *engine.Registry) {
	r.Register("total-hours", engine.Signature{
		Args: []engine.Type{engine.TClkDuration},
		Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
			d, _ := args[0].AsClkDuration()
			return []engine.Value{engine.NewDecimal(d.Hours())}, nil
		},
		Returns: []engine.Type{engine.TDecimal},
	})
}

func registerTotalMinutes(r *engine.Registry) {
	r.Register("total-minutes", engine.Signature{
		Args: []engine.Type{engine.TClkDuration},
		Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
			d, _ := args[0].AsClkDuration()
			return []engine.Value{engine.NewDecimal(d.Minutes())}, nil
		},
		Returns: []engine.Type{engine.TDecimal},
	})
}

func registerTotalSeconds(r *engine.Registry) {
	r.Register("total-seconds", engine.Signature{
		Args: []engine.Type{engine.TClkDuration},
		Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
			d, _ := args[0].AsClkDuration()
			return []engine.Value{engine.NewDecimal(d.Seconds())}, nil
		},
		Returns: []engine.Type{engine.TDecimal},
	})
}

func registerTotalMs(r *engine.Registry) {
	r.Register("total-ms", engine.Signature{
		Args: []engine.Type{engine.TClkDuration},
		Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
			d, _ := args[0].AsClkDuration()
			return []engine.Value{engine.NewDecimal(float64(d.Milliseconds()))}, nil
		},
		Returns: []engine.Type{engine.TInteger},
	})
}

func registerDurYearsExtract(r *engine.Registry) {
	r.Register("dur-years", engine.Signature{
		Args: []engine.Type{engine.TCalDuration},
		Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
			cd, _ := args[0].AsCalDuration()
			return []engine.Value{engine.NewInteger(int64(cd.Years))}, nil
		},
		Returns: []engine.Type{engine.TInteger},
	})
}

func registerDurMonthsExtract(r *engine.Registry) {
	r.Register("dur-months", engine.Signature{
		Args: []engine.Type{engine.TCalDuration},
		Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
			cd, _ := args[0].AsCalDuration()
			return []engine.Value{engine.NewInteger(int64(cd.Months))}, nil
		},
		Returns: []engine.Type{engine.TInteger},
	})
}

func registerDurDaysExtract(r *engine.Registry) {
	r.Register("dur-days", engine.Signature{
		Args: []engine.Type{engine.TCalDuration},
		Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
			cd, _ := args[0].AsCalDuration()
			return []engine.Value{engine.NewInteger(int64(cd.Days))}, nil
		},
		Returns: []engine.Type{engine.TInteger},
	})
}

func registerDurSign(r *engine.Registry) {
	r.Register("dur-sign", engine.Signature{
		Args: []engine.Type{engine.TCalDuration},
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
		Returns: []engine.Type{engine.TInteger},
	})
	r.Register("dur-sign", engine.Signature{
		Args: []engine.Type{engine.TClkDuration},
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
		Returns: []engine.Type{engine.TInteger},
	})
}

// --- Arithmetic ---

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

func registerUntil(r *engine.Registry) {
	// d1 d2 until → duration from d1 to d2. args[0]=d2 (nearest), args[1]=d1
	r.Register("until", engine.Signature{
		Args: []engine.Type{engine.TDate, engine.TDate},
		Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
			from := extractTime(args[1])
			to := extractTime(args[0])
			cd := dateDiffCalDuration(from, to)
			return []engine.Value{engine.NewCalDuration(cd.Years, cd.Months, cd.Days)}, nil
		},
		Returns: []engine.Type{engine.TClkDuration},
	})
}

func registerSince(r *engine.Registry) {
	// d1 d2 since → duration from d2 to d1. args[0]=d2 (nearest), args[1]=d1
	r.Register("since", engine.Signature{
		Args: []engine.Type{engine.TDate, engine.TDate},
		Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
			from := extractTime(args[0])
			to := extractTime(args[1])
			cd := dateDiffCalDuration(from, to)
			return []engine.Value{engine.NewCalDuration(cd.Years, cd.Months, cd.Days)}, nil
		},
		Returns: []engine.Type{engine.TClkDuration},
	})
}

func registerDiff(r *engine.Registry) {
	// ins1 ins2 diff → ClkDuration. args[0]=ins2 (nearest), args[1]=ins1
	r.Register("diff", engine.Signature{
		Args: []engine.Type{engine.TInstant, engine.TInstant},
		Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
			t1 := extractTime(args[1])
			t2 := extractTime(args[0])
			return []engine.Value{engine.NewClkDuration(t2.Sub(t1))}, nil
		},
		Returns: []engine.Type{engine.TClkDuration},
	})
}

func registerElapsed(r *engine.Registry) {
	r.Register("elapsed", engine.Signature{
		Args: []engine.Type{engine.TInstant},
		Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
			start := extractTime(args[0])
			return []engine.Value{engine.NewClkDuration(time.Since(start))}, nil
		},
		Returns: []engine.Type{engine.TClkDuration},
	})
}

// --- Comparison extended ---

func registerTimeCompare(r *engine.Registry) {
	r.Register("time-compare", engine.Signature{
		Args: []engine.Type{engine.TDate, engine.TDate},
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
		Returns: []engine.Type{engine.TInteger},
	})
}

func registerBetween(r *engine.Registry) {
	// d start end between? → args[0]=end (nearest), args[1]=start, args[2]=d (deepest)
	r.Register("between?", engine.Signature{
		Args: []engine.Type{engine.TDate, engine.TDate, engine.TDate},
		Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
			d := extractTime(args[2])
			start := extractTime(args[1])
			end := extractTime(args[0])
			return []engine.Value{engine.NewBoolean(!d.Before(start) && !d.After(end))}, nil
		},
		Returns: []engine.Type{engine.TBoolean},
	})
}

func registerEarliest(r *engine.Registry) {
	r.Register("earliest", engine.Signature{
		Args: []engine.Type{engine.TDate, engine.TDate},
		Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
			t1 := extractTime(args[1])
			t2 := extractTime(args[0])
			if t1.Before(t2) {
				return []engine.Value{engine.NewDate(t1)}, nil
			}
			return []engine.Value{engine.NewDate(t2)}, nil
		},
		Returns: []engine.Type{engine.TAny},
	})
}

func registerLatest(r *engine.Registry) {
	r.Register("latest", engine.Signature{
		Args: []engine.Type{engine.TDate, engine.TDate},
		Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
			t1 := extractTime(args[1])
			t2 := extractTime(args[0])
			if t1.After(t2) {
				return []engine.Value{engine.NewDate(t1)}, nil
			}
			return []engine.Value{engine.NewDate(t2)}, nil
		},
		Returns: []engine.Type{engine.TAny},
	})
}

// --- Conversion ---

func registerToDate(r *engine.Registry) {
	r.Register("to-date", engine.Signature{
		Args: []engine.Type{engine.TDateTime},
		Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
			t := extractTime(args[0])
			return []engine.Value{engine.NewDate(time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location()))}, nil
		},
		Returns: []engine.Type{engine.TDate},
	})
	r.Register("to-date", engine.Signature{
		Args: []engine.Type{engine.TInstant},
		Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
			t := extractTime(args[0])
			return []engine.Value{engine.NewDate(time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC))}, nil
		},
		Returns: []engine.Type{engine.TDate},
	})
}

func registerToTimeOfDay(r *engine.Registry) {
	r.Register("to-time-of-day", engine.Signature{
		Args: []engine.Type{engine.TDateTime},
		Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
			t := extractTime(args[0])
			d := time.Duration(t.Hour())*time.Hour + time.Duration(t.Minute())*time.Minute +
				time.Duration(t.Second())*time.Second + time.Duration(t.Nanosecond())
			return []engine.Value{engine.NewTimeOfDay(d)}, nil
		},
		Returns: []engine.Type{engine.TTimeOfDay},
	})
	r.Register("to-time-of-day", engine.Signature{
		Args: []engine.Type{engine.TInstant},
		Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
			t := extractTime(args[0])
			d := time.Duration(t.Hour())*time.Hour + time.Duration(t.Minute())*time.Minute +
				time.Duration(t.Second())*time.Second + time.Duration(t.Nanosecond())
			return []engine.Value{engine.NewTimeOfDay(d)}, nil
		},
		Returns: []engine.Type{engine.TTimeOfDay},
	})
}

func registerToDateTime(r *engine.Registry) {
	r.Register("to-datetime", engine.Signature{
		Args: []engine.Type{engine.TDate},
		Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
			t := extractTime(args[0])
			return []engine.Value{engine.NewDateTime(t)}, nil
		},
		Returns: []engine.Type{engine.TDateTime},
	})
}

func registerToInstant(r *engine.Registry) {
	// dt tz to-instant → args[0]=tz (nearest), args[1]=dt
	r.Register("to-instant", engine.Signature{
		Args: []engine.Type{engine.TTimezone, engine.TDateTime},
		Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
			dt := extractTime(args[1])
			loc := args[0].AsTimezone()
			if loc == nil {
				loc = time.UTC
			}
			t := time.Date(dt.Year(), dt.Month(), dt.Day(), dt.Hour(), dt.Minute(), dt.Second(), dt.Nanosecond(), loc)
			return []engine.Value{engine.NewInstant(t)}, nil
		},
		Returns: []engine.Type{engine.TInstant},
	})
}

func registerToLocal(r *engine.Registry) {
	// ins tz to-local → args[0]=tz (nearest), args[1]=ins
	r.Register("to-local", engine.Signature{
		Args: []engine.Type{engine.TTimezone, engine.TInstant},
		Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
			t := extractTime(args[1])
			loc := args[0].AsTimezone()
			if loc == nil {
				loc = time.UTC
			}
			return []engine.Value{engine.NewDateTime(t.In(loc))}, nil
		},
		Returns: []engine.Type{engine.TDateTime},
	})
}

func registerToUtc(r *engine.Registry) {
	r.Register("to-utc", engine.Signature{
		Args: []engine.Type{engine.TInstant},
		Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
			t := extractTime(args[0])
			return []engine.Value{engine.NewDateTime(t.UTC())}, nil
		},
		Returns: []engine.Type{engine.TDateTime},
	})
}

// --- Rounding ---

func registerStartOf(r *engine.Registry) {
	r.Register("start-of", engine.Signature{
		Args: []engine.Type{engine.TDate, engine.TString},
		Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
			t := extractTime(args[0])
			unit, err := args[1].AsConcreteString()
			if err != nil {
				return nil, err
			}
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
		Returns: []engine.Type{engine.TAny},
	})
}

func registerEndOf(r *engine.Registry) {
	r.Register("end-of", engine.Signature{
		Args: []engine.Type{engine.TDate, engine.TString},
		Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
			t := extractTime(args[0])
			unit, err := args[1].AsConcreteString()
			if err != nil {
				return nil, err
			}
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
		Returns: []engine.Type{engine.TAny},
	})
}

// --- Timezone ---

func registerTzUtc(r *engine.Registry) {
	r.RegisterStackOnly("tz-utc", engine.Signature{
		Args: []engine.Type{},
		Handler: func(_ []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
			return []engine.Value{engine.NewTimezone(time.UTC)}, nil
		},
		Returns: []engine.Type{engine.TTimezone},
	})
}

func registerTzLocal(r *engine.Registry) {
	r.RegisterStackOnly("tz-local", engine.Signature{
		Args: []engine.Type{},
		Handler: func(_ []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
			return []engine.Value{engine.NewTimezone(time.Local)}, nil
		},
		Returns: []engine.Type{engine.TTimezone},
	})
}

func registerTzName(r *engine.Registry) {
	r.Register("tz-name", engine.Signature{
		Args: []engine.Type{engine.TTimezone},
		Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
			loc := args[0].AsTimezone()
			if loc == nil {
				return []engine.Value{engine.NewString("UTC")}, nil
			}
			return []engine.Value{engine.NewString(loc.String())}, nil
		},
		Returns: []engine.Type{engine.TString},
	})
}

func registerTzOffset(r *engine.Registry) {
	// ins tz tz-offset → args[0]=tz (nearest), args[1]=ins
	r.Register("tz-offset", engine.Signature{
		Args: []engine.Type{engine.TTimezone, engine.TInstant},
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
		Returns: []engine.Type{engine.TClkDuration},
	})
}

func registerDst(r *engine.Registry) {
	// ins tz dst? → args[0]=tz (nearest), args[1]=ins
	r.Register("dst?", engine.Signature{
		Args: []engine.Type{engine.TTimezone, engine.TInstant},
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
		Returns: []engine.Type{engine.TBoolean},
	})
}

// --- Parsing ---

func registerParseDate(r *engine.Registry) {
	// "15/03/2024" "02/01/2006" parse-date → args[0]=layout (nearest), args[1]=str
	r.Register("parse-date", engine.Signature{
		Args: []engine.Type{engine.TString, engine.TString},
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
		Returns: []engine.Type{engine.TDate},
	})
}

func registerParseDatetime(r *engine.Registry) {
	r.Register("parse-datetime", engine.Signature{
		Args: []engine.Type{engine.TString, engine.TString},
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
		Returns: []engine.Type{engine.TDateTime},
	})
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

func registerAutoDate(r *engine.Registry) {
	r.Register("auto-date", engine.Signature{
		Args: []engine.Type{engine.TString},
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
		Returns: []engine.Type{engine.TDateTime},
	})
}
