package eng

import "strings"

// UnifyError describes a unification failure. Threaded through the
// recursive unify helpers so a failure inside a deeply-nested field
// reports the path back to the root rather than just "~unify-fail".
//
// Path uses readable, dotted segments:
//
//	field:addr.field:zip      — record/options field
//	[3]                       — list element by index
//	key:foo                   — map value by key
//	disjunct                  — disjunct fold (no alternative matched)
//
// The two values that failed to unify at the leaf are kept in A and B
// for diagnostic callers; the kernel itself uses only Reason and Path.
type UnifyError struct {
	Reason string
	Path   []string
	A, B   Value
}

// Error renders the failure as `path: reason` (or just `reason` when
// the failure is at the root).
func (e *UnifyError) Error() string {
	if e == nil {
		return ""
	}
	if len(e.Path) == 0 {
		return e.Reason
	}
	return strings.Join(e.Path, ".") + ": " + e.Reason
}

// withPath prepends seg to the error's path, returning a fresh error
// so the original (which may be referenced elsewhere) is left intact.
// Nil-safe: returns nil unchanged so callers can write
// `return Value{}, err.withPath("key:" + k)` without a guard.
func (e *UnifyError) withPath(seg string) *UnifyError {
	if e == nil {
		return nil
	}
	path := make([]string, 0, len(e.Path)+1)
	path = append(path, seg)
	path = append(path, e.Path...)
	return &UnifyError{
		Reason: e.Reason,
		Path:   path,
		A:      e.A,
		B:      e.B,
	}
}

// unifyFail constructs a leaf-level error.
func unifyFail(reason string, a, b Value) *UnifyError {
	return &UnifyError{Reason: reason, A: a, B: b}
}
