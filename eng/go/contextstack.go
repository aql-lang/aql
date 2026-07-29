package eng

// ContextStack is the kernel's scoped-context stack. Each entry is a
// StoreInstanceInfo; the top is the currently-active context. Key
// resolution walks the prototype chain (each entry's Prototype field),
// giving scope-like lookup across nested pushes.
//
// Extracted from Registry to keep its surface small.
type ContextStack struct {
	stack []*StoreInstanceInfo
}

// NewContextStack returns an empty context stack.
func NewContextStack() *ContextStack {
	return &ContextStack{}
}

// Push pushes a new child Store whose prototype is parent. Resolution
// walks the prototype chain, so the new Store sees parent's keys
// underneath its own.
func (cs *ContextStack) Push(parent *StoreInstanceInfo) {
	if cs == nil {
		return
	}
	// Data is left NIL rather than an empty map, and that is load-bearing
	// rather than a micro-optimisation. Every write to a context layer goes
	// through CowSet, which builds a WHOLE NEW layer (`Data: {key: val}`,
	// prototype = the old one) instead of mutating this map — so a pushed
	// layer's map is never written to, only read, and a nil map reads exactly
	// like an empty one in Go.
	//
	// It matters because the VM pushes one of these per nested body
	// invocation (vmContext.enterBodyUnit): on every `each` / `fold` /
	// `filter` element, whether or not the body mentions `context`. Allocating
	// a map there doubled the per-invocation cost of the frame for no
	// observable benefit — see design/verse-report-defects-investigation.0.md
	// §B blocker 3, which named "or the map made lazy" as one of the two
	// remedies. StoreInstanceInfo.Set allocates on demand, so the one path
	// that DOES write in place stays safe.
	child := &StoreInstanceInfo{
		TypeName:  "Ideal/Store",
		Prototype: parent,
	}
	cs.stack = append(cs.stack, child)
}

// PushExisting appends an existing StoreInstanceInfo without wrapping it
// in a new child layer. Used by module loading to inherit the parent's
// context as the module's base before the module pushes its own
// copy-on-write layer. The common case (creating a fresh child) is
// Push.
func (cs *ContextStack) PushExisting(ctx *StoreInstanceInfo) {
	if cs == nil || ctx == nil {
		return
	}
	cs.stack = append(cs.stack, ctx)
}

// Pop removes the top entry, restoring the parent layer. No-op if the
// stack is empty.
func (cs *ContextStack) Pop() {
	if cs == nil || len(cs.stack) == 0 {
		return
	}
	cs.stack = cs.stack[:len(cs.stack)-1]
}

// Depth returns the number of live layers. Used by the check-mode
// context-shape minting to stamp StoreShapeInfo.Scope (the stage-2
// layering substrate — see store_shape.go).
func (cs *ContextStack) Depth() int {
	if cs == nil {
		return 0
	}
	return len(cs.stack)
}

// Top returns the top context Store, or nil if the stack is empty.
func (cs *ContextStack) Top() *StoreInstanceInfo {
	if cs == nil || len(cs.stack) == 0 {
		return nil
	}
	return cs.stack[len(cs.stack)-1]
}

// TopData returns the top context's Data map for handler-compat
// callers that work directly with map[string]Value. Returns nil if
// the stack is empty.
func (cs *ContextStack) TopData() map[string]Value {
	si := cs.Top()
	if si == nil {
		return nil
	}
	return si.Data
}

// UpdateChain updates stack entries affected by a COW operation.
// origRoot is the original Store that was COW'd (the prototype of the
// new root). newRoot is the COW'd replacement. Scans from the top of
// the stack (most likely match) and uses direct pointer comparison as
// a fast path before walking prototype chains.
//
// The walk relinks prototypes AS IT GOES, so it must never relink newRoot
// itself. `newRoot.Prototype == origRoot` by construction — newRoot is the
// COW of origRoot — so a walk that reached newRoot would match the relink
// condition and set `newRoot.Prototype = newRoot`: a self-cycle that makes
// every later missing-key lookup spin forever, as a `fatal error: stack
// overflow` the engine's recover() cannot catch.
//
// That is reachable as soon as TWO stack entries reach origRoot only through
// their prototypes: the first entry's relink points its chain at newRoot, and
// the second entry's walk then arrives there. Today the compiled path pushes
// only one such entry, so the cycle is latent rather than live — but it is
// the reason design/verse-report-defects-investigation.0.md §B lists "make
// UpdateChain cycle-safe" as the step nothing else can land before: giving
// the VM a per-body context frame creates the second entry, and a three-line
// patch that did so turned this into a default-path crash from a documented
// idiom (`do [ context.__sys.fs set mem true 1 ]`).
//
// Stopping at newRoot is both necessary and sufficient. Nothing beyond it
// needs relinking: the chain past newRoot continues to origRoot and then to
// origRoot's own ancestors, none of which have origRoot as their prototype.
func (cs *ContextStack) UpdateChain(origRoot, newRoot *StoreInstanceInfo) {
	if cs == nil {
		return
	}
	for i := len(cs.stack) - 1; i >= 0; i-- {
		entry := cs.stack[i]
		if entry == origRoot {
			cs.stack[i] = newRoot
			continue
		}
		for p := entry; p != nil; p = p.Prototype {
			if p == newRoot {
				// Already relinked by an earlier entry's walk (or the
				// replacement itself). Relinking here would self-cycle.
				break
			}
			if p.Prototype == origRoot {
				p.Prototype = newRoot
				break
			}
		}
	}
}

// Snapshot returns a shallow copy of the stack slice. Pair with Restore
// to roll back a region of code that may push, pop, or replace entries.
// Used by the predicate sandbox.
func (cs *ContextStack) Snapshot() []*StoreInstanceInfo {
	if cs == nil {
		return nil
	}
	out := make([]*StoreInstanceInfo, len(cs.stack))
	copy(out, cs.stack)
	return out
}

// Restore replaces the stack with snap (typically obtained from
// Snapshot). The caller owns snap — Restore does not copy.
func (cs *ContextStack) Restore(snap []*StoreInstanceInfo) {
	if cs == nil {
		return
	}
	cs.stack = snap
}
