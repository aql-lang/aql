package policy

import (
	"encoding/json"
	"io"
	"os"
)

// Test seams (design/TEST-SEAMS.10.md). Each var defaults to the real
// implementation and is swapped only by tests (with a t.Cleanup restore)
// to drive OS / stdlib failure arms that are otherwise unreachable.
var (
	// readAllStdin seams the "@-" inline-policy stdin read so its
	// failure arm is drivable (and tests can feed stdin data without
	// re-plumbing os.Stdin).
	readAllStdin = func() ([]byte, error) { return io.ReadAll(os.Stdin) }
	// userHomeDir seams os.UserHomeDir for expandTilde's no-home arm.
	userHomeDir = os.UserHomeDir
	// jsonMarshal seams json.Marshal for parseProfile's canonicalisation
	// arm — jsonic parse output is always marshalable, so only a seam can
	// observe the propagation.
	jsonMarshal = json.Marshal
)
