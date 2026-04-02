package engine

// matchSignature is the unified signature matching function.
//
// Algorithm:
//
//	0.1 Using the ordered signatures, attempt to match in order,
//	    stopping at the first match.
//	1.1 If stack-only (or /s) and not /f: skip forward, go to step 2.
//	    If stack-only but /f: override → do forward scan.
//	1.2 Match each parameter in order against future tokens.
//	1.3 Stop if all params matched, or if /N params reached.
//	1.4 Move to step 2 if you hit a boundary condition:
//	    a function word, a pipe barrier, or "end".
//	1.5 If you hit an open paren, resolve the paren expression first
//	    (count as one forward arg, skip past matching close-paren).
//	2.1 Match the remaining parameters against the stack, working
//	    backwards (top of stack first).
//	2.2 Stop once all or /N params reached.
//
// This is implemented as one outer loop over signatures and one inner
// loop over parameters. No separate functions are called for matching.
//
// Returns: matched signature, forward arg count, stack arg count, needsRearrange.
// If forwardCount > 0 the caller must insertForward for deferred collection.
// If forwardCount == 0 all args are on the stack (immediate execution).
// needsRearrange is true when the stack match used nearest-first order
// and the caller should call rearrangeForForward to put values in sig order.
// Returns nil sig if no signature matches.
func (e *Engine) matchSignature(fn *FnDefInfo, w WordInfo, resolved []Value) (*Signature, int, int, bool) {
	stackOnly := !fn.ForwardPrecedence && !w.ForceForward
	skipForward := (stackOnly || w.ForceStack) && !w.ForceForward
	insideForward := false
	if !stackOnly {
		insideForward = e.isInsidePendingForward()
	}

	// When the next forward token is a defined word, prefer signatures
	// expecting TWord or /q at position 0 (inspect-style name capture).
	// We handle this by trying TWord/q sigs first, then the rest, all
	// within the single outer loop. bestMatch tracks the best result
	// found so far; a TWord/q match is returned immediately if found.
	preferWordSig := false
	if !skipForward && e.pointer+1 < len(e.stack) {
		next := e.stack[e.pointer+1]
		if next.IsWord() {
			nw := next.AsWord()
			if len(e.registry.DefStacks[nw.Name]) > 0 {
				preferWordSig = true
			}
		}
	}

	// Track the best non-preferred match so that if no preferred sig
	// matches, we can fall back to it without a second pass.
	type matchResult struct {
		sig       *Signature
		fwd, stk  int
		rearrange bool
	}
	var bestDeferred *matchResult

	// ── 0.1: one outer loop over sorted signatures ───────────────
	for si := range fn.Signatures {
		sig := &fn.Signatures[si]

		if sig.Fallback {
			continue
		}
		if w.ArgCount >= 0 && sig.TotalArgs() != w.ArgCount {
			continue
		}

		nArgs := len(sig.Args)

		// 0-arg sigs are deferred to the fallback section at the bottom.
		if nArgs == 0 {
			continue
		}

		// Check if this is a preferred (TWord/q at arg[0]) signature.
		isPreferred := preferWordSig && nArgs > 0 &&
			(sig.Args[0].Equal(TWord) || (sig.QuoteArgs != nil && sig.QuoteArgs[0]))

		// ── Step 1: forward matching ─────────────────────────────

		fwd := 0 // number of params matched by forward tokens

		// 1.1: if stack-only and not /f, skip forward scan entirely.
		if !skipForward {
			scanIdx := e.pointer + 1

			// One inner loop over parameters, matching forward tokens.
			for fwd < nArgs && scanIdx < len(e.stack) {
				// 1.3: stop at /N limit (encoded as BarrierPos).
				if sig.BarrierPos > 0 && fwd >= sig.BarrierPos {
					break
				}

				tok := e.stack[scanIdx]
				expectedType := sig.Args[fwd]

				// 1.4: structural boundaries — stop forward scan.
				if tok.IsForward() || tok.VType.Matches(TMark) || tok.VType.Matches(TMove) ||
					tok.VType.Matches(TInternal) || tok.VType.Matches(TReturnCheck) {
					break
				}

				if tok.IsWord() {
					ww := tok.AsWord()

					// 1.4: "end", ")" — boundary, stop.
					if ww.Name == "end" || ww.Name == ")" {
						break
					}

					// 1.5: open parens are pre-evaluated by preEvalParens
					// before matching begins. If one remains, treat as boundary.
					if ww.Name == "(" {
						break
					}

					// Sig expects TWord: any word matches directly.
					if TWord.Matches(expectedType) {
						fwd++
						scanIdx++
						continue
					}

					// /q modifier: word treated as Atom.
					if sig.QuoteArgs != nil && sig.QuoteArgs[fwd] {
						if TAtom.Matches(expectedType) {
							fwd++
							scanIdx++
							continue
						}
						break
					}

					// Defined word: resolves to its def type.
					if ds := e.registry.DefStacks[ww.Name]; len(ds) > 0 {
						top := ds[len(ds)-1]
						if sigTypeMatches(top, expectedType) || expectedType.Equal(TAny) {
							fwd++
							scanIdx++
							continue
						}
						if _, ok := top.Data.(FnDefInfo); !ok {
							break // simple def, type mismatch
						}
					}

					// 1.4: function word — boundary, stop.
					if e.registry.Lookup(ww.Name) != nil {
						break
					}

					// Unknown word: resolve to Atom, Boolean, or type literal.
					resolvedType := TAtom
					if ww.Name == "true" || ww.Name == "false" {
						resolvedType = TBoolean
					} else if tn, isType := typeNames[ww.Name]; isType {
						typeLit := NewTypeLiteral(tn)
						if sigTypeMatches(typeLit, expectedType) {
							fwd++
							scanIdx++
							continue
						}
						break
					} else if tn, isType := ResolveTypePath(ww.Name); isType {
						typeLit := NewTypeLiteral(tn)
						if sigTypeMatches(typeLit, expectedType) {
							fwd++
							scanIdx++
							continue
						}
						break
					}
					if sigTypeMatches(Value{VType: resolvedType}, expectedType) || expectedType.Equal(TAny) {
						fwd++
						scanIdx++
						continue
					}
					break // unknown word, type mismatch
				}

				// Open paren marker: boundary, stop forward scan.
				if tok.IsOpenParen() {
					break
				}

				// Literal value: direct type check.
				if sigTypeMatches(tok, expectedType) || expectedType.Equal(TAny) {
					if tok.Data == nil && tok.VType.Equal(expectedType) &&
						(expectedType.Equal(TMap) || expectedType.Equal(TList)) {
						break // reject type literals for concrete Map/List
					}
					fwd++
					scanIdx++
					continue
				}

				// Type mismatch — stop forward scanning.
				break
			}
		}

		// 1.3: all params matched by forward?
		if fwd == nArgs {
			if preferWordSig && !isPreferred {
				// Defer: a preferred sig might match later.
				if bestDeferred == nil {
					bestDeferred = &matchResult{sig, fwd, 0, false}
				}
				continue
			}
			return sig, fwd, 0, false
		}

		// Inside a pending forward scope: all args must come from
		// forward. Accept only if forward+stack would satisfy the sig.
		if insideForward && fwd > 0 {
			remaining := nArgs - fwd
			if len(resolved) >= remaining {
				canStack := true
				for j := 0; j < remaining; j++ {
					stackVal := resolved[len(resolved)-1-j]
					if !sigTypeMatches(stackVal, sig.Args[fwd+j]) {
						canStack = false
						break
					}
				}
				if canStack {
					if preferWordSig && !isPreferred {
						if bestDeferred == nil {
							bestDeferred = &matchResult{sig, nArgs, 0, false}
						}
						continue
					}
					return sig, nArgs, 0, false
				}
			}
			continue
		}

		// /f means all args must come from forward.
		if w.ForceForward && fwd > 0 {
			continue
		}

		// ── Step 2: stack matching ───────────────────────────────

		remaining := nArgs - fwd
		if len(resolved) < remaining {
			continue // not enough stack values
		}

		// 2.1: match remaining params against the stack.
		// Forward-precedence words: top of stack first (nearest).
		// Stack-only words: bottom of resolved first (deepest).
		reversed := !stackOnly && !w.ForceStack
		stackBase := len(resolved) - remaining

		// When forward matched some args (fwd > 0), only nearest-first
		// is valid for the stack remainder — deepest-first would accept
		// combos the old code rejected.
		if fwd > 0 && !reversed {
			continue
		}

		allMatch := true
		for j := 0; j < remaining; j++ {
			var stackVal Value
			if reversed {
				stackVal = resolved[len(resolved)-1-j]
			} else {
				stackVal = resolved[stackBase+j]
			}
			sigIdx := fwd + j

			if sig.QuoteArgs != nil && sig.QuoteArgs[sigIdx] && stackVal.VType.Equal(TWord) {
				if !TAtom.Matches(sig.Args[sigIdx]) {
					allMatch = false
					break
				}
				continue
			}
			if !sigTypeMatches(stackVal, sig.Args[sigIdx]) {
				allMatch = false
				break
			}
			if stackVal.Data == nil && (sig.Args[sigIdx].Equal(TMap) || sig.Args[sigIdx].Equal(TList)) {
				allMatch = false
				break
			}
		}
		if !allMatch {
			continue
		}

		// Check structural patterns on stack args.
		if sig.Patterns != nil {
			patternOk := true
			for idx, pattern := range sig.Patterns {
				if idx < fwd {
					continue
				}
				stackJ := idx - fwd
				var val Value
				if reversed {
					val = resolved[len(resolved)-1-stackJ]
				} else {
					val = resolved[stackBase+stackJ]
				}
				if pattern.VType.Equal(TMap) && val.VType.Equal(TMap) &&
					pattern.Data != nil && val.Data != nil &&
					!pattern.IsOptionsType() &&
					!val.IsRecordType() && !val.IsTypedMap() && !val.IsOptionsType() {
					if !openUnifyMap(pattern, val) {
						patternOk = false
						break
					}
				} else {
					if _, uOk := Unify(val, pattern); !uOk {
						patternOk = false
						break
					}
				}
			}
			if !patternOk {
				continue
			}
		}

		// Full match found.
		if preferWordSig && !isPreferred {
			if bestDeferred == nil {
				bestDeferred = &matchResult{sig, fwd, remaining, reversed}
			}
			continue
		}
		return sig, fwd, remaining, reversed
	}

	// Return deferred non-preferred match if one was found.
	if bestDeferred != nil {
		return bestDeferred.sig, bestDeferred.fwd, bestDeferred.stk, bestDeferred.rearrange
	}

	// Try fallback (0-arg or Fallback handler).
	for si := range fn.Signatures {
		sig := &fn.Signatures[si]
		if w.ArgCount >= 0 && sig.TotalArgs() != w.ArgCount {
			continue
		}
		if len(sig.Args) == 0 || sig.Fallback {
			return sig, 0, 0, false
		}
	}

	return nil, 0, 0, false
}
