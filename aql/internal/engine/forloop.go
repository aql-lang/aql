package engine

import "fmt"

// registerFor registers the "for" word for numeric iteration.
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
func registerFor(r *Registry) {
	// for [integer, list] — count from 0 to N-1
	forCountHandler := func(args []Value) ([]Value, error) {
		n := args[0].AsInteger()
		body := args[1]
		return runForLoop(r, 0, n, 1, "i", body)
	}

	// for [list, list] — range spec [end] or [start,end] or [start,end,step]
	forRangeHandler := func(args []Value) ([]Value, error) {
		if args[0].Data == nil {
			return nil, fmt.Errorf("for: range must be a concrete list, got type literal")
		}
		rangeSpec := args[0].AsList()
		body := args[1]
		start, end, step, err := parseRange(rangeSpec)
		if err != nil {
			return nil, fmt.Errorf("for: %w", err)
		}
		return runForLoop(r, start, end, step, "i", body)
	}

	r.Register("for",
		Signature{
			Args:    []Type{TInteger, TList},
			Handler: forCountHandler,
		},
		Signature{
			Args:    []Type{TList, TList},
			Handler: forRangeHandler,
		},
	)

	// break: stops the current for loop iteration and exits the loop.
	r.RegisterStackOnly("break", Signature{
		Handler: func(_ []Value) ([]Value, error) {
			return nil, errBreak
		},
	})

	// continue: stops the current iteration and moves to the next.
	r.RegisterStackOnly("continue", Signature{
		Handler: func(_ []Value) ([]Value, error) {
			return nil, errContinue
		},
	})
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
	bodyElems := body.AsList()

	// Install the iterator variable for the first iteration.
	installDef(r, iterName, NewInteger(start))

	// Create the continuation state.
	bodyCopy := make([]Value, len(bodyElems))
	copy(bodyCopy, bodyElems)

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
	tokens := make([]Value, 0, len(bodyElems)+2)
	tokens = append(tokens, NewMark(id, bodyElems...))
	bodyTokens := make([]Value, len(bodyElems))
	copy(bodyTokens, bodyElems)
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
		return 0, elems[0].AsInteger(), 1, nil
	case 2:
		if !elems[0].VType.Matches(TInteger) || !elems[1].VType.Matches(TInteger) {
			return 0, 0, 0, fmt.Errorf("range: expected integers")
		}
		return elems[0].AsInteger(), elems[1].AsInteger(), 1, nil
	case 3:
		if !elems[0].VType.Matches(TInteger) || !elems[1].VType.Matches(TInteger) || !elems[2].VType.Matches(TInteger) {
			return 0, 0, 0, fmt.Errorf("range: expected integers")
		}
		return elems[0].AsInteger(), elems[1].AsInteger(), elems[2].AsInteger(), nil
	default:
		return 0, 0, 0, fmt.Errorf("range: expected 1-3 elements, got %d", len(elems))
	}
}
