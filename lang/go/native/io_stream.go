package native

import (
	"github.com/aql-lang/aql/eng/go"
)

// StreamKindType is the type of the three standard-stream handles
// returned by the IO words `stdin` / `stdout` / `stderr`. It is an Atom
// subtype whose inhabitants are EXACTLY the three atoms `stdin`,
// `stdout`, `stderr` — a closed enumeration the `read`/`write`
// signatures use to tell a stream target apart from a file path. The
// handles are plain Atoms (the language's purpose-built symbol type);
// StreamKindType is the refinement that admits only the three of them.
//
// It is NOT a builtin: it is MINTED into the lattice (OriginUserDef, no
// FixedID, absent from the builtin name index and the FixedID snapshot)
// and surfaced to AQL only through the `aql:io` module, as
// `IO.StreamKind` (see modules/io.go). The stable package-level pointer
// lets the read/write signature slices reference it at slice-init time
// without making it globally resolvable as a bare type name.
var StreamKindType = registerStreamKindType()

// streamSentinels maps each admitted stream atom to the internal routing
// path doRead/doWrite use to reach the host stream. The angle-bracket
// strings are no longer user-visible — they are an implementation
// detail behind the Stream atoms.
var streamSentinels = map[string]string{
	"stdin":  pathStdin,
	"stdout": pathStdout,
	"stderr": pathStderr,
}

func registerStreamKindType() *eng.Type {
	// MintTypeWithBehavior adds the node to eng.Builtin's ID index (so
	// canonicalization / serialization resolve it) but, unlike
	// RegisterExternalBuiltin, assigns no FixedID and no name/path entry
	// — it is a user-minted lattice node, reached only via the module
	// export, never as a global builtin.
	return eng.Builtin.MintTypeWithBehavior("StreamKind", eng.TAtom, streamBehavior{})
}

// streamBehavior admits exactly the three stream atoms. A concrete atom
// whose name is one of the three is a Stream regardless of whether its
// tag is the base Atom type or StreamKind itself; everything else (other
// atoms, non-atoms) falls back to the default lattice check, so a bare
// `StreamKind` type literal still answers "is this the type itself"
// while no stray value sneaks in.
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

// Unify admits a concrete stream atom against the StreamKind type, in
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
// with the StreamKind type so its static and runtime types agree (the
// `read`/`write` Stream signatures, `typeof`, and the soundness checker
// all see StreamKind). The payload is a plain AtomPayload, so every
// atom-aware path (rendering, comparison, `quote`) treats it as the
// symbol it is.
func newStreamAtom(name string) Value {
	return eng.ReparentValue(eng.NewAtom(name), StreamKindType)
}

// streamSentinel maps a stream-handle value to its internal routing
// path, reporting whether v is a stream handle at all. Used by
// extractPath / returnPath to route the Stream-typed signatures through
// the shared doRead/doWrite plumbing.
func streamSentinel(v Value) (string, bool) {
	name, err := v.AsConcreteAtom()
	if err != nil {
		return "", false
	}
	sentinel, ok := streamSentinels[name]
	return sentinel, ok
}
