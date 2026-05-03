package engine

import "fmt"

// RegisterAny registers `any`: applies `or` semantics across a list,
// short-circuiting on the first truthy element. Returns the winning
// element value (or the last falsy element). Returns `false` for an
// empty list (the identity for OR).
//
//	[1 0 2] any   → 1
//	[0 0 2] any   → 2
//	[0 0 0] any   → 0
//	[]    any   → false
func RegisterAny(r *Registry) {
	handler := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		if args[0].Data == nil {
			return []Value{NewBoolean(false)}, nil
		}
		list := args[0].AsList()
		n := list.Len()
		if n == 0 {
			return []Value{NewBoolean(false)}, nil
		}
		var last Value
		for i := 0; i < n; i++ {
			v := list.Get(i)
			if CoerceBoolean(v) {
				return []Value{v}, nil
			}
			last = v
		}
		return []Value{last}, nil
	}

	r.RegisterNativeFunc(NativeFunc{
		Name:              "any",
		ForwardPrecedence: true,
		Signatures: []NativeSig{
			{Args: []Type{TList}, Handler: handler, Returns: []Type{TAny}},
		},
	})
}

// RegisterAll registers `all`: applies `and` semantics across a list,
// short-circuiting on the first falsy element. Returns the winning
// element value (or the last truthy element). Returns `true` for an
// empty list (the identity for AND).
//
//	[1 2 3] all   → 3
//	[1 0 3] all   → 0
//	[]      all   → true
func RegisterAll(r *Registry) {
	handler := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		if args[0].Data == nil {
			return []Value{NewBoolean(true)}, nil
		}
		list := args[0].AsList()
		n := list.Len()
		if n == 0 {
			return []Value{NewBoolean(true)}, nil
		}
		var last Value
		for i := 0; i < n; i++ {
			v := list.Get(i)
			if !CoerceBoolean(v) {
				return []Value{v}, nil
			}
			last = v
		}
		return []Value{last}, nil
	}

	r.RegisterNativeFunc(NativeFunc{
		Name:              "all",
		ForwardPrecedence: true,
		Signatures: []NativeSig{
			{Args: []Type{TList}, Handler: handler, Returns: []Type{TAny}},
		},
	})
}

// RegisterTany registers `tany`: applies `tor` semantics across a
// list, building a flattened disjunct of all elements. Errors on an
// empty list. A single-element list returns that element unchanged.
//
//	[String None] tany           → String|None
//	[1 2 3] tany                 → 1|2|3
//	[(String tor None) Number] tany → String|None|Number   (flattened)
func RegisterTany(r *Registry) {
	handler := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		if args[0].Data == nil {
			return nil, fmt.Errorf("tany: expected a concrete list")
		}
		list := args[0].AsList()
		n := list.Len()
		if n == 0 {
			return nil, fmt.Errorf("tany: empty list has no alternatives")
		}
		if n == 1 {
			return []Value{list.Get(0)}, nil
		}
		var alts []Value
		for i := 0; i < n; i++ {
			v := list.Get(i)
			if v.IsDisjunct() {
				d, _ := v.AsDisjunct()
				alts = append(alts, d.Alternatives...)
			} else {
				alts = append(alts, v)
			}
		}
		// Filter Never (identity for union); collapse trivial cases.
		filtered := alts[:0]
		for _, alt := range alts {
			if !alt.VType.Equal(TNever) {
				filtered = append(filtered, alt)
			}
		}
		if len(filtered) == 0 {
			return []Value{NewTypeLiteral(TNever)}, nil
		}
		if len(filtered) == 1 {
			return []Value{filtered[0]}, nil
		}
		return []Value{NewDisjunct(filtered)}, nil
	}

	r.RegisterNativeFunc(NativeFunc{
		Name:              "tany",
		ForwardPrecedence: true,
		Signatures: []NativeSig{
			{Args: []Type{TList}, Handler: handler, Returns: []Type{TAny}},
		},
	})
}

// RegisterTall registers `tall`: applies `tand` semantics across a
// list, folding via map-merge / unify. Errors on an empty list. A
// single-element list returns that element unchanged.
//
//	[{x:1} {y:2}] tall            → {x:1, y:2}
//	[{x:1} {x:Integer y:2}] tall  → {x:1, y:2}
//	[1 Integer Number] tall       → 1
func RegisterTall(r *Registry) {
	handler := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		if args[0].Data == nil {
			return nil, fmt.Errorf("tall: expected a concrete list")
		}
		list := args[0].AsList()
		n := list.Len()
		if n == 0 {
			return nil, fmt.Errorf("tall: empty list has no values to combine")
		}
		acc := list.Get(0)
		for i := 1; i < n; i++ {
			acc = tandValues(acc, list.Get(i))
		}
		return []Value{acc}, nil
	}

	r.RegisterNativeFunc(NativeFunc{
		Name:              "tall",
		ForwardPrecedence: true,
		Signatures: []NativeSig{
			{Args: []Type{TList}, Handler: handler, Returns: []Type{TAny}},
		},
	})
}
