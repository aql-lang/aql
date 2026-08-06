package core

// StoreShapeInfo's state methods, moved beside their type at the core
// cut (Stage 4g): the shape RECORD and its accessors are core-owned
// state; the check piece's store-shape ANALYSIS stays above.

// RecordKey joins a written value carrier into the shape under key —
// the per-store twin of CheckState.RecordContextSet, same join
// semantics. Join-only: repeat writes widen, never replace (replace-
// on-write is the stage-4 flow-sensitivity step; join keeps every
// sandbox/branch interaction monotone).
func (s *StoreShapeInfo) RecordKey(key string, v Value) {
	if s == nil || key == "" {
		return
	}
	if s.KeyTypes == nil {
		s.KeyTypes = map[string]Value{}
	}
	if existing, ok := s.KeyTypes[key]; ok {
		s.KeyTypes[key] = JoinCarriersHook(existing, v)
		return
	}
	s.KeyTypes[key] = v
}

// LookupKey returns the joined carrier recorded for key through THIS
// store, or ok=false when this store never saw the key written (the
// caller then falls back to the flat map — today's behaviour).
func (s *StoreShapeInfo) LookupKey(key string) (Value, bool) {
	if s == nil {
		return Value{}, false
	}
	v, ok := s.KeyTypes[key]
	return v, ok
}

// RecordVal joins an unkeyed value write (a patrun `add`) into Vals. A
// value the shape cannot describe honestly — a dispatch-bearing stored
// Function/FnDef/Reach/Splice, whose reader re-dispatches it — poisons
// the join instead, and LookupVals declines from then on.
func (s *StoreShapeInfo) RecordVal(v Value) {
	if s == nil || s.ValsPoisoned {
		return
	}
	if v.Parent == nil || v.Parent.ConformsTo(TFunction) ||
		IsReach(v) || IsSplice(v) {
		s.ValsPoisoned = true
		s.Vals = Value{}
		return
	}
	if s.Vals.Parent == nil {
		s.Vals = v
		return
	}
	s.Vals = JoinCarriersHook(s.Vals, v)
}

// LookupVals returns the unkeyed value join, or ok=false when nothing
// was recorded or the join is poisoned.
func (s *StoreShapeInfo) LookupVals() (Value, bool) {
	if s == nil || s.ValsPoisoned || s.Vals.Parent == nil {
		return Value{}, false
	}
	return s.Vals, true
}

// CloneShape returns a shape with a COPIED KeyTypes map (entries — and
// any nested shape pointers inside them — still shared). Used where the
// RUNTIME disconnects aliasing with a deep copy (`flex` of a flex): the
// copy's later writes must not narrow reads through the original's
// claims and vice versa. Entry-level sharing is safe because all
// mutation is join-only.
func (s *StoreShapeInfo) CloneShape() *StoreShapeInfo {
	if s == nil {
		return nil
	}
	cp := &StoreShapeInfo{Scope: s.Scope, Vals: s.Vals, ValsPoisoned: s.ValsPoisoned, DeclaredVal: s.DeclaredVal}
	if s.KeyTypes != nil {
		cp.KeyTypes = make(map[string]Value, len(s.KeyTypes))
		for k, v := range s.KeyTypes {
			cp.KeyTypes[k] = v
		}
	}
	return cp
}
