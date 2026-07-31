//go:build borudebug

package eng

// vmFreshArgsPerCall is true under -tags borudebug: every CALL_NATIVE gets a
// freshly allocated args slice instead of the reused per-run scratch buffer.
// This deliberately forgoes the hot-path allocation elision so that a native
// which (incorrectly) retains or mutates its args slice past return cannot
// corrupt a later dispatch's buffer — surfacing the bug as a clean divergence
// under the differential / race gates rather than silent, coverage-dependent
// corruption. See vm_args_release.go for the production constant.
const vmFreshArgsPerCall = true

// VMArgsDebugBuild reports whether this binary was built with -tags borudebug
// (a fresh args slice per CALL_NATIVE). True here. Exposed so allocation-
// ceiling guards can skip themselves under the debug tag, whose extra
// per-call allocations are intentional.
const VMArgsDebugBuild = true
