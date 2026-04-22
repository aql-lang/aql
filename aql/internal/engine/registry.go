package engine

import (
	"fmt"
	"io"
	"os"
	"strconv"

	"github.com/metsitaba/voxgig-exp/aql/internal/fileops"
)

// Registry maps function names to their definitions.
type Registry struct {
	DefStacks         map[string][]Value                                 // stacked bodies for def-defined words
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
	ArgsStack         []Value                                            // stack of args lists for nested fn calls
	KnownTypeParts    map[string]bool                                    // set of all type path parts (for uniqueness enforcement)
	Manager           any                                                // external manager (e.g. UniversalManager) for SDK operations
	SDKCache          map[string]any                                     // cached SDK instances keyed by spec name
	BaseDir           string                                             // base directory for resolving relative file paths (set by loadFileModule)
	Source            string                                             // most recent source text for error reporting
	errs              []error                                            // registration errors accumulated during setup
	ready             bool                                               // true after initial setup; triggers dynamic help generation
	OnRegisterHook    func(name string)                                  // called when a function is registered after startup
	NativeModResolver func(name string, r *Registry) (ModuleDesc, error) // resolves "aql:<name>" native module imports
	ModuleInitFunc    func(*Registry)                                    // called when creating module sub-registries to register extension words
	loadedNativeMods  map[string]bool                                    // tracks which native modules have been loaded

	// CheckMode toggles static type-checking execution. When true, the
	// engine runs the same dispatch/matching machinery but carries
	// type-only Carrier values instead of concrete payloads, and
	// replaces signature handlers with carrier-typed return propagation
	// (see Signature.Returns). Diagnostics are accumulated into
	// CheckDiagnostics rather than returned as hard errors.
	CheckMode        bool
	CheckDiagnostics []CheckDiagnostic

	// CheckFnSummaries caches carrier return-stacks for user-defined
	// fn bodies keyed by (name + "#" + argTypesJoined). Populated by
	// analyseFnBody; re-entrant calls (recursion) consult this cache
	// to break cycles and converge on a fixed point.
	CheckFnSummaries map[string][]Value

	// CheckFnInflight tracks which (name, arg-types) analyses are
	// currently running so that recursive calls can bail out with a
	// placeholder instead of looping.
	CheckFnInflight map[string]bool

	// CheckStepCount is the running total of engine steps consumed
	// by the current check run, summed across every sub-engine.
	// Used with CheckStepBudget to cap total analysis effort.
	CheckStepCount int

	// CheckStepBudget is the maximum total steps the check run may
	// consume. Zero means "use DefaultCheckStepBudget". Once
	// exceeded, the engine emits a step_budget_exceeded diagnostic
	// and returns the current residual stack immediately.
	CheckStepBudget int

	// CheckBudgetTripped is set to true after the first budget
	// overshoot so we emit at most one diagnostic per check run.
	CheckBudgetTripped bool

	// CheckDefsInstalled records the names (and source positions)
	// that the user's program defined during a check run via the
	// def word. Populated by recordCheckDef; consulted at end of
	// run to emit unused_def warnings.
	CheckDefsInstalled map[string]SrcPos

	// CheckDefsUsed records names looked up via Registry.Lookup or
	// simple-value substitution in check mode. Used to filter out
	// defs that were referenced at least once.
	CheckDefsUsed map[string]bool

	// CheckContextTypes is a best-effort record of keys that user
	// code wrote to a Store during a check run. The value is the
	// last-seen carrier type for that key, joined via
	// JoinCarriers on repeated writes. Used by get's ReturnsFn so
	// subsequent reads can produce a typed carrier rather than
	// falling back to Any. Shared across the entire check run —
	// not keyed by store identity — to keep the model simple for
	// the common "one context store" usage pattern.
	CheckContextTypes map[string]Value
}

// DefaultCheckStepBudget caps total check-mode steps across all
// sub-engines. Chosen to comfortably fit typical programs
// (thousands of words) while preventing pathological runaways.
const DefaultCheckStepBudget = 500_000

// CheckSeverity classifies a diagnostic as an error, warning, or info.
// Errors indicate a real type/signature violation that prevents
// successful execution. Warnings flag suspicious patterns that are
// still type-correct. Info is everything else (missing annotation,
// budget overshoot, etc.).
type CheckSeverity string

const (
	SeverityError   CheckSeverity = "error"
	SeverityWarning CheckSeverity = "warning"
	SeverityInfo    CheckSeverity = "info"
)

// checkCodeSeverity maps a diagnostic code to its default severity.
// Unknown codes default to SeverityInfo so new codes don't
// accidentally trip CI gates until they're classified.
var checkCodeSeverity = map[string]CheckSeverity{
	"no_signature":         SeverityError,
	"undefined_word":       SeverityError,
	"fn_body_error":        SeverityError,
	"branch_error":         SeverityError,
	"missing_returns":      SeverityWarning,
	"step_budget_exceeded": SeverityWarning,
	"body_error":           SeverityWarning,
}

// SeverityFor returns the default severity classification for a
// diagnostic code. Exported so consumers can tag custom codes.
func SeverityFor(code string) CheckSeverity {
	if s, ok := checkCodeSeverity[code]; ok {
		return s
	}
	return SeverityInfo
}

// CheckDiagnostic is a single static type-check finding.
type CheckDiagnostic struct {
	Code     string        `json:"code"`               // short stable code, e.g. "missing_returns", "no_signature"
	Detail   string        `json:"detail"`             // human-readable description
	Word     string        `json:"word,omitempty"`     // word name relevant to the diagnostic, if any
	Row      int           `json:"row,omitempty"`      // 1-based line number, 0 if unknown
	Col      int           `json:"col,omitempty"`      // 1-based column number, 0 if unknown
	Severity CheckSeverity `json:"severity,omitempty"` // default severity from checkCodeSeverity; empty = info
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
	_as0, _ := memVal.AsBoolean()
	if memVal.VType.Matches(TBoolean) && _as0 {
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

// UpdateCtxStoreChain updates ctxStack entries affected by a COW operation.
// origRoot is the original Store that was COW'd (the prototype of the new
// root). newRoot is the COW'd replacement. Scans from the top of the stack
// (most likely match) and uses direct pointer comparison as a fast path
// before walking prototype chains.
func (r *Registry) UpdateCtxStoreChain(origRoot, newRoot *StoreInstanceInfo) {
	for i := len(r.ctxStack) - 1; i >= 0; i-- {
		entry := r.ctxStack[i]
		// Fast path: direct match (the common case for top-of-stack COW).
		if entry == origRoot {
			r.ctxStack[i] = newRoot
			continue
		}
		// Walk the prototype chain only if needed. Limit walk depth to
		// short-circuit if the chain is long and doesn't contain origRoot.
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
			fnDef.MaxForwardArgs = calcMaxForwardArgs(fnDef.Signatures)
			stack[len(stack)-1].Data = fnDef
			return
		}
	}
	// No existing FnDefInfo on top — push a new one.
	fnDef := FnDefInfo{
		Name:              name,
		Signatures:        append([]Signature(nil), sigs...),
		ForwardPrecedence: forwardPrec,
	}
	SortSignatures(fnDef.Signatures)
	fnDef.MaxForwardArgs = calcMaxForwardArgs(fnDef.Signatures)
	r.DefStacks[name] = append(r.DefStacks[name], NewFnDef(fnDef))
}

// calcMaxForwardArgs returns the maximum number of forward args needed
// across all signatures. For sigs with a barrier, only positions before
// the barrier count. This tells the engine how far ahead to scan and
// pre-evaluate paren expressions before signature matching.
func calcMaxForwardArgs(sigs []Signature) int {
	max := 0
	for i := range sigs {
		n := len(sigs[i].Args)
		if sigs[i].BarrierPos > 0 && sigs[i].BarrierPos < n {
			n = sigs[i].BarrierPos
		}
		if n > max {
			max = n
		}
	}
	return max
}

// Lookup returns the top FnDefInfo for a name from DefStacks, or nil.
//
// Lookup deliberately does NOT record a check-mode "use" of the name
// because it is called from internal machinery (installDef, undef,
// match dispatch) that would inflate use counts. User-code usage is
// recorded by the engine.stepWord paths (simple-value substitution
// and the post-Lookup dispatch path).
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

// DefaultRegistry returns a registry populated with built-in primitives
// plus any additional provider functions passed in. Each provider is a
// function that registers words (e.g. engine.Register, native.Register).
// Called with no providers, it registers only engine's built-in core words.
func DefaultRegistry(providers ...func(*Registry)) (*Registry, error) {
	r, err := NewRegistry()
	if err != nil {
		return nil, err
	}
	Register(r)
	for _, p := range providers {
		p(r)
	}
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

func Register(r *Registry) {
	// String
	RegisterUpper(r)
	RegisterLower(r)
	RegisterConcat(r)
	RegisterSplit(r)
	RegisterTrim(r)
	RegisterContains(r)
	RegisterIndexOf(r)
	RegisterReplace(r)
	RegisterChangeCase(r)
	RegisterNormalize(r)
	RegisterRepeat(r)
	RegisterPad(r)
	// slice moved to native.
	RegisterMatch(r)
	RegisterEscape(r)

	// Stack ops (StackCollect moved to native; rest stay because engine
	// internal tests depend on them).
	RegisterDup(r)
	RegisterSwap(r)
	RegisterDrop(r)
	RegisterOver(r)
	RegisterRot(r)
	RegisterNip(r)
	RegisterTuck(r)
	Register2dup(r)
	Register2swap(r)
	Register2drop(r)
	Register2over(r)
	RegisterDepth(r)
	RegisterPick(r)
	RegisterRoll(r)

	// String slice moved to native.

	// Math: basic arithmetic (always available)
	RegisterAdd(r)
	RegisterSub(r)
	RegisterMul(r)
	RegisterDiv(r)
	RegisterMod(r)
	RegisterPow(r)

	// Math: extended operations are in the "aql:math" native module.
	// Use: "aql:math" import

	// Boolean: implies moved to native. or, not, and, xor, nand stay because
	// engine tests depend on them (or uses disjunct specially).
	RegisterOr(r)
	RegisterAnd(r)
	RegisterXor(r)
	RegisterNand(r)
	RegisterNot(r)

	// Comparison
	RegisterComparison(r)

	// Storage
	RegisterSet(r)
	RegisterGet(r)
	RegisterContext(r)

	// Definition — popargs moved to native. var, call, dblcall, args stay
	// because engine tests depend on them.
	RegisterDef(r)
	RegisterUndef(r)
	RegisterVar(r)
	RegisterFn(r)
	RegisterCall(r)
	RegisterDblcall(r)
	RegisterArgs(r)
	RegisterPopArgs(r)

	// Type — record, table, typeof, fulltypeof stay because engine tests
	// depend on them.
	RegisterConvert(r)
	RegisterRecord(r)
	RegisterTable(r)
	RegisterObject(r)
	RegisterResource(r)
	RegisterMake(r)
	RegisterTypeDef(r)
	RegisterTypeof(r)
	RegisterFullTypeof(r)
	RegisterIs(r)
	RegisterInspect(r)
	RegisterBase(r)

	// Control flow — quote moved to native.
	RegisterDo(r)
	RegisterIf(r)
	RegisterFor(r)
	RegisterError(r)

	// Accessors — getr stays because engine tests depend on dotr/getr.
	RegisterGetr(r)

	// I/O — folder moved to native.
	RegisterFileIO(r)
	RegisterPrint(r)
	RegisterTrace(r)

	// Query (temporarily disabled — precedence removal)
	// RegisterQuery(r)

	// Unify
	RegisterUnify(r)

	// Module
	RegisterModule(r)

	// Array
	RegisterIota(r)
	RegisterShape(r)
	RegisterRank(r)
	RegisterLength(r)
	RegisterReshape(r)
	RegisterArrFlatten(r)
	RegisterArrTranspose(r)
	RegisterReverse(r)
	RegisterTake(r)
	RegisterShed(r)
	RegisterWhere(r)
	RegisterUnique(r)
	RegisterGrade(r)
	RegisterAt(r)
	RegisterSortby(r)
	RegisterMember(r)
	RegisterArrIndexof(r)
	RegisterGroup(r)
	RegisterReplicate(r)
	RegisterExpand(r)
	RegisterWindow(r)
	RegisterPairs(r)

	// Array higher-order
	RegisterEach(r)
	RegisterFold(r)
	RegisterScan(r)
	RegisterOuter(r)
	RegisterInner(r)

	// Temporal — now, sleep, interval, cancel moved to native.
	RegisterTimeout(r)
	RegisterAwait(r)

	// Help
	RegisterHelp(r)
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
		_as2, _ := args[0].AsInteger()
		_as1, _ := args[1].AsInteger()
		result, err := op(_as2, _as1)
		if err != nil {
			return nil, err
		}
		return []Value{NewInteger(result)}, nil
	}
	r.Register(name, Signature{
		Args:    []Type{TInteger, TInteger},
		Handler: handler,
		Returns: []Type{TInteger},
	})
}

// registerBinaryNumOp registers a binary numeric operation with three
// overloads: [decimal, decimal], [number, decimal], and [decimal, number].
func registerBinaryNumOp(r *Registry, name string, op func(a, b float64) (float64, error)) {
	handler := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		_as4, _ := args[0].AsNumber()
		_as3, _ := args[1].AsNumber()
		result, err := op(_as4, _as3)
		if err != nil {
			return nil, err
		}
		return []Value{NewDecimal(result)}, nil
	}
	r.Register(name, Signature{
		Args:    []Type{TDecimal, TDecimal},
		Handler: handler,
		Returns: []Type{TDecimal},
	})
	r.Register(name, Signature{
		Args:    []Type{TNumber, TDecimal},
		Handler: handler,
		Returns: []Type{TDecimal},
	})
	r.Register(name, Signature{
		Args:    []Type{TDecimal, TNumber},
		Handler: handler,
		Returns: []Type{TDecimal},
	})
}

// registerUnaryNumOp registers a unary numeric operation with two overloads:
// [integer] -> [decimal] and [decimal] -> [decimal].
func registerUnaryNumOp(r *Registry, name string, op func(float64) float64) {
	handler := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		_as5, _ := args[0].AsNumber()
		return []Value{NewDecimal(op(_as5))}, nil
	}
	r.Register(name, Signature{
		Args:    []Type{TInteger},
		Handler: handler,
		Returns: []Type{TDecimal},
	})
	r.Register(name, Signature{
		Args:    []Type{TDecimal},
		Handler: handler,
		Returns: []Type{TDecimal},
	})
}

// RegisterBinaryBoolOp registers a binary boolean operation with a single
// signature Args:[boolean, boolean] and forward precedence.
func RegisterBinaryBoolOp(r *Registry, name string, op func(a, b bool) bool) {
	handler := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		_as7, _ := args[0].AsBoolean()
		_as6, _ := args[1].AsBoolean()
		return []Value{NewBoolean(op(_as7, _as6))}, nil
	}
	r.Register(name, Signature{
		Args:    []Type{TBoolean, TBoolean},
		Handler: handler,
		Returns: []Type{TBoolean},
	})
}

// valToString converts any scalar Value to its string representation.
func valToString(v Value) string {
	if v.Data == nil && !v.VType.Equal(TNone) {
		return v.VType.String()
	}
	switch {
	case v.VType.Matches(TString):
		_as8, _ := v.AsString()
		return _as8
	case v.IsAtom():
		_as9, _ := v.AsAtom()
		return _as9
	case v.VType.Matches(TDecimal):
		_as10, _ := v.AsDecimal()
		return strconv.FormatFloat(_as10, 'f', -1, 64)
	case v.VType.Matches(TInteger):
		_as11, _ := v.AsInteger()
		return strconv.FormatInt(_as11, 10)
	case v.VType.Matches(TBoolean):
		_as12, _ := v.AsBoolean()
		if _as12 {
			return "true"
		}
		return "false"
	case v.IsPath():
		_as13, _ := v.AsPath()
		return _as13.String()
	case v.IsWord():
		_as14, _ := v.AsWord()
		return _as14.Name
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
		_as15, _ := v.AsWord()
		return _as15.Name
	}
	if v.VType.Matches(TString) {
		_as16, _ := v.AsString()
		return _as16
	}
	if v.IsAtom() {
		_as17, _ := v.AsAtom()
		return _as17
	}
	return fmt.Sprintf("%v", v.Data)
}
