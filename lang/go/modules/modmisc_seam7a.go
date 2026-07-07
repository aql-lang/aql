package modules

import "github.com/aql-lang/aql/lang/go/policy"

// Test seams for wave-7A coverage (design/TEST-SEAMS.10.md). Each var
// defaults to the real implementation and is swapped only by tests (with a
// t.Cleanup restore, never in parallel) to drive an otherwise-unreachable
// error arm. Topic-prefixed to avoid collision with the shared seams.go and
// with the other coverage agent's identifiers.
var (
	// s7aPolicyLoad seams policy.Load for the vm run words' profile-load
	// error arms (Vm.run / Vm.run-sandbox / Vm.run-compute). The built-in
	// "sandbox" / "compute" profiles always compile from their embedded
	// source, so only a seam can observe the load-failure propagation.
	s7aPolicyLoad = policy.Load
)
