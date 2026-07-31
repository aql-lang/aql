// Seam-based coverage for the thin binary entrypoint
// (design/TEST-SEAMS.10.md): main() is a single dispatch to boru.Run,
// observed through the runCLI seam.
//
// The test swaps a package-level seam, so it must not use t.Parallel().
package main

import "testing"

func TestSeam5MainDispatchesToBoruRun(t *testing.T) {
	orig := runCLI
	t.Cleanup(func() { runCLI = orig })

	calls := 0
	runCLI = func() { calls++ }

	main()

	if calls != 1 {
		t.Fatalf("main() invoked boru.Run %d times, want exactly 1", calls)
	}
}
