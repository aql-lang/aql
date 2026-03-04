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
	funcs     map[string]*Function
	Store     map[string]Value   // key-value store for set/get
	DefStacks map[string][]Value // stacked bodies for def-defined words
}

// NewRegistry creates an empty registry.
func NewRegistry() *Registry {
	return &Registry{
		funcs:     make(map[string]*Function),
		Store:     make(map[string]Value),
		DefStacks: make(map[string][]Value),
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

	// Boolean: binary ops have Args:[boolean, boolean] with suffix precedence.
	// Precedence: or/xor/implies=1, and/nand=2 (higher binds tighter).
	// not is unary with suffix precedence and no precedence level.
	registerBinaryBoolOp(r, "or", 1, func(a, b bool) bool { return a || b })
	registerBinaryBoolOp(r, "and", 2, func(a, b bool) bool { return a && b })
	registerBinaryBoolOp(r, "xor", 1, func(a, b bool) bool { return a != b })
	registerBinaryBoolOp(r, "nand", 2, func(a, b bool) bool { return !(a && b) })
	registerBinaryBoolOp(r, "implies", 1, func(a, b bool) bool { return !a || b })

	// not: [boolean] -> [boolean]
	r.Register("not", Signature{
		Args: []Type{TBoolean},
		Handler: func(args []Value) ([]Value, error) {
			return []Value{NewBoolean(!args[0].AsBoolean())}, nil
		},
	})

	registerStorage(r)
	registerUnify(r)
	registerDef(r)
	registerUndef(r)
	registerVar(r)
	registerFn(r)
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

// registerBinaryBoolOp registers a binary boolean operation with a single
// signature Args:[boolean, boolean] and suffix precedence.
func registerBinaryBoolOp(r *Registry, name string, prec int, op func(a, b bool) bool) {
	handler := func(args []Value) ([]Value, error) {
		return []Value{NewBoolean(op(args[0].AsBoolean(), args[1].AsBoolean()))}, nil
	}
	r.Register(name, Signature{
		Args:       []Type{TBoolean, TBoolean},
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

// installDef registers a new word as a literal substitution or a typed
// function definition. Multiple defs for the same name stack; undef pops
// the top.
//
// When body is a FnDefInfo value (produced by the fn word), installDef
// registers typed signatures. Otherwise, body is stored directly as a
// literal substitution.
func installDef(r *Registry, name string, body Value) {
	if len(r.DefStacks[name]) == 0 {
		// First definition: register one generic fallback handler
		// that reads the top of the definition stack.
		r.Register(name, Signature{
			Handler: func(_ []Value) ([]Value, error) {
				stack := r.DefStacks[name]
				if len(stack) == 0 {
					return nil, fmt.Errorf("undefined: %s", name)
				}
				top := stack[len(stack)-1]
				// Guard: function definitions have typed signatures;
				// the generic handler should not expand them as literals.
				if _, ok := top.Data.(FnDefInfo); ok {
					return nil, fmt.Errorf("signature error: no matching signature for %s", name)
				}
				if top.VType.Equal(TList) {
					elems := top.AsList()
					result := make([]Value, len(elems))
					copy(result, elems)
					return result, nil
				}
				return []Value{top}, nil
			},
		})
	}

	// FnDefInfo body (from fn word): install typed signatures.
	if body.VType.Equal(TFnDef) {
		fnDef := body.Data.(FnDefInfo)
		installFnDef(r, name, fnDef)
		r.DefStacks[name] = append(r.DefStacks[name], body)
		return
	}

	r.DefStacks[name] = append(r.DefStacks[name], body)
}

// uninstallDef removes the most recent def for a word. If no definitions
// remain, the function entry is removed so the word falls through to
// normal resolution (unknown word → string).
func uninstallDef(r *Registry, name string) {
	stack := r.DefStacks[name]
	if len(stack) == 0 {
		return
	}

	top := stack[len(stack)-1]
	r.DefStacks[name] = stack[:len(stack)-1]

	// Count typed signatures to remove (function defs register N typed sigs).
	sigsToRemove := 0
	if fnDef, ok := top.Data.(FnDefInfo); ok {
		sigsToRemove = len(fnDef.Sigs)
	}

	fn := r.funcs[name]
	if fn == nil {
		return
	}

	// Remove typed signatures from the end.
	if sigsToRemove > 0 && len(fn.Signatures) >= sigsToRemove {
		fn.Signatures = fn.Signatures[:len(fn.Signatures)-sigsToRemove]
	}

	// If DefStacks is now empty, also remove the generic fallback handler.
	if len(r.DefStacks[name]) == 0 {
		if len(fn.Signatures) > 0 {
			fn.Signatures = fn.Signatures[:len(fn.Signatures)-1]
		}
		if len(fn.Signatures) == 0 {
			delete(r.funcs, name)
		}
		delete(r.DefStacks, name)
	}
}

// registerUndef registers the "undef" word for removing word definitions.
// undef removes the most recent definition, potentially revealing a
// shadowed one. Signature: [word|string] -> [].
func registerUndef(r *Registry) {
	undefHandler := func(args []Value) ([]Value, error) {
		name := defName(args[0])
		uninstallDef(r, name)
		return nil, nil
	}

	r.Register("undef",
		Signature{
			Args:    []Type{TWord},
			Handler: undefHandler,
		},
		Signature{
			Args:    []Type{TString},
			Handler: undefHandler,
		},
	)
}

// registerVar registers the "var" word for scoped variable definitions.
//
// var takes one list argument. The first element is a list of variable
// declarations. The remaining elements form the body. After the body,
// all variables are automatically undefined.
//
// Each declaration is either:
//   - A bare word: takes its value from the stack (def name end)
//   - A list [name value]: defines with the given value (def name value end)
//
// Example: var [[x] x mul x]  means  def x end x mul x undef x
// Example: var [[[x 2] y] x add y]  means  def x 2 end def y end x add y undef y undef x
func registerVar(r *Registry) {
	varHandler := func(args []Value) ([]Value, error) {
		list := args[0]
		if !list.VType.Equal(TList) {
			return nil, fmt.Errorf("var: argument must be a list")
		}
		elems := list.AsList()
		if len(elems) == 0 {
			return nil, fmt.Errorf("var: empty list")
		}

		// First element must be a list of variable declarations.
		declVal := elems[0]
		if !declVal.VType.Equal(TList) {
			return nil, fmt.Errorf("var: first element must be a list of variable declarations")
		}
		decls := declVal.AsList()
		body := elems[1:]

		var result []Value
		var varNames []string

		for _, decl := range decls {
			switch {
			case decl.IsWord():
				// Bare word: take value from stack.
				name := decl.AsWord().Name
				varNames = append(varNames, name)
				result = append(result, NewWord("def"), NewWord(name), NewWord("end"))

			case decl.VType.Equal(TList):
				// List [name value...]: define with given value.
				declElems := decl.AsList()
				if len(declElems) < 2 {
					return nil, fmt.Errorf("var: declaration list must have name and value")
				}
				var name string
				if declElems[0].IsWord() {
					name = declElems[0].AsWord().Name
				} else if declElems[0].VType.Matches(TString) {
					name = declElems[0].AsString()
				} else {
					return nil, fmt.Errorf("var: declaration name must be a word or string")
				}
				varNames = append(varNames, name)
				result = append(result, NewWord("def"), NewWord(name))
				result = append(result, declElems[1:]...)
				result = append(result, NewWord("end"))

			case decl.VType.Matches(TString):
				// String name: take value from stack.
				name := decl.AsString()
				varNames = append(varNames, name)
				result = append(result, NewWord("def"), NewWord(name), NewWord("end"))

			default:
				return nil, fmt.Errorf("var: invalid declaration: %s", decl.String())
			}
		}

		// Append body.
		result = append(result, body...)

		// Append undefs in reverse order.
		for i := len(varNames) - 1; i >= 0; i-- {
			result = append(result, NewWord("undef"), NewWord(varNames[i]))
		}

		return result, nil
	}

	r.Register("var", Signature{
		Args:    []Type{TList},
		Handler: varHandler,
	})
}

// --- Function specification helpers ---

// registerFn registers the "fn" word, which parses a list of signature
// triples into a FnDefInfo value. Use with def: def name fn [[params] [out] [body]].
func registerFn(r *Registry) {
	fnHandler := func(args []Value) ([]Value, error) {
		list := args[0]
		if !list.VType.Equal(TList) {
			return nil, fmt.Errorf("fn: argument must be a list")
		}
		elems := list.AsList()
		if len(elems) == 0 || len(elems)%3 != 0 {
			return nil, fmt.Errorf("fn: list must contain signature triples (length must be a multiple of 3)")
		}
		fnDef, err := parseFnDef(elems)
		if err != nil {
			return nil, err
		}
		return []Value{NewFnDef(fnDef)}, nil
	}

	r.Register("fn", Signature{
		Args:    []Type{TList},
		Handler: fnHandler,
	})
}

// parseFnDef parses a function specification list into FnDefInfo.
// The list contains signature triples: [input-sig, output-sig, body] ...
func parseFnDef(list []Value) (FnDefInfo, error) {
	var sigs []FnSig
	for i := 0; i < len(list); i += 3 {
		inputSig := list[i]
		// list[i+1] is the output signature — informational only
		body := list[i+2]

		params, err := parseFnParams(inputSig)
		if err != nil {
			return FnDefInfo{}, err
		}

		if !body.VType.Equal(TList) {
			return FnDefInfo{}, fmt.Errorf("function spec: body must be a list")
		}

		sigs = append(sigs, FnSig{
			Params: params,
			Body:   body.AsList(),
		})
	}
	return FnDefInfo{Sigs: sigs}, nil
}

// parseFnParams extracts parameters from an input signature list.
// Each element is either:
//   - A map with one key (named param from pair syntax): {x: type}
//   - A word (unnamed param): type name
//   - A type literal (Data==nil): already resolved type
func parseFnParams(inputSig Value) ([]FnParam, error) {
	if !inputSig.VType.Equal(TList) {
		return nil, fmt.Errorf("function spec: input signature must be a list")
	}
	elems := inputSig.AsList()
	var params []FnParam

	for _, elem := range elems {
		switch {
		case elem.VType.Equal(TMap):
			// Named parameter from pair syntax: {name: type}
			m := elem.AsMap()
			keys := m.Keys()
			if len(keys) != 1 {
				return nil, fmt.Errorf("function spec: parameter map must have exactly one key")
			}
			name := keys[0]
			typeVal, _ := m.Get(name)
			paramType := resolveSigType(typeVal)
			params = append(params, FnParam{Name: name, Type: paramType})

		case elem.IsWord():
			// Unnamed parameter: bare word is a type name
			typeName := elem.AsWord().Name
			paramType := resolveTypeName(typeName)
			params = append(params, FnParam{Type: paramType})

		case elem.Data == nil:
			// Type literal (already resolved by parser)
			params = append(params, FnParam{Type: elem.VType})

		default:
			return nil, fmt.Errorf("function spec: invalid parameter: %s", elem.String())
		}
	}

	return params, nil
}

// resolveSigType converts a Value (from a pair's value side) to a Type.
func resolveSigType(v Value) Type {
	if v.Data == nil {
		// Type literal (e.g., number, string) — already resolved by parser
		return v.VType
	}
	if v.IsWord() {
		return resolveTypeName(v.AsWord().Name)
	}
	if v.VType.Matches(TString) {
		return resolveTypeName(v.AsString())
	}
	return TAny
}

// resolveTypeName maps a type name string to its engine Type.
func resolveTypeName(name string) Type {
	switch name {
	case "any":
		return TAny
	case "none":
		return TNone
	case "number":
		return TNumber
	case "integer":
		return TInteger
	case "string":
		return TString
	case "boolean":
		return TBoolean
	case "list":
		return TList
	case "map":
		return TMap
	default:
		return NewType(name)
	}
}

// installFnDef registers typed signatures for a function definition.
// For each signature, it creates a handler that binds named parameters
// via installDef, returns body tokens, and appends undef cleanup.
func installFnDef(r *Registry, name string, fnDef FnDefInfo) {
	for _, sig := range fnDef.Sigs {
		argTypes := make([]Type, len(sig.Params))
		for i, p := range sig.Params {
			argTypes[i] = p.Type
		}
		s := sig // capture for closure
		handler := func(args []Value) ([]Value, error) {
			var result []Value
			var names []string
			for i, p := range s.Params {
				if p.Name != "" {
					installDef(r, p.Name, args[i])
					names = append(names, p.Name)
				} else {
					// Unnamed parameter: push value back for the body to use
					result = append(result, args[i])
				}
			}
			body := make([]Value, len(s.Body))
			copy(body, s.Body)
			result = append(result, body...)
			for i := len(names) - 1; i >= 0; i-- {
				result = append(result, NewWord("undef"), NewWord(names[i]))
			}
			return result, nil
		}
		r.Register(name, Signature{Args: argTypes, Handler: handler})
	}
}
