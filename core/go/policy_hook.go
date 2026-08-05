package core

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

// ModuleCallChecker is the one-method interface the engine's
// module-export dispatch gates use — the per-export twin of
// WordChecker, looked up on the SAME CapPolicy slot. The
// lang/go/policy.Compiled type satisfies it; tests can supply
// hand-rolled implementations. A stored policy that implements only
// WordChecker leaves the module-call gates inert (allow-all), exactly
// as a WordChecker-less value leaves the word gates inert.
type ModuleCallChecker interface {
	CheckModuleCall(module, export string) error
}

// LookupModuleCallChecker returns the ModuleCallChecker installed on
// r, or nil if none — mirroring LookupWordChecker, including its
// permissive treatment of CapPolicy values that do not implement the
// interface.
func LookupModuleCallChecker(r *Registry) ModuleCallChecker {
	if r == nil {
		return nil
	}
	v, ok, _ := r.Capabilities.Get(CapPolicy)
	if !ok || v == nil {
		return nil
	}
	mc, _ := v.(ModuleCallChecker)
	return mc
}

// ModuleCallID names one module export for the per-export policy gate:
// the module's import id ("boru:time-util") and the export key the
// import published it under ("sleep") — exactly the {module, export}
// args of Check("modules","call"). A single ID is stamped per export
// at module-resolution time (StampModuleCallGates) and shared by every
// signature copy that descends from it, so the dispatch gates are one
// nil test on unstamped (non-module) signatures.
type ModuleCallID struct {
	Module string
	Export string
}

// policyGateModuleCallReg is the registry-level per-export gate every
// dispatch site shares: nil gate (a non-module signature), check mode
// (static analysis must see every dispatch), or no installed checker
// all allow; otherwise the ONE checker installed at CapPolicy decides,
// and its error is returned verbatim so both engines raise the
// identical denial.
func policyGateModuleCallReg(r *Registry, gate *ModuleCallID) error {
	if gate == nil || r == nil || r.analysisActive() {
		return nil
	}
	if mc := LookupModuleCallChecker(r); mc != nil {
		return mc.CheckModuleCall(gate.Module, gate.Export)
	}
	return nil
}

// PolicyDenied wraps a WordChecker denial raised at a VM dispatch gate
// (vmContext.gateWord), so the compiled entry points can recognise the
// error as the PROGRAM's verdict — the interpreter's policyGateWord raises
// the same checker error for the same word — rather than a foreign error
// eligible for the runtime-bail re-run (which would evaluate the program
// twice and, after an escaped effect, fence-block into a DIVERGENT
// internal_error). Error() returns the checker's text verbatim, so the two
// engines' denials render identically.
type PolicyDenied struct{ Err error }

func (p PolicyDenied) Error() string { return p.Err.Error() }

// Unwrap exposes the checker's error for errors.Is/As chains.
func (p PolicyDenied) Unwrap() error { return p.Err }

// isInternalMarker reports whether name is a parser/engine-internal
// marker that should bypass policy checks (the `__`-prefixed names
// used for forward-collection cleanup, def-snapshot pop, etc.).
// These names are not directly addressable from user code.
func IsInternalMarker(name string) bool {
	return len(name) >= 2 && name[0] == '_' && name[1] == '_'
}
