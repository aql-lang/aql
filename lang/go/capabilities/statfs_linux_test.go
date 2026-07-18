//go:build linux

package capabilities

import (
	"errors"
	"testing"

	"golang.org/x/sys/unix"
)

// statfs_linux_test.go drives the linux osStatfs arms through the
// syscall seam: the error arm, the known-magic name, and the
// unknown-magic hex fallback (runner filesystems vary, so the branch
// must not depend on the host mount's type).
func TestOSStatfsSeamArms(t *testing.T) {
	orig := statfsFn
	t.Cleanup(func() { statfsFn = orig })

	boom := errors.New("statfs boom")
	statfsFn = func(string, *unix.Statfs_t) error { return boom }
	if _, err := osStatfs("/p"); !errors.Is(err, boom) {
		t.Errorf("error arm: %v", err)
	}

	statfsFn = func(_ string, st *unix.Statfs_t) error {
		st.Type = 0x01021994 // tmpfs
		st.Bsize = 4096
		st.Blocks, st.Bfree, st.Bavail = 100, 60, 50
		return nil
	}
	fs, err := osStatfs("/p")
	if err != nil || fs.Type != "tmpfs" || fs.TotalBytes != 100*4096 || fs.AvailBytes != 50*4096 {
		t.Errorf("known-magic arm = %+v (%v)", fs, err)
	}

	statfsFn = func(_ string, st *unix.Statfs_t) error {
		st.Type = 0x1234
		st.Bsize = 512
		return nil
	}
	fs, err = osStatfs("/p")
	if err != nil || fs.Type != "0x1234" {
		t.Errorf("hex fallback arm = %+v (%v)", fs, err)
	}
}
