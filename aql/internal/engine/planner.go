package engine

// plannerBestSigForForward computes the best signature and stack arg count for
// setting up forward collection. It centralizes scoring for:
//   - signature intrinsic rank (arity + specificity)
//   - consumed stack values
//   - first forward lookahead compatibility
func (e *Engine) plannerBestSigForForward(fn *Function, w WordInfo, resolved []Value) (*Signature, int) {
	var best *Signature
	var bestScore int
	var bestStackCount int

	peekVal := e.peekPlannableForwardValue()

	for i := range fn.Signatures {
		sig := &fn.Signatures[i]
		if len(sig.Args) == 0 {
			continue
		}
		if w.ArgCount >= 0 && sig.TotalArgs() != w.ArgCount {
			continue
		}

		stackCount, usedArgs := plannerForwardStackCoverage(sig.Args, resolved)
		score := signatureScore(sig)

		// Prefer signatures that can consume already-resolved stack values.
		score += stackCount * 25

		// Prefer signatures whose first currently-unmatched argument can be
		// satisfied by the immediate forward candidate.
		if peekVal != nil && stackCount < len(sig.Args) {
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
			bestStackCount = stackCount
		}
	}

	return best, bestStackCount
}

// plannerStackCoverage returns how many values from the top of the resolved
// stack can be positionally matched to the first N signature arguments, and
// the arg-slot usage bitmap for that assignment. Arguments are never permuted.
func plannerStackCoverage(sigArgs []Type, resolved []Value) (int, []bool) {
	usedArgs := make([]bool, len(sigArgs))
	maxTry := len(sigArgs)
	if maxTry > len(resolved) {
		maxTry = len(resolved)
	}

	stackCount := 0
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
			stackCount = tryN
			for i := 0; i < tryN; i++ {
				usedArgs[i] = true
			}
			break
		}
	}

	return stackCount, usedArgs
}

// plannerForwardStackCoverage returns how many values from the top of the
// resolved stack can fill the LAST N positions of a signature, reading the
// stack in reverse order (top → first remaining sig arg). This supports
// forward-first matching: forward tokens fill sigArgs from the beginning,
// and stack values fill the remainder from the end.
//
// For example, with sigArgs=[Integer, String, Boolean] and stack=[true, "a"]:
//   - tryN=2: sigStart=1, stack top="a" → String ✓, next=true → Boolean ✓
//   - Returns stackCount=2, usedArgs=[false, true, true]
func plannerForwardStackCoverage(sigArgs []Type, resolved []Value) (int, []bool) {
	usedArgs := make([]bool, len(sigArgs))
	maxTry := len(sigArgs)
	if maxTry > len(resolved) {
		maxTry = len(resolved)
	}

	for tryN := maxTry; tryN >= 1; tryN-- {
		sigStart := len(sigArgs) - tryN
		ok := true
		for j := 0; j < tryN; j++ {
			// Stack top (resolved[len-1]) → sigArgs[sigStart]
			// Stack deeper (resolved[len-2]) → sigArgs[sigStart+1], etc.
			stackVal := resolved[len(resolved)-1-j]
			if !stackVal.VType.Matches(sigArgs[sigStart+j]) {
				ok = false
				break
			}
		}
		if ok {
			for j := sigStart; j < len(sigArgs); j++ {
				usedArgs[j] = true
			}
			return tryN, usedArgs
		}
	}
	return 0, usedArgs
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
