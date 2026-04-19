package engine

import "time"

// registerNow registers the "now" word as a standard (non-module) native word.
// now: [] -> [Instant] — returns the current UTC instant.
func registerNow(r *Registry) {
	r.RegisterStackOnly("now", Signature{
		Args: []Type{},
		Handler: func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
			return []Value{NewInstant(time.Now())}, nil
		},
		Returns: []Type{TInstant},
	})
}
