package native

import (
	"github.com/aql-lang/aql/eng/go"
)

// streamSentinels maps each admitted stream atom to the internal routing
// path doRead/doWrite use to reach the host stream. The angle-bracket
// strings are not user-visible — they are an implementation detail
// behind the Stream atoms. Stream membership is decided by NAME (these
// keys), so read/write routing is independent of which StreamKind node
// tags a handle.
var streamSentinels = map[string]string{
	"stdin":  pathStdin,
	"stdout": pathStdout,
	"stderr": pathStderr,
}

// MintStreamKind mints the StreamKind type into r's type table and
// returns the node. StreamKind is owned by the aql:io module —
// BuildIOModule mints one per import into the module's sub-registry (see
// modules/io.go) — and is deliberately NOT a global builtin: it has no
// FixedID, is absent from the builtin name index and the FixedID
// snapshot, and is reachable from AQL only through the module export
// `IO.StreamKind`. The returned node tags the stdin/stdout/stderr
// handles and backs that module instance's read/write Stream signatures.
//
// It is an Atom subtype whose inhabitants are EXACTLY the three atoms
// stdin/stdout/stderr — a closed enumeration the read/write signatures
// use to tell a stream target apart from a file path.
func MintStreamKind(r *Registry) *Type {
	return r.Types.MintTypeWithBehavior("StreamKind", eng.TAtom, streamBehavior{})
}

// NewStreamKind mints a standalone StreamKind (into its own dynamic type
// table) for test helpers that register the io words under bare names
// without a host registry to mint into. Production code mints per import
// via MintStreamKind into the module sub-registry. Membership is
// name-based, so a standalone StreamKind tags and admits the handles
// exactly like a per-import one.
func NewStreamKind() *Type {
	return eng.NewDynamicTypeTable().MintTypeWithBehavior("StreamKind", eng.TAtom, streamBehavior{})
}

// streamBehavior admits exactly the three stream atoms. A concrete atom
// whose name is one of the three is a StreamKind regardless of whether
// its tag is the base Atom type or a StreamKind node; everything else
// (other atoms, non-atoms) falls back to the default lattice check, so a
// bare `StreamKind` type literal still answers "is this the type itself"
// while no stray value sneaks in. Membership is name-based, so handles
// minted by one import are accepted by another's StreamKind too.
type streamBehavior struct{}

func (streamBehavior) Match(v Value, t *Type) bool {
	if name, err := v.AsConcreteAtom(); err == nil {
		_, ok := streamSentinels[name]
		return ok
	}
	return eng.DefaultBehavior.Match(v, t)
}

func (streamBehavior) Equal(a, b Value) bool { return eng.DefaultBehavior.Equal(a, b) }
func (streamBehavior) Format(v Value) string { return eng.DefaultBehavior.Format(v) }

// FormatDelegate marks streamBehavior as delegating Format to the kernel
// default, so canon / Value.String render a stream handle by its Atom
// family (`stdout` / `stdout/q`) rather than through Format. Without it,
// canon would route this minted (non-builtin) type's Format and drop the
// `/q` atom suffix.
func (streamBehavior) FormatDelegate() {}

// Unify admits a concrete stream atom against a StreamKind type, in
// either argument order. dispatchUnifier starts its walk at the more
// specific denoted type, so `atom is IO.StreamKind` (and `case`, return
// checks, typed containers — every Unify-based membership test) reaches
// this method. The concrete atom is the narrower side and wins; anything
// else opts out so the kernel's structural dispatch runs unchanged.
func (streamBehavior) Unify(a, b Value) (Value, *eng.UnifyError) {
	for _, v := range []Value{a, b} {
		if name, err := v.AsConcreteAtom(); err == nil {
			if _, ok := streamSentinels[name]; ok {
				return v, nil
			}
		}
	}
	return Value{}, eng.ErrNoUnifier
}

// newStreamAtom returns the Atom handle for a standard stream, tagged
// with the given StreamKind type so its static and runtime types agree
// (the read/write Stream signatures, `typeof`, and the soundness checker
// all see StreamKind). The payload is a plain AtomPayload, so every
// atom-aware path (rendering, comparison, `quote`) treats it as the
// symbol it is.
func newStreamAtom(name string, streamKind *Type) Value {
	return eng.ReparentValue(eng.NewAtom(name), streamKind)
}

// streamSentinel maps a stream-handle value to its internal routing
// path, reporting whether v is a stream handle at all. Used by
// extractPath / returnPath to route the Stream-typed signatures through
// the shared doRead/doWrite plumbing. Name-based: independent of the
// handle's StreamKind tag.
func streamSentinel(v Value) (string, bool) {
	name, err := v.AsConcreteAtom()
	if err != nil {
		return "", false
	}
	sentinel, ok := streamSentinels[name]
	return sentinel, ok
}
