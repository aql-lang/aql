//go:build !linux && !darwin

package capabilities

import (
	"errors"
	"os"
)

// xattr_other.go is the non-unix fallback: extended attributes are not
// supported (Windows alternate data streams and other mechanisms are
// deliberately NOT mapped — they have different semantics), and file
// ownership is unknowable, reported as the -1 sentinel.

// errXattrUnsupported classifies like ENOTSUP so real-side callers that
// probe-and-skip treat this platform exactly like an ext-mounted-noattr
// filesystem on Linux.
var errXattrUnsupported = errors.New("extended attributes not supported on this platform")

func osXattrGet(path, _ string) ([]byte, error) {
	return nil, &os.PathError{Op: "xattr-get", Path: path, Err: errXattrUnsupported}
}

func osXattrSet(path, _ string, _ []byte) error {
	return &os.PathError{Op: "xattr-set", Path: path, Err: errXattrUnsupported}
}

func osXattrList(path string) ([]string, error) {
	return nil, &os.PathError{Op: "xattr-list", Path: path, Err: errXattrUnsupported}
}

func osXattrRemove(path, _ string) error {
	return &os.PathError{Op: "xattr-remove", Path: path, Err: errXattrUnsupported}
}

// osFileOwner reports the unknowable-ownership sentinel on this platform.
func osFileOwner(_ os.FileInfo) (uid, gid int) { return -1, -1 }
