package nativemod

import (
	"fmt"
	"time"

	"github.com/metsitaba/voxgig-exp/aql/internal/engine"
)

// BuildTimeModule creates the "aql:time" native module. It registers the
// Go-implemented time words into an isolated sub-registry and returns a
// ModuleDesc with a "time" export containing FnDef wrappers for each word.
func BuildTimeModule(parent *engine.Registry) (engine.ModuleDesc, error) {
	subReg, err := engine.DefaultRegistry()
	if err != nil {
		return engine.ModuleDesc{}, err
	}

	registerAllTimeWords(subReg)

	exports := engine.NewOrderedMap()

	// Construction
	exports.Set("date", makeTimeFnDef("date", []engine.FnParam{{Type: engine.TString}}, []engine.Type{engine.TDate}, subReg))
	exports.Set("now", makeTimeFnDef("time-now", []engine.FnParam{}, []engine.Type{engine.TDate}, subReg))
	exports.Set("today", makeTimeFnDef("time-today", []engine.FnParam{}, []engine.Type{engine.TDate}, subReg))

	// Extraction (unary: Date -> Integer)
	for _, name := range []string{"year", "month", "day", "weekday", "year-day"} {
		exports.Set(name, makeTimeFnDef(name, []engine.FnParam{{Type: engine.TDate}}, []engine.Type{engine.TInteger}, subReg))
	}

	// Comparison (binary: Date Date -> Boolean)
	for _, name := range []string{"before?", "after?", "equal?"} {
		exports.Set(name, makeTimeFnDef(name, []engine.FnParam{{Type: engine.TDate}, {Type: engine.TDate}}, []engine.Type{engine.TBoolean}, subReg))
	}

	// Formatting
	exports.Set("to-string", makeTimeFnDef("to-string", []engine.FnParam{{Type: engine.TDate}}, []engine.Type{engine.TString}, subReg))
	exports.Set("format", makeTimeFnDef("format", []engine.FnParam{{Type: engine.TDate}, {Type: engine.TString}}, []engine.Type{engine.TString}, subReg))

	// Arithmetic (binary: Date Integer -> Date)
	for _, name := range []string{"add-days", "add-months", "add-years"} {
		exports.Set(name, makeTimeFnDef(name, []engine.FnParam{{Type: engine.TDate}, {Type: engine.TInteger}}, []engine.Type{engine.TDate}, subReg))
	}

	// Info
	exports.Set("days-in-month", makeTimeFnDef("days-in-month", []engine.FnParam{{Type: engine.TDate}}, []engine.Type{engine.TInteger}, subReg))
	exports.Set("leap-year?", makeTimeFnDef("leap-year?", []engine.FnParam{{Type: engine.TDate}}, []engine.Type{engine.TBoolean}, subReg))

	modID := parent.NextModuleID()
	desc := engine.ModuleDesc{
		ID:      modID,
		Exports: map[string]*engine.OrderedMap{"time": exports},
	}
	parent.Modules[modID] = desc
	return desc, nil
}

// makeTimeFnDef creates a FnDef value with the given params, returns, and word name.
func makeTimeFnDef(wordName string, params []engine.FnParam, returns []engine.Type, subReg *engine.Registry) engine.Value {
	fnDef := engine.FnDefInfo{
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
	registerDate(r)
	registerNow(r)
	registerToday(r)

	registerYear(r)
	registerMonth(r)
	registerDay(r)
	registerWeekday(r)
	registerYearDay(r)

	registerBefore(r)
	registerAfter(r)
	registerEqual(r)

	registerToString(r)
	registerFormat(r)

	registerAddDays(r)
	registerAddMonths(r)
	registerAddYears(r)

	registerDaysInMonth(r)
	registerLeapYear(r)
}

// --- Construction ---

func registerDate(r *engine.Registry) {
	r.Register("date", engine.Signature{
		Args: []engine.Type{engine.TString},
		Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
			s := args[0].AsString()
			t, err := time.Parse("2006-01-02", s)
			if err != nil {
				return nil, fmt.Errorf("date: invalid ISO 8601 date string: %q", s)
			}
			return []engine.Value{engine.NewDate(t)}, nil
		},
	})
}

func registerNow(r *engine.Registry) {
	r.RegisterStackOnly("time-now", engine.Signature{
		Args: []engine.Type{},
		Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
			now := time.Now().UTC()
			d := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
			return []engine.Value{engine.NewDate(d)}, nil
		},
	})
}

func registerToday(r *engine.Registry) {
	r.RegisterStackOnly("time-today", engine.Signature{
		Args: []engine.Type{},
		Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
			now := time.Now()
			d := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
			return []engine.Value{engine.NewDate(d)}, nil
		},
	})
}

// --- Extraction ---

func registerYear(r *engine.Registry) {
	r.Register("year", engine.Signature{
		Args: []engine.Type{engine.TDate},
		Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
			t := args[0].AsDate()
			return []engine.Value{engine.NewInteger(int64(t.Year()))}, nil
		},
	})
}

func registerMonth(r *engine.Registry) {
	r.Register("month", engine.Signature{
		Args: []engine.Type{engine.TDate},
		Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
			t := args[0].AsDate()
			return []engine.Value{engine.NewInteger(int64(t.Month()))}, nil
		},
	})
}

func registerDay(r *engine.Registry) {
	r.Register("day", engine.Signature{
		Args: []engine.Type{engine.TDate},
		Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
			t := args[0].AsDate()
			return []engine.Value{engine.NewInteger(int64(t.Day()))}, nil
		},
	})
}

func registerWeekday(r *engine.Registry) {
	r.Register("weekday", engine.Signature{
		Args: []engine.Type{engine.TDate},
		Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
			t := args[0].AsDate()
			wd := t.Weekday() // Sunday=0, Monday=1, ..., Saturday=6
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
			t := args[0].AsDate()
			return []engine.Value{engine.NewInteger(int64(t.YearDay()))}, nil
		},
	})
}

// --- Comparison ---

func registerBefore(r *engine.Registry) {
	// d1 d2 before? → args[0]=d2 (top), args[1]=d1 (deeper)
	r.Register("before?", engine.Signature{
		Args: []engine.Type{engine.TDate, engine.TDate},
		Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
			return []engine.Value{engine.NewBoolean(args[1].AsDate().Before(args[0].AsDate()))}, nil
		},
	})
}

func registerAfter(r *engine.Registry) {
	// d1 d2 after? → args[0]=d2 (top), args[1]=d1 (deeper)
	r.Register("after?", engine.Signature{
		Args: []engine.Type{engine.TDate, engine.TDate},
		Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
			return []engine.Value{engine.NewBoolean(args[1].AsDate().After(args[0].AsDate()))}, nil
		},
	})
}

func registerEqual(r *engine.Registry) {
	r.Register("equal?", engine.Signature{
		Args: []engine.Type{engine.TDate, engine.TDate},
		Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
			d1 := args[0].AsDate()
			d2 := args[1].AsDate()
			return []engine.Value{engine.NewBoolean(d1.Equal(d2))}, nil
		},
	})
}

// --- Formatting ---

func registerToString(r *engine.Registry) {
	r.Register("to-string", engine.Signature{
		Args: []engine.Type{engine.TDate},
		Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
			t := args[0].AsDate()
			return []engine.Value{engine.NewString(t.Format("2006-01-02"))}, nil
		},
	})
}

func registerFormat(r *engine.Registry) {
	r.Register("format", engine.Signature{
		Args: []engine.Type{engine.TDate, engine.TString},
		Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
			t := args[0].AsDate()
			layout := args[1].AsString()
			return []engine.Value{engine.NewString(t.Format(layout))}, nil
		},
	})
}

// --- Arithmetic ---

func registerAddDays(r *engine.Registry) {
	r.Register("add-days", engine.Signature{
		Args: []engine.Type{engine.TDate, engine.TInteger},
		Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
			t := args[0].AsDate()
			n := int(args[1].AsInteger())
			return []engine.Value{engine.NewDate(t.AddDate(0, 0, n))}, nil
		},
	})
}

func registerAddMonths(r *engine.Registry) {
	r.Register("add-months", engine.Signature{
		Args: []engine.Type{engine.TDate, engine.TInteger},
		Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
			t := args[0].AsDate()
			n := int(args[1].AsInteger())
			return []engine.Value{engine.NewDate(t.AddDate(0, n, 0))}, nil
		},
	})
}

func registerAddYears(r *engine.Registry) {
	r.Register("add-years", engine.Signature{
		Args: []engine.Type{engine.TDate, engine.TInteger},
		Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
			t := args[0].AsDate()
			n := int(args[1].AsInteger())
			return []engine.Value{engine.NewDate(t.AddDate(n, 0, 0))}, nil
		},
	})
}

// --- Info ---

func registerDaysInMonth(r *engine.Registry) {
	r.Register("days-in-month", engine.Signature{
		Args: []engine.Type{engine.TDate},
		Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
			t := args[0].AsDate()
			// Day 0 of the next month gives the last day of the current month.
			last := time.Date(t.Year(), t.Month()+1, 0, 0, 0, 0, 0, t.Location())
			return []engine.Value{engine.NewInteger(int64(last.Day()))}, nil
		},
	})
}

func registerLeapYear(r *engine.Registry) {
	r.Register("leap-year?", engine.Signature{
		Args: []engine.Type{engine.TDate},
		Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
			y := args[0].AsDate().Year()
			leap := y%4 == 0 && (y%100 != 0 || y%400 == 0)
			return []engine.Value{engine.NewBoolean(leap)}, nil
		},
	})
}
