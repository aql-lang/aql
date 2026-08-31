package core

import "sync/atomic"

// The dispatch-agreement PROBE — an instrument seam, not a behavior seam
// (design/FULL-COMPILATION.0.md §10's Tier-1 triggers; the admission-
// agreement census in test/go/langspec/dispatch_agreement_census_test.go
// is its client).
//
// WHY IT EXISTS. boru has two signature matchers: the interpreter's
// plan-time matcher (Engine.MatchSignature — forward collection, barrier
// splits, /q word preference) and the kernel window matcher
// (MatchSignature in signature.go — the one the VM re-runs at runtime
// and the one §6.5's OpDispatchGeneric is specified to call over the
// claimed window). Their per-position admission rules are spelled as
// separate texts, and §6.5 turns any divergence from a harmless defer
// into a wrong answer the moment the generic op EXECUTES on a match. The
// probe lets a census measure that agreement surface over real corpus
// dispatches — the repo's instrument-first idiom (the bind ledger's "a
// reading of the call graph is a hypothesis; only instrumenting it is
// evidence") — before Stage 4 designs against a guess.
//
// WHY IT IS NOT A SLOT WITH A NAMED INACTIVE DEFAULT. The seam-slot
// pattern serves permanently-installed behavior halves; this is a
// test-armed measurement on the hottest function in the interpreter
// (~22% of interpreter CPU), so the disarmed cost must be one atomic
// load and a predictable branch — no call, no alloc. The atomic is not
// optional: core runs under -race, and a censused program may fork
// engines that dispatch concurrently while the probe is armed.
//
// The probe fires at the interpreter's dispatch-COMMIT sites (the two
// MatchSignature call-site pairs, after their ForceStack retry), with
// the final outcome: sig is nil when the dispatch found no signature.
// positions are TAPE-ABSOLUTE indices in signature order (the stack fill
// records resolvedIdx entries; forward slots record their tape slots),
// so a client reads the matched operand for sig position i as
// e.Tape.At(positions[i]) and must apply its own eligibility rules for
// tokens not yet resolved at plan time (a forward Word resolves at
// arrival; the census skips those windows and counts why).
type DispatchProbe func(e *Engine, fn *FnDefInfo, w WordInfo, sig *Signature, positions []int, specAt int)

var dispatchProbe atomic.Pointer[DispatchProbe]

// InstallDispatchProbe arms the probe and returns its uninstaller. One
// probe at a time — a second install replaces the first; implementations
// must be goroutine-safe (concurrent engine forks dispatch too). Install
// around a bounded run and uninstall when done: the armed cost is a
// dynamic call per dispatch, corpus-wide that is millions of calls.
func InstallDispatchProbe(p DispatchProbe) func() {
	dispatchProbe.Store(&p)
	return func() { dispatchProbe.Store(nil) }
}

// probeDispatch fires the armed probe, if any — called from the commit
// sites in engine.go. Kept tiny so the disarmed path inlines to a load
// and a branch.
func (e *Engine) probeDispatch(fn *FnDefInfo, w WordInfo, sig *Signature, positions []int, specAt int) {
	if p := dispatchProbe.Load(); p != nil {
		(*p)(e, fn, w, sig, positions, specAt)
	}
}
