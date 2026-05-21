package eng

// DefEntry is one binding on a name's stack. Body is the bound value.
// TypeDef is the minted lattice type when the binding is a *type*
// binding (a capitalised `def`), or nil for an ordinary value binding.
type DefEntry struct {
	Body    Value
	TypeDef *Type
}

// DefTable holds the stacked bindings for every name. Post the
// TYPE-UNIFORM Phase 4 collapse it is the *single* binding store —
// both `def`-bound values and the type bindings a capitalised `def`
// installs live here, keyed by name (the capitalisation convention
// keeps the two kinds of name disjoint, so one map suffices). Each
// name maps to a stack; the top is the active binding. `def NAME body`
// pushes, `undef NAME` (and the def-cleanup machinery) pops.
//
// A *type* binding additionally carries the minted lattice `*Type`
// (DefEntry.TypeDef) so `undef` can retire it from the type lattice.
//
// Extracted from Registry to keep that struct from accumulating stack-
// bookkeeping methods. Pair it with Snapshot / Restore for sandboxing
// patterns (fn-body, predicate, carrier merges) that need to roll back
// a region of pushes wholesale.
type DefTable struct {
	stacks map[string][]DefEntry
}

// NewDefTable returns an empty def table ready for use.
func NewDefTable() *DefTable {
	return &DefTable{stacks: make(map[string][]DefEntry)}
}

// Top returns the body of the most recent binding for name, or
// (zero Value, false) if name is unbound. Canonical read for "what
// does this name resolve to right now".
func (dt *DefTable) Top(name string) (Value, bool) {
	if dt == nil {
		return Value{}, false
	}
	ds := dt.stacks[name]
	if len(ds) == 0 {
		return Value{}, false
	}
	return ds[len(ds)-1].Body, true
}

// TopEntry returns the most recent binding (body plus the type def, if
// any) for name, or (zero DefEntry, false) if name is unbound.
func (dt *DefTable) TopEntry(name string) (DefEntry, bool) {
	if dt == nil {
		return DefEntry{}, false
	}
	ds := dt.stacks[name]
	if len(ds) == 0 {
		return DefEntry{}, false
	}
	return ds[len(ds)-1], true
}

// Push pushes a new value binding for name.
func (dt *DefTable) Push(name string, v Value) {
	if dt == nil {
		return
	}
	dt.stacks[name] = append(dt.stacks[name], DefEntry{Body: v})
}

// PushType pushes a new type binding for name: the body plus the
// minted lattice type that carries this declaration's identity.
func (dt *DefTable) PushType(name string, def *Type, body Value) {
	if dt == nil {
		return
	}
	dt.stacks[name] = append(dt.stacks[name], DefEntry{Body: body, TypeDef: def})
}

// Pop pops the top binding for name. Returns true if there was a
// binding to pop. When the stack becomes empty the entry is removed
// from the map so Has returns false.
func (dt *DefTable) Pop(name string) bool {
	_, ok := dt.PopEntry(name)
	return ok
}

// PopEntry pops the top binding for name and returns it. The returned
// DefEntry's TypeDef is non-nil when a type binding was popped — the
// caller (undef) uses it to retire the type from the lattice.
func (dt *DefTable) PopEntry(name string) (DefEntry, bool) {
	if dt == nil {
		return DefEntry{}, false
	}
	ds := dt.stacks[name]
	if len(ds) == 0 {
		return DefEntry{}, false
	}
	top := ds[len(ds)-1]
	if len(ds) == 1 {
		delete(dt.stacks, name)
	} else {
		dt.stacks[name] = ds[:len(ds)-1]
	}
	return top, true
}

// Has reports whether name has any active binding.
func (dt *DefTable) Has(name string) bool {
	if dt == nil {
		return false
	}
	return len(dt.stacks[name]) > 0
}

// IsType reports whether name's active binding is a type binding.
func (dt *DefTable) IsType(name string) bool {
	if dt == nil {
		return false
	}
	ds := dt.stacks[name]
	return len(ds) > 0 && ds[len(ds)-1].TypeDef != nil
}

// Depth returns the number of bindings currently stacked for name
// (0 if unbound).
func (dt *DefTable) Depth(name string) int {
	if dt == nil {
		return 0
	}
	return len(dt.stacks[name])
}

// Replace overwrites the body of the top binding for name with v,
// preserving the binding's type def. Returns true if there was a
// binding to replace; false (and no-op) if the stack was empty. Used
// by carrier-narrowing in `is` to re-bind the active iteration
// variable to a narrowed type.
func (dt *DefTable) Replace(name string, v Value) bool {
	if dt == nil {
		return false
	}
	ds := dt.stacks[name]
	if len(ds) == 0 {
		return false
	}
	ds[len(ds)-1].Body = v
	return true
}

// Truncate pops bindings from the top of name's stack until its depth
// equals want. If want >= current depth, no-op. If the stack becomes
// empty the entry is removed from the map.
func (dt *DefTable) Truncate(name string, want int) {
	if dt == nil {
		return
	}
	ds := dt.stacks[name]
	if want < 0 {
		want = 0
	}
	if want >= len(ds) {
		return
	}
	if want == 0 {
		delete(dt.stacks, name)
		return
	}
	dt.stacks[name] = ds[:want]
}

// Delete removes name's stack entirely. No-op if name is unbound.
func (dt *DefTable) Delete(name string) {
	if dt == nil {
		return
	}
	delete(dt.stacks, name)
}

// Set replaces name's entire stack with value bindings carrying the
// given bodies. If bodies is empty the entry is removed from the map.
// Used by UninstallFnSigs (removes a specific middle entry then writes
// back) and by the def-handler's compile-then-replace path that
// filters out fallback entries before re-installing — both operate on
// value-binding (fn-def) stacks.
func (dt *DefTable) Set(name string, bodies []Value) {
	if dt == nil {
		return
	}
	if len(bodies) == 0 {
		delete(dt.stacks, name)
		return
	}
	entries := make([]DefEntry, len(bodies))
	for i, b := range bodies {
		entries[i] = DefEntry{Body: b}
	}
	dt.stacks[name] = entries
}

// Stack returns a snapshot of the bodies currently stacked for name,
// oldest-first. Returns nil if name is unbound. The returned slice is
// owned by the caller.
func (dt *DefTable) Stack(name string) []Value {
	if dt == nil {
		return nil
	}
	ds := dt.stacks[name]
	if len(ds) == 0 {
		return nil
	}
	bodies := make([]Value, len(ds))
	for i, e := range ds {
		bodies[i] = e.Body
	}
	return bodies
}

// Names returns a snapshot of all names currently bound. The slice is
// owned by the caller — mutating it has no effect on the table.
// Iteration order is map-iteration order.
func (dt *DefTable) Names() []string {
	if dt == nil {
		return nil
	}
	names := make([]string, 0, len(dt.stacks))
	for k := range dt.stacks {
		names = append(names, k)
	}
	return names
}

// Snapshot returns a per-name depth map covering every currently-bound
// name. Pair with Restore to roll a region of code back to the
// snapshotted state — additions and pushes during the region are
// unwound in one call. Used by the fn-body sandbox, predicate
// sandboxing, and the carrier-merge join points that need to compare
// branch states against a common pre-state.
func (dt *DefTable) Snapshot() map[string]int {
	if dt == nil {
		return nil
	}
	snap := make(map[string]int, len(dt.stacks))
	for k, v := range dt.stacks {
		snap[k] = len(v)
	}
	return snap
}

// Restore rolls every stack back to the depths recorded in snap
// (typically obtained from Snapshot). Names that are present in the
// table but absent from snap are deleted entirely. Names whose
// recorded depth is zero are also deleted.
func (dt *DefTable) Restore(snap map[string]int) {
	if dt == nil {
		return
	}
	for name := range dt.stacks {
		want, ok := snap[name]
		if !ok {
			delete(dt.stacks, name)
			continue
		}
		dt.Truncate(name, want)
	}
}
