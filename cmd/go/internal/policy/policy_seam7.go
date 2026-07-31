// Test seam (design/TEST-SEAMS.10.md): policyLoadAuto seams
// pol.LoadAuto for testCheck's Policy resolution, so tests can inject
// a fake Policy whose Check returns a plain (non-*pol.Denied) error —
// every real Policy implementation (*pol.Compiled, the composed AND
// wrapper) only ever returns nil or *pol.Denied from Check, so the
// "reason" (non-Denied) branch of the explain output is otherwise
// unreachable.

package policy

import pol "github.com/boru-lang/boru/lang/go/policy"

var policyLoadAuto = pol.LoadAuto
