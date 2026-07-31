// Test seam (design/TEST-SEAMS.10.md): policyLoadFull seams
// policy.Load("full") for Resolve's mods-without-base fallback. The
// builtin "full" profile always compiles, so only a seam can drive
// this failure arm.

package permsflags

import "github.com/boru-lang/boru/lang/go/policy"

var policyLoadFull = func() (policy.Policy, error) { return policy.Load("full") }
