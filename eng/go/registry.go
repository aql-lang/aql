package eng

import (
	"fmt"
	"io"
	"os"
	"strconv"
)

// Registry is the kernel's shared state: function-name registrations,
// def/type stacks, capabilities, IO writers, check-mode state, control
// flow flags. Sub-engines share one Registry so state propagates
// naturally across nested Run calls.
//
// Concerns are grouped into sub-stores rather than living as flat
// fields:
//   - r.Defs    (*DefTable)    — def-name shadowing stacks
//   - r.Types   (*TypeTable)   — type-name shadowing stacks
//   - r.Check   (CheckState)   — static-analysis state
//
// New stack-like concerns should follow the same pattern.
type Registry struct {
	// Defs holds the stacked bodies for `def`-defined words. See deftable.go.
	Defs *DefTable
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
	Types *TypeTable // dynamic types installed by the `type` word; each push mints a fresh Type
	// Capabilities holds host-installed plugin slots. See capability.go.
	Capabilities *CapabilityRegistry
	// Ideals holds the type-kind descriptors — the registered,
	// dynamically controllable constructors `type` dispatches through.
	// See ideal.go and lang/doc/design/IDEAL.0.md.
	Ideals    *IdealRegistry
	Output    io.Writer // output writer for print/printstr and stdout
	ErrOutput io.Writer // error output writer for stderr
	Input     io.Reader // input reader for stdin
	// Modules owns module-loading state: the load set, the
	// module-ID counter, the host's init callback, and the native-
	// module resolver. See modules.go.
	Modules   *ModuleRegistry
	ParseFunc func(string) ([]Value, error) // parser callback (set externally to avoid circular import)
	// Contexts is the scoped context stack; top = current engine's context Store. See contextstack.go.
	Contexts *ContextStack
	// Args is the per-call args list stack. See argsstack.go.
	Args           *ArgsStack
	Manager        any               // external manager (e.g. UniversalManager) for SDK operations
	SDKCache       map[string]any    // cached SDK instances keyed by spec name
	BaseDir        string            // base directory for resolving relative file paths (set by loadFileModule)
	Source         string            // most recent source text for error reporting
	errs           []error           // registration errors accumulated during setup
	ready          bool              // true after initial setup; triggers dynamic help generation
	OnRegisterHook func(name string) // called when a function is registered after startup

	// Check holds all static type-checking state, bundled together
	// so the future predicate-sandbox work (TYPE-SYSTEM-REVIEW.md
	// §3.3) can snapshot/restore one field instead of ten.
	Check CheckState

	// FlowCtrl carries the active control-flow signal (break, continue,
	// ...). Set by the corresponding handlers; consumed by the engine's
	// Run loop. Lives on the registry rather than the engine so that
	// sub-engines (which share a registry) naturally propagate the
	// signal upward — the outer Run sees the flag after its handler
	// returns, without the signal having to ride the error channel.
	// See flowctrl.go.
	FlowCtrl FlowCtrl
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
	// consume. The "unset" sentinel is -1 — that's what gets
	// substituted with DefaultCheckStepBudget at run time. A real
	// zero is honored as "abort on the first step", which is
	// rarely useful but unambiguous. Once the running count
	// exceeds the resolved budget, the engine emits a
	// step_budget_exceeded diagnostic and returns immediately.
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
		Defs:         NewDefTable(),
		Contexts:     NewContextStack(),
		Args:         NewArgsStack(),
		Types:        NewDynamicTypeTable(),
		Capabilities: NewCapabilityRegistry(),
		Ideals:       NewIdealRegistry(),
		Modules:      NewModuleRegistry(),
		Output:       os.Stdout,
		ErrOutput:    os.Stderr,
		Input:        os.Stdin,
		SDKCache:     make(map[string]any),
		// StepBudget uses -1 as the "unset, use the project default"
		// sentinel. The Go zero (0) is honored as "abort on the first
		// step" so callers who want that have an unambiguous way to
		// express it; callers who omit the field get the default
		// without the historical zero-as-magic overload.
		Check: CheckState{StepBudget: -1},
	}
	registerKernelIdeals(r)
	return r, nil
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

// Register adds one or more signatures to a named function.
// Sentinel resolution: any sig with `BarrierPos == BarrierAllForward`
// (-1) is lifted to `len(Args)` by upsertFnDef. Stack-only sigs must
// set `BarrierPos: 0` explicitly. Signatures are stored in a
// FnDefInfo entry in DefStacks.
//
// There is only one registration entry point now — the historical
// `RegisterStackOnly` wrapper was retired in favor of an explicit
// per-sig `BarrierPos`.
func (r *Registry) Register(name string, sigs ...Signature) {
	for _, sig := range sigs {
		if len(sig.Args) > MaxArgs {
			r.errs = append(r.errs, fmt.Errorf("signature for %q has %d args, max is %d", name, len(sig.Args), MaxArgs))
			return
		}
	}
	r.upsertFnDef(name, sigs...)
	if r.ready && r.OnRegisterHook != nil {
		r.OnRegisterHook(name)
	}
}

// upsertFnDef finds or creates a FnDefInfo at the top of DefStacks[name]
// and appends the given compiled signatures. If the top entry is already
// a FnDefInfo, its Signatures are updated in place. Otherwise a new
// FnDefInfo is pushed.
//
// Sentinel resolution: `BarrierPos == BarrierAllForward` (-1) means
// "no `|` boundary specified — default this sig to all-forward
// dispatch." Resolved here to `len(Args)`. Stack-only sigs MUST set
// `BarrierPos: 0` explicitly at the call site; there is no per-word
// "stack default" mode (the ForwardArgs flag and RegisterStackOnly
// method were retired in the BarrierPos cleanup).
func (r *Registry) upsertFnDef(name string, sigs ...Signature) {
	for i := range sigs {
		if sigs[i].BarrierPos == BarrierAllForward {
			sigs[i].BarrierPos = len(sigs[i].Args)
		}
	}
	// If the top of the stack is already a FnDefInfo, update it in place.
	if top, ok := r.Defs.Top(name); ok {
		if fnDef, ok := top.Data.(FnDefInfo); ok {
			fnDef.Signatures = append(fnDef.Signatures, sigs...)
			SortSignatures(fnDef.Signatures)
			fnDef.MaxForwardArgs = calcMaxForwardArgs(fnDef.Signatures)
			top.Data = fnDef
			r.Defs.Replace(name, top)
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
	r.Defs.Push(name, NewFnDef(fnDef))
}

// calcMaxForwardArgs returns the maximum number of forward args
// needed across all signatures. Under the unified dispatch rule the
// forward limit is exactly `sig.BarrierPos`, which upsertFnDef
// resolves at registration: `BarrierAllForward` (-1) becomes
// `len(Args)`, `0` stays as explicit all-stack, intermediates pass
// through. This tells the engine how far ahead to scan and
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
	stack := r.Defs.Stack(name)
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
	top, ok := r.Defs.Top(name)
	if !ok {
		return
	}
	if fnDef, ok := top.Data.(FnDefInfo); ok {
		fnDef.Signatures = KeepFallback(fnDef.Signatures)
		top.Data = fnDef
		r.Defs.Replace(name, top)
	}
}

// InitRootContext initializes the root context Store with the __sys key.
// The __sys value is a Store/System instance containing system configuration.
// All containers at every depth are Stores.
func (r *Registry) InitRootContext() {
	root := &StoreInstanceInfo{
		TypeName: "Ideal/Store",
		Data:     make(map[string]Value),
	}

	// Create the System store.
	sysStore := &StoreInstanceInfo{
		TypeName: "Ideal/Store/System",
		Data:     make(map[string]Value),
	}

	// fs: a Store with {mem: false, impl: None}
	fsStore := &StoreInstanceInfo{
		TypeName: "Ideal/Store",
		Data:     make(map[string]Value),
	}
	fsStore.Set("mem", NewBoolean(false))
	fsStore.Set("impl", NewTypeLiteral(TNone))
	sysStore.Set("fs", NewStoreValue(TStore, fsStore))

	// __val: a Store for user-defined values
	valStore := &StoreInstanceInfo{
		TypeName: "Ideal/Store",
		Data:     make(map[string]Value),
	}
	sysStore.Set("__val", NewStoreValue(TStore, valStore))

	root.Set("__sys", NewStoreValue(TStore, sysStore))
	r.Contexts.PushExisting(root)
}

// Err returns the first registration error, or nil if none occurred.
func (r *Registry) Err() error {
	if len(r.errs) == 0 {
		return nil
	}
	return r.errs[0]
}

// --- Shared helpers used by multiple builtin files ---

// UnaryNumOpNative builds a NativeFunc for a unary numeric operation with
// two overloads: [integer] -> [decimal] and [decimal] -> [decimal]. This
// is the value-returning sibling of RegisterUnaryNumOp; use it when
// composing a NativeFunc slice instead of mutating a Registry.
func UnaryNumOpNative(name string, op func(float64) float64) NativeFunc {
	handler := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		v, _ := AsNumber(args[0])
		return []Value{NewDecimal(op(v))}, nil
	}
	return NativeFunc{
		Name: name,

		Signatures: []NativeSig{
			{Args: []*Type{TInteger}, Handler: handler, Returns: []*Type{TDecimal}, BarrierPos: -1},
			{Args: []*Type{TDecimal}, Handler: handler, Returns: []*Type{TDecimal}, BarrierPos: -1},
		},
	}
}

// BinaryNumOpNative builds a NativeFunc for a binary numeric operation
// with three float-typed overloads matching RegisterBinaryNumOp:
// [decimal, decimal], [number, decimal], and [decimal, number].
func BinaryNumOpNative(name string, op func(a, b float64) (float64, error)) NativeFunc {
	handler := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		a, _ := AsNumber(args[0])
		b, _ := AsNumber(args[1])
		result, err := op(a, b)
		if err != nil {
			return nil, err
		}
		return []Value{NewDecimal(result)}, nil
	}
	return NativeFunc{
		Name: name,

		Signatures: []NativeSig{
			{Args: []*Type{TDecimal, TDecimal}, Handler: handler, Returns: []*Type{TDecimal}, BarrierPos: -1},
			{Args: []*Type{TNumber, TDecimal}, Handler: handler, Returns: []*Type{TDecimal}, BarrierPos: -1},
			{Args: []*Type{TDecimal, TNumber}, Handler: handler, Returns: []*Type{TDecimal}, BarrierPos: -1},
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
		Name: name,

		Signatures: []NativeSig{
			{Args: []*Type{TInteger, TInteger}, Handler: handler, Returns: []*Type{TInteger}, BarrierPos: -1},
		},
	}
}

// ValToString converts any scalar Value to its string representation.
func ValToString(v Value) string {
	if v.Data == nil {
		return v.String()
	}
	switch {
	case v.IsDepScalar():
		// Must come before TString/TInteger/etc. matches: the
		// lattice override makes DepString.Matches(TString) true,
		// so without this case AsString would crash on the wrong
		// payload type.
		return renderDepScalar(v)
	case v.Parent.Matches(TString):
		_as8, _ := AsString(v)
		return _as8
	case IsAtom(v):
		_as9, _ := AsAtom(v)
		return _as9
	case v.Parent.Matches(TDecimal):
		_as10, _ := AsDecimal(v)
		return formatDecimal(_as10)
	case v.Parent.Matches(TInteger):
		_as11, _ := AsInteger(v)
		return strconv.FormatInt(_as11, 10)
	case v.Parent.Matches(TBoolean):
		_as12, _ := AsBoolean(v)
		if _as12 {
			return "true"
		}
		return "false"
	case IsPath(v):
		_as13, _ := AsPath(v)
		return _as13.String()
	case IsWord(v):
		_as14, _ := AsWord(v)
		return _as14.Name
	default:
		return v.String()
	}
}

// ContextStoreLookup looks up a key in the registry's context store,
// walking the prototype chain. Returns the value and true if found.
func ContextStoreLookup(r *Registry, key string) (Value, bool) {
	store := r.Contexts.Top()
	if store == nil {
		return Value{}, false
	}
	return store.Get(key)
}

// ContextSet stores a key-value pair in the root context store.
// Convenience method for programmatic setup (e.g. tests, query setup).
func (r *Registry) ContextSet(key string, val Value) {
	store := r.Contexts.Top()
	if store == nil {
		r.InitRootContext()
		store = r.Contexts.Top()
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
	if v.Data != nil || reg == nil || v.Carrier {
		return v
	}
	// A type literal IS its lattice node (by-value copy), so the
	// canonical identity is the value's own ID, not v.Parent.ID (the
	// supertype's ID).
	name := TypeNameByID(v.ID)
	if name == "" {
		return v
	}
	if top, ok := reg.Defs.Top(name); ok && IsObjectType(top) {
		return top
	}
	return v
}

// StoreKey converts a Value to a string key for the store.
func StoreKey(v Value) string {
	if v.Data == nil {
		return v.Parent.String()
	}
	if IsWord(v) {
		_as15, _ := AsWord(v)
		return _as15.Name
	}
	if v.Parent.Matches(TString) {
		_as16, _ := AsString(v)
		return _as16
	}
	if IsAtom(v) {
		_as17, _ := AsAtom(v)
		return _as17
	}
	if v.Parent.Matches(TInteger) {
		n, _ := AsInteger(v)
		return strconv.FormatInt(n, 10)
	}
	if v.Parent.Matches(TDecimal) {
		f, _ := AsDecimal(v)
		return FormatDecimal(f)
	}
	if v.Parent.Matches(TBoolean) {
		b, _ := AsBoolean(v)
		if b {
			return "true"
		}
		return "false"
	}
	return fmt.Sprintf("%v", v.Data)
}

// RegisterNativeFunc installs a NativeFunc into the registry, converts
// NativeSig to Signature, and registers with the appropriate precedence.
//
// The function name is validated against the language-fundamental
// word-name rule (ValidateWordName in word_name.go): must begin with
// [a-z] and contain only [a-z0-9-]. Engine-internal markers (`__`-
// prefixed) are exempt. A bad name accumulates into r.errs — callers
// can check r.Err() before relying on the registration.
func (r *Registry) RegisterNativeFunc(fn NativeFunc) {
	if err := ValidateWordName(fn.Name); err != nil {
		r.errs = append(r.errs, err)
		return
	}
	for _, sig := range fn.Signatures {
		//nolint:staticcheck // S1016: explicit field-by-field copy keeps any NativeSig↔Signature divergence visible
		s := Signature{
			Args:             sig.Args,
			Handler:          sig.Handler,
			FullStack:        sig.FullStack,
			Patterns:         sig.Patterns,
			QuoteArgs:        sig.QuoteArgs,
			NoEvalArgs:       sig.NoEvalArgs,
			NoEvalMapArgs:    sig.NoEvalMapArgs,
			TypeArgs:         sig.TypeArgs,
			BarrierPos:       sig.BarrierPos,
			Fallback:         sig.Fallback,
			Returns:          sig.Returns,
			ReturnsFn:        sig.ReturnsFn,
			RunInCheckMode:   sig.RunInCheckMode,
			CheckFullStackFn: sig.CheckFullStackFn,
		}
		// One path. `BarrierAllForward` (-1) on a NativeSig is the
		// "default all-forward" sentinel; `0` is explicit all-stack.
		// upsertFnDef resolves the sentinel once.
		r.Register(fn.Name, s)
	}
}

// CallAQL invokes an AQL function value (FnDefInfo) with a pre-matched
// signature and arguments in a sub-engine. The caller is responsible for
// signature matching — use MatchFnSig to find the matching sig.
//
//	sig := MatchFnSig(fn, args)
//	result, err := r.CallAQL(sig, args)
func (r *Registry) CallAQL(sig *FnSig, args []Value) ([]Value, error) {
	// Build token sequence (same as InstallFnDef handler).
	var tokens []Value
	var names []string

	// Push args list onto the args stack.
	argsCopy := make([]Value, len(args))
	copy(argsCopy, args)
	argsList := NewList(argsCopy)
	if err := r.Args.Push(argsList); err != nil {
		return nil, err
	}

	for i, p := range sig.Params {
		if p.Name != "" {
			arg := args[i]
			if arg.Parent.Equal(TList) && !arg.Quoted {
				arg.Quoted = true
			}
			InstallDef(r, p.Name, arg)
			names = append(names, p.Name)
		} else {
			tokens = append(tokens, args[i])
		}
	}
	body := make([]Value, len(sig.Body))
	copy(body, sig.Body)
	tokens = append(tokens, body...)

	// Snapshot DefStacks lengths before body execution so we can
	// clean up any defs created during body execution (Issue 2
	// from AQL-DX-REPORT: def leakage from fn bodies).
	defSnapshot := r.Defs.Snapshot()

	// Evaluate in a sub-engine with higher step limit for complex bodies.
	sub := NewTop(r)
	result, err := sub.Run(tokens)

	// Cleanup: pop args stack, undef named params, then clean up
	// any defs that were created during body execution. A Pop error
	// here means the args stack is nil — a misconfigured registry;
	// surface it only if sub.Run didn't already fail (the run error
	// is more informative).
	if _, popErr := r.Args.Pop(); popErr != nil && err == nil {
		err = popErr
	}
	for i := len(names) - 1; i >= 0; i-- {
		UninstallDef(r, names[i])
	}

	// Remove defs that were added during body execution.
	// Collect names first, then clean up outside the range loop
	// to avoid mutating DefStacks during iteration (UninstallDef
	// triggers InstallFnDef → Register → upsertFnDef which can
	// modify DefStacks entries for other names).
	var toClean []string
	for _, name := range r.Defs.Names() {
		if r.Defs.Depth(name) > defSnapshot[name] {
			toClean = append(toClean, name)
		}
	}
	for _, name := range toClean {
		target := defSnapshot[name]
		// Pop entries down to the snapshot length. Use a bounded
		// loop to avoid infinite looping if UninstallDef's rebuild
		// creates new entries.
		for attempts := 0; attempts < 100 && r.Defs.Depth(name) > target; attempts++ {
			UninstallDef(r, name)
		}
	}

	if err != nil {
		return nil, fmt.Errorf("CallAQL: %w", err)
	}
	return result, nil
}

// --- Error construction -------------------------------------------------

// AqlError constructs an AqlError that picks up the registry's source
// text automatically. Replaces the recurring `makeAqlError(code,
// detail, name, r.Source, "")` pattern across handlers — handlers
// just call `r.AqlError("signature_error", "no match for "+name,
// name)` and source threading is handled centrally.
//
// Use AqlErrorHint when a hint string is needed.
func (r *Registry) AqlError(code, detail, word string) error {
	src := ""
	if r != nil {
		src = r.Source
	}
	return makeAqlError(code, detail, word, src, "")
}

// AqlErrorHint is AqlError with an explicit hint string.
func (r *Registry) AqlErrorHint(code, detail, word, hint string) error {
	src := ""
	if r != nil {
		src = r.Source
	}
	return makeAqlError(code, detail, word, src, hint)
}

// ResolveTypedName resolves a name to its bound value. Post the
// TYPE-UNIFORM Phase 4 collapse there is a single binding store
// (DefTable) holding both type and value bindings, so this is one
// lookup: the capitalisation convention keeps type names and value
// names disjoint, so a name is bound at most one way.
func (r *Registry) ResolveTypedName(name string) (Value, bool) {
	if r == nil {
		return Value{}, false
	}
	return r.Defs.Top(name)
}

// TopTypeBody returns the body of name's active binding when that
// binding is a *type* binding (installed by a capitalised `def`), and
// (zero Value, false) otherwise — including when name is unbound or
// bound only as a value.
func (r *Registry) TopTypeBody(name string) (Value, bool) {
	if r == nil {
		return Value{}, false
	}
	if e, ok := r.Defs.TopEntry(name); ok && e.TypeDef != nil {
		return e.Body, true
	}
	return Value{}, false
}

// LookupTypeName returns the active lattice *Type for name: the minted
// def of a dynamic type binding (in the DefTable), or an external
// builtin registered by name. Returns nil if name names no type.
func (r *Registry) LookupTypeName(name string) *Type {
	if r == nil {
		return nil
	}
	if e, ok := r.Defs.TopEntry(name); ok && e.TypeDef != nil {
		return e.TypeDef
	}
	return r.Types.LookupBuiltinByName(name)
}

// ResolveTypedNameValue resolves a Value-shaped type reference to its
// concrete type value, capturing the source name when the input was
// a Word. Returns the resolved value, the source name (empty if v
// wasn't a Word), and ok=false only when v WAS a Word but couldn't
// be resolved through r.types or DefStacks.
//
// Replaces the
// `if v.IsWord() { w, _ := v.AsWord(); typeName = w.Name; if tv, ok :=
// r.types[w.Name]; ok { v = tv } else if ds := r.defStacks[w.Name];
// len(ds) > 0 { v = ds[len(ds)-1] } }` pattern in `defTypedHandler`,
// `is`, `inspect`, and `typeof` — extracting the name capture so
// downstream error messages can surface "type Bbd" rather than the
// rendered value form.
func (r *Registry) ResolveTypedNameValue(v Value) (resolved Value, name string, ok bool) {
	if !IsWord(v) {
		return v, "", true
	}
	w, _ := AsWord(v)
	rv, hit := r.ResolveTypedName(w.Name)
	if !hit {
		return v, w.Name, false
	}
	return rv, w.Name, true
}

// RunPredicate invokes a predicate-type fn against a candidate
// value, applying the None-or-value contract. Returns the
// predicate's output, a `matched` flag (true when the result is
// not-None), and an error for malformed predicates or invocation
// failures.
//
// The constraint must be a TFnDef or TFunction value carrying
// FnDefInfo with a single-arg first signature. Predicate types
// from `type Foo fn [x:Any Any [body]]` always satisfy this; other
// shapes return an error.
//
// CheckMode short-circuit: when r.Check.Mode is true the predicate
// body would run against carrier-typed input, which the body's
// `(x is String)`/`(x gte 10)`/etc. checks can't usefully evaluate
// (carriers fail those checks → every typed binding errors). Under
// CheckMode this helper returns (candidate, matched=true, nil) so
// downstream typed-def installation proceeds; the predicate's real
// behaviour is exercised at runtime.
//
// Sandboxing: predicate bodies are user-controlled fn bodies that
// could otherwise mutate registry state during a unify check.
// runPredicateSandboxed snapshots r.types and r.ctxStack before the
// CallAQL invocation and restores them on return — additions to
// r.types via `type Foo …` and pushes onto the context stack are
// rolled back. r.defStacks is already protected by CallAQL's own
// snapshot.
func (r *Registry) RunPredicate(constraint, candidate Value) (out Value, matched bool, err error) {
	if !constraint.Parent.Equal(TFnDef) && !constraint.Parent.Equal(TFunction) {
		return Value{}, false, fmt.Errorf("RunPredicate: constraint is not a fn (got %s)", constraint.Parent.String())
	}
	fnDef, ok := constraint.Data.(FnDefInfo)
	if !ok {
		return Value{}, false, fmt.Errorf("RunPredicate: constraint has invalid payload (got %T)", constraint.Data)
	}
	if len(fnDef.Sigs) == 0 || len(fnDef.Sigs[0].Params) != 1 {
		return Value{}, false, fmt.Errorf("RunPredicate: predicate must take exactly one argument")
	}
	// CheckMode: accept the binding without running the body. Real
	// predicate behaviour is asserted at runtime; here we only need
	// the analyser to keep flowing past the typed slot.
	if r != nil && r.Check.Mode {
		return candidate, true, nil
	}
	// Input-type gate: a predicate's declared input type acts as a
	// pre-filter. `"x" is Pos` for `Pos fn [[n:Integer] …]` rejects
	// at this gate without running the body, because the predicate
	// body's behavior on a non-Integer input is undefined (and
	// cross-type comparators like `gt` produce confusing answers).
	// Skip the gate for the empty case (input declared as Any or
	// unset) — those predicates explicitly accept any input.
	if inputT := fnDef.Sigs[0].Params[0].Type; inputT != nil && !inputT.Equal(TAny) {
		if candidate.Data == nil && !candidate.Carrier {
			// Bare type literal: skip the gate (the literal IS a type,
			// not an inhabitant — predicate has no value to test).
		} else if !candidate.Parent.Matches(inputT) {
			return candidate, false, nil
		}
	}
	// Sandbox the call so a mischievous predicate body can't mutate
	// r.types or the context stack out from under the surrounding
	// program.
	saved := snapshotPredicateState(r)
	defer restorePredicateState(r, saved)

	result, err := r.CallAQL(&fnDef.Sigs[0], []Value{candidate})
	if err != nil {
		return Value{}, false, err
	}
	if len(result) != 1 {
		return Value{}, false, fmt.Errorf("RunPredicate: predicate must return exactly one value, got %d", len(result))
	}
	out = result[0]
	// A predicate signals "doesn't match" by returning:
	//  - None — sentinel value or bare type literal (IsNoneShape).
	//  - Boolean false — but ONLY when the predicate's input domain
	//    doesn't include Boolean. The `n gt 0` style predicate
	//    (input=Integer, body→Boolean) uses Boolean as a verdict, so
	//    `false` means "no match". A Boolean-domain predicate like
	//    `def Flag fn [[b:Boolean] [Boolean] [b]]` legitimately
	//    accepts `false` as a value, so we must NOT short-circuit on
	//    Boolean returns when the input type accepts Boolean.
	//
	// When the body returns Boolean true (verdict form), the
	// candidate flows through unchanged — this preserves the
	// typed-def Reparent invariant (def x:Pos 5 ⇒ x's payload is 5,
	// not Boolean true). For value-transforming bodies (`guard val`)
	// the non-Boolean output IS the new value.
	if IsNoneShape(out) {
		return out, false, nil
	}
	inputT := fnDef.Sigs[0].Params[0].Type
	booleanIsValue := inputT != nil && TBoolean.Matches(inputT)
	if !booleanIsValue && out.Parent != nil && out.Parent.Equal(TBoolean) && out.Data != nil {
		if b, ok := out.Data.(BoolPayload); ok {
			if !b.B {
				return out, false, nil
			}
			return candidate, true, nil
		}
	}
	return out, true, nil
}
