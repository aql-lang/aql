package engine

import (
	"fmt"
	"time"
)

// registerSleep registers the "sleep" word.
// sleep: [Integer] -> [] — pauses execution for the given number of milliseconds.
func registerSleep(r *Registry) {
	r.RegisterNativeFunc(NativeFunc{
		Name:              "sleep",
		ForwardPrecedence: true,
		Signatures: []NativeSig{
			{
				Args: []Type{TInteger},
				Handler: func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
					ms, _ := args[0].AsInteger()
					if ms < 0 {
						return nil, fmt.Errorf("sleep: milliseconds must be non-negative, got %d", ms)
					}
					time.Sleep(time.Duration(ms) * time.Millisecond)
					return nil, nil
				},
				Returns: []Type{},
			},
		},
	})
}
