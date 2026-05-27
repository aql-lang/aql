package eng

// CapPolicy is the capability slot name where the host installs a
// permissions policy. The engine consults the policy via the small
// WordChecker / GlobalChecker interfaces below — it does not import
// the lang/go/policy package.
//
// When no value is installed at this slot, the engine takes no
// policy actions (allow-everything default).
const CapPolicy = "engine.policy"

// WordChecker is the one-method interface the engine's stepWord
// uses to gate kernel-word dispatch. The lang/go/policy.Compiled
// type satisfies it; tests can supply hand-rolled implementations.
type WordChecker interface {
	CheckWord(name string) error
}

// LookupWordChecker returns the WordChecker installed on r, or nil
// if none. Used by stepWord and stepLiteral-derived dispatch sites
// before invoking a handler that resolves to a kernel-registered
// word.
//
// The lookup ignores the typed interface assertion failure path:
// anything stored under CapPolicy that does not implement
// WordChecker is treated as if no policy were installed. This
// keeps the engine permissive for hosts that store other kinds of
// values under the same slot (a deliberate but rare scenario).
func LookupWordChecker(r *Registry) WordChecker {
	if r == nil {
		return nil
	}
	v, ok, _ := r.Capabilities.Get(CapPolicy)
	if !ok || v == nil {
		return nil
	}
	wc, _ := v.(WordChecker)
	return wc
}

// isInternalMarker reports whether name is a parser/engine-internal
// marker that should bypass policy checks (the `__`-prefixed names
// used for forward-collection cleanup, def-snapshot pop, etc.).
// These names are not directly addressable from user code.
func isInternalMarker(name string) bool {
	return len(name) >= 2 && name[0] == '_' && name[1] == '_'
}
