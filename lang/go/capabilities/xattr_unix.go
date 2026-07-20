//go:build linux || darwin

package capabilities

import (
	"os"
	"sort"
	"syscall"

	"golang.org/x/sys/unix"
)

// xattr_unix.go implements the OS extended-attribute quartet via x/sys
// on Linux and Darwin. Names are passed through verbatim — the WORD
// layer owns the host-namespace mapping (`user.<name>` on Linux, raw on
// Darwin), so this layer stays a thin syscall veneer. Every error is
// wrapped in an os.PathError so the io handlers and the differential
// harness classify it like any other filesystem error.

// The syscall seams: tests substitute these to drive every arm of the
// quartet deterministically — real-filesystem xattr support varies by
// runner (overlayfs/tmpfs CI mounts often refuse user xattrs), so
// branch coverage must not depend on the host mount's behaviour. The
// same seam idiom as OSFileOps.getwd; see design/TEST-SEAMS.10.md.
var (
	unixGetxattr    = unix.Getxattr
	unixSetxattr    = unix.Setxattr
	unixListxattr   = unix.Listxattr
	unixRemovexattr = unix.Removexattr
)

// osXattrGet reads one attribute with the two-call size-then-fetch
// pattern (a zero-size probe returns the value length).
func osXattrGet(path, name string) ([]byte, error) {
	size, err := unixGetxattr(path, name, nil)
	if err != nil {
		return nil, &os.PathError{Op: "xattr-get", Path: path, Err: err}
	}
	if size == 0 {
		return []byte{}, nil
	}
	buf := make([]byte, size)
	n, err := unixGetxattr(path, name, buf)
	if err != nil {
		return nil, &os.PathError{Op: "xattr-get", Path: path, Err: err}
	}
	return buf[:n], nil
}

func osXattrSet(path, name string, value []byte) error {
	if err := unixSetxattr(path, name, value, 0); err != nil {
		return &os.PathError{Op: "xattr-set", Path: path, Err: err}
	}
	return nil
}

// osXattrList returns the sorted attribute names (the syscall yields a
// NUL-separated buffer).
func osXattrList(path string) ([]string, error) {
	size, err := unixListxattr(path, nil)
	if err != nil {
		return nil, &os.PathError{Op: "xattr-list", Path: path, Err: err}
	}
	if size == 0 {
		return []string{}, nil
	}
	buf := make([]byte, size)
	n, err := unixListxattr(path, buf)
	if err != nil {
		return nil, &os.PathError{Op: "xattr-list", Path: path, Err: err}
	}
	names := []string{}
	start := 0
	for i := 0; i < n; i++ {
		if buf[i] == 0 {
			if i > start {
				names = append(names, string(buf[start:i]))
			}
			start = i + 1
		}
	}
	sort.Strings(names)
	return names, nil
}

func osXattrRemove(path, name string) error {
	if err := unixRemovexattr(path, name); err != nil {
		return &os.PathError{Op: "xattr-remove", Path: path, Err: err}
	}
	return nil
}

// osFileOwner extracts the owning uid/gid from a stat result — on unix
// os.FileInfo.Sys() carries a *syscall.Stat_t. A FileInfo whose Sys is
// something else (a synthetic test double) reports -1/-1, the shared
// "backend cannot know" sentinel.
func osFileOwner(fi os.FileInfo) (uid, gid int) {
	if st, ok := fi.Sys().(*syscall.Stat_t); ok {
		return int(st.Uid), int(st.Gid)
	}
	return -1, -1
}
