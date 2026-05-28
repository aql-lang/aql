package native

import "fmt"

// accessorNatives covers strict-access words.
//
// `getr` is the strict variant of `get`: same arg order
// ([Key, Container]) but it returns an error when the parent is None
// or the key/index is missing, instead of silently returning None.
//
// Usage:
//
//	{a:1} getr a       → 1
//	getr a {a:1}       → 1
//	{a:1} b getr       → ERROR (key not found)
//	none a getr        → ERROR (parent is none)
//	[10,20] 5 getr     → ERROR (index out of bounds)
var accessorNatives = []NativeFunc{
	{
		Name: "getr",

		Signatures: []NativeSig{
			// [Key | Node] — key forward, container from stack
			{Args: []*Type{TAtom, TNode}, QuoteArgs: map[int]bool{0: true}, BarrierPos: 1, Handler: getrMapHandler},
			{Args: []*Type{TString, TNode}, BarrierPos: 1, Handler: getrMapHandler},
			{Args: []*Type{TInteger, TNode}, BarrierPos: 1, Handler: getrMapHandler},
			// [Key | Object]
			{Args: []*Type{TAtom, TObject}, QuoteArgs: map[int]bool{0: true}, BarrierPos: 1, Handler: getrObjectHandler},
			{Args: []*Type{TString, TObject}, BarrierPos: 1, Handler: getrObjectHandler},
			{Args: []*Type{TInteger, TObject}, BarrierPos: 1, Handler: getrObjectHandler},
			// [Key | None]
			{Args: []*Type{TAny, TNone}, BarrierPos: 1, Handler: getrNoneHandler},
		},
	},
}

func getrMapHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	key := args[0]
	container := args[1]
	if !IsConcrete(container) {
		return nil, r.AqlError("getr_error", "getr: cannot access property on type literal", "getr")
	}
	// Integer key on list.
	if key.Parent.Matches(TInteger) {
		if list, _ := AsList(container); !list.IsNil() && container.Parent.Matches(TList) {
			_as3, _ := AsInteger(key)
			idx := int(_as3)
			if idx < 0 || idx >= list.Len() {
				return nil, fmt.Errorf("getr: index %d out of bounds (length %d)", idx, list.Len())
			}
			return []Value{list.Get(idx)}, nil
		}
	}
	k := getKey(key)
	m, _ := AsMap(container)
	if m == nil {
		return nil, fmt.Errorf("getr: expected a map, got %s", container.Parent.String())
	}
	val, ok := m.Get(k)
	if !ok {
		return nil, r.AqlError("getr_error", fmt.Sprintf("getr: key %q not found in map", k), "getr")
	}
	return []Value{val}, nil
}

func getrObjectHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	key := args[0]
	container := args[1]
	if !IsConcrete(container) {
		return nil, r.AqlError("getr_error", "getr: cannot access property on type literal", "getr")
	}
	k := getKey(key)
	if m, err := AsMutableMap(container); err == nil {
		val, found := m.Get(k)
		if !found {
			return nil, r.AqlError("getr_error", fmt.Sprintf("getr: key %q not found in object", k), "getr")
		}
		return []Value{val}, nil
	}
	oi, _ := AsObjectInstance(container)
	val, ok := oi.GetField(k)
	if !ok {
		return nil, r.AqlError("getr_error", fmt.Sprintf("getr: field %q not found in object", k), "getr")
	}
	return []Value{val}, nil
}

func getrNoneHandler(_ []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	return nil, r.AqlError("getr_error", "getr: parent is None", "getr")
}
