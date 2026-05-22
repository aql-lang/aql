package eng

// TypeBehavior is the per-type operation bundle that the kernel
// consults when it needs to act on a value of a given type. Each
// *Type carries a Behavior; the well-known dispatch points
// (`v.Is(t)`, `Value.String`, `ValuesEqual`) route through it
// rather than switching on a closed enumeration of type identities.
//
// A nil Behavior on *Type means "use defaultBehavior" — the
// TypeTable installs the default on every type that doesn't supply
// its own (see TypeTable.MintType, registerBuiltin). Types provide
// a custom Behavior only when their semantics demand it: predicate
// types, dependent scalars, structural records, domain payloads
// like Date or Matrix, etc.
//
// The three required operations mirror the kernel's three fundamental
// type-system questions:
//
//   - Match: does value v satisfy this type? Used by `v.Is(t)`, the
//     signature matcher, `is` / `guard` / `typeof`, and the unifier.
//   - Format: how is v rendered as a string? Used by Value.String,
//     error messages, the canon writer, and the spec runner.
//   - Equal: are two values of this type semantically equal? Used
//     by ValuesEqual and the `eq` / `neq` words.
//
// Adding a new operation to TypeBehavior is a breaking change for
// every implementor. Optional capability sub-interfaces (Comparer,
// Hasher, Walker, Sizer — see below) let a type opt into extra
// operations without expanding the required surface.
type TypeBehavior interface {
	// Match reports whether v conforms to the type t. The
	// canonical default is a lattice walk (v.Parent is t or a
	// descendant). Predicate types override to invoke the
	// predicate body; refinement types override to check the
	// refinement clause; record types override to do field-by-
	// field conformance.
	Match(v Value, t *Type) bool

	// Format renders v as a string. The canonical default
	// delegates to Value.String (which uses the kernel's
	// existing switch). Domain types override to produce a
	// type-specific rendering (e.g. CalDuration → "P1Y2M3D").
	Format(v Value) string

	// Equal reports semantic equality. The canonical default is
	// the existing ValuesEqual deep-compare. Types with
	// normalisation semantics (CalDuration, DepScalar) override
	// to do their type-specific compare.
	Equal(a, b Value) bool
}

// Comparer is an optional capability interface. Types implementing
// it expose an ordering relation; the `sort` / `min` / `max` /
// `lt` / `gt` words consult this when ordering values of that
// type. Types lacking Comparer produce a clear "type does not
// support compare" error rather than a silent miscompile.
//
// Conventions match cmp.Compare: negative if a < b, zero if a == b,
// positive if a > b. The error return surfaces failures from
// user-defined comparator bodies (the `cmp` word installs a
// Comparer that may return errors propagated from the body); kernel
// and native Comparers return a nil error.
type Comparer interface {
	Compare(a, b Value) (int, error)
}

// Hasher is an optional capability interface. Types implementing it
// produce a stable hash for use in sets and maps keyed by Value.
type Hasher interface {
	Hash(v Value) uint64
}

// Walker is an optional capability interface. Types implementing it
// expose a traversal over contained Values (lists walk elements,
// maps walk entries, structural types walk fields).
type Walker interface {
	Walk(v Value, visit func(Value))
}

// Sizer is an optional capability interface. Types implementing it
// report a natural size — the length of a dominant collection (a
// List's elements, a Map's keys, a Path's segments, an Object's
// fields), a number's floored magnitude, a string's length. SizeOf
// consults it; a type with no Sizer in its lattice sizes to 0.
type Sizer interface {
	Size(v Value) int
}

// defaultBehavior provides the canonical Match / Format / Equal
// implementations every *Type starts with. Each delegates to the
// existing kernel paths so introducing the Behavior seam is
// observably a no-op:
//
//   - Match → v.Parent.Matches(t) (the historical lattice walk plus
//     DepScalar override).
//   - Format → Value.String() (today's full switch).
//   - Equal → ValuesEqual (today's deep-compare).
//
// Types with custom semantics override one or more of these by
// supplying their own Behavior; the default remains the fall-back
// for every type that doesn't.
type defaultBehavior struct{}

// DefaultBehavior is the canonical no-op TypeBehavior. Exported so
// callers writing custom Behaviors can embed or fall through to it.
var DefaultBehavior TypeBehavior = defaultBehavior{}

func (defaultBehavior) Match(v Value, t *Type) bool {
	return v.Parent.Matches(t)
}

func (defaultBehavior) Format(v Value) string {
	// Bypass Value.String's Behavior walk: the walk skips
	// DefaultBehavior and types tagged formatDelegatesToDefault, so
	// it would always fall through here anyway. Calling
	// kernelFormatDefault directly avoids any chance of recursion
	// via embedded-defaultBehavior Format inheritance.
	return kernelFormatDefault(v)
}

func (defaultBehavior) Equal(a, b Value) bool {
	return valuesEqualDefault(a, b)
}
