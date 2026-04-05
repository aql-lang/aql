package engine

import "time"

func registerAdd(r *Registry) {
	intHandler := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		_as2, _ := args[0].AsInteger()
		_as1, _ := args[1].AsInteger()
		result := _as2 + _as1
		return []Value{NewInteger(result)}, nil
	}

	// String concatenation for add: [TScalar, TScalar] converts both
	// args to strings and concatenates. More specific signatures
	// (e.g. [TInteger, TInteger]) win due to higher specificity.
	// args[0] = nearest (top/forward), args[1] = farther. `a add b` → args=[b,a] → a+b.
	concatHandler := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		return []Value{NewString(valToString(args[1]) + valToString(args[0]))}, nil
	}

	numHandler := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		_as4, _ := args[0].AsNumber()
		_as3, _ := args[1].AsNumber()
		result := _as4 + _as3
		return []Value{NewDecimal(result)}, nil
	}

	// Temporal: Date + CalDuration → Date
	// date add 1 months → args[0]=CalDuration (nearest), args[1]=Date
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

	r.RegisterNativeFunc(NativeFunc{
		Name:              "add",
		ForwardPrecedence: true,
		Signatures: []NativeSig{
			{
				Args:    []Type{TInteger, TInteger},
				Handler: intHandler,
			},
			{
				Args:    []Type{TScalar, TScalar},
				Handler: concatHandler,
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
				Handler: addDateCalHandler,
			},
			{
				Args:    []Type{TClkDuration, TDateTime},
				Handler: addDtClkHandler,
			},
			{
				Args:    []Type{TClkDuration, TInstant},
				Handler: addInsClkHandler,
			},
			{
				Args:    []Type{TClkDuration, TDate},
				Handler: addDateClkHandler,
			},
		},
	})
}

// Ensure time import is used.
var _ = time.Now
