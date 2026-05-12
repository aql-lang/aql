package eng

import (
	"fmt"
	"io"
	"os"
	"strconv"
)

// Registry maps function names to their definitions.
//
// The def-stack and type-stack registries are stored in unexported
// fields. ALL access — read, write, snapshot, restore — goes through
// the helper API in util.go (PushDef/PopDef/TopOfDefStack/HasDef/
// DefStackDepth/SetDefStack/DefStack/DefNames/SnapshotDefDepths/
// RestoreToDefDepths/TruncateDefStack/ReplaceDefTop/DeleteDef and the
// matching *Type-side helpers PushType/PopType/HasType/TopOfTypeStack/
// TypeStackDepth/TypeNames/SnapshotTypeStacks/RestoreTypeStacks). New
// callers must use those methods; direct field access is deliberately
// disabled by the lowercase identifiers.
type Registry struct {
	defStacks map[string][]Value // stacked bodies for def-defined words
	// types holds named type definitions installed by the `type` word —
	// type literals, records, disjuncts, typed lists/maps, options,
	// records, object types, dependent scalars (DepInteger, DepString,
	// …), function-shape types (FnUndef), and predicate types
	// (FnDef/Function used as type-defining functions). *Type values
	// live here, not in defStacks, because they are NOT independently
	// callable — a predicate type Bbd is only ever consulted via type
	// operations (`def n:Bbd v`, `v is Bbd`, `inspect Bbd`), never
	// invoked as a free-standing fn.
	//
	// Stacked: each name maps to a stack of definitions. `type Foo X`
	// pushes; `untype Foo` pops. The top is the active type. Once a
	// stack empties the entry is removed from the map. This mirrors
	// `def`'s shadowing semantics so users can introduce a temporary
	// alias inside a sub-program and revert it without registry
	// surgery.
	Types             *TypeTable                                         // dynamic types installed by the `type` word; each push mints a fresh Type
	capabilities      map[string]any                                     // host-installed plugin slots (see capability.go)
	Output            io.Writer                                          // output writer for print/printstr and stdout
	ErrOutput         io.Writer                                          // error output writer for stderr
	Input             io.Reader                                          // input reader for stdin
	moduleSeq         int                                                // counter for generating module IDs
	ParseFunc         func(string) ([]Value, error)                      // parser callback (set externally to avoid circular import)
	ctxStack          []*StoreInstanceInfo                               // scoped context stack; top = current engine's context Store
	argsStack         []Value                                            // stack of args lists for nested fn calls; access via PushArgs/PopArgs/TopArgs
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

	// Check holds all static type-checking state, bundled together
	// so the future predicate-sandbox work (TYPE-SYSTEM-REVIEW.md
	// §3.3) can snapshot/restore one field instead of ten.
	Check CheckState
}

// CheckState aggregates the static type-checking state that used to
// live as ten loose fields on Registry. Bundling them serves two
// purposes:
//
//   - **Sandboxing.** A predicate body that runs under unify checks
//     should not mutate enclosing analysis state. With a single
//     struct, snapshot/restore is `saved := r.Check; defer func()
//     { r.Check = saved }()` rather than ten parallel assignments.
//   - **Discoverability.** Anyone reading `Registry` can see the
//     check-mode footprint at a glance instead of scanning ten
//     adjacent declarations.
type CheckState struct {
	// Mode toggles static type-checking execution. When true, the
	// engine runs the same dispatch/matching machinery but carries
	// type-only Carrier values instead of concrete payloads, and
	// replaces signature handlers with carrier-typed return
	// propagation (see Signature.Returns). Diagnostics are
	// accumulated into Diagnostics rather than returned as hard
	// errors.
	Mode        bool
	Diagnostics []CheckDiagnostic

	// FnSummaries caches carrier return-stacks for user-defined fn
	// bodies keyed by (name + "#" + argTypesJoined). Populated by
	// analyseFnBody; re-entrant calls (recursion) consult this
	// cache to break cycles and converge on a fixed point.
	FnSummaries map[string][]Value

	// FnInflight tracks which (name, arg-types) analyses are
	// currently running so that recursive calls can bail out with
	// a placeholder instead of looping.
	FnInflight map[string]bool

	// StepCount is the running total of engine steps consumed by
	// the current check run, summed across every sub-engine. Used
	// with StepBudget to cap total analysis effort.
	StepCount int

	// StepBudget is the maximum total steps the check run may
	// consume. Zero means "use DefaultCheckStepBudget". Once
	// exceeded, the engine emits a step_budget_exceeded diagnostic
	// and returns the current residual stack immediately.
	StepBudget int

	// BudgetTripped is set to true after the first budget overshoot
	// so we emit at most one diagnostic per check run.
	BudgetTripped bool

	// DefsInstalled records the names (and source positions) that
	// the user's program defined during a check run via the def
	// word. Populated by RecordCheckDef; consulted at end of run
	// to emit unused_def warnings.
	DefsInstalled map[string]SrcPos

	// DefsUsed records names looked up via Registry.Lookup or
	// simple-value substitution in check mode. Used to filter out
	// defs that were referenced at least once.
	DefsUsed map[string]bool

	// ContextTypes is a best-effort record of keys that user code
	// wrote to a Store during a check run. The value is the
	// last-seen carrier type for that key, joined via JoinCarriers
	// on repeated writes. Used by get's ReturnsFn so subsequent
	// reads can produce a typed carrier rather than falling back to
	// Any. Shared across the entire check run — not keyed by store
	// identity — to keep the model simple for the common
	// "one context store" usage pattern.
	ContextTypes map[string]Value
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
	"type_error":           SeverityError,
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
//
// The returned Registry has no built-in capabilities — no file
// operations, no format registry, no SQL store. The host package
// installs those via Registry.SetCapability before running user code.
// See capability.go for the plugin contract.
func NewRegistry() (*Registry, error) {
	r := &Registry{
		defStacks:    make(map[string][]Value),
		Types:        NewDynamicTypeTable(),
		capabilities: make(map[string]any),
		Output:       os.Stdout,
		ErrOutput:    os.Stderr,
		Input:        os.Stdin,
		SDKCache:     make(map[string]any),
	}
	return r, nil
}

// NextModuleID generates a unique module identifier.
func (r *Registry) NextModuleID() string {
	r.moduleSeq++
	return fmt.Sprintf("mod_%d", r.moduleSeq)
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

// Register adds one or more signatures to a named function. Sigs are
// treated as forward-arg defaults: any sig with BarrierPos still 0 has
// it lifted to len(Args), so all positions are forward-eligible.
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

// RegisterStackOnly adds signatures to a named function. Sigs are
// taken as-is: BarrierPos stays at 0 (no forward-arg defaulting), so
// every position is matched from the stack unless the sig itself
// raises BarrierPos. Signatures are stored in a FnDefInfo entry in
// DefStacks.
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
//
// Normalisation: every appended sig has its BarrierPos set on the way in.
// BarrierPos is the position of the boundary marker (`|`) in the sig:
// args before it can be forward-collected, args from it onward must come
// from the stack (top-down). When forwardPrec is true and BarrierPos is
// still zero, we set it to len(Args) — i.e. the boundary is at the end,
// every arg is forward-eligible. When forwardPrec is false (old
// stack-only registration), BarrierPos stays at zero — boundary at the
// start, every arg from the stack.
func (r *Registry) upsertFnDef(name string, forwardArgs bool, sigs ...Signature) {
	// If the caller registered with forward-arg defaults, fill in
	// BarrierPos for any sig that didn't set it explicitly. Sigs with
	// BarrierPos already non-zero, or sigs registered via the
	// stack-only path, are left alone.
	for i := range sigs {
		if sigs[i].BarrierPos == 0 && forwardArgs && len(sigs[i].Args) > 0 {
			sigs[i].BarrierPos = len(sigs[i].Args)
		}
	}
	// If the top of the stack is already a FnDefInfo, update it in place.
	if top, ok := r.TopOfDefStack(name); ok {
		if fnDef, ok := top.Data.(FnDefInfo); ok {
			fnDef.Signatures = append(fnDef.Signatures, sigs...)
			SortSignatures(fnDef.Signatures)
			fnDef.MaxForwardArgs = calcMaxForwardArgs(fnDef.Signatures)
			top.Data = fnDef
			r.ReplaceDefTop(name, top)
			return
		}
	}
	// No existing FnDefInfo on top — push a new one.
	fnDef := FnDefInfo{
		Name:       name,
		Signatures: append([]Signature(nil), sigs...),
	}
	SortSignatures(fnDef.Signatures)
	fnDef.MaxForwardArgs = calcMaxForwardArgs(fnDef.Signatures)
	r.PushDef(name, NewFnDef(fnDef))
}

// calcMaxForwardArgs returns the maximum number of forward args needed
// across all signatures. Under the unified dispatch rule, the forward
// limit is exactly sig.BarrierPos (which has been normalised at
// registration to len(Args) for old forward-prec sigs and 0 for
// old stack-only). This tells the engine how far ahead to scan and
// pre-evaluate paren expressions before signature matching.
func calcMaxForwardArgs(sigs []Signature) int {
	max := 0
	for i := range sigs {
		n := sigs[i].BarrierPos
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
	stack := r.DefStack(name)
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
	top, ok := r.TopOfDefStack(name)
	if !ok {
		return
	}
	if fnDef, ok := top.Data.(FnDefInfo); ok {
		fnDef.Signatures = KeepFallback(fnDef.Signatures)
		top.Data = fnDef
		r.ReplaceDefTop(name, top)
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
	sysStore.Set("fs", NewStoreValue(TStore, fsStore))

	// __val: a Store for user-defined values
	valStore := &StoreInstanceInfo{
		TypeName: "Object/Store",
		Data:     make(map[string]Value),
	}
	sysStore.Set("__val", NewStoreValue(TStore, valStore))

	root.Set("__sys", NewStoreValue(TStore, sysStore))
	r.PushExistingContext(root)
}

// Err returns the first registration error, or nil if none occurred.
func (r *Registry) Err() error {
	if len(r.errs) == 0 {
		return nil
	}
	return r.errs[0]
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

// UnaryNumOpNative builds a NativeFunc for a unary numeric operation with
// two overloads: [integer] -> [decimal] and [decimal] -> [decimal]. This
// is the value-returning sibling of RegisterUnaryNumOp; use it when
// composing a NativeFunc slice instead of mutating a Registry.
func UnaryNumOpNative(name string, op func(float64) float64) NativeFunc {
	handler := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		v, _ := args[0].AsNumber()
		return []Value{NewDecimal(op(v))}, nil
	}
	return NativeFunc{
		Name:        name,
		ForwardArgs: true,
		Signatures: []NativeSig{
			{Args: []*Type{TInteger}, Handler: handler, Returns: []*Type{TDecimal}},
			{Args: []*Type{TDecimal}, Handler: handler, Returns: []*Type{TDecimal}},
		},
	}
}

// BinaryNumOpNative builds a NativeFunc for a binary numeric operation
// with three float-typed overloads matching RegisterBinaryNumOp:
// [decimal, decimal], [number, decimal], and [decimal, number].
func BinaryNumOpNative(name string, op func(a, b float64) (float64, error)) NativeFunc {
	handler := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		a, _ := args[0].AsNumber()
		b, _ := args[1].AsNumber()
		result, err := op(a, b)
		if err != nil {
			return nil, err
		}
		return []Value{NewDecimal(result)}, nil
	}
	return NativeFunc{
		Name:        name,
		ForwardArgs: true,
		Signatures: []NativeSig{
			{Args: []*Type{TDecimal, TDecimal}, Handler: handler, Returns: []*Type{TDecimal}},
			{Args: []*Type{TNumber, TDecimal}, Handler: handler, Returns: []*Type{TDecimal}},
			{Args: []*Type{TDecimal, TNumber}, Handler: handler, Returns: []*Type{TDecimal}},
		},
	}
}

// BinaryIntOpNative builds a NativeFunc for a binary integer operation
// with one signature [integer, integer] -> [integer]. The
// value-returning sibling of RegisterBinaryIntOp.
func BinaryIntOpNative(name string, op func(a, b int64) (int64, error)) NativeFunc {
	handler := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		a, _ := args[0].AsConcreteInteger()
		b, _ := args[1].AsConcreteInteger()
		result, err := op(a, b)
		if err != nil {
			return nil, err
		}
		return []Value{NewInteger(result)}, nil
	}
	return NativeFunc{
		Name:        name,
		ForwardArgs: true,
		Signatures: []NativeSig{
			{Args: []*Type{TInteger, TInteger}, Handler: handler, Returns: []*Type{TInteger}},
		},
	}
}

// ValToString converts any scalar Value to its string representation.
func ValToString(v Value) string {
	if v.Data == nil && !v.VType.Equal(TNone) {
		return v.VType.String()
	}
	switch {
	case v.IsDepScalar():
		// Must come before TString/TInteger/etc. matches: the
		// lattice override makes DepString.Matches(TString) true,
		// so without this case AsString would crash on the wrong
		// payload type.
		return renderDepScalar(v)
	case v.VType.Matches(TString):
		_as8, _ := v.AsString()
		return _as8
	case v.IsAtom():
		_as9, _ := v.AsAtom()
		return _as9
	case v.VType.Matches(TDecimal):
		_as10, _ := v.AsDecimal()
		return formatDecimal(_as10)
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

// ContextStoreLookup looks up a key in the registry's context store,
// walking the prototype chain. Returns the value and true if found.
func ContextStoreLookup(r *Registry, key string) (Value, bool) {
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

// IsKnownPart reports whether part is already used by any registered
// type — builtin or dynamic. Used to enforce part-name uniqueness when
// installing a new `type Foo …`.
func (r *Registry) IsKnownPart(part string) bool {
	if Builtin.parts[part] {
		return true
	}
	if r != nil && r.Types != nil && r.Types.parts[part] {
		return true
	}
	return false
}

// RegisterPart records part as used by this Registry's dynamic types
// so subsequent IsKnownPart calls flag it. Idempotent.
func (r *Registry) RegisterPart(part string) {
	if r == nil || r.Types == nil {
		return
	}
	r.Types.parts[part] = true
}

// ResolveTypeLiteralDef checks whether a bare type literal (Data==nil) has
// a richer definition installed under the same name (e.g. an ObjectTypeInfo
// from RegisterResource or a `type Foo object {…}` binding). If so it
// returns that value; otherwise it returns the original unchanged. This
// lets the parser eagerly resolve all type names while the engine still
// picks up installed ObjectType defs.
//
// User-defined types now live in r.Types (post-§5.2); the DefStacks
// fallback is retained only for value-side ObjectType installations
// from outside the type word (e.g. legacy RegisterResource paths).
func ResolveTypeLiteralDef(v Value, reg *Registry) Value {
	if v.Data != nil || reg == nil || v.VType == nil {
		return v
	}
	name := TypeNameByID(v.VType.ID)
	if name == "" {
		return v
	}
	if tv, ok := reg.TopOfTypeStack(name); ok && tv.IsObjectType() {
		return tv
	}
	if top, ok := reg.TopOfDefStack(name); ok {
		if top.IsObjectType() {
			return top
		}
	}
	return v
}

// StoreKey converts a Value to a string key for the store.
func StoreKey(v Value) string {
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
