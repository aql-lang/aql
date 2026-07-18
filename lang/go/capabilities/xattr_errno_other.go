//go:build !linux && !darwin

package capabilities

import "errors"

// errXattrAbsent is the absent-attribute error on platforms with no
// xattr syscalls — only MemFileOps raises it here (the OS quartet
// refuses with errXattrUnsupported before any attribute is consulted).
var errXattrAbsent = errors.New("attribute not found")

// XattrNamespacePrefix is prepended to plain attribute names by the io
// word layer — Linux user xattrs live in the mandatory "user." namespace,
// Darwin and the fallback platforms use flat names. Exported so the word
// layer and the parity tests share ONE mapping.
const XattrNamespacePrefix = ""
