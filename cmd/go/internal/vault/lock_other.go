//go:build !unix && !windows

package vault

import "os"

// On platforms without flock/LockFileEx (e.g. js/wasm, plan9) the lock
// is a no-op: those targets don't run concurrent aql processes against
// a shared vault, and the atomic rename still prevents torn files.
func lockFile(*os.File) error   { return nil }
func unlockFile(*os.File) error { return nil }
