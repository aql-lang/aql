package engine

import "strings"

// Function groups all signatures for a named function.
type Function struct {
	Name       string
	Signatures []Signature
}

// Registry maps function names to their definitions.
type Registry struct {
	funcs map[string]*Function
}

// NewRegistry creates an empty registry.
func NewRegistry() *Registry {
	return &Registry{funcs: make(map[string]*Function)}
}

// Register adds one or more signatures to a named function.
func (r *Registry) Register(name string, sigs ...Signature) {
	fn, ok := r.funcs[name]
	if !ok {
		fn = &Function{Name: name}
		r.funcs[name] = fn
	}
	fn.Signatures = append(fn.Signatures, sigs...)
}

// Lookup returns the Function for a name, or nil.
func (r *Registry) Lookup(name string) *Function {
	return r.funcs[name]
}

// Match finds the best matching signature for a function name given the
// resolved stack state and word modifiers.
func (r *Registry) Match(name string, stack []Value, modifiers WordInfo) *MatchResult {
	fn := r.funcs[name]
	if fn == nil {
		return nil
	}
	return MatchSignature(fn.Signatures, stack, modifiers)
}

// DefaultRegistry returns a registry populated with built-in primitives.
func DefaultRegistry() *Registry {
	r := NewRegistry()
	registerBuiltins(r)
	return r
}

func registerBuiltins(r *Registry) {
	// upper: [string] -> [string]
	r.Register("upper", Signature{
		Prefix: []Type{TString},
		Handler: func(args []Value) ([]Value, error) {
			s := args[0].AsString()
			return []Value{NewString(strings.ToUpper(s))}, nil
		},
	})

	// lower: [string] -> [string]  (prefix)
	//        [|string] -> [string] (suffix)
	r.Register("lower",
		Signature{
			Prefix: []Type{TString},
			Handler: func(args []Value) ([]Value, error) {
				s := args[0].AsString()
				return []Value{NewString(strings.ToLower(s))}, nil
			},
		},
		Signature{
			Suffix: []Type{TString},
			Handler: func(args []Value) ([]Value, error) {
				s := args[0].AsString()
				return []Value{NewString(strings.ToLower(s))}, nil
			},
		},
	)

	// dup: [any] -> [any, any]
	r.Register("dup", Signature{
		Prefix: []Type{TAny},
		Handler: func(args []Value) ([]Value, error) {
			return []Value{args[0], args[0]}, nil
		},
	})

	// swap: [any, any] -> [any, any]
	r.Register("swap", Signature{
		Prefix: []Type{TAny, TAny},
		Handler: func(args []Value) ([]Value, error) {
			return []Value{args[1], args[0]}, nil
		},
	})

	// drop: [any] -> []
	r.Register("drop", Signature{
		Prefix: []Type{TAny},
		Handler: func(args []Value) ([]Value, error) {
			return nil, nil
		},
	})
}
