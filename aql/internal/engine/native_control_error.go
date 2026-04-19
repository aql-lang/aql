package engine

import "fmt"

// registerError registers the "error" word.
//
// error consumes an Error value and a handler list from the stack,
// then evaluates the list with the error value left on the stack
// so the handler can inspect it.
//
//   - [Error List] → evaluates the list with the error on the stack
//
// Examples:
//
//	do [1 div 0] error [print]          # prints "division by zero"
//	do [1 div 0] error [drop 42]        # recovers with 42
func registerError(r *Registry) {
	// [List, Error] — evaluate the list with the error on the stack.
	// With ForwardPrecedence, the list (forward) fills sig[0],
	// the error (stack) fills sig[1].
	listHandler := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		if args[0].Data == nil {
			return nil, fmt.Errorf("error: handler must be a concrete list, got type literal")
		}
		sub := New(r)
		body := args[0].AsList().Slice()
		// Prepend the error value so it is on the stack when the handler runs.
		input := make([]Value, 0, 1+len(body))
		input = append(input, args[1])
		input = append(input, body...)
		return sub.Run(input)
	}

	r.RegisterNativeFunc(NativeFunc{
		Name:              "error",
		ForwardPrecedence: true,
		Signatures: []NativeSig{
			{
				Args:       []Type{TList, TError},
				NoEvalArgs: map[int]bool{0: true},
				Handler:    listHandler,
				Returns: []Type{TError},
			},
		},
	})
}
