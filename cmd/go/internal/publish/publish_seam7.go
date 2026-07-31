// Test seam (design/TEST-SEAMS.10.md): packRun seams pack.Run so
// tests can make it report success while pointing at a zip path that
// then fails to read — the real pack.Run always writes the path it
// prints, so the subsequent os.ReadFile(zipPath) failure arm is
// otherwise unreachable through the public Run() surface.

package publish

import (
	"io"

	"github.com/boru-lang/boru/cmd/go/internal/pack"
)

var packRun = func(args []string, stdout, stderr io.Writer) int {
	return pack.Run(args, stdout, stderr)
}
