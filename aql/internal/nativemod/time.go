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
			s, err := args[0].AsString()
			if err != nil {
				return nil, err
			}
			t, err := time.Parse("2006-01-02", s)
			if err != nil {
				return nil, fmt.Errorf("date: invalid ISO 8601 date string: %q", s)
			}
			return []engine.Value{engine.NewDate(t)}, nil
		},
	})
}

func registerTimeDateTime(r *engine.Registry) {
	r.Register("time-datetime", engine.Signature{
		Args: []engine.Type{engine.TString},
		Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
			s, err := args[0].AsString()
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
	})
}

func registerTimeInstant(r *engine.Registry) {
	r.Register("time-instant", engine.Signature{
		Args: []engine.Type{engine.TString},
		Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
			s, err := args[0].AsString()
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
	})
}

func registerTimeTimeOfDay(r *engine.Registry) {
	r.Register("time-time-of-day", engine.Signature{
		Args: []engine.Type{engine.TString},
		Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
			s, err := args[0].AsString()
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
	})
}

func registerTimeTz(r *engine.Registry) {
	r.Register("time-tz", engine.Signature{
		Args: []engine.Type{engine.TString},
		Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
			s, err := args[0].AsString()
			if err != nil {
				return nil, err
			}
			loc, err := time.LoadLocation(s)
			if err != nil {
				return nil, fmt.Errorf("tz: unknown timezone: %q", s)
			}
			return []engine.Value{engine.NewTimezone(loc)}, nil
		},
	})
}

func registerTimeUnix(r *engine.Registry) {
	r.Register("time-unix", engine.Signature{
		Args: []engine.Type{engine.TInteger},
		Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
			n, err := args[0].AsInteger()
			if err != nil {
				return nil, err
			}
			return []engine.Value{engine.NewInstant(time.Unix(n, 0))}, nil
		},
	})
}

func registerTimeUnixMs(r *engine.Registry) {
	r.Register("time-unix-ms", engine.Signature{
		Args: []engine.Type{engine.TInteger},
		Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
			n, err := args[0].AsInteger()
			if err != nil {
				return nil, err
			}
			return []engine.Value{engine.NewInstant(time.UnixMilli(n))}, nil
		},
	})
}

func registerTimeUnixNs(r *engine.Registry) {
	r.Register("time-unix-ns", engine.Signature{
		Args: []engine.Type{engine.TInteger},
		Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
			n, err := args[0].AsInteger()
			if err != nil {
				return nil, err
			}
			t := time.Unix(0, n)
			return []engine.Value{engine.NewInstant(t)}, nil
		},
	})
}

// --- Current Time ---

func registerTimeNowLocal(r *engine.Registry) {
	r.RegisterStackOnly("time-now-local", engine.Signature{
		Args: []engine.Type{},
		Handler: func(_ []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
			return []engine.Value{engine.NewDateTime(time.Now())}, nil
		},
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
	})
}

// --- Extraction ---

func registerYear(r *engine.Registry) {
	r.Register("year", engine.Signature{
		Args: []engine.Type{engine.TDate},
		Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
			return []engine.Value{engine.NewInteger(int64(extractTime(args[0]).Year()))}, nil
		},
	})
}

func registerMonth(r *engine.Registry) {
	r.Register("month", engine.Signature{
		Args: []engine.Type{engine.TDate},
		Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
			return []engine.Value{engine.NewInteger(int64(extractTime(args[0]).Month()))}, nil
		},
	})
}

func registerDay(r *engine.Registry) {
	r.Register("day", engine.Signature{
		Args: []engine.Type{engine.TDate},
		Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
			return []engine.Value{engine.NewInteger(int64(extractTime(args[0]).Day()))}, nil
		},
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
	})
}

func registerYearDay(r *engine.Registry) {
	r.Register("year-day", engine.Signature{
		Args: []engine.Type{engine.TDate},
		Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
			return []engine.Value{engine.NewInteger(int64(extractTime(args[0]).YearDay()))}, nil
		},
	})
}

func registerWeekdayName(r *engine.Registry) {
	r.Register("weekday-name", engine.Signature{
		Args: []engine.Type{engine.TDate},
		Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
			return []engine.Value{engine.NewString(extractTime(args[0]).Weekday().String())}, nil
		},
	})
}

func registerMonthName(r *engine.Registry) {
	r.Register("month-name", engine.Signature{
		Args: []engine.Type{engine.TDate},
		Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
			return []engine.Value{engine.NewString(extractTime(args[0]).Month().String())}, nil
		},
	})
}

func registerIsoWeek(r *engine.Registry) {
	r.Register("iso-week", engine.Signature{
		Args: []engine.Type{engine.TDate},
		Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
			_, week := extractTime(args[0]).ISOWeek()
			return []engine.Value{engine.NewInteger(int64(week))}, nil
		},
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
	})
}

func registerToUnix(r *engine.Registry) {
	r.Register("to-unix", engine.Signature{
		Args: []engine.Type{engine.TInstant},
		Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
			return []engine.Value{engine.NewInteger(extractTime(args[0]).Unix())}, nil
		},
	})
}

func registerToUnixMs(r *engine.Registry) {
	r.Register("to-unix-ms", engine.Signature{
		Args: []engine.Type{engine.TInstant},
		Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
			return []engine.Value{engine.NewInteger(extractTime(args[0]).UnixMilli())}, nil
		},
	})
}

// --- Comparison ---

func registerBefore(r *engine.Registry) {
	r.Register("before?", engine.Signature{
		Args: []engine.Type{engine.TDate, engine.TDate},
		Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
			return []engine.Value{engine.NewBoolean(extractTime(args[1]).Before(extractTime(args[0])))}, nil
		},
	})
}

func registerAfter(r *engine.Registry) {
	r.Register("after?", engine.Signature{
		Args: []engine.Type{engine.TDate, engine.TDate},
		Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
			return []engine.Value{engine.NewBoolean(extractTime(args[1]).After(extractTime(args[0])))}, nil
		},
	})
}

func registerEqual(r *engine.Registry) {
	r.Register("equal?", engine.Signature{
		Args: []engine.Type{engine.TDate, engine.TDate},
		Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
			return []engine.Value{engine.NewBoolean(extractTime(args[0]).Equal(extractTime(args[1])))}, nil
		},
	})
}

// --- Formatting ---

func registerToString(r *engine.Registry) {
	r.Register("to-string", engine.Signature{
		Args: []engine.Type{engine.TDate},
		Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
			return []engine.Value{engine.NewString(extractTime(args[0]).Format("2006-01-02"))}, nil
		},
	})
}

func registerFormat(r *engine.Registry) {
	r.Register("format", engine.Signature{
		Args: []engine.Type{engine.TDate, engine.TString},
		Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
			layout, err := args[1].AsString()
			if err != nil {
				return nil, err
			}
			return []engine.Value{engine.NewString(extractTime(args[0]).Format(layout))}, nil
		},
	})
}

func registerToIso(r *engine.Registry) {
	r.Register("to-iso", engine.Signature{
		Args: []engine.Type{engine.TDate},
		Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
			return []engine.Value{engine.NewString(extractTime(args[0]).Format("2006-01-02"))}, nil
		},
	})
}

// --- Legacy Arithmetic ---

func registerAddDays(r *engine.Registry) {
	r.Register("add-days", engine.Signature{
		Args: []engine.Type{engine.TDate, engine.TInteger},
		Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
			t := extractTime(args[0])
			n, err := args[1].AsInteger()
			if err != nil {
				return nil, err
			}
			return []engine.Value{engine.NewDate(t.AddDate(0, 0, int(n)))}, nil
		},
	})
}

func registerAddMonths(r *engine.Registry) {
	r.Register("add-months", engine.Signature{
		Args: []engine.Type{engine.TDate, engine.TInteger},
		Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
			t := extractTime(args[0])
			n, err := args[1].AsInteger()
			if err != nil {
				return nil, err
			}
			return []engine.Value{engine.NewDate(t.AddDate(0, int(n), 0))}, nil
		},
	})
}

func registerAddYears(r *engine.Registry) {
	r.Register("add-years", engine.Signature{
		Args: []engine.Type{engine.TDate, engine.TInteger},
		Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
			t := extractTime(args[0])
			n, err := args[1].AsInteger()
			if err != nil {
				return nil, err
			}
			return []engine.Value{engine.NewDate(t.AddDate(int(n), 0, 0))}, nil
		},
	})
}

// --- Duration Construction ---

func registerDurYears(r *engine.Registry) {
	r.Register("years", engine.Signature{
		Args: []engine.Type{engine.TInteger},
		Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
			n, err := args[0].AsInteger()
			if err != nil {
				return nil, err
			}
			return []engine.Value{engine.NewCalDuration(int(n), 0, 0)}, nil
		},
	})
}

func registerDurMonths(r *engine.Registry) {
	r.Register("months", engine.Signature{
		Args: []engine.Type{engine.TInteger},
		Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
			n, err := args[0].AsInteger()
			if err != nil {
				return nil, err
			}
			return []engine.Value{engine.NewCalDuration(0, int(n), 0)}, nil
		},
	})
}

func registerDurWeeks(r *engine.Registry) {
	r.Register("weeks", engine.Signature{
		Args: []engine.Type{engine.TInteger},
		Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
			n, err := args[0].AsInteger()
			if err != nil {
				return nil, err
			}
			return []engine.Value{engine.NewCalDuration(0, 0, int(n)*7)}, nil
		},
	})
}

func registerDurDays(r *engine.Registry) {
	r.Register("days", engine.Signature{
		Args: []engine.Type{engine.TInteger},
		Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
			n, err := args[0].AsInteger()
			if err != nil {
				return nil, err
			}
			return []engine.Value{engine.NewCalDuration(0, 0, int(n))}, nil
		},
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
	})
}

func registerCalDur(r *engine.Registry) {
	// cal-dur 1 6 15 → args[0]=15 (nearest), args[1]=6, args[2]=1 (deepest)
	r.Register("cal-dur", engine.Signature{
		Args: []engine.Type{engine.TInteger, engine.TInteger, engine.TInteger},
		Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
			y, err := args[2].AsInteger()
			if err != nil {
				return nil, err
			}
			m, err := args[1].AsInteger()
			if err != nil {
				return nil, err
			}
			d, err := args[0].AsInteger()
			if err != nil {
				return nil, err
			}
			return []engine.Value{engine.NewCalDuration(int(y), int(m), int(d))}, nil
		},
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
			s, err := args[0].AsString()
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
	})
}

func registerTotalMinutes(r *engine.Registry) {
	r.Register("total-minutes", engine.Signature{
		Args: []engine.Type{engine.TClkDuration},
		Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
			d, _ := args[0].AsClkDuration()
			return []engine.Value{engine.NewDecimal(d.Minutes())}, nil
		},
	})
}

func registerTotalSeconds(r *engine.Registry) {
	r.Register("total-seconds", engine.Signature{
		Args: []engine.Type{engine.TClkDuration},
		Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
			d, _ := args[0].AsClkDuration()
			return []engine.Value{engine.NewDecimal(d.Seconds())}, nil
		},
	})
}

func registerTotalMs(r *engine.Registry) {
	r.Register("total-ms", engine.Signature{
		Args: []engine.Type{engine.TClkDuration},
		Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
			d, _ := args[0].AsClkDuration()
			return []engine.Value{engine.NewDecimal(float64(d.Milliseconds()))}, nil
		},
	})
}

func registerDurYearsExtract(r *engine.Registry) {
	r.Register("dur-years", engine.Signature{
		Args: []engine.Type{engine.TCalDuration},
		Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
			cd, _ := args[0].AsCalDuration()
			return []engine.Value{engine.NewInteger(int64(cd.Years))}, nil
		},
	})
}

func registerDurMonthsExtract(r *engine.Registry) {
	r.Register("dur-months", engine.Signature{
		Args: []engine.Type{engine.TCalDuration},
		Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
			cd, _ := args[0].AsCalDuration()
			return []engine.Value{engine.NewInteger(int64(cd.Months))}, nil
		},
	})
}

func registerDurDaysExtract(r *engine.Registry) {
	r.Register("dur-days", engine.Signature{
		Args: []engine.Type{engine.TCalDuration},
		Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
			cd, _ := args[0].AsCalDuration()
			return []engine.Value{engine.NewInteger(int64(cd.Days))}, nil
		},
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
	})
}
