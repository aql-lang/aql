package engine

import "fmt"

// storageNatives covers the `context` entry point. The `set` and
// `get` words moved into eng (eng/go/core_storage.go) and are
// installed via eng.RegisterCoreStorage from register.go.
var storageNatives = []NativeFunc{
	{
		Name:              "context",
		ForwardPrecedence: true,
		Signatures: []NativeSig{{
			Args:    []Type{},
			Handler: contextHandler,
			Returns: []Type{TStore},
		}},
	},
}

// ---- context handler ----

// contextHandler implements the "context" word that pushes the
// current context Store onto the stack.
//
// The context is a Store (Object/Store), allowing get/set to operate on it
// directly and prototype chain resolution for nested scopes.
func contextHandler(_ []Value, _ map[string]Value, _ []Value, reg *Registry) ([]Value, error) {
	store := reg.ContextStore()
	if store == nil {
		return nil, fmt.Errorf("context: no active context")
	}
	return []Value{NewStoreValue(store)}, nil
}

// CowSet: re-exported from aqleng via aliases.go
