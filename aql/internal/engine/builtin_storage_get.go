package engine

import "fmt"

func registerGet(r *Registry) {
	getHandler := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		// If the argument was already resolved (e.g. a defined word
		// that the engine evaluated before get collected it), return
		// it directly — no Store lookup needed.
		if args[0].VType.Matches(TMap) || args[0].VType.Matches(TList) ||
			args[0].VType.Matches(TFunction) || args[0].VType.Equal(TFnDef) {
			return []Value{args[0]}, nil
		}
		key := storeKey(args[0])
		val, ok := r.Store[key]
		if !ok {
			return nil, fmt.Errorf("unknown key: %s", key)
		}
		return []Value{val}, nil
	}

	// get: [any] -> [any]
	r.Register("get", Signature{
		Args:    []Type{TAny},
		Handler: getHandler,
	})
}
