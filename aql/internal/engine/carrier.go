package engine

import "strings"

// Carrier-based static type-checking support.
//
// A "carrier" is a normal Value with Carrier=true and (typically)
// Data=nil: it carries only type information, not a concrete payload.
// The engine is driven in check mode by Registry.CheckMode. In that
// mode, the same dispatch machinery (matchSignature, forward
// collection, sort order, etc.) runs, but execMatch consults
// Signature.Returns to synthesise carrier results instead of calling
// the handler. This keeps runtime and checker in absolute parity.
//
// This file contains only the minimal helpers needed for the initial
// slice: a conversion from concrete literal values to carriers, and a
// carrier-result builder for a matched signature.

// NewCarrier constructs a carrier Value for the given type. Data is
// nil for scalar types. For TList and TMap, Data is set to a
// ChildTypeInfo wrapping an Any carrier so the carrier satisfies
// positionalMatch's "concrete list/map" rule (it rejects values
// whose Data==nil when the signature requires a concrete TList or
// TMap). Typed-list carriers (element type known) are produced via
// NewCarrierTypedList / NewCarrierTypedListValue.
func NewCarrier(t Type) Value {
	v := newValue(t, nil)
	v.Carrier = true
	if t.Equal(TList) || t.Equal(TMap) {
		v.Data = ChildTypeInfo{Child: Value{VType: TAny, Carrier: true}}
	}
	return v
}

// NewCarrierTypedList constructs a typed-list carrier — a list
// carrier whose element type is known. Implemented as a regular
// Value with VType=TList and Data=ChildTypeInfo{Child: NewCarrier(elem)}.
// The Carrier flag is still set so the rest of the engine treats it
// as abstract. Downstream list-consuming words can recover the
// element carrier via dataListElemType.
func NewCarrierTypedList(elem Type) Value {
	v := NewTypedList(NewCarrier(elem))
	v.Carrier = true
	return v
}

// NewCarrierTypedListValue constructs a typed-list carrier whose
// element is an arbitrary carrier Value. Use this when the element
// itself is a typed list (nested lists), a disjunct, or otherwise
// needs more structure than a bare VType.
func NewCarrierTypedListValue(child Value) Value {
	v := NewTypedList(child)
	v.Carrier = true
	return v
}

// ReturnsPreserveListAt builds a ReturnsFunc that returns a typed-
// list carrier whose element type matches the data-list arg at
// index i. Used by list-preserving words like reverse, take, shed,
// unique, at, sortby — they return a list of the same element type
// as their input.
func ReturnsPreserveListAt(i int) ReturnsFunc {
	return func(args []Value) []Value {
		if i < 0 || i >= len(args) {
			return []Value{NewCarrier(TList)}
		}
		elem := dataListElemTypeFromValue(args[i])
		return []Value{NewCarrierTypedList(elem)}
	}
}

// ReturnsListElemAt builds a ReturnsFunc that returns the element
// type carrier of the data-list arg at index i. Used by words like
// head/first (if added) that pick a single element out of a list.
func ReturnsListElemAt(i int) ReturnsFunc {
	return func(args []Value) []Value {
		if i < 0 || i >= len(args) {
			return []Value{NewCarrier(TAny)}
		}
		elem := dataListElemTypeFromValue(args[i])
		return []Value{NewCarrier(elem)}
	}
}

// dataListElemTypeFromValue is a package-level duplicate of
// dataListElemType that lives in carrier.go so ReturnsFunc helpers
// don't depend on the native_array_higher.go symbol. It reads the
// ChildTypeInfo first, then joins concrete element VTypes.
func dataListElemTypeFromValue(data Value) Type {
	if data.Data == nil {
		return TAny
	}
	if ct, ok := data.Data.(ChildTypeInfo); ok {
		return ct.Child.VType
	}
	list := data.AsList()
	if list.IsNil() || list.Len() == 0 {
		return TAny
	}
	t := list.Get(0).VType
	for i := 1; i < list.Len(); i++ {
		t = commonAncestorType(t, list.Get(i).VType)
		if t.Equal(TAny) {
			break
		}
	}
	return t
}

// toCarrier converts a concrete Value to its carrier form. Control /
// structural tokens (words, marks, moves, open-paren, paren-expr,
// interp-string, return-check, def-cleanup, forward) are returned
// unchanged: they drive dispatch and must retain their payloads.
// Lists and maps are returned unchanged for now so that list/map
// signature matching keeps working; carrier-aware list/map handling
// is future work.
func toCarrier(v Value) Value {
	if v.IsWord() || v.IsForward() || v.IsMark() || v.IsMove() ||
		v.IsOpenParen() || v.IsParenExpr() || v.IsInterpString() ||
		v.IsReturnCheck() || v.IsDefCleanup() {
		return v
	}
	// Keep lists and maps concrete for now — matchSignature relies
	// on Data presence for a few compound cases.
	if v.VType.Equal(TList) || v.VType.Equal(TMap) {
		return v
	}
	// Already a carrier.
	if v.Carrier {
		return v
	}
	v.Carrier = true
	v.Data = nil
	return v
}

// StripToCarriers returns a copy of in where every non-structural value
// has been converted to its carrier form. Used at the top-level Run()
// entry to bootstrap check-mode execution.
func StripToCarriers(in []Value) []Value {
	out := make([]Value, len(in))
	for i, v := range in {
		out[i] = toCarrier(v)
	}
	return out
}

// carrierResults returns the carrier Values that a matched signature
// produces in check mode. Resolution order:
//
//  1. If sig.ReturnsFn is set, it is invoked with the carrier-typed
//     args; the results are coerced to carriers (Carrier=true, Data
//     stripped for scalar types) and returned.
//  2. Otherwise, if sig.Returns is non-empty, one fresh carrier is
//     produced per declared Returns type.
//  3. Otherwise a diagnostic is recorded and a single TAny carrier is
//     returned so the checker can keep making progress.
//
// args are the carrier-typed input values in signature order (same
// args that would be passed to the runtime handler). pos carries the
// word's source location so diagnostics can point at it.
func carrierResults(r *Registry, word string, sig *Signature, args []Value, pos SrcPos) []Value {
	if sig.ReturnsFn != nil {
		raw := sig.ReturnsFn(args)
		out := make([]Value, len(raw))
		for i, v := range raw {
			out[i] = toCarrier(v)
		}
		return out
	}
	// Explicit nil (no annotation) triggers the fallback. An empty but
	// non-nil slice is a valid "returns nothing" declaration.
	if sig.Returns == nil {
		r.addCheckDiagnostic(CheckDiagnostic{
			Code:   "missing_returns",
			Detail: "word " + word + " has no declared Returns for matched signature; assuming Any",
			Word:   word,
			Row:    pos.Row,
			Col:    pos.Col,
		})
		return []Value{NewCarrier(TAny)}
	}
	out := make([]Value, len(sig.Returns))
	for i, t := range sig.Returns {
		out[i] = NewCarrier(t)
	}
	return out
}

// ReturnsIdentity is a ReturnsFunc helper that returns its inputs
// unchanged (as carriers). Use for stack operations that preserve
// their inputs — dup, swap, over, rot, etc. — where the output types
// are directly expressible in terms of the input types.
//
// The mapping is a permutation-description slice: result[i] = args[mapping[i]].
// Example: swap is ReturnsIdentity(1, 0); over is ReturnsIdentity(0, 1, 0).
func ReturnsIdentity(mapping ...int) ReturnsFunc {
	return func(args []Value) []Value {
		out := make([]Value, len(mapping))
		for i, m := range mapping {
			if m < 0 || m >= len(args) {
				out[i] = NewCarrier(TAny)
				continue
			}
			out[i] = args[m]
		}
		return out
	}
}

// ReturnsStatic builds a ReturnsFunc that always produces a fixed list
// of carrier types, independent of args. Equivalent to setting Returns
// directly; provided so ReturnsFn call sites can be uniform.
func ReturnsStatic(types ...Type) ReturnsFunc {
	return func(_ []Value) []Value {
		out := make([]Value, len(types))
		for i, t := range types {
			out[i] = NewCarrier(t)
		}
		return out
	}
}

// ReturnsNumericBinary models the common arithmetic pattern: when
// both args are integers the result is an integer, otherwise the
// result is a decimal. Applies to add, sub, mul, div, mod, pow when
// the matched signature is [TNumber, TNumber].
func ReturnsNumericBinary() ReturnsFunc {
	return func(args []Value) []Value {
		if len(args) == 2 &&
			args[0].VType.Matches(TInteger) &&
			args[1].VType.Matches(TInteger) {
			return []Value{NewCarrier(TInteger)}
		}
		return []Value{NewCarrier(TDecimal)}
	}
}

// commonAncestorType returns the longest common prefix of two type
// paths, as a new Type. For example, given Number/Integer/42 and
// Number/Integer/99, returns Number/Integer. Returns TAny if there is
// no shared prefix.
func commonAncestorType(a, b Type) Type {
	n := len(a.Parts)
	if len(b.Parts) < n {
		n = len(b.Parts)
	}
	shared := 0
	for shared < n && a.Parts[shared] == b.Parts[shared] {
		shared++
	}
	if shared == 0 {
		return TAny
	}
	parts := make([]string, shared)
	copy(parts, a.Parts[:shared])
	return Type{Parts: parts}
}

// CarrierDisjunctCap is the maximum number of alternatives a carrier
// disjunction may hold before it is widened to the common ancestor
// of all alternatives. Matches the report's recommended cap of 8.
const CarrierDisjunctCap = 8

// flattenAlternatives walks a carrier value and returns the unique
// type literals it represents. For a disjunct carrier, flattens its
// alternatives recursively; for any other carrier, returns a single
// type literal of its VType.
func flattenAlternatives(v Value) []Value {
	if v.IsDisjunct() {
		di, _ := v.AsDisjunct()
		var out []Value
		for _, alt := range di.Alternatives {
			out = append(out, flattenAlternatives(alt)...)
		}
		return out
	}
	return []Value{NewTypeLiteral(v.VType)}
}

// JoinCarriers folds two carriers into a single carrier that
// represents the disjunction of both. Applies a few simple
// normalisations:
//
//   - Identical VTypes collapse to one carrier.
//   - If one side is a strict subtype of the other, the parent wins.
//   - Sibling literal types (e.g. Number/Integer/42 vs Number/Integer/99)
//     collapse to their nearest common ancestor (Number/Integer).
//   - Disjunctions wider than CarrierDisjunctCap widen to the common
//     ancestor of all alternatives.
//   - Otherwise a TDisjunct carrier is returned whose Data is a
//     DisjunctInfo listing the unique alternative type literals.
//
// This is the primary join used when the checker needs to combine
// two branch outcomes (e.g. `if` then/else).
func JoinCarriers(a, b Value) Value {
	if a.VType.Equal(b.VType) && !a.IsDisjunct() && !b.IsDisjunct() {
		out := a
		out.Carrier = true
		out.Data = nil
		return out
	}
	if !a.IsDisjunct() && !b.IsDisjunct() {
		if a.VType.Matches(b.VType) {
			// a is subtype of b → widen to b
			return NewCarrier(b.VType)
		}
		if b.VType.Matches(a.VType) {
			return NewCarrier(a.VType)
		}
		// Check for a non-trivial common ancestor (shared prefix of at
		// least one part). This collapses value-tagged literals (e.g.
		// Number/Integer/42 vs Number/Integer/99 → Number/Integer).
		anc := commonAncestorType(a.VType, b.VType)
		if len(anc.Parts) > 0 && !anc.Equal(TAny) {
			return NewCarrier(anc)
		}
	}
	// Gather unique alternatives across a and b, subsume subtypes,
	// then apply the width cap.
	alts := mergeAlternatives(
		flattenAlternatives(a),
		flattenAlternatives(b),
	)
	if len(alts) == 1 {
		return NewCarrier(alts[0].VType)
	}
	if len(alts) > CarrierDisjunctCap {
		t := alts[0].VType
		for i := 1; i < len(alts); i++ {
			t = commonAncestorType(t, alts[i].VType)
		}
		return NewCarrier(t)
	}
	v := NewDisjunct(alts)
	v.Carrier = true
	return v
}

// mergeAlternatives merges two slices of type-literal alternatives
// into a single slice with subsumption: if one alternative is a
// subtype of another, only the parent is kept. Duplicates are
// removed.
func mergeAlternatives(a, b []Value) []Value {
	combined := append([]Value(nil), a...)
	combined = append(combined, b...)
	// Deduplicate by VType path, subsume subtypes.
	out := combined[:0]
outer:
	for i, cand := range combined {
		// Skip if already subsumed by an earlier entry.
		for j := 0; j < i; j++ {
			if combined[j].VType.Equal(cand.VType) {
				continue outer // duplicate
			}
		}
		// Drop candidates whose type is a strict subtype of any
		// other candidate.
		for j, other := range combined {
			if j == i {
				continue
			}
			if cand.VType.Equal(other.VType) {
				continue
			}
			if cand.VType.Matches(other.VType) {
				continue outer // subsumed
			}
		}
		out = append(out, cand)
	}
	return out
}

// checkModeFallbackPositions returns up to n stack indices to use as
// argument positions when a check-mode fallback fires (no signature
// matched, assume first candidate). Values before the pointer are
// preferred (normal stack order); any shortfall is filled from
// values after the pointer, skipping control tokens. Types are not
// verified — this is the "assume" path.
func (e *Engine) checkModeFallbackPositions(n int) []int {
	positions := e.resolvedIndicesBefore(n)
	remaining := n - len(positions)
	for i := e.pointer + 1; remaining > 0 && i < len(e.stack); i++ {
		v := e.stack[i]
		if v.IsForward() || v.IsMark() || v.IsMove() ||
			v.IsOpenParen() || v.IsReturnCheck() || v.IsDefCleanup() {
			continue
		}
		positions = append(positions, i)
		remaining--
	}
	return positions
}

// checkModeAssumeSig is the recovery path for unmatched signatures in
// check mode: emit a diagnostic (with pos attached), gather up to N
// adjacent positions as synthetic args, synthesise carrier results
// from the assumed signature, and splice them over the word +
// consumed positions.
//
// This path deliberately bypasses forward collection and type
// matching — both would cascade failures. The trade-off is that the
// checker reports one diagnostic per site and keeps going with the
// assumed signature's declared return types (or Any if unannotated).
func (e *Engine) checkModeAssumeSig(w WordInfo, fn *FnDefInfo, fallback *Signature, pos SrcPos) error {
	// Gather candidate positions once and try to pick a signature
	// whose arity matches and whose declared types are compatible
	// with (or at least not contradicted by) the actual carrier
	// args. TAny carriers are treated as wildcards.
	best := fallback
	bestMatch := -1
	// Scan all signatures; prefer: concrete matches > same arity > fallback.
	for i := range fn.Signatures {
		s := &fn.Signatures[i]
		if s.Fallback {
			continue
		}
		n := len(s.Args)
		pos := e.checkModeFallbackPositions(n)
		if len(pos) != n {
			continue
		}
		score := 0
		compatible := true
		for j, p := range pos {
			av := e.stack[p]
			// TAny carriers match any type without penalty.
			if av.VType.Equal(TAny) {
				continue
			}
			if sigTypeMatches(av, s.Args[j]) {
				score++
				continue
			}
			// Incompatible unless the actual carrier is Any.
			compatible = false
			break
		}
		if compatible && score > bestMatch {
			bestMatch = score
			best = s
		}
	}
	sig := best
	e.registry.addCheckDiagnostic(CheckDiagnostic{
		Code:   "no_signature",
		Detail: "no matching signature for " + w.Name + "; assuming best-fit candidate for analysis",
		Word:   w.Name,
		Row:    pos.Row,
		Col:    pos.Col,
	})
	n := len(sig.Args)
	positions := e.checkModeFallbackPositions(n)
	args := make([]Value, len(positions))
	for i, p := range positions {
		args[i] = e.stack[p]
	}
	results := carrierResults(e.registry, w.Name, sig, args, pos)

	// Remove the word and any consumed positions, then splice results
	// in at the word's slot. We rely on ascending order for removal.
	indices := append([]int{e.pointer}, positions...)
	// Insertion sort (small n).
	for i := 1; i < len(indices); i++ {
		for j := i; j > 0 && indices[j] < indices[j-1]; j-- {
			indices[j], indices[j-1] = indices[j-1], indices[j]
		}
	}
	// Deduplicate (defensive).
	uniq := indices[:0]
	prev := -1
	for _, idx := range indices {
		if idx != prev {
			uniq = append(uniq, idx)
			prev = idx
		}
	}
	// Remove from highest to lowest to avoid shifting.
	insertAt := e.pointer
	for i := len(uniq) - 1; i >= 0; i-- {
		if uniq[i] < insertAt {
			insertAt--
		}
		e.stackRemove(uniq[i])
	}
	e.stackSplice(insertAt, 0, results...)
	e.pointer = insertAt
	return nil
}

// RunCarrierBody runs a list body (a Value with VType=TList) through a
// fresh sub-engine in check mode and returns the residual carrier
// stack. Returns nil if the body is not a concrete list. Requires
// that the registry is already in CheckMode (callers set it).
//
// Used by branch-aware words (e.g. `if`) to analyse each branch
// symbolically.
func RunCarrierBody(r *Registry, body Value) []Value {
	stk, _ := runCarrierBodyWithDefs(r, body)
	return stk
}

// runCarrierBodyWithDefs is the branch-aware helper that snapshots
// DefStack depths, runs the body through a sub-engine in check
// mode, and returns both the residual carrier stack and a map of
// every DefStacks[name] -> top-of-stack entry that was added
// during analysis. The top entry is popped (restored to snapshot)
// so the caller can decide whether to re-push, join, or discard.
//
// Only per-name "net additions" are reported. If a branch both
// pushes and pops for the same name, the net change is zero and
// the name is not in the returned map.
func runCarrierBodyWithDefs(r *Registry, body Value) ([]Value, map[string]Value) {
	if body.Data == nil {
		return nil, nil
	}
	elems := body.AsList()
	if elems.IsNil() {
		return nil, nil
	}

	// Snapshot def-stack depths (all known names).
	snapshot := make(map[string]int, len(r.DefStacks))
	for k, v := range r.DefStacks {
		snapshot[k] = len(v)
	}

	tokens := make([]Value, elems.Len())
	copy(tokens, elems.Slice())
	sub := New(r)
	result, err := sub.Run(tokens)
	if err != nil {
		r.addCheckDiagnostic(CheckDiagnostic{
			Code:   "branch_error",
			Detail: "branch analysis error: " + err.Error(),
		})
		result = nil
	}

	// Collect the top of each def stack whose depth grew, then
	// restore depths back to snapshot.
	adds := map[string]Value{}
	for k, ds := range r.DefStacks {
		before := snapshot[k] // zero for names not present before
		if len(ds) > before {
			adds[k] = ds[len(ds)-1]
			r.DefStacks[k] = ds[:before]
			if len(r.DefStacks[k]) == 0 {
				delete(r.DefStacks, k)
			}
		}
	}
	return result, adds
}

// installJoinedDefs merges the `adds` maps from two branches back
// into r.DefStacks. If both branches defined the same name, their
// carriers are joined via JoinCarriers and the joined carrier is
// pushed. If only one branch defined it, that def is pushed back —
// but joined with the pre-branch carrier (if any) since the other
// branch's path kept the original binding.
func installJoinedDefs(r *Registry, then, else_ map[string]Value) {
	seen := make(map[string]bool)
	for k, tv := range then {
		seen[k] = true
		if ev, ok := else_[k]; ok {
			r.DefStacks[k] = append(r.DefStacks[k], JoinCarriers(tv, ev))
			continue
		}
		// then-only: join with the pre-branch top-of-stack if any.
		if ds := r.DefStacks[k]; len(ds) > 0 {
			r.DefStacks[k] = append(ds, JoinCarriers(tv, ds[len(ds)-1]))
		} else {
			r.DefStacks[k] = append(r.DefStacks[k], tv)
		}
	}
	for k, ev := range else_ {
		if seen[k] {
			continue
		}
		// else-only: join with pre-branch top-of-stack.
		if ds := r.DefStacks[k]; len(ds) > 0 {
			r.DefStacks[k] = append(ds, JoinCarriers(ev, ds[len(ds)-1]))
		} else {
			r.DefStacks[k] = append(r.DefStacks[k], ev)
		}
	}
}

// JoinCarrierStacks folds two carrier result stacks (e.g. produced by
// two branches of an `if`) into a single stack. The shorter stack is
// padded out with TNone carriers; per-position join uses JoinCarriers.
func JoinCarrierStacks(a, b []Value) []Value {
	n := len(a)
	if len(b) > n {
		n = len(b)
	}
	out := make([]Value, n)
	for i := 0; i < n; i++ {
		var ai, bi Value
		if i < len(a) {
			ai = a[i]
		} else {
			ai = NewCarrier(TNone)
		}
		if i < len(b) {
			bi = b[i]
		} else {
			bi = NewCarrier(TNone)
		}
		out[i] = JoinCarriers(ai, bi)
	}
	return out
}

// addCheckDiagnostic appends a diagnostic to the registry. Safe to call
// outside of check mode — it simply records the finding. If the
// diagnostic's Severity is empty, the default mapping from its Code
// is applied via SeverityFor.
func (r *Registry) addCheckDiagnostic(d CheckDiagnostic) {
	if d.Severity == "" {
		d.Severity = SeverityFor(d.Code)
	}
	r.CheckDiagnostics = append(r.CheckDiagnostics, d)
}

// GuardClause describes one `x is T` clause detected in a condition.
type GuardClause struct {
	Name string
	Type Type
}

// extractGuardClauses walks a condition list looking for triplets
// `Word(x) Word(is) TypeLiteral(T)` and returns the corresponding
// GuardClause entries. Skips anything that doesn't resolve to a
// bare type literal or an ObjectType. Accepts type-word references
// by looking them up on DefStacks.
func extractGuardClauses(r *Registry, condList Value) []GuardClause {
	if r == nil || condList.Data == nil {
		return nil
	}
	list := condList.AsList()
	if list.IsNil() || list.Len() < 3 {
		return nil
	}
	elems := list.Slice()
	var out []GuardClause
	for i := 0; i+2 < len(elems); i++ {
		if !elems[i].VType.Equal(TWord) || !elems[i+1].VType.Equal(TWord) {
			continue
		}
		wx, err := elems[i].AsWord()
		if err != nil {
			continue
		}
		wis, err := elems[i+1].AsWord()
		if err != nil || wis.Name != "is" {
			continue
		}
		tv := elems[i+2]
		if tv.Data != nil && tv.VType.Equal(TWord) {
			inner, _ := tv.AsWord()
			if ds := r.DefStacks[inner.Name]; len(ds) > 0 {
				tv = ds[len(ds)-1]
			}
		}
		if tv.Data != nil && !tv.IsObjectType() {
			continue
		}
		out = append(out, GuardClause{Name: wx.Name, Type: tv.VType})
	}
	return out
}

// boolWord returns "true" / "false" for use in human-readable
// diagnostic text.
func boolWord(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

// literalCondValue inspects a condition list for a single boolean
// literal (true/false word or Boolean carrier). Returns (value,
// true) when the condition is statically determinable, or (false,
// false) otherwise. Used by `if` analysis to warn about
// unreachable branches.
func literalCondValue(condList Value) (bool, bool) {
	if condList.Data == nil {
		return false, false
	}
	list := condList.AsList()
	if list.IsNil() || list.Len() != 1 {
		return false, false
	}
	only := list.Get(0)
	// Bare true/false word (parser emits these as Word values that
	// resolve to booleans in engine.stepWord; in check mode the
	// words stay as Words until the branch runs).
	if only.VType.Equal(TWord) {
		w, err := only.AsWord()
		if err == nil {
			if w.Name == "true" {
				return true, true
			}
			if w.Name == "false" {
				return false, true
			}
		}
	}
	// Concrete Boolean value with Data set (post-runtime path).
	if only.VType.Matches(TBoolean) && only.Data != nil {
		b, err := only.AsBoolean()
		if err == nil {
			return b, true
		}
	}
	return false, false
}

// applyGuardNarrowing installs then-branch narrowings for each
// `x is T` clause in the condition. Returns a restore func to pop
// the narrowings after the then-branch runs.
func applyGuardNarrowing(r *Registry, condList Value) func() {
	noop := func() {}
	if r == nil || !r.CheckMode {
		return noop
	}
	clauses := extractGuardClauses(r, condList)
	if len(clauses) == 0 {
		return noop
	}
	for _, c := range clauses {
		r.DefStacks[c.Name] = append(r.DefStacks[c.Name], NewCarrier(c.Type))
	}
	return func() {
		for _, c := range clauses {
			ds := r.DefStacks[c.Name]
			if len(ds) > 0 {
				r.DefStacks[c.Name] = ds[:len(ds)-1]
			}
			if len(r.DefStacks[c.Name]) == 0 {
				delete(r.DefStacks, c.Name)
			}
		}
	}
}

// applyComplementNarrowing installs else-branch narrowings — for
// each `x is T` clause it tries to compute the complement of T in
// x's current carrier type and, if non-trivial, pushes the
// complement carrier onto x's DefStack. Currently only refines
// when x's existing binding is a disjunction: the matching
// alternative is subtracted. Returns a restore func.
func applyComplementNarrowing(r *Registry, condList Value) func() {
	noop := func() {}
	if r == nil || !r.CheckMode {
		return noop
	}
	clauses := extractGuardClauses(r, condList)
	if len(clauses) == 0 {
		return noop
	}
	type applied struct{ name string }
	var pushed []applied
	for _, c := range clauses {
		ds := r.DefStacks[c.Name]
		if len(ds) == 0 {
			continue
		}
		cur := ds[len(ds)-1]
		if !cur.IsDisjunct() {
			continue
		}
		di, err := cur.AsDisjunct()
		if err != nil {
			continue
		}
		var remaining []Value
		for _, alt := range di.Alternatives {
			if alt.VType.Equal(c.Type) {
				continue
			}
			remaining = append(remaining, alt)
		}
		if len(remaining) == len(di.Alternatives) || len(remaining) == 0 {
			// No change (alt not found) or all subtracted — skip.
			continue
		}
		var narrowed Value
		if len(remaining) == 1 {
			narrowed = NewCarrier(remaining[0].VType)
		} else {
			narrowed = NewDisjunct(remaining)
			narrowed.Carrier = true
		}
		r.DefStacks[c.Name] = append(r.DefStacks[c.Name], narrowed)
		pushed = append(pushed, applied{name: c.Name})
	}
	if len(pushed) == 0 {
		return noop
	}
	return func() {
		for _, p := range pushed {
			ds := r.DefStacks[p.name]
			if len(ds) > 0 {
				r.DefStacks[p.name] = ds[:len(ds)-1]
			}
			if len(r.DefStacks[p.name]) == 0 {
				delete(r.DefStacks, p.name)
			}
		}
	}
}

// AnalyseFnBody runs a user-defined fn body through a sub-engine in
// check mode, treating named parameters as deffed values bound to
// their arg carriers and unnamed parameters as pre-pushed stack
// values. Results are cached on the registry keyed by (name,
// arg-types) so recursive functions converge instead of looping.
//
// Returns the residual carrier stack. An empty or nil return means
// the analyser aborted (recursion detected or body not available) —
// callers should treat that as an Any carrier.
func AnalyseFnBody(r *Registry, name string, paramNames []string, body []Value, args []Value) []Value {
	if len(body) == 0 {
		return nil
	}
	// Memoisation key: name + arg type paths.
	var sb strings.Builder
	sb.WriteString(name)
	sb.WriteByte('#')
	for _, a := range args {
		sb.WriteString(a.VType.String())
		sb.WriteByte(',')
	}
	key := sb.String()

	if r.CheckFnSummaries == nil {
		r.CheckFnSummaries = map[string][]Value{}
	}
	if r.CheckFnInflight == nil {
		r.CheckFnInflight = map[string]bool{}
	}
	if cached, ok := r.CheckFnSummaries[key]; ok {
		return cached
	}
	if r.CheckFnInflight[key] {
		// Recursion detected — break the cycle with an Any carrier.
		return []Value{NewCarrier(TAny)}
	}
	r.CheckFnInflight[key] = true
	defer delete(r.CheckFnInflight, key)

	// Snapshot def-stack depths so we can unwind any defs the body
	// or parameter binding created.
	snapshot := make(map[string]int, len(r.DefStacks))
	for k, v := range r.DefStacks {
		snapshot[k] = len(v)
	}

	// Bind named parameters as simple defs (carrier-typed). Unnamed
	// parameters flow through the stack — push them before the body.
	var input []Value
	for i, arg := range args {
		if i < len(paramNames) && paramNames[i] != "" {
			r.DefStacks[paramNames[i]] = append(r.DefStacks[paramNames[i]], arg)
		} else {
			input = append(input, arg)
		}
	}
	input = append(input, body...)

	sub := New(r)
	result, err := sub.Run(input)
	if err != nil {
		r.addCheckDiagnostic(CheckDiagnostic{
			Code:   "fn_body_error",
			Detail: "fn body analysis error for " + name + ": " + err.Error(),
			Word:   name,
		})
		result = nil
	}

	// Restore def-stacks to snapshot.
	for k := range r.DefStacks {
		want := snapshot[k]
		if len(r.DefStacks[k]) > want {
			r.DefStacks[k] = r.DefStacks[k][:want]
		}
		if len(r.DefStacks[k]) == 0 {
			delete(r.DefStacks, k)
		}
	}

	r.CheckFnSummaries[key] = result
	return result
}
