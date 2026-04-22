package engine

import (
	"fmt"
	"time"
)

// RegisterTimeout registers the "timeout" word.
// timeout: [Integer, (List/q or Word/q)] -> [Timeout]
// Schedules callback execution after the specified milliseconds.
// The callback is executed with do semantics in a new sub-engine.
func RegisterTimeout(r *Registry) {
	makeHandler := func(isList bool) Handler {
		return func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
			ms, _ := args[0].AsInteger()
			if ms < 0 {
				return nil, fmt.Errorf("timeout: milliseconds must be non-negative, got %d", ms)
			}
			callback := args[1]

			id := GenerateID("T_")
			timer := time.AfterFunc(time.Duration(ms)*time.Millisecond, func() {
				RunTimerCallback(r, callback, isList)
			})

			info := &TimeoutInfo{
				ID:    id,
				Ms:    ms,
				Timer: timer,
			}
			return []Value{NewTimeout(info)}, nil
		}
	}

	r.RegisterNativeFunc(NativeFunc{
		Name:              "timeout",
		ForwardPrecedence: true,
		Signatures: []NativeSig{
			{
				Args:      []Type{TInteger, TList},
				QuoteArgs: map[int]bool{1: true},
				Handler:   makeHandler(true),
				Returns:   []Type{TTimeout},
			},
			{
				Args:      []Type{TInteger, TAtom},
				QuoteArgs: map[int]bool{1: true},
				Handler:   makeHandler(false),
				Returns:   []Type{TTimeout},
			},
		},
	})
}

// RunTimerCallback executes a timer callback with do semantics.
// For lists, it runs the list elements as a sub-program.
// For words/atoms, it looks up the word and executes it.
func RunTimerCallback(r *Registry, callback Value, isList bool) {
	sub := New(r)
	var input []Value
	if isList {
		if callback.Data == nil {
			return
		}
		input = make([]Value, len(callback.AsList().Slice()))
		copy(input, callback.AsList().Slice())
	} else {
		name, _ := callback.AsString()
		input = []Value{NewWord(name)}
	}
	// Execute and discard results — timer callbacks run for side effects.
	_, _ = sub.Run(input)
}
