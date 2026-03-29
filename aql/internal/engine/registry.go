package engine

import (
	"fmt"
	"io"
	"os"
	"strconv"

	"github.com/metsitaba/voxgig-exp/aql/internal/fileops"
)

// Function groups all signatures for a named function.
type Function struct {
	Name             string
	Signatures       []Signature
	ForwardPrecedence bool // true = engine tries forward-first; false = stack-only
}

// TypeDef describes a named complex type in the type registry.
// The Type field holds the full type path (e.g. Node/Map/Resource/Table).
// The Constraint holds the type's structure — a record type, disjunct, etc.
type TypeDef struct {
	Type       Type  // full type path
	Constraint Value // structural constraint (RecordTypeInfo, ChildTypeInfo, etc.)
}

// Registry maps function names to their definitions.
type Registry struct {
	funcs     map[string]*Function
	Store     map[string]Value   // key-value store for set/get
	DefStacks map[string][]Value // stacked bodies for def-defined words
	Types     map[string]TypeDef // complex type registry keyed by full type path
	FileOps   fileops.FileOps    // file operations for read/write words
	Formats   map[string]Format  // format registry for read/write (keyed by name)
	Output    io.Writer          // output writer for print/printstr and stdout
	ErrOutput io.Writer          // error output writer for stderr
	Input     io.Reader          // input reader for stdin
	SQLite    *SQLiteStore       // in-memory SQLite store for table data
	Modules        map[string]ModuleDesc    // child modules keyed by generated ID
	moduleSeq      int                      // counter for generating module IDs
	ParseFunc      func(string) ([]Value, error) // parser callback (set externally to avoid circular import)
	ctxStack       []map[string]Value // scoped context stack; top = current engine's context
	argsStack      []Value            // stack of args lists for nested fn calls
	KnownTypeParts map[string]bool    // set of all type path parts (for uniqueness enforcement)
	Manager        any                // external manager (e.g. UniversalManager) for SDK operations
	SDKCache       map[string]any     // cached SDK instances keyed by spec name
	BaseDir        string             // base directory for resolving relative file paths (set by loadFileModule)
}

// NewRegistry creates an empty registry.
func NewRegistry() (*Registry, error) {
	sqlStore, err := NewSQLiteStore()
	if err != nil {
		return nil, fmt.Errorf("failed to initialize SQLite store: %w", err)
	}
	ops := fileops.NewDefault()
	formats := DefaultFormats()

	// Wire the multisource resolver into the jsonic format so that
	// @"path" references in .jsonic files are resolved via FileOps.
	if jf, ok := formats["jsonic"].(*JsonicFormat); ok {
		jf.Resolver = MakeFileOpsResolver(ops)
	}

	r := &Registry{
		funcs:          make(map[string]*Function),
		Store:          make(map[string]Value),
		DefStacks:      make(map[string][]Value),
		Types:          make(map[string]TypeDef),
		FileOps:        ops,
		Formats:        formats,
		Output:         os.Stdout,
		ErrOutput:      os.Stderr,
		Input:          os.Stdin,
		SQLite:         sqlStore,
		Modules:        make(map[string]ModuleDesc),
		KnownTypeParts: builtinTypeParts(),
		SDKCache:       make(map[string]any),
	}
	return r, nil
}

// NextModuleID generates a unique module identifier.
func (r *Registry) NextModuleID() string {
	r.moduleSeq++
	return fmt.Sprintf("mod_%d", r.moduleSeq)
}

// SetFileOps replaces the file operations implementation and updates the
// jsonic format's multisource resolver to use the new ops.
func (r *Registry) SetFileOps(ops fileops.FileOps) {
	r.FileOps = ops
	if jf, ok := r.Formats["jsonic"].(*JsonicFormat); ok {
		jf.Resolver = MakeFileOpsResolver(ops)
	}
}

// SetParseFunc sets the parser callback used by file-based import.
func (r *Registry) SetParseFunc(fn func(string) ([]Value, error)) {
	r.ParseFunc = fn
}

// PushContext pushes a new context layer that is a shallow copy of parent.
// Values are copied by reference (like Go's context.WithValue pattern).
func (r *Registry) PushContext(parent map[string]Value) {
	child := make(map[string]Value, len(parent))
	for k, v := range parent {
		child[k] = v
	}
	r.ctxStack = append(r.ctxStack, child)
}

// PopContext removes the top context layer, restoring the parent.
func (r *Registry) PopContext() {
	if len(r.ctxStack) > 0 {
		r.ctxStack = r.ctxStack[:len(r.ctxStack)-1]
	}
}

// Context returns the current (top) context map, or nil if no context is active.
func (r *Registry) Context() map[string]Value {
	if len(r.ctxStack) == 0 {
		return nil
	}
	return r.ctxStack[len(r.ctxStack)-1]
}

// Register adds one or more signatures to a named function with forward precedence.
func (r *Registry) Register(name string, sigs ...Signature) {
	fn, ok := r.funcs[name]
	if !ok {
		fn = &Function{Name: name, ForwardPrecedence: true}
		r.funcs[name] = fn
	}
	fn.Signatures = append(fn.Signatures, sigs...)
}

// RegisterStackOnly adds signatures to a named function without forward precedence.
func (r *Registry) RegisterStackOnly(name string, sigs ...Signature) {
	fn, ok := r.funcs[name]
	if !ok {
		fn = &Function{Name: name, ForwardPrecedence: false}
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
func DefaultRegistry() (*Registry, error) {
	r, err := NewRegistry()
	if err != nil {
		return nil, err
	}
	registerBuiltins(r)
	return r, nil
}

func registerBuiltins(r *Registry) {
	// String
	registerUpper(r)
	registerLower(r)
	registerConcat(r)
	registerSplit(r)
	registerTrim(r)
	registerContains(r)
	registerIndexOf(r)
	registerReplace(r)
	registerSlice(r)
	registerChangeCase(r)
	registerNormalize(r)
	registerRepeat(r)
	registerPad(r)
	registerMatch(r)
	registerEscape(r)

	// Stack
	registerDup(r)
	registerSwap(r)
	registerDrop(r)
	registerOver(r)
	registerRot(r)
	registerNip(r)
	registerTuck(r)
	register2dup(r)
	register2swap(r)
	register2drop(r)
	register2over(r)
	registerDepth(r)
	registerPick(r)
	registerRoll(r)
	registerStackCollect(r)

	// Math: arithmetic
	registerAdd(r)
	registerSub(r)
	registerMul(r)
	registerDiv(r)
	registerMod(r)
	registerAbs(r)
	registerNegate(r)
	registerMin(r)
	registerMax(r)
	registerPow(r)
	registerSign(r)

	// Math: rounding
	registerCeil(r)
	registerFloor(r)
	registerRound(r)
	registerTrunc(r)

	// Math: roots, exp/log
	registerSqrt(r)
	registerCbrt(r)
	registerExp(r)
	registerLog(r)
	registerLog2(r)
	registerLog10(r)

	// Math: trigonometry
	registerSin(r)
	registerCos(r)
	registerTan(r)
	registerAsin(r)
	registerAcos(r)
	registerAtan(r)
	registerAtan2(r)
	registerHypot(r)

	// Math: constants
	registerMathConstants(r)

	// Boolean
	registerOr(r)
	registerAnd(r)
	registerXor(r)
	registerNand(r)
	registerImplies(r)
	registerNot(r)

	// Comparison
	registerComparison(r)

	// Storage
	registerSet(r)
	registerGet(r)
	registerContext(r)

	// Definition
	registerDef(r)
	registerUndef(r)
	registerVar(r)
	registerFn(r)
	registerCall(r)
	registerDblcall(r)
	registerArgs(r)
	registerPopArgs(r)

	// Type
	registerConvert(r)
	registerRecord(r)
	registerTable(r)
	registerObject(r)
	registerResource(r)
	registerMake(r)
	registerTypeDef(r)
	registerTypeof(r)
	registerFullTypeof(r)
	registerIs(r)
	registerInspect(r)
	registerBase(r)

	// Control flow
	registerDo(r)
	registerIf(r)
	registerFor(r)
	registerError(r)
	registerQuote(r)

	// Accessors
	registerDot(r)
	registerDotr(r)

	// I/O
	registerFileIO(r)
	registerPrint(r)
	registerTrace(r)

	// Query
	registerQuery(r)

	// Unify
	registerUnify(r)

	// Module
	registerModule(r)

	// Help
	registerHelp(r)
}

// --- Shared helpers used by multiple builtin files ---

// registerBinaryIntOp registers a binary integer operation with a single
// signature Args:[int, int] and forward precedence.
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

// registerBinaryNumOp registers a binary numeric operation with three
// overloads: [decimal, decimal], [number, decimal], and [decimal, number].
func registerBinaryNumOp(r *Registry, name string, prec int, op func(a, b float64) (float64, error)) {
	handler := func(args []Value) ([]Value, error) {
		result, err := op(args[0].AsNumber(), args[1].AsNumber())
		if err != nil {
			return nil, err
		}
		return []Value{NewDecimal(result)}, nil
	}
	r.Register(name, Signature{
		Args:       []Type{TDecimal, TDecimal},
		Precedence: prec,
		Handler:    handler,
	})
	r.Register(name, Signature{
		Args:       []Type{TNumber, TDecimal},
		Precedence: prec,
		Handler:    handler,
	})
	r.Register(name, Signature{
		Args:       []Type{TDecimal, TNumber},
		Precedence: prec,
		Handler:    handler,
	})
}

// registerUnaryNumOp registers a unary numeric operation with two overloads:
// [integer] -> [decimal] and [decimal] -> [decimal].
func registerUnaryNumOp(r *Registry, name string, op func(float64) float64) {
	handler := func(args []Value) ([]Value, error) {
		return []Value{NewDecimal(op(args[0].AsNumber()))}, nil
	}
	r.Register(name, Signature{
		Args:    []Type{TInteger},
		Handler: handler,
	})
	r.Register(name, Signature{
		Args:    []Type{TDecimal},
		Handler: handler,
	})
}

// registerBinaryBoolOp registers a binary boolean operation with a single
// signature Args:[boolean, boolean] and forward precedence.
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

// valToString converts any scalar Value to its string representation.
func valToString(v Value) string {
	if v.Data == nil && !v.VType.Equal(TNone) {
		return v.VType.String()
	}
	switch {
	case v.VType.Matches(TString):
		return v.AsString()
	case v.IsAtom():
		return v.AsAtom()
	case v.VType.Matches(TDecimal):
		return strconv.FormatFloat(v.AsDecimal(), 'f', -1, 64)
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
	if v.Data == nil {
		return v.VType.String()
	}
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

