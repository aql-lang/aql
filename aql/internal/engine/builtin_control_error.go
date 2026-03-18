package engine

import "fmt"

// registerError registers the "error" word.
//
// error consumes an Error value from the stack and handles it:
//
//   - [Error] → prints the error description, consumes the error
//   - [Error List] → prints the error description, then evaluates
//     the list in a do block for custom handling
//
// Examples:
//
//	do [1 div 0] error              # prints "division by zero", continues
//	do [1 div 0] error [print "!"]  # prints "division by zero", then "!"
func registerError(r *Registry) {
	// [Error] — print the error and consume it.
	simpleHandler := func(args []Value) ([]Value, error) {
		info := args[0].AsError()
		fmt.Fprintf(r.Output, "  error: %s\n", info.Message)
		return nil, nil
	}

	// [Error List] — print the error, then evaluate the list as a
	// do block for custom error handling.
	listHandler := func(args []Value) ([]Value, error) {
		info := args[0].AsError()
		fmt.Fprintf(r.Output, "  error: %s\n", info.Message)
		sub := New(r)
		body := make([]Value, len(args[1].AsList()))
		copy(body, args[1].AsList())
		return sub.Run(body)
	}

	r.Register("error",
		Signature{
			Args:    []Type{TError, TList},
			Handler: listHandler,
		},
		Signature{
			Args:    []Type{TError},
			Handler: simpleHandler,
		},
	)
}
