package capabilities

import "fmt"

// hexMagic renders an unrecognised filesystem magic (platform-neutral so
// it is testable on every build).
func hexMagic(magic int64) string { return fmt.Sprintf("0x%x", magic) }
