package engine

// matchSignature is the unified signature matching function. It replaces
// MatchSignature, MatchSignatureReversed, plannerSequentialForward, and
// trySequentialMatch with a single algorithm and one main loop.
//
// Algorithm (per the user's steps):
//
//	0.1 Try each pre-sorted signature in order; first full match wins.
//	1.1 If stack-only (or /s) and not /f: skip forward, go to step 2.
//	    If stack-only but called with /f: override → do forward scan.
//	1.2 Match each parameter in order against future tokens.
//	1.3 Stop if all params matched, or if /N params reached.
//	1.4 Move to step 2 at boundary: function word, pipe barrier.
//	1.5 If open paren, treat as one arg (sub-expression).
//	2.1 Match remaining params against stack, working backwards (top first).
//	2.2 Stop once all or /N params reached.
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

	// Pre-priority: when the next forward token is a defined word, prefer
	// signatures expecting TWord or /q at position 0. This lets inspect-style
	// words capture names without evaluating the def body.
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

	// Two passes when preferWordSig is true: first pass only considers
	// sigs with TWord/q at arg[0]; second pass considers all sigs.
	// When preferWordSig is false, just one pass considering all sigs.
	passes := 1
	if preferWordSig {
		passes = 2
	}

	for pass := 0; pass < passes; pass++ {
		// Phase A: try forward+stack matching across all sigs.
		// Phase B: try pure-stack matching across all sigs.
		// Phase A runs first so forward+stack matches (especially barrier
		// sigs) take priority over pure-stack matches of other sigs.
		for phase := 0; phase < 2; phase++ {
			for si := range fn.Signatures {
				sig := &fn.Signatures[si]

				if sig.Fallback {
					continue
				}

				if w.ArgCount >= 0 && sig.TotalArgs() != w.ArgCount {
					continue
				}

				nArgs := len(sig.Args)

				// Pre-priority filter.
				if preferWordSig && pass == 0 && nArgs > 0 {
					if !sig.Args[0].Equal(TWord) &&
						!(sig.QuoteArgs != nil && sig.QuoteArgs[0]) {
						continue
					}
				}
				if preferWordSig && pass == 1 && nArgs > 0 {
					if sig.Args[0].Equal(TWord) ||
						(sig.QuoteArgs != nil && sig.QuoteArgs[0]) {
						continue
					}
				}

				// ── Phase 1: forward matching ─────────────────────────────

				forwardMatched := 0

				if !skipForward && nArgs > 0 {
					scanIdx := e.pointer + 1

					for forwardMatched < nArgs && scanIdx < len(e.stack) {
						if sig.BarrierPos > 0 && forwardMatched >= sig.BarrierPos {
							break
						}

						tok := e.stack[scanIdx]
						expectedType := sig.Args[forwardMatched]

						if tok.IsForward() || tok.VType.Matches(TMark) || tok.VType.Matches(TMove) ||
							tok.VType.Matches(TInternal) || tok.VType.Matches(TReturnCheck) {
							break
						}

						if tok.IsWord() {
							ww := tok.AsWord()

							if ww.Name == "end" || ww.Name == ")" {
								break
							}

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

							// Sig expects TWord: any word matches directly.
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
								break
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

							// 1.4: function word → boundary, stop forward scan.
							if e.registry.Lookup(ww.Name) != nil {
								break
							}

							// Unknown word: becomes Atom (or Boolean for true/false).
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
								break
							} else if tn, isType := ResolveTypePath(ww.Name); isType {
								typeLit := NewTypeLiteral(tn)
								if sigTypeMatches(typeLit, expectedType) {
									forwardMatched++
									scanIdx++
									continue
								}
								break
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

						// Literal value: direct type check.
						if sigTypeMatches(tok, expectedType) || expectedType.Equal(TAny) {
							if tok.Data == nil && tok.VType.Equal(expectedType) &&
								(expectedType.Equal(TMap) || expectedType.Equal(TList)) {
								break // reject type literals for concrete Map/List
							}
							forwardMatched++
							scanIdx++
							continue
						}

						// Type mismatch → stop forward scanning.
						break
					}
				}

				// 0-arg sigs: defer to fallback section so longer sigs
				// get a chance via stack matching in Phase B first.
				if nArgs == 0 {
					continue
				}

				// All params matched by forward?
				if forwardMatched == nArgs {
					// Phase A only: forward-only matches.
					if phase == 0 {
						return sig, forwardMatched, 0, false
					}
					continue
				}

				// ── Phase 2: stack matching ───────────────────────────────

				// Phase A: try forward+stack (forwardMatched > 0).
				// Phase B: try pure-stack (forwardMatched = 0).
				fwd := forwardMatched
				if phase == 0 {
					// Phase A: only interested in mixed forward+stack.
					if fwd == 0 {
						continue
					}
				} else {
					// Phase B: only interested in pure-stack.
					fwd = 0
				}

				remaining := nArgs - fwd

				// Inside a pending forward scope: all args will be collected
				// from forward. Accept if either (a) forward matched all
				// params, or (b) forward+stack would satisfy the sig
				// (the old plannerSequentialForward validated fwd+stack==nArgs
				// before the insideForward override forced all args forward).
				if insideForward && fwd > 0 {
					if fwd == nArgs {
						return sig, nArgs, 0, false
					}
					// Check if stack could fill the remaining (validates
					// the match like the old code did).
					if len(resolved) >= remaining {
						canStack := true
						for j := 0; j < remaining; j++ {
							stackVal := resolved[len(resolved)-1-j]
							sigIdx := fwd + j
							if !sigTypeMatches(stackVal, sig.Args[sigIdx]) {
								canStack = false
								break
							}
						}
						if canStack {
							return sig, nArgs, 0, false
						}
					}
					continue
				}

				// /f means all args must come from forward.
				if w.ForceForward && fwd > 0 && remaining > 0 {
					continue
				}

				if len(resolved) < remaining {
					continue // not enough stack values
				}

				stackBase := len(resolved) - remaining
				// Forward+stack (fwd > 0): nearest-first for stack remainder
				// (old trySequentialMatch always used top-first).
				// Pure-stack (fwd == 0): forward-prec words use nearest-first
				// (old MatchSignatureReversed), stack-only use deepest-first.
				useNearest := !stackOnly && !w.ForceStack

				for order := 0; order < 2; order++ {
					if order == 0 && !useNearest {
						continue
					}
					// When forward matched some args (fwd > 0), only try
					// nearest-first for the stack remainder. The old
					// trySequentialMatch only used top-first ordering;
					// falling back to deepest-first would accept combos
					// the old code rejected (e.g. internal tokens matched
					// as forward args).
					if order == 1 && fwd > 0 && useNearest {
						break
					}

					reversed := (order == 0)

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

					return sig, fwd, remaining, reversed
				}
			}
		}
	}

	// Try fallback (0-arg def handler).
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
