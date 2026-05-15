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
		{
			Args:       []*Type{TAtom, TList},
			QuoteArgs:  map[int]bool{0: true},
			NoEvalArgs: map[int]bool{1: true},
			Handler:    cmpHandler,
			Returns:    []*Type{},
		},
		{
			Args:       []*Type{TString, TList},
			NoEvalArgs: map[int]bool{1: true},
			Handler:    cmpHandler,
			Returns:    []*Type{},
		},
	},
}

func cmpHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	name := defName(args[0])
	if !IsCapitalisedName(name) {
		return nil, fmt.Errorf("cmp %s: type names must start with a capital letter", name)
	}
	def := r.Types.LookupByName(name)
	if def == nil {
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
