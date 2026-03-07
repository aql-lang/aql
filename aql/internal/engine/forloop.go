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
// Each iteration runs the body in a sub-engine with only one copy of
// the body on the stack. This avoids the unlimited stack growth of
// eager body expansion. The iterator variable is installed via def
// and scoped to each iteration.
//
// Break and continue use sentinel errors to control the loop. Inside
// each sub-engine iteration, mark/move wrap the body so that continue
// can skip the remainder of the body by triggering a move back to the
// mark, which then terminates (one-shot).
func registerFor(r *Registry) {
	// for [integer, list] — count from 0 to N-1
	forCountHandler := func(args []Value) ([]Value, error) {
		n := args[0].AsInteger()
		body := args[1]
		return runForLoop(r, 0, n, 1, "i", body)
	}

	// for [list, list] — range spec [end] or [start,end] or [start,end,step]
	forRangeHandler := func(args []Value) ([]Value, error) {
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
	r.RegisterPrefixOnly("break", Signature{
		Handler: func(_ []Value) ([]Value, error) {
			return nil, errBreak
		},
	})

	// continue: stops the current iteration and moves to the next.
	r.RegisterPrefixOnly("continue", Signature{
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

// runForLoop executes a for loop from start to end (exclusive) with the
// given step. Each iteration runs the body in a sub-engine with a single
// copy of the body on the stack. Mark and move bracket the body inside
// each sub-engine so that the body can be replayed (one-shot) by continue.
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

	bodyElems := body.AsList()

	var results []Value
	for i := start; (step > 0 && i < end) || (step < 0 && i > end); i += step {
		// Install the iterator variable.
		installDef(r, iterName, NewInteger(i))

		// Build the sub-engine input: one copy of the body.
		input := make([]Value, len(bodyElems))
		copy(input, bodyElems)

		sub := New(r)
		out, err := sub.Run(input)

		// Uninstall the iterator variable.
		uninstallDef(r, iterName)

		if isBreak(err) {
			break
		}
		if isContinue(err) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("for: %w", err)
		}

		results = append(results, out...)
	}

	return results, nil
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
