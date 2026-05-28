package eng

// bareRefineUnifier is the kernel-installed Unifier for bare nominal
// subtypes — types whose body has no added structure beyond a base
// type, e.g. `def Pos refine Integer`. Pos is a fresh lattice node
// parented at Integer; its only difference from Integer is identity.
//
// Without this Unifier, Pos kept DefaultBehavior, whose Match runs a
// plain lattice walk. For `42.Is(Pos)`, the walk asks
// `42.Parent.Matches(Pos)` — does Pos sit on Integer's ancestry chain?
// No, Pos is a *descendant* of Integer (Pos's Parent = Integer), not
// an ancestor. So `42.Is(Pos)` returned false at every dispatch site
// even though `Pos unify 42` succeeded and produced a Pos value.
//
// Semantics: a bare refinement adds no constraint, so any value whose
// declared type satisfies the base type is structurally a member of
// the refinement. The Unifier admits exactly those values.
//
// Type literals (Data==nil, !Carrier) pass through to the
// prev/DefaultBehavior walk — the type itself isn't an inhabitant.
type bareRefineUnifier struct {
	prev     TypeBehavior
	baseType *Type
	typeName string
}

func (b *bareRefineUnifier) Match(v Value, t *Type) bool {
	if v.Data == nil && !v.Carrier {
		if b.prev != nil {
			return b.prev.Match(v, t)
		}
		return DefaultBehavior.Match(v, t)
	}
	return v.Parent.Matches(b.baseType)
}

func (b *bareRefineUnifier) Format(v Value) string {
	if b.prev != nil {
		return b.prev.Format(v)
	}
	return DefaultBehavior.Format(v)
}

func (b *bareRefineUnifier) Equal(a, c Value) bool {
	if b.prev != nil {
		return b.prev.Equal(a, c)
	}
	return DefaultBehavior.Equal(a, c)
}

// installBareRefineUnifier attaches a bareRefineUnifier to def. Called
// by InstallType when minting/renaming a bare-refinement prefab so
// dispatch admits values whose declared type matches the base.
func installBareRefineUnifier(def *Type, base *Type, name string) {
	def.Behavior = &bareRefineUnifier{
		prev:     def.Behavior,
		baseType: base,
		typeName: name,
	}
}
