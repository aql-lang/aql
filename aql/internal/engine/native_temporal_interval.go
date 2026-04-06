package engine

import (
	"fmt"
	"time"
)

// registerInterval registers the "interval" word.
// interval: [Integer, (List/q or Word/q)] -> [Interval]
// Schedules repeated callback execution at the specified millisecond interval.
// The callback is executed with do semantics in a new sub-engine each tick.
func registerInterval(r *Registry) {
	makeHandler := func(isList bool) Handler {
		return func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
			ms, _ := args[0].AsInteger()
			if ms <= 0 {
				return nil, fmt.Errorf("interval: milliseconds must be positive, got %d", ms)
			}
			callback := args[1]

			id := GenerateID("T_")
			ticker := time.NewTicker(time.Duration(ms) * time.Millisecond)
			done := make(chan struct{})

			go func() {
				for {
					select {
					case <-done:
						return
					case <-ticker.C:
						runTimerCallback(r, callback, isList)
					}
				}
			}()

			info := &IntervalInfo{
				ID:     id,
				Ms:     ms,
				Ticker: ticker,
				Done:   done,
			}
			return []Value{NewInterval(info)}, nil
		}
	}

	r.RegisterNativeFunc(NativeFunc{
		Name:              "interval",
		ForwardPrecedence: true,
		Signatures: []NativeSig{
			{
				Args:      []Type{TInteger, TList},
				QuoteArgs: map[int]bool{1: true},
				Handler:   makeHandler(true),
			},
			{
				Args:      []Type{TInteger, TAtom},
				QuoteArgs: map[int]bool{1: true},
				Handler:   makeHandler(false),
			},
		},
	})
}
