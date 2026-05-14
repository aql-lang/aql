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
	child := &StoreInstanceInfo{
		TypeName:  "Object/Store",
		Data:      make(map[string]Value),
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
