package engine

import (
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/metsitaba/voxgig-exp/aql/internal/fileops"
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
	FileOps   fileops.FileOps    // file operations for read/write words
	Formats   map[string]Format  // format registry for read/write (keyed by name)
	Output    io.Writer          // output writer for print/printstr and stdout
	ErrOutput io.Writer          // error output writer for stderr
	Input     io.Reader          // input reader for stdin
	SQLite    *SQLiteStore       // in-memory SQLite store for table data
}

// NewRegistry creates an empty registry.
func NewRegistry() *Registry {
	sqlStore, err := NewSQLiteStore()
	if err != nil {
		panic("failed to initialize SQLite store: " + err.Error())
	}
	return &Registry{
		funcs:     make(map[string]*Function),
		Store:     make(map[string]Value),
		DefStacks: make(map[string][]Value),
		FileOps:   fileops.NewDefault(),
		Formats:   DefaultFormats(),
		Output:    os.Stdout,
		ErrOutput: os.Stderr,
		Input:     os.Stdin,
		SQLite:    sqlStore,
	}
}

// SetFileOps replaces the file operations implementation.
func (r *Registry) SetFileOps(ops fileops.FileOps) {
	r.FileOps = ops
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
	// upper: [string|atom] -> [string]
	upperHandler := func(args []Value) ([]Value, error) {
		s := args[0].Data.(string)
		return []Value{NewString(strings.ToUpper(s))}, nil
	}
	r.Register("upper",
		Signature{Args: []Type{TString}, Handler: upperHandler},
		Signature{Args: []Type{TAtom}, Handler: upperHandler},
	)

	// lower: [string|atom] -> [string]
	lowerHandler := func(args []Value) ([]Value, error) {
		s := args[0].Data.(string)
		return []Value{NewString(strings.ToLower(s))}, nil
	}
	r.Register("lower",
		Signature{Args: []Type{TString}, Handler: lowerHandler},
		Signature{Args: []Type{TAtom}, Handler: lowerHandler},
	)

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

	// over: [a, b] -> [a, b, a] (prefix-only)
	r.RegisterPrefixOnly("over", Signature{
		Args: []Type{TAny, TAny},
		Handler: func(args []Value) ([]Value, error) {
			return []Value{args[0], args[1], args[0]}, nil
		},
	})

	// rot: [a, b, c] -> [b, c, a] (prefix-only)
	r.RegisterPrefixOnly("rot", Signature{
		Args: []Type{TAny, TAny, TAny},
		Handler: func(args []Value) ([]Value, error) {
			return []Value{args[1], args[2], args[0]}, nil
		},
	})

	// nip: [a, b] -> [b] (prefix-only)
	r.RegisterPrefixOnly("nip", Signature{
		Args: []Type{TAny, TAny},
		Handler: func(args []Value) ([]Value, error) {
			return []Value{args[1]}, nil
		},
	})

	// tuck: [a, b] -> [b, a, b] (prefix-only)
	r.RegisterPrefixOnly("tuck", Signature{
		Args: []Type{TAny, TAny},
		Handler: func(args []Value) ([]Value, error) {
			return []Value{args[1], args[0], args[1]}, nil
		},
	})

	// 2dup: [a, b] -> [a, b, a, b] (prefix-only)
	r.RegisterPrefixOnly("2dup", Signature{
		Args: []Type{TAny, TAny},
		Handler: func(args []Value) ([]Value, error) {
			return []Value{args[0], args[1], args[0], args[1]}, nil
		},
	})

	// 2swap: [a, b, c, d] -> [c, d, a, b] (prefix-only)
	r.RegisterPrefixOnly("2swap", Signature{
		Args: []Type{TAny, TAny, TAny, TAny},
		Handler: func(args []Value) ([]Value, error) {
			return []Value{args[2], args[3], args[0], args[1]}, nil
		},
	})

	// 2drop: [a, b] -> [] (prefix-only)
	r.RegisterPrefixOnly("2drop", Signature{
		Args: []Type{TAny, TAny},
		Handler: func(args []Value) ([]Value, error) {
			return nil, nil
		},
	})

	// 2over: [a, b, c, d] -> [a, b, c, d, a, b] (prefix-only)
	r.RegisterPrefixOnly("2over", Signature{
		Args: []Type{TAny, TAny, TAny, TAny},
		Handler: func(args []Value) ([]Value, error) {
			return []Value{args[0], args[1], args[2], args[3], args[0], args[1]}, nil
		},
	})

	// depth: [] -> [n] pushes the number of items on the stack (prefix-only)
	r.RegisterPrefixOnly("depth", Signature{
		FullStackHandler: func(args []Value, stack []Value) ([]Value, error) {
			return append(stack, NewInteger(int64(len(stack)))), nil
		},
	})

	// pick: [n] -> [v] copies the nth item from the stack (0-indexed from top) (prefix-only)
	// 1 2 3 0 pick → 1 2 3 3   (pick 0 = dup)
	// 1 2 3 2 pick → 1 2 3 1   (pick 2 = copy bottom)
	r.RegisterPrefixOnly("pick", Signature{
		Args: []Type{TInteger},
		FullStackHandler: func(args []Value, stack []Value) ([]Value, error) {
			n := int(args[0].AsInteger())
			if n < 0 || n >= len(stack) {
				return nil, fmt.Errorf("pick: index %d out of range (stack depth %d)", n, len(stack))
			}
			return append(stack, stack[len(stack)-1-n]), nil
		},
	})

	// roll: [n] -> [] rotates the nth item to the top (0-indexed from top) (prefix-only)
	// 1 2 3 2 roll → 2 3 1   (roll 2 = rot)
	// 1 2 3 1 roll → 1 3 2   (roll 1 = swap)
	r.RegisterPrefixOnly("roll", Signature{
		Args: []Type{TInteger},
		FullStackHandler: func(args []Value, stack []Value) ([]Value, error) {
			n := int(args[0].AsInteger())
			if n < 0 || n >= len(stack) {
				return nil, fmt.Errorf("roll: index %d out of range (stack depth %d)", n, len(stack))
			}
			// Rotate: remove stack[len-1-n], append it on top.
			idx := len(stack) - 1 - n
			result := make([]Value, 0, len(stack))
			result = append(result, stack[:idx]...)
			result = append(result, stack[idx+1:]...)
			result = append(result, stack[idx])
			return result, nil
		},
	})

	// Arithmetic: each has Args:[int, int] with suffix precedence.
	// Precedence: add/sub=1, mul/div/mod=2 (higher binds tighter).
	registerBinaryIntOp(r, "add", 1, func(a, b int64) (int64, error) { return a + b, nil })

	// String concatenation for add: [TScalar, TScalar] converts both
	// args to strings and concatenates. The [TInteger, TInteger] sig
	// from registerBinaryIntOp has higher specificity (204 vs 202) so
	// it wins for two integers when combined with prefix/peek bonuses.
	r.Register("add", Signature{
		Args:       []Type{TScalar, TScalar},
		Precedence: 1,
		Handler: func(args []Value) ([]Value, error) {
			return []Value{NewString(valToString(args[0]) + valToString(args[1]))}, nil
		},
	})
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

	// abs: [int] -> [int] (suffix precedence)
	r.Register("abs", Signature{
		Args: []Type{TInteger},
		Handler: func(args []Value) ([]Value, error) {
			v := args[0].AsInteger()
			if v < 0 {
				v = -v
			}
			return []Value{NewInteger(v)}, nil
		},
	})

	// negate: [int] -> [int] (suffix precedence)
	r.Register("negate", Signature{
		Args: []Type{TInteger},
		Handler: func(args []Value) ([]Value, error) {
			return []Value{NewInteger(-args[0].AsInteger())}, nil
		},
	})

	// min: [int, int] -> [int] (suffix precedence)
	registerBinaryIntOp(r, "min", 1, func(a, b int64) (int64, error) {
		if a < b {
			return a, nil
		}
		return b, nil
	})

	// max: [int, int] -> [int] (suffix precedence)
	registerBinaryIntOp(r, "max", 1, func(a, b int64) (int64, error) {
		if a > b {
			return a, nil
		}
		return b, nil
	})

	// Boolean: binary ops have Args:[boolean, boolean] with suffix precedence.
	// Precedence: or/xor/implies=1, and/nand=2 (higher binds tighter).
	// not is unary with suffix precedence and no precedence level.
	registerBinaryBoolOp(r, "or", 1, func(a, b bool) bool { return a || b })

	// or for non-boolean values: creates a disjunct (union type).
	// The boolean signature (specificity 202) ties with this (202), but the
	// boolean signature is registered first, so it wins for boolean args.
	r.Register("or", Signature{
		Args:       []Type{TAny, TAny},
		Precedence: 1,
		Handler: func(args []Value) ([]Value, error) {
			var alts []Value
			// Flatten left side if already a disjunct.
			if args[0].IsDisjunct() {
				alts = append(alts, args[0].AsDisjunct().Alternatives...)
			} else {
				alts = append(alts, args[0])
			}
			// Flatten right side if already a disjunct.
			if args[1].IsDisjunct() {
				alts = append(alts, args[1].AsDisjunct().Alternatives...)
			} else {
				alts = append(alts, args[1])
			}
			return []Value{NewDisjunct(alts)}, nil
		},
	})

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

	registerComparison(r)

	registerStorage(r)
	registerUnify(r)
	registerDef(r)
	registerUndef(r)
	registerVar(r)
	registerFn(r)
	registerConvert(r)
	registerRecord(r)
	registerTable(r)
	registerMake(r)
	registerTypeDef(r)
	registerDo(r)
	registerTypeof(r)
	registerBase(r)
	registerFileIO(r)
	registerQuery(r)
	registerIf(r)
	registerPrint(r)
	registerDot(r)
	registerDotr(r)
	registerTrace(r)
}

// valToString converts any scalar Value to its string representation.
func valToString(v Value) string {
	switch {
	case v.VType.Matches(TString):
		return v.AsString()
	case v.IsAtom():
		return v.AsAtom()
	case v.VType.Matches(TInteger):
		return strconv.FormatInt(v.AsInteger(), 10)
	case v.VType.Matches(TBoolean):
		if v.AsBoolean() {
			return "true"
		}
		return "false"
	default:
		return v.String()
	}
}

// storeKey converts a Value to a string key for the store.
func storeKey(v Value) string {
	if v.IsWord() {
		return v.AsWord().Name
	}
	if v.VType.Matches(TString) {
		return v.AsString()
	}
	if v.IsAtom() {
		return v.AsAtom()
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

// defPrefixOnly returns true if the name word carries the /p modifier,
// indicating the defined word should be prefix-only (not suffix precedence).
func defPrefixOnly(v Value) bool {
	if v.IsWord() {
		return v.AsWord().ForcePrefix
	}
	return false
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
		prefixOnly := defPrefixOnly(args[0])
		body := args[1]
		installDef(r, name, body, prefixOnly)
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
func installDef(r *Registry, name string, body Value, prefixOnly ...bool) {
	isPrefixOnly := len(prefixOnly) > 0 && prefixOnly[0]
	registerFn := r.Register
	if isPrefixOnly {
		registerFn = r.RegisterPrefixOnly
	}
	if len(r.DefStacks[name]) == 0 {
		// First definition: register one generic fallback handler
		// that reads the top of the definition stack.
		registerFn(name, Signature{
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
				if top.VType.Equal(TList) && !top.IsTypedList() && !top.IsTableType() {
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
		installFnDef(r, name, fnDef, isPrefixOnly)
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
// Each element of a triple may be abbreviated: a non-list value is treated
// as a single-element list (e.g., `string` is equivalent to `[string]`).
func parseFnDef(list []Value) (FnDefInfo, error) {
	var sigs []FnSig
	for i := 0; i < len(list); i += 3 {
		inputSig := list[i]
		// list[i+1] is the output signature — informational only
		body := list[i+2]

		// Abbreviation: non-list input sig is treated as [inputSig].
		if !inputSig.VType.Equal(TList) {
			inputSig = NewList([]Value{inputSig})
		}

		params, err := parseFnParams(inputSig)
		if err != nil {
			return FnDefInfo{}, err
		}

		// Abbreviation: non-list body is treated as [body].
		var bodyElems []Value
		if body.VType.Equal(TList) {
			bodyElems = body.AsList()
		} else {
			bodyElems = []Value{body}
		}

		sigs = append(sigs, FnSig{
			Params: params,
			Body:   bodyElems,
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

		case elem.VType.Matches(TInteger):
			// Integer literal as type constraint (e.g., 0 matches number/integer/0)
			params = append(params, FnParam{Type: elem.VType})

		case elem.VType.Matches(TBoolean):
			// Boolean literal as type constraint
			params = append(params, FnParam{Type: elem.VType})

		case elem.VType.Matches(TString):
			// String literal as type constraint
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
	// Literal values (integers, booleans) carry their literal type.
	if v.VType.Matches(TInteger) || v.VType.Matches(TBoolean) {
		return v.VType
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
func installFnDef(r *Registry, name string, fnDef FnDefInfo, prefixOnly ...bool) {
	isPrefixOnly := len(prefixOnly) > 0 && prefixOnly[0]
	registerFn := r.Register
	if isPrefixOnly {
		registerFn = r.RegisterPrefixOnly
	}
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
		registerFn(name, Signature{Args: argTypes, Handler: handler})
	}
}

// registerConvert registers the "convert" word for type conversions.
//
// convert takes a source value, a target type literal, and an optional
// third argument: either a string base shorthand (e.g. "hex") or a
// settings map. The string shorthand is a convenience for the "base"
// key in the settings map.
//
// Settings map keys:
//   - "base": base for number conversion ("hex", "HEX", "bin", "oct")
//   - "size": maximum length for string output (default 222)
//
//	convert 99 string                          => "99"
//	convert 10 string "hex"                    => "a"
//	convert 10 string {base:hex}               => "a"
//	convert "42" number                        => 42
//	convert true string                        => "true"
//	convert "hello" string {size:3}            => "hel"
//	convert 255 string {base:hex,size:1}       => "f"
func registerConvert(r *Registry) {
	const defaultSize = 222

	// truncate limits a string to maxLen characters.
	truncate := func(s string, maxLen int) string {
		if maxLen < 0 {
			maxLen = 0
		}
		if len(s) > maxLen {
			return s[:maxLen]
		}
		return s
	}

	// convertTo performs the actual conversion.
	convertTo := func(src Value, targetType Type, variant string, size int) (Value, error) {
		switch {
		case targetType.Matches(TString):
			// Convert to string.
			if variant == "" {
				return NewString(truncate(valToString(src), size)), nil
			}
			// Variant-based string conversion (only for numbers).
			if !src.VType.Matches(TNumber) {
				return Value{}, fmt.Errorf("convert: variant %q only supported for number to string", variant)
			}
			n := src.AsInteger()
			var s string
			switch variant {
			case "hex":
				s = strconv.FormatInt(n, 16)
			case "HEX":
				s = strings.ToUpper(strconv.FormatInt(n, 16))
			case "bin":
				s = strconv.FormatInt(n, 2)
			case "oct":
				s = strconv.FormatInt(n, 8)
			default:
				return Value{}, fmt.Errorf("convert: unknown string variant %q", variant)
			}
			return NewString(truncate(s, size)), nil

		case targetType.Matches(TNumber) || targetType.Matches(TInteger):
			// Convert to number.
			text := valToString(src)
			if variant == "" {
				n, err := strconv.ParseInt(text, 10, 64)
				if err != nil {
					return Value{}, fmt.Errorf("convert: cannot convert %q to number", text)
				}
				return NewInteger(n), nil
			}
			var base int
			switch variant {
			case "hex":
				base = 16
			case "bin":
				base = 2
			case "oct":
				base = 8
			default:
				return Value{}, fmt.Errorf("convert: unknown number variant %q", variant)
			}
			n, err := strconv.ParseInt(text, base, 64)
			if err != nil {
				return Value{}, fmt.Errorf("convert: cannot convert %q to number (base %d)", text, base)
			}
			return NewInteger(n), nil

		case targetType.Matches(TBoolean):
			// Convert to boolean.
			switch {
			case src.VType.Matches(TBoolean):
				return src, nil
			case src.VType.Matches(TNumber):
				return NewBoolean(src.AsInteger() != 0), nil
			default:
				text := valToString(src)
				switch text {
				case "true":
					return NewBoolean(true), nil
				case "false":
					return NewBoolean(false), nil
				default:
					return NewBoolean(text != ""), nil
				}
			}

		case targetType.Equal(TAtom):
			// Convert to atom.
			return NewAtom(valToString(src)), nil

		default:
			return Value{}, fmt.Errorf("convert: unsupported target type %s", targetType)
		}
	}

	// 2-arg: convert value type
	convert2Handler := func(args []Value) ([]Value, error) {
		src := args[0]
		if args[1].Data != nil {
			return nil, fmt.Errorf("convert: second argument must be a type literal")
		}
		result, err := convertTo(src, args[1].VType, "", defaultSize)
		if err != nil {
			return nil, err
		}
		return []Value{result}, nil
	}

	// 3-arg: convert value type <base-or-settings>
	// The third argument is either a string base shorthand (e.g. "hex")
	// or a settings map (e.g. {base:hex, size:3}).
	convert3Handler := func(args []Value) ([]Value, error) {
		src := args[0]
		if args[1].Data != nil {
			return nil, fmt.Errorf("convert: second argument must be a type literal")
		}
		base := ""
		size := defaultSize
		if args[2].VType.Equal(TMap) {
			m := args[2].AsMap()
			if v, ok := m.Get("base"); ok {
				base = valToString(v)
			}
			if v, ok := m.Get("size"); ok && v.VType.Matches(TInteger) {
				size = int(v.AsInteger())
			}
		} else {
			base = valToString(args[2])
		}
		result, err := convertTo(src, args[1].VType, base, size)
		if err != nil {
			return nil, err
		}
		return []Value{result}, nil
	}

	r.Register("convert",
		// 3-arg variant registered first (higher score from more args)
		Signature{
			Args:    []Type{TAny, TAny, TAny},
			Handler: convert3Handler,
		},
		Signature{
			Args:    []Type{TAny, TAny},
			Handler: convert2Handler,
		},
	)
}

// registerRecord registers the "record" word for creating record type values.
//
// record takes a list argument containing field pairs. Each element is a
// single-key map (from pair syntax) defining a field name and its type
// constraint.
//
//	record [x:number y:string]                   => record{x:number,y:string}
//	record [{x:{z:boolean}} "y":1]               => record{x:{z:boolean},y:1}
//	def Point record [x:number y:number]         => defines Point as a record type
//	{x:1, y:2} Point unify                       => {x:1,y:2} true
func registerRecord(r *Registry) {
	recordHandler := func(args []Value) ([]Value, error) {
		list := args[0]
		if !list.VType.Equal(TList) {
			return nil, fmt.Errorf("record: argument must be a list")
		}
		elems := list.AsList()
		if len(elems) == 0 {
			return nil, fmt.Errorf("record: list must have at least one field")
		}
		fields := NewOrderedMap()
		for _, elem := range elems {
			if !elem.VType.Equal(TMap) {
				return nil, fmt.Errorf("record: each element must be a pair (map), got %s", elem.String())
			}
			m, ok := elem.Data.(*OrderedMap)
			if !ok {
				return nil, fmt.Errorf("record: each element must be a concrete pair, got %s", elem.String())
			}
			for _, key := range m.Keys() {
				val, _ := m.Get(key)
				val = resolveFieldType(r, val)
				fields.Set(key, val)
			}
		}
		return []Value{NewRecordType(fields)}, nil
	}

	r.Register("record", Signature{
		Args:    []Type{TList},
		Handler: recordHandler,
	})
}

// registerTable registers the "table" word for creating table type values.
//
// table takes a single argument which must be a record type value. It produces
// a table type that represents a list of record instances conforming to that
// record schema.
//
//	type foo record [x:integer y:string]
//	type bar table foo                        => defines bar as table{x:number/integer,y:string}
//	table record [x:number]                   => table{x:number}
func registerTable(r *Registry) {
	tableHandler := func(args []Value) ([]Value, error) {
		target := args[0]
		if !target.IsRecordType() {
			return nil, fmt.Errorf("table: argument must be a record type, got %s", target.String())
		}
		return []Value{NewTableType(target.AsRecordType())}, nil
	}

	r.Register("table", Signature{
		Args:    []Type{TAny},
		Handler: tableHandler,
	})
}

// registerMake registers the "make" word for creating typed instances.
//
// Scalar forms:
//
//	make string 42       => '42'     (convert integer to string)
//	make number "99"     => 99       (convert string to number)
//	make boolean 1       => true     (convert number to boolean)
//
// Record form (first arg is a record type value, e.g. from a def):
//
//	def bar record [x:number y:boolean z:string]
//	make bar [1 false "s"]         => {x:1,y:false,z:'s'} (positional)
//	make bar [y:true x:2 z:'Z']   => {x:2,y:true,z:'Z'}  (named)
func registerMake(r *Registry) {
	// makeRecord creates a record instance from a source value and options.
	makeRecord := func(recType RecordTypeInfo, srcVal Value, useBase bool) ([]Value, error) {
		fieldKeys := recType.Fields.Keys()
		result := NewOrderedMap()

		// Helper: fill result from a provided map, defaulting
		// missing fields based on useBase flag.
		fillFromMap := func(provided *OrderedMap) error {
			for _, key := range provided.Keys() {
				if _, ok := recType.Fields.Get(key); !ok {
					return fmt.Errorf("make: unknown field %q", key)
				}
			}
			for _, key := range fieldKeys {
				constraint, _ := recType.Fields.Get(key)
				val, ok := provided.Get(key)
				if !ok {
					if useBase {
						// base:true — fill with base value for the field type.
						bv, err := baseValueForConstraint(constraint)
						if err != nil {
							return fmt.Errorf("make: field %q: %w", key, err)
						}
						result.Set(key, bv)
						continue
					}
					// Default: allow if none unifies with constraint.
					noneVal := NewTypeLiteral(TNone)
					if _, unifOK := Unify(constraint, noneVal); unifOK {
						result.Set(key, noneVal)
						continue
					}
					return fmt.Errorf("make: missing field %q", key)
				}
				converted, err := makeFieldValue(val, constraint)
				if err != nil {
					return fmt.Errorf("make: field %q: %w", key, err)
				}
				result.Set(key, converted)
			}
			return nil
		}

		// Map form: make RecType {x:1 y:"hello"}
		if srcVal.VType.Equal(TMap) {
			provided, ok := srcVal.Data.(*OrderedMap)
			if !ok {
				return nil, fmt.Errorf("make: expected concrete map, got %s", srcVal.String())
			}
			if err := fillFromMap(provided); err != nil {
				return nil, err
			}
			return []Value{NewMap(result)}, nil
		}

		if !srcVal.VType.Equal(TList) {
			return nil, fmt.Errorf("make: record values must be a list or map, got %s", srcVal.String())
		}
		elems := srcVal.AsList()

		// Check if named or positional.
		isNamed := len(elems) > 0 && elems[0].VType.Equal(TMap)
		if isNamed {
			if _, ok := elems[0].Data.(*OrderedMap); !ok {
				isNamed = false
			}
		}

		if isNamed {
			provided := NewOrderedMap()
			for _, elem := range elems {
				if !elem.VType.Equal(TMap) {
					return nil, fmt.Errorf("make: mixed named and positional fields")
				}
				m, ok := elem.Data.(*OrderedMap)
				if !ok {
					return nil, fmt.Errorf("make: expected concrete map pair, got %s", elem.String())
				}
				for _, key := range m.Keys() {
					val, _ := m.Get(key)
					provided.Set(key, val)
				}
			}
			if err := fillFromMap(provided); err != nil {
				return nil, err
			}
		} else {
			if len(elems) != len(fieldKeys) {
				return nil, fmt.Errorf("make: expected %d values, got %d",
					len(fieldKeys), len(elems))
			}
			for i, key := range fieldKeys {
				constraint, _ := recType.Fields.Get(key)
				converted, err := makeFieldValue(elems[i], constraint)
				if err != nil {
					return nil, fmt.Errorf("make: field %q: %w", key, err)
				}
				result.Set(key, converted)
			}
		}

		return []Value{NewMap(result)}, nil
	}

	// parseOptions extracts make options from an options map.
	parseOptions := func(opts Value) (useBase bool, err error) {
		if !opts.VType.Equal(TMap) {
			return false, fmt.Errorf("make: options must be a map, got %s", opts.String())
		}
		m, ok := opts.Data.(*OrderedMap)
		if !ok {
			return false, fmt.Errorf("make: expected concrete options map")
		}
		if v, ok := m.Get("base"); ok {
			v = resolveWordValue(v)
			if v.VType.Matches(TBoolean) && v.Data.(bool) {
				useBase = true
			}
		}
		return useBase, nil
	}

	makeHandler := func(args []Value) ([]Value, error) {
		targetVal := args[0]
		srcVal := args[1]

		// Record type instance creation.
		if targetVal.IsRecordType() {
			recType := targetVal.AsRecordType()
			return makeRecord(recType, srcVal, false)
		}

		// Table type instance creation.
		if targetVal.IsTableType() {
			tableType := targetVal.AsTableType()
			recType := tableType.Record

			if !srcVal.VType.Equal(TList) {
				return nil, fmt.Errorf("make: table values must be a list of row lists, got %s", srcVal.String())
			}
			rows := srcVal.AsList()
			fieldKeys := recType.Fields.Keys()
			resultRows := make([]Value, 0, len(rows))

			for rowIdx, rowVal := range rows {
				if !rowVal.VType.Equal(TList) {
					return nil, fmt.Errorf("make: table row %d must be a list, got %s", rowIdx, rowVal.String())
				}
				rowElems := rowVal.AsList()

				// Check if named or positional.
				isNamed := len(rowElems) > 0 && rowElems[0].VType.Equal(TMap)
				if isNamed {
					if _, ok := rowElems[0].Data.(*OrderedMap); !ok {
						isNamed = false
					}
				}

				result := NewOrderedMap()
				if isNamed {
					provided := NewOrderedMap()
					for _, elem := range rowElems {
						if !elem.VType.Equal(TMap) {
							return nil, fmt.Errorf("make: table row %d: mixed named and positional fields", rowIdx)
						}
						m, ok := elem.Data.(*OrderedMap)
						if !ok {
							return nil, fmt.Errorf("make: table row %d: expected concrete map pair, got %s", rowIdx, elem.String())
						}
						for _, key := range m.Keys() {
							val, _ := m.Get(key)
							provided.Set(key, val)
						}
					}
					for _, key := range fieldKeys {
						val, ok := provided.Get(key)
						if !ok {
							return nil, fmt.Errorf("make: table row %d: missing field %q", rowIdx, key)
						}
						constraint, _ := recType.Fields.Get(key)
						converted, err := makeFieldValue(val, constraint)
						if err != nil {
							return nil, fmt.Errorf("make: table row %d: field %q: %w", rowIdx, key, err)
						}
						result.Set(key, converted)
					}
					for _, key := range provided.Keys() {
						if _, ok := recType.Fields.Get(key); !ok {
							return nil, fmt.Errorf("make: table row %d: unknown field %q", rowIdx, key)
						}
					}
				} else {
					if len(rowElems) != len(fieldKeys) {
						return nil, fmt.Errorf("make: table row %d: expected %d values, got %d",
							rowIdx, len(fieldKeys), len(rowElems))
					}
					for i, key := range fieldKeys {
						constraint, _ := recType.Fields.Get(key)
						converted, err := makeFieldValue(rowElems[i], constraint)
						if err != nil {
							return nil, fmt.Errorf("make: table row %d: field %q: %w", rowIdx, key, err)
						}
						result.Set(key, converted)
					}
				}

				resultRows = append(resultRows, NewMap(result))
			}

			return []Value{NewList(resultRows)}, nil
		}

		// Scalar type conversion.
		if targetVal.Data != nil {
			return nil, fmt.Errorf("make: first argument must be a type literal or record type, got %s", targetVal.String())
		}

		targetType := targetVal.VType
		if srcVal.VType.Matches(targetType) {
			return []Value{srcVal}, nil
		}

		result, err := makeConvert(srcVal, targetType)
		if err != nil {
			return nil, err
		}
		return []Value{result}, nil
	}

	// 3-arg handler: make RecType source {options}
	makeWithOpts := func(args []Value) ([]Value, error) {
		targetVal := args[0]
		srcVal := args[1]
		optsVal := args[2]

		useBase, err := parseOptions(optsVal)
		if err != nil {
			return nil, err
		}

		if targetVal.IsRecordType() {
			recType := targetVal.AsRecordType()
			return makeRecord(recType, srcVal, useBase)
		}

		// For non-record types, options are ignored — delegate to 2-arg.
		return makeHandler(args[:2])
	}

	r.Register("make",
		Signature{
			Args:    []Type{TAny, TAny, TMap},
			Handler: makeWithOpts,
		},
		Signature{
			Args:    []Type{TAny, TAny},
			Handler: makeHandler,
		},
	)
}

// makeConvert converts a source value to a target scalar type for the make word.
func makeConvert(src Value, targetType Type) (Value, error) {
	switch {
	case targetType.Matches(TString):
		return NewString(valToString(src)), nil

	case targetType.Matches(TNumber) || targetType.Matches(TInteger):
		text := valToString(src)
		n, err := strconv.ParseInt(text, 10, 64)
		if err != nil {
			return Value{}, fmt.Errorf("make: cannot convert %q to number", text)
		}
		return NewInteger(n), nil

	case targetType.Matches(TBoolean):
		switch {
		case src.VType.Matches(TBoolean):
			return src, nil
		case src.VType.Matches(TNumber):
			return NewBoolean(src.AsInteger() != 0), nil
		default:
			text := valToString(src)
			switch text {
			case "true":
				return NewBoolean(true), nil
			case "false":
				return NewBoolean(false), nil
			default:
				return NewBoolean(text != ""), nil
			}
		}

	case targetType.Equal(TAtom):
		return NewAtom(valToString(src)), nil

	default:
		return Value{}, fmt.Errorf("make: unsupported target type %s", targetType)
	}
}

// makeFieldValue converts a value to match a record field's type constraint.
// If the constraint is a type literal, the value is converted to that type.
// If the value already matches, it is returned as-is.
func makeFieldValue(val Value, constraint Value) (Value, error) {
	// Resolve words to their semantic value first (e.g. word(false) → boolean false).
	val = resolveWordValue(val)

	// If the constraint is a type literal (Data==nil), convert the value.
	if constraint.Data == nil {
		constraintType := constraint.VType
		if val.VType.Matches(constraintType) {
			return val, nil
		}
		return makeConvert(val, constraintType)
	}

	// If the constraint has a concrete value, just check via unification.
	unified, ok := Unify(constraint, val)
	if !ok {
		return Value{}, fmt.Errorf("value %s does not match constraint %s", val.String(), constraint.String())
	}
	return unified, nil
}

// resolveWordValue converts a word value to its semantic value.
// Words named "true"/"false" become booleans, "none" becomes a type literal,
// and other words become atoms (bare strings).
func resolveWordValue(v Value) Value {
	if !v.IsWord() {
		return v
	}
	name := v.AsWord().Name
	switch name {
	case "true":
		return NewBoolean(true)
	case "false":
		return NewBoolean(false)
	case "none":
		return NewTypeLiteral(TNone)
	default:
		return NewAtom(name)
	}
}

// resolveFieldType resolves a record field's type constraint value.
//
// Three resolution strategies:
//  1. String matching a user-defined type name in DefStacks → replaced
//     with the defined type value (e.g., disjunctions by name).
//  2. Concrete list → evaluated as code in a sub-engine so that
//     expressions like [string or none] produce a disjunction.
//  3. Everything else passes through unchanged.
//
// Examples:
//
//	type OptStr (string or none)
//	record [x:number y:OptStr]              => record{x:number,y:string|none}
//	record [x:number y:[string or none]]    => record{x:number,y:string|none}
func resolveFieldType(r *Registry, v Value) Value {
	// Strategy 1: string matching a defined type name.
	if v.Data != nil && v.VType.Matches(TString) {
		name := v.AsString()
		stack := r.DefStacks[name]
		if len(stack) > 0 {
			top := stack[len(stack)-1]
			if isTypeValue(top) {
				return top
			}
		}
		return v
	}

	// Strategy 2: evaluate concrete list as code.
	if v.VType.Equal(TList) && !v.IsTypedList() && !v.IsTableType() {
		elems := v.AsList()
		// Promote strings that name registered functions to words,
		// since list elements inside pairs are parsed in data context.
		input := make([]Value, len(elems))
		for i, e := range elems {
			if (e.VType.Matches(TString) || e.VType.Matches(TAtom)) && e.Data != nil {
				name := e.AsString()
				if r.Lookup(name) != nil {
					input[i] = NewWord(name)
					continue
				}
			}
			input[i] = e
		}
		sub := New(r)
		results, err := sub.Run(input)
		if err == nil && len(results) == 1 {
			return results[0]
		}
		// If evaluation fails or produces multiple values, keep original.
		return v
	}

	return v
}

// registerTypeDef registers the "type" word for defining custom types.
//
// type defines a named type. The body must be a type-like value:
// a record type, disjunct, type literal, typed list, or typed map.
//
//	type Point record [x:number y:number]
//	type OptNum (number or none)
//	type NumList [:number]
//	type Num number
func registerTypeDef(r *Registry) {
	typeHandler := func(args []Value) ([]Value, error) {
		name := defName(args[0])
		body := args[1]

		// Validate that the body is a type-like value.
		if !isTypeValue(body) {
			return nil, fmt.Errorf("type: body must be a type value (record, disjunct, type literal, typed list, or typed map), got %s", body.String())
		}

		installDef(r, name, body)
		return nil, nil
	}

	r.Register("type",
		Signature{
			Args:    []Type{TWord, TAny},
			Handler: typeHandler,
		},
		Signature{
			Args:    []Type{TString, TAny},
			Handler: typeHandler,
		},
	)
}

// registerDo registers the "do" word for evaluating list and map contents.
//
// For lists, do evaluates the elements as a token stream in a sub-engine:
//
//	do [1 add 2]  →  3
//
// For maps, do evaluates any list values (depth-first), leaving non-list
// values unchanged:
//
//	do {x: [3 add 4], y: [upper a]}  →  {x:7, y:"A"}
func registerDo(r *Registry) {
	// promoteToWord converts a string value to a word if the string
	// names a registered function. This is needed because list elements
	// inside maps are parsed in data context (bare text → string),
	// but do needs to evaluate them as code (bare text → word).
	promoteToWord := func(v Value) Value {
		if v.VType.Matches(TString) || v.VType.Matches(TAtom) {
			name := v.AsString()
			if r.Lookup(name) != nil {
				return NewWord(name)
			}
		}
		return v
	}

	evalList := func(elems []Value) ([]Value, error) {
		sub := New(r)
		input := make([]Value, len(elems))
		copy(input, elems)
		return sub.Run(input)
	}

	// evalDataList evaluates a list from data context (inside a map).
	// Strings that name registered functions are promoted to words.
	evalDataList := func(elems []Value) ([]Value, error) {
		sub := New(r)
		input := make([]Value, len(elems))
		for i, e := range elems {
			input[i] = promoteToWord(e)
		}
		return sub.Run(input)
	}

	var evalMapValue func(v Value) (Value, error)
	evalMapValue = func(v Value) (Value, error) {
		if v.VType.Equal(TList) && !v.IsTypedList() && !v.IsTableType() {
			results, err := evalDataList(v.AsList())
			if err != nil {
				return Value{}, err
			}
			if len(results) == 1 {
				return results[0], nil
			}
			return NewList(results), nil
		}
		if v.VType.Equal(TMap) && !v.IsTypedMap() && !v.IsRecordType() {
			m := v.AsMap()
			out := NewOrderedMap()
			for _, key := range m.Keys() {
				val, _ := m.Get(key)
				evaluated, err := evalMapValue(val)
				if err != nil {
					return Value{}, err
				}
				out.Set(key, evaluated)
			}
			return NewMap(out), nil
		}
		return v, nil
	}

	r.Register("do",
		Signature{
			Args: []Type{TList},
			Handler: func(args []Value) ([]Value, error) {
				return evalList(args[0].AsList())
			},
		},
		Signature{
			Args: []Type{TMap},
			Handler: func(args []Value) ([]Value, error) {
				result, err := evalMapValue(args[0])
				if err != nil {
					return nil, err
				}
				return []Value{result}, nil
			},
		},
	)
}

// isTypeValue reports whether a value is a valid type definition body.
func isTypeValue(v Value) bool {
	// Type literal (Data==nil): number, string, boolean, any, etc.
	if v.Data == nil {
		return true
	}
	// Record type
	if v.IsRecordType() {
		return true
	}
	// Table type
	if v.IsTableType() {
		return true
	}
	// Disjunct
	if v.IsDisjunct() {
		return true
	}
	// Typed list [:type]
	if v.IsTypedList() {
		return true
	}
	// Typed map {:type}
	if v.IsTypedMap() {
		return true
	}
	return false
}

// registerTypeof registers the "typeof" word that returns the base type
// of its argument as an atom.
//
//	typeof 42          => integer
//	typeof "hello"     => string
//	typeof true        => boolean
//	typeof [1 2 3]     => list
//	typeof {x:1}       => map
//	typeof none        => none
func registerTypeof(r *Registry) {
	typeofHandler := func(args []Value) ([]Value, error) {
		v := args[0]
		name := v.VType.Parts[0]
		return []Value{NewAtom(name)}, nil
	}

	r.Register("typeof",
		Signature{Args: []Type{TAny}, Handler: typeofHandler},
	)
}

// registerBase registers the "base" word that returns the zero/default value
// for a given type, similar to Go's zero values.
//
//	base integer    => 0
//	base string     => ''
//	base boolean    => false
//	base list       => []
//	base map        => {}
//	base none       => none
// baseValue returns the zero/default value for a given type, similar to Go's
// zero values. Used by both the "base" word and "make" with base:true option.
func baseValue(t Type) (Value, error) {
	switch {
	case t.Matches(TInteger):
		return NewInteger(0), nil
	case t.Matches(TNumber):
		return NewInteger(0), nil
	case t.Matches(TString):
		return NewString(""), nil
	case t.Matches(TBoolean):
		return NewBoolean(false), nil
	case t.Matches(TList):
		return NewList([]Value{}), nil
	case t.Matches(TMap):
		return NewMap(NewOrderedMap()), nil
	case t.Matches(TNone):
		return NewTypeLiteral(TNone), nil
	case t.Matches(TAtom):
		return NewAtom(""), nil
	default:
		return Value{}, fmt.Errorf("base: unsupported type %s", t.String())
	}
}

// baseValueForConstraint returns the base value for a field constraint.
// For type literals, returns the zero value directly.
// For disjunctions (e.g. string|none), returns the base of the first
// non-none alternative.
func baseValueForConstraint(constraint Value) (Value, error) {
	if constraint.IsDisjunct() {
		di := constraint.AsDisjunct()
		for _, alt := range di.Alternatives {
			if alt.Data == nil && !alt.VType.Equal(TNone) {
				return baseValue(alt.VType)
			}
		}
		// All alternatives are none.
		return NewTypeLiteral(TNone), nil
	}
	if constraint.Data == nil {
		return baseValue(constraint.VType)
	}
	return Value{}, fmt.Errorf("base: cannot determine base value for %s", constraint.String())
}

func registerBase(r *Registry) {
	baseHandler := func(args []Value) ([]Value, error) {
		v := args[0]
		t := v.VType
		result, err := baseValue(t)
		if err != nil {
			return nil, err
		}
		return []Value{result}, nil
	}

	r.Register("base",
		Signature{Args: []Type{TAny}, Handler: baseHandler},
	)
}

// registerDot registers the "dot" word and its "." alias for extracting
// key values from maps and lists. Supports null-safe access: if the parent
// is none, the result is none (like optional chaining in JavaScript).
// An integer argument indexes into a list, or is converted to a string
// for map key lookup.
//
// Usage (prefix):
//
//	{a:1} a dot       => 1
//	[10,20,30] 1 dot  => 20
//	{0:"z"} 0 dot     => z
//
// Usage (suffix):
//
//	{a:1} dot a       => 1
//
// Usage (dot notation, handled by parser):
//
//	set foo a:b:1 foo.a.b  => 1
func registerDot(r *Registry) {
	dotMapAtomHandler := func(args []Value) ([]Value, error) {
		key := args[0].AsAtom()
		m := args[1].AsMap()
		val, ok := m.Get(key)
		if !ok {
			return []Value{NewTypeLiteral(TNone)}, nil
		}
		return []Value{val}, nil
	}

	dotMapStringHandler := func(args []Value) ([]Value, error) {
		key := args[0].AsString()
		m := args[1].AsMap()
		val, ok := m.Get(key)
		if !ok {
			return []Value{NewTypeLiteral(TNone)}, nil
		}
		return []Value{val}, nil
	}

	dotListHandler := func(args []Value) ([]Value, error) {
		idx := int(args[0].AsInteger())
		list := args[1].AsList()
		if idx < 0 || idx >= len(list) {
			return []Value{NewTypeLiteral(TNone)}, nil
		}
		return []Value{list[idx]}, nil
	}

	dotMapIntegerHandler := func(args []Value) ([]Value, error) {
		key := strconv.FormatInt(args[0].AsInteger(), 10)
		m := args[1].AsMap()
		val, ok := m.Get(key)
		if !ok {
			return []Value{NewTypeLiteral(TNone)}, nil
		}
		return []Value{val}, nil
	}

	dotNoneHandler := func(args []Value) ([]Value, error) {
		return []Value{NewTypeLiteral(TNone)}, nil
	}

	sigs := []Signature{
		{Args: []Type{TAtom, TMap}, Handler: dotMapAtomHandler},
		{Args: []Type{TString, TMap}, Handler: dotMapStringHandler},
		{Args: []Type{TInteger, TList}, Handler: dotListHandler},
		{Args: []Type{TInteger, TMap}, Handler: dotMapIntegerHandler},
		{Args: []Type{TAny, TNone}, Handler: dotNoneHandler},
	}

	r.Register("dot", sigs...)
	r.Register(".", sigs...)
}

// registerDotr registers "dotr" and its "!." alias — a strict variant of
// dot that returns an error when the parent is none or the key/index is
// missing, instead of silently returning none.
//
// Usage:
//
//	{a:1} a dotr      => 1
//	{a:1} b dotr      => ERROR (key not found)
//	none a dotr       => ERROR (parent is none)
//	[10,20] 5 dotr    => ERROR (index out of bounds)
func registerDotr(r *Registry) {
	dotrMapAtomHandler := func(args []Value) ([]Value, error) {
		key := args[0].AsAtom()
		m := args[1].AsMap()
		val, ok := m.Get(key)
		if !ok {
			return nil, fmt.Errorf("dotr: key %q not found in map", key)
		}
		return []Value{val}, nil
	}

	dotrMapStringHandler := func(args []Value) ([]Value, error) {
		key := args[0].AsString()
		m := args[1].AsMap()
		val, ok := m.Get(key)
		if !ok {
			return nil, fmt.Errorf("dotr: key %q not found in map", key)
		}
		return []Value{val}, nil
	}

	dotrListHandler := func(args []Value) ([]Value, error) {
		idx := int(args[0].AsInteger())
		list := args[1].AsList()
		if idx < 0 || idx >= len(list) {
			return nil, fmt.Errorf("dotr: index %d out of bounds (length %d)", idx, len(list))
		}
		return []Value{list[idx]}, nil
	}

	dotrMapIntegerHandler := func(args []Value) ([]Value, error) {
		key := strconv.FormatInt(args[0].AsInteger(), 10)
		m := args[1].AsMap()
		val, ok := m.Get(key)
		if !ok {
			return nil, fmt.Errorf("dotr: key %q not found in map", key)
		}
		return []Value{val}, nil
	}

	dotrNoneHandler := func(args []Value) ([]Value, error) {
		return nil, fmt.Errorf("dotr: parent is none")
	}

	sigs := []Signature{
		{Args: []Type{TAtom, TMap}, Handler: dotrMapAtomHandler},
		{Args: []Type{TString, TMap}, Handler: dotrMapStringHandler},
		{Args: []Type{TInteger, TList}, Handler: dotrListHandler},
		{Args: []Type{TInteger, TMap}, Handler: dotrMapIntegerHandler},
		{Args: []Type{TAny, TNone}, Handler: dotrNoneHandler},
	}

	r.Register("dotr", sigs...)
	r.Register("!.", sigs...)
}
