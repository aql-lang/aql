package native

import (
	"github.com/metsitaba/voxgig-exp/aql/internal/engine"
)
// RegisterCancel registers the "cancel" word.
// cancel: [(Timeout or Interval)] -> [] — cancels a pending timeout or interval.
func RegisterCancel(r *engine.Registry) {
	r.RegisterNativeFunc(engine.NativeFunc{
		Name:              "cancel",
		ForwardPrecedence: true,
		Signatures: []engine.NativeSig{
			{
				Args: []engine.Type{engine.TTimeout},
				Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
					ti, err := args[0].AsTimeout()
					if err != nil {
						return nil, err
					}
					if ti.Timer != nil {
						ti.Timer.Stop()
						ti.Timer = nil
					}
					return nil, nil
				},
				Returns: []engine.Type{},
			},
			{
				Args: []engine.Type{engine.TInterval},
				Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
					ii, err := args[0].AsInterval()
					if err != nil {
						return nil, err
					}
					if ii.Ticker != nil {
						ii.Ticker.Stop()
						close(ii.Done)
						ii.Ticker = nil
					}
					return nil, nil
				},
				Returns: []engine.Type{},
			},
		},
	})
}
