package engine

// isTruthy converts a Value to a boolean using the same rules as convert boolean:
// - booleans: direct value
// - numbers: non-zero is true
// - strings: "true" is true, "false" and "" are false, non-empty is true
// - atoms: same as string conversion
// - none: false
// - lists/maps: non-empty is true
func isTruthy(v Value) bool {
	switch {
	case v.VType.Matches(TBoolean):
		_as0, _ := v.AsBoolean()
		return _as0
	case v.VType.Matches(TInteger):
		_as1, _ := v.AsInteger()
		return _as1 != 0
	case v.VType.Equal(TNone):
		return false
	case v.VType.Equal(TList):
		if elems, ok := v.Data.([]Value); ok {
			return len(elems) > 0
		}
		return true
	case v.VType.Equal(TMap):
		if om, ok := v.Data.(*OrderedMap); ok {
			return om.Len() > 0
		}
		return true
	default:
		text := valToString(v)
		switch text {
		case "true":
			return true
		case "false", "":
			return false
		default:
			return text != ""
		}
	}
}

// spliceArg returns tokens for a branch value. If the value is a list,
// its elements are returned wrapped in parens so the main engine evaluates
// them as a sub-expression. Scalars are returned as-is.
func spliceArg(v Value) []Value {
	if v.VType.Equal(TList) && v.Data != nil && !v.IsTypedList() && !v.IsTableType() {
		elems := v.AsList()
		result := make([]Value, 0, elems.Len()+2)
		result = append(result, NewOpenParen())
		result = append(result, elems.Slice()...)
		result = append(result, NewWord(")"))
		return result
	}
	return []Value{v}
}

func registerIf(r *Registry) {
	// if: [any, any, any] -> [any] — 3-arg
	// "if cond then else": args=[cond, then, else]
	if3Handler := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		cond := args[0]
		thenBranch := spliceArg(args[1])
		elseBranch := spliceArg(args[2])

		// If condition is a list, use mark/move to evaluate it in-place.
		if cond.VType.Equal(TList) && cond.Data != nil && !cond.IsTypedList() && !cond.IsTableType() {
			condSlice := cond.AsList().Slice()
			id := NextMarkID()
			tokens := make([]Value, 0, len(condSlice)+2)
			tokens = append(tokens, NewMark(id, condSlice...))
			tokens = append(tokens, condSlice...)
			tokens = append(tokens, NewMoveIf(id, "if", &IfCont{
				Then: thenBranch,
				Else: elseBranch,
			}))
			return tokens, nil
		}

		// Scalar condition: evaluate immediately.
		if isTruthy(cond) {
			return thenBranch, nil
		}
		return elseBranch, nil
	}

	// if: [any, any] -> [any] — 2-arg
	// "if cond then": args=[cond, then]
	if2Handler := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		cond := args[0]
		thenBranch := spliceArg(args[1])

		// If condition is a list, use mark/move to evaluate it in-place.
		if cond.VType.Equal(TList) && cond.Data != nil && !cond.IsTypedList() && !cond.IsTableType() {
			condSlice := cond.AsList().Slice()
			id := NextMarkID()
			tokens := make([]Value, 0, len(condSlice)+2)
			tokens = append(tokens, NewMark(id, condSlice...))
			tokens = append(tokens, condSlice...)
			tokens = append(tokens, NewMoveIf(id, "if", &IfCont{
				Then: thenBranch,
				Else: nil,
			}))
			return tokens, nil
		}

		// Scalar condition: evaluate immediately.
		if isTruthy(cond) {
			return thenBranch, nil
		}
		return nil, nil
	}

	r.RegisterNativeFunc(NativeFunc{
		Name:              "if",
		ForwardPrecedence: true,
		Signatures: []NativeSig{
			{
				Args:       []Type{TAny, TAny, TAny},
				NoEvalArgs: map[int]bool{0: true, 1: true, 2: true},
				Handler:    if3Handler,
			},
			{
				Args:       []Type{TAny, TAny},
				NoEvalArgs: map[int]bool{0: true, 1: true},
				Handler:    if2Handler,
			},
		},
	})
}
