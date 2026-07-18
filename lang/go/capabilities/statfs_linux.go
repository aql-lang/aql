//go:build linux

package capabilities

import (
	"os"

	"golang.org/x/sys/unix"
)

// statfsFn is the syscall seam (runner-independent branch coverage, the
// xattr-seam idiom).
var statfsFn = unix.Statfs

// fsTypeNames maps the common statfs(2) f_type magics onto names. An
// unlisted magic renders as hex — honest, and rare enough not to chase
// the kernel's full table.
var fsTypeNames = map[int64]string{
	0xEF53:     "ext4", // EXT2/3/4 share the magic
	0x01021994: "tmpfs",
	0x794C7630: "overlayfs",
	0x9123683E: "btrfs",
	0x58465342: "xfs",
	0x6969:     "nfs",
	0x4d44:     "vfat",
	0x2FC12FC1: "zfs",
	0x137F:     "minix",
	0x9660:     "iso9660",
	0x5346544E: "ntfs",
	0x62656572: "sysfs",
	0x64626720: "debugfs",
	0x27E0EB:   "cgroup",
	0x63677270: "cgroup2",
	0x73717368: "squashfs",
}

func osStatfs(path string) (FsInfo, error) {
	var st unix.Statfs_t
	if err := statfsFn(path, &st); err != nil {
		return FsInfo{}, &os.PathError{Op: "statfs", Path: path, Err: err}
	}
	bsize := st.Bsize
	name, ok := fsTypeNames[st.Type]
	if !ok {
		name = hexMagic(st.Type)
	}
	return FsInfo{
		TotalBytes: st.Blocks * uint64(bsize),
		FreeBytes:  st.Bfree * uint64(bsize),
		AvailBytes: st.Bavail * uint64(bsize),
		BlockSize:  bsize,
		Type:       name,
	}, nil
}
