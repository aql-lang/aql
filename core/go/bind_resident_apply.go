package core

// ApplyResidentBind — the runtime half of an ARM-RESIDENT twin (§6.5's
// each-body recovery; the placement half lives in the compiler's
// OpBindResident). Where ApplyBindTwin REPLAYS a captured entry once at
// its root-stream position, a resident bind executes INSIDE a compiled
// per-invocation unit, once per invocation, with the RUNTIME value —
// because a multi-run body's ledger entry is one generalized
// carrier-valued capture that cannot represent N per-element installs
// (measured: `[10 20] each [var [[r] def x r x]]` leaks x = [20, 10]
// top-down where the ledger holds one dynamic carrier).
//
// The install arm goes through InstallDef — the interpreter's OWN
// installer — so per-element repeats stack exactly as the interpreter's
// leak does (a plain value pushes a fresh level each iteration; a
// Function-valued body runs the same overlap filter). The undef arm
// mirrors ApplyBindTwin's BindUndef: pop whatever is live, retire a
// minted node only when this binding minted it (a var param never does,
// but the arms must not drift). Neither arm records on any unwind trail
// — leak persistence is the semantics; a mid-iteration raise leaves
// earlier elements' installs in place, interpreter-identical.
func ApplyResidentBind(r *Registry, name string, undef bool, v Value) {
	if r == nil {
		return
	}
	if undef {
		if e, ok := r.Defs.PopEntry(name); ok && e.TypeDef != nil && e.Minted {
			r.Types.Retire(e.TypeDef)
		}
		return
	}
	InstallDef(r, name, v)
}
