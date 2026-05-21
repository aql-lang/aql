package eng

import "math"

// SizeOf reports the natural size of a value. It is a total function
// — every value has a size — so unlike CompareValues it never errors.
//
// Dispatch is type-driven: the size logic lives on the type's
// Behavior (the Sizer capability), reached by walking the parent
// chain so a descendant inherits its branch's Sizer. The kernel
// Sizers follow one rule — a value sizes to the length of the
// collection it stands for: a List's elements, a Map's keys, a
// Path's segments. A number sizes to its floored magnitude, a string
// or atom to its character length. None is the empty value and sizes
// 0; a type with no Sizer in its lattice sizes 1 (a single,
// indivisible value).
func SizeOf(v Value) int {
	if v.VType == nil || v.VType.Equal(TNone) {
		return 0
	}
	for t := v.VType; t != nil; t = t.Parent {
		if sz, ok := t.Behavior.(Sizer); ok {
			return sz.Size(v)
		}
	}
	return 1
}

// The Size methods below attach the Sizer capability to the kernel
// Behaviors declared in compare_scalar_behaviors.go (the scalar
// branch) and coretype_list_map_behaviors.go (List / Map). Gathering
// them here keeps the one size rule auditable in one place.

// Size of a Number is its floored magnitude: an Integer floors to
// itself, a Decimal drops its fraction (7.9 → 7).
func (numberCompareBehavior) Size(v Value) int {
	n, _ := AsNumber(v)
	return int(math.Floor(n))
}

// Size of a String is its length in bytes.
func (stringCompareBehavior) Size(v Value) int {
	s, _ := AsString(v)
	return len(s)
}

// Size of a Boolean is 1 for true, 0 for false.
func (booleanCompareBehavior) Size(v Value) int {
	if b, _ := AsBoolean(v); b {
		return 1
	}
	return 0
}

// Size of an Atom is the length of its name.
func (atomCompareBehavior) Size(v Value) int {
	a, _ := AsAtom(v)
	return len(a)
}

// Size on the Scalar root is reached only by Path — its branch has
// no Sizer of its own — so a Path sizes to its segment count, the
// length of its dominant list. Any other bare-Scalar value is a
// single thing.
func (scalarCompareBehavior) Size(v Value) int {
	if p, err := AsPath(v); err == nil {
		return len(p.Parts)
	}
	return 1
}

// Size of a List is its element count.
func (listFormatBehavior) Size(v Value) int {
	lst, err := RequireConcreteList(v, "size")
	if err != nil {
		return 0
	}
	return lst.Len()
}

// Size of a Map is its key count.
func (mapFormatBehavior) Size(v Value) int {
	m, err := RequireConcreteMap(v, "size")
	if err != nil {
		return 0
	}
	return m.Len()
}
