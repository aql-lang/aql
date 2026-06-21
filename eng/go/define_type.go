package eng

import "strings"

// Host-Go type definition that routes through the SAME installer the AQL
// `def Name body` word uses (InstallType — see the def handler in
// lang/go/native/native_definition.go, which calls eng.InstallType). A
// host gets every type kind the language supports — alias, nominal
// newtype (refine), value/type union (tor), negation (tnot), refined
// scalar, record, object, schema — with the full behaviour wiring
// (unifier, canon, dispatch) the AQL path installs, and the minted *Type
// back for use in native signatures. This is the construction-axis
// counterpart to MintMemberType (which covers the one case InstallType
// cannot: a membership rule expressed as a Go func rather than an AQL
// body).

// DefineType installs `name` with `body` exactly as the `def` word does,
// then returns the minted lattice node. `body` is an ordinary type body
// Value — a bare type literal (alias), a refine prefab (newtype, via
// Types.MintRefinePrefab), a disjunct (NewDisjunct — union/enum), a
// negation (NewNegation), a record/object/schema body, etc. — so the host
// builds the body with the same constructors the parser produces and the
// kernel does the rest.
//
// Like every `def`-installed type the node is a per-registry binding
// (resolvable as the bare name within that registry), carries no FixedID,
// and is absent from the builtin name index and the FixedID snapshot — it
// is owned by whoever defined it (a module sub-registry, a host plugin),
// not a kernel builtin.
func (r *Registry) DefineType(name string, body Value) (*Type, error) {
	if err := InstallType(r, name, body); err != nil {
		return nil, err
	}
	t := r.LookupTypeName(name)
	if t == nil {
		return nil, &AqlError{
			Code:   "type_error",
			Detail: "type " + name + ": installed but did not resolve",
		}
	}
	return t, nil
}

// DefineEnum installs `name` as the union of the given alternatives — the
// host-Go equivalent of `def Name (v0 tor v1 tor …)`. The alternatives
// may be concrete values (a closed enumeration, e.g. atoms) or type
// literals (a union, e.g. `Integer tor String`); membership is the same
// disjunct rule the AQL `tor` path installs. Returns the minted *Type.
func (r *Registry) DefineEnum(name string, alternatives ...Value) (*Type, error) {
	return r.DefineType(name, NewDisjunct(alternatives))
}

// DefineMemberType installs `name` as the type whose inhabitants are the
// concrete values satisfying member — a membership rule expressed as a Go
// func, the one case DefineType's AQL-body path cannot cover. It mints
// the node with the full MemberBehavior wiring AND binds the name (so it
// resolves in source and exports like a `def`-installed type), then
// returns the *Type.
//
// (MintMemberType is the lower primitive — mint only, no name binding —
// for module-scoped types reached solely through an export, like
// aql:io's StreamKind. DefineMemberType is the def-style sibling that
// also binds the name.)
func (r *Registry) DefineMemberType(name string, parent *Type, member func(v Value) bool) (*Type, error) {
	if !IsCapitalisedName(name) {
		return nil, &AqlError{
			Code:   "type_error",
			Detail: "type " + name + ": type names must start with a capital letter",
		}
	}
	def := r.Types.MintMemberType(name, parent, member)
	r.Defs.PushType(name, def, NewTypeLiteral(def))
	for _, p := range strings.Split(name, "/") {
		r.RegisterPart(p)
	}
	return def, nil
}
