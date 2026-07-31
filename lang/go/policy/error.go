package policy

import "fmt"

// Error codes attached to *Denied. These mirror the BORU error codes
// surfaced through the engine's r.BoruError machinery; the engine
// adapter copies these onto the produced BoruError when a *Denied
// bubbles up.
const (
	CodePermissionDenied       = "boru/permission_denied"
	CodeCapabilityNotInstalled = "boru/capability_not_installed"
	CodeModulesDisabled        = "boru/modules_disabled"
	CodePolicyAttenuation      = "boru/policy_attenuation"
)

// Denied is the error returned by Policy.Check / CheckGlobal /
// CheckWord on refusal. It carries the blame chain so callers can
// surface "denied by which profile, which rule, on which args."
type Denied struct {
	// Code is one of the CodeXxx constants above. Routed onto the
	// BoruError code by the engine adapter.
	Code string
	// Scope is the scope that produced the denial ("fileops",
	// "global", "modules", etc.).
	Scope string
	// Op is the operation name that was checked.
	Op string
	// Profile is the resolved profile name that owns the deciding
	// rule (post-extends).
	Profile string
	// Blame is a human-readable trail to the deciding rule. Forms:
	//   - "global.disk.write"                — denied by a global hard cap
	//   - "fileops.words rule #3"            — denied by an explicit rule
	//   - "fileops.words default=deny"       — no rule matched; default denies
	//   - "fileops.install=false"            — capability uninstalled
	//   - "modules.scopes.boru:math missing"  — module subscope absent
	Blame string
	// Args captures the request that was denied (path, host:port,
	// module name, …). Stored for tooling (boru policy explain) to
	// echo back.
	Args Args
}

// Error returns the human-readable form used by BoruError detail.
func (d *Denied) Error() string {
	switch d.Code {
	case CodeCapabilityNotInstalled:
		return fmt.Sprintf("capability %q is not installed (policy %q: %s)",
			d.Scope, d.Profile, d.Blame)
	case CodeModulesDisabled:
		return fmt.Sprintf("modules disabled by policy %q", d.Profile)
	case CodePolicyAttenuation:
		return fmt.Sprintf("attenuation error: child policy exceeds parent (%s)", d.Blame)
	default:
		return fmt.Sprintf("permission denied: %s.%s (policy %q: %s, args=%v)",
			d.Scope, d.Op, d.Profile, d.Blame, d.Args)
	}
}

// profileError wraps fmt.Errorf with a "policy:" prefix so
// schema/load errors are visually distinct from runtime denials.
func profileError(format string, a ...any) error {
	return fmt.Errorf("policy: "+format, a...)
}
