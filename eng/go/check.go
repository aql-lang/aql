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
