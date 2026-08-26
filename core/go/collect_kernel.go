package core

// The collection kernel — the shared machine behind boru's forward
// collection (design/FULL-COMPILATION.0.md §6.2, Stage 2).
//
// Forward collection is not one algorithm. It is THREE loops over the same
// tokens with three different stop-condition sets and three different
// position countings: the phase-1 plan walk (collectForward here, once per
// dispatch, over the UNION of viable barriers — it EVALUATES), the
// per-candidate scan inside MatchSignature (once per candidate signature,
// over that signature's OWN BarrierPos — it CLASSIFIES), and the arrival
// loop in stepLiteral (once per arriving value — it DISPATCHES). They are
// not refinements of one another, and merging them would be a behaviour
// change rather than a tidy-up: phase ORDER is load-bearing. Phase 1
// commits or strands before the scan's weaker per-signature tests are ever
// consulted, and it pre-evaluates forms out of existence before the scan
// would have to classify them.
//
// So the kernel is not a merged loop. It is the SEAM the three loops walk
// through: a mutable, spliceable, live-length token window plus the host
// evaluations only the host can perform. The Engine is re-seated on it one
// loop at a time — the differential cannot say which re-seat broke
// something otherwise.
//
// Two properties the seam exists to preserve, both learned the hard way:
//
//   - THE WINDOW MUTATION IS THE INTERFACE BETWEEN THE PHASES. The
//     per-candidate scan has no arm for an interp-string, an XML literal or
//     a paren expression, and needs none — phase 1 has already replaced the
//     form with a plain value by the time the scan runs. An adapter that
//     ran the classifying scan over un-pre-evaluated slots would meet forms
//     it cannot classify. Every mutation performed through this seam
//     therefore OUTLIVES the walk, which is why each mutating arm below is
//     gated on some still-viable overload consuming the position: a form
//     rewritten in the window of a pruned overload would have been
//     rewritten for a dispatch that never happens.
//   - SLOT INDICES ARE NOT STABLE ACROSS EVALUATION. A zero-value paren
//     collapse slides the next token into the evaluated slot; a multi-value
//     collapse leaves extras to be re-examined as later positions. Which
//     slots a statement owns is an OUTPUT of running it — which is why the
//     window is live-length and the walk re-reads Len() after every
//     mutation instead of trusting a computed extent.
//
// Scope note. Today the interpreter is the sole implementation: *Engine for
// the host half, its *Tape for the window. The VM's region-descriptor
// adapter is the second, and it arrives WITH ITS CLIENT in Stage 4, not
// before — core/go is held to 100% coverage BY ITS OWN SUITE
// (cover-gate-core), so a seam arm only the VM adapter reaches would be
// dead code in core's profile and fail the gate. The host methods are
// unexported for exactly that reason: nothing outside core can implement
// this yet, and pretending otherwise would put an unused public API on
// *Engine. Stage 4 exports the interface, its methods, and the two helper
// types they name (viableSig, fwdKind) when the second implementation
// makes that real.

// collectWindow is the token window a collection walk reads and mutates: a
// live-length, spliceable sequence, NOT a frozen slot array. The
// interpreter's *Tape satisfies it as written.
type collectWindow interface {
	// Len is the window's CURRENT length, re-read after every mutation.
	Len() int
	At(i int) Value
	Set(i int, v Value)
	// Splice replaces count tokens at i with repl, changing the length.
	Splice(i, count int, repl ...Value)
}

// collectHost is the evaluator half of the seam: what a collection walk
// cannot do for itself. The two groups are deliberately distinct — the
// EVALUATIONS mutate the window and may raise, the CLASSIFICATIONS are pure
// questions about a token against the live binding set — because the second
// implementation builds them from different material.
type collectHost interface {
	// collectWindow is the token window this host collects over.
	collectWindow() collectWindow

	// --- evaluations: these mutate the window and may raise ---

	// evalGroupAt collapses the paren group whose OpenParen sits at i, in
	// place, to its result value(s).
	evalGroupAt(i int) error
	// evalInterp evaluates an interpolated template string to its String
	// value. It does not write the window; the caller does.
	evalInterp(tok Value) (Value, error)
	// evalXml evaluates an interpolated XML literal to its Node/Xml value.
	// It does not write the window; the caller does.
	evalXml(tok Value) (Value, error)
	// expandSugarAt lowers a sugar marker at i in place, reporting whether
	// it expanded (false means the marker is a boundary). A lowering that
	// is a function of the marker ALONE may commit; one that is a function
	// of the VIABLE SET may not, which is why the viable slice is a
	// parameter rather than host state.
	expandSugarAt(tok Value, pos, i int, viable []viableSig) (bool, error)
	// flowInterrupted reports whether flow control (break / continue /
	// return) was raised inside an evaluation, which abandons the walk to
	// the enclosing frame.
	flowInterrupted() bool
	// scratchParenSpan wraps items in paren markers using the host's
	// reusable span buffer — valid only until the caller splices it.
	scratchParenSpan(items []Value) []Value

	// --- classifications: pure questions against the live binding set ---

	// defTop resolves a name to its active binding, if any.
	defTop(name string) (Value, bool)
	// isFnWordBarrier reports whether tok is a bare function word acting as
	// a forward-collection barrier.
	isFnWordBarrier(tok Value) bool
	// isReachCallHead reports whether a reach-collapsed named fn at i is a
	// CALL head rather than an operand — the fn-word barrier's value twin
	// (NUR038).
	isReachCallHead(tok Value, viable []viableSig, pos, i int) bool
	// staticForwardType classifies a token by what it presents to signature
	// matching WITHOUT evaluation.
	staticForwardType(tok Value) (Value, fwdKind)
}

// The interpreter's seat, asserted at compile time: *Engine is the host and
// its *Tape the window. When the second implementation lands these gain a
// sibling; until then this is what makes "the Engine is re-seated on the
// kernel" a fact the compiler checks rather than a claim in a comment.
var (
	_ collectHost   = (*Engine)(nil)
	_ collectWindow = (*Tape)(nil)
)

// collectForward is the PHASE-1 plan walk, seated on the seam: once per
// dispatch, over the union of the viable signatures' barriers, evaluating
// the groups a still-viable overload consumes and pruning the viable set on
// what it finds. start is the first window index after the dispatching word.
//
// It is the first of the three loops to be re-seated; the per-candidate scan
// and the arrival loop follow separately, because a differential failure has
// to name one re-seat to be worth anything.
func collectForward(h collectHost, fn *FnDefInfo, w WordInfo, start int) error {
	win := h.collectWindow()
	// Forward-eligible signatures paired with their effective barrier
	// (the /s and /f modifiers override the declared BarrierPos, mirroring
	// matchSignature's forwardLimit computation).
	viable := make([]viableSig, 0, len(fn.Signatures))
	maxBarrier := 0
	for si := range fn.Signatures {
		sig := &fn.Signatures[si]
		if sig.Fallback {
			continue
		}
		barrier := effectiveForwardLimit(sig, w)
		if barrier > 0 {
			viable = append(viable, viableSig{sig, barrier})
			if barrier > maxBarrier {
				maxBarrier = barrier
			}
		}
	}
	if maxBarrier <= 0 {
		return nil
	}

	// viableConsumes reports whether any still-viable signature collects a
	// forward argument at position pos (i.e. pos is within its barrier).
	viableConsumes := func(pos int) bool {
		return viableConsumesAt(viable, pos)
	}

	// pruneViable drops every signature that a concrete forward value at
	// position pos definitely rules out (parity with matchSignature's
	// per-position rejection). Raw/Form/TypeArg slots and Any slots are
	// never used to prune (conservative — keep the signature viable);
	// a concrete Pattern on the slot prunes exactly as patternsOk would
	// reject the position (forwardPatternRejects) — fn's `tnot List`
	// triple sig must fall out of the viable set on a spec-list token,
	// or its 3-token window pre-evaluates groups past the call.
	pruneViable := func(pos int, v Value) {
		kept := viable[:0]
		for _, vs := range viable {
			keep := true
			if pos < vs.barrier && !sigRawSlot(vs.sig, pos) {
				if et := SigArgType(vs.sig, pos); !et.Equal(TAny) && !SigArgMatches(vs.sig, pos, v) {
					keep = false
				} else if forwardPatternRejects(vs.sig, pos, v) {
					keep = false
				}
			}
			if keep {
				kept = append(kept, vs)
			}
		}
		viable = kept
	}

	// scanHasKeyword: computed once so the per-token keyword prune below
	// is zero-cost for the overwhelming majority of words, which carry
	// no keyword slots.
	scanHasKeyword := sigsHaveKeywordSlot(viable)

	// prunePatterns is the PATTERN-ONLY prune for values whose TYPE must
	// not prune (a collapsed paren result — multi-value accounting — or a
	// def-bound word's binding, whose matchSignature treatment is
	// contextual). The pattern verdict is position-exact either way: the
	// value tested is what matchSignature's patternsOk will test at this
	// position, so a definite concrete-pattern rejection (the same
	// forwardPatternRejects parity pruneViable uses) is sound. Quote slots
	// are exempt — a /q position captures the word's NAME, not its
	// binding.
	prunePatterns := func(pos int, v Value) {
		kept := viable[:0]
		for _, vs := range viable {
			keep := true
			if pos < vs.barrier && !sigRawSlot(vs.sig, pos) &&
				!(vs.sig.QuoteArgs != nil && vs.sig.QuoteArgs[pos]) &&
				forwardPatternRejects(vs.sig, pos, v) {
				keep = false
			}
			if keep {
				kept = append(kept, vs)
			}
		}
		viable = kept
	}

	// pruneResolvedPatterns applies prunePatterns to a token, resolving a
	// WORD through Defs.Top first — the same resolution patternsOk applies
	// before unifying — so the pattern is tested against the binding the
	// matcher will actually see. An unbound word never prunes.
	pruneResolvedPatterns := func(pos int, tok Value) {
		if IsWord(tok) {
			if wi, werr := AsWord(tok); werr == nil {
				if top, ok := h.defTop(wi.Name); ok {
					prunePatterns(pos, top)
				}
			}
			return
		}
		prunePatterns(pos, tok)
	}

	pos := 0
	scanIdx := start
	for pos < maxBarrier && scanIdx < win.Len() {
		tok := win.At(scanIdx)

		// Boundary tokens (engine structurals, end / `)`): stop scanning.
		if scanBoundaryToken(tok) {
			break
		}

		// A sugar marker expands HERE — once per dispatch, before
		// matchSignature's per-candidate scans (which must never mutate
		// the tape per sig). A marker the expansion helper refuses is a
		// boundary; a selected-head expansion failure is the user's
		// syntax error, surfaced now.
		if IsSugar(tok) {
			expanded, serr := h.expandSugarAt(tok, pos, scanIdx, viable)
			if serr != nil {
				return serr
			}
			if !expanded {
				break
			}
			continue
		}

		// Keyword slots are decided by the raw token at their position —
		// prune before any group evaluation or word expansion below, so a
		// keyword overload's larger arity never widens the scan past the
		// dispatch the non-keyword overloads will actually make: `def g
		// (fn […]) (g 3)` must not pre-evaluate `(g 3)` before the
		// 2-arg def binds g.
		if scanHasKeyword {
			viable = pruneKeywordViable(viable, pos, tok)
		}

		// Open paren: a forward group of unknown type.
		if IsOpenParen(tok) {
			// Structure-first gate: evaluate ONLY if some still-viable
			// overload consumes a forward argument at this position.
			// Otherwise no signature wants a value here — leave the
			// paren raw so matchSignature treats it as a boundary.
			if !viableConsumes(pos) {
				break
			}
			if err := h.evalGroupAt(scanIdx); err != nil {
				return err
			}
			// Flow-control raised inside the paren: let the outer Run
			// frame resolve it (parity with the former preEvalParens).
			if h.flowInterrupted() {
				return nil
			}
			// The paren collapsed to its result value(s) at scanIdx; count
			// it as one resolved position and advance, exactly as the
			// former scan did. (The result's runtime type is not used to
			// prune further: a group can collapse to zero or many values,
			// so we keep the conservative one-slot accounting.) A concrete
			// PATTERN mismatch does prune: whatever now sits at scanIdx is
			// exactly what matchSignature will test at this sig position,
			// so a sig whose pattern definitely rejects it can never be
			// selected here — without this, a paren-spelled spec list
			// (`fn (quote [[…]]) …`) left fn's 3-token triple window open
			// and pre-evaluated the NEXT statement's groups. A WORD at
			// scanIdx (the group collapsed to zero values and the next
			// token slid in) prunes only through its resolved binding.
			// A tagged reach-collapsed named fn that WOULD CLAIM its
			// next token is a CALL head — the group resolved a callee,
			// not an operand: stop the scan (the fn-word barrier's
			// value twin, NUR038). A claim-less one is an operand, and
			// so is one filling a FUNCTION slot of the collecting word
			// (`usurp (m dot a)` — the higher-order consumer wants the
			// fn itself; Any slots stay barred).
			res := win.At(scanIdx)
			if h.isReachCallHead(res, viable, pos, scanIdx) {
				break
			}
			pruneResolvedPatterns(pos, res)
			pos++
			scanIdx++
			continue
		}

		// Interpolated template string: an expression, not a value — its
		// type is only knowable after evaluation (always a String).
		// Treated like a paren group: when a still-viable overload
		// consumes this position, evaluate it in place so a typed slot
		// (`raise` msg:String, `add`'s Scalar overload) sees the String
		// it will actually receive. Left raw, the token's internal
		// InterpString type would prune every typed signature and a
		// `raise `bad: ${x}`` mis-dispatched to the 0-arg fallback.
		if IsInterpString(tok) {
			if !viableConsumes(pos) {
				break
			}
			result, err := h.evalInterp(tok)
			if err != nil {
				return err
			}
			result.pos = tok.pos
			win.Set(scanIdx, result)
			pruneViable(pos, result)
			pos++
			scanIdx++
			continue
		}

		// Interpolated XML literal: same as InterpString — its type
		// (Node/Xml) is only knowable after evaluation, so evaluate in
		// place when a viable overload consumes this position, then prune.
		if IsXmlInterp(tok) {
			if !viableConsumes(pos) {
				break
			}
			result, err := h.evalXml(tok)
			if err != nil {
				return err
			}
			result.pos = tok.pos
			win.Set(scanIdx, result)
			pruneViable(pos, result)
			pos++
			scanIdx++
			continue
		}

		// Paren expression value (paren-nesting Step 3): expand it back to
		// its OpenParen … CloseParen marker span in place, then re-process
		// — the IsOpenParen branch above collapses it on THIS engine. See
		// design/PAREN-REPRESENTATION.9.md Step 3.
		if IsParenExpr(tok) {
			// Step 4: a quote-captured ParenExpr (already Quoted) or a
			// raw-capture forward position is left unevaluated so the
			// matched sig captures the paren as code.
			if tok.Quoted || rawParenForward(fn, pos) || rawFormForward(fn, pos) {
				pos++
				scanIdx++
				continue
			}
			peItems, _ := AsParenExpr(tok)
			win.Splice(scanIdx, 1, h.scratchParenSpan(peItems)...)
			continue
		}

		// A Reach in the forward window evaluates like a ParenExpr (Reach
		// Phase B): expand to its lowered get-chain marker span in place,
		// then re-process. Quoted/raw-capture reaches are left for the
		// matched sig (parity with the ParenExpr branch above).
		if IsReach(tok) {
			if !isEvalReach(tok) || rawParenForward(fn, pos) || rawFormForward(fn, pos) {
				pos++
				scanIdx++
				continue
			}
			// A dot-access chain is a single-value navigation, not a
			// forward-collection barrier (see the statement-branch Reach
			// hook): pre-evaluate it into the collecting word's slot exactly
			// like a paren group — uniformly, strict or not. Only a bare
			// function word that collects its own args stops the scan
			// (below). design/STRICT-FORWARD-BARRIER.0.md.
			info, _ := AsReach(tok)
			win.Splice(scanIdx, 1, expandReach(info)...)
			continue
		}

		// A word def-bound to a DATA __SP splice marker occupies its
		// forward position as the paren group (w) — the `f w ≡ f (w)`
		// equivalence: a plain (non-function) def-bound word expands into
		// the token stream wherever it stands. Rewriting the token to
		// ParenExpr([w]) and reprocessing routes it through the ParenExpr/
		// OpenParen branches above, so evaluation gating, multi-value
		// collapse, and raw-capture handling are byte-identical to a
		// written (w). Exemptions: positions no still-viable overload
		// consumes (viableConsumes — the rewrite is a TAPE MUTATION that
		// outlives this dispatch, so a word in the window of a pruned
		// overload must stay a word for the NEXT word to capture; the
		// paren/interp branches above gate the same way), structural-
		// capture slots (/q takes the word's NAME, a KEYWORD slot takes the
		// matching literal word, form/raw/type slots take the raw token —
		// see capturesForwardToken), code-bearing splices (Forth-style
		// macros that must run against the live stack — see spliceIsData),
		// and binder operands (`def y xs` rebinds the MARKER so y aliases
		// the splice — see bindsReferent).
		if IsWord(tok) && viableConsumes(pos) && !bindsReferent(fn.Name) && !capturesForwardToken(fn, pos, tok) {
			if wi, werr := AsWord(tok); werr == nil {
				if top, ok := h.defTop(wi.Name); ok && IsSplice(top) {
					if info, serr := AsSplice(top); serr == nil && spliceIsData(info) {
						pe := NewParenExpr([]Value{tok})
						pe.pos = tok.pos
						win.Set(scanIdx, pe)
						continue
					}
				}
			}
		}

		// A registered FUNCTION word in the forward window that the collecting
		// word does NOT capture is the NEXT dispatch — the runtime's "another
		// function word is a barrier" rule (commitBarrierForward) stops forward
		// collection here, and the pre-evaluation scan must stop too. The
		// former scan counted the word as one resolved forward position and
		// kept going, so a LATER group was pre-evaluated ACROSS the barrier
		// once the COLLECTING word's own max arity (maxBarrier) reached past
		// it. With a heterogeneous-arity overload — e.g. a 3-arg `add` —
		// `(g) add (g) add (g)` evaluated the third group before the first add
		// ran; the recorded events then put both later operands on the
		// simulated stack and the operand layout refused "not adjacent on
		// top". Stop so each dispatch pre-evaluates only the groups IT
		// collects, in source order. Lookup mirrors commitBarrierForward's own
		// function-word test. The capturesForwardToken guard preserves a word
		// the collecting sig takes STRUCTURALLY as an operand (a /q name like
		// `undef foo`, a raw/form/type slot, a matching KEYWORD literal like
		// def's `fn`) — there the function word is the argument, not a
		// barrier, and the scan must walk past it.
		if IsWord(tok) && !capturesForwardToken(fn, pos, tok) && h.isFnWordBarrier(tok) {
			break
		}

		// A tagged reach-collapsed named fn already in the window (a
		// re-plan after the arrival gate closed a statement) that WOULD
		// CLAIM its next token is a CALL head — the fn-word barrier's
		// value twin (NUR038): stop. A claim-less one is an operand, and
		// so is one filling a FUNCTION slot of the collecting word.
		if h.isReachCallHead(tok, viable, pos, scanIdx) {
			break
		}

		// Non-group token. A concrete literal carries a final type that
		// matchSignature tests identically, so it is sound to prune the
		// viable set on it. Words and other non-concrete tokens are left
		// un-pruned (their matchSignature treatment is contextual) but are
		// still counted as one resolved position — so, exactly like the
		// former scan, groups beyond a NON-FUNCTION word remain reachable.
		if mt, kind := h.staticForwardType(tok); kind == fwdValue {
			pruneViable(pos, mt)
		} else if IsWord(tok) {
			// A def-bound word's TYPE stays un-pruned (contextual), but
			// its concrete-PATTERN verdict is exact — patternsOk resolves
			// the word through Defs.Top the same way before unifying — so
			// a sig whose pattern rejects the binding can never be
			// selected with this word at this position. Without this, a
			// word-spelled spec list (`def sw quote [[…]]  fn sw …`) left
			// fn's 3-token triple window open past the call.
			pruneResolvedPatterns(pos, tok)
		}
		pos++
		scanIdx++
	}
	return nil
}
