package native

import (
	"fmt"
	"github.com/metsitaba/voxgig-exp/aql/internal/engine"
	"time"
)

// RegisterInterval registers the "interval" word.
// interval: [Integer, (List/q or Word/q)] -> [Interval]
// Schedules repeated callback execution at the specified millisecond interval.
// The callback is executed with do semantics in a new sub-engine each tick.
func RegisterInterval(r *engine.Registry) {
	makeHandler := func(isList bool) engine.Handler {
		return func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
			ms, _ := args[0].AsConcreteInteger()
			if ms <= 0 {
				return nil, fmt.Errorf("interval: milliseconds must be positive, got %d", ms)
			}
			callback := args[1]

			id := engine.GenerateID("T_")
			ticker := time.NewTicker(time.Duration(ms) * time.Millisecond)
			done := make(chan struct{})

			go func() {
				for {
					select {
					case <-done:
						return
					case <-ticker.C:
						engine.RunTimerCallback(r, callback, isList)
					}
				}
			}()

			info := &engine.IntervalInfo{
				ID:     id,
				Ms:     ms,
				Ticker: ticker,
				Done:   done,
			}
			return []engine.Value{engine.NewInterval(info)}, nil
		}
	}

	r.RegisterNativeFunc(engine.NativeFunc{
		Name:              "interval",
		ForwardPrecedence: true,
		Signatures: []engine.NativeSig{
			{
				Args:      []engine.Type{engine.TInteger, engine.TList},
				QuoteArgs: map[int]bool{1: true},
				Handler:   makeHandler(true),
				Returns:   []engine.Type{engine.TInterval},
			},
			{
				Args:      []engine.Type{engine.TInteger, engine.TAtom},
				QuoteArgs: map[int]bool{1: true},
				Handler:   makeHandler(false),
				Returns:   []engine.Type{engine.TInterval},
			},
		},
	})
}
