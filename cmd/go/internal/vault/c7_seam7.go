package vault

// c7_seam7.go — additional package-level test seams (design/TEST-SEAMS.10.md),
// declared by the seam-7 coverage pass. Each defaults to the real
// implementation and is unexported; production behaviour with the defaults
// is identical to calling the underlying function directly. Tests swap them
// (with t.Cleanup restore) to drive OS/stdlib/store failure arms that are
// otherwise unreachable in a synchronous unit test.

import (
	"os/user"
	"path/filepath"
)

var (
	// c7userCurrent seams os/user.Current in defaultActor so the
	// "OS user unavailable" fallback ("local") is drivable in a container
	// where user.Current normally succeeds.
	c7userCurrent = user.Current

	// c7loadStore seams LoadStore at the re-load-under-lock call sites
	// (init/policy-apply/restore closures) so the "store vanished / became
	// locked / went corrupt between the pre-check and the locked re-read"
	// arms are drivable without a concurrent writer. Only the closure sites
	// are routed through it; the outer pre-checks keep the real LoadStore,
	// so a swap affects only the guarded re-load.
	c7loadStore = LoadStore

	// c7newCapability seams (*Store).NewCapability at the grant/policy call
	// sites so the entropy-failure arm (randomID/randomToken) is drivable.
	c7newCapability = (*Store).NewCapability

	// c7walkDir seams filepath.WalkDir in scanPath so the walk-error arm
	// (a directory whose read fails mid-walk) is drivable as root.
	c7walkDir = filepath.WalkDir

	// c7findJournalGeneration seams findJournalGeneration in runRestore so
	// its error arm is drivable independently of the earlier readJournal
	// (which, sharing the same file, would otherwise fail first).
	c7findJournalGeneration = findJournalGeneration

	// c7requireScope seams requireScope at export's read-scope gate. Every
	// valid scope tier (and a legacy session) satisfies OpRead, so the
	// error arm is otherwise unreachable; the seam lets a test drive it,
	// mirroring the import path's write-scope gate.
	c7requireScope = requireScope

	// c7scryptKey seams scryptKey in the export envelope crypto so the
	// key-derivation / bad-key-length arms (scryptKey err, aes.NewCipher
	// err) are drivable; with the fixed scrypt params the real call never
	// fails, so the guards need the seam.
	c7scryptKey = scryptKey

	// c7sessGetValue / c7sessDeleteValue seam the Session keyring
	// round-trip in the `vault mv` copy→verify→delete dance so its
	// consistency-failure arms (copy read/verify mismatch, stale-entry
	// delete) are drivable without a corrupt keyring.
	c7sessGetValue    = (*Session).getValue
	c7sessDeleteValue = (*Session).deleteValue
)
