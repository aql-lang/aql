package engine

import "fmt"

// RegisterContext registers the "context" word that pushes the current
// context Store onto the stack.
//
// The context is a Store (Object/Store), allowing get/set to operate on it
// directly and prototype chain resolution for nested scopes.
func RegisterContext(r *Registry) {
	r.RegisterNativeFunc(NativeFunc{
		Name:              "context",
		ForwardPrecedence: true,
		Signatures: []NativeSig{{
			Args: []Type{},
			Handler: func(_ []Value, _ map[string]Value, _ []Value, reg *Registry) ([]Value, error) {
				store := reg.ContextStore()
				if store == nil {
					return nil, fmt.Errorf("context: no active context")
				}
				return []Value{NewStoreValue(store)}, nil
			},
			Returns: []Type{TStore},
		}},
	})
}
