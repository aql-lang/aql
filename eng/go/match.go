package eng

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
//
//nolint:gocyclo,gocognit // dispatch is inherently a big switch; see STATIC_ANALYSIS_REPORT.md
func (e *Engine) matchSignature(fn *FnDefInfo, w WordInfo, resolved []Value) (*Signature, []int) {
	// Unified dispatch (post §1.4 fix): no more stackOnly/forward-prec
	// dichotomy at the word level. Each sig declares its own boundary
	// via BarrierPos — the count of leading args that may be collected
	// from forward tokens. Args at sig[BarrierPos..N-1] always come
	// from the stack, top-down. The /s and /f modifiers override
	// BarrierPos at the call site:
	//   - /s (ForceStack)   → boundary at 0, all stack
	//   - /f (ForceForward) → boundary at N, all forward
	insideForward := e.isInsidePendingForward()

	// When the next forward token is a Word, prefer signatures with
	// /q at position 0 (inspect-style name capture). The user wrote a
	// Word, not a String — the /q sig captures the user's intent that
	// the name is data, not a call site. The non-/q TString sister
	// sig is for callers who pass a string literal. This also covers
	// untype Foo (Foo in r.Types), `m.Color` after import (Color is a
	// key in the imported map), and inspect-style name capture.
	preferWordSig := false
	if e.pointer+1 < len(e.stack) {
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

		// Check if this is a preferred (/q at arg[0]) signature.
		isPreferred := preferWordSig && nArgs > 0 &&
			sig.QuoteArgs != nil && sig.QuoteArgs[0]

		// Effective forward limit for this match attempt. /s and /f
		// override the sig's declared boundary.
		forwardLimit := sig.BarrierPos
		switch {
		case w.ForceStack:
			forwardLimit = 0
		case w.ForceForward:
			forwardLimit = nArgs
		}

		// ── Step 1: forward matching ─────────────────────────────

		positions := make([]int, nArgs)
		fwd := 0 // number of params matched by forward tokens

		// Always run the forward scan up to forwardLimit; if it's 0
		// the loop simply doesn't execute and all args come from
		// the stack below.
		{
			scanIdx := e.pointer + 1

			// One inner loop over parameters, matching forward tokens.
			for fwd < forwardLimit && scanIdx < len(e.stack) {

				tok := e.stack[scanIdx]
				expectedType := sig.Args[fwd]

				// 1.4: structural boundaries — stop forward scan.
				if tok.IsForward() || tok.VType.Matches(TMark) || tok.VType.Matches(TMove) ||
					tok.VType.Matches(TInternal) || tok.VType.Matches(TReturnCheck) {
					break
				}

				// 1.4: end, ) — boundary, stop.
				if tok.IsEnd() || tok.IsCloseParen() {
					break
				}

				// 1.5: open parens are pre-evaluated by preEvalParens
				// before matching begins. If one remains, treat as boundary.
				if tok.IsOpenParen() {
					break
				}

				if tok.IsWord() {
					ww, _ := tok.AsWord()
					// /q modifier: capture the upcoming Word as an Atom
					// (the conversion happens at insertForward / stepLiteral
					// time; here we just count it as a match).
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
					if rejectsTypeLiteral(tok, expectedType) {
						break // reject type literal at concrete-payload sig
					}
					positions[fwd] = scanIdx
					fwd++
					scanIdx++
					continue
				}

				// *Type mismatch — stop forward scanning.
				break
			}
		}

		// 1.3: all params matched by forward?
		if fwd == nArgs {
			// Pattern check (post §1.1 fix): scalar literals route
			// through Signature.Patterns instead of value-tagged
			// type paths, so the pattern check has to run for
			// forward-matched positions too. The previous code
			// short-circuited here without consulting Patterns,
			// which made `def fact[0] (1)` fire for any integer.
			if !patternsOk(sig, positions, e.stack, fwd) {
				continue
			}
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

		// /f means all args must come from forward — if any args
		// remain unmatched after the forward scan, this sig fails.
		if w.ForceForward && fwd < nArgs {
			continue
		}

		// ── Step 2: stack matching ───────────────────────────────

		remaining := nArgs - fwd
		if len(resolved) < remaining {
			continue // not enough stack values
		}

		// 2.1: match remaining sig positions against the stack,
		// top-down. sig[fwd] = top of stack, sig[fwd+1] = next deeper,
		// etc. This is the "stack in reverse order" half of the
		// unified rule — same for stack-only sigs (BarrierPos=0)
		// as for partial-boundary sigs.

		allMatch := true
		for j := 0; j < remaining; j++ {
			ri := len(resolvedIdx) - 1 - j
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
			if rejectsTypeLiteral(stackVal, sig.Args[sigIdx]) {
				allMatch = false
				break
			}
			positions[sigIdx] = resolvedIdx[ri]
		}
		if !allMatch {
			continue
		}

		// Check structural patterns on every matched position. Post
		// §1.1 fix: scalar literals route through Patterns regardless
		// of whether they came from forward or stack matching, so the
		// pattern check no longer skips forward positions.
		if !patternsOk(sig, positions, e.stack, fwd) {
			continue
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

// patternsOk runs Signature.Patterns against the matched arg
// positions. `fwd` is the count of positions filled from forward
// tokens; positions [0..fwd) are forward args and [fwd..) are stack
// args.
//
// Forward vs stack handling:
//
//   - Scalar-literal patterns (Integer, Decimal, String, Boolean,
//     Atom — concrete `Data != nil` payloads) are checked on EVERY
//     position. This is the §1.1 entry point: a sig with
//     `Patterns[0] = NewInteger(0)` must reject any non-zero arg
//     regardless of which side it came from.
//   - Structural patterns (record/map shapes, `OpenUnifyMap`
//     candidates) are checked ONLY on stack-matched positions.
//     The legacy semantics — that handlers may further constrain
//     forward args inside the handler body — depends on this skip.
//     Tightening it would break callers like `create` whose 1-arg
//     `(Map) Patterns={kind:"api"}` sig was previously matched on
//     non-api maps when the handler then routed by stack contents.
func patternsOk(sig *Signature, positions []int, stack []Value, fwd int) bool {
	if sig.Patterns == nil {
		return true
	}
	for idx, pattern := range sig.Patterns {
		if idx >= len(positions) {
			continue
		}
		isForward := idx < fwd
		val := stack[positions[idx]]
		if pattern.VType.Equal(TMap) && val.VType.Equal(TMap) &&
			pattern.Data != nil && val.Data != nil &&
			!pattern.IsOptionsType() &&
			!val.IsRecordType() && !val.IsTypedMap() && !val.IsOptionsType() {
			if isForward {
				// Legacy: structural map patterns only enforced on
				// stack positions. See doc comment.
				continue
			}
			if !OpenUnifyMap(pattern, val) {
				return false
			}
			continue
		}
		// Concrete scalar pattern? Always check.
		// *Type-literal / non-concrete pattern on a forward position?
		// Skip — handlers may further constrain inside the body.
		if isForward && pattern.Data == nil {
			continue
		}
		if _, uOk := Unify(val, pattern); !uOk {
			return false
		}
	}
	return true
}
