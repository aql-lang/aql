package engine

import (
	"fmt"
	"sort"
)

// arrayNatives covers the array words: core scalar/vector ops
// (iota, shape, rank, length, reshape, arr-flatten, arr-transpose,
// reverse, take, shed, where, unique, grade, at, sortby, member,
// arr-indexof, group, replicate, expand, window, pairs) and the
// higher-order ops (each, fold, scan, outer, inner).
//
// Pure helpers (computeShape, flattenList, buildNested,
// arrCompareValues, transposeListOfLists, doFold,
// analyseHigherOrderBody) live alongside their handlers below.
var arrayNatives = []NativeFunc{
	// ---- core ----
	{
		Name:        "iota",
		ForwardArgs: true,
		Signatures: []NativeSig{{
			Args:      []*Type{TInteger},
			Handler:   iotaHandler,
			ReturnsFn: returnsCarrierTypedListInteger,
		}},
	},
	{
		Name:        "shape",
		ForwardArgs: true,
		Signatures: []NativeSig{{
			Args:      []*Type{TList},
			Handler:   shapeHandler,
			ReturnsFn: returnsCarrierTypedListInteger,
		}},
	},
	{
		Name:        "rank",
		ForwardArgs: true,
		Signatures: []NativeSig{{
			Args:    []*Type{TList},
			Handler: rankHandler,
			Returns: []*Type{TInteger},
		}},
	},
	{
		Name:        "length",
		ForwardArgs: true,
		Signatures: []NativeSig{{
			Args:    []*Type{TList},
			Handler: lengthHandler,
			Returns: []*Type{TInteger},
		}},
	},
	{
		Name:        "reshape",
		ForwardArgs: true,
		Signatures: []NativeSig{{
			Args:      []*Type{TList, TList},
			Handler:   reshapeHandler,
			ReturnsFn: ReturnsPreserveListAt(1),
		}},
	},
	{
		Name:        "arr-flatten",
		ForwardArgs: true,
		Signatures: []NativeSig{{
			Args:      []*Type{TList},
			Handler:   arrFlattenHandler,
			ReturnsFn: ReturnsPreserveListAt(0),
		}},
	},
	{
		Name:        "arr-transpose",
		ForwardArgs: true,
		Signatures: []NativeSig{{
			Args:      []*Type{TList},
			Handler:   arrTransposeHandler,
			ReturnsFn: ReturnsPreserveListAt(0),
		}},
	},
	{
		Name:        "reverse",
		ForwardArgs: true,
		Signatures: []NativeSig{{
			Args:      []*Type{TList},
			Handler:   reverseHandler,
			ReturnsFn: ReturnsPreserveListAt(0),
		}},
	},
	{
		Name:        "take",
		ForwardArgs: true,
		Signatures: []NativeSig{{
			Args:      []*Type{TInteger, TList},
			Handler:   takeHandler,
			ReturnsFn: ReturnsPreserveListAt(1),
		}},
	},
	{
		Name:        "shed",
		ForwardArgs: true,
		Signatures: []NativeSig{{
			Args:      []*Type{TInteger, TList},
			Handler:   shedHandler,
			ReturnsFn: ReturnsPreserveListAt(1),
		}},
	},
	{
		Name:        "where",
		ForwardArgs: true,
		Signatures: []NativeSig{{
			Args:      []*Type{TList},
			Handler:   whereHandler,
			ReturnsFn: returnsCarrierTypedListInteger,
		}},
	},
	{
		Name:        "unique",
		ForwardArgs: true,
		Signatures: []NativeSig{{
			Args:      []*Type{TList},
			Handler:   uniqueHandler,
			ReturnsFn: ReturnsPreserveListAt(0),
		}},
	},
	{
		Name:        "grade",
		ForwardArgs: true,
		Signatures: []NativeSig{{
			Args:      []*Type{TList},
			Handler:   gradeHandler,
			ReturnsFn: returnsCarrierTypedListInteger,
		}},
	},
	{
		Name:        "at",
		ForwardArgs: true,
		Signatures: []NativeSig{{
			Args:      []*Type{TList, TList},
			Handler:   atHandler,
			ReturnsFn: ReturnsPreserveListAt(1),
		}},
	},
	{
		Name:        "sortby",
		ForwardArgs: true,
		Signatures: []NativeSig{{
			Args:      []*Type{TList, TList},
			Handler:   sortbyHandler,
			ReturnsFn: ReturnsPreserveListAt(1),
		}},
	},
	{
		Name:        "member",
		ForwardArgs: true,
		Signatures: []NativeSig{{
			Args:      []*Type{TList, TList},
			Handler:   memberHandler,
			ReturnsFn: returnsCarrierTypedListBoolean,
		}},
	},
	{
		Name:        "arr-indexof",
		ForwardArgs: true,
		Signatures: []NativeSig{{
			Args:      []*Type{TList, TList},
			Handler:   arrIndexofHandler,
			ReturnsFn: returnsCarrierTypedListInteger,
		}},
	},
	{
		Name:        "group",
		ForwardArgs: true,
		Signatures: []NativeSig{
			{
				Args:    []*Type{TList, TList},
				Handler: groupTwoHandler,
				Returns: []*Type{TMap},
			},
			{
				Args:    []*Type{TList},
				Handler: groupOneHandler,
				Returns: []*Type{TMap},
			},
		},
	},
	{
		Name:        "replicate",
		ForwardArgs: true,
		Signatures: []NativeSig{{
			Args:      []*Type{TList, TList},
			Handler:   replicateHandler,
			ReturnsFn: ReturnsPreserveListAt(1),
		}},
	},
	{
		Name:        "expand",
		ForwardArgs: true,
		Signatures: []NativeSig{{
			Args:      []*Type{TList, TList},
			Handler:   expandHandler,
			ReturnsFn: ReturnsPreserveListAt(1),
		}},
	},
	{
		Name:        "window",
		ForwardArgs: true,
		Signatures: []NativeSig{{
			Args:      []*Type{TInteger, TList},
			Handler:   windowHandler,
			ReturnsFn: windowReturnsFn,
		}},
	},
	{
		Name:        "pairs",
		ForwardArgs: true,
		Signatures: []NativeSig{{
			Args:      []*Type{TList},
			Handler:   pairsHandler,
			ReturnsFn: pairsReturnsFn,
		}},
	},

	// ---- higher-order ----
	{
		Name:        "each",
		ForwardArgs: true,
		Signatures: []NativeSig{{
			Args:       []*Type{TList, TList},
			NoEvalArgs: map[int]bool{0: true},
			Handler:    eachHandler,
			ReturnsFn:  eachReturnsFn,
		}},
	},
	{
		Name:        "fold",
		ForwardArgs: true,
		Signatures: []NativeSig{
			{
				// With initial value: init fold body data → result.
				// Sig is body-first (matching each/scan) so the swap form
				// `init fold body data` collects body+data forward and
				// init from the stack.
				Args:       []*Type{TList, TList, TAny},
				NoEvalArgs: map[int]bool{0: true},
				Handler:    foldWithInitHandler,
				ReturnsFn:  foldWithInitReturnsFn,
			},
			{
				// Without initial: body data → result (uses first element as init)
				Args:       []*Type{TList, TList},
				NoEvalArgs: map[int]bool{0: true},
				Handler:    foldNoInitHandler,
				ReturnsFn:  foldNoInitReturnsFn,
			},
		},
	},
	{
		Name:        "scan",
		ForwardArgs: true,
		Signatures: []NativeSig{{
			Args:       []*Type{TList, TList},
			NoEvalArgs: map[int]bool{0: true},
			Handler:    scanHandler,
			ReturnsFn:  scanReturnsFn,
		}},
	},
	{
		Name:        "outer",
		ForwardArgs: true,
		Signatures: []NativeSig{{
			Args:       []*Type{TList, TList, TList},
			NoEvalArgs: map[int]bool{0: true},
			Handler:    outerHandler,
			ReturnsFn:  outerReturnsFn,
		}},
	},
	{
		Name:        "inner",
		ForwardArgs: true,
		Signatures: []NativeSig{{
			Args:       []*Type{TList, TList, TList, TList},
			NoEvalArgs: map[int]bool{0: true, 1: true},
			Handler:    innerHandler,
			ReturnsFn:  innerReturnsFn,
		}},
	},
}

// ---- shared ReturnsFn helpers ----

func returnsCarrierTypedListInteger(_ []Value, _ *Registry) []Value {
	return []Value{NewCarrierTypedList(TInteger)}
}

func returnsCarrierTypedListBoolean(_ []Value, _ *Registry) []Value {
	return []Value{NewCarrierTypedList(TBoolean)}
}

// ---- iota ----

func iotaHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	_as0, _ := args[0].AsConcreteInteger()
	n := int(_as0)
	if n < 0 {
		return nil, fmt.Errorf("iota: negative count %d", n)
	}
	elems := make([]Value, n)
	for i := 0; i < n; i++ {
		elems[i] = NewInteger(int64(i))
	}
	return []Value{NewList(elems)}, nil
}

// ---- shape ----

func shapeHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	if !IsConcrete(args[0]) {
		return nil, fmt.Errorf("shape: expected concrete list")
	}
	dims := computeShape(args[0])
	elems := make([]Value, len(dims))
	for i, d := range dims {
		elems[i] = NewInteger(int64(d))
	}
	return []Value{NewList(elems)}, nil
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

// ---- rank ----

func rankHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	if !IsConcrete(args[0]) {
		return nil, fmt.Errorf("rank: expected concrete list")
	}
	dims := computeShape(args[0])
	return []Value{NewInteger(int64(len(dims)))}, nil
}

// ---- length ----

func lengthHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	if !IsConcrete(args[0]) {
		return nil, fmt.Errorf("length: expected concrete list")
	}
	list := args[0].AsList()
	return []Value{NewInteger(int64(list.Len()))}, nil
}

// ---- reshape ----

func reshapeHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	if !IsConcrete(args[0]) {
		return nil, fmt.Errorf("reshape: expected concrete shape list")
	}
	if !IsConcrete(args[1]) {
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

// ---- arr-flatten ----

func arrFlattenHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	if !IsConcrete(args[0]) {
		return nil, fmt.Errorf("arr-flatten: expected concrete list")
	}
	flat := flattenList(args[0])
	return []Value{NewList(flat)}, nil
}

// ---- arr-transpose ----

func arrTransposeHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	if !IsConcrete(args[0]) {
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
}

// ---- reverse ----

func reverseHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	if !IsConcrete(args[0]) {
		return nil, fmt.Errorf("reverse: expected concrete list")
	}
	list := args[0].AsList()
	n := list.Len()
	elems := make([]Value, n)
	for i := 0; i < n; i++ {
		elems[i] = list.Get(n - 1 - i)
	}
	return []Value{NewList(elems)}, nil
}

// ---- take ----

func takeHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	if !IsConcrete(args[1]) {
		return nil, fmt.Errorf("take: expected concrete list")
	}
	_as2, _ := args[0].AsConcreteInteger()
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
}

// ---- shed ----

func shedHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	if !IsConcrete(args[1]) {
		return nil, fmt.Errorf("shed: expected concrete list")
	}
	_as3, _ := args[0].AsConcreteInteger()
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
}

// ---- where ----

func whereHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	if !IsConcrete(args[0]) {
		return nil, fmt.Errorf("where: expected concrete list")
	}
	list := args[0].AsList()
	var result []Value
	for i := 0; i < list.Len(); i++ {
		elem := list.Get(i)
		if CoerceBoolean(elem) {
			result = append(result, NewInteger(int64(i)))
		}
	}
	if result == nil {
		result = []Value{}
	}
	return []Value{NewList(result)}, nil
}

// ---- unique ----

func uniqueHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	if !IsConcrete(args[0]) {
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
}

// ---- grade ----

func gradeHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	if !IsConcrete(args[0]) {
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
}

// arrCompareValues is a non-error variant of CompareValues for use in sort functions.
// Falls back to string comparison if CompareValues returns an error.
func arrCompareValues(a, b Value) int {
	cmp, err := CompareValues(a, b)
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

// ---- at ----

func atHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	if !IsConcrete(args[0]) {
		return nil, fmt.Errorf("at: expected concrete indices list")
	}
	if !IsConcrete(args[1]) {
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
}

// ---- sortby ----

func sortbyHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	if !IsConcrete(args[0]) {
		return nil, fmt.Errorf("sortby: expected concrete keys list")
	}
	if !IsConcrete(args[1]) {
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
}

// ---- member ----

func memberHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	if !IsConcrete(args[0]) {
		return nil, fmt.Errorf("member: expected concrete needles list")
	}
	if !IsConcrete(args[1]) {
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
}

// ---- arr-indexof ----

func arrIndexofHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	if !IsConcrete(args[0]) {
		return nil, fmt.Errorf("arr-indexof: expected concrete needles list")
	}
	if !IsConcrete(args[1]) {
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
}

// ---- group ----

func groupTwoHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	if !IsConcrete(args[0]) {
		return nil, fmt.Errorf("group: expected concrete keys list")
	}
	if !IsConcrete(args[1]) {
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
}

func groupOneHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	if !IsConcrete(args[0]) {
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
}

// ---- replicate ----

func replicateHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	if !IsConcrete(args[0]) {
		return nil, fmt.Errorf("replicate: expected concrete counts list")
	}
	if !IsConcrete(args[1]) {
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
}

// ---- expand ----

func expandHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	if !IsConcrete(args[0]) {
		return nil, fmt.Errorf("expand: expected concrete mask list")
	}
	if !IsConcrete(args[1]) {
		return nil, fmt.Errorf("expand: expected concrete data list")
	}
	mask := args[0].AsList()
	data := args[1].AsList()
	result := make([]Value, mask.Len())
	dataIdx := 0
	for i := 0; i < mask.Len(); i++ {
		if CoerceBoolean(mask.Get(i)) {
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
}

// ---- window ----

func windowHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	if !IsConcrete(args[1]) {
		return nil, fmt.Errorf("window: expected concrete list")
	}
	_as6, _ := args[0].AsConcreteInteger()
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
}

// window yields a TList<TList<sameElem>>: wrap the source-data
// element carrier twice.
func windowReturnsFn(args []Value, _ *Registry) []Value {
	elem := DataListElemTypeFromValue(args[1])
	inner := NewCarrierTypedList(elem)
	return []Value{NewCarrierTypedListValue(inner)}
}

// ---- pairs ----

func pairsHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	if !IsConcrete(args[0]) {
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
}

// pairs yields TList<TList<sameElem>> (2-tuples).
func pairsReturnsFn(args []Value, _ *Registry) []Value {
	elem := DataListElemTypeFromValue(args[0])
	inner := NewCarrierTypedList(elem)
	return []Value{NewCarrierTypedListValue(inner)}
}

// ---- each ----

func eachHandler(args []Value, _ map[string]Value, _ []Value, reg *Registry) ([]Value, error) {
	if !IsConcrete(args[0]) || !IsConcrete(args[1]) {
		return nil, fmt.Errorf("each: expected concrete lists")
	}
	bodySlice := args[0].AsList().Slice()
	dataList := args[1].AsList()

	results := make([]Value, dataList.Len())
	for i := 0; i < dataList.Len(); i++ {
		elem := dataList.Get(i)
		input := make([]Value, len(bodySlice)+1)
		input[0] = elem
		copy(input[1:], bodySlice)

		sub := New(reg)
		res, err := sub.Run(input)
		if err != nil {
			return nil, fmt.Errorf("each: element %d: %w", i, err)
		}
		if len(res) == 0 {
			return nil, fmt.Errorf("each: element %d: body produced no result", i)
		}
		results[i] = res[len(res)-1] // take top of stack
	}
	return []Value{NewList(results)}, nil
}

// each returns a list whose element type is whatever the body's
// top-of-stack produces. Pass the concrete data list's element
// carrier into the body so diagnostics fire against realistic types.
func eachReturnsFn(args []Value, r *Registry) []Value {
	elem := DataListElemTypeFromValue(args[1])
	stk := analyseHigherOrderBody(r, args[0], elem)
	if len(stk) == 0 {
		return []Value{NewCarrier(TList)}
	}
	return []Value{NewCarrierTypedList(stk[len(stk)-1].VType)}
}

// analyseHigherOrderBody runs a literal code-body list through a
// sub-engine in check mode, prepending the given element carrier
// type(s) so body words see realistic input carriers. Returns the
// residual carrier stack, or nil if the body is not concrete. The
// primary purpose is side-effect: any diagnostics the body produces
// (type mismatches, undefined words) are accumulated on the registry.
func analyseHigherOrderBody(r *Registry, body Value, elems ...*Type) []Value {
	if body.Data == nil {
		return nil
	}
	bodyList := body.AsList()
	if bodyList.IsNil() {
		return nil
	}
	input := make([]Value, 0, len(elems)+bodyList.Len())
	for _, t := range elems {
		input = append(input, NewCarrier(t))
	}
	input = append(input, bodyList.Slice()...)
	sub := New(r)
	result, err := sub.Run(input)
	if err != nil {
		r.AddCheckDiagnostic(CheckDiagnostic{
			Code:   "body_error",
			Detail: "higher-order body analysis error: " + err.Error(),
		})
		return nil
	}
	return result
}

// ---- fold ----

func foldWithInitHandler(args []Value, _ map[string]Value, _ []Value, reg *Registry) ([]Value, error) {
	// Sig is [TList, TList, TAny]: args[0]=body, args[1]=data, args[2]=init.
	if !IsConcrete(args[0]) || !IsConcrete(args[1]) {
		return nil, fmt.Errorf("fold: expected concrete lists")
	}
	bodySlice := args[0].AsList().Slice()
	dataList := args[1].AsList()
	init := args[2]
	return doFold(reg, init, bodySlice, dataList)
}

// Fold result type is the body's output. Analyse the body once with
// (init, element) as carrier inputs; use the residual top-of-stack
// carrier as the result. A proper fixed-point would iterate until
// the accumulator type stabilises — one pass is a close
// approximation for bounded-lattice types.
func foldWithInitReturnsFn(args []Value, r *Registry) []Value {
	elem := DataListElemTypeFromValue(args[1])
	stk := analyseHigherOrderBody(r, args[0], args[2].VType, elem)
	if len(stk) == 0 {
		return []Value{NewCarrier(TAny)}
	}
	return []Value{stk[len(stk)-1]}
}

func foldNoInitHandler(args []Value, _ map[string]Value, _ []Value, reg *Registry) ([]Value, error) {
	if !IsConcrete(args[0]) || !IsConcrete(args[1]) {
		return nil, fmt.Errorf("fold: expected concrete lists")
	}
	bodySlice := args[0].AsList().Slice()
	dataList := args[1].AsList()
	if dataList.Len() == 0 {
		return nil, fmt.Errorf("fold: empty list with no initial value")
	}
	init := dataList.Get(0)
	// Create a sub-list from element 1 onwards
	rest := make([]Value, dataList.Len()-1)
	for i := 1; i < dataList.Len(); i++ {
		rest[i-1] = dataList.Get(i)
	}
	restList := NewReadList(rest)
	return doFold(reg, init, bodySlice, restList)
}

// No init — accumulator type and element type both come from the
// data list.
func foldNoInitReturnsFn(args []Value, r *Registry) []Value {
	elem := DataListElemTypeFromValue(args[1])
	stk := analyseHigherOrderBody(r, args[0], elem, elem)
	if len(stk) == 0 {
		return []Value{NewCarrier(TAny)}
	}
	return []Value{stk[len(stk)-1]}
}

// doFold is the shared fold implementation used by both fold signatures.
func doFold(reg *Registry, acc Value, bodySlice []Value, data ReadList) ([]Value, error) {
	for i := 0; i < data.Len(); i++ {
		elem := data.Get(i)
		input := make([]Value, len(bodySlice)+2)
		input[0] = acc
		input[1] = elem
		copy(input[2:], bodySlice)

		sub := New(reg)
		res, err := sub.Run(input)
		if err != nil {
			return nil, fmt.Errorf("fold: step %d: %w", i, err)
		}
		if len(res) == 0 {
			return nil, fmt.Errorf("fold: step %d: body produced no result", i)
		}
		acc = res[len(res)-1]
	}
	return []Value{acc}, nil
}

// ---- scan ----

func scanHandler(args []Value, _ map[string]Value, _ []Value, reg *Registry) ([]Value, error) {
	if !IsConcrete(args[0]) || !IsConcrete(args[1]) {
		return nil, fmt.Errorf("scan: expected concrete lists")
	}
	bodySlice := args[0].AsList().Slice()
	dataList := args[1].AsList()
	if dataList.Len() == 0 {
		return []Value{NewList(nil)}, nil
	}

	results := make([]Value, dataList.Len())
	acc := dataList.Get(0)
	results[0] = acc

	for i := 1; i < dataList.Len(); i++ {
		elem := dataList.Get(i)
		input := make([]Value, len(bodySlice)+2)
		input[0] = acc
		input[1] = elem
		copy(input[2:], bodySlice)

		sub := New(reg)
		res, err := sub.Run(input)
		if err != nil {
			return nil, fmt.Errorf("scan: step %d: %w", i, err)
		}
		if len(res) == 0 {
			return nil, fmt.Errorf("scan: step %d: body produced no result", i)
		}
		acc = res[len(res)-1]
		results[i] = acc
	}
	return []Value{NewList(results)}, nil
}

func scanReturnsFn(args []Value, r *Registry) []Value {
	elem := DataListElemTypeFromValue(args[1])
	stk := analyseHigherOrderBody(r, args[0], elem, elem)
	if len(stk) == 0 {
		return []Value{NewCarrier(TList)}
	}
	return []Value{NewCarrierTypedList(stk[len(stk)-1].VType)}
}

// ---- outer ----

func outerHandler(args []Value, _ map[string]Value, _ []Value, reg *Registry) ([]Value, error) {
	if !IsConcrete(args[0]) || !IsConcrete(args[1]) || !IsConcrete(args[2]) {
		return nil, fmt.Errorf("outer: expected concrete lists")
	}
	bodySlice := args[0].AsList().Slice()
	left := args[1].AsList()
	right := args[2].AsList()

	rows := make([]Value, left.Len())
	for i := 0; i < left.Len(); i++ {
		row := make([]Value, right.Len())
		for j := 0; j < right.Len(); j++ {
			input := make([]Value, len(bodySlice)+2)
			input[0] = left.Get(i)
			input[1] = right.Get(j)
			copy(input[2:], bodySlice)

			sub := New(reg)
			res, err := sub.Run(input)
			if err != nil {
				return nil, fmt.Errorf("outer: (%d,%d): %w", i, j, err)
			}
			if len(res) == 0 {
				return nil, fmt.Errorf("outer: (%d,%d): body produced no result", i, j)
			}
			row[j] = res[len(res)-1]
		}
		rows[i] = NewList(row)
	}
	return []Value{NewList(rows)}, nil
}

func outerReturnsFn(args []Value, r *Registry) []Value {
	leftElem := DataListElemTypeFromValue(args[1])
	rightElem := DataListElemTypeFromValue(args[2])
	stk := analyseHigherOrderBody(r, args[0], leftElem, rightElem)
	// outer produces a 2D list: TList<TList<body-result>>.
	innerElem := TAny
	if len(stk) > 0 {
		innerElem = stk[len(stk)-1].VType
	}
	inner := NewCarrierTypedList(innerElem)
	return []Value{NewCarrierTypedListValue(inner)}
}

// ---- inner ----

func innerHandler(args []Value, _ map[string]Value, _ []Value, reg *Registry) ([]Value, error) {
	if !IsConcrete(args[0]) || !IsConcrete(args[1]) || !IsConcrete(args[2]) || !IsConcrete(args[3]) {
		return nil, fmt.Errorf("inner: expected concrete lists")
	}
	pairOp := args[0].AsList().Slice()
	aggOp := args[1].AsList().Slice()
	left := args[2].AsList()
	right := args[3].AsList()

	// 1D case: zip then fold
	if left.Len() > 0 && !left.Get(0).VType.Matches(TList) {
		if left.Len() != right.Len() {
			return nil, fmt.Errorf("inner: vectors must have same length")
		}
		// Apply pair-op to each pair
		paired := make([]Value, left.Len())
		for i := 0; i < left.Len(); i++ {
			input := make([]Value, len(pairOp)+2)
			input[0] = left.Get(i)
			input[1] = right.Get(i)
			copy(input[2:], pairOp)
			sub := New(reg)
			res, err := sub.Run(input)
			if err != nil {
				return nil, fmt.Errorf("inner: pair %d: %w", i, err)
			}
			if len(res) == 0 {
				return nil, fmt.Errorf("inner: pair %d: no result", i)
			}
			paired[i] = res[len(res)-1]
		}
		// Fold with agg-op
		acc := paired[0]
		for i := 1; i < len(paired); i++ {
			input := make([]Value, len(aggOp)+2)
			input[0] = acc
			input[1] = paired[i]
			copy(input[2:], aggOp)
			sub := New(reg)
			res, err := sub.Run(input)
			if err != nil {
				return nil, fmt.Errorf("inner: fold %d: %w", i, err)
			}
			if len(res) == 0 {
				return nil, fmt.Errorf("inner: fold %d: no result", i)
			}
			acc = res[len(res)-1]
		}
		return []Value{acc}, nil
	}

	// 2D case: matrix inner product
	// left is list of rows, right is list of rows
	// Need to transpose right to get columns
	rightCols := transposeListOfLists(right)

	rows := make([]Value, left.Len())
	for i := 0; i < left.Len(); i++ {
		leftRow := left.Get(i).AsList()
		cols := make([]Value, len(rightCols))
		for j := 0; j < len(rightCols); j++ {
			rightCol := rightCols[j]
			if leftRow.Len() != len(rightCol) {
				return nil, fmt.Errorf("inner: dimension mismatch")
			}
			// Pair then fold
			paired := make([]Value, leftRow.Len())
			for k := 0; k < leftRow.Len(); k++ {
				input := make([]Value, len(pairOp)+2)
				input[0] = leftRow.Get(k)
				input[1] = rightCol[k]
				copy(input[2:], pairOp)
				sub := New(reg)
				res, err := sub.Run(input)
				if err != nil {
					return nil, err
				}
				if len(res) == 0 {
					return nil, fmt.Errorf("inner: pair (%d,%d,%d): no result", i, j, k)
				}
				paired[k] = res[len(res)-1]
			}
			acc := paired[0]
			for k := 1; k < len(paired); k++ {
				input := make([]Value, len(aggOp)+2)
				input[0] = acc
				input[1] = paired[k]
				copy(input[2:], aggOp)
				sub := New(reg)
				res, err := sub.Run(input)
				if err != nil {
					return nil, err
				}
				if len(res) == 0 {
					return nil, fmt.Errorf("inner: fold (%d,%d,%d): no result", i, j, k)
				}
				acc = res[len(res)-1]
			}
			cols[j] = acc
		}
		rows[i] = NewList(cols)
	}
	return []Value{NewList(rows)}, nil
}

func innerReturnsFn(args []Value, r *Registry) []Value {
	leftElem := DataListElemTypeFromValue(args[2])
	rightElem := DataListElemTypeFromValue(args[3])
	// pair op consumes (left-elem, right-elem); agg consumes
	// (accumulator, pair-result). Without carrier list element
	// tracking we use the pair output as TAny for the agg input.
	analyseHigherOrderBody(r, args[0], leftElem, rightElem)
	analyseHigherOrderBody(r, args[1], TAny, TAny)
	return []Value{NewCarrier(TList)}
}

// transposeListOfLists transposes a list-of-lists, returning columns as [][]Value.
func transposeListOfLists(rows ReadList) [][]Value {
	if rows.Len() == 0 {
		return nil
	}
	firstRow := rows.Get(0).AsList()
	cols := firstRow.Len()
	result := make([][]Value, cols)
	for j := 0; j < cols; j++ {
		result[j] = make([]Value, rows.Len())
	}
	for i := 0; i < rows.Len(); i++ {
		row := rows.Get(i).AsList()
		for j := 0; j < cols && j < row.Len(); j++ {
			result[j][i] = row.Get(j)
		}
	}
	return result
}
