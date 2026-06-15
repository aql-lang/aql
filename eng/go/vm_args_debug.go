//go:build aqldebug

package eng

// vmFreshArgsPerCall is true under -tags aqldebug: every CALL_NATIVE gets a
// freshly allocated args slice instead of the reused per-run scratch buffer.
// This deliberately forgoes the hot-path allocation elision so that a native
// which (incorrectly) retains or mutates its args slice past return cannot
// corrupt a later dispatch's buffer — surfacing the bug as a clean divergence
// under the differential / race gates rather than silent, coverage-dependent
// corruption. See vm_args_release.go for the production constant.
const vmFreshArgsPerCall = true
