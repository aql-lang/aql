package eng

import "sort"

// MaxArgs is the maximum number of arguments a signature may declare.
const MaxArgs = 32

// Handler is the unified function handler type for all AQL words.
// It receives the matched arguments, the current context map, the
// resolved stack (only populated for FullStack signatures), and the
// registry.
type Handler func(args []Value, ctx map[string]Value, stack []Value, r *Registry) ([]Value, error)

// Signature describes one way a function can be called.
// Args lists the types the word needs, ordered deepest-first (Args[0] = deepest
// on the stack, Args[last] = top of the stack for stack matching).
//
// Args[0..BarrierPos-1] are forward-eligible — the engine collects them
// from the tokens following the word, then dispatches once all are
// present. Args[BarrierPos..N-1] are matched from the stack in reverse.
type Signature struct {
	Args    []*Type
	Handler Handler

	// FullStack, when true, causes the engine to pass the full resolved
	// stack (excluding matched args) and to splice the results as a
	// complete replacement for base..pointer. Use this for words like
	// depth, pick, roll that need to inspect or manipulate the entire stack.
	FullStack bool

	// Patterns holds optional structural patterns for arguments (e.g. map
	// literals in fn signatures). Key is arg index, value is the pattern.
	// When set, the argument must unify with the pattern in addition to
	// matching the type.
	Patterns map[int]Value

	// QuoteArgs marks arg positions with the /q modifier ("implicit quote").
	// /q is a FORWARD-ONLY language rule: it intervenes during forward arg
	// collection so that an upcoming Word is captured as an Atom rather than
	// being executed by stepWord. This is what makes `def name body`,
	// `set foo 42 store`, `get a {a:1}`, etc. work without an explicit `quote`.
	//
	// Outside a /q slot, an undefined word at the pointer is an error
	// (see stepWord). To pass a name as data without /q, the caller must
	// quote it explicitly: `quote foo`, `(quote foo)`, or a literal atom.
	//
	// /q has no effect on stack matching: by the time a value reaches the
	// resolved stack it is no longer a Word — stepWord has either invoked a
	// registered word, resolved a defined name, or (under CheckMode only)
	// converted an undefined Word to an `Undefined=true` Atom. The only
	// way to put a name on the stack as a value is `quote name`, which
	// produces an Atom; that Atom matches an [Atom/q, X] sig via the
	// normal sigTypeMatches fall-through. So a single [Atom/q, X] sig
	// covers BOTH the forward Word case and the explicit-Atom case —
	// there is no need to declare a separate non-/q Atom sig.
	QuoteArgs map[int]bool

	// NoEvalArgs marks arg positions where list auto-evaluation should be
	// suppressed in execMatch. Unlike QuoteArgs, this does NOT affect
	// forward collection or word→atom conversion — it only prevents
	// autoEvalList from running on consumed list arguments at marked
	// positions. Map auto-evaluation (autoEvalMap) is NOT affected;
	// for that use NoEvalMapArgs.
	// Use this for code-body positions (def body, if branches, for body,
	// etc.) where the list contains code to execute later, not data to
	// resolve now.
	NoEvalArgs map[int]bool

	// NoEvalMapArgs marks arg positions where map auto-evaluation
	// (autoEvalMap) should be suppressed. Used by def's typed-name
	// signature so a Word at the type position arrives raw — without
	// this, a fn-as-type name (registered as a callable AND stored as
	// a type value) would be called by the auto-eval pipeline before
	// the handler could resolve it as a type.
	NoEvalMapArgs map[int]bool

	// TypeArgs marks arg positions that must receive a *type literal*
	// (or a structural type body) rather than a concrete value. The
	// slot's declared type in Args[i] is the upper bound of the
	// permitted lattice node — Args[i]=TScalar with TypeArgs[i]=true
	// accepts any scalar type literal (`Integer`, `String`, `Boolean`,
	// …); Args[i]=TIdeal with TypeArgs[i]=true accepts any ideal type
	// body (Record, Options, Table, Object, …).
	//
	// This replaced the historical metatype nodes (Type/ScalarType,
	// Type/NodeType, Type/IdealType) — the lattice no longer carries a
	// "type of types" parallel hierarchy; "this slot wants a type" is
	// now a sig-level concern.
	TypeArgs map[int]bool

	// BarrierPos is the arg index where forward collection must stop.
	// Positions before BarrierPos are collected forward; positions from
	// BarrierPos onward are matched from the stack in reverse. 0 means
	// no barrier (default, greedy forward). Implements the | syntax in
	// fn signatures: def f fn [[Integer | String] ...] sets BarrierPos=1.
	BarrierPos int

	// Fallback marks the generic 0-arg handler installed by def as the
	// fallback entry. Fallback sigs have zero args so the arity-first
	// rule in SortSignatures already sinks them to the end.
	Fallback bool

	// Returns lists the declared return types for this signature. It is
	// used by static type-checking mode: when the engine runs in check
	// mode, it skips the handler and pushes carrier values typed by
	// Returns. When nil or empty, the checker falls back to a
	// conservative approximation (see engine carrier handling).
	Returns []*Type

	// ReturnsFn, when non-nil, overrides Returns for static
	// type-checking: the checker calls it with the carrier-typed args
	// and uses the resulting slice as the return carriers. This is
	// required for signatures whose return type depends on the input
	// types (e.g. Integer+Integer → Integer, otherwise Decimal) or on
	// the input values themselves (e.g. dup, swap propagate their
	// inputs). When both Returns and ReturnsFn are set, ReturnsFn
	// wins.
	ReturnsFn ReturnsFunc

	// RunInCheckMode, when true, causes the engine to execute this
	// signature's Handler even when Registry.Check.Mode is on. Use it
	// for words with registry-level side effects that later words
	// rely on (def, undef, fn, type, import, export, module). The
	// handler still runs against carrier args, so it must tolerate
	// Data==nil / Carrier=true values at its input positions.
	RunInCheckMode bool

	// CheckFullStackFn, when non-nil, replaces both Returns and
	// ReturnsFn for FullStack signatures in check mode. It is
	// passed the matched args and the full resolved carrier stack
	// (from the nearest paren/root barrier through to the pointer
	// exclusive of args). The returned slice is the complete
	// replacement for that base..pointer range — mirroring the
	// runtime FullStack handler's semantics.
	CheckFullStackFn CheckFullStackFunc
}

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

// TotalArgs returns the number of arguments.
func (s *Signature) TotalArgs() int {
	return len(s.Args)
}

// MatchResult holds a matched signature and the positionally matched args.
type MatchResult struct {
	Sig       *Signature
	Args      []Value // args in signature order
	Positions []int   // absolute stack indices of each arg (nil for 0-arg)
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

		n := len(sig.Args)
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
		if sig.Patterns != nil {
			patternOk := true
			for idx, pattern := range sig.Patterns {
				if pattern.Parent.Equal(TMap) && ordered[idx].Parent.Equal(TMap) &&
					pattern.Data != nil && ordered[idx].Data != nil &&
					!IsOptionsType(pattern) &&
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
		}

		args := make([]Value, n)
		copy(args, ordered)
		return &MatchResult{Sig: sig, Args: args}
	}

	return nil
}

// FlexibleMatch checks whether values match the given signature positionally.
// Arguments are never permuted — values[i] must match sig.Args[i].
// Returns the values slice unchanged if matched, or false.
func FlexibleMatch(values []Value, sig *Signature) ([]Value, bool) {
	n := len(sig.Args)
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
	if v.Is(t) {
		return true
	}
	// Options values have Parent=TMap but should match TOptions signatures.
	if IsOptionsType(v) && t.Equal(TOptions) {
		return true
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
	if v.Data == nil {
		// Bare None has Parent=TNone; treat it as not-a-type for type
		// args. Lattice roots have Parent=nil but are still valid type
		// literals — &v is the lattice node either way.
		if v.Parent != nil && v.Parent.Equal(TNone) && v.Name == "" {
			return false
		}
		return (&v).Matches(t)
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
		return v.Parent.Matches(t)
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
		return sigTypeMatchesAsType(v, sig.Args[idx])
	}
	return sigTypeMatches(v, sig.Args[idx])
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
	for i, t := range sig.Args {
		v := values[i]
		// /q modifier (forward-only): treat Word as Atom for matching.
		if sig.QuoteArgs != nil && sig.QuoteArgs[i] && v.Parent.Equal(TWord) {
			if !TAtom.Matches(t) {
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
		if !isTypeArg && v.Data == nil && (t.Equal(TMap) || t.Equal(TList)) {
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
	if sig.Patterns != nil {
		if p, ok := sig.Patterns[i]; ok {
			return p
		}
	}
	return NewTypeLiteral(sig.Args[i])
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

// KeepFallback returns a slice containing only the fallback signature.
// If no fallback is found, returns nil.
func KeepFallback(sigs []Signature) []Signature {
	for i := range sigs {
		if sigs[i].Fallback {
			return []Signature{sigs[i]}
		}
	}
	return nil
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
