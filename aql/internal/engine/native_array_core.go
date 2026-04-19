package engine

import (
	"fmt"
	"sort"
)

// iota: [TInteger] -> [TList]
func registerIota(r *Registry) {
	r.RegisterNativeFunc(NativeFunc{
		Name:              "iota",
		ForwardPrecedence: true,
		Signatures: []NativeSig{{
			Args: []Type{TInteger},
			Handler: func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
				_as0, _ := args[0].AsInteger()
				n := int(_as0)
				if n < 0 {
					return nil, fmt.Errorf("iota: negative count %d", n)
				}
				elems := make([]Value, n)
				for i := 0; i < n; i++ {
					elems[i] = NewInteger(int64(i))
				}
				return []Value{NewList(elems)}, nil
			},
			Returns: []Type{TList},
		}},
	})
}

// shape: [TList] -> [TList]
func registerShape(r *Registry) {
	r.RegisterNativeFunc(NativeFunc{
		Name:              "shape",
		ForwardPrecedence: true,
		Signatures: []NativeSig{{
			Args: []Type{TList},
			Handler: func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
				if args[0].Data == nil {
					return nil, fmt.Errorf("shape: expected concrete list")
				}
				dims := computeShape(args[0])
				elems := make([]Value, len(dims))
				for i, d := range dims {
					elems[i] = NewInteger(int64(d))
				}
				return []Value{NewList(elems)}, nil
			},
			Returns: []Type{TList},
		}},
	})
}

func computeShape(v Value) []int {
	list := v.AsList()
	if list.IsNil() {
		return nil
	}
	dims := []int{list.Len()}
	if list.Len() == 0 {
		return dims
	}
	first := list.Get(0)
	if !first.VType.Matches(TList) || first.Data == nil {
		return dims
	}
	firstLen := first.AsList().Len()
	for i := 1; i < list.Len(); i++ {
		sub := list.Get(i)
		if !sub.VType.Matches(TList) || sub.Data == nil || sub.AsList().Len() != firstLen {
			return dims
		}
	}
	return append(dims, computeShape(first)...)
}

// rank: [TList] -> [TInteger]
func registerRank(r *Registry) {
	r.RegisterNativeFunc(NativeFunc{
		Name:              "rank",
		ForwardPrecedence: true,
		Signatures: []NativeSig{{
			Args: []Type{TList},
			Handler: func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
				if args[0].Data == nil {
					return nil, fmt.Errorf("rank: expected concrete list")
				}
				dims := computeShape(args[0])
				return []Value{NewInteger(int64(len(dims)))}, nil
			},
			Returns: []Type{TInteger},
		}},
	})
}

// length: [TList] -> [TInteger]
func registerLength(r *Registry) {
	r.RegisterNativeFunc(NativeFunc{
		Name:              "length",
		ForwardPrecedence: true,
		Signatures: []NativeSig{{
			Args: []Type{TList},
			Handler: func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
				if args[0].Data == nil {
					return nil, fmt.Errorf("length: expected concrete list")
				}
				list := args[0].AsList()
				return []Value{NewInteger(int64(list.Len()))}, nil
			},
			Returns: []Type{TInteger},
		}},
	})
}

// reshape: [TList, TList] -> [TList]
// Args[0] = shape, Args[1] = data
func registerReshape(r *Registry) {
	r.RegisterNativeFunc(NativeFunc{
		Name:              "reshape",
		ForwardPrecedence: true,
		Signatures: []NativeSig{{
			Args: []Type{TList, TList},
			Handler: func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
				if args[0].Data == nil {
					return nil, fmt.Errorf("reshape: expected concrete shape list")
				}
				if args[1].Data == nil {
					return nil, fmt.Errorf("reshape: expected concrete data list")
				}
				shapeList := args[0].AsList()
				dims := make([]int, shapeList.Len())
				for i := 0; i < shapeList.Len(); i++ {
					_as1, _ := shapeList.Get(i).AsInteger()
					dims[i] = int(_as1)
					if dims[i] < 0 {
						return nil, fmt.Errorf("reshape: negative dimension %d", dims[i])
					}
				}
				flat := flattenList(args[1])
				product := 1
				for _, d := range dims {
					product *= d
				}
				if product != len(flat) {
					return nil, fmt.Errorf("reshape: shape product %d does not match data length %d", product, len(flat))
				}
				result := buildNested(flat, dims)
				return []Value{result}, nil
			},
			Returns: []Type{TList},
		}},
	})
}

func flattenList(v Value) []Value {
	list := v.AsList()
	if list.IsNil() {
		return nil
	}
	var result []Value
	for i := 0; i < list.Len(); i++ {
		elem := list.Get(i)
		if elem.VType.Matches(TList) && elem.Data != nil {
			result = append(result, flattenList(elem)...)
		} else {
			result = append(result, elem)
		}
	}
	return result
}

func buildNested(flat []Value, dims []int) Value {
	if len(dims) == 0 {
		if len(flat) == 1 {
			return flat[0]
		}
		return NewList(flat)
	}
	if len(dims) == 1 {
		return NewList(flat)
	}
	size := dims[0]
	subSize := 1
	for _, d := range dims[1:] {
		subSize *= d
	}
	elems := make([]Value, size)
	for i := 0; i < size; i++ {
		start := i * subSize
		end := start + subSize
		elems[i] = buildNested(flat[start:end], dims[1:])
	}
	return NewList(elems)
}

// arr-flatten: [TList] -> [TList]
func registerArrFlatten(r *Registry) {
	r.RegisterNativeFunc(NativeFunc{
		Name:              "arr-flatten",
		ForwardPrecedence: true,
		Signatures: []NativeSig{{
			Args: []Type{TList},
			Handler: func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
				if args[0].Data == nil {
					return nil, fmt.Errorf("arr-flatten: expected concrete list")
				}
				flat := flattenList(args[0])
				return []Value{NewList(flat)}, nil
			},
			Returns: []Type{TList},
		}},
	})
}

// arr-transpose: [TList] -> [TList]
func registerArrTranspose(r *Registry) {
	r.RegisterNativeFunc(NativeFunc{
		Name:              "arr-transpose",
		ForwardPrecedence: true,
		Signatures: []NativeSig{{
			Args: []Type{TList},
			Handler: func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
				if args[0].Data == nil {
					return nil, fmt.Errorf("arr-transpose: expected concrete list")
				}
				outer := args[0].AsList()
				if outer.Len() == 0 {
					return []Value{NewList(nil)}, nil
				}
				first := outer.Get(0)
				if !first.VType.Matches(TList) || first.Data == nil {
					return nil, fmt.Errorf("arr-transpose: expected rank-2 list")
				}
				cols := first.AsList().Len()
				for i := 1; i < outer.Len(); i++ {
					sub := outer.Get(i)
					if !sub.VType.Matches(TList) || sub.Data == nil || sub.AsList().Len() != cols {
						return nil, fmt.Errorf("arr-transpose: expected rectangular rank-2 list")
					}
				}
				rows := outer.Len()
				result := make([]Value, cols)
				for c := 0; c < cols; c++ {
					row := make([]Value, rows)
					for r := 0; r < rows; r++ {
						row[r] = outer.Get(r).AsList().Get(c)
					}
					result[c] = NewList(row)
				}
				return []Value{NewList(result)}, nil
			},
			Returns: []Type{TList},
		}},
	})
}

// reverse: [TList] -> [TList]
func registerReverse(r *Registry) {
	r.RegisterNativeFunc(NativeFunc{
		Name:              "reverse",
		ForwardPrecedence: true,
		Signatures: []NativeSig{{
			Args: []Type{TList},
			Handler: func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
				if args[0].Data == nil {
					return nil, fmt.Errorf("reverse: expected concrete list")
				}
				list := args[0].AsList()
				n := list.Len()
				elems := make([]Value, n)
				for i := 0; i < n; i++ {
					elems[i] = list.Get(n - 1 - i)
				}
				return []Value{NewList(elems)}, nil
			},
			Returns: []Type{TList},
		}},
	})
}

// take: [TInteger, TList] -> [TList]
// Args[0] = count, Args[1] = data
func registerTake(r *Registry) {
	r.RegisterNativeFunc(NativeFunc{
		Name:              "take",
		ForwardPrecedence: true,
		Signatures: []NativeSig{{
			Args: []Type{TInteger, TList},
			Handler: func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
				if args[1].Data == nil {
					return nil, fmt.Errorf("take: expected concrete list")
				}
				_as2, _ := args[0].AsInteger()
				n := int(_as2)
				list := args[1].AsList()
				length := list.Len()
				var start, end int
				if n >= 0 {
					end = n
					if end > length {
						end = length
					}
					start = 0
				} else {
					abs := -n
					if abs > length {
						abs = length
					}
					start = length - abs
					end = length
				}
				elems := make([]Value, end-start)
				for i := start; i < end; i++ {
					elems[i-start] = list.Get(i)
				}
				return []Value{NewList(elems)}, nil
			},
			Returns: []Type{TList},
		}},
	})
}

// shed: [TInteger, TList] -> [TList]
// Args[0] = count, Args[1] = data
func registerShed(r *Registry) {
	r.RegisterNativeFunc(NativeFunc{
		Name:              "shed",
		ForwardPrecedence: true,
		Signatures: []NativeSig{{
			Args: []Type{TInteger, TList},
			Handler: func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
				if args[1].Data == nil {
					return nil, fmt.Errorf("shed: expected concrete list")
				}
				_as3, _ := args[0].AsInteger()
				n := int(_as3)
				list := args[1].AsList()
				length := list.Len()
				var start, end int
				if n >= 0 {
					start = n
					if start > length {
						start = length
					}
					end = length
				} else {
					abs := -n
					if abs > length {
						abs = length
					}
					start = 0
					end = length - abs
				}
				elems := make([]Value, end-start)
				for i := start; i < end; i++ {
					elems[i-start] = list.Get(i)
				}
				return []Value{NewList(elems)}, nil
			},
			Returns: []Type{TList},
		}},
	})
}

// where: [TList] -> [TList]
func registerWhere(r *Registry) {
	r.RegisterNativeFunc(NativeFunc{
		Name:              "where",
		ForwardPrecedence: true,
		Signatures: []NativeSig{{
			Args: []Type{TList},
			Handler: func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
				if args[0].Data == nil {
					return nil, fmt.Errorf("where: expected concrete list")
				}
				list := args[0].AsList()
				var result []Value
				for i := 0; i < list.Len(); i++ {
					elem := list.Get(i)
					if isTruthy(elem) {
						result = append(result, NewInteger(int64(i)))
					}
				}
				if result == nil {
					result = []Value{}
				}
				return []Value{NewList(result)}, nil
			},
			Returns: []Type{TList},
		}},
	})
}

// unique: [TList] -> [TList]
func registerUnique(r *Registry) {
	r.RegisterNativeFunc(NativeFunc{
		Name:              "unique",
		ForwardPrecedence: true,
		Signatures: []NativeSig{{
			Args: []Type{TList},
			Handler: func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
				if args[0].Data == nil {
					return nil, fmt.Errorf("unique: expected concrete list")
				}
				list := args[0].AsList()
				seen := make(map[string]bool)
				var result []Value
				for i := 0; i < list.Len(); i++ {
					elem := list.Get(i)
					key := elem.String()
					if !seen[key] {
						seen[key] = true
						result = append(result, elem)
					}
				}
				if result == nil {
					result = []Value{}
				}
				return []Value{NewList(result)}, nil
			},
			Returns: []Type{TList},
		}},
	})
}

// grade: [TList] -> [TList]
func registerGrade(r *Registry) {
	r.RegisterNativeFunc(NativeFunc{
		Name:              "grade",
		ForwardPrecedence: true,
		Signatures: []NativeSig{{
			Args: []Type{TList},
			Handler: func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
				if args[0].Data == nil {
					return nil, fmt.Errorf("grade: expected concrete list")
				}
				list := args[0].AsList()
				n := list.Len()
				indices := make([]int, n)
				for i := 0; i < n; i++ {
					indices[i] = i
				}
				sort.Slice(indices, func(a, b int) bool {
					va := list.Get(indices[a])
					vb := list.Get(indices[b])
					return arrCompareValues(va, vb) < 0
				})
				elems := make([]Value, n)
				for i, idx := range indices {
					elems[i] = NewInteger(int64(idx))
				}
				return []Value{NewList(elems)}, nil
			},
			Returns: []Type{TList},
		}},
	})
}

// arrCompareValues is a non-error variant of compareValues for use in sort functions.
// Falls back to string comparison if compareValues returns an error.
func arrCompareValues(a, b Value) int {
	cmp, err := compareValues(a, b)
	if err != nil {
		// Fallback: compare string representations
		as, bs := a.String(), b.String()
		if as < bs {
			return -1
		}
		if as > bs {
			return 1
		}
		return 0
	}
	return cmp
}

// at: [TList, TList] -> [TList]
// Args[0] = indices, Args[1] = data
func registerAt(r *Registry) {
	r.RegisterNativeFunc(NativeFunc{
		Name:              "at",
		ForwardPrecedence: true,
		Signatures: []NativeSig{{
			Args: []Type{TList, TList},
			Handler: func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
				if args[0].Data == nil {
					return nil, fmt.Errorf("at: expected concrete indices list")
				}
				if args[1].Data == nil {
					return nil, fmt.Errorf("at: expected concrete data list")
				}
				indices := args[0].AsList()
				data := args[1].AsList()
				dataLen := data.Len()
				result := make([]Value, indices.Len())
				for i := 0; i < indices.Len(); i++ {
					_as4, _ := indices.Get(i).AsInteger()
					idx := int(_as4)
					if idx < 0 || idx >= dataLen {
						return nil, fmt.Errorf("at: index %d out of bounds (length %d)", idx, dataLen)
					}
					result[i] = data.Get(idx)
				}
				return []Value{NewList(result)}, nil
			},
			Returns: []Type{TList},
		}},
	})
}

// sortby: [TList, TList] -> [TList]
// Args[0] = keys, Args[1] = data
func registerSortby(r *Registry) {
	r.RegisterNativeFunc(NativeFunc{
		Name:              "sortby",
		ForwardPrecedence: true,
		Signatures: []NativeSig{{
			Args: []Type{TList, TList},
			Handler: func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
				if args[0].Data == nil {
					return nil, fmt.Errorf("sortby: expected concrete keys list")
				}
				if args[1].Data == nil {
					return nil, fmt.Errorf("sortby: expected concrete data list")
				}
				keys := args[0].AsList()
				data := args[1].AsList()
				if keys.Len() != data.Len() {
					return nil, fmt.Errorf("sortby: keys length %d does not match data length %d", keys.Len(), data.Len())
				}
				n := keys.Len()
				indices := make([]int, n)
				for i := 0; i < n; i++ {
					indices[i] = i
				}
				sort.Slice(indices, func(a, b int) bool {
					return arrCompareValues(keys.Get(indices[a]), keys.Get(indices[b])) < 0
				})
				result := make([]Value, n)
				for i, idx := range indices {
					result[i] = data.Get(idx)
				}
				return []Value{NewList(result)}, nil
			},
			Returns: []Type{TList},
		}},
	})
}

// member: [TList, TList] -> [TList]
// Args[0] = needles, Args[1] = haystack
func registerMember(r *Registry) {
	r.RegisterNativeFunc(NativeFunc{
		Name:              "member",
		ForwardPrecedence: true,
		Signatures: []NativeSig{{
			Args: []Type{TList, TList},
			Handler: func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
				if args[0].Data == nil {
					return nil, fmt.Errorf("member: expected concrete needles list")
				}
				if args[1].Data == nil {
					return nil, fmt.Errorf("member: expected concrete haystack list")
				}
				needles := args[0].AsList()
				haystack := args[1].AsList()
				haystackSet := make(map[string]bool, haystack.Len())
				for i := 0; i < haystack.Len(); i++ {
					haystackSet[haystack.Get(i).String()] = true
				}
				result := make([]Value, needles.Len())
				for i := 0; i < needles.Len(); i++ {
					result[i] = NewBoolean(haystackSet[needles.Get(i).String()])
				}
				return []Value{NewList(result)}, nil
			},
			Returns: []Type{TList},
		}},
	})
}

// arr-indexof: [TList, TList] -> [TList]
// Args[0] = needles, Args[1] = haystack
func registerArrIndexof(r *Registry) {
	r.RegisterNativeFunc(NativeFunc{
		Name:              "arr-indexof",
		ForwardPrecedence: true,
		Signatures: []NativeSig{{
			Args: []Type{TList, TList},
			Handler: func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
				if args[0].Data == nil {
					return nil, fmt.Errorf("arr-indexof: expected concrete needles list")
				}
				if args[1].Data == nil {
					return nil, fmt.Errorf("arr-indexof: expected concrete haystack list")
				}
				needles := args[0].AsList()
				haystack := args[1].AsList()
				haystackLen := haystack.Len()
				indexMap := make(map[string]int, haystackLen)
				for i := 0; i < haystackLen; i++ {
					key := haystack.Get(i).String()
					if _, exists := indexMap[key]; !exists {
						indexMap[key] = i
					}
				}
				result := make([]Value, needles.Len())
				for i := 0; i < needles.Len(); i++ {
					key := needles.Get(i).String()
					if idx, exists := indexMap[key]; exists {
						result[i] = NewInteger(int64(idx))
					} else {
						result[i] = NewInteger(int64(haystackLen))
					}
				}
				return []Value{NewList(result)}, nil
			},
			Returns: []Type{TList},
		}},
	})
}

// group: two signatures
// [TList, TList] -> [TMap] — group values by keys
// [TList] -> [TMap] — group by value, returning map of value to list of indices
func registerGroup(r *Registry) {
	r.RegisterNativeFunc(NativeFunc{
		Name:              "group",
		ForwardPrecedence: true,
		Signatures: []NativeSig{
			{
				// Two-arg: Args[0] = keys, Args[1] = values
				Args: []Type{TList, TList},
				Handler: func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
					if args[0].Data == nil {
						return nil, fmt.Errorf("group: expected concrete keys list")
					}
					if args[1].Data == nil {
						return nil, fmt.Errorf("group: expected concrete values list")
					}
					keys := args[0].AsList()
					values := args[1].AsList()
					if keys.Len() != values.Len() {
						return nil, fmt.Errorf("group: keys length %d does not match values length %d", keys.Len(), values.Len())
					}
					om := NewOrderedMap()
					groups := make(map[string][]Value)
					order := make([]string, 0)
					for i := 0; i < keys.Len(); i++ {
						k := keys.Get(i).String()
						if _, exists := groups[k]; !exists {
							order = append(order, k)
						}
						groups[k] = append(groups[k], values.Get(i))
					}
					for _, k := range order {
						om.Set(k, NewList(groups[k]))
					}
					return []Value{NewMap(om)}, nil
				},
				Returns: []Type{TMap},
			},
			{
				// Single-arg: group by value, return map of value -> list of indices
				Args: []Type{TList},
				Handler: func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
					if args[0].Data == nil {
						return nil, fmt.Errorf("group: expected concrete list")
					}
					list := args[0].AsList()
					om := NewOrderedMap()
					groups := make(map[string][]Value)
					order := make([]string, 0)
					for i := 0; i < list.Len(); i++ {
						k := list.Get(i).String()
						if _, exists := groups[k]; !exists {
							order = append(order, k)
						}
						groups[k] = append(groups[k], NewInteger(int64(i)))
					}
					for _, k := range order {
						om.Set(k, NewList(groups[k]))
					}
					return []Value{NewMap(om)}, nil
				},
				Returns: []Type{TMap},
			},
		},
	})
}

// replicate: [TList, TList] -> [TList]
// Args[0] = counts, Args[1] = data
func registerReplicate(r *Registry) {
	r.RegisterNativeFunc(NativeFunc{
		Name:              "replicate",
		ForwardPrecedence: true,
		Signatures: []NativeSig{{
			Args: []Type{TList, TList},
			Handler: func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
				if args[0].Data == nil {
					return nil, fmt.Errorf("replicate: expected concrete counts list")
				}
				if args[1].Data == nil {
					return nil, fmt.Errorf("replicate: expected concrete data list")
				}
				counts := args[0].AsList()
				data := args[1].AsList()
				if counts.Len() != data.Len() {
					return nil, fmt.Errorf("replicate: counts length %d does not match data length %d", counts.Len(), data.Len())
				}
				var result []Value
				for i := 0; i < counts.Len(); i++ {
					_as5, _ := counts.Get(i).AsInteger()
					c := int(_as5)
					if c < 0 {
						return nil, fmt.Errorf("replicate: negative count %d at index %d", c, i)
					}
					elem := data.Get(i)
					for j := 0; j < c; j++ {
						result = append(result, elem)
					}
				}
				if result == nil {
					result = []Value{}
				}
				return []Value{NewList(result)}, nil
			},
			Returns: []Type{TList},
		}},
	})
}

// expand: [TList, TList] -> [TList]
// Args[0] = mask (booleans), Args[1] = data
func registerExpand(r *Registry) {
	r.RegisterNativeFunc(NativeFunc{
		Name:              "expand",
		ForwardPrecedence: true,
		Signatures: []NativeSig{{
			Args: []Type{TList, TList},
			Handler: func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
				if args[0].Data == nil {
					return nil, fmt.Errorf("expand: expected concrete mask list")
				}
				if args[1].Data == nil {
					return nil, fmt.Errorf("expand: expected concrete data list")
				}
				mask := args[0].AsList()
				data := args[1].AsList()
				result := make([]Value, mask.Len())
				dataIdx := 0
				for i := 0; i < mask.Len(); i++ {
					if isTruthy(mask.Get(i)) {
						if dataIdx >= data.Len() {
							return nil, fmt.Errorf("expand: not enough data elements for mask")
						}
						result[i] = data.Get(dataIdx)
						dataIdx++
					} else {
						result[i] = NewInteger(0)
					}
				}
				return []Value{NewList(result)}, nil
			},
			Returns: []Type{TList},
		}},
	})
}

// window: [TInteger, TList] -> [TList]
// Args[0] = size, Args[1] = data
func registerWindow(r *Registry) {
	r.RegisterNativeFunc(NativeFunc{
		Name:              "window",
		ForwardPrecedence: true,
		Signatures: []NativeSig{{
			Args: []Type{TInteger, TList},
			Handler: func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
				if args[1].Data == nil {
					return nil, fmt.Errorf("window: expected concrete list")
				}
				_as6, _ := args[0].AsInteger()
				size := int(_as6)
				list := args[1].AsList()
				length := list.Len()
				if size <= 0 {
					return nil, fmt.Errorf("window: size must be positive, got %d", size)
				}
				if size > length {
					return []Value{NewList([]Value{})}, nil
				}
				windows := make([]Value, length-size+1)
				for i := 0; i <= length-size; i++ {
					win := make([]Value, size)
					for j := 0; j < size; j++ {
						win[j] = list.Get(i + j)
					}
					windows[i] = NewList(win)
				}
				return []Value{NewList(windows)}, nil
			},
			Returns: []Type{TList},
		}},
	})
}

// pairs: [TList] -> [TList]
func registerPairs(r *Registry) {
	r.RegisterNativeFunc(NativeFunc{
		Name:              "pairs",
		ForwardPrecedence: true,
		Signatures: []NativeSig{{
			Args: []Type{TList},
			Handler: func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
				if args[0].Data == nil {
					return nil, fmt.Errorf("pairs: expected concrete list")
				}
				list := args[0].AsList()
				length := list.Len()
				if length < 2 {
					return []Value{NewList([]Value{})}, nil
				}
				result := make([]Value, length-1)
				for i := 0; i < length-1; i++ {
					pair := []Value{list.Get(i), list.Get(i + 1)}
					result[i] = NewList(pair)
				}
				return []Value{NewList(result)}, nil
			},
			Returns: []Type{TList},
		}},
	})
}
