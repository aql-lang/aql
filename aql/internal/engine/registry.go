package engine

import (
	"fmt"
	"strings"
)

// Function groups all signatures for a named function.
type Function struct {
	Name             string
	Signatures       []Signature
	SuffixPrecedence bool // true = engine tries suffix-first; false = prefix-only
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

// Register adds one or more signatures to a named function with suffix precedence.
func (r *Registry) Register(name string, sigs ...Signature) {
	fn, ok := r.funcs[name]
	if !ok {
		fn = &Function{Name: name, SuffixPrecedence: true}
		r.funcs[name] = fn
	}
	fn.Signatures = append(fn.Signatures, sigs...)
}

// RegisterPrefixOnly adds signatures to a named function without suffix precedence.
func (r *Registry) RegisterPrefixOnly(name string, sigs ...Signature) {
	fn, ok := r.funcs[name]
	if !ok {
		fn = &Function{Name: name, SuffixPrecedence: false}
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
		Args: []Type{TString},
		Handler: func(args []Value) ([]Value, error) {
			s := args[0].AsString()
			return []Value{NewString(strings.ToUpper(s))}, nil
		},
	})

	// lower: [string] -> [string]
	r.Register("lower", Signature{
		Args: []Type{TString},
		Handler: func(args []Value) ([]Value, error) {
			s := args[0].AsString()
			return []Value{NewString(strings.ToLower(s))}, nil
		},
	})

	// dup: [any] -> [any, any] (prefix-only)
	dupHandler := func(args []Value) ([]Value, error) {
		return []Value{args[0], args[0]}, nil
	}
	r.RegisterPrefixOnly("dup", Signature{
		Args:    []Type{TAny},
		Handler: dupHandler,
	})

	// swap: [any, any] -> [any, any] (prefix-only)
	swapHandler := func(args []Value) ([]Value, error) {
		return []Value{args[1], args[0]}, nil
	}
	r.RegisterPrefixOnly("swap", Signature{
		Args:    []Type{TAny, TAny},
		Handler: swapHandler,
	})

	// drop: [any] -> [] (prefix-only)
	dropHandler := func(args []Value) ([]Value, error) {
		return nil, nil
	}
	r.RegisterPrefixOnly("drop", Signature{
		Args:    []Type{TAny},
		Handler: dropHandler,
	})

	// Arithmetic: each has Args:[int, int] with suffix precedence.
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
	registerDef(r)
}

// storeKey converts a Value to a string key for the store.
func storeKey(v Value) string {
	if v.IsWord() {
		return v.AsWord().Name
	}
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

	// set: key and value → store[key] = value
	// Three signatures for different key types:
	//   [TString, TAny] — string key (also handles coerced words)
	//   [TWord, TAny]   — word key (unknown word collected as word literal)
	//   [TAny, TAny]    — fallback for integer/other keys
	// Flexible matching handles reordering: "99 set foo end" →
	// [99, foo_word] → swap → [foo_word, 99] matching [TWord, TAny].
	// Registration order matters for tiebreaking: TString first so it wins
	// when peeking gives no disambiguation (e.g. paren expressions).
	r.Register("set",
		Signature{
			Args:    []Type{TString, TAny},
			Handler: setHandler,
		},
		Signature{
			Args:    []Type{TWord, TAny},
			Handler: setHandler,
		},
		Signature{
			Args:    []Type{TAny, TAny},
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

	// get: [any] -> [any]
	r.Register("get", Signature{
		Args:    []Type{TAny},
		Handler: getHandler,
	})
}

// registerBinaryIntOp registers a binary integer operation with a single
// signature Args:[int, int] and suffix precedence.
func registerBinaryIntOp(r *Registry, name string, prec int, op func(a, b int64) (int64, error)) {
	handler := func(args []Value) ([]Value, error) {
		result, err := op(args[0].AsInteger(), args[1].AsInteger())
		if err != nil {
			return nil, err
		}
		return []Value{NewInteger(result)}, nil
	}
	r.Register(name, Signature{
		Args:       []Type{TInteger, TInteger},
		Precedence: prec,
		Handler:    handler,
	})
}

// defName extracts a word name from a Value that is either a word or a string.
func defName(v Value) string {
	if v.IsWord() {
		return v.AsWord().Name
	}
	return v.AsString()
}

// registerDef registers the "def" word for defining new words.
//
// def creates literal substitutions: the body replaces the word during
// evaluation. If the body is a list, its elements are spliced into the
// stack. Otherwise the single value is pushed.
//
// Single handler, two signatures:
//
//	Args:[TWord, TAny]   – def name body  or  body def name
//	Args:[TString, TAny] – def "name" body  or  body def "name"
//
// Flexible matching handles reordering: in "body def name", suffix collects
// name(TWord), pushes it, then prefix sees [body, name] and flexible match
// reorders to [name, body] matching [TWord, TAny].
func registerDef(r *Registry) {
	defHandler := func(args []Value) ([]Value, error) {
		name := defName(args[0])
		body := args[1]
		installDef(r, name, body)
		return nil, nil
	}

	r.Register("def",
		// Args:[TWord, TAny] — word name
		Signature{
			Args:    []Type{TWord, TAny},
			Handler: defHandler,
		},
		// Args:[TString, TAny] — string name
		Signature{
			Args:    []Type{TString, TAny},
			Handler: defHandler,
		},
	)
}

// installDef registers a new word as a literal substitution.
func installDef(r *Registry, name string, body Value) {
	r.Register(name, Signature{
		Handler: func(_ []Value) ([]Value, error) {
			if body.VType.Equal(TList) {
				elems := body.AsList()
				result := make([]Value, len(elems))
				copy(result, elems)
				return result, nil
			}
			return []Value{body}, nil
		},
	})
}
