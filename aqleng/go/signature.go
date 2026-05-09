package aqleng

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
// For forward-precedence words the engine collects future values into Args[0],
// Args[1], ... in order, then pushes them onto the stack and retries as a stack match.
type Signature struct {
	Args    []Type
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

	// BarrierPos is the arg index where forward collection must stop.
	// Positions before BarrierPos are collected forward; positions from
	// BarrierPos onward are matched from the stack in reverse. 0 means
	// no barrier (default, greedy forward). Implements the | syntax in
	// fn signatures: def f fn [[Integer | String] ...] sets BarrierPos=1.
	BarrierPos int

	// Fallback marks the generic 0-arg handler installed by def as the
	// fallback entry. SortSignatures always places fallbacks last.
	Fallback bool

	// Returns lists the declared return types for this signature. It is
	// used by static type-checking mode: when the engine runs in check
	// mode, it skips the handler and pushes carrier values typed by
	// Returns. When nil or empty, the checker falls back to a
	// conservative approximation (see engine carrier handling).
	Returns []Type

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
				if pattern.VType.Equal(TMap) && ordered[idx].VType.Equal(TMap) &&
					pattern.Data != nil && ordered[idx].Data != nil &&
					!pattern.IsOptionsType() &&
					!ordered[idx].IsRecordType() && !ordered[idx].IsTypedMap() && !ordered[idx].IsOptionsType() {
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

// sigTypeMatches checks whether a value's type matches a signature arg
// type, including metatype awareness: a type literal (Data==nil) whose
// metatype matches a metatype signature arg (e.g. String literal
// matches TScalarType).
//
// **The carrier rule.** Carriers occupy a deliberately ambiguous role
// in the type system: they have a concrete VType (e.g. TInteger) and
// nil Data, identical to a type literal at the field level. But
// semantically they are abstract VALUES, not types. To preserve that
// distinction at sig-match time, sigTypeMatches treats them as
// values:
//
//   - Carrier{Integer} satisfies TInteger (the value-level slot).
//   - Carrier{Integer} does NOT satisfy TScalarType (the metatype slot).
//
// Without the carrier exclusion at the metatype branch, every
// check-mode pass through `is`/`typeof`/`unify` would silently
// upgrade carriers into metatype matches and produce wrong
// dispatch. The Carrier=false guard on the metatype path is the
// only place this distinction is enforced — adding new metatype
// branches must preserve it.
//
// Genuine type literals produced by stepWord on a type-name word
// (e.g. the Word `Integer` resolves to NewTypeLiteral(TInteger))
// always have Carrier=false, so they continue to match metatype
// slots correctly.
//
// See `LANGREF.md` "Type-Registry Internals" → "Carriers" for the
// user-facing description of this rule.
func sigTypeMatches(v Value, t Type) bool {
	if v.VType.Matches(t) {
		return true
	}
	if v.Data == nil && !v.Carrier && IsMetaType(t) {
		return MetatypeFor(v.VType).Matches(t)
	}
	if _, ok := v.Data.(ObjectTypeInfo); ok && IsMetaType(t) {
		return MetatypeFor(v.VType).Matches(t)
	}
	if v.IsRecordType() || v.IsTableType() || v.IsOptionsType() {
		if IsMetaType(t) {
			return MetatypeFor(v.VType).Matches(t)
		}
	}
	// Options values have VType=TMap but should match TOptions signatures.
	if v.IsOptionsType() && t.Equal(TOptions) {
		return true
	}
	return false
}

// rejectsTypeLiteral reports whether a value with Data==nil should be
// rejected at a concrete-payload sig slot — even if sigTypeMatches
// said the VType matches.
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
//   - Metatype slots (Type/*) — `make`, `is`, `typeof`, `convert`,
//     etc. consume a type literal as the type itself.
//
// Carriers (Data==nil but Carrier=true) are abstract VALUES, not
// types — sigTypeMatches deliberately treats them as values, and
// this rejection check follows suit. The value `none` (the unique
// inhabitant of None) is also legitimate at a TNone slot — None
// has a single inhabitant and that's it. This covers both the spec
// runner's NewNone() (Data != nil sentinel) and production aql's
// `NewTypeLiteral(TNone)` (Data == nil, used as the "null" value).
func rejectsTypeLiteral(v Value, expectedType Type) bool {
	if v.Data != nil {
		return false
	}
	if v.Carrier {
		return false
	}
	if expectedType.Equal(TAny) {
		return false
	}
	if IsMetaType(expectedType) {
		return false
	}
	if v.VType.Equal(TNone) {
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
		if sig.QuoteArgs != nil && sig.QuoteArgs[i] && v.VType.Equal(TWord) {
			if !TAtom.Matches(t) {
				return false
			}
			continue
		}
		if !sigTypeMatches(v, t) {
			return false
		}
		// Reject type literals (Data==nil) for concrete Map/List signatures.
		if v.Data == nil && (t.Equal(TMap) || t.Equal(TList)) {
			return false
		}
	}
	return true
}

// typeInherentScores maps fully-qualified type paths to an inherent score
// reflecting roughly how many values the type can represent. Within the same
// specificity level, types that match more values score higher and sort
// earlier (tried first). Every type has a unique score. Unknown types
// default to 1000.
var typeInherentScores = map[string]int{
	// Depth 1 — Any/None/Never are special; concrete roots ordered by breadth.
	// Never (uninhabited) is most specific, then None (unit), then Any (top).
	"Never":  50,
	"None":   100,
	"Any":    200,
	"Type":   300,
	"Object": 400,
	"Word":   500,
	"Scalar": 600,
	"Node":   700,

	// Depth 2 — Word internals (structural tokens, narrow cardinality)
	"Word/__DJ":      100,
	"Word/__FN":      200,
	"Word/__FW":      300,
	"Word/__IN":      400,
	"Word/__IN/__DC": 400,
	"Word/__MK":      500,
	"Word/__MD":      600,
	"Word/__MV":      700,
	"Word/__OP":      800,
	"Word/__PE":      900,
	"Word/__RC":      1000,
	"Word/__UF":      1100,

	// Depth 2 — regular types, ordered by cardinality
	"Scalar/Boolean":  1200,
	"Scalar/Path":     1250,
	"Scalar/Atom":     1300,
	"Object/Error":    1400,
	"Object/Fetch":    1500,
	"Object/Store":    1600,
	"Object/Array":    1650,
	"Object/Resource": 1700,
	"Scalar/Number":   1800,
	"Word/Function":   1900,
	"Object/Table":    2000,
	"Object/Record":   2100,
	"Scalar/String":   2200,
	"Node/List":       2300,
	"Node/Map":        2400,
	"Type/ScalarType": 2500,
	"Type/NodeType":   2600,
	"Type/ObjectType": 2700,

	// Depth 3 — Scalar subtypes
	"Scalar/String/Empty":   900,
	"Scalar/String/Proper":  1000,
	"Scalar/Number/Integer": 1100,
	"Scalar/Number/Decimal": 1200,

	// Depth 3 — Node subtypes
	"Node/List/Args":   1300,
	"Node/Map/Options": 1400,
	"Node/Map/Inspect": 1500,

	// Depth 3 — Object subtypes
	"Object/Fetch/Request":   1600,
	"Object/Fetch/Response":  1700,
	"Object/Resource/Entity": 1800,
	"Object/Store/System":    1900,
}

// typeInherentScore returns the inherent score for a type.
// Defaults to 1000 for types not in the map.
func typeInherentScore(t Type) int {
	path := t.String()
	if s, ok := typeInherentScores[path]; ok {
		return s
	}
	return 1000
}

// signatureScore computes an intrinsic ranking score for a signature.
// Higher is better: more args and more specific types win.
//
// Formula: arity * 1_000_000 + sum(specificity * 10_000 + inherentScore)
//
// Arity dominates (1e6), then specificity (1e4 per arg), then inherent
// type score (up to ~9000) as a tiebreaker within the same specificity.
func signatureScore(sig *Signature) int {
	score := sig.TotalArgs() * 1_000_000
	if sig.BarrierPos > 0 {
		// Piped signatures sort before non-piped. Barriers closer to the
		// start (lower BarrierPos) are more constrained and score higher.
		score += 500_000 + (MaxArgs-sig.BarrierPos)*10_000
	}
	for _, t := range sig.Args {
		score += t.Specificity() * 10_000
		score += typeInherentScore(t)
	}
	// Post §1.1 fix: scalar-literal dispatch lives in Patterns
	// rather than in value-tagged subtype paths. A sig with a
	// concrete-value pattern at position i is MORE SPECIFIC than
	// one without — give each pattern entry the same boost a
	// type-path leaf used to provide. Without this, two sigs with
	// equal arg-types tie on score and registration order picks
	// the winner, defeating literal-value overloads.
	for _, pat := range sig.Patterns {
		if pat.Data != nil {
			score += 10_000
		}
	}
	return score
}

// SignatureScore exports signatureScore for testing.
func SignatureScore(sig *Signature) int {
	return signatureScore(sig)
}

// SortSignatures sorts a slice of signatures in-place by priority:
// longest first, then most specific types. Fallback signatures always
// sort last. Stable sort preserves registration order for equal scores.
func SortSignatures(sigs []Signature) {
	for i := 1; i < len(sigs); i++ {
		for j := i; j > 0; j-- {
			// Fallbacks always sink to the end.
			if sigs[j-1].Fallback && !sigs[j].Fallback {
				sigs[j], sigs[j-1] = sigs[j-1], sigs[j]
				continue
			}
			if sigs[j].Fallback {
				break
			}
			if signatureScore(&sigs[j]) > signatureScore(&sigs[j-1]) {
				sigs[j], sigs[j-1] = sigs[j-1], sigs[j]
			} else {
				break
			}
		}
	}
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

// RankSignatures returns the indices of sigs sorted by priority (best first).
// Longer signatures and narrower (more specific) types rank higher.
func RankSignatures(sigs []Signature) []int {
	indices := make([]int, len(sigs))
	for i := range indices {
		indices[i] = i
	}
	// Stable sort: preserve registration order for equal scores.
	for i := 1; i < len(indices); i++ {
		for j := i; j > 0; j-- {
			si, sj := signatureScore(&sigs[indices[j]]), signatureScore(&sigs[indices[j-1]])
			if si > sj {
				indices[j], indices[j-1] = indices[j-1], indices[j]
			} else {
				break
			}
		}
	}
	return indices
}
