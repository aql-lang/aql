package eng

// DefTable holds the stacked bodies for `def`-defined words. Each name
// maps to a stack; the top is the active binding. `def NAME body` pushes,
// `undef NAME` (and the def-cleanup machinery) pops. Mirrors the
// shadowing semantics that TypeTable provides for typed names.
//
// Extracted from Registry to keep that struct from accumulating thirteen
// stack-bookkeeping methods. Pair it with SnapshotDefDepths /
// RestoreToDefDepths for sandboxing patterns (fn-body, predicate, carrier
// merges) that need to roll back a region of pushes wholesale.
type DefTable struct {
	stacks map[string][]Value
}

// NewDefTable returns an empty def table ready for use.
func NewDefTable() *DefTable {
	return &DefTable{stacks: make(map[string][]Value)}
}

// Top returns the most recent binding for name, or (zero Value, false)
// if name is unbound. Canonical read for "what does this def resolve to
// right now".
func (dt *DefTable) Top(name string) (Value, bool) {
	if dt == nil {
		return Value{}, false
	}
	ds := dt.stacks[name]
	if len(ds) == 0 {
		return Value{}, false
	}
	return ds[len(ds)-1], true
}

// Push pushes a new binding for name onto the def stack.
func (dt *DefTable) Push(name string, v Value) {
	if dt == nil {
		return
	}
	dt.stacks[name] = append(dt.stacks[name], v)
}

// Pop pops the top binding for name. Returns true if there was a binding
// to pop. When the stack becomes empty the entry is removed from the map
// so Has returns false.
func (dt *DefTable) Pop(name string) bool {
	if dt == nil {
		return false
	}
	ds := dt.stacks[name]
	if len(ds) == 0 {
		return false
	}
	if len(ds) == 1 {
		delete(dt.stacks, name)
		return true
	}
	dt.stacks[name] = ds[:len(ds)-1]
	return true
}

// Has reports whether name has any active binding.
func (dt *DefTable) Has(name string) bool {
	if dt == nil {
		return false
	}
	return len(dt.stacks[name]) > 0
}

// Depth returns the number of bindings currently stacked for name
// (0 if unbound).
func (dt *DefTable) Depth(name string) int {
	if dt == nil {
		return 0
	}
	return len(dt.stacks[name])
}

// Replace overwrites the top binding for name with v. Returns true if
// there was a binding to replace; false (and no-op) if the stack was
// empty. Used by carrier-narrowing in `is` to re-bind the active
// iteration variable to a narrowed type.
func (dt *DefTable) Replace(name string, v Value) bool {
	if dt == nil {
		return false
	}
	ds := dt.stacks[name]
	if len(ds) == 0 {
		return false
	}
	ds[len(ds)-1] = v
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

// Set replaces name's entire stack with stack. If stack is empty the
// entry is removed from the map. Used by UninstallFnSigs (removes a
// specific middle entry then writes back) and by the def-handler's
// compile-then-replace path that filters out fallback entries before
// re-installing.
func (dt *DefTable) Set(name string, stack []Value) {
	if dt == nil {
		return
	}
	if len(stack) == 0 {
		delete(dt.stacks, name)
		return
	}
	dt.stacks[name] = stack
}

// Stack returns a read-only view of the current bindings stacked for
// name. The returned slice aliases the table's storage — callers must
// not mutate it. Returns nil if name is unbound.
func (dt *DefTable) Stack(name string) []Value {
	if dt == nil {
		return nil
	}
	return dt.stacks[name]
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
// snapshotted state — additions and pushes during the region are unwound
// in one call. Used by the fn-body sandbox, predicate sandboxing, and
// the carrier-merge join points that need to compare branch states
// against a common pre-state.
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

// Restore rolls every def stack back to the depths recorded in snap
// (typically obtained from Snapshot). Names that are present in the
// table but absent from snap are deleted entirely. Names whose recorded
// depth is zero are also deleted.
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
