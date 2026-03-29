package engine

// plannerBestSigForForward computes the best signature and prefix arg count for
// setting up forward collection. It centralizes scoring for:
//   - signature intrinsic rank (arity + specificity)
//   - consumed prefix values
//   - first forward lookahead compatibility
func (e *Engine) plannerBestSigForForward(fn *Function, w WordInfo, resolved []Value) (*Signature, int) {
	var best *Signature
	var bestScore int
	var bestPrefixCount int

	peekVal := e.peekPlannableForwardValue()

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
		// satisfied by the immediate forward candidate.
		if peekVal != nil && prefixCount < len(sig.Args) {
			firstUnmatched := -1
			for ai := range sig.Args {
				if !usedArgs[ai] {
					firstUnmatched = ai
					break
				}
			}
			if firstUnmatched >= 0 && e.couldProduceType(*peekVal, sig.Args[firstUnmatched]) {
				// Specific type matches get a stronger bonus than catch-all
				// TAny matches. This ensures e.g. [TWord, TAny] is preferred
				// over [TAny, TString] when the peek value is a word.
				if sig.Args[firstUnmatched].Equal(TAny) {
					score += 25
				} else {
					score += 50
				}
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
// stack can be positionally matched to the first N signature arguments, and
// the arg-slot usage bitmap for that assignment. Arguments are never permuted.
func plannerPrefixCoverage(sigArgs []Type, resolved []Value) (int, []bool) {
	usedArgs := make([]bool, len(sigArgs))
	maxTry := len(sigArgs)
	if maxTry > len(resolved) {
		maxTry = len(resolved)
	}

	prefixCount := 0
	for tryN := maxTry; tryN >= 1; tryN-- {
		top := resolved[len(resolved)-tryN:]
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
	}

	return prefixCount, usedArgs
}

// peekPlannableForwardValue returns the next non-structural candidate forward
// token, if one exists.
func (e *Engine) peekPlannableForwardValue() *Value {
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
