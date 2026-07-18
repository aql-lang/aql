//go:build !unix

package capabilities

import "os"

// lock_other.go — the non-unix fallback (js/wasm, plan9). Those targets
// do not run concurrent AQL processes against a shared file, so the OS
// lock is a best-effort no-op (the vault's lock_other.go posture). Not
// compiled on unix, so it carries no cover-gate obligation there.
func osLockFile(*os.File, bool, bool) error { return nil }
func osUnlockFile(*os.File) error           { return nil }
