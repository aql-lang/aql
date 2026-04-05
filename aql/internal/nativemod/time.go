package nativemod

import (
	"fmt"
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
