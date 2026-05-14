package eng

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

// NewArgsStack returns an empty args stack.
func NewArgsStack() *ArgsStack {
	return &ArgsStack{}
}

// Push pushes an args list onto the stack.
func (as *ArgsStack) Push(args Value) {
	if as == nil {
		return
	}
	as.stack = append(as.stack, args)
}

// Pop pops the top args entry. Returns true if there was an entry to
// pop; silent on empty.
func (as *ArgsStack) Pop() bool {
	if as == nil || len(as.stack) == 0 {
		return false
	}
	as.stack = as.stack[:len(as.stack)-1]
	return true
}

// Top returns the current top args entry. Returns (zero Value, false)
// if the stack is empty.
func (as *ArgsStack) Top() (Value, bool) {
	if as == nil || len(as.stack) == 0 {
		return Value{}, false
	}
	return as.stack[len(as.stack)-1], true
}
