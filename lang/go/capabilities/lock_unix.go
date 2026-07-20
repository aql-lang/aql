//go:build unix

package capabilities

import (
	"os"

	"golang.org/x/sys/unix"
)

// lock_unix.go — advisory file locks via flock(2) (the vault lock triad,
// reimplemented here so capabilities has no cmd/go dependency). The lock
// is tied to the open file description, so it releases automatically if
// the process dies — crash-safe, like the vault's.

// flockFn is the syscall seam (runner-independent branch coverage, the
// xattr/statfs-seam idiom).
var flockFn = unix.Flock

// osLockFile takes an advisory lock on f: shared (LOCK_SH) vs exclusive
// (LOCK_EX), blocking vs non-blocking (LOCK_NB). A non-blocking call on a
// held lock maps EWOULDBLOCK to ErrLockBusy.
func osLockFile(f *os.File, shared, block bool) error {
	how := unix.LOCK_EX
	if shared {
		how = unix.LOCK_SH
	}
	if !block {
		how |= unix.LOCK_NB
	}
	if err := flockFn(int(f.Fd()), how); err != nil {
		if err == unix.EWOULDBLOCK {
			return ErrLockBusy
		}
		return err
	}
	return nil
}

// osUnlockFile releases f's advisory lock.
func osUnlockFile(f *os.File) error { return flockFn(int(f.Fd()), unix.LOCK_UN) }
