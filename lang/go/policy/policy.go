package policy

// Effect is the per-rule decision: allow or deny.
type Effect string

const (
	EffectAllow Effect = "allow"
	EffectDeny  Effect = "deny"
)

// Args carries the runtime arguments of an operation for predicate
// matching. The keys are convention-stable per scope (e.g. "path" for
// fileops, "host"/"port" for network, "module"/"export" for modules).
type Args map[string]any

// Policy is the evaluable interface every wrapper consults. The
// concrete implementation is *Compiled, returned by Profile.Compile.
//
// All methods are safe for concurrent use.
type Policy interface {
	// Check verifies (scope, op, args). Returns nil on allow, *Denied
	// on refusal. Globals bound to the (scope, op) pair are checked
	// first; any global deny short-circuits the call.
	Check(scope, op string, args Args) error

	// CheckGlobal verifies a single hard-cap name from the global
	// enum. Used by the capability wrappers and the engine's mutate
	// guard. Returns nil on allow, *Denied on refusal.
	CheckGlobal(name string) error

	// CheckWord is the narrow shim the engine's dispatch loop calls.
	// Equivalent to Check("engine", name, nil), but exposed as a
	// dedicated method so eng/ can take it via a one-method interface.
	CheckWord(name string) error

	// Installed reports whether the given capability scope is
	// installed in this policy. Used by SetHostX hooks to skip
	// constructing a wrapper (and the underlying capability) when
	// the policy structurally disables it.
	Installed(scope string) bool

	// Limits returns the resource limits declared by the policy,
	// with package defaults filled in for unset fields.
	Limits() Limits

	// Name returns the profile name (post-extends chain), for blame
	// and audit reporting.
	Name() string
}

// Limits is the resource-budget block of a policy. Zero on a field
// means "no limit" / "package default applies".
type Limits struct {
	// TimeoutMs bounds a single Run call. Zero = no timeout.
	TimeoutMs int64 `json:"timeoutMs,omitempty"`
	// MaxStepBudget bounds engine steps. Zero = no bound.
	MaxStepBudget int64 `json:"maxStepBudget,omitempty"`
	// MaxStackDepth bounds the engine value-stack depth. Zero = no bound.
	MaxStackDepth int `json:"maxStackDepth,omitempty"`
	// MaxMemoryBytes bounds (best-effort) heap allocation. Zero = no bound.
	MaxMemoryBytes int64 `json:"maxMemoryBytes,omitempty"`
	// MaxOutputBytes bounds captured output (print, etc.). Zero = no bound.
	MaxOutputBytes int64 `json:"maxOutputBytes,omitempty"`
	// MaxSubEngineDepth bounds aql:vm nesting depth. Zero =
	// DefaultMaxSubEngineDepth (8).
	MaxSubEngineDepth int `json:"maxSubEngineDepth,omitempty"`
}

// DefaultMaxSubEngineDepth caps aql:vm.run recursion when the policy
// does not specify a value. Eight is generous: realistic nesting
// (test harness inside REPL inside service) stays under three.
const DefaultMaxSubEngineDepth = 8

// GlobalOps lists the hardcoded coarse-cap names addressed by the
// global scope. The schema rejects any other name; capability
// wrappers consult only these.
var GlobalOps = []string{
	"disk.read",
	"disk.write",
	"network",
	"process",
	"env",
	"clock",
	"system-info",
	"mutate",
}

// IsGlobalOp returns true if name is in the fixed GlobalOps enum.
func IsGlobalOp(name string) bool {
	for _, g := range GlobalOps {
		if g == name {
			return true
		}
	}
	return false
}

// KnownScopes lists the scope names the policy schema recognises.
// Profile validation rejects any others. "global" and "engine" are
// special (engine is the kernel words; global is the hard-cap enum);
// "modules" carries per-module subscopes; the rest are capabilities.
var KnownScopes = []string{
	"global",
	"engine",
	"modules",
	"fileops",
	"network",
	"sqlite",
	"formats",
	"env",
	"process",
	"clock",
}

// IsCapabilityScope returns true if scope is a host-installable
// capability (fileops, network, sqlite, formats, env, process,
// clock). These are the scopes that honour scope.install = false.
func IsCapabilityScope(scope string) bool {
	switch scope {
	case "fileops", "network", "sqlite", "formats", "env", "process", "clock":
		return true
	}
	return false
}

// WordChecker is the engine-side shim. Importing this interface
// (rather than the full Policy) lets eng/ stay independent of
// policy/ — the engine only needs to ask "is this word allowed?".
type WordChecker interface {
	CheckWord(name string) error
}

// GlobalsFor returns the global ops the (scope, op) pair touches. A
// nil/empty result means "no global cap involved" — the call is
// gated only by the capability scope itself. The table is fixed in
// code; capability authors declare their bindings here.
func GlobalsFor(scope, op string) []string {
	switch scope {
	case "fileops":
		switch op {
		case "read":
			return []string{"disk.read"}
		case "write", "mkdir":
			return []string{"disk.write"}
		}
	case "sqlite":
		switch op {
		case "open-ro", "query":
			return []string{"disk.read"}
		case "open-rw":
			return []string{"disk.read", "disk.write"}
		case "exec":
			return []string{"disk.write"}
		}
	case "network":
		return []string{"network"}
	case "process":
		return []string{"process"}
	case "env":
		return []string{"env"}
	case "clock":
		return []string{"clock"}
	}
	return nil
}
