package engine

import "fmt"

// registerContext registers the "context" word that pushes the current
// context Store onto the stack.
//
// The context is a Store (Object/Store), allowing get/set to operate on it
// directly and prototype chain resolution for nested scopes.
func registerContext(r *Registry) {
	r.Register("context",
		Signature{
			Args: []Type{},
			Handler: func(_ []Value, _ map[string]Value, _ []Value, reg *Registry) ([]Value, error) {
				store := reg.ContextStore()
				if store == nil {
					return nil, fmt.Errorf("context: no active context")
				}
				return []Value{NewStoreValue(store)}, nil
			},
		},
	)
}
