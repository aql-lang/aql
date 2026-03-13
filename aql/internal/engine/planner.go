package engine

// plannerBestSigForForward computes the best signature and prefix arg count for
// setting up suffix collection. It centralizes scoring for:
//   - signature intrinsic rank (arity + specificity)
//   - consumed prefix values
//   - first suffix lookahead compatibility
func (e *Engine) plannerBestSigForForward(fn *Function, w WordInfo, resolved []Value) (*Signature, int) {
	var best *Signature
	var bestScore int
	var bestPrefixCount int

	peekVal := e.peekPlannableSuffixValue()

	for i := range fn.Signatures {
		sig := &fn.Signatures[i]
		if len(sig.Args) == 0 {
			continue
		}
		if w.ArgCount >= 0 && sig.TotalArgs() != w.ArgCount {
			continue
		}

		prefixCount, usedArgs := plannerPrefixCoverage(sig.Args, resolved)
		score := signatureScore(sig)

		// Prefer signatures that can consume already-resolved prefix values.
		score += prefixCount * 25

		// Prefer signatures whose first currently-unmatched argument can be
		// satisfied by the immediate suffix candidate.
		if peekVal != nil && prefixCount < len(sig.Args) {
			firstUnmatched := -1
			for ai := range sig.Args {
				if !usedArgs[ai] {
					firstUnmatched = ai
					break
				}
			}
			if firstUnmatched >= 0 && e.couldProduceType(*peekVal, sig.Args[firstUnmatched]) {
				score += 50
			}
		}

		if best == nil || score > bestScore {
			best = sig
			bestScore = score
			bestPrefixCount = prefixCount
		}
	}

	return best, bestPrefixCount
}

// plannerPrefixCoverage returns how many values from the top of the resolved
// stack can be assigned to distinct signature arguments, and the arg-slot usage
// bitmap for that assignment.
func plannerPrefixCoverage(sigArgs []Type, resolved []Value) (int, []bool) {
	usedArgs := make([]bool, len(sigArgs))
	maxTry := len(sigArgs)
	if maxTry > len(resolved) {
		maxTry = len(resolved)
	}

	prefixCount := 0
	for tryN := maxTry; tryN >= 1; tryN-- {
		top := resolved[len(resolved)-tryN:]
		match, ok := stableSubsetTypeAssignment(top, sigArgs)
		if ok {
			prefixCount = tryN
			for _, argIdx := range match {
				usedArgs[argIdx] = true
			}
			break
		}
	}

	return prefixCount, usedArgs
}

// peekPlannableSuffixValue returns the next non-structural candidate suffix
// token, if one exists.
func (e *Engine) peekPlannableSuffixValue() *Value {
	peekIdx := e.pointer + 1
	if peekIdx >= len(e.stack) {
		return nil
	}
	v := e.stack[peekIdx]
	if v.IsForward() || v.IsOpenParen() {
		return nil
	}
	if v.IsWord() {
		w := v.AsWord()
		if w.Name == "(" || w.Name == ")" || w.Name == "end" {
			return nil
		}
	}
	return &v
}

// stableSubsetTypeAssignment assigns each value to a distinct matching type
// slot. Unlike stableTypeAssignment, values may be fewer than types.
// The returned slice maps value index -> type index.
func stableSubsetTypeAssignment(values []Value, types []Type) ([]int, bool) {
	if len(values) == 0 {
		return []int{}, true
	}
	if len(values) > len(types) {
		return nil, false
	}

	options := make([][]int, len(values))
	for vi := 0; vi < len(values); vi++ {
		if vi < len(types) && values[vi].VType.Matches(types[vi]) {
			options[vi] = append(options[vi], vi)
		}
		for ti := 0; ti < len(types); ti++ {
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

	assignedType := make([]int, len(values))
	for i := range assignedType {
		assignedType[i] = -1
	}
	matchTypeToVal := make([]int, len(types))
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

	for vi := 0; vi < len(values); vi++ {
		seen := make([]bool, len(types))
		if !dfs(vi, seen) {
			return nil, false
		}
	}

	return assignedType, true
}
