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
// Positional matches (values in stack order) are preferred over reordered
// matches at the same arity via a small score penalty for reordering.
//
// Returns nil if no signature matches.
func MatchSignature(sigs []Signature, stack []Value, modifiers WordInfo) *MatchResult {
	var best *MatchResult
	var bestScore int
	var bestPositional bool

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

		// Try positional match first, then flexible.
		var ordered []Value
		isPositional := false
		if positionalMatch(top, sig.Args) {
			ordered = top
			isPositional = true
		} else {
			var ok bool
			ordered, ok = flexibleMatch(top, sig.Args)
			if !ok {
				continue
			}
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
				score += 30 // prefix (inexact) match — high enough to
				// reward specific constraints over TAny even when
				// a competing sig has one exact TMap match.
			}
		}

		// Penalty for reordered (non-positional) matches: positional
		// argument order should be preferred when scores are close.
		if !isPositional {
			score -= 40
		}

		// At equal scores, prefer the first match (earlier registration).
		// Exception: a positional match beats a reordered one.
		if best != nil {
			if score < bestScore {
				continue
			}
			if score == bestScore {
				if bestPositional || !isPositional {
					continue
				}
			}
		}

		args := make([]Value, n)
		copy(args, ordered)
		best = &MatchResult{Sig: sig, Args: args}
		bestScore = score
		bestPositional = isPositional
	}

	return best
}

// flexibleMatch checks whether values match the given types. It first tries
// positional matching (values[i] matches types[i]). If that fails, it tries
// type-based reordering: assigning each value to a type slot it matches using
// backtracking. Same-type slots preserve the original positional order of
// values because candidates are tried in order.
func flexibleMatch(values []Value, types []Type) ([]Value, bool) {
	n := len(types)
	if len(values) < n {
		return nil, false
	}

	if positionalMatch(values, types) {
		return values, true
	}

	// Try type-based reordering.
	result := make([]Value, n)
	used := make([]bool, n)
	if flexibleAssign(values[:n], types, result, used, 0) {
		return result, true
	}

	return nil, false
}

// flexibleAssign tries to assign values to type slots using backtracking.
// For each slot, it tries values in their original order, so same-type
// slots preserve positional ordering. N is small (typically ≤7) so the
// worst-case N! search is bounded at 5040.
func flexibleAssign(values []Value, types []Type, result []Value, used []bool, slot int) bool {
	if slot == len(types) {
		return true
	}
	t := types[slot]
	for i := 0; i < len(values); i++ {
		if used[i] {
			continue
		}
		if !values[i].VType.Matches(t) {
			continue
		}
		// Reject type literals for concrete Map/List signatures.
		if values[i].Data == nil && (t.Equal(TMap) || t.Equal(TList)) {
			continue
		}
		used[i] = true
		result[slot] = values[i]
		if flexibleAssign(values, types, result, used, slot+1) {
			return true
		}
		used[i] = false
	}
	return false
}

// flexibleAssignAny assigns N values to any N slots in the FULL sigArgs array
// (not necessarily the first N). filled[i] tracks which sig slots are used.
// Used by plannerPrefixCoverage to allow prefix values to fill later sig slots.
func flexibleAssignAny(values []Value, sigArgs []Type, filled []bool, usedVals []bool, idx int) bool {
	if idx == len(values) {
		return true
	}
	for i := range sigArgs {
		if filled[i] {
			continue
		}
		if !values[idx].VType.Matches(sigArgs[i]) {
			continue
		}
		if values[idx].Data == nil && (sigArgs[i].Equal(TMap) || sigArgs[i].Equal(TList)) {
			continue
		}
		filled[i] = true
		usedVals[idx] = true
		if flexibleAssignAny(values, sigArgs, filled, usedVals, idx+1) {
			return true
		}
		filled[i] = false
		usedVals[idx] = false
	}
	return false
}

// flexTypeAssign does the same backtracking assignment as flexibleAssign but
// at the type level (no values involved). It determines which sigArgs slots
// can be filled by the given collected types. filled[i]=true means sigArgs[i]
// is assigned. used[j]=true means collected[j] is consumed.
func flexTypeAssign(collected []Type, sigArgs []Type, filled []bool, used []bool, idx int) bool {
	if idx == len(collected) {
		return true
	}
	for i := range sigArgs {
		if filled[i] {
			continue
		}
		if !collected[idx].Matches(sigArgs[i]) {
			continue
		}
		filled[i] = true
		used[idx] = true
		if flexTypeAssign(collected, sigArgs, filled, used, idx+1) {
			return true
		}
		filled[i] = false
		used[idx] = false
	}
	return false
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
