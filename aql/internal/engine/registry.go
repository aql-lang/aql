package engine

import (
	"fmt"
	"io"
	"os"
	"strconv"

	"github.com/metsitaba/voxgig-exp/aql/internal/fileops"
)

// TypeDef describes a named complex type in the type registry.
// The Type field holds the full type path (e.g. Node/Map/Resource/Table).
// The Constraint holds the type's structure — a record type, disjunct, etc.
type TypeDef struct {
	Type       Type  // full type path
	Constraint Value // structural constraint (RecordTypeInfo, ChildTypeInfo, etc.)
}

// Registry maps function names to their definitions.
type Registry struct {
	DefStacks         map[string][]Value                                 // stacked bodies for def-defined words
	Types             map[string]TypeDef                                 // complex type registry keyed by full type path
	FileOps           fileops.FileOps                                    // file operations for read/write words (OS-backed default)
	MemOps            *fileops.MemFileOps                                // in-memory file ops (used when __sys.fs.mem = true)
	Formats           map[string]Format                                  // format registry for read/write (keyed by name)
	Output            io.Writer                                          // output writer for print/printstr and stdout
	ErrOutput         io.Writer                                          // error output writer for stderr
	Input             io.Reader                                          // input reader for stdin
	SQLite            *SQLiteStore                                       // in-memory SQLite store for table data
	moduleSeq         int                                                // counter for generating module IDs
	ParseFunc         func(string) ([]Value, error)                      // parser callback (set externally to avoid circular import)
	ctxStack          []*StoreInstanceInfo                               // scoped context stack; top = current engine's context Store
	argsStack         []Value                                            // stack of args lists for nested fn calls
	KnownTypeParts    map[string]bool                                    // set of all type path parts (for uniqueness enforcement)
	Manager           any                                                // external manager (e.g. UniversalManager) for SDK operations
	SDKCache          map[string]any                                     // cached SDK instances keyed by spec name
	BaseDir           string                                             // base directory for resolving relative file paths (set by loadFileModule)
	errs              []error                                            // registration errors accumulated during setup
	ready             bool                                               // true after initial setup; triggers dynamic help generation
	OnRegisterHook    func(name string)                                  // called when a function is registered after startup
	NativeModResolver func(name string, r *Registry) (ModuleDesc, error) // resolves "aql:<name>" native module imports
	loadedNativeMods  map[string]bool                                    // tracks which native modules have been loaded
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
		DefStacks:      make(map[string][]Value),
		Types:          make(map[string]TypeDef),
		FileOps:        ops,
		Formats:        formats,
		Output:         os.Stdout,
		ErrOutput:      os.Stderr,
		Input:          os.Stdin,
		SQLite:         sqlStore,
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

// EffectiveFileOps returns the file operations to use based on __sys.fs.mem.
// If mem is true, returns the in-memory file ops; otherwise the OS-backed default.
func (r *Registry) EffectiveFileOps() fileops.FileOps {
	store := r.ContextStore()
	if store == nil {
		return r.FileOps
	}
	sysVal, ok := store.Get("__sys")
	if !ok {
		return r.FileOps
	}
	sysStore, ok := sysVal.Data.(*StoreInstanceInfo)
	if !ok {
		return r.FileOps
	}
	fsVal, ok := sysStore.Get("fs")
	if !ok {
		return r.FileOps
	}
	fsStore, ok := fsVal.Data.(*StoreInstanceInfo)
	if !ok {
		return r.FileOps
	}
	memVal, ok := fsStore.Get("mem")
	if !ok {
		return r.FileOps
	}
	if memVal.VType.Matches(TBoolean) && memVal.AsBoolean() {
		if r.MemOps == nil {
			r.MemOps = fileops.NewMem()
		}
		return r.MemOps
	}
	return r.FileOps
}

// SetParseFunc sets the parser callback used by file-based import.
func (r *Registry) SetParseFunc(fn func(string) ([]Value, error)) {
	r.ParseFunc = fn
}

// MarkReady signals that initial setup is complete. Subsequent Register
// calls will trigger dynamic help example generation via OnRegisterHook.
func (r *Registry) MarkReady() {
	r.ready = true
}

// PushContext pushes a new context Store whose prototype is the parent.
// Key resolution walks the prototype chain, enabling scope-like lookup.
func (r *Registry) PushContext(parent *StoreInstanceInfo) {
	child := &StoreInstanceInfo{
		TypeName:  "Object/Store",
		Data:      make(map[string]Value),
		Prototype: parent,
	}
	r.ctxStack = append(r.ctxStack, child)
}

// PopContext removes the top context layer, restoring the parent.
func (r *Registry) PopContext() {
	if len(r.ctxStack) > 0 {
		r.ctxStack = r.ctxStack[:len(r.ctxStack)-1]
	}
}

// Context returns the current (top) context as a map for handler compatibility.
// Returns nil if no context is active.
func (r *Registry) Context() map[string]Value {
	si := r.ContextStore()
	if si == nil {
		return nil
	}
	return si.Data
}

// ContextStore returns the current (top) context Store, or nil if no context is active.
func (r *Registry) ContextStore() *StoreInstanceInfo {
	if len(r.ctxStack) == 0 {
		return nil
	}
	return r.ctxStack[len(r.ctxStack)-1]
}

// UpdateCtxStoreChain updates ALL ctxStack entries affected by a COW operation.
// origRoot is the original Store that was COW'd (the prototype of the new
// root). newRoot is the COW'd replacement. For each ctxStack entry whose
// prototype chain passes through origRoot, replace origRoot with newRoot
// in that chain.
func (r *Registry) UpdateCtxStoreChain(origRoot, newRoot *StoreInstanceInfo) {
	for i := 0; i < len(r.ctxStack); i++ {
		entry := r.ctxStack[i]
		// Direct match: this ctxStack entry IS the original root.
		if entry == origRoot {
			r.ctxStack[i] = newRoot
			continue
		}
		// Check if the entry's prototype chain passes through origRoot.
		// If so, rebuild the chain with newRoot substituted.
		for p := entry; p != nil; p = p.Prototype {
			if p.Prototype == origRoot {
				p.Prototype = newRoot
				break
			}
		}
	}
}

// Register adds one or more signatures to a named function with forward precedence.
// Signatures are stored in a FnDefInfo entry in DefStacks.
func (r *Registry) Register(name string, sigs ...Signature) {
	for _, sig := range sigs {
		if len(sig.Args) > MaxArgs {
			r.errs = append(r.errs, fmt.Errorf("signature for %q has %d args, max is %d", name, len(sig.Args), MaxArgs))
			return
		}
	}
	r.upsertFnDef(name, true, sigs...)
	if r.ready && r.OnRegisterHook != nil {
		r.OnRegisterHook(name)
	}
}

// RegisterStackOnly adds signatures to a named function without forward precedence.
// Signatures are stored in a FnDefInfo entry in DefStacks.
func (r *Registry) RegisterStackOnly(name string, sigs ...Signature) {
	for _, sig := range sigs {
		if len(sig.Args) > MaxArgs {
			r.errs = append(r.errs, fmt.Errorf("signature for %q has %d args, max is %d", name, len(sig.Args), MaxArgs))
			return
		}
	}
	r.upsertFnDef(name, false, sigs...)
	if r.ready && r.OnRegisterHook != nil {
		r.OnRegisterHook(name)
	}
}

// upsertFnDef finds or creates a FnDefInfo at the top of DefStacks[name]
// and appends the given compiled signatures. If the top entry is already a
// FnDefInfo, its Signatures are updated in place. Otherwise a new FnDefInfo
// is pushed.
func (r *Registry) upsertFnDef(name string, forwardPrec bool, sigs ...Signature) {
	stack := r.DefStacks[name]
	// If the top of the stack is already a FnDefInfo, update it in place.
	if len(stack) > 0 {
		if fnDef, ok := stack[len(stack)-1].Data.(FnDefInfo); ok {
			fnDef.Signatures = append(fnDef.Signatures, sigs...)
			SortSignatures(fnDef.Signatures)
			fnDef.ForwardPrecedence = forwardPrec
			stack[len(stack)-1].Data = fnDef
			return
		}
	}
	// No existing FnDefInfo on top — push a new one.
	fnDef := FnDefInfo{
		Name:              name,
		Signatures:        append([]Signature(nil), sigs...),
		ForwardPrecedence: forwardPrec,
		Builtin:           !r.ready,
	}
	SortSignatures(fnDef.Signatures)
	r.DefStacks[name] = append(r.DefStacks[name], NewFnDef(fnDef))
}

// Lookup returns the top FnDefInfo for a name from DefStacks, or nil.
func (r *Registry) Lookup(name string) *FnDefInfo {
	stack := r.DefStacks[name]
	for i := len(stack) - 1; i >= 0; i-- {
		if fnDef, ok := stack[i].Data.(FnDefInfo); ok {
			return &fnDef
		}
	}
	return nil
}

// Match finds the best matching signature for a function name given the
// resolved stack state and word modifiers.
func (r *Registry) Match(name string, resolved []Value, modifiers WordInfo) *MatchResult {
	fnDef := r.Lookup(name)
	if fnDef == nil {
		return nil
	}
	return MatchSignature(fnDef.Signatures, resolved, modifiers)
}

// clearSigsKeepFallback resets the Signatures on the top FnDefInfo in
// DefStacks[name] to only the Fallback entries (if any). Used during
// rebuild after overlap filtering or undef.
func (r *Registry) clearSigsKeepFallback(name string) {
	stack := r.DefStacks[name]
	if len(stack) == 0 {
		return
	}
	if fnDef, ok := stack[len(stack)-1].Data.(FnDefInfo); ok {
		fnDef.Signatures = KeepFallback(fnDef.Signatures)
		stack[len(stack)-1].Data = fnDef
	}
}

// InitRootContext initializes the root context Store with the __sys key.
// The __sys value is a Store/System instance containing system configuration.
// All containers at every depth are Stores.
func (r *Registry) InitRootContext() {
	root := &StoreInstanceInfo{
		TypeName: "Object/Store",
		Data:     make(map[string]Value),
	}

	// Create the System store.
	sysStore := &StoreInstanceInfo{
		TypeName: "Object/Store/System",
		Data:     make(map[string]Value),
	}

	// fs: a Store with {mem: false, impl: None}
	fsStore := &StoreInstanceInfo{
		TypeName: "Object/Store",
		Data:     make(map[string]Value),
	}
	fsStore.Set("mem", NewBoolean(false))
	fsStore.Set("impl", NewTypeLiteral(TNone))
	sysStore.Set("fs", NewStoreValue(fsStore))

	// __val: a Store for user-defined values
	valStore := &StoreInstanceInfo{
		TypeName: "Object/Store",
		Data:     make(map[string]Value),
	}
	sysStore.Set("__val", NewStoreValue(valStore))

	root.Set("__sys", NewStoreValue(sysStore))
	r.ctxStack = append(r.ctxStack, root)
}

// DefaultRegistry returns a registry populated with built-in primitives.
func DefaultRegistry() (*Registry, error) {
	r, err := NewRegistry()
	if err != nil {
		return nil, err
	}
	registerBuiltins(r)
	if err := r.Err(); err != nil {
		return nil, err
	}
	r.InitRootContext()
	return r, nil
}

// Err returns the first registration error, or nil if none occurred.
func (r *Registry) Err() error {
	if len(r.errs) == 0 {
		return nil
	}
	return r.errs[0]
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
	registerChangeCase(r)
	registerNormalize(r)
	registerRepeat(r)
	registerPad(r)
	registerSlice(r)
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

	// Math: basic arithmetic (always available)
	registerAdd(r)
	registerSub(r)
	registerMul(r)
	registerDiv(r)
	registerMod(r)
	registerPow(r)

	// Math: extended operations are in the "aql:math" native module.
	// Use: "aql:math" import

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
	registerGetr(r)

	// I/O
	registerFileIO(r)
	registerFolder(r)
	registerPrint(r)
	registerTrace(r)

	// Query (temporarily disabled — precedence removal)
	// registerQuery(r)

	// Unify
	registerUnify(r)

	// Module
	registerModule(r)

	// Array
	registerIota(r)
	registerShape(r)
	registerRank(r)
	registerLength(r)
	registerReshape(r)
	registerArrFlatten(r)
	registerArrTranspose(r)
	registerReverse(r)
	registerTake(r)
	registerShed(r)
	registerWhere(r)
	registerUnique(r)
	registerGrade(r)
	registerAt(r)
	registerSortby(r)
	registerMember(r)
	registerArrIndexof(r)
	registerGroup(r)
	registerReplicate(r)
	registerExpand(r)
	registerWindow(r)
	registerPairs(r)

	// Array higher-order
	registerEach(r)
	registerFold(r)
	registerScan(r)
	registerOuter(r)
	registerInner(r)

	// Help
	registerHelp(r)
}

// IsNativeModLoaded returns true if the named native module has already been loaded.
func (r *Registry) IsNativeModLoaded(name string) bool {
	if r.loadedNativeMods == nil {
		return false
	}
	return r.loadedNativeMods[name]
}

// MarkNativeModLoaded records that the named native module has been loaded.
func (r *Registry) MarkNativeModLoaded(name string) {
	if r.loadedNativeMods == nil {
		r.loadedNativeMods = make(map[string]bool)
	}
	r.loadedNativeMods[name] = true
}

// --- Shared helpers used by multiple builtin files ---

// RegisterBinaryIntOp registers a binary integer operation with a single
// signature Args:[int, int] and forward precedence.
func RegisterBinaryIntOp(r *Registry, name string, op func(a, b int64) (int64, error)) {
	registerBinaryIntOp(r, name, op)
}

// RegisterBinaryNumOp registers a binary numeric operation with three
// overloads: [decimal, decimal], [number, decimal], and [decimal, number].
func RegisterBinaryNumOp(r *Registry, name string, op func(a, b float64) (float64, error)) {
	registerBinaryNumOp(r, name, op)
}

// RegisterUnaryNumOp registers a unary numeric operation with two overloads:
// [integer] -> [decimal] and [decimal] -> [decimal].
func RegisterUnaryNumOp(r *Registry, name string, op func(float64) float64) {
	registerUnaryNumOp(r, name, op)
}

// registerBinaryIntOp registers a binary integer operation with a single
// signature Args:[int, int] and forward precedence.
func registerBinaryIntOp(r *Registry, name string, op func(a, b int64) (int64, error)) {
	handler := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		result, err := op(args[0].AsInteger(), args[1].AsInteger())
		if err != nil {
			return nil, err
		}
		return []Value{NewInteger(result)}, nil
	}
	r.Register(name, Signature{
		Args:    []Type{TInteger, TInteger},
		Handler: handler,
	})
}

// registerBinaryNumOp registers a binary numeric operation with three
// overloads: [decimal, decimal], [number, decimal], and [decimal, number].
func registerBinaryNumOp(r *Registry, name string, op func(a, b float64) (float64, error)) {
	handler := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		result, err := op(args[0].AsNumber(), args[1].AsNumber())
		if err != nil {
			return nil, err
		}
		return []Value{NewDecimal(result)}, nil
	}
	r.Register(name, Signature{
		Args:    []Type{TDecimal, TDecimal},
		Handler: handler,
	})
	r.Register(name, Signature{
		Args:    []Type{TNumber, TDecimal},
		Handler: handler,
	})
	r.Register(name, Signature{
		Args:    []Type{TDecimal, TNumber},
		Handler: handler,
	})
}

// registerUnaryNumOp registers a unary numeric operation with two overloads:
// [integer] -> [decimal] and [decimal] -> [decimal].
func registerUnaryNumOp(r *Registry, name string, op func(float64) float64) {
	handler := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
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
func registerBinaryBoolOp(r *Registry, name string, op func(a, b bool) bool) {
	handler := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		return []Value{NewBoolean(op(args[0].AsBoolean(), args[1].AsBoolean()))}, nil
	}
	r.Register(name, Signature{
		Args:    []Type{TBoolean, TBoolean},
		Handler: handler,
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
	case v.IsPath():
		return v.AsPath().String()
	case v.IsWord():
		return v.AsWord().Name
	default:
		return v.String()
	}
}

// contextStoreLookup looks up a key in the registry's context store,
// walking the prototype chain. Returns the value and true if found.
func contextStoreLookup(r *Registry, key string) (Value, bool) {
	store := r.ContextStore()
	if store == nil {
		return Value{}, false
	}
	return store.Get(key)
}

// ContextSet stores a key-value pair in the root context store.
// Convenience method for programmatic setup (e.g. tests, query setup).
func (r *Registry) ContextSet(key string, val Value) {
	store := r.ContextStore()
	if store == nil {
		r.InitRootContext()
		store = r.ContextStore()
	}
	store.Set(key, val)
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
