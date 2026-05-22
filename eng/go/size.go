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
// Path's segments, an Object's fields, a Table's rows. A number
// sizes to its floored magnitude, a string or atom to its character
// length. A type with no Sizer in its lattice — None, a Date, a
// bare scalar — sizes 0.
func SizeOf(v Value) int {
	for t := v.VType; t != nil; t = t.Parent {
		if sz, ok := t.Behavior.(Sizer); ok {
			return sz.Size(v)
		}
	}
	return 0
}

// The Size methods below attach the Sizer capability to the kernel
// Behaviors declared in compare_scalar_behaviors.go (the scalar
// branch) and coretype_list_map_behaviors.go (List / Map), plus
// idealSizeBehavior (declared here) for the Ideal family.
// Gathering them keeps the one size rule auditable in one place.

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
// length of its dominant list. Any other value that walks here has
// no size rule and sizes 0.
func (scalarCompareBehavior) Size(v Value) int {
	if p, err := AsPath(v); err == nil {
		return len(p.Parts)
	}
	return 0
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

// idealSizeBehavior carries the Sizer capability for the Ideal
// family. Installed on the Ideal root, the SizeOf walk reaches it
// for any Ideal-family instance whose own type has no Sizer. Each
// kind sizes to its member count: an Object's fields, an Array's
// elements, a Store's entries, a Table's rows. Record instances are
// field-maps and size via the Map Sizer instead.
type idealSizeBehavior struct{ defaultBehavior }

func (idealSizeBehavior) formatDelegate() {}

func (idealSizeBehavior) Size(v Value) int {
	switch d := v.Data.(type) {
	case ObjectInstanceInfo:
		if d.Fields != nil {
			return d.Fields.Len()
		}
	case *ArrayInstanceInfo:
		if d != nil {
			return len(d.Elems)
		}
	case *StoreInstanceInfo:
		if d != nil {
			return len(d.Data)
		}
	case TableData:
		return len(d.Rows)
	}
	return 0
}

func init() {
	TIdeal.Behavior = idealSizeBehavior{}
}
