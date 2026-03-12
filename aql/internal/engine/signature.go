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
// Tries positional (identity) first, then permutations for N <= 6.
// Among valid permutations, prefers the one closest to the original order
// (fewest displacements), with positional always winning if it matches.
// Returns the values reordered to match types, or false.
func flexibleMatch(values []Value, types []Type) ([]Value, bool) {
	n := len(types)
	if len(values) < n {
		return nil, false
	}

	// Try positional first — always preferred.
	if positionalMatch(values, types) {
		return values, true
	}

	if match, ok := stableTypeAssignment(values[:n], types); ok {
		result := make([]Value, n)
		for vi, ti := range match {
			result[ti] = values[vi]
		}
		return result, true
	}

	return nil, false
}

// stableTypeAssignment returns a deterministic assignment from values to types.
// The result is a slice where result[valueIdx] = typeIdx.
// Preference order is stable and minimizes movement from the source ordering.
func stableTypeAssignment(values []Value, types []Type) ([]int, bool) {
	n := len(types)
	if len(values) < n {
		return nil, false
	}

	// Build candidate type slots per value, preferring same index first.
	options := make([][]int, n)
	for vi := 0; vi < n; vi++ {
		if values[vi].VType.Matches(types[vi]) {
			options[vi] = append(options[vi], vi)
		}
		for ti := 0; ti < n; ti++ {
			if ti == vi {
				continue
			}
			if values[vi].VType.Matches(types[ti]) {
				options[vi] = append(options[vi], ti)
			}
		}
		if len(options[vi]) == 0 {
			return nil, false
		}
	}

	assignedType := make([]int, n)
	for i := range assignedType {
		assignedType[i] = -1
	}
	matchTypeToVal := make([]int, n)
	for i := range matchTypeToVal {
		matchTypeToVal[i] = -1
	}

	var dfs func(v int, seen []bool) bool
	dfs = func(v int, seen []bool) bool {
		for _, t := range options[v] {
			if seen[t] {
				continue
			}
			seen[t] = true
			if matchTypeToVal[t] == -1 || dfs(matchTypeToVal[t], seen) {
				matchTypeToVal[t] = v
				assignedType[v] = t
				return true
			}
		}
		return false
	}

	for vi := 0; vi < n; vi++ {
		seen := make([]bool, n)
		if !dfs(vi, seen) {
			return nil, false
		}
	}

	return assignedType, true
}

// permMatch finds a permutation of values that matches types.
// Among valid permutations, returns the one with fewest displacements
// from the original order (most values staying in place).
func permMatch(values []Value, types []Type) ([]Value, bool) {
	n := len(values)
	perm := make([]int, n)
	for i := range perm {
		perm[i] = i
	}

	var bestPerm []int
	bestDisplaced := n + 1

	// Generate all permutations via Heap's algorithm.
	var generate func(k int)
	generate = func(k int) {
		if k == 1 {
			// Check this permutation.
			match := true
			for i, t := range types {
				if !values[perm[i]].VType.Matches(t) {
					match = false
					break
				}
			}
			if match {
				displaced := 0
				for i := range perm {
					if perm[i] != i {
						displaced++
					}
				}
				if displaced < bestDisplaced {
					bestDisplaced = displaced
					bestPerm = make([]int, n)
					copy(bestPerm, perm)
				}
			}
			return
		}
		for i := 0; i < k; i++ {
			generate(k - 1)
			if k%2 == 0 {
				perm[i], perm[k-1] = perm[k-1], perm[i]
			} else {
				perm[0], perm[k-1] = perm[k-1], perm[0]
			}
		}
	}

	generate(n)

	if bestPerm == nil {
		return nil, false
	}

	result := make([]Value, n)
	for i, idx := range bestPerm {
		result[i] = values[idx]
	}
	return result, true
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
