package native

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
//
// All [TNumber, TNumber] handlers compute b op a (i.e.
// args[1] op args[0]). Under §1.4 the swap form `a op b` is the
// preferred surface syntax, and binds args[0]=b, args[1]=a; the
// b-op-a body therefore yields the natural reading (`10 sub 3` → 7,
// `10 div 3` → 3). The mirror forms (`op a b`, `b op a`, `b a op`)
// produce the reversed result.
var mathNatives = []NativeFunc{
	{
		Name:        "add",
		ForwardArgs: true,
		Signatures: []NativeSig{
			{
				Args: []*Type{TNumber, TNumber},
				Handler: numericBinaryHandler(
					func(a, b int64) (Value, error) { return NewInteger(b + a), nil },
					func(a, b float64) (Value, error) { return NewDecimal(b + a), nil },
				),
				ReturnsFn: ReturnsNumericBinary(),
			},
			{Args: []*Type{TScalar, TScalar}, Handler: addConcatHandler, Returns: []*Type{TString}},
			{Args: []*Type{TDate, TCalDuration}, Handler: addDateCalHandler, Returns: []*Type{TDate}},
			{Args: []*Type{TDateTime, TClkDuration}, Handler: addDateTimeClkHandler, Returns: []*Type{TDateTime}},
			{Args: []*Type{TInstant, TClkDuration}, Handler: addInstantClkHandler, Returns: []*Type{TInstant}},
			{Args: []*Type{TDate, TClkDuration}, Handler: addDateClkHandler, Returns: []*Type{TDateTime}},
		},
	},
	{
		Name:        "sub",
		ForwardArgs: true,
		Signatures: []NativeSig{
			{
				Args: []*Type{TNumber, TNumber},
				Handler: numericBinaryHandler(
					func(a, b int64) (Value, error) { return NewInteger(b - a), nil },
					func(a, b float64) (Value, error) { return NewDecimal(b - a), nil },
				),
				ReturnsFn: ReturnsNumericBinary(),
			},
			{Args: []*Type{TDate, TCalDuration}, Handler: subDateCalHandler, Returns: []*Type{TDate}},
			{Args: []*Type{TDateTime, TClkDuration}, Handler: subDateTimeClkHandler, Returns: []*Type{TDateTime}},
			{Args: []*Type{TInstant, TClkDuration}, Handler: subInstantClkHandler, Returns: []*Type{TInstant}},
		},
	},
	{
		Name:        "mul",
		ForwardArgs: true,
		Signatures: []NativeSig{{
			Args: []*Type{TNumber, TNumber},
			Handler: numericBinaryHandler(
				func(a, b int64) (Value, error) { return NewInteger(b * a), nil },
				func(a, b float64) (Value, error) { return NewDecimal(b * a), nil },
			),
			ReturnsFn: ReturnsNumericBinary(),
		}},
	},
	{
		Name:        "div",
		ForwardArgs: true,
		Signatures: []NativeSig{{
			Args: []*Type{TNumber, TNumber},
			Handler: numericBinaryHandler(
				func(a, b int64) (Value, error) {
					if a == 0 {
						return Value{}, fmt.Errorf("division by zero")
					}
					return NewInteger(b / a), nil
				},
				func(a, b float64) (Value, error) {
					if a == 0 {
						return Value{}, fmt.Errorf("division by zero")
					}
					return NewDecimal(b / a), nil
				},
			),
			ReturnsFn: ReturnsNumericBinary(),
		}},
	},
	{
		Name:        "mod",
		ForwardArgs: true,
		Signatures: []NativeSig{{
			Args: []*Type{TNumber, TNumber},
			Handler: numericBinaryHandler(
				func(a, b int64) (Value, error) {
					if a == 0 {
						return Value{}, fmt.Errorf("modulo by zero")
					}
					return NewInteger(b % a), nil
				},
				func(a, b float64) (Value, error) {
					if a == 0 {
						return Value{}, fmt.Errorf("modulo by zero")
					}
					return NewDecimal(math.Mod(b, a)), nil
				},
			),
			ReturnsFn: ReturnsNumericBinary(),
		}},
	},
	{
		Name:        "pow",
		ForwardArgs: true,
		Signatures: []NativeSig{{
			Args: []*Type{TNumber, TNumber},
			Handler: numericBinaryHandler(
				func(a, b int64) (Value, error) {
					// Compute b ** a under §1.4 swap-form preference.
					if a < 0 {
						return Value{}, fmt.Errorf("pow: negative exponent %d", a)
					}
					result := int64(1)
					base := b
					exp := a
					for exp > 0 {
						if exp%2 == 1 {
							result *= base
						}
						base *= base
						exp /= 2
					}
					return NewInteger(result), nil
				},
				func(a, b float64) (Value, error) { return NewDecimal(math.Pow(b, a)), nil },
			),
			ReturnsFn: ReturnsNumericBinary(),
		}},
	},
}

func addConcatHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	return []Value{NewString(ValToString(args[1]) + ValToString(args[0]))}, nil
}

func addDateCalHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	t := AsDate(args[0])
	cd, _ := AsCalDuration(args[1])
	return []Value{NewDate(t.AddDate(cd.Years, cd.Months, cd.Days))}, nil
}

func addDateTimeClkHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	t := AsDateTime(args[0])
	d, _ := AsClkDuration(args[1])
	return []Value{NewDateTime(t.Add(d))}, nil
}

func addInstantClkHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	t := AsInstant(args[0])
	d, _ := AsClkDuration(args[1])
	return []Value{NewInstant(t.Add(d))}, nil
}

func addDateClkHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	t := AsDate(args[0])
	d, _ := AsClkDuration(args[1])
	return []Value{NewDateTime(t.Add(d))}, nil
}

func subDateCalHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	t := AsDate(args[0])
	cd, _ := AsCalDuration(args[1])
	return []Value{NewDate(t.AddDate(-cd.Years, -cd.Months, -cd.Days))}, nil
}

func subDateTimeClkHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	t := AsDateTime(args[0])
	d, _ := AsClkDuration(args[1])
	return []Value{NewDateTime(t.Add(-d))}, nil
}

func subInstantClkHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	t := AsInstant(args[0])
	d, _ := AsClkDuration(args[1])
	return []Value{NewInstant(t.Add(-d))}, nil
}
