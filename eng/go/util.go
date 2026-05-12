package eng

import "fmt"

// Shared utility helpers consolidating duplicated patterns spread
// across the engine package. The goal is a single, well-tested
// surface for the half-dozen interactions every handler repeats —
// type-literal vs. concrete-value distinction, panic-safe map/list
// access, type-checked map-field lookups, and DefStacks resolution.
//
// Each helper is independently testable; the test file
// (util_test.go) targets 100% coverage. New duplicated patterns
// found in future code should land here, not be re-inlined.

// IsTypeLiteral reports whether v is a bare type literal — a Value
// whose VType identifies a type but carries no concrete payload and
// is not a CheckMode carrier. Type literals have Data == nil and
// Carrier == false; the convention is established in CLAUDE.md and
// scattered through the codebase as raw `v.Data == nil` checks.
//
// The single exception is None: Value{VType:TNone, Data:nil}
// represents both the unit type AND the unique None value, so
// IsTypeLiteral returns false for None — the caller would otherwise
// treat None as a "type" rather than a value.
func IsTypeLiteral(v Value) bool {
	if v.Data != nil || v.Carrier {
		return false
	}
	if v.VType.Equal(TNone) {
		return false
	}
	return true
}

// IsConcrete reports whether v carries a real payload (not a type
// literal, not a Carrier). Mirrors the negation of IsTypeLiteral
// without the None exception — None values are concrete in the
// sense that they're real values flowing through the program.
func IsConcrete(v Value) bool {
	return v.Data != nil && !v.Carrier
}

// RequireConcreteList unwraps a list-typed Value into its ReadList,
// returning an error if the value is a type literal, a carrier, or
// otherwise lacks a concrete list payload. Use this at native-handler
// entry points that take a list arg via [TList] or [TAny]; it
// replaces the recurring `if args[i].Data == nil { return error }`
// preamble.
//
// The op string is included in the error so callers can identify
// their site without wrapping again.
func RequireConcreteList(v Value, op string) (ReadList, error) {
	if v.Data == nil {
		return ReadList{}, fmt.Errorf("%s: expected a concrete list, got type literal %s", op, v.VType.String())
	}
	if v.Carrier {
		return ReadList{}, fmt.Errorf("%s: expected a concrete list, got carrier %s", op, v.VType.String())
	}
	list := v.AsList()
	if list.IsNil() {
		return ReadList{}, fmt.Errorf("%s: value is not a list (got %s)", op, v.VType.String())
	}
	return list, nil
}

// RequireConcreteMap unwraps a map-typed Value into its ReadMap. As
// with RequireConcreteList, type literals, carriers, and map-subtypes
// (RecordTypeInfo, OptionsTypeInfo, ChildTypeInfo) that lack a
// concrete OrderedMap payload return an error.
func RequireConcreteMap(v Value, op string) (ReadMap, error) {
	if v.Data == nil {
		return nil, fmt.Errorf("%s: expected a concrete map, got type literal %s", op, v.VType.String())
	}
	if v.Carrier {
		return nil, fmt.Errorf("%s: expected a concrete map, got carrier %s", op, v.VType.String())
	}
	m := v.AsMap()
	if m == nil {
		return nil, fmt.Errorf("%s: value is not a concrete map (got %s)", op, v.VType.String())
	}
	return m, nil
}

// MapFieldString fetches a String-valued field from a ReadMap.
// Returns the string and true on hit; "" and false when the key is
// absent OR the value's type is not String. Replaces the
// `if v, ok := m.Get(k); ok && v.VType.Matches(TString) { s, _ := v.AsString(); … }`
// pattern that appeared in fileio.go, native_string_helpers.go, and
// native_type_make.go.
//
// Nil map → false (panic-safe).
func MapFieldString(m ReadMap, key string) (string, bool) {
	if m == nil {
		return "", false
	}
	v, ok := m.Get(key)
	if !ok || !v.VType.Matches(TString) {
		return "", false
	}
	s, err := v.AsString()
	if err != nil {
		return "", false
	}
	return s, true
}

// MapFieldInteger fetches an Integer-valued field. Same shape as
// MapFieldString.
func MapFieldInteger(m ReadMap, key string) (int64, bool) {
	if m == nil {
		return 0, false
	}
	v, ok := m.Get(key)
	if !ok || !v.VType.Matches(TInteger) || v.IsDepScalar() {
		return 0, false
	}
	n, err := v.AsInteger()
	if err != nil {
		return 0, false
	}
	return n, true
}

// MapFieldBoolean fetches a Boolean-valued field.
func MapFieldBoolean(m ReadMap, key string) (bool, bool) {
	if m == nil {
		return false, false
	}
	v, ok := m.Get(key)
	if !ok || !v.VType.Matches(TBoolean) {
		return false, false
	}
	b, err := v.AsBoolean()
	if err != nil {
		return false, false
	}
	return b, true
}

// MapFieldDecimal fetches a Decimal-valued field.
func MapFieldDecimal(m ReadMap, key string) (float64, bool) {
	if m == nil {
		return 0, false
	}
	v, ok := m.Get(key)
	if !ok || !v.VType.Matches(TDecimal) || v.IsDepScalar() {
		return 0, false
	}
	f, err := v.AsDecimal()
	if err != nil {
		return 0, false
	}
	return f, true
}

// TopOfDefStack returns the most recent binding for a name in the
// def stack, or zero Value and false if the stack is empty / name
// unbound. The canonical read for "what does this def-name resolve
// to right now"; all consumer code outside this file goes through
// this helper rather than indexing r.defStacks directly.
func (r *Registry) TopOfDefStack(name string) (Value, bool) {
	if r == nil {
		return Value{}, false
	}
	ds := r.defStacks[name]
	if len(ds) == 0 {
		return Value{}, false
	}
	return ds[len(ds)-1], true
}

// PushDef pushes a new binding for name onto the def stack. Mirrors
// PushType for the def side. Carrier merging, fn-body parameter
// installation, fold/scan/each accumulators, and module imports all
// route their stack writes through this helper.
func (r *Registry) PushDef(name string, v Value) {
	if r == nil {
		return
	}
	r.defStacks[name] = append(r.defStacks[name], v)
}

// PopDef pops the top binding for name. Returns true if there was a
// binding to pop. Mirrors PopType: when the stack becomes empty the
// entry is removed from the map so HasDef returns false.
func (r *Registry) PopDef(name string) bool {
	if r == nil {
		return false
	}
	ds := r.defStacks[name]
	if len(ds) == 0 {
		return false
	}
	if len(ds) == 1 {
		delete(r.defStacks, name)
		return true
	}
	r.defStacks[name] = ds[:len(ds)-1]
	return true
}

// HasDef reports whether name has any active def binding.
func (r *Registry) HasDef(name string) bool {
	if r == nil {
		return false
	}
	return len(r.defStacks[name]) > 0
}

// DefStackDepth returns the number of bindings currently stacked for
// name (0 if unbound). Used by the carrier-merge and fn-body sandbox
// paths that need to truncate back to a specific depth.
func (r *Registry) DefStackDepth(name string) int {
	if r == nil {
		return 0
	}
	return len(r.defStacks[name])
}

// ReplaceDefTop overwrites the top binding for name with v. Returns
// true if there was a binding to replace; false (and no-op) if the
// stack was empty. Used by carrier-narrowing in `is` to re-bind the
// active iteration variable to a narrowed type.
func (r *Registry) ReplaceDefTop(name string, v Value) bool {
	if r == nil {
		return false
	}
	ds := r.defStacks[name]
	if len(ds) == 0 {
		return false
	}
	ds[len(ds)-1] = v
	return true
}

// TruncateDefStack pops bindings from the top of name's stack until
// its depth equals want. If want >= current depth, no-op. If the
// stack becomes empty the entry is removed from the map.
func (r *Registry) TruncateDefStack(name string, want int) {
	if r == nil {
		return
	}
	ds := r.defStacks[name]
	if want < 0 {
		want = 0
	}
	if want >= len(ds) {
		return
	}
	if want == 0 {
		delete(r.defStacks, name)
		return
	}
	r.defStacks[name] = ds[:want]
}

// DeleteDef removes name's stack entirely. No-op if name is unbound.
func (r *Registry) DeleteDef(name string) {
	if r == nil {
		return
	}
	delete(r.defStacks, name)
}

// SetDefStack replaces name's entire stack with stack. If stack is
// empty the entry is removed from the map. Used by UninstallFnSigs
// (removes a specific middle entry then writes back) and by the
// def-handler's compile-then-replace path that filters out fallback
// entries before re-installing.
func (r *Registry) SetDefStack(name string, stack []Value) {
	if r == nil {
		return
	}
	if len(stack) == 0 {
		delete(r.defStacks, name)
		return
	}
	r.defStacks[name] = stack
}

// DefStack returns a read-only view of the current bindings stacked
// for name. The returned slice aliases the registry's storage —
// callers must not mutate it. Returns an empty slice if name is
// unbound.
func (r *Registry) DefStack(name string) []Value {
	if r == nil {
		return nil
	}
	return r.defStacks[name]
}

// DefNames returns a snapshot of all names currently bound in the
// def stacks. The slice is owned by the caller — mutating it has no
// effect on the registry. Iteration order is map-iteration order.
func (r *Registry) DefNames() []string {
	if r == nil {
		return nil
	}
	names := make([]string, 0, len(r.defStacks))
	for k := range r.defStacks {
		names = append(names, k)
	}
	return names
}

// SnapshotDefDepths returns a per-name depth map covering every
// currently-bound def name. Pair with RestoreToDefDepths to roll a
// region of code back to the snapshotted state — additions and pushes
// during the region are unwound in one call. Used by the fn-body
// sandbox, predicate sandboxing, and the carrier-merge join points
// that need to compare branch states against a common pre-state.
func (r *Registry) SnapshotDefDepths() map[string]int {
	if r == nil {
		return nil
	}
	snap := make(map[string]int, len(r.defStacks))
	for k, v := range r.defStacks {
		snap[k] = len(v)
	}
	return snap
}

// RestoreToDefDepths rolls every def stack back to the depths
// recorded in snap (typically obtained from SnapshotDefDepths). Names
// that are present in the registry but absent from snap are deleted
// entirely. Names whose recorded depth is zero are also deleted.
func (r *Registry) RestoreToDefDepths(snap map[string]int) {
	if r == nil {
		return
	}
	for name := range r.defStacks {
		want, ok := snap[name]
		if !ok {
			delete(r.defStacks, name)
			continue
		}
		r.TruncateDefStack(name, want)
	}
}

// TopOfTypeStack returns the most recent binding for a type name in
// the type stack, or zero Value and false if the stack is empty /
// name unbound. The type counterpart of TopOfDefStack.
func (r *Registry) TopOfTypeStack(name string) (Value, bool) {
	if r == nil || r.Types == nil {
		return Value{}, false
	}
	return r.Types.TopBody(name)
}

// PushType pushes a new binding for name onto the type stack. Each
// push mints a fresh Type so the new declaration carries a distinct
// identity even when it shadows an outer one. The previous top (if any)
// becomes shadowed and is restored when an `untype name` pops the new
// entry.
func (r *Registry) PushType(name string, v Value) {
	if r == nil || r.Types == nil {
		return
	}
	r.Types.PushType(name, v)
}

// PopType pops the top binding for name. Returns true if there was
// a binding to pop. If the stack becomes empty the entry is
// removed from the map so HasType returns false.
func (r *Registry) PopType(name string) bool {
	if r == nil || r.Types == nil {
		return false
	}
	_, ok := r.Types.PopType(name)
	return ok
}

// HasType reports whether name has any active type binding.
func (r *Registry) HasType(name string) bool {
	if r == nil || r.Types == nil {
		return false
	}
	return r.Types.Has(name)
}

// TypeStackDepth returns the number of bindings currently stacked for
// type name (0 if unbound).
func (r *Registry) TypeStackDepth(name string) int {
	if r == nil || r.Types == nil {
		return 0
	}
	return r.Types.Depth(name)
}

// TypeNames returns a snapshot of all names currently bound in the
// type stacks. Mirrors DefNames.
func (r *Registry) TypeNames() []string {
	if r == nil || r.Types == nil {
		return nil
	}
	return r.Types.Names()
}

// SnapshotTypeStacks returns a deep copy of the dynamic type table.
// Pair with RestoreTypeStacks to roll back arbitrary mutations (push,
// pop, replace) — depth-only snapshots aren't sufficient for callers
// like the predicate sandbox where the body may pop a stack and then
// re-push a different value.
func (r *Registry) SnapshotTypeStacks() *TypeTable {
	if r == nil || r.Types == nil {
		return nil
	}
	return r.Types.Clone()
}

// RestoreTypeStacks replaces the entire dynamic type table with snap.
// The caller is responsible for snap being a deep copy (see
// SnapshotTypeStacks); RestoreTypeStacks does not duplicate again.
func (r *Registry) RestoreTypeStacks(snap *TypeTable) {
	if r == nil {
		return
	}
	r.Types = snap
}

// --- ArgsStack helpers --------------------------------------------------

// PushArgs pushes an args list onto the fn-call args stack. Used by
// fn-body invocation paths (CallAQL, execFnDefSig) to make args
// available to the body via the `args` word.
func (r *Registry) PushArgs(args Value) {
	if r == nil {
		return
	}
	r.argsStack = append(r.argsStack, args)
}

// PopArgs pops the top args entry. Returns true if there was an entry
// to pop. Mirrors the PopDef shape — silent on empty rather than
// panicking.
func (r *Registry) PopArgs() bool {
	if r == nil || len(r.argsStack) == 0 {
		return false
	}
	r.argsStack = r.argsStack[:len(r.argsStack)-1]
	return true
}

// TopArgs returns the current top args entry (set by the active fn
// call). Returns zero Value and false if the stack is empty.
func (r *Registry) TopArgs() (Value, bool) {
	if r == nil || len(r.argsStack) == 0 {
		return Value{}, false
	}
	return r.argsStack[len(r.argsStack)-1], true
}

// --- Context-stack helpers (for the rare push-existing-ctx case) -------

// PushExistingContext appends an existing StoreInstanceInfo to the
// context stack without wrapping it in a new child layer. Used by
// module loading to inherit the parent's context as the module's base
// before the module pushes its own copy-on-write layer. The common
// case (creating a fresh child) is `PushContext`.
func (r *Registry) PushExistingContext(ctx *StoreInstanceInfo) {
	if r == nil || ctx == nil {
		return
	}
	r.ctxStack = append(r.ctxStack, ctx)
}

// --- CheckMode helpers --------------------------------------------------

// IsCheckMode reports whether the registry is currently in check
// (analyser) mode. Use this in handlers that need to short-circuit
// real work to avoid side effects during static analysis. When false,
// the handler should proceed as normal.
func (r *Registry) IsCheckMode() bool {
	return r != nil && r.Check.Mode
}

// CheckModeSkipsSideEffect reports whether check mode should suppress
// a side-effecting operation. Equivalent to IsCheckMode for now —
// kept distinct so the policy can be refined per category later
// (file write vs network vs store mutation) without churning every
// call site again.
func (r *Registry) CheckModeSkipsSideEffect() bool {
	return r.IsCheckMode()
}

// BeginCheckMode enables check mode and resets the per-pass state
// (diagnostics, step count, budget flag, defs-installed/used,
// context-type tracking). Returns a function that switches mode off
// when called — typically via `defer`. Diagnostics gathered during
// the pass remain accessible on r.Check.Diagnostics for the caller
// to inspect after the deferred function runs.
func (r *Registry) BeginCheckMode() func() {
	if r == nil {
		return func() {}
	}
	r.Check.Mode = true
	r.Check.Diagnostics = nil
	r.Check.StepCount = 0
	r.Check.BudgetTripped = false
	r.Check.DefsInstalled = nil
	r.Check.DefsUsed = nil
	r.Check.ContextTypes = nil
	return func() {
		r.Check.Mode = false
	}
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

// --- Pos threading ------------------------------------------------------

// WithPos returns v with its Pos copied from src. Use when a handler
// constructs a new Value from an input — error reporting downstream
// then has the source location even though the new value is
// structurally unrelated to the input.
func WithPos(v, src Value) Value {
	v.Pos = src.Pos
	return v
}

// ResolveTypedName resolves a name to its type value through the
// type-resolution chain used by the typed-def handler and `is`:
// r.types first (the dedicated type registry), then DefStacks (legacy
// path for record/object/DepScalar definitions). Returns the resolved
// value and true if found; zero Value and false otherwise.
//
// Centralises the lookup so future namespace changes (a single
// type/def store, scoped types) only need to update one site.
func (r *Registry) ResolveTypedName(name string) (Value, bool) {
	if r == nil {
		return Value{}, false
	}
	if tv, ok := r.TopOfTypeStack(name); ok {
		return tv, true
	}
	return r.TopOfDefStack(name)
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
	if !v.IsWord() {
		return v, "", true
	}
	w, _ := v.AsWord()
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
	if !constraint.VType.Equal(TFnDef) && !constraint.VType.Equal(TFunction) {
		return Value{}, false, fmt.Errorf("RunPredicate: constraint is not a fn (got %s)", constraint.VType.String())
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
	matched = !out.VType.Equal(TNone)
	return out, matched, nil
}

// predicateSandbox holds the slice/map state that RunPredicate
// snapshots before invoking a predicate body. DefStacks is NOT
// included — CallAQL handles that itself. r.Check is preserved by
// reference (the entire CheckState struct is copied) so any
// per-call diagnostics or step counters set during the predicate
// don't leak.
type predicateSandbox struct {
	types    *TypeTable
	ctxStack []*StoreInstanceInfo
	check    CheckState
}

func snapshotPredicateState(r *Registry) predicateSandbox {
	if r == nil {
		return predicateSandbox{}
	}
	ctxCopy := make([]*StoreInstanceInfo, len(r.ctxStack))
	copy(ctxCopy, r.ctxStack)
	return predicateSandbox{
		types:    r.SnapshotTypeStacks(),
		ctxStack: ctxCopy,
		check:    r.Check,
	}
}

func restorePredicateState(r *Registry, s predicateSandbox) {
	if r == nil {
		return
	}
	r.RestoreTypeStacks(s.types)
	r.ctxStack = s.ctxStack
	r.Check = s.check
}

// AsConcreteString unwraps a String-typed Value into its Go string,
// returning a clear error if the value is a DepScalar constraint
// payload rather than a concrete String. The lattice override makes
// `DepString.Matches(TString)` true for sig-matching purposes, so
// any code path that sees a TString value and immediately calls
// `AsString` will hit a `DepString → "" + error` silent miscompile
// when the caller swallows the error. Use AsConcreteString in any
// path where the concrete payload is required (display, comparison,
// indexing, …); the error is loud and discoverable.
func (v Value) AsConcreteString() (string, error) {
	if v.IsDepScalar() {
		return "", fmt.Errorf("AsConcreteString: value is a dependent-type constraint (%s), not a concrete String", v.VType.String())
	}
	return v.AsString()
}

// AsConcreteInteger — DepScalar-rejecting accessor. See AsConcreteString.
func (v Value) AsConcreteInteger() (int64, error) {
	if v.IsDepScalar() {
		return 0, fmt.Errorf("AsConcreteInteger: value is a dependent-type constraint (%s), not a concrete Integer", v.VType.String())
	}
	return v.AsInteger()
}

// AsConcreteDecimal — DepScalar-rejecting accessor. See AsConcreteString.
func (v Value) AsConcreteDecimal() (float64, error) {
	if v.IsDepScalar() {
		return 0, fmt.Errorf("AsConcreteDecimal: value is a dependent-type constraint (%s), not a concrete Decimal", v.VType.String())
	}
	return v.AsDecimal()
}

// AsConcreteBoolean — DepScalar-rejecting accessor. See AsConcreteString.
func (v Value) AsConcreteBoolean() (bool, error) {
	if v.IsDepScalar() {
		return false, fmt.Errorf("AsConcreteBoolean: value is a dependent-type constraint (%s), not a concrete Boolean", v.VType.String())
	}
	return v.AsBoolean()
}

// AsConcreteAtom — DepScalar-rejecting accessor. See AsConcreteString.
func (v Value) AsConcreteAtom() (string, error) {
	if v.IsDepScalar() {
		return "", fmt.Errorf("AsConcreteAtom: value is a dependent-type constraint (%s), not a concrete Atom", v.VType.String())
	}
	return v.AsAtom()
}

// FlattenDisjunctAlts returns the alternatives of a disjunct value
// or a single-element slice containing v if it isn't a disjunct.
// Replaces the
// `if v.IsDisjunct() { d, _ := v.AsDisjunct(); alts = append(alts,
// d.Alternatives...) } else { alts = append(alts, v) }` pattern in
// `tor`, `tand`, `tany`, and several unify branches.
//
// Inlines the type-assertion the IsDisjunct/AsDisjunct pair would
// otherwise duplicate so there's no unreachable error branch.
// Values whose VType is a Disjunct (Type/Disjunct or a subtype such
// as Type/Disjunct/Enum) but whose payload isn't a real DisjunctInfo
// fall back to the single-element slice.
func FlattenDisjunctAlts(v Value) []Value {
	if d, ok := v.Data.(DisjunctInfo); ok && v.VType.Matches(TDisjunct) {
		return d.Alternatives
	}
	return []Value{v}
}
