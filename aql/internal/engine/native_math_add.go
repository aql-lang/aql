package engine

func RegisterAdd(r *Registry) {
	// String concatenation: [TScalar, TScalar] converts both to strings.
	concatHandler := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		return []Value{NewString(ValToString(args[0]) + ValToString(args[1]))}, nil
	}

	addDateCal := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		t := args[0].AsDate()
		cd, _ := args[1].AsCalDuration()
		return []Value{NewDate(t.AddDate(cd.Years, cd.Months, cd.Days))}, nil
	}
	addDtClk := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		t := args[0].AsDateTime()
		d, _ := args[1].AsClkDuration()
		return []Value{NewDateTime(t.Add(d))}, nil
	}
	addInsClk := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		t := args[0].AsInstant()
		d, _ := args[1].AsClkDuration()
		return []Value{NewInstant(t.Add(d))}, nil
	}
	addDateClk := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		t := args[0].AsDate()
		d, _ := args[1].AsClkDuration()
		return []Value{NewDateTime(t.Add(d))}, nil
	}

	r.RegisterNativeFunc(NativeFunc{
		Name:              "add",
		ForwardPrecedence: true,
		Signatures: []NativeSig{
			{
				Args: []Type{TNumber, TNumber},
				Handler: numericBinaryHandler(
					func(a, b int64) (Value, error) { return NewInteger(a + b), nil },
					func(a, b float64) (Value, error) { return NewDecimal(a + b), nil },
				),
				ReturnsFn: ReturnsNumericBinary(),
			},
			{Args: []Type{TScalar, TScalar}, Handler: concatHandler, Returns: []Type{TString}},
			{Args: []Type{TDate, TCalDuration}, Handler: addDateCal, Returns: []Type{TDate}},
			{Args: []Type{TDateTime, TClkDuration}, Handler: addDtClk, Returns: []Type{TDateTime}},
			{Args: []Type{TInstant, TClkDuration}, Handler: addInsClk, Returns: []Type{TInstant}},
			{Args: []Type{TDate, TClkDuration}, Handler: addDateClk, Returns: []Type{TDateTime}},
		},
	})
}
