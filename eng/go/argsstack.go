package eng

import "errors"

// ArgsStack is the per-call args list stack. Each fn-body invocation
// (CallAQL, execFnDefSig) pushes the caller's args list before
// executing the body; the body retrieves the current list via the
// `args` word. Nested calls push their own list so each level sees
// only its own args.
//
// Extracted from Registry to match the DefTable / TypeTable /
// ContextStack pattern.
type ArgsStack struct {
	stack []Value
}

// errArgsStackNil is returned by every method when the receiver is
// nil. The production path constructs the stack through NewRegistry,
// so a nil receiver indicates a misconfigured (zero-initialised or
// partially built) Registry — a programming error that should
// surface rather than be silently ignored.
var errArgsStackNil = errors.New("argsstack: nil stack (registry was not initialised via NewRegistry)")

// NewArgsStack returns an empty args stack.
func NewArgsStack() *ArgsStack {
	return &ArgsStack{}
}

// Push pushes an args list onto the stack. Returns an error only if
// the receiver is nil; empty pushes are not possible.
func (as *ArgsStack) Push(args Value) error {
	if as == nil {
		return errArgsStackNil
	}
	as.stack = append(as.stack, args)
	return nil
}

// Pop pops the top args entry. Returns (true, nil) on success,
// (false, nil) when the stack is empty (a normal flow-control signal
// for callers that expect Pop to be a best-effort cleanup), or
// (false, error) when the receiver is nil.
func (as *ArgsStack) Pop() (bool, error) {
	if as == nil {
		return false, errArgsStackNil
	}
	if len(as.stack) == 0 {
		return false, nil
	}
	as.stack = as.stack[:len(as.stack)-1]
	return true, nil
}

// Top returns the current top args entry. Returns (value, true, nil)
// on success, (zero Value, false, nil) when the stack is empty (the
// `args` word relies on this to return an empty list outside any fn
// call), or (zero Value, false, error) when the receiver is nil.
func (as *ArgsStack) Top() (Value, bool, error) {
	if as == nil {
		return Value{}, false, errArgsStackNil
	}
	if len(as.stack) == 0 {
		return Value{}, false, nil
	}
	return as.stack[len(as.stack)-1], true, nil
}
