package vault

// seams.go — package-level test seams (design/TEST-SEAMS.10.md).
//
// Each variable defaults to the real implementation and is swapped by
// tests (with t.Cleanup restore) to drive OS/stdlib failure arms and
// platform-specific dispatch that are otherwise unreachable in a unit
// test. None of these is API: they are unexported, production code
// never checks "is the seam set", and behaviour with the defaults is
// identical to calling the underlying function directly.

import (
	"crypto/cipher"
	"crypto/rand"
	"encoding/json"
	"os"

	"golang.org/x/term"
)

var (
	// randRead seams crypto/rand.Read so entropy-failure arms
	// (key/nonce/salt/id generation) are drivable.
	randRead = rand.Read

	// gcmFromBlock seams cipher.NewGCM; it cannot fail for an AES block,
	// so the error arms need the seam.
	gcmFromBlock = cipher.NewGCM

	// jsonMarshal seams the JSON encoder where a marshal-error arm
	// exists but the marshaled shape can never fail.
	jsonMarshal = json.Marshal

	// isTerminal seams term.IsTerminal for the TTY-only prompt and
	// output-guard arms.
	isTerminal = term.IsTerminal

	// openTTY seams the controlling-terminal open used when stdin is
	// redirected to the child of `vault exec`.
	openTTY = func() (*os.File, error) { return os.OpenFile("/dev/tty", os.O_RDWR, 0) }

	// writeFileAtomic's per-step seams: temp creation, chmod, write,
	// fsync, close, and the final rename. Only the temp creation and
	// rename can fail on a healthy filesystem; the rest need the seams.
	createTemp = os.CreateTemp
	fileChmod  = (*os.File).Chmod
	fileWrite  = (*os.File).Write
	fileSync   = (*os.File).Sync
	fileClose  = (*os.File).Close
	osRename   = os.Rename
)
