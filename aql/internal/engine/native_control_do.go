package engine

import "fmt"

// RegisterDo registers the "do" word.
//
// For lists, do evaluates the list as a sub-program:
//
//	do [1 add 2]  →  3
//
// For maps, word values are already resolved by autoEvalMap (called by
// execMatch before the handler runs). The handler additionally evaluates
// any remaining list values (depth-first), unwrapping single results:
//
//	do {r:rv}                  →  {r:10}    (word resolved by autoEvalMap)
//	do {x:[3 add 4]}          →  {x:7}     (list evaluated, single result unwrapped)
//	do {r:255, g:136, b:0}    →  {r:255, g:136, b:0}  (literals pass through)
func RegisterDo(r *Registry) {
	// promoteToWord converts a string or atom value to a word if it
	// names a registered function. With the current parser, list elements
	// inside maps are already words (word context), so this mainly
	// handles edge cases and backward compatibility.
	promoteToWord := func(v Value) Value {
		if v.VType.Matches(TString) || v.VType.Matches(TAtom) {
			name, _ := v.AsString()
			if r.Lookup(name) != nil {
				return NewWord(name)
			}
		}
		return v
	}

	evalList := func(elems []Value) ([]Value, error) {
		sub := New(r)
		input := make([]Value, len(elems))
		copy(input, elems)
		result, err := sub.Run(input)
		if err != nil {
			// Catch the error and leave it on the stack as an error value.
			return []Value{NewError(err)}, nil
		}
		return result, nil
	}

	// evalDataList evaluates a list from data context (inside a map).
	// Strings that name registered functions are promoted to words.
	evalDataList := func(elems []Value) ([]Value, error) {
		sub := New(r)
		input := make([]Value, len(elems))
		for i, e := range elems {
			input[i] = promoteToWord(e)
		}
		return sub.Run(input)
	}

	var evalMapValue func(v Value) (Value, error)
	evalMapValue = func(v Value) (Value, error) {
		if v.VType.Equal(TList) && v.Data != nil && !v.IsTypedList() && !v.IsTableType() {
			results, err := evalDataList(v.AsList().Slice())
			if err != nil {
				return Value{}, err
			}
			if len(results) == 1 {
				return results[0], nil
			}
			return NewList(results), nil
		}
		if v.VType.Equal(TMap) && v.Data != nil && !v.IsTypedMap() && !v.IsRecordType() && !v.IsOptionsType() {
			m := v.AsMap()
			out := NewOrderedMap()
			for _, key := range m.Keys() {
				val, _ := m.Get(key)
				evaluated, err := evalMapValue(val)
				if err != nil {
					return Value{}, err
				}
				out.Set(key, evaluated)
			}
			return NewMap(out), nil
		}
		return v, nil
	}

	r.RegisterNativeFunc(NativeFunc{
		Name:              "do",
		ForwardPrecedence: true,
		Signatures: []NativeSig{
			{
				Args:       []Type{TList},
				NoEvalArgs: map[int]bool{0: true},
				Handler: func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
					if !IsConcrete(args[0]) {
						return nil, fmt.Errorf("do: argument must be a concrete list, got type literal")
					}
					return evalList(args[0].AsList().Slice())
				},
				// Static type-check: when the body is a literal
				// list (concrete Data), run it through a sub-engine
				// in check mode and use the residual top-of-stack
				// carrier as the result. If the arg is a Word
				// reference to a def'd list, resolve via DefStacks
				// first. Falls back to TAny for truly computed
				// bodies.
				ReturnsFn: func(args []Value) []Value {
					body := args[0]
					if body.IsWord() {
						w, _ := body.AsWord()
						if v, ok := r.TopOfDefStack(w.Name); ok {
							body = v
						}
					}
					stk := RunCarrierBody(r, body)
					if len(stk) == 0 {
						return []Value{NewCarrier(TAny)}
					}
					return []Value{stk[len(stk)-1]}
				},
			},
			{
				Args: []Type{TMap},
				Handler: func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
					result, err := evalMapValue(args[0])
					if err != nil {
						return nil, err
					}
					return []Value{result}, nil
				},
				Returns: []Type{TAny},
			},
		},
	})
}
