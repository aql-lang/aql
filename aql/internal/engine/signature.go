package engine

// Signature describes one way a function can be called.
// Args lists the types the word needs, ordered deepest-first (Args[0] = deepest
// on the stack, Args[last] = top of the stack for prefix matching).
//
// For suffix-precedence words the engine collects future values into Args[0],
// Args[1], ... in order, then pushes them onto the stack and retries as prefix.
type Signature struct {
	Args       []Type
	Precedence int // higher binds tighter; 0 = default (no precedence)
	Handler    func(args []Value) ([]Value, error)

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

// MatchSignature finds the best matching signature for a function given the
// resolved stack and optional word modifiers.
//
// stack is the resolved portion of the stack (index 0 = bottom, last = top).
// modifiers control filtering (forcePrefix, forceSuffix, argCount).
//
// Returns nil if no signature matches.
func MatchSignature(sigs []Signature, stack []Value, modifiers WordInfo) *MatchResult {
	var best *MatchResult
	var bestScore int

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
					!ordered[idx].IsRecordType() && !ordered[idx].IsTypedMap() {
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

		score := signatureScore(sig)

		// Match quality bonus: reward signatures where the actual values
		// match specific (non-Any) type constraints. This prevents TAny
		// from inflating scores when competing with specific types at
		// different hierarchy depths (e.g. TWord vs TString).
		for j := 0; j < n; j++ {
			if sig.Args[j].Equal(TAny) {
				continue
			}
			if ordered[j].VType.Equal(sig.Args[j]) {
				score += 50 // exact type match
			} else {
				score += 10 // prefix (inexact) match
			}
		}

		if best != nil && score <= bestScore {
			continue
		}

		args := make([]Value, n)
		copy(args, ordered)
		best = &MatchResult{Sig: sig, Args: args}
		bestScore = score
	}

	return best
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

// signatureScore computes an intrinsic ranking score for a signature.
// Higher is better: more args and more specific types win.
func signatureScore(sig *Signature) int {
	score := sig.TotalArgs() * 100 // arg count dominates
	for _, t := range sig.Args {
		score += t.Specificity()
	}
	return score
}

// SignatureScore exports signatureScore for testing.
func SignatureScore(sig *Signature) int {
	return signatureScore(sig)
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
