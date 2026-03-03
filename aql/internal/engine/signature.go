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
}

// TotalArgs returns the number of arguments.
func (s *Signature) TotalArgs() int {
	return len(s.Args)
}

// MatchResult holds a matched signature and the reordered args.
type MatchResult struct {
	Sig  *Signature
	Args []Value // args reordered to match Sig.Args types
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

		score := signatureScore(sig)

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

// flexibleMatch checks whether values can be reordered to match the given types.
// - Try positional (identity) first.
// - For 2 args with distinct types, try swap.
// Returns the values reordered to match types, or false.
func flexibleMatch(values []Value, types []Type) ([]Value, bool) {
	n := len(types)
	if len(values) < n {
		return nil, false
	}

	// Try positional first.
	if positionalMatch(values, types) {
		return values, true
	}

	// For 2 args, try swap.
	if n == 2 {
		swapped := []Value{values[1], values[0]}
		if positionalMatch(swapped, types) {
			return swapped, true
		}
	}

	return nil, false
}

// positionalMatch checks whether values match types in order.
func positionalMatch(values []Value, types []Type) bool {
	for i, t := range types {
		if !values[i].VType.Matches(t) {
			return false
		}
	}
	return true
}

// signatureScore computes a ranking score for tie-breaking.
// Higher is better: more args and more specific types win.
func signatureScore(sig *Signature) int {
	score := sig.TotalArgs() * 100 // arg count dominates
	for _, t := range sig.Args {
		score += t.Specificity()
	}
	return score
}
