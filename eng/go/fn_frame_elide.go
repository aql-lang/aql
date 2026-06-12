package eng

// Shell-variant tail-call elision (design/TCO-STAGED.0.md Stage 3).
//
// When a fn-body dispatch is a direct self-recursive tail call, the
// enclosing frame's cleanup tail — already on the tape, already
// scheduled to run after the callee returns — is executed NOW instead,
// and its marker tokens are deleted. The callee then splices into the
// emptied shell exactly as it would have nested. Nothing about the
// callee's execution changes; the caller's teardown just runs before
// it instead of after, which the gate proves the callee cannot
// observe:
//
//   - the callee's args are already concrete values (auto-eval ran);
//   - captures are construction-time snapshots, reinstalled per call;
//   - the probe proved nothing pending sits below the call inside the
//     frame, so no caller code runs after the callee;
//   - the gate proved arg auto-evaluation touched no bindings, so
//     teardown-now removes exactly what teardown-later would have.
//
// The frame's SHELL — its marked open paren, its ReturnCheck, and its
// close paren — stays in place, so return checking and leftover-value
// semantics are byte-identical to nesting. What the elision buys:
// the three per-call stacks (Args, def snapshot, FnBaselines) stay
// O(1) across any self-recursive tail chain, and the parked tail
// markers (2 + 2·names tokens per call) stop accumulating. The shell
// itself (paren pair + ReturnCheck) still accretes per call — full
// frame replacement (Stage 4) removes that residue; until then the
// tape-exhaustion guard still bounds a runaway, just several times
// deeper.

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

// elideTailFrame executes the scanned frame tail eagerly — the same
// operations the parked markers would have performed, via the same
// helpers — then deletes the marker run from the tape. The frame's
// ReturnCheck (when present) and close paren are deliberately kept;
// the callee splices into the shell at the call region as usual.
//
// All tape edits here sit strictly AHEAD of the pointer (the marker
// run lies beyond the call region), so the pointer, the matched arg
// positions, and every index below them are untouched.
func (e *Engine) elideTailFrame(scan frameTailScan) error {
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
	// Delete the executed markers; keep the ReturnCheck (when
	// declared) and the frame's close paren — the shell.
	end := scan.RCIdx
	if end < 0 {
		end = scan.CloseIdx
	}
	e.tape.Splice(scan.TailStart, end-scan.TailStart)
	return nil
}
