package engine

import (
	"fmt"
	"math"
)

// mathNatives are the basic arithmetic words: add, sub, mul, div,
// mod, pow. Each shares a [TNumber, TNumber] base signature wired
// through numericBinaryHandler; add/sub additionally carry the
// temporal overloads (date+CalDuration, datetime+ClkDuration, etc.)
// and add carries the [TScalar, TScalar] string-concatenation
// overload used when both inputs coerce to strings.
var mathNatives = []NativeFunc{
	{
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
			{Args: []Type{TScalar, TScalar}, Handler: addConcatHandler, Returns: []Type{TString}},
			{Args: []Type{TDate, TCalDuration}, Handler: addDateCalHandler, Returns: []Type{TDate}},
			{Args: []Type{TDateTime, TClkDuration}, Handler: addDateTimeClkHandler, Returns: []Type{TDateTime}},
			{Args: []Type{TInstant, TClkDuration}, Handler: addInstantClkHandler, Returns: []Type{TInstant}},
			{Args: []Type{TDate, TClkDuration}, Handler: addDateClkHandler, Returns: []Type{TDateTime}},
		},
	},
	{
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
			{Args: []Type{TDate, TCalDuration}, Handler: subDateCalHandler, Returns: []Type{TDate}},
			{Args: []Type{TDateTime, TClkDuration}, Handler: subDateTimeClkHandler, Returns: []Type{TDateTime}},
			{Args: []Type{TInstant, TClkDuration}, Handler: subInstantClkHandler, Returns: []Type{TInstant}},
		},
	},
	{
		Name:              "mul",
		ForwardPrecedence: true,
		Signatures: []NativeSig{{
			Args: []Type{TNumber, TNumber},
			Handler: numericBinaryHandler(
				func(a, b int64) (Value, error) { return NewInteger(a * b), nil },
				func(a, b float64) (Value, error) { return NewDecimal(a * b), nil },
			),
			ReturnsFn: ReturnsNumericBinary(),
		}},
	},
	{
		Name:              "div",
		ForwardPrecedence: true,
		Signatures: []NativeSig{{
			Args: []Type{TNumber, TNumber},
			Handler: numericBinaryHandler(
				func(a, b int64) (Value, error) {
					if b == 0 {
						return Value{}, fmt.Errorf("division by zero")
					}
					return NewInteger(a / b), nil
				},
				func(a, b float64) (Value, error) {
					if b == 0 {
						return Value{}, fmt.Errorf("division by zero")
					}
					return NewDecimal(a / b), nil
				},
			),
			ReturnsFn: ReturnsNumericBinary(),
		}},
	},
	{
		Name:              "mod",
		ForwardPrecedence: true,
		Signatures: []NativeSig{{
			Args: []Type{TNumber, TNumber},
			Handler: numericBinaryHandler(
				func(a, b int64) (Value, error) {
					if b == 0 {
						return Value{}, fmt.Errorf("modulo by zero")
					}
					return NewInteger(a % b), nil
				},
				func(a, b float64) (Value, error) {
					if b == 0 {
						return Value{}, fmt.Errorf("modulo by zero")
					}
					return NewDecimal(math.Mod(a, b)), nil
				},
			),
			ReturnsFn: ReturnsNumericBinary(),
		}},
	},
	{
		Name:              "pow",
		ForwardPrecedence: true,
		Signatures: []NativeSig{{
			Args: []Type{TNumber, TNumber},
			Handler: numericBinaryHandler(
				func(base, exp int64) (Value, error) {
					if exp < 0 {
						return Value{}, fmt.Errorf("pow: negative exponent %d", exp)
					}
					result := int64(1)
					b := base
					e := exp
					for e > 0 {
						if e%2 == 1 {
							result *= b
						}
						b *= b
						e /= 2
					}
					return NewInteger(result), nil
				},
				func(base, exp float64) (Value, error) { return NewDecimal(math.Pow(base, exp)), nil },
			),
			ReturnsFn: ReturnsNumericBinary(),
		}},
	},
}

func addConcatHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	return []Value{NewString(ValToString(args[0]) + ValToString(args[1]))}, nil
}

func addDateCalHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	t := args[0].AsDate()
	cd, _ := args[1].AsCalDuration()
	return []Value{NewDate(t.AddDate(cd.Years, cd.Months, cd.Days))}, nil
}

func addDateTimeClkHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	t := args[0].AsDateTime()
	d, _ := args[1].AsClkDuration()
	return []Value{NewDateTime(t.Add(d))}, nil
}

func addInstantClkHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	t := args[0].AsInstant()
	d, _ := args[1].AsClkDuration()
	return []Value{NewInstant(t.Add(d))}, nil
}

func addDateClkHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	t := args[0].AsDate()
	d, _ := args[1].AsClkDuration()
	return []Value{NewDateTime(t.Add(d))}, nil
}

func subDateCalHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	t := args[0].AsDate()
	cd, _ := args[1].AsCalDuration()
	return []Value{NewDate(t.AddDate(-cd.Years, -cd.Months, -cd.Days))}, nil
}

func subDateTimeClkHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	t := args[0].AsDateTime()
	d, _ := args[1].AsClkDuration()
	return []Value{NewDateTime(t.Add(-d))}, nil
}

func subInstantClkHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	t := args[0].AsInstant()
	d, _ := args[1].AsClkDuration()
	return []Value{NewInstant(t.Add(-d))}, nil
}
