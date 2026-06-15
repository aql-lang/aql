//go:build !aqldebug

package eng

// vmFreshArgsPerCall is false in normal builds: CALL_NATIVE reuses a per-run
// scratch buffer for the args slice (the dominant per-dispatch allocation).
// The reuse is safe only under an invariant maintained in another package —
// no compiled-reachable native retains or mutates its args slice past return.
// Build with -tags aqldebug to flip this true (see vm_args_debug.go) so a
// violating handler corrupts nothing and the differential / race gates can
// localize the retention directly instead of relying on incidental coverage.
// A const lets the compiler dead-code-eliminate the unused branch, so the
// release build keeps the scratch reuse at zero overhead.
const vmFreshArgsPerCall = false
