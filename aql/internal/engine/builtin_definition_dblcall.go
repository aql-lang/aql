package engine

import "fmt"

// registerDblcall registers "dblcall", an example function that takes an
// integer and a callback (list body). It doubles the integer, then invokes
// the callback with the doubled value on the stack.
//
// This demonstrates the general principle of a plain AQL function accepting
// a callback parameter. The callback body is spliced onto the main engine
// stack using internal mark/move markers — no sub-engine is created.
//
//	dblcall 5 [dup mul]   => 100  (doubles 5→10, then callback: 10 dup mul → 100)
//	3 dblcall [add 1]     => 7    (doubles 3→6, then callback: 6 add 1 → 7)
func registerDblcall(r *Registry) {
	r.Register("dblcall", Signature{
		Args: []Type{TInteger, TList},
		Handler: func(args []Value) ([]Value, error) {
			n := args[0].AsInteger()
			body := args[1]

			if body.IsTypedList() || body.IsTableType() {
				return nil, fmt.Errorf("dblcall: callback must be a plain list")
			}

			doubled := NewInteger(n * 2)

			bodyElems := body.AsList()
			if len(bodyElems) == 0 {
				return []Value{doubled}, nil
			}

			// Splice: ( doubled body_tokens... )
			// The open/close paren pair creates a sub-expression scope
			// on the main engine stack (no sub-engine). The callback
			// body executes with the doubled value on the stack, and
			// the paren scope collapses to the result.
			tokens := make([]Value, 0, len(bodyElems)+3)
			tokens = append(tokens, NewOpenParen())
			tokens = append(tokens, doubled)
			bodyCopy := make([]Value, len(bodyElems))
			copy(bodyCopy, bodyElems)
			tokens = append(tokens, bodyCopy...)
			tokens = append(tokens, NewWord(")"))
			return tokens, nil
		},
	})
}
