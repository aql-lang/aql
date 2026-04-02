package engine

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
	// When set, a Word value at that position is treated as an Atom for
	// matching purposes and is captured without evaluation during forward
	// collection.
	QuoteArgs map[int]bool

	// NoEvalArgs marks arg positions where list auto-evaluation should be
	// suppressed in execMatch. Unlike QuoteArgs, this does NOT affect
	// forward collection or word→atom conversion — it only prevents
	// autoEvalList from running on consumed list arguments at marked
	// positions. Map auto-evaluation (autoEvalMap) is NOT affected.
	// Use this for code-body positions (def body, if branches, for body,
	// etc.) where the list contains code to execute later, not data to
	// resolve now.
	NoEvalArgs map[int]bool

	// BarrierPos is the arg index where forward collection must stop.
	// Positions before BarrierPos are collected forward; positions from
	// BarrierPos onward are matched from the stack in reverse. 0 means
	// no barrier (default, greedy forward). Implements the | syntax in
	// fn signatures: def f fn [[Integer | String] ...] sets BarrierPos=1.
	BarrierPos int

	// Fallback marks the generic 0-arg handler installed by def as the
	// fallback entry. SortSignatures always places fallbacks last.
	Fallback bool
}

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
		ordered, ok := flexibleMatch(top, sig)
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
					if !openUnifyMap(pattern, ordered[idx]) {
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


// flexibleMatch checks whether values match the given signature positionally.
// Arguments are never permuted — values[i] must match sig.Args[i].
// Returns the values slice unchanged if matched, or false.
func flexibleMatch(values []Value, sig *Signature) ([]Value, bool) {
	n := len(sig.Args)
	if len(values) < n {
		return nil, false
	}

	if positionalMatch(values, sig) {
		return values, true
	}

	return nil, false
}

// sigTypeMatches checks whether a value's type matches a signature arg type,
// including metatype awareness: a type literal (Data==nil) whose metatype
// matches a metatype signature arg (e.g. String literal matches TScalarType).
func sigTypeMatches(v Value, t Type) bool {
	if v.VType.Matches(t) {
		return true
	}
	if v.Data == nil && IsMetaType(t) {
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

// positionalMatch checks whether values match the signature's types in order.
// Handles the /q modifier: a Word value at a QuoteArgs position is treated
// as an Atom for type matching purposes.
func positionalMatch(values []Value, sig *Signature) bool {
	for i, t := range sig.Args {
		v := values[i]
		// /q modifier: treat Word as Atom for matching.
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
	// Depth 1 — Any/None are special; concrete roots ordered by breadth.
	"None":   100,
	"Any":    200,
	"Type":   300,
	"Object": 400,
	"Word":   500,
	"Scalar": 600,
	"Node":   700,

	// Depth 2 — Word internals (structural tokens, narrow cardinality)
	"Word/__DJ": 100,
	"Word/__FN": 200,
	"Word/__FW": 300,
	"Word/__IN": 400,
	"Word/__MK": 500,
	"Word/__MD": 600,
	"Word/__MV": 700,
	"Word/__OP": 800,
	"Word/__PE": 900,
	"Word/__RC": 1000,
	"Word/__UF": 1100,

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
