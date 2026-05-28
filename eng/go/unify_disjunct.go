package eng

// disjunctUnifier is the kernel-installed Unifier for user-defined
// disjunct types — types whose body is a (`tor`-joined) disjunction
// of alternatives such as `def Maybe (Integer tor none)`.
//
// Before this Unifier existed, `def Maybe (Integer tor none)` minted
// a lattice node parented at TDisjunct but left its Behavior as the
// kernel DefaultBehavior. DefaultBehavior.Match falls back to a plain
// lattice walk: it asks `v.Parent.Matches(Maybe)`, which is false for
// every value (Integer's lattice path is Integer → Number → Scalar →
// Any, none of which is Maybe). So `42.Is(Maybe)` returned false at
// every dispatch site — fn sig matching, the `is` word, options
// fields, record fields — even though the language-level `unify` word
// (which goes through unifyDisjunct directly) accepted 42 happily.
//
// Like predicateUnifier, this lives on the lattice node so every
// "is v a Maybe?" path consults it.
type disjunctUnifier struct {
	prev         TypeBehavior // previous Behavior (delegate Format/Equal)
	alternatives []Value      // copy of the disjunct's alternatives
	typeName     string
}

// Match runs the disjunct alternatives against v via unifyDisjunct.
// The disjunct accepts v iff some alternative unifies with it. Type
// literals (Data==nil, !Carrier) pass through to the prev/DefaultBehavior
// walk — a bare Maybe-literal is "the type itself", not an inhabitant.
func (d *disjunctUnifier) Match(v Value, t *Type) bool {
	if IsBareTypeNode(v) {
		if d.prev != nil {
			return d.prev.Match(v, t)
		}
		return DefaultBehavior.Match(v, t)
	}
	_, err := unifyDisjunct(DisjunctInfo{Alternatives: d.alternatives}, v)
	return err == nil
}

func (d *disjunctUnifier) Format(v Value) string {
	if d.prev != nil {
		return d.prev.Format(v)
	}
	return DefaultBehavior.Format(v)
}

func (d *disjunctUnifier) Equal(a, b Value) bool {
	if d.prev != nil {
		return d.prev.Equal(a, b)
	}
	return DefaultBehavior.Equal(a, b)
}

// installDisjunctUnifier attaches a disjunctUnifier to def, wrapping
// any existing Behavior. Called by InstallType when minting a disjunct
// type so the alternatives drive every Is/Match call site.
func installDisjunctUnifier(def *Type, alternatives []Value, name string) {
	def.Behavior = &disjunctUnifier{
		prev:         def.Behavior,
		alternatives: alternatives,
		typeName:     name,
	}
}

// unifyDisjunct tries to unify a value against each alternative in a
// disjunct. Returns the first successful unification. For map
// alternatives, uses open (subset) matching where the candidate only
// needs to contain the alternative's key-value pairs.
//
// Asymmetric by design: disj is always the disjunct side, val is the
// other side. The top dispatcher in unify.go handles the swap.
func unifyDisjunct(disj DisjunctInfo, val Value) (Value, *UnifyError) {
	// "any" unifies with the whole disjunct, preserving it. Covers
	// two value shapes: the bare type literal NewTypeLiteral(TAny)
	// (Data=nil; the value IS the TAny lattice node) and the Any-
	// typed carrier (Data=nil, Carrier=true, Parent=TAny).
	if !IsConcrete(val) && (val.Parent.Equal(TAny) || (&val).Equal(TAny)) {
		return NewDisjunct(disj.Alternatives), nil
	}

	for _, alt := range disj.Alternatives {
		// Concrete map alternative against a concrete map value uses
		// open (subset) matching — the disjunct alternative acts as a
		// pattern, not a full schema.
		if alt.Parent.Equal(TMap) && val.Parent.Equal(TMap) &&
			!IsRecordType(alt) && !IsRecordType(val) &&
			!IsTypedMap(alt) && !IsTypedMap(val) &&
			!IsOptionsType(alt) && !IsOptionsType(val) {
			if alt.Data != nil && val.Data != nil {
				if OpenUnifyMap(alt, val) {
					return val, nil
				}
				continue
			}
		}
		if unified, err := unifyInner(alt, val); err == nil {
			return unified, nil
		}
	}
	return Value{}, unifyFail("no disjunct alternative matched", NewDisjunct(disj.Alternatives), val)
}
