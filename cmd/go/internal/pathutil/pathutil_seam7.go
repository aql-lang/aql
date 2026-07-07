// Test seam (design/TEST-SEAMS.10.md): userHomeDir seams os.UserHomeDir
// for Expand's no-home-dir arm. On a normally-configured test host
// os.UserHomeDir succeeds, so only a seam can drive the fallback path.

package pathutil

import "os"

var userHomeDir = os.UserHomeDir
