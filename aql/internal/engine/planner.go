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

		// Prefer signatures where the immediate suffix candidate can satisfy
		// any currently-unmatched argument (flexible matching allows
		// reordering, so the peek value need not match the first slot).
		if peekVal != nil && prefixCount < len(sig.Args) {
			bestBonus := 0
			for ai := range sig.Args {
				if usedArgs[ai] {
					continue
				}
				if e.couldProduceType(*peekVal, sig.Args[ai]) {
					bonus := 50
					if sig.Args[ai].Equal(TAny) {
						bonus = 25
					}
					if bonus > bestBonus {
						bestBonus = bonus
					}
				}
			}
			score += bestBonus
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
// stack can be matched to signature arguments, and the arg-slot usage bitmap.
// With flexible matching, prefix values can fill ANY sig slot (not just the
// first N), so a prefix Integer can fill a later Integer slot even if the
// first slot expects Map.
func plannerPrefixCoverage(sigArgs []Type, resolved []Value) (int, []bool) {
	usedArgs := make([]bool, len(sigArgs))
	maxTry := len(sigArgs)
	if maxTry > len(resolved) {
		maxTry = len(resolved)
	}

	prefixCount := 0
	for tryN := maxTry; tryN >= 1; tryN-- {
		top := resolved[len(resolved)-tryN:]
		// Try positional match against first N slots first.
		ok := true
		for i := 0; i < tryN; i++ {
			if !top[i].VType.Matches(sigArgs[i]) {
				ok = false
				break
			}
		}
		if ok {
			prefixCount = tryN
			for i := 0; i < tryN; i++ {
				usedArgs[i] = true
			}
			break
		}
		// Try flexible assignment against ALL sig slots.
		filled := make([]bool, len(sigArgs))
		usedV := make([]bool, tryN)
		if flexibleAssignAny(top, sigArgs, filled, usedV, 0) {
			prefixCount = tryN
			copy(usedArgs, filled)
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
