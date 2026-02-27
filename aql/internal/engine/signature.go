package engine

// Signature describes one way a function can be called.
// Prefix types are consumed from the resolved stack (rightmost = top).
// Suffix types are consumed from future values via the forward mechanism.
type Signature struct {
	Prefix     []Type
	Suffix     []Type
	Precedence int // higher binds tighter; 0 = default (no precedence)
	Handler    func(args []Value) ([]Value, error)
}

// IsPrefixOnly reports whether this signature takes only prefix args.
func (s *Signature) IsPrefixOnly() bool {
	return len(s.Suffix) == 0
}

// TotalArgs returns the total number of arguments (prefix + suffix).
func (s *Signature) TotalArgs() int {
	return len(s.Prefix) + len(s.Suffix)
}

// MatchResult holds a matched signature and how it was matched.
type MatchResult struct {
	Sig       *Signature
	PrefixLen int // number of prefix args consumed from the stack
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

		// Filter by modifiers.
		if modifiers.ForcePrefix && len(sig.Suffix) > 0 {
			continue
		}
		if modifiers.ForceSuffix && sig.IsPrefixOnly() {
			continue
		}
		if modifiers.ArgCount >= 0 && sig.TotalArgs() != modifiers.ArgCount {
			continue
		}

		// Check prefix match against the top of the stack.
		if !prefixMatches(sig.Prefix, stack) {
			// If this signature has suffix args, it can still match even
			// with no prefix args (pure suffix signature).
			if len(sig.Prefix) > 0 {
				continue
			}
		}

		score := signatureScore(sig)

		// Prefer prefix-only on equal score.
		if best != nil && score == bestScore && sig.IsPrefixOnly() && !best.Sig.IsPrefixOnly() {
			// Prefix wins tie.
		} else if best != nil && score <= bestScore {
			continue
		}

		best = &MatchResult{Sig: sig, PrefixLen: len(sig.Prefix)}
		bestScore = score
	}

	return best
}

// prefixMatches checks whether the top of the stack satisfies the prefix types.
// Prefix[0] is the deepest arg, Prefix[last] is the top of the stack.
func prefixMatches(prefix []Type, stack []Value) bool {
	if len(prefix) == 0 {
		return true
	}
	if len(stack) < len(prefix) {
		return false
	}
	// The top of the stack is stack[len(stack)-1].
	// Prefix[len-1] should match stack top, Prefix[0] matches deeper.
	base := len(stack) - len(prefix)
	for i, pt := range prefix {
		if !stack[base+i].VType.Matches(pt) {
			return false
		}
	}
	return true
}

// signatureScore computes a ranking score for tie-breaking.
// Higher is better: more args and more specific types win.
func signatureScore(sig *Signature) int {
	score := sig.TotalArgs() * 100 // arg count dominates
	for _, t := range sig.Prefix {
		score += t.Specificity()
	}
	for _, t := range sig.Suffix {
		score += t.Specificity()
	}
	return score
}
