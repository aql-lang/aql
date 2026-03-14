package engine

import "fmt"

// registerContext registers the "context" word for scoped key-value storage.
// Usage:
//
//	context set "key" value   — store value under key in current context
//	context set foo value     — store with word key
//	context get "key"         — retrieve value (returns none if key not found)
//	context get foo           — retrieve with word key
func registerContext(r *Registry) {
	ctxSetHandler := func(args []Value) ([]Value, error) {
		ctx := r.Context()
		if ctx == nil {
			return nil, fmt.Errorf("context: no active context")
		}
		key := storeKey(args[0])
		ctx[key] = args[1]
		return nil, nil
	}

	ctxGetHandler := func(args []Value) ([]Value, error) {
		ctx := r.Context()
		if ctx == nil {
			return []Value{NewTypeLiteral(TNone)}, nil
		}
		key := storeKey(args[0])
		val, ok := ctx[key]
		if !ok {
			return []Value{NewTypeLiteral(TNone)}, nil
		}
		return []Value{val}, nil
	}

	// Register "context-set" and "context-get" as the implementation words.
	r.Register("context-set",
		Signature{Args: []Type{TString, TAny}, Handler: ctxSetHandler},
		Signature{Args: []Type{TWord, TAny}, Handler: ctxSetHandler},
		Signature{Args: []Type{TAny, TAny}, Handler: ctxSetHandler},
	)

	r.Register("context-get",
		Signature{Args: []Type{TString}, Handler: ctxGetHandler},
		Signature{Args: []Type{TWord}, Handler: ctxGetHandler},
		Signature{Args: []Type{TAny}, Handler: ctxGetHandler},
	)

	// Register "context" as a dispatcher that converts the sub-command
	// into the appropriate hyphenated word call.
	r.Register("context",
		Signature{
			Args: []Type{TWord},
			Handler: func(args []Value) ([]Value, error) {
				cmd := args[0].AsWord().Name
				switch cmd {
				case "set":
					return []Value{NewWord("context-set")}, nil
				case "get":
					return []Value{NewWord("context-get")}, nil
				default:
					return nil, fmt.Errorf("context: unknown sub-command: %s (expected set or get)", cmd)
				}
			},
		},
	)
}
