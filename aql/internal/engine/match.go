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
//	1.5 If you hit an open paren, treat as boundary (pre-evaluated).
//	2.1 Match the remaining parameters against the stack, working
//	    backwards (top of stack first).
//	2.2 Stop once all or /N params reached.
//
// This is implemented as one outer loop over signatures and one inner
// loop over parameters. No separate functions are called for matching.
//
// Returns: matched signature and arg positions (absolute stack indices
// in signature order). Positions > pointer are forward args that need
// deferred collection. Positions < pointer are stack args. Returns nil
// sig if no signature matches.
func (e *Engine) matchSignature(fn *FnDefInfo, w WordInfo, resolved []Value) (*Signature, []int) {
	stackOnly := !fn.ForwardPrecedence && !w.ForceForward
	skipForward := (stackOnly || w.ForceStack) && !w.ForceForward
	insideForward := false
	if !stackOnly {
		insideForward = e.isInsidePendingForward()
	}

	// When the next forward token is a Word, prefer signatures
	// expecting TWord or /q at position 0 (inspect-style name
	// capture). The user wrote a Word, not a String — the /q sig
	// captures the user's intent that the name is data, not a call
	// site. The non-/q TString sister sig is for callers who pass a
	// string literal. This also covers untype Foo (Foo in r.Types),
	// `m.Color` after import (Color is a key in the imported map),
	// and inspect-style name capture.
	preferWordSig := false
	if !skipForward && e.pointer+1 < len(e.stack) {
		next := e.stack[e.pointer+1]
		if next.IsWord() {
			preferWordSig = true
		}
	}

	// Build a map from resolved values to their absolute stack indices.
	// This lets us record exact positions for stack-matched args.
	resolvedIdx := e.resolvedIndicesBefore(len(resolved))

	// Track the best non-preferred match so that if no preferred sig
	// matches, we can fall back to it without a second pass.
	type matchResult struct {
		sig       *Signature
		positions []int
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

		positions := make([]int, nArgs)
		fwd := 0 // number of params matched by forward tokens

		// 1.1: if stack-only and not /f, skip forward scan entirely.
		if !skipForward {
			scanIdx := e.pointer + 1

			// One inner loop over parameters, matching forward tokens.
			for fwd < nArgs && scanIdx < len(e.stack) {
				// 1.3: stop at /N limit (encoded as BarrierPos).
				// Only apply when stack args exist; in pure prefix
				// position, allow collecting all args forward.
				if sig.BarrierPos > 0 && fwd >= sig.BarrierPos && len(resolved) > 0 {
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
					ww, _ := tok.AsWord()

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
						positions[fwd] = scanIdx
						fwd++
						scanIdx++
						continue
					}

					// /q modifier: word treated as Atom.
					if sig.QuoteArgs != nil && sig.QuoteArgs[fwd] {
						if TAtom.Matches(expectedType) {
							positions[fwd] = scanIdx
							fwd++
							scanIdx++
							continue
						}
						break
					}

					// Defined word: resolves to its def type.
					if top, ok := e.registry.TopOfDefStack(ww.Name); ok {
						if sigTypeMatches(top, expectedType) || expectedType.Equal(TAny) {
							positions[fwd] = scanIdx
							fwd++
							scanIdx++
							continue
						}
						if _, ok := top.Data.(FnDefInfo); !ok {
							break // simple def, type mismatch
						}
					}

					// Named type from r.Types: resolves to the type
					// value (mirror of stepWord's r.Types lookup so the
					// planner's expected type matches what stepWord
					// will actually push at runtime). Predicate types
					// arrive as TFnDef/TFunction values; plan against
					// that VType for sig matching.
					if tv, ok := e.registry.TopOfTypeStack(ww.Name); ok {
						if sigTypeMatches(tv, expectedType) || expectedType.Equal(TAny) {
							positions[fwd] = scanIdx
							fwd++
							scanIdx++
							continue
						}
						break // named-type value doesn't fit this slot
					}

					// 1.4: function word — boundary, stop.
					if e.registry.Lookup(ww.Name) != nil {
						break
					}

					// Known literals: true/false → Boolean, type names → type literal.
					if ww.Name == "true" || ww.Name == "false" {
						if sigTypeMatches(Value{VType: TBoolean}, expectedType) || expectedType.Equal(TAny) {
							positions[fwd] = scanIdx
							fwd++
							scanIdx++
							continue
						}
						break
					}
					if tn, isType := typeNames[ww.Name]; isType {
						if sigTypeMatches(NewTypeLiteral(tn), expectedType) {
							positions[fwd] = scanIdx
							fwd++
							scanIdx++
							continue
						}
						break
					}
					if tn, isType := ResolveTypePath(ww.Name); isType {
						if sigTypeMatches(NewTypeLiteral(tn), expectedType) {
							positions[fwd] = scanIdx
							fwd++
							scanIdx++
							continue
						}
						break
					}

					// Undefined word: always resolves to Atom.
					if sigTypeMatches(Value{VType: TAtom}, expectedType) || expectedType.Equal(TAny) {
						positions[fwd] = scanIdx
						fwd++
						scanIdx++
						continue
					}
					break // type mismatch
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
					positions[fwd] = scanIdx
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
				if bestDeferred == nil {
					bestDeferred = &matchResult{sig, append([]int(nil), positions...)}
				}
				continue
			}
			return sig, positions
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
					// Fill remaining positions from stack (nearest first).
					for j := 0; j < remaining; j++ {
						ri := len(resolvedIdx) - 1 - j
						positions[fwd+j] = resolvedIdx[ri]
					}
					if preferWordSig && !isPreferred {
						if bestDeferred == nil {
							bestDeferred = &matchResult{sig, append([]int(nil), positions...)}
						}
						continue
					}
					return sig, positions
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
		// Forward-prec words: nearest first (top → sig.Args[fwd]).
		// Stack-only / ForceStack: deepest first.
		nearestFirst := !stackOnly && !w.ForceStack

		// When forward matched some args (fwd > 0), only nearest-first
		// is valid for the stack remainder.
		if fwd > 0 && !nearestFirst {
			continue
		}

		allMatch := true
		for j := 0; j < remaining; j++ {
			var ri int
			if nearestFirst {
				ri = len(resolvedIdx) - 1 - j
			} else {
				ri = len(resolvedIdx) - remaining + j
			}
			stackVal := resolved[ri]
			sigIdx := fwd + j

			// /q is a forward-only rule (see Signature.QuoteArgs doc).
			// stackVal cannot be a Word in normal execution: stepWord
			// has already resolved any Word at the pointer to a function
			// call, defined value, or Atom, and quote produces Atoms.
			// The branch below is defensive only — a stack Atom matches
			// an [Atom/q, ...] sig via the regular sigTypeMatches path
			// just below, no /q involvement required.
			if sig.QuoteArgs != nil && sig.QuoteArgs[sigIdx] && stackVal.VType.Equal(TWord) {
				if !TAtom.Matches(sig.Args[sigIdx]) {
					allMatch = false
					break
				}
				positions[sigIdx] = resolvedIdx[ri]
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
			positions[sigIdx] = resolvedIdx[ri]
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
				val := e.stack[positions[idx]]
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
				bestDeferred = &matchResult{sig, append([]int(nil), positions...)}
			}
			continue
		}
		return sig, positions
	}

	// Return deferred non-preferred match if one was found.
	if bestDeferred != nil {
		return bestDeferred.sig, bestDeferred.positions
	}

	// Try fallback (0-arg or Fallback handler).
	for si := range fn.Signatures {
		sig := &fn.Signatures[si]
		if w.ArgCount >= 0 && sig.TotalArgs() != w.ArgCount {
			continue
		}
		if len(sig.Args) == 0 || sig.Fallback {
			return sig, nil
		}
	}

	return nil, nil
}
