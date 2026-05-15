package engine

import (
	"fmt"

	"github.com/aql-lang/aql/eng"
)

// cmp NAME [BODY]
//
// Installs a user-defined comparator on the type NAME. The body
// runs as AQL code with the two operands available as the defs
// `a` and `b`; the trailing stack value (an Integer) is the
// comparison result: negative → a < b, zero → equal, positive
// → a > b.
//
// Routes through eng.CompareValues, so `lt`/`gt`/`lte`/`gte` and
// `sort` automatically pick it up when both operands match NAME.
//
// Signature: cmp [Type/q List]. The first arg is a type — either
// a bare type-name Word (captured as an Atom by /q) OR a Type
// literal already on the stack (e.g. produced by `typeof v`).
// The handler extracts a type name from either form and looks up
// the type in the registry's type table.
//
// `cmp` works on any type registered in the registry's type table —
// `type Foo object {…}` mints a fresh *Type that values produced via
// `make Foo {…}` carry as their VType, so the lattice walk finds the
// installed comparator. Builtin (`OriginBuiltin`) types are
// off-limits to keep the kernel ordering for Integer / String / etc.
// stable.
var cmpNative = NativeFunc{
	Name:        "cmp",
	ForwardArgs: true,
	Signatures: []NativeSig{
		// Type/q: a bare type-name Word captured as Atom, or an
		// already-quoted Atom. Forward-collected.
		{
			Args:       []*Type{TAtom, TList},
			QuoteArgs:  map[int]bool{0: true},
			NoEvalArgs: map[int]bool{1: true},
			Handler:    cmpHandler,
			Returns:    []*Type{},
		},
		// Type literal directly (e.g. `cmp (typeof p) body`).
		{
			Args:       []*Type{TType, TList},
			NoEvalArgs: map[int]bool{1: true},
			Handler:    cmpHandler,
			Returns:    []*Type{},
		},
		// Object-rooted type literals (TObjectType, TRecord, etc.)
		// don't satisfy TType via the default match — pick them up
		// explicitly so `cmp (typeof person) body` works regardless
		// of which branch the type lives on.
		{
			Args:       []*Type{TAny, TList},
			NoEvalArgs: map[int]bool{1: true},
			Handler:    cmpHandler,
			Returns:    []*Type{},
		},
	},
}

func cmpHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	name, err := cmpTypeName(args[0])
	if err != nil {
		return nil, err
	}
	if !IsCapitalisedName(name) {
		return nil, fmt.Errorf("cmp %s: type names must start with a capital letter", name)
	}
	def := r.Types.LookupByName(name)
	if def == nil {
		// Fall back to the kernel builtin table. Hitting it means
		// the caller is trying to override a kernel scalar's
		// canonical Comparer (Integer / String / Boolean / Atom)
		// — refused so the kernel ordering stays stable.
		if eng.Builtin.LookupByName(name) != nil {
			return nil, fmt.Errorf("cmp %s: cannot override comparator on builtin type", name)
		}
		return nil, fmt.Errorf("cmp %s: no such type", name)
	}
	if def.Origin == eng.OriginBuiltin {
		return nil, fmt.Errorf("cmp %s: cannot override comparator on builtin type", name)
	}

	bodyVal := args[1]
	bodyList, err := eng.AsList(bodyVal)
	if err != nil {
		return nil, fmt.Errorf("cmp %s: body must be a concrete list", name)
	}
	body := append([]Value{}, bodyList.Slice()...)

	def.Behavior = &userTypeBehavior{
		prev:     def.Behavior,
		registry: r,
		body:     body,
		typeName: name,
	}
	return nil, nil
}

// cmpTypeName extracts a type-name string from the first cmp arg,
// supporting all three sig variants:
//   - Atom (from a bare-name Word via /q): use the atom's payload.
//   - Type literal (Data == nil): use the VType's leaf name.
//   - Structural type body (ObjectType etc.): use its declared name.
func cmpTypeName(v Value) (string, error) {
	if v.VType.Equal(eng.TAtom) {
		s, _ := eng.AsAtom(v)
		return s, nil
	}
	if v.VType.Equal(eng.TString) {
		s, _ := eng.AsString(v)
		return s, nil
	}
	if v.Data == nil && v.VType != nil {
		return v.VType.Leaf(), nil
	}
	if ot, err := eng.AsObjectType(v); err == nil && ot.Name != "" {
		// Object/Foo → "Foo"
		name := ot.Name
		if idx := lastSlash(name); idx >= 0 {
			name = name[idx+1:]
		}
		return name, nil
	}
	return "", fmt.Errorf("cmp: first argument must be a type name or type literal, got %s", v.VType.String())
}

func lastSlash(s string) int {
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] == '/' {
			return i
		}
	}
	return -1
}

// userTypeBehavior wraps a Type's existing TypeBehavior with a
// Compare method backed by an AQL body. Match/Format/Equal delegate
// to the previous Behavior so installing a comparator does not
// disturb the type's other semantics.
type userTypeBehavior struct {
	prev     eng.TypeBehavior
	registry *Registry
	body     []Value
	typeName string
}

func (u *userTypeBehavior) Match(v Value, t *Type) bool {
	if u.prev != nil {
		return u.prev.Match(v, t)
	}
	return eng.DefaultBehavior.Match(v, t)
}

func (u *userTypeBehavior) Format(v Value) string {
	if u.prev != nil {
		return u.prev.Format(v)
	}
	return eng.DefaultBehavior.Format(v)
}

func (u *userTypeBehavior) Equal(a, b Value) bool {
	if u.prev != nil {
		return u.prev.Equal(a, b)
	}
	return eng.DefaultBehavior.Equal(a, b)
}

// Compare runs the user-supplied body with the operands pushed as
// defs `a` and `b`, then reads the single Integer left on the stack
// as the comparison result. Returns an error from any body
// evaluation failure or a non-Integer trailing value.
func (u *userTypeBehavior) Compare(a, b Value) (int, error) {
	if u.registry == nil {
		return 0, fmt.Errorf("cmp %s: no registry attached", u.typeName)
	}
	r := u.registry

	r.Defs.Push("a", a)
	r.Defs.Push("b", b)
	defer r.Defs.Pop("a")
	defer r.Defs.Pop("b")

	tokens := append([]Value{}, u.body...)
	sub := eng.NewTop(r)
	result, err := sub.Run(tokens)
	if err != nil {
		return 0, fmt.Errorf("cmp %s: body error: %w", u.typeName, err)
	}
	if len(result) == 0 {
		return 0, fmt.Errorf("cmp %s: body produced no result", u.typeName)
	}
	top := result[len(result)-1]
	if !top.VType.Matches(eng.TInteger) {
		return 0, fmt.Errorf("cmp %s: body must return Integer, got %s", u.typeName, top.VType.String())
	}
	n, err := eng.AsInteger(top)
	if err != nil {
		return 0, fmt.Errorf("cmp %s: %w", u.typeName, err)
	}
	switch {
	case n < 0:
		return -1, nil
	case n > 0:
		return 1, nil
	default:
		return 0, nil
	}
}
