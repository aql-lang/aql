//go:build darwin

package capabilities

import (
	"os"

	"golang.org/x/sys/unix"
)

// statfsFn is the syscall seam (runner-independent branch coverage).
var statfsFn = unix.Statfs

func osStatfs(path string) (FsInfo, error) {
	var st unix.Statfs_t
	if err := statfsFn(path, &st); err != nil {
		return FsInfo{}, &os.PathError{Op: "statfs", Path: path, Err: err}
	}
	// Darwin reports the type NAME directly (Fstypename), and sizes via
	// the fragment size (Bsize).
	name := unix.ByteSliceToString(st.Fstypename[:])
	bsize := int64(st.Bsize)
	return FsInfo{
		TotalBytes: st.Blocks * uint64(bsize),
		FreeBytes:  st.Bfree * uint64(bsize),
		AvailBytes: st.Bavail * uint64(bsize),
		BlockSize:  bsize,
		Type:       name,
	}, nil
}
