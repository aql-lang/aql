package eng

import "sort"

// MaxArgs is the maximum number of arguments a signature may declare.
const MaxArgs = 32

// Handler is the unified function handler type for all AQL words.
// It receives the matched arguments, the current context map, the
// resolved stack (only populated for FullStack signatures), and the
// registry.
type Handler func(args []Value, ctx map[string]Value, stack []Value, r *Registry) ([]Value, error)

// Signature describes one way a function can be called. Params lists the
// per-position descriptors (name + type + pattern + optional), ordered
// top-first (Params[0] = top of stack for stack matching).
//
// Params[0..BarrierPos-1] are forward-eligible — the engine collects them
// from the tokens following the word, then dispatches once all are
// present. Params[BarrierPos..N-1] are matched from the stack in reverse.
type Signature = FnSig

// CheckFullStackFunc produces the full base..pointer replacement
// for a FullStack signature in check mode. args are the matched
// carrier args in signature order; stack is the preserved carrier
// stack segment below the args; r is the registry the analysis is
// running against (for emitting diagnostics, reading defs, etc.).
type CheckFullStackFunc func(args []Value, stack []Value, r *Registry) []Value

// ReturnsFunc computes the carrier return values for a signature in
// static type-check mode. args are the carrier-typed input values in
// signature order; r is the registry (for emitting diagnostics,
// reading defs, running sub-analyses, etc.) — the same one passed to
// the runtime Handler.
type ReturnsFunc func(args []Value, r *Registry) []Value

// TotalArgs returns the number of arguments. Backed by Params (the
// unified per-position descriptor. Falls back to len(Args) for a
// Signature built positionally via the Args constructor-convenience
// field that hasn't been normalized into Params yet.
func (s *Signature) TotalArgs() int {
	if len(s.Params) == 0 {
		return len(s.Args)
	}
	return len(s.Params)
}

// ArgTypes returns the per-position declared types of the signature,
// derived from Params (the unified source of arg shape). This is the
// EXPORTED accessor external consumers (the lang help/inspect surface)
// should use instead of the legacy Args field, so that field can later
// be retired without a public-API break. Order is sig order (position 0
// = top of stack).
func (s *Signature) ArgTypes() []*Type {
	if len(s.Params) == 0 && len(s.Args) > 0 {
		return s.Args
	}
	out := make([]*Type, len(s.Params))
	for i := range s.Params {
		out[i] = s.Params[i].Type
	}
	return out
}

// normalizeSig makes Params authoritative for a Signature that may have
// been built via the positional Args/Patterns constructor-convenience
// fields. If Params is empty but Args is set, it derives Params from
// Args+Patterns. It then refreshes the Args/Patterns mirrors from Params
// so introspection that reads either view stays consistent. Idempotent.
func normalizeSig(s *Signature) {
	if len(s.Params) == 0 && len(s.Args) > 0 {
		s.Params = make([]FnParam, len(s.Args))
		for i, t := range s.Args {
			s.Params[i] = FnParam{Type: t}
			if s.Patterns != nil {
				if pat, ok := s.Patterns[i]; ok {
					p := pat
					s.Params[i].Pattern = &p
				}
			}
		}
	}
	// Refresh the exported mirrors from Params (the source of truth).
	s.Args = make([]*Type, len(s.Params))
	var patterns map[int]Value
	for i, p := range s.Params {
		s.Args[i] = p.Type
		if p.Pattern != nil {
			if patterns == nil {
				patterns = make(map[int]Value)
			}
			patterns[i] = *p.Pattern
		}
	}
	s.Patterns = patterns
}

// sigArgType returns the declared type at signature position i. It is the
// single accessor every matcher / forward-planner reader uses for a
// signature's per-position type. Storage lives in Params[i].Type; the
// legacy Args slice is still populated at construction sites and used as
// a fallback for legacy/external callers that built a Signature with only
// Args set. Callers must ensure 0 <= i < TotalArgs().
func sigArgType(s *Signature, i int) *Type {
	if len(s.Params) == 0 && len(s.Args) > 0 {
		return s.Args[i]
	}
	return s.Params[i].Type
}

// sigPattern returns the optional structural pattern at signature
// position i, mirroring sigArgType: storage lives in Params[i].Pattern,
// with a fallback to the legacy Patterns map for callers that built a
// Signature with only Args+Patterns set. ok is false when no pattern is
// declared at i.
func sigPattern(s *Signature, i int) (Value, bool) {
	if i < len(s.Params) && s.Params[i].Pattern != nil {
		return *s.Params[i].Pattern, true
	}
	// Constructor-convenience fallback: a Signature built with only the
	// positional Args/Patterns fields (tests, plugins) hasn't been
	// normalized into Params yet.
	if len(s.Params) == 0 && s.Patterns != nil {
		p, ok := s.Patterns[i]
		return p, ok
	}
	return Value{}, false
}

// MatchResult holds a matched signature and the positionally matched args.
type MatchResult struct {
	Sig       *Signature
	Args      []Value // args in signature order
	Positions []int   // absolute stack indices of each arg (nil for 0-arg)
	Name      string  // word name being dispatched (for tracing/recording)
}

// MatchSignature finds the first matching signature for a function given the
// resolved stack and optional word modifiers.
//
// Signatures are assumed to be pre-sorted by SortSignatures (longest and most
// specific first, fallbacks last). The first match wins.
//
// stack is the resolved portion of the stack (index 0 = bottom, last = top).
// modifiers control filtering (forceStack, forceForward, argCount).
//
// Returns nil if no signature matches.
func MatchSignature(sigs []Signature, stack []Value, modifiers WordInfo) *MatchResult {
	for i := range sigs {
		sig := &sigs[i]

		if modifiers.ArgCount >= 0 && sig.TotalArgs() != modifiers.ArgCount {
			continue
		}

		n := sig.TotalArgs()
		if len(stack) < n {
			continue
		}

		// Extract top n values from the stack.
		base := len(stack) - n
		top := stack[base:]

		// Try flexible match.
		ordered, ok := FlexibleMatch(top, sig)
		if !ok {
			continue
		}

		// Check structural patterns (e.g. map literals in fn signatures).
		// Maps use open (subset) matching: the pattern's key-value pairs
		// must be present in the argument, but extra keys are allowed.
		patternOk := true
		for idx := 0; idx < sig.TotalArgs(); idx++ {
			pattern, pok := sigPattern(sig, idx)
			if !pok {
				continue
			}
			if pattern.Parent.Equal(TMap) && ordered[idx].Parent.Equal(TMap) &&
				pattern.Data != nil && ordered[idx].Data != nil &&
				!IsOptionsType(pattern) && !IsTypedMap(pattern) &&
				!IsRecordType(ordered[idx]) && !IsTypedMap(ordered[idx]) && !IsOptionsType(ordered[idx]) {
				if !OpenUnifyMap(pattern, ordered[idx]) {
					patternOk = false
					break
				}
			} else {
				if _, uOk := Unify(ordered[idx], pattern); !uOk {
					patternOk = false
					break
				}
			}
		}
		if !patternOk {
			continue
		}

		args := make([]Value, n)
		copy(args, ordered)
		return &MatchResult{Sig: sig, Args: args}
	}

	return nil
}

// FlexibleMatch checks whether values match the given signature positionally.
// Arguments are never permuted — values[i] must match Params[i].
// Returns the values slice unchanged if matched, or false.
func FlexibleMatch(values []Value, sig *Signature) ([]Value, bool) {
	n := sig.TotalArgs()
	if len(values) < n {
		return nil, false
	}

	if positionalMatch(values, sig) {
		return values, true
	}

	return nil, false
}

// sigTypeMatches checks whether a value's type matches a signature
// arg type for an ordinary (non-TypeArgs) slot. Routes the primary
// subtype check through Behavior so per-type custom Match
// implementations participate in signature matching.
//
// A type-literal expectation lives on the sig as TypeArgs[i]=true
// (see sigTypeMatchesAsType); this function is the value-side path.
//
// **The carrier rule.** Carriers have a concrete Parent (e.g.
// TInteger) and nil Data, identical to a type literal at the field
// level — but semantically they are abstract VALUES, not types. They
// satisfy ordinary value slots (Carrier{Integer} matches TInteger)
// and are rejected at TypeArgs slots by sigTypeMatchesAsType.
func sigTypeMatches(v Value, t *Type) bool {
	// Gradual (dynamic) carrier: matches the slot unless its bound is
	// PROVABLY disjoint from t — the not-disjoint rule, the optimistic
	// dual of strict ConformsTo (design/dynamic-modality-report.0.md).
	// Reuses `tand` for the disjointness proof; dynamic(Any) matches
	// every inhabited slot, dynamic(Integer) fails only provably-disjoint
	// slots (String, Atom, …). Checked first so a dynamic carrier never
	// falls into the strict path below. The flag is cleared on the
	// operand copy so the bound flows through `tand` as an ordinary
	// carrier.
	if v.Dynamic {
		bound := v
		bound.Dynamic = false
		return !isNeverShape(TandValues(bound, NewCarrier(t)))
	}
	if v.Is(t) {
		return true
	}
	// Options is structurally a keyword-args map. A parameter typed
	// `Options` (a bare `opts:Options` annotation → TOptions slot)
	// accepts both an Options-tagged value AND a plain concrete map —
	// the latter is how callers actually pass options (`f {a:1}`).
	// Without this, every make-style fn is forced to declare `Map`
	// instead of the more descriptive `Options`. A bare `Map` type
	// literal (Data==nil) is excluded; only a concrete map matches.
	if t.Equal(TOptions) {
		if IsOptionsType(v) {
			return true
		}
		if v.Parent.Equal(TMap) && IsConcrete(v) {
			return true
		}
	}
	return false
}

// sigTypeMatchesAsType is the TypeArgs-slot match: v must be a type
// literal (or a structural type body) whose denoted lattice node
// matches t. Used for sig positions like the second arg of
// `Integer gte 10` — the "Integer" type literal — or the first arg
// of `make Foo {...}` — the Foo type body.
//
// A type literal is a by-value copy of its lattice node (Data==nil,
// Parent set to the supertype); the denoted node is &v. A structural
// type body (RecordType, OptionsType, TableType, ObjectType,
// ChildType) carries non-nil Data but its Parent is the family root
// (TMap, TList, TObject) — we match against the Parent for those.
// Carriers (Data==nil but Carrier=true) are values, not types, and
// are rejected here.
func sigTypeMatchesAsType(v Value, t *Type) bool {
	if v.Carrier {
		return false
	}
	if IsBareTypeNode(v) {
		// Bare None has Parent=TNone; treat it as not-a-type for type
		// args. Lattice roots have Parent=nil but are still valid type
		// literals — &v is the lattice node either way.
		if v.Parent != nil && v.Parent.Equal(TNone) && v.Name == "" {
			return false
		}
		return (&v).ConformsTo(t)
	}
	// DepScalar bodies are NOT accepted at TypeArgs slots: they're
	// constraints over a base scalar (used as runtime values), not
	// bare scalar type literals — the dep-sig fallthrough would
	// otherwise loop back on itself for `(Integer gt 10) lt
	// (Integer gt 20)`.
	if v.IsDepScalar() {
		return false
	}
	// Other structural type bodies (Record, Options, Table, Object,
	// ChildType, Disjunct, Enum, Function/FnUndef, ImplicitMap
	// record shape) are "types" — accept them when their lattice
	// family matches the slot.
	if IsTypeBody(v) {
		return v.Parent.ConformsTo(t)
	}
	return false
}

// sigArgMatches dispatches a positional sig match to either the
// ordinary value matcher or the TypeArgs (type-literal) matcher
// based on sig.TypeArgs[idx]. Use this at every call site that has
// a *Signature in hand; bare sigTypeMatches stays for the
// no-sig-context paths (carrier promotion, predicate sandbox).
func sigArgMatches(sig *Signature, idx int, v Value) bool {
	if sig.TypeArgs != nil && sig.TypeArgs[idx] {
		return sigTypeMatchesAsType(v, sigArgType(sig, idx))
	}
	return sigTypeMatches(v, sigArgType(sig, idx))
}

// rejectsTypeLiteral reports whether a value with Data==nil should be
// rejected at a concrete-payload sig slot — even if sigTypeMatches
// said the Parent matches.
//
// A type literal (e.g. `Integer` resolved from a bare type-name word)
// has Data==nil, so handlers that read its payload via AsX() would
// silently pull the zero value. That used to make programs like
// `addq Integer 1` quietly compute `addq 0 1` instead of raising. Now
// the matcher rejects type literals at every concrete-payload slot
// and dispatch falls through to a TAny overload (or signature_error
// if none exists).
//
// Type literals are still legitimately accepted at:
//
//   - TAny slots — universal catch-all; the handler is expected to
//     handle both concrete payloads and type literals.
//   - TypeArgs slots — the sig-level "I want a type literal here"
//     marker (the successor to the historical metatype slots).
//     rejectsTypeLiteral has no sig in hand; callers wrap the
//     check with a `!sig.TypeArgs[i]` guard.
//
// Carriers (Data==nil but Carrier=true) are abstract VALUES, not
// types — sigTypeMatches deliberately treats them as values, and
// this rejection check follows suit. The value `none` is also
// legitimate at a TNone slot — None has a single inhabitant and
// that's it. This covers the spec runner's NewNone() (Data != nil
// sentinel value with Parent=TNone) AND production aql's
// `NewTypeLiteral(TNone)` (Data == nil, value IS the TNone lattice
// node — its own Parent is nil since None is a degenerate root).
func rejectsTypeLiteral(v Value, expectedType *Type) bool {
	if v.Data != nil {
		return false
	}
	if v.Carrier {
		return false
	}
	if expectedType.Equal(TAny) {
		return false
	}
	if expectedType.Equal(TNone) {
		// At a TNone slot, the None type literal is the canonical
		// inhabitant; sigTypeMatches has already verified the value
		// is None-typed.
		return false
	}
	return true
}

// positionalMatch checks whether values match the signature's types in order.
// Handles the /q modifier: a Word value at a QuoteArgs position is treated
// as an Atom for type matching purposes.
//
// /q is a forward-only language rule (see Signature.QuoteArgs doc). The
// Word→Atom branch below is reachable only through the forward-collection
// path, where a raw Word can land at the sig position. For stack-only
// matching the value is never a Word (stepWord has already resolved it),
// so the branch falls through to the regular sigTypeMatches check.
func positionalMatch(values []Value, sig *Signature) bool {
	for i := 0; i < sig.TotalArgs(); i++ {
		t := sigArgType(sig, i)
		v := values[i]
		// /q modifier (forward-only): treat Word as Atom for matching.
		if sig.QuoteArgs != nil && sig.QuoteArgs[i] && v.Parent.Equal(TWord) {
			if !TAtom.ConformsTo(t) {
				return false
			}
			continue
		}
		if !sigArgMatches(sig, i, v) {
			return false
		}
		// Reject type literals (Data==nil) for concrete Map/List signatures
		// unless this slot explicitly wants a type literal.
		isTypeArg := sig.TypeArgs != nil && sig.TypeArgs[i]
		if !isTypeArg && IsBareTypeNode(v) && (t.Equal(TMap) || t.Equal(TList)) {
			return false
		}
	}
	return true
}

// sigSlotValue returns the expectation value the sig declares at
// position i: the structural Pattern if one is set, otherwise a bare
// type literal of the declared arg type. The result feeds
// CompareValues so the unified type/value lattice settles per-position
// ordering — a concrete Pattern (Data != nil) sorts strictly above the
// bare type literal of the same family via litVsConcreteOrder, and two
// bare type literals fall through to compareTypes (Rank → depth →
// name → ID).
func sigSlotValue(sig *Signature, i int) Value {
	if p, ok := sigPattern(sig, i); ok {
		return p
	}
	return NewTypeLiteral(sigArgType(sig, i))
}

// CompareSignatures imposes a total order on Signatures using the
// unified type/value lattice, in REVERSE — the more specific sig
// sorts first so MatchSignature's first-match-wins loop picks the
// tightest overload available.
//
// Each sig's args are treated as a List of expectation values
// (per sigSlotValue). Comparison follows the list-comparison contract
// from CompareValues: list size first (longer lists sort below shorter
// in natural order; reversed here, so longer arity wins), then
// element-wise. At each position the reversed CompareValues result
// places the more specific value (concrete pattern, deeper type, …)
// first. BarrierPos breaks the final tie: a sig with a stack barrier
// (non-zero BarrierPos) sorts before an otherwise identical sig
// without one, since the barrier is an additional dispatch constraint.
//
// Fallback sigs need no special-case: a fallback is always 0-arg, so
// the arity-first rule already sinks it to the end.
func CompareSignatures(a, b *Signature) int {
	if c := cmpInt(b.TotalArgs(), a.TotalArgs()); c != 0 {
		return c
	}
	for i := 0; i < a.TotalArgs(); i++ {
		av := sigSlotValue(a, i)
		bv := sigSlotValue(b, i)
		c, err := CompareValues(av, bv)
		if err != nil || c == 0 {
			continue
		}
		return -c
	}
	// "Has a piped barrier" means an intermediate position — neither
	// all-stack (0) nor all-forward (len(Args)). The two extremes are
	// the default shapes and don't represent an additional dispatch
	// constraint worth sorting on.
	aBarrier := a.BarrierPos > 0 && a.BarrierPos < a.TotalArgs()
	bBarrier := b.BarrierPos > 0 && b.BarrierPos < b.TotalArgs()
	if aBarrier && !bBarrier {
		return -1
	}
	if !aBarrier && bBarrier {
		return 1
	}
	return 0
}

// SortSignatures sorts a slice of signatures in-place by reversed
// lattice order (see CompareSignatures): longer arity first, then per
// position the more specific type/pattern first. Stable: sigs that
// compare equal preserve registration order.
func SortSignatures(sigs []Signature) {
	sort.SliceStable(sigs, func(i, j int) bool {
		return CompareSignatures(&sigs[i], &sigs[j]) < 0
	})
}

// RankSignatures returns the indices of sigs sorted by priority (best
// first), using the same reversed-lattice order as SortSignatures.
func RankSignatures(sigs []Signature) []int {
	indices := make([]int, len(sigs))
	for i := range indices {
		indices[i] = i
	}
	sort.SliceStable(indices, func(i, j int) bool {
		return CompareSignatures(&sigs[indices[i]], &sigs[indices[j]]) < 0
	})
	return indices
}
