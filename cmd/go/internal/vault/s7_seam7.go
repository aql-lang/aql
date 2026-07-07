package vault

// s7_seam7.go — additional package-level test seams for the seam7
// coverage pass (design/TEST-SEAMS.10.md). Each variable defaults to the
// real implementation and is unexported; TestS7_* tests swap it (with a
// t.Cleanup restore) to drive otherwise-unreachable OS/stdlib and crypto
// failure arms. Production code never checks "is the seam set", so
// behaviour with the defaults is identical to calling the underlying
// function directly.

import (
	"crypto/rand"
	"os"
)

var (
	// s7randRead seams crypto/rand.Read for randomID / randomToken
	// (store.go), so their entropy-failure arms are drivable.
	s7randRead = rand.Read

	// s7osRemove seams os.Remove for runVerify's stale-temp cleanup
	// (verify.go), whose remove-error arm is otherwise unreachable when the
	// tests run as root (DAC permission checks are bypassed).
	s7osRemove = os.Remove

	// s7newNDK / s7newNDKID seam the namespace-data-key and id generators
	// used by ensureSessionNamespace (session.go); both bottom out in
	// crypto/rand, so their error arms need a seam.
	s7newNDK   = newNDK
	s7newNDKID = newNDKID

	// s7vaultMasterKEK / s7slotKEK seam the scrypt/HKDF key derivations in
	// openSession (session.go). With fixed valid parameters neither can
	// fail in production, so their defensive error arms are only reachable
	// through the seam.
	s7vaultMasterKEK = vaultMasterKEK
	s7slotKEK        = slotKEK

	// s7lockFile seams the advisory flock in withVaultLock (lock.go) so the
	// lock-failure arm is drivable without an unlockable descriptor.
	s7lockFile = lockFile
)
