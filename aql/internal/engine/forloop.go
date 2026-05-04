package engine

import "fmt"

// RegisterFor registers the "for" word for numeric iteration.
//
// for takes two arguments: a range specification and a body list.
//
//	for 10 [print i]            — iterate 0..9, default iterator "i"
//	for [1,10] [print i]        — iterate 1..9
//	for [0,10,2] [print i]      — iterate 0,2,4,6,8
//
// Implementation: the for handler returns mark + body + move tokens that
// get spliced onto the caller's stack. The move carries a ForCont with
// iteration state. When the engine reaches the move after processing the
// body, stepMoveCont collects the iteration's results, advances the
// iterator, and either splices a fresh mark+body+move for the next
// iteration or finalizes the loop with accumulated results.
//
// This ensures only one copy of the body is on the stack at a time,
// eliminating the unlimited stack growth of eager body expansion.
//
// Break and continue use sentinel errors caught by the engine's Run loop,
// which delegates to handleLoopBreak/handleLoopContinue.
func RegisterFor(r *Registry) {
	// for [integer, list] — count from 0 to N-1
	forCountHandler := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		n, _ := args[0].AsInteger()
		body := args[1]
		return runForLoop(r, 0, n, 1, "i", body)
	}

	// for [list, list] — range spec [end] or [start,end] or [start,end,step]
	forRangeHandler := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		if args[0].Data == nil {
			return nil, fmt.Errorf("for: range must be a concrete list, got type literal")
		}
		rangeSpec := args[0].AsList().Slice()
		body := args[1]
		start, end, step, err := parseRange(rangeSpec)
		if err != nil {
			return nil, fmt.Errorf("for: %w", err)
		}
		return runForLoop(r, start, end, step, "i", body)
	}

	r.RegisterNativeFunc(NativeFunc{
		Name:              "for",
		ForwardPrecedence: true,
		Signatures: []NativeSig{
			{
				Args:       []Type{TInteger, TList},
				NoEvalArgs: map[int]bool{1: true},
				Handler:    forCountHandler,
				// for accumulates per-iteration results into a
				// list at runtime. Carrier model: analyse the body
				// once with the iterator (named "i") bound to an
				// Integer carrier, then return a typed list whose
				// element type is the body's top-of-stack.
				ReturnsFn: forCarrierReturns(r, "i", TInteger),
			},
			{
				Args:       []Type{TList, TList},
				NoEvalArgs: map[int]bool{1: true},
				Handler:    forRangeHandler,
				ReturnsFn:  forCarrierReturns(r, "i", TInteger),
			},
		},
	})

	// break: stops the current for loop iteration and exits the loop.
	r.RegisterNativeFunc(NativeFunc{
		Name:              "break",
		ForwardPrecedence: false,
		Signatures: []NativeSig{{
			Handler: func(_ []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
				return nil, errBreak
			},
			Returns: []Type{},
		}},
	})

	// continue: stops the current iteration and moves to the next.
	r.RegisterNativeFunc(NativeFunc{
		Name:              "continue",
		ForwardPrecedence: false,
		Signatures: []NativeSig{{
			Handler: func(_ []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
				return nil, errContinue
			},
			Returns: []Type{},
		}},
	})
}

// forCarrierReturns builds a ReturnsFn for the for-loop's carrier
// analysis. The iterator name (e.g. "i") is installed as a typed
// carrier on DefStacks for the duration of a single body analysis,
// then popped. Returns a typed list whose element type mirrors the
// body's residual top-of-stack.
func forCarrierReturns(r *Registry, iterName string, iterType Type) ReturnsFunc {
	return func(args []Value) []Value {
		// body is always the last arg (Integer,List or List,List).
		body := args[len(args)-1]
		r.PushDef(iterName, NewCarrier(iterType))
		stk, _ := runCarrierBodyWithDefs(r, body)
		r.PopDef(iterName)
		if len(stk) == 0 {
			return []Value{NewCarrier(TList)}
		}
		return []Value{NewCarrierTypedList(stk[len(stk)-1].VType)}
	}
}

// Sentinel errors for break and continue.
var (
	errBreak    = fmt.Errorf("break")
	errContinue = fmt.Errorf("continue")
)

// isBreak reports whether the error is a break sentinel.
func isBreak(err error) bool {
	return err == errBreak
}

// isContinue reports whether the error is a continue sentinel.
func isContinue(err error) bool {
	return err == errContinue
}

// runForLoop builds the mark+body+move tokens for a for loop and returns
// them. The engine splices these onto the stack and processes them; the
// move's ForCont drives subsequent iterations via stepMoveCont.
func runForLoop(r *Registry, start, end, step int64, iterName string, body Value) ([]Value, error) {
	if step == 0 {
		return nil, fmt.Errorf("for: step cannot be zero")
	}
	if step > 0 && start >= end {
		return nil, nil
	}
	if step < 0 && start <= end {
		return nil, nil
	}

	if body.Data == nil {
		return nil, fmt.Errorf("for: body must be a concrete list, got type literal")
	}
	bodySlice := body.AsList().Slice()

	// Install the iterator variable for the first iteration.
	installDef(r, iterName, NewInteger(start))

	// Create the continuation state.
	bodyCopy := make([]Value, len(bodySlice))
	copy(bodyCopy, bodySlice)

	cont := &ForCont{
		Registry: r,
		IterName: iterName,
		Current:  start,
		End:      end,
		Step:     step,
		Body:     bodyCopy,
	}

	// Build the stack segment: mark + body + move.
	id := NextMarkID()
	tokens := make([]Value, 0, len(bodySlice)+2)
	tokens = append(tokens, NewMark(id, bodySlice...))
	bodyTokens := make([]Value, len(bodySlice))
	copy(bodyTokens, bodySlice)
	tokens = append(tokens, bodyTokens...)
	tokens = append(tokens, NewMoveCont(id, "for loop", cont))

	return tokens, nil
}

// parseRange parses a range specification list into start, end, step.
//
//	[end]             — 0 to end, step 1
//	[start, end]      — start to end, step 1
//	[start, end, step] — start to end, step
func parseRange(elems []Value) (start, end, step int64, err error) {
	switch len(elems) {
	case 1:
		if !elems[0].VType.Matches(TInteger) {
			return 0, 0, 0, fmt.Errorf("range: expected integer, got %s", elems[0].VType)
		}
		_as0, _ := elems[0].AsInteger()
		return 0, _as0, 1, nil
	case 2:
		if !elems[0].VType.Matches(TInteger) || !elems[1].VType.Matches(TInteger) {
			return 0, 0, 0, fmt.Errorf("range: expected integers")
		}
		_as2, _ := elems[0].AsInteger()
		_as1, _ := elems[1].AsInteger()
		return _as2, _as1, 1, nil
	case 3:
		if !elems[0].VType.Matches(TInteger) || !elems[1].VType.Matches(TInteger) || !elems[2].VType.Matches(TInteger) {
			return 0, 0, 0, fmt.Errorf("range: expected integers")
		}
		_as5, _ := elems[0].AsInteger()
		_as4, _ := elems[1].AsInteger()
		_as3, _ := elems[2].AsInteger()
		return _as5, _as4, _as3, nil
	default:
		return 0, 0, 0, fmt.Errorf("range: expected 1-3 elements, got %d", len(elems))
	}
}
