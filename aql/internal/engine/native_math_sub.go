package engine

func registerSub(r *Registry) {
	// Signature [Integer, Integer]: args[0] = nearest to word (top/forward),
	// args[1] = farther (deeper/later). `a b sub` → args=[b,a] → a-b.
	intHandler := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		_as2, _ := args[0].AsInteger()
		_as1, _ := args[1].AsInteger()
		result := _as1 - _as2
		return []Value{NewInteger(result)}, nil
	}

	numHandler := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		_as4, _ := args[0].AsNumber()
		_as3, _ := args[1].AsNumber()
		result := _as3 - _as4
		return []Value{NewDecimal(result)}, nil
	}

	// Temporal: Date sub CalDuration → Date
	// date sub 1 months → args[0]=CalDuration (nearest), args[1]=Date
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

	r.RegisterNativeFunc(NativeFunc{
		Name:              "sub",
		ForwardPrecedence: true,
		Signatures: []NativeSig{
			{
				Args:    []Type{TInteger, TInteger},
				Handler: intHandler,
			},
			{
				Args:    []Type{TDecimal, TDecimal},
				Handler: numHandler,
			},
			{
				Args:    []Type{TNumber, TDecimal},
				Handler: numHandler,
			},
			{
				Args:    []Type{TDecimal, TNumber},
				Handler: numHandler,
			},
			// Temporal signatures
			{
				Args:    []Type{TCalDuration, TDate},
				Handler: subDateCalHandler,
			},
			{
				Args:    []Type{TClkDuration, TDateTime},
				Handler: subDtClkHandler,
			},
			{
				Args:    []Type{TClkDuration, TInstant},
				Handler: subInsClkHandler,
			},
		},
	})
}
