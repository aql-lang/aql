//go:build unix

package vault

import (
	"os"

	"golang.org/x/sys/unix"
)

// lockFile takes an exclusive advisory lock (flock LOCK_EX), blocking
// until it is available. The lock is tied to the open file
// description, so it releases automatically if the process exits
// without unlocking — including on a crash.
func lockFile(f *os.File) error { return unix.Flock(int(f.Fd()), unix.LOCK_EX) }

// unlockFile releases the advisory lock.
func unlockFile(f *os.File) error { return unix.Flock(int(f.Fd()), unix.LOCK_UN) }
