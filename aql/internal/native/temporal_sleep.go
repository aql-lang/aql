package native

import (
	"fmt"
	"github.com/metsitaba/voxgig-exp/aql/internal/engine"
	"time"
)

// RegisterSleep registers the "sleep" word.
// sleep: [Integer] -> [] — pauses execution for the given number of milliseconds.
func RegisterSleep(r *engine.Registry) {
	r.RegisterNativeFunc(engine.NativeFunc{
		Name:              "sleep",
		ForwardPrecedence: true,
		Signatures: []engine.NativeSig{
			{
				Args: []engine.Type{engine.TInteger},
				Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
					ms, _ := args[0].AsConcreteInteger()
					if ms < 0 {
						return nil, fmt.Errorf("sleep: milliseconds must be non-negative, got %d", ms)
					}
					time.Sleep(time.Duration(ms) * time.Millisecond)
					return nil, nil
				},
				Returns: []engine.Type{},
			},
		},
	})
}
