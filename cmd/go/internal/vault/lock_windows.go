//go:build windows

package vault

import (
	"os"

	"golang.org/x/sys/windows"
)

// lockFile takes an exclusive lock on the whole file via LockFileEx,
// blocking until available. Windows releases it when the handle closes
// (including on process exit), matching the flock semantics used on
// unix.
func lockFile(f *os.File) error {
	return windows.LockFileEx(windows.Handle(f.Fd()),
		windows.LOCKFILE_EXCLUSIVE_LOCK, 0, 1, 0, new(windows.Overlapped))
}

func unlockFile(f *os.File) error {
	return windows.UnlockFileEx(windows.Handle(f.Fd()), 0, 1, 0, new(windows.Overlapped))
}
