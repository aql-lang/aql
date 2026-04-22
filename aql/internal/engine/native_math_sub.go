package engine

func RegisterSub(r *Registry) {
	// Temporal: Date sub CalDuration → Date
	subDateCalHandler := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		t := args[1].AsDate()
		cd, _ := args[0].AsCalDuration()
		return []Value{NewDate(t.AddDate(-cd.Years, -cd.Months, -cd.Days))}, nil
	}

	// Temporal: DateTime sub ClkDuration → DateTime
	subDtClkHandler := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		t := args[1].AsDateTime()
		d, _ := args[0].AsClkDuration()
		return []Value{NewDateTime(t.Add(-d))}, nil
	}

	// Temporal: Instant sub ClkDuration → Instant
	subInsClkHandler := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		t := args[1].AsInstant()
		d, _ := args[0].AsClkDuration()
		return []Value{NewInstant(t.Add(-d))}, nil
	}

	registerBinaryMathWord(r, "sub",
		func(a, b float64) (Value, error) { return NewDecimal(a - b), nil },
		func(a, b int64) (Value, error) { return NewInteger(a - b), nil },
		NativeSig{Args: []Type{TCalDuration, TDate}, Handler: subDateCalHandler, Returns: []Type{TDate}},
		NativeSig{Args: []Type{TClkDuration, TDateTime}, Handler: subDtClkHandler, Returns: []Type{TDateTime}},
		NativeSig{Args: []Type{TClkDuration, TInstant}, Handler: subInsClkHandler, Returns: []Type{TInstant}},
	)
}
