package native

// Init-time type-registration errors.
//
// The native package registers several external builtin types at import
// time via package-level `var T… = registerXxxType(...)` initialisers
// (Timeout/Interval, the temporal family, the fetch family, KeyVal,
// ModuleExport/Module). A failure can only come from a programmer error
// — a duplicate FixedID or a malformed type path — never from runtime
// input. Per ADR-005 (no deliberate panics; infrastructure code always
// returns errors) these helpers record the error here instead of
// panicking, and DefaultRegistryWithPolicy surfaces it as an error when
// a registry is constructed.

var typeInitErrs []error

// recordTypeInitErr appends an init-time registration error. Safe to
// call from package-level var initialisers (single-threaded init).
func recordTypeInitErr(err error) {
	if err != nil {
		typeInitErrs = append(typeInitErrs, err)
	}
}

// SwapTypeInitErrs replaces the recorded init-error list and returns the
// previous one. It is a cross-package test seam (design/TEST-SEAMS.10.md):
// sibling packages' coverage tests exercise their registration error arms
// (a duplicate FixedID re-registration) and must snapshot/restore the
// accumulator, or the recorded error would poison every later registry
// construction in the test binary.
func SwapTypeInitErrs(errs []error) []error {
	prev := typeInitErrs
	typeInitErrs = errs
	return prev
}

// RecordTypeInitError is the exported spelling of recordTypeInitErr for
// sibling packages (lang/go/modules) that register global external
// builtins of their own (the aql:net Socket/Listener handles) and must
// funnel init-time failures through the same surfaced-at-construction
// channel.
func RecordTypeInitError(err error) { recordTypeInitErr(err) }

// TypeInitError returns the first init-time type-registration error, or
// nil. Surfaced by DefaultRegistryWithPolicy so a bad registration is
// reported at construction rather than crashing at import.
func TypeInitError() error {
	if len(typeInitErrs) == 0 {
		return nil
	}
	return typeInitErrs[0]
}
