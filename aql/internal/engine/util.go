package engine

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
// unbound. Replaces the `if ds := r.DefStacks[name]; len(ds) > 0
// { top := ds[len(ds)-1]; … }` indexing dance that appears 16+
// times across carrier.go, forloop.go, native_definition_def.go,
// native_help.go, native_type_is.go, native_type_typedef.go,
// native_control_do.go, and match.go.
func (r *Registry) TopOfDefStack(name string) (Value, bool) {
	if r == nil {
		return Value{}, false
	}
	ds := r.DefStacks[name]
	if len(ds) == 0 {
		return Value{}, false
	}
	return ds[len(ds)-1], true
}

// TopOfTypeStack returns the most recent binding for a type name in
// the type stack, or zero Value and false if the stack is empty /
// name unbound. The type counterpart of TopOfDefStack.
func (r *Registry) TopOfTypeStack(name string) (Value, bool) {
	if r == nil {
		return Value{}, false
	}
	ts := r.Types[name]
	if len(ts) == 0 {
		return Value{}, false
	}
	return ts[len(ts)-1], true
}

// PushType pushes a new binding for name onto the type stack. The
// previous top (if any) becomes shadowed and is restored when an
// `untype name` pops the new entry.
func (r *Registry) PushType(name string, v Value) {
	if r == nil {
		return
	}
	r.Types[name] = append(r.Types[name], v)
}

// PopType pops the top binding for name. Returns true if there was
// a binding to pop. If the stack becomes empty the entry is
// removed from the map so HasType returns false.
func (r *Registry) PopType(name string) bool {
	if r == nil {
		return false
	}
	ts := r.Types[name]
	if len(ts) == 0 {
		return false
	}
	if len(ts) == 1 {
		delete(r.Types, name)
		return true
	}
	r.Types[name] = ts[:len(ts)-1]
	return true
}

// HasType reports whether name has any active type binding.
func (r *Registry) HasType(name string) bool {
	if r == nil {
		return false
	}
	return len(r.Types[name]) > 0
}

// ResolveTypedName resolves a name to its type value through the
// type-resolution chain used by the typed-def handler and `is`:
// r.Types first (the dedicated type registry), then DefStacks (legacy
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
// be resolved through r.Types or DefStacks.
//
// Replaces the
// `if v.IsWord() { w, _ := v.AsWord(); typeName = w.Name; if tv, ok :=
// r.Types[w.Name]; ok { v = tv } else if ds := r.DefStacks[w.Name];
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
// runPredicateSandboxed snapshots r.Types and r.ctxStack before the
// CallAQL invocation and restores them on return — additions to
// r.Types via `type Foo …` and pushes onto the context stack are
// rolled back. r.DefStacks is already protected by CallAQL's own
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
	// r.Types or the context stack out from under the surrounding
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
	types    map[string][]Value
	ctxStack []*StoreInstanceInfo
	check    CheckState
}

func snapshotPredicateState(r *Registry) predicateSandbox {
	if r == nil {
		return predicateSandbox{}
	}
	// Deep-copy each type stack so a predicate body that calls
	// PushType / PopType can't mutate our snapshot via slice
	// aliasing.
	typesCopy := make(map[string][]Value, len(r.Types))
	for k, stack := range r.Types {
		dup := make([]Value, len(stack))
		copy(dup, stack)
		typesCopy[k] = dup
	}
	ctxCopy := make([]*StoreInstanceInfo, len(r.ctxStack))
	copy(ctxCopy, r.ctxStack)
	return predicateSandbox{
		types:    typesCopy,
		ctxStack: ctxCopy,
		check:    r.Check,
	}
}

func restorePredicateState(r *Registry, s predicateSandbox) {
	if r == nil {
		return
	}
	r.Types = s.types
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
// Values whose VType is TDisjunct but whose payload isn't a real
// DisjunctInfo fall back to the single-element slice.
func FlattenDisjunctAlts(v Value) []Value {
	if d, ok := v.Data.(DisjunctInfo); ok && v.VType.Equal(TDisjunct) {
		return d.Alternatives
	}
	return []Value{v}
}
