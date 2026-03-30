package engine

// MaxArgs is the maximum number of arguments a signature may declare.
const MaxArgs = 32

// Signature describes one way a function can be called.
// Args lists the types the word needs, ordered deepest-first (Args[0] = deepest
// on the stack, Args[last] = top of the stack for stack matching).
//
// For forward-precedence words the engine collects future values into Args[0],
// Args[1], ... in order, then pushes them onto the stack and retries as a stack match.
type Signature struct {
	Args    []Type
	Handler func(args []Value) ([]Value, error)

	// FullStackHandler, when non-nil, is called instead of Handler.
	// It receives the matched args AND the full resolved stack before
	// the word (excluding forwards and the matched args themselves).
	// Use this for words like depth, pick, roll that need to inspect
	// or manipulate the entire stack.
	FullStackHandler func(args []Value, stack []Value) ([]Value, error)

	// Patterns holds optional structural patterns for arguments (e.g. map
	// literals in fn signatures). Key is arg index, value is the pattern.
	// When set, the argument must unify with the pattern in addition to
	// matching the type.
	Patterns map[int]Value

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
	Sig  *Signature
	Args []Value // args matched positionally to Sig.Args types
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
		ordered, ok := flexibleMatch(top, sig.Args)
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

// MatchSignatureReversed is like MatchSignature but reads the stack in reverse
// order: the top of the stack maps to sigArgs[0], the next deeper value maps
// to sigArgs[1], etc. This is used for forward-precedence functions when all
// arguments come from the stack (zero forward tokens).
//
// Signatures are assumed to be pre-sorted. The first match wins.
func MatchSignatureReversed(sigs []Signature, stack []Value, modifiers WordInfo) *MatchResult {
	for i := range sigs {
		sig := &sigs[i]

		if modifiers.ArgCount >= 0 && sig.TotalArgs() != modifiers.ArgCount {
			continue
		}

		n := len(sig.Args)
		if len(stack) < n {
			continue
		}

		// Extract top n values from the stack in reversed order.
		reversed := make([]Value, n)
		for j := 0; j < n; j++ {
			reversed[j] = stack[len(stack)-1-j]
		}

		// Try positional match on the reversed values.
		ordered, ok := flexibleMatch(reversed, sig.Args)
		if !ok {
			continue
		}

		// Check structural patterns.
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

// flexibleMatch checks whether values match the given types positionally.
// Arguments are never permuted — values[i] must match types[i].
// Returns the values slice unchanged if matched, or false.
func flexibleMatch(values []Value, types []Type) ([]Value, bool) {
	n := len(types)
	if len(values) < n {
		return nil, false
	}

	if positionalMatch(values, types) {
		return values, true
	}

	return nil, false
}

// positionalMatch checks whether values match types in order.
func positionalMatch(values []Value, types []Type) bool {
	for i, t := range types {
		if !values[i].VType.Matches(t) {
			return false
		}
		// Reject type literals (Data==nil) for concrete Map/List signatures.
		if values[i].Data == nil && (t.Equal(TMap) || t.Equal(TList)) {
			return false
		}
	}
	return true
}

// typeInherentScores maps fully-qualified type paths to an inherent score
// reflecting roughly how many values the type can represent. Within the same
// specificity level, types that match more values score higher and sort
// earlier (tried first). Increments of 100. Unknown types default to 1000.
var typeInherentScores = map[string]int{
	// Depth 1 — Any is a wildcard (matches everything) so it gets the
	// lowest score to ensure it sorts after all concrete root types.
	"Any":    100,
	"None":   100,
	"Scalar": 3000,
	"Node":   3000,
	"Word":   2000,
	"Object": 2000,

	// Depth 2 — Scalar
	"Scalar/String":  2000,
	"Scalar/Number":  1500,
	"Scalar/Boolean": 1000,

	// Depth 2 — Node
	"Node/List":  2000,
	"Node/Map":   2000,
	"Node/Error": 1000,

	// Depth 2 — Word
	"Word/Atom":     1000,
	"Word/Function": 1500,
	"Word/__FW":     500,
	"Word/__OP":     500,
	"Word/__PE":     500,
	"Word/__FN":     500,
	"Word/__UF":     500,
	"Word/__RC":     500,
	"Word/__DJ":     500,
	"Word/__MK":     500,
	"Word/__MV":     500,
	"Word/__MD":     500,
	"Word/__IN":     500,

	// Depth 2 — Object
	"Object/Table":    1500,
	"Object/Record":   1500,
	"Object/Fetch":    1000,
	"Object/Resource": 1000,

	// Depth 3 — Scalar
	"Scalar/Number/Integer": 1100,
	"Scalar/Number/Decimal": 1200,
	"Scalar/String/Proper":  1000,
	"Scalar/String/Empty":   1000,

	// Depth 3 — Node
	"Node/List/Args":    1000,
	"Node/Map/Options":  1000,
	"Node/Map/Word":     1000,
	"Node/Map/Type":     1000,

	// Depth 3 — Object
	"Object/Fetch/Request":   1000,
	"Object/Fetch/Response":  1000,
	"Object/Resource/Entity": 1000,

	// Depth 4
	"Node/Map/Word/Inspect": 1000,
	"Node/Map/Type/Inspect": 1000,
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
