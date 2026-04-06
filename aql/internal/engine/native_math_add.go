package engine

import "time"

func registerAdd(r *Registry) {
	// String concatenation: [TScalar, TScalar] converts both to strings.
	// More specific signatures (Integer×Integer) win due to higher specificity.
	concatHandler := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		return []Value{NewString(valToString(args[1]) + valToString(args[0]))}, nil
	}

	// Temporal: Date + CalDuration → Date
	addDateCalHandler := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		t := args[1].AsDate()
		cd, _ := args[0].AsCalDuration()
		return []Value{NewDate(t.AddDate(cd.Years, cd.Months, cd.Days))}, nil
	}

	// Temporal: DateTime + ClkDuration → DateTime
	addDtClkHandler := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		t := args[1].AsDateTime()
		d, _ := args[0].AsClkDuration()
		return []Value{NewDateTime(t.Add(d))}, nil
	}

	// Temporal: Instant + ClkDuration → Instant
	addInsClkHandler := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		t := args[1].AsInstant()
		d, _ := args[0].AsClkDuration()
		return []Value{NewInstant(t.Add(d))}, nil
	}

	// Temporal: Date + ClkDuration → DateTime (promote)
	addDateClkHandler := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		t := args[1].AsDate()
		d, _ := args[0].AsClkDuration()
		return []Value{NewDateTime(t.Add(d))}, nil
	}

	registerBinaryMathWord(r, "add",
		func(a, b int64) (Value, error) { return NewInteger(a + b), nil },
		func(a, b float64) (Value, error) { return NewDecimal(a + b), nil },
		NativeSig{Args: []Type{TScalar, TScalar}, Handler: concatHandler},
		NativeSig{Args: []Type{TCalDuration, TDate}, Handler: addDateCalHandler},
		NativeSig{Args: []Type{TClkDuration, TDateTime}, Handler: addDtClkHandler},
		NativeSig{Args: []Type{TClkDuration, TInstant}, Handler: addInsClkHandler},
		NativeSig{Args: []Type{TClkDuration, TDate}, Handler: addDateClkHandler},
	)
}

// Ensure time import is used.
var _ = time.Now
