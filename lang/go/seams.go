package lang

import "github.com/boru-lang/boru/lang/go/native"

// newDefaultRegistryWithPolicy is a test seam (design/TEST-SEAMS.10.md):
// tests swap it (with a t.Cleanup restore) to drive New's
// registry-construction error arm, which is otherwise unreachable because
// the default registry construction cannot fail once the process has
// started.
var newDefaultRegistryWithPolicy = native.DefaultRegistryWithPolicy
