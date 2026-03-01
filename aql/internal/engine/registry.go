package engine

import (
	"fmt"
	"strings"
)

// Function groups all signatures for a named function.
type Function struct {
	Name       string
	Signatures []Signature
}

// Registry maps function names to their definitions.
type Registry struct {
	funcs map[string]*Function
	Store map[string]Value // key-value store for set/get
}

// NewRegistry creates an empty registry.
func NewRegistry() *Registry {
	return &Registry{
		funcs: make(map[string]*Function),
		Store: make(map[string]Value),
	}
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

	// Arithmetic: each has prefix [int, int] and infix [int | int].
	// Precedence: add/sub=1, mul/div/mod=2 (higher binds tighter).
	registerBinaryIntOp(r, "add", 1, func(a, b int64) (int64, error) { return a + b, nil })
	registerBinaryIntOp(r, "sub", 1, func(a, b int64) (int64, error) { return a - b, nil })
	registerBinaryIntOp(r, "mul", 2, func(a, b int64) (int64, error) { return a * b, nil })
	registerBinaryIntOp(r, "div", 2, func(a, b int64) (int64, error) {
		if b == 0 {
			return 0, fmt.Errorf("division by zero")
		}
		return a / b, nil
	})
	registerBinaryIntOp(r, "mod", 2, func(a, b int64) (int64, error) {
		if b == 0 {
			return 0, fmt.Errorf("modulo by zero")
		}
		return a % b, nil
	})

	registerStorage(r)
	registerUnify(r)

	// Note: "end" is handled directly by the engine as a keyword,
	// not registered here. It terminates any pending forward early.
}

// storeKey converts a Value to a string key for the store.
func storeKey(v Value) string {
	if v.VType.Matches(TString) {
		return v.AsString()
	}
	return fmt.Sprintf("%v", v.Data)
}

func registerStorage(r *Registry) {
	setHandler := func(args []Value) ([]Value, error) {
		key := storeKey(args[0])
		r.Store[key] = args[1]
		return nil, nil
	}

	// set: [any, any] -> [] (prefix)
	//      [| any, any] -> [] (suffix)
	r.Register("set",
		Signature{
			Prefix:  []Type{TAny, TAny},
			Handler: setHandler,
		},
		Signature{
			Suffix:  []Type{TAny, TAny},
			Handler: setHandler,
		},
	)

	getHandler := func(args []Value) ([]Value, error) {
		key := storeKey(args[0])
		val, ok := r.Store[key]
		if !ok {
			return nil, fmt.Errorf("unknown key: %s", key)
		}
		return []Value{val}, nil
	}

	// get: [any] -> [any] (prefix)
	//      [| any] -> [any] (suffix)
	r.Register("get",
		Signature{
			Prefix:  []Type{TAny},
			Handler: getHandler,
		},
		Signature{
			Suffix:  []Type{TAny},
			Handler: getHandler,
		},
	)
}

// registerBinaryIntOp registers a binary integer operation with both a
// prefix signature [int, int] → [int] and an infix signature [int | int] → [int].
func registerBinaryIntOp(r *Registry, name string, prec int, op func(a, b int64) (int64, error)) {
	handler := func(args []Value) ([]Value, error) {
		result, err := op(args[0].AsInteger(), args[1].AsInteger())
		if err != nil {
			return nil, err
		}
		return []Value{NewInteger(result)}, nil
	}
	r.Register(name,
		// Prefix (Forth-style): 1 2 add → 3
		Signature{
			Prefix:     []Type{TInteger, TInteger},
			Precedence: prec,
			Handler:    handler,
		},
		// Infix (via forward): 1 add 2 → 3
		Signature{
			Prefix:     []Type{TInteger},
			Suffix:     []Type{TInteger},
			Precedence: prec,
			Handler:    handler,
		},
	)
}
