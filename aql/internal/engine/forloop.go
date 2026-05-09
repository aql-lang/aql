package engine

import "fmt"

// runForLoop builds the mark+body+move tokens for a for loop and returns
// them. The engine splices these onto the stack and processes them; the
// move's ForCont drives subsequent iterations via stepMoveCont.
//
// Break and continue use sentinel errors caught by the engine's Run loop,
// which delegates to handleLoopBreak/handleLoopContinue.
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
	InstallDef(r, iterName, NewInteger(start))

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
//	[end]              — 0 to end, step 1
//	[start, end]       — start to end, step 1
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

// ErrBreak, ErrContinue: re-exported from aqleng via aliases.go
// IsBreak: re-exported from aqleng via aliases.go
// IsContinue: re-exported from aqleng via aliases.go
