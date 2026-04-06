package engine

// registerCancel registers the "cancel" word.
// cancel: [(Timeout or Interval)] -> [] — cancels a pending timeout or interval.
func registerCancel(r *Registry) {
	r.RegisterNativeFunc(NativeFunc{
		Name:              "cancel",
		ForwardPrecedence: true,
		Signatures: []NativeSig{
			{
				Args: []Type{TTimeout},
				Handler: func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
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
			},
			{
				Args: []Type{TInterval},
				Handler: func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
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
			},
		},
	})
}
