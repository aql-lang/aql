package engine

// plannerSequentialForward selects the best signature for forward collection
// by attempting each pre-sorted signature in order and scanning forward tokens
// sequentially. Unlike the scoring-based plannerBestSigForForward, this
// approach walks the actual forward token stream and uses structural cues
// (end, function words, open parens) to decide when to stop.
//
// For each signature (already sorted by signatureScore, highest first):
//  1. Compute how many suffix args the stack can provide.
//  2. Scan forward for the remaining prefix args.
//  3. The first signature that can be fully satisfied wins.
//
// Token handling during forward scan:
//   - Literal/value: match against the current sig arg type
//   - end / ")": stop scanning, try stack match for remaining args
//   - Known function word: accept as one forward arg, then stop (case D)
//   - Open paren "(" : will resolve to a value — count as one arg
//   - Word at TWord position: matches directly (raw word)
//   - Word with /q: treat as quoted (Atom) for matching
//   - Defined word: resolve to its def type for matching
//   - Unknown word: becomes Atom (or Boolean for true/false)
//   - Type name: if expected type is a metatype, collect as forward arg
func (e *Engine) plannerSequentialForward(fn *Function, w WordInfo, resolved []Value) (*Signature, int) {
	// If the next forward token is a word with a DefStack entry, prefer
	// signatures that accept TWord at position 0 — this allows inspect and
	// similar words to capture the name without executing the def body.
	if e.pointer+1 < len(e.stack) {
		next := e.stack[e.pointer+1]
		if next.IsWord() {
			nw := next.AsWord()
			if len(e.registry.DefStacks[nw.Name]) > 0 {
				for i := range fn.Signatures {
					sig := &fn.Signatures[i]
					if len(sig.Args) == 0 || sig.Fallback {
						continue
					}
					if w.ArgCount >= 0 && sig.TotalArgs() != w.ArgCount {
						continue
					}
					if sig.Args[0].Equal(TWord) || (sig.QuoteArgs != nil && sig.QuoteArgs[0]) {
						fm, sc := e.trySequentialMatch(sig, resolved, w.ForceForward)
						if fm+sc == len(sig.Args) && fm > 0 {
							return sig, sc
						}
					}
				}
			}
		}
	}

	for i := range fn.Signatures {
		sig := &fn.Signatures[i]
		if len(sig.Args) == 0 {
			continue
		}
		if w.ArgCount >= 0 && sig.TotalArgs() != w.ArgCount {
			continue
		}
		if sig.Fallback {
			continue
		}

		forwardMatched, stackCount := e.trySequentialMatch(sig, resolved, w.ForceForward)
		if forwardMatched+stackCount == len(sig.Args) && forwardMatched > 0 {
			return sig, stackCount
		}
	}
	return nil, 0
}

// trySequentialMatch walks the forward token stream and tries to match
// sig args from the front, then fills remaining args from the stack.
//
// The stack is checked FIRST to determine how many suffix positions it can
// fill. The forward scan then only needs to fill the remaining prefix
// positions. This prevents over-collection in chained infix expressions
// like "1 add 2 add 3" — the stack provides arg 1, so only 1 forward
// arg is needed, and the scan stops after "2".
//
// Returns (forwardMatched, stackCount).
func (e *Engine) trySequentialMatch(sig *Signature, resolved []Value, forceForward bool) (int, int) {
	nArgs := len(sig.Args)

	// Step 1: compute how many SUFFIX args the stack can provide.
	stackCount := 0
	if !forceForward {
		stackCount = sequentialSuffixMatch(sig.Args, resolved)
		if stackCount >= nArgs {
			// Stack alone satisfies — defer to stack-only matcher.
			return 0, 0
		}
	}

	// The forward scan needs to fill positions 0..forwardNeeded-1.
	forwardNeeded := nArgs - stackCount
	forwardMatched := 0

	// Step 2: scan forward tokens for the prefix positions.
	scanIdx := e.pointer + 1

	for forwardMatched < forwardNeeded && scanIdx < len(e.stack) {
		tok := e.stack[scanIdx]
		expectedType := sig.Args[forwardMatched]

		// Structural token: stop forward scanning.
		if tok.IsForward() {
			break
		}

		if tok.IsWord() {
			ww := tok.AsWord()

			// "end" terminates forward collection.
			if ww.Name == "end" {
				break
			}

			// ")" terminates forward collection.
			if ww.Name == ")" {
				break
			}

			// "(" starts a sub-expression — assume it can produce any type.
			// Skip past the matching ")" so the scan continues with
			// the token AFTER the paren group.
			if ww.Name == "(" {
				forwardMatched++
				scanIdx++
				depth := 1
				for scanIdx < len(e.stack) && depth > 0 {
					inner := e.stack[scanIdx]
					if inner.IsWord() {
						iw := inner.AsWord()
						if iw.Name == "(" {
							depth++
						} else if iw.Name == ")" {
							depth--
						}
					}
					scanIdx++
				}
				continue
			}

			// Sig expects TWord: any word matches directly, regardless
			// of whether it's a function name.
			if TWord.Matches(expectedType) {
				forwardMatched++
				scanIdx++
				continue
			}

			// /q modifier: word at this position is treated as Atom.
			if sig.QuoteArgs != nil && sig.QuoteArgs[forwardMatched] {
				if TAtom.Matches(expectedType) {
					forwardMatched++
					scanIdx++
					continue
				}
				break // /q position but type doesn't match
			}

			// Defined word (simple value): resolves to its def type.
			if ds := e.registry.DefStacks[ww.Name]; len(ds) > 0 {
				top := ds[len(ds)-1]
				if sigTypeMatches(top, expectedType) || expectedType.Equal(TAny) {
					forwardMatched++
					scanIdx++
					continue
				}
				if _, ok := top.Data.(FnDefInfo); !ok {
					break // simple def, type mismatch
				}
			}

			// Case D: see a function → stop before it.
			// Do not consume the function word. The remaining
			// signature positions will be tried from the stack.
			if e.registry.Lookup(ww.Name) != nil {
				break
			}

			// Unknown word: becomes Atom (or Boolean for true/false).
			// Type names: if expected type is a metatype, collect as forward arg.
			resolvedType := TAtom
			if ww.Name == "true" || ww.Name == "false" {
				resolvedType = TBoolean
			} else if tn, isType := typeNames[ww.Name]; isType {
				typeLit := NewTypeLiteral(tn)
				if sigTypeMatches(typeLit, expectedType) {
					forwardMatched++
					scanIdx++
					continue
				}
				break // type name doesn't match expected metatype
			}
			if sigTypeMatches(Value{VType: resolvedType}, expectedType) || expectedType.Equal(TAny) {
				forwardMatched++
				scanIdx++
				continue
			}
			break // unknown word type doesn't match
		}

		// Open paren value (already an OpenParen marker).
		if tok.IsOpenParen() {
			forwardMatched++
			scanIdx++
			continue
		}

		// Literal value: direct type check (metatype-aware).
		if sigTypeMatches(tok, expectedType) || expectedType.Equal(TAny) {
			if tok.Data == nil && (expectedType.Equal(TMap) || expectedType.Equal(TList)) {
				break // reject type literals for concrete Map/List
			}
			forwardMatched++
			scanIdx++
			continue
		}

		// Type mismatch — stop forward scanning.
		break
	}

	// Check if forward + stack fully satisfies the signature.
	if forwardMatched == forwardNeeded {
		return forwardMatched, stackCount
	}

	// Forward scan stopped short. Try filling remaining from stack suffix.
	remaining := nArgs - forwardMatched
	if forwardMatched > 0 && remaining > 0 && len(resolved) >= remaining {
		ok := true
		for j := 0; j < remaining; j++ {
			stackVal := resolved[len(resolved)-remaining+j]
			sigIdx := forwardMatched + j
			if !sigTypeMatches(stackVal, sig.Args[sigIdx]) {
				ok = false
				break
			}
		}
		if ok {
			return forwardMatched, remaining
		}
	}

	return forwardMatched, 0
}

// sequentialSuffixMatch returns how many values from the top of the resolved
// stack can fill the LAST N positions of sigArgs. Tries the largest coverage
// first, shrinks until it fits. Stack top maps to the last sig position.
//
// Example: sigArgs=[Integer, Integer], resolved=[1]
//   tryN=1: sigStart=1, stack top=1(Integer) matches sigArgs[1] → returns 1
func sequentialSuffixMatch(sigArgs []Type, resolved []Value) int {
	maxTry := len(sigArgs)
	if maxTry > len(resolved) {
		maxTry = len(resolved)
	}
	for tryN := maxTry; tryN >= 1; tryN-- {
		sigStart := len(sigArgs) - tryN
		ok := true
		for j := 0; j < tryN; j++ {
			// Map bottom-up: resolved[len-tryN+j] → sigArgs[sigStart+j].
			// This matches MatchSignature's ordering where stack[base+j]
			// corresponds to sigArgs[j].
			stackVal := resolved[len(resolved)-tryN+j]
			if !sigTypeMatches(stackVal, sigArgs[sigStart+j]) {
				ok = false
				break
			}
		}
		if ok {
			return tryN
		}
	}
	return 0
}
