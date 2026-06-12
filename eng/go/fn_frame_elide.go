package eng

// Tail-call elimination for direct self-recursion
// (design/TCO-STAGED.0.md Stages 3 and 4a).
//
// When a fn-body dispatch is a direct self-recursive tail call, the
// enclosing frame's cleanup tail — already on the tape, already
// scheduled to run after the callee returns — is executed NOW instead.
// Nothing about the callee's execution changes; the caller's teardown
// just runs before it instead of after, which the gate proves the
// callee cannot observe:
//
//   - the callee's args are already concrete values (auto-eval ran);
//   - captures are construction-time snapshots, reinstalled per call;
//   - the probe proved nothing pending sits below the call inside the
//     frame, so no caller code runs after the callee;
//   - the gate proved arg auto-evaluation touched no bindings, so
//     teardown-now removes exactly what teardown-later would have.
//
// Two tape treatments, chosen by frame-interior cleanliness:
//
// FULL REPLACEMENT (Stage 4a, the clean case — nothing parked below
// the call): after the handler produces the callee's frame tokens,
// the caller's ENTIRE frame region — open paren through close paren,
// ReturnCheck included — is replaced by the callee's frame. Zero
// residue: tape, paren depth, and all three per-call stacks are O(1)
// across any self-recursive tail chain, so tail recursion is a real
// iteration construct and a tail runaway is CPU-bound
// (evaluation_limit), not memory-bound. Dropping the caller's
// ReturnCheck is sound precisely because the callee is the SAME
// compiled overload: its own ReturnCheck arrives with its frame and
// enforces the identical declared returns.
//
// SHELL ELISION (Stage 3, the values-below case): inert values parked
// below the call belong to the caller's result scope, so the frame's
// shell — open paren, ReturnCheck, close paren — stays in place and
// only the marker run is deleted; the callee splices into the shell at
// the call region. Leftover-value and return semantics are
// byte-identical to nesting; the shell still accretes per call, so
// the tape-exhaustion guard bounds a values-below runaway as before
// (such frames accrete real values and are not constant-space-able
// anyway).

// tcoEligible decides whether a detected tail call may be elided.
// Everything here is deny-by-default on top of the probe's own
// default-deny; a declined call simply nests, so correctness never
// depends on firing.
func (e *Engine) tcoEligible(scan frameTailScan, sig *Signature, defMutsBefore int64) bool {
	if e.registry.TCO.Disable {
		return false
	}
	// Direct self-recursion only: the dispatching overload IS the
	// overload that opened the enclosing frame. Pointer identity —
	// same compiled sig, so params, returns, and ReturnCheck are
	// identical and the kept shell checks exactly what the callee's
	// own shell would have.
	if scan.Meta == nil || scan.Meta != sig.FnFrame {
		return false
	}
	// Generic fns install per-call type-parameter bindings; their
	// teardown/Retire interaction is not yet proven under elision.
	if scan.Meta.HasGen {
		return false
	}
	// Arg auto-evaluation ran code (evaluated list/map args) between
	// collection and this dispatch. If it touched any binding, the
	// parked teardown would have sequenced around that change
	// differently than an eager one — decline.
	if e.registry.Defs.Mutations() != defMutsBefore {
		return false
	}
	// The synthesized undef tail must be replicable by UninstallDef
	// alone: a capitalised name takes undef's type-retire path, and a
	// builtin-shadowing name would ERROR at the parked token — both
	// decline so eager teardown matches the token-by-token behaviour
	// exactly.
	for _, name := range scan.UndefNames {
		if IsCapitalisedName(name) || e.registry.IsBuiltinWord(name) {
			return false
		}
	}
	return true
}

// teardownFrameState executes the scanned frame tail's REGISTRY
// effects eagerly — the same operations the parked markers would have
// performed, via the same helpers. Tape edits are the caller's
// business: the shell variant deletes just the marker run, the full
// replacement deletes the whole frame after the callee's tokens are
// in hand.
func (e *Engine) teardownFrameState(scan frameTailScan) error {
	// __DC: truncate body-local defs to the frame's entry snapshot.
	e.stepDefCleanup(e.tape.At(scan.TailStart))
	// __pa: pop the per-call Args list and FnBaseline, paired.
	if err := PopFrameArgs(e.registry); err != nil {
		return err
	}
	// The undef tail: captures+params, already in reverse install
	// order in the scan. The gate proved every name takes undef's
	// plain UninstallDef path.
	for _, name := range scan.UndefNames {
		UninstallDef(e.registry, name)
	}
	return nil
}

// elideTailFrame is the SHELL variant: run the frame tail's registry
// effects eagerly, then delete the marker run from the tape. The
// frame's ReturnCheck (when present) and close paren are deliberately
// kept; the callee splices into the shell at the call region as usual.
// Used for frames with inert values parked below the call — the shell
// keeps them, so leftover-value semantics are identical to nesting.
//
// All tape edits here sit strictly AHEAD of the pointer (the marker
// run lies beyond the call region), so the pointer, the matched arg
// positions, and every index below them are untouched.
func (e *Engine) elideTailFrame(scan frameTailScan) error {
	if err := e.teardownFrameState(scan); err != nil {
		return err
	}
	// Delete the executed markers; keep the ReturnCheck (when
	// declared) and the frame's close paren — the shell.
	end := scan.RCIdx
	if end < 0 {
		end = scan.CloseIdx
	}
	e.tape.Splice(scan.TailStart, end-scan.TailStart)
	return nil
}
