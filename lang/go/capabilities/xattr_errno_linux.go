//go:build linux

package capabilities

import "golang.org/x/sys/unix"

// errXattrAbsent is the platform errno an absent extended attribute
// raises — ENODATA on Linux — shared by MemFileOps so the differential
// parity harness classifies mem and OS errors identically.
var errXattrAbsent error = unix.ENODATA

// XattrNamespacePrefix is prepended to plain attribute names by the io
// word layer — Linux user xattrs live in the mandatory "user." namespace,
// Darwin and the fallback platforms use flat names. Exported so the word
// layer and the parity tests share ONE mapping.
const XattrNamespacePrefix = "user."
