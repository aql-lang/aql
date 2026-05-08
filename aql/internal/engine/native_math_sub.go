package engine

func RegisterSub(r *Registry) {
	// Temporal: Date sub CalDuration → Date
	subDateCal := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		t := args[0].AsDate()
		cd, _ := args[1].AsCalDuration()
		return []Value{NewDate(t.AddDate(-cd.Years, -cd.Months, -cd.Days))}, nil
	}
	subDtClk := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		t := args[0].AsDateTime()
		d, _ := args[1].AsClkDuration()
		return []Value{NewDateTime(t.Add(-d))}, nil
	}
	subInsClk := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		t := args[0].AsInstant()
		d, _ := args[1].AsClkDuration()
		return []Value{NewInstant(t.Add(-d))}, nil
	}

	r.RegisterNativeFunc(NativeFunc{
		Name:              "sub",
		ForwardPrecedence: true,
		Signatures: []NativeSig{
			{
				Args: []Type{TNumber, TNumber},
				Handler: numericBinaryHandler(
					func(a, b int64) (Value, error) { return NewInteger(a - b), nil },
					func(a, b float64) (Value, error) { return NewDecimal(a - b), nil },
				),
				ReturnsFn: ReturnsNumericBinary(),
			},
			{Args: []Type{TDate, TCalDuration}, Handler: subDateCal, Returns: []Type{TDate}},
			{Args: []Type{TDateTime, TClkDuration}, Handler: subDtClk, Returns: []Type{TDateTime}},
			{Args: []Type{TInstant, TClkDuration}, Handler: subInsClk, Returns: []Type{TInstant}},
		},
	})
}
