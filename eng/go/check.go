package eng

// Methods on CheckState — the static-analysis state bundle defined in
// registry.go. Grouped here so the static-checker surface is one file
// rather than three: previously these lived as r.* methods spread
// across util.go and carrier.go.
//
// Call shape: callers reach the receiver via the Registry field, e.g.
// `r.Check.IsActive()`. CheckState methods have pointer receivers
// because most mutate state (Diagnostics, DefsUsed, …); r.Check is
// addressable so Go auto-takes &r.Check at the call site.

// cloneNestedSet deep-copies a name→set map so a sandbox's mutation of an
// inner set cannot bleed into the snapshot (cloneMap only copies the outer
// header, leaving the inner maps shared).
func cloneNestedSet(m map[string]map[string]bool) map[string]map[string]bool {
	if m == nil {
		return nil
	}
	cp := make(map[string]map[string]bool, len(m))
	for k, inner := range m {
		cp[k] = cloneMap(inner)
	}
	return cp
}

func cloneMap[K comparable, V any](m map[K]V) map[K]V {
	if m == nil {
		return nil
	}
	cp := make(map[K]V, len(m))
	for k, v := range m {
		cp[k] = v
	}
	return cp
}

// SpecUndefBlocked reports whether an `undef` of name inside the CURRENT
// speculative check region would pop a binding that PREDATES the region —
// the deletion the model must not commit: the region may never execute at
// run time, so leaking the pop flagged `undefined_word` on clean programs
// (the wrapped-undef FP class — an `undef` in a skipped error handler, an
// each body over an empty list, an uncalled fn body). False outside check
// mode, outside any speculative region, and for bindings pushed INSIDE the
// region (frame params, body defs — their depth exceeds the entry
// snapshot), so teardown and body-local undef are untouched.
func (r *Registry) SpecUndefBlocked(name string) bool {
	if !r.Check.IsActive() || len(r.Check.SpecBaselines) == 0 {
		return false
	}
	base := r.Check.SpecBaselines[len(r.Check.SpecBaselines)-1]
	return r.Defs.Depth(name) <= base[name]
}

// RescueForwardRefDiagnostics drops undefined_word diagnostics that
// were emitted INSIDE a fn-body analysis (FnBody tag) for names that
// have a binding by the end of the pass. A fn body runs at CALL
// time, when the whole program's defs exist — so a body reference to
// a later definition is the documented forward-reference idiom
// (recursion via forward ref, mutual recursion: lang/spec/
// recursion.tsv §3), not a defect; the install-time body analysis
// just runs too early to see it. Names still unbound at end of pass
// keep their diagnostic (a genuine typo). Top-level (non-FnBody)
// uses before a def keep theirs too — those genuinely error at run
// time.
//
// Known limitation: a top-level CALL placed before the dependent
// def (`def f fn […g…] f 1 def g …`) errors at run time but is
// rescued here — the checker doesn't order call sites against defs.
//
// Call at end of a check pass, before reading Diagnostics.
func (r *Registry) RescueForwardRefDiagnostics() {
	if r == nil || r.Check.Diagnostics == nil {
		return
	}
	kept := r.Check.Diagnostics[:0]
	for _, d := range r.Check.Diagnostics {
		if d.Code == "undefined_word" && d.FnBody && d.Word != "" {
			// Module-scope forward reference: the name has a binding by end of
			// pass (recursion, mutual recursion, a later top-level def).
			if _, bound := r.Defs.Top(d.Word); bound || r.Lookup(d.Word) != nil {
				continue
			}
			// Dynamic-scope reference: the name lives only in a per-call frame
			// (a fn parameter or a body-local def), popped before end of pass,
			// but boru's dynamic scoping makes it visible to a fn REACHED from
			// the binder's frame. Rescue iff some fn that binds the name can
			// actually reach the reading fn through the call graph — the SOUND
			// condition. A name merely bound by an unrelated fn that never
			// calls the reader (`def f fn [[] [x]] def g fn [[x:Integer] [1]] f`)
			// stays flagged: it genuinely errors at run time.
			if r.Check.dynamicScopeReachable(d.Word, d.FnName) {
				continue
			}
		}
		kept = append(kept, d)
	}
	r.Check.Diagnostics = kept
}

// (Moved from micron.go with the family split — these are generic
// check-mode helpers, not Micron content.)
//
// CheckAddUniqueDiagnostic adds a check-mode diagnostic unless an
// identical one (code+detail+position) is already recorded — ReturnsFns
// run once per analysed call shape, and a body can be analysed under
// several shapes. Every caller mirrors a GUARANTEED runtime error over
// exactly-known operands, so the diagnostic is stamped RuntimeMirror
// (the compile pipeline does not refuse on it — the recording model is
// exact) and inside an error-catching `do` body AddDiagnostic
// re-attributes it to a caught info finding. A caught (downgraded)
// entry never blocks a later REAL emission of the same finding at
// another site, so the dedupe skips it.
func CheckAddUniqueDiagnostic(r *Registry, code, detail, word string, pos SrcPos) {
	CheckAddUnique(r, CheckDiagnostic{
		Code:          code,
		Detail:        detail,
		Word:          word,
		Row:           pos.Row,
		Col:           pos.Col,
		RuntimeMirror: true,
	})
}

// CheckAddUnique is CheckAddUniqueDiagnostic's dedupe over a diagnostic the
// caller shapes itself — for a finding that must NOT be stamped
// RuntimeMirror because the compile pipeline should refuse on it. That is
// the MODEL-UNDERMINING class (eng/go/CLAUDE.md): a mirror promises the
// program compiles and then raises the identical error, which is false when
// dispatch itself did not resolve (`no_signature`, `undefined_word`,
// `uncalled_function` — there is no call to compile).
func CheckAddUnique(r *Registry, d CheckDiagnostic) {
	for _, prev := range r.Check.Diagnostics {
		if prev.Code == d.Code && prev.Detail == d.Detail &&
			prev.Row == d.Row && prev.Col == d.Col && !prev.CaughtAtRuntime {
			return
		}
	}
	r.Check.AddDiagnostic(d)
}
