//go:build !linux && !darwin

package capabilities

import (
	"errors"
	"os"
)

// osStatfs is unsupported outside linux/darwin (Windows would need
// GetDiskFreeSpaceEx — deliberately deferred; the refusal is clean).
func osStatfs(path string) (FsInfo, error) {
	return FsInfo{}, &os.PathError{Op: "statfs", Path: path,
		Err: errors.New("disk information not supported on this platform")}
}
