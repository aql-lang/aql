package engine

import "fmt"

// registerCall registers "call", which takes a list and splices its contents
// onto the engine stack as code to execute (wrapped in a paren scope).
// This enables higher-order functions defined with def fn to invoke callback
// parameters.
//
//	[dup mul] call   => (evaluates "dup mul" on whatever is on the stack)
func registerCall(r *Registry) {
	r.RegisterNativeFunc(NativeFunc{
		Name:              "call",
		ForwardPrecedence: true,
		SkipSafetyCheck:   true,
		Signatures: []NativeSig{{
			Args:       []Type{TList},
			NoEvalArgs: map[int]bool{0: true},
			Handler: func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
				body := args[0]

				if body.Data == nil {
					return nil, fmt.Errorf("call: argument must be a concrete list, got type literal")
				}
				if body.IsTypedList() || body.IsTableType() {
					return nil, fmt.Errorf("call: argument must be a plain list")
				}

				bodyElems := body.AsList()
				if bodyElems.Len() == 0 {
					return nil, nil
				}

				bodyCopy := bodyElems.Slice()
				return bodyCopy, nil
			},
		}},
	})
}
