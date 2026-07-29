package native

import (
	"errors"

	"github.com/aql-lang/aql/lang/go/policy"
)

// This file is the engine adapter `lang/go/policy/error.go` has promised
// since it was written: "the engine adapter copies these onto the produced
// AqlError when a *Denied bubbles up." For a long time no such adapter
// existed, so EVERY policy refusal reached the user code-less —
//
//	aql -deny network.connect -e '… do [Net.fetch …] error [dot code]'  → None
//
// and the code is the only part of an Error a handler can dispatch on. A
// refusal that cannot be distinguished from a transport failure cannot be
// handled: `permission_denied` says widen the policy, `net_error` says the
// network is broken, and `None` says neither.
//
// The fileops half was written first (fileOpError, fileio.go) because
// REFERENCE.md's codes table is mostly about it. This is the general form,
// and the reason it is one shared adapter rather than five copies is recorded
// in design/verse-report-defects-investigation.0.md §E: the note's own
// alternative to a global `Denied.Unwrap` was "repeat fileOpError's trick at
// each remaining wrap site … but it has to be redone for every future gated
// capability." One adapter removes that objection without taking on the other
// option's risk.
//
// What this deliberately does NOT do is give `*policy.Denied` an
// `Unwrap() error` returning an AqlError. That would code every refusal
// everywhere at a stroke — and would also flip `runtimeShouldFallback`
// (lang/go/aql.go) from "foreign error, re-run on the interpreter" to "AQL
// error, surface" for every denial in compiled mode, including denials from
// sites nobody has audited. That is a semantic call about the fallback fence.
// Coding a refusal at a site whose effect has already been refused carries
// none of it: nothing was executed, so there is nothing for a re-run to
// double.
//
// THE CODE LITERALS MUST STAY AT THE ATTACHING CALL, and that is not a style
// preference. test/go/docexamples/errorcodes_test.go builds its truth set by
// extracting codes from construction sites, and it deliberately does not read
// constant declarations — policy's own four `CodeXxx` constants sat declared
// and never attached for as long as they existed, so a gate that counted
// declarations would have been vacuous exactly where it was most needed.
// A first draft of this file hoisted the two literals into a classifier that
// merely RETURNED them, and the gate immediately reported `permission_denied`
// as unmintable: true, from its point of view, and the code had just become
// invisible to the one check that keeps REFERENCE.md honest. Hence the switch
// below sits directly on top of `r.AqlError("…", …)`.

// policyRefusalCoded converts a policy refusal into a dispatchable AqlError
// carrying the code the refusal identifies itself by. ok is false when err is
// not a refusal, or is a refusal whose code no capability GATE can produce —
// in which case the caller keeps whatever error it already had.
//
// Only the two codes a gate can actually produce are mapped. `pol.Check` and
// the `Installed` guards in front of it (policy/evaluate.go) mint exactly
// `CodePermissionDenied` and `CodeCapabilityNotInstalled`; the other two
// constants cannot arrive here — nothing in the tree constructs
// `CodeModulesDisabled` at all, and `CodePolicyAttenuation` is minted by
// child-policy composition (policy/subset.go), which is not a gate. An arm
// for either would be unreachable and unprovable, and inventing a code for
// them would be worse than passing the original error through: the caller's
// own code is at least true.
//
// detail is passed in rather than taken from err because the two callers want
// different text. A file operation has already built "read: <path>: …", which
// names the thing the reader has to look at; a bare capability gate has only
// the refusal itself, whose text is policy.Denied's blame trail.
func policyRefusalCoded(r *Registry, word, detail string, err error) (error, bool) {
	var denied *policy.Denied
	if !errors.As(err, &denied) {
		return nil, false
	}
	switch denied.Code {
	case policy.CodePermissionDenied:
		return r.AqlError("permission_denied", detail, word), true
	case policy.CodeCapabilityNotInstalled:
		return r.AqlError("capability_not_installed", detail, word), true
	}
	return nil, false
}

// PolicyRefusal converts a policy refusal into a dispatchable AqlError,
// passing nil and every non-refusal error through unchanged. Call it on a
// capability gate's result so the refusal keeps its own code:
//
//	return PolicyRefusal(r, "fetch", pol.Check("network", "connect", args))
//
// It belongs INSIDE the gate function rather than at each handler that calls
// one. There are nine gate call sites across five capabilities today and the
// count only grows; wrapping at the gate means a new caller cannot forget,
// which is the failure mode that left four capabilities code-less for as long
// as the fileops half stood alone.
//
// The detail is the refusal's own text, so the blame trail policy.Denied
// builds ("network.words default=deny, args=…") still reaches the user: the
// code makes the failure dispatchable, the detail keeps it diagnosable.
func PolicyRefusal(r *Registry, word string, err error) error {
	if err == nil {
		return nil
	}
	if coded, ok := policyRefusalCoded(r, word, err.Error(), err); ok {
		return coded
	}
	return err
}
