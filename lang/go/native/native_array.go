package native

import (
	"fmt"
	"sort"
)

// arrayNatives (core) and ArrayModuleNatives (the aql:array module)
// are both derived from allArrayNatives below. The split follows one
// rule: words that take a quoted code body, the basic constructors,
// and everyday slicing stay in core; the specialised APL-style data
// vocabulary (shape/structure, selection/ordering, membership/
// grouping, neighborhoods) lives in the module and is reached via
// dot-access (array.shape, array.where, …).
//
//	core   — iota, range, each, fold, scan, outer, inner,
//	         take, shed, reverse
//	module — shape, rank, reshape, transpose, where, unique, grade,
//	         at, sortby, member, group, replicate, expand, window,
//	         pairs
//
// Per ADR-001 (no module export shadows a core word — see ADR.md in
// the repo root), the two operations that overlap a core word are NOT
// array-module words: deep flatten is `flatten -1` (a depth on the
// core flatten word, flatten.go) and list indexof is a [List, List]
// overload of the core indexof word (native_string.go). transpose has
// no core counterpart, so it keeps its plain name.
//
// Pure helpers (computeShape, flattenList, buildNested,
// arrCompareValues, transposeListOfLists, doFold,
// analyseHigherOrderBody) live alongside their handlers below.
var allArrayNatives = []NativeFunc{
	// ---- core ----
	{
		Name: "iota",

		Signatures: []NativeSig{{
			Args:      []*Type{TInteger},
			Handler:   iotaHandler,
			ReturnsFn: returnsCarrierTypedListInteger, BarrierPos: -1,
		}},
	},
	{
		// range is iota's start/stop/step cousin: an arithmetic
		// sequence generator. The 3-arg sig is listed first so
		// `range start stop step` forward-collects all three; the
		// 2-arg sig (step defaults to 1) handles `range start stop`.
		Name: "range",

		Signatures: []NativeSig{
			{
				Args:      []*Type{TInteger, TInteger, TInteger},
				Handler:   rangeThreeHandler,
				ReturnsFn: returnsCarrierTypedListInteger, BarrierPos: -1,
			},
			{
				Args:      []*Type{TInteger, TInteger},
				Handler:   rangeTwoHandler,
				ReturnsFn: returnsCarrierTypedListInteger, BarrierPos: -1,
			},
		},
	},
	{
		Name: "shape",

		Signatures: []NativeSig{{
			Args:      []*Type{TList},
			Handler:   shapeHandler,
			ReturnsFn: returnsCarrierTypedListInteger, BarrierPos: -1,
		}},
	},
	{
		Name: "rank",

		Signatures: []NativeSig{{
			Args:    []*Type{TList},
			Handler: rankHandler,
			Returns: []*Type{TInteger}, BarrierPos: -1,
		}},
	},
	{
		Name: "reshape",

		Signatures: []NativeSig{{
			Args:      []*Type{TList, TList},
			Handler:   reshapeHandler,
			ReturnsFn: ReturnsPreserveListAt(1), BarrierPos: -1,
		}},
	},
	{
		// transpose has no core-word counterpart, so it keeps its plain
		// name (no "arr-" prefix needed). Deep flatten and list indexof,
		// by contrast, are now overloads of the core flatten/indexof
		// words rather than separate array words — see flatten.go and
		// native_string.go.
		Name: "transpose",

		Signatures: []NativeSig{{
			Args:      []*Type{TList},
			Handler:   arrTransposeHandler,
			ReturnsFn: ReturnsPreserveListAt(0), BarrierPos: -1,
		}},
	},
	{
		Name: "reverse",

		Signatures: []NativeSig{{
			Args:      []*Type{TList},
			Handler:   reverseHandler,
			ReturnsFn: ReturnsPreserveListAt(0), BarrierPos: -1,
		}},
	},
	{
		Name: "take",

		Signatures: []NativeSig{{
			Args:      []*Type{TInteger, TList},
			Handler:   takeHandler,
			ReturnsFn: ReturnsPreserveListAt(1), BarrierPos: -1,
		}},
	},
	{
		Name: "shed",

		Signatures: []NativeSig{{
			Args:      []*Type{TInteger, TList},
			Handler:   shedHandler,
			ReturnsFn: ReturnsPreserveListAt(1), BarrierPos: -1,
		}},
	},
	{
		Name: "where",

		Signatures: []NativeSig{{
			Args:      []*Type{TList},
			Handler:   whereHandler,
			ReturnsFn: returnsCarrierTypedListInteger, BarrierPos: -1,
		}},
	},
	{
		Name: "unique",

		Signatures: []NativeSig{{
			Args:      []*Type{TList},
			Handler:   uniqueHandler,
			ReturnsFn: ReturnsPreserveListAt(0), BarrierPos: -1,
		}},
	},
	{
		Name: "grade",

		Signatures: []NativeSig{{
			Args:      []*Type{TList},
			Handler:   gradeHandler,
			ReturnsFn: returnsCarrierTypedListInteger, BarrierPos: -1,
		}},
	},
	{
		Name: "at",

		Signatures: []NativeSig{{
			Args:      []*Type{TList, TList},
			Handler:   atHandler,
			ReturnsFn: ReturnsPreserveListAt(1), BarrierPos: -1,
		}},
	},
	{
		Name: "sortby",

		Signatures: []NativeSig{{
			Args:      []*Type{TList, TList},
			Handler:   sortbyHandler,
			ReturnsFn: ReturnsPreserveListAt(1), BarrierPos: -1,
		}},
	},
	{
		Name: "member",

		Signatures: []NativeSig{{
			Args:      []*Type{TList, TList},
			Handler:   memberHandler,
			ReturnsFn: returnsCarrierTypedListBoolean, BarrierPos: -1,
		}},
	},
	{
		Name: "group",

		Signatures: []NativeSig{
			{
				Args:    []*Type{TList, TList},
				Handler: groupTwoHandler,
				Returns: []*Type{TMap}, BarrierPos: -1,
			},
			{
				Args:    []*Type{TList},
				Handler: groupOneHandler,
				Returns: []*Type{TMap}, BarrierPos: -1,
			},
		},
	},
	{
		Name: "replicate",

		Signatures: []NativeSig{{
			Args:      []*Type{TList, TList},
			Handler:   replicateHandler,
			ReturnsFn: ReturnsPreserveListAt(1), BarrierPos: -1,
		}},
	},
	{
		Name: "expand",

		Signatures: []NativeSig{{
			Args:      []*Type{TList, TList},
			Handler:   expandHandler,
			ReturnsFn: ReturnsPreserveListAt(1), BarrierPos: -1,
		}},
	},
	{
		Name: "window",

		Signatures: []NativeSig{{
			Args:      []*Type{TInteger, TList},
			Handler:   windowHandler,
			ReturnsFn: windowReturnsFn, BarrierPos: -1,
		}},
	},
	{
		Name: "pairs",

		Signatures: []NativeSig{{
			Args:      []*Type{TList},
			Handler:   pairsHandler,
			ReturnsFn: pairsReturnsFn, BarrierPos: -1,
		}},
	},

	// ---- higher-order ----
	{
		Name: "each",

		Signatures: []NativeSig{{
			Args:       []*Type{TList, TList},
			NoEvalArgs: map[int]bool{0: true},
			Handler:    eachHandler,
			ReturnsFn:  eachReturnsFn, BarrierPos: -1,
		}},
	},
	{
		Name: "fold",

		Signatures: []NativeSig{
			{
				// With initial value: init fold body data → result.
				// Sig is body-first (matching each/scan) so the swap form
				// `init fold body data` collects body+data forward and
				// init from the stack.
				Args:       []*Type{TList, TList, TAny},
				NoEvalArgs: map[int]bool{0: true},
				Handler:    foldWithInitHandler,
				ReturnsFn:  foldWithInitReturnsFn, BarrierPos: -1,
			},
			{
				// Without initial: body data → result (uses first element as init)
				Args:       []*Type{TList, TList},
				NoEvalArgs: map[int]bool{0: true},
				Handler:    foldNoInitHandler,
				ReturnsFn:  foldNoInitReturnsFn, BarrierPos: -1,
			},
		},
	},
	{
		Name: "scan",

		Signatures: []NativeSig{{
			Args:       []*Type{TList, TList},
			NoEvalArgs: map[int]bool{0: true},
			Handler:    scanHandler,
			ReturnsFn:  scanReturnsFn, BarrierPos: -1,
		}},
	},
	{
		Name: "outer",

		Signatures: []NativeSig{{
			Args:       []*Type{TList, TList, TList},
			NoEvalArgs: map[int]bool{0: true},
			Handler:    outerHandler,
			ReturnsFn:  outerReturnsFn, BarrierPos: -1,
		}},
	},
	{
		Name: "inner",

		Signatures: []NativeSig{{
			Args:       []*Type{TList, TList, TList, TList},
			NoEvalArgs: map[int]bool{0: true, 1: true},
			Handler:    innerHandler,
			ReturnsFn:  innerReturnsFn, BarrierPos: -1,
		}},
	},
}

// arrayCoreNames is the set of array words that remain built-in. The
// rest of allArrayNatives moves to the aql:array module. See the
// allArrayNatives comment for the rationale behind the split.
var arrayCoreNames = map[string]bool{
	"iota": true, "range": true,
	"each": true, "fold": true, "scan": true, "outer": true, "inner": true,
	"take": true, "shed": true, "reverse": true,
}

// arrayNatives are the core array words registered globally (see
// register.go). ArrayModuleNatives are the specialised words that the
// aql:array module registers into its own sub-registry instead — they
// are NOT globally available, matching how aql:math gates sin/cos/etc.
var arrayNatives, ArrayModuleNatives = func() (core, module []NativeFunc) {
	for _, n := range allArrayNatives {
		if arrayCoreNames[n.Name] {
			core = append(core, n)
		} else {
			module = append(module, n)
		}
	}
	return core, module
}()

// ---- shared ReturnsFn helpers ----

func returnsCarrierTypedListInteger(_ []Value, _ *Registry) []Value {
	return []Value{NewCarrierTypedList(TInteger)}
}

func returnsCarrierTypedListBoolean(_ []Value, _ *Registry) []Value {
	return []Value{NewCarrierTypedList(TBoolean)}
}

// ---- iota ----

func iotaHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	_as0, _ := args[0].AsConcreteInteger()
	n := int(_as0)
	if n < 0 {
		return nil, r.AqlError("iota_error", fmt.Sprintf("iota: negative count %d", n), "iota")
	}
	elems := make([]Value, n)
	for i := 0; i < n; i++ {
		elems[i] = NewInteger(int64(i))
	}
	return []Value{NewList(elems)}, nil
}

// ---- range ----

func rangeThreeHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	start, _ := args[0].AsConcreteInteger()
	stop, _ := args[1].AsConcreteInteger()
	step, _ := args[2].AsConcreteInteger()
	return buildRange(start, stop, step, r)
}

func rangeTwoHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	start, _ := args[0].AsConcreteInteger()
	stop, _ := args[1].AsConcreteInteger()
	return buildRange(start, stop, 1, r)
}

// buildRange produces the arithmetic sequence [start, start+step, ...)
// stopping before stop. A positive step counts up, a negative step
// counts down; a zero step is an error. The half-open convention makes
// `range 0 n 1` equal to `iota n`, and an empty range (start already
// past stop in the step direction) yields [].
func buildRange(start, stop, step int64, r *Registry) ([]Value, error) {
	if step == 0 {
		return nil, r.AqlError("range_error", "range: step must be non-zero", "range")
	}
	elems := []Value{}
	if step > 0 {
		for i := start; i < stop; i += step {
			elems = append(elems, NewInteger(i))
		}
	} else {
		for i := start; i > stop; i += step {
			elems = append(elems, NewInteger(i))
		}
	}
	return []Value{NewList(elems)}, nil
}

// ---- shape ----

func shapeHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	if !IsConcrete(args[0]) {
		return nil, r.AqlError("shape_error", "shape: expected concrete list", "shape")
	}
	dims := computeShape(args[0])
	elems := make([]Value, len(dims))
	for i, d := range dims {
		elems[i] = NewInteger(int64(d))
	}
	return []Value{NewList(elems)}, nil
}

func computeShape(v Value) []int {
	list, _ := AsList(v)
	if list.IsNil() {
		return nil
	}
	dims := []int{list.Len()}
	if list.Len() == 0 {
		return dims
	}
	first := list.Get(0)
	if !first.Parent.Matches(TList) || !IsConcrete(first) {
		return dims
	}
	_lst, _ := AsList(first)
	firstLen := _lst.Len()
	for i := 1; i < list.Len(); i++ {
		sub := list.Get(i)
		_subLst, _ := AsList(sub)
		if !sub.Parent.Matches(TList) || !IsConcrete(sub) || _subLst.Len() != firstLen {
			return dims
		}
	}
	return append(dims, computeShape(first)...)
}

// ---- rank ----

func rankHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	if !IsConcrete(args[0]) {
		return nil, r.AqlError("rank_error", "rank: expected concrete list", "rank")
	}
	dims := computeShape(args[0])
	return []Value{NewInteger(int64(len(dims)))}, nil
}

// ---- reshape ----

func reshapeHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	if !IsConcrete(args[0]) {
		return nil, r.AqlError("reshape_error", "reshape: expected concrete shape list", "reshape")
	}
	if !IsConcrete(args[1]) {
		return nil, r.AqlError("reshape_error", "reshape: expected concrete data list", "reshape")
	}
	shapeList, _ := AsList(args[0])
	dims := make([]int, shapeList.Len())
	for i := 0; i < shapeList.Len(); i++ {
		_as1, _ := AsInteger(shapeList.Get(i))
		dims[i] = int(_as1)
		if dims[i] < 0 {
			return nil, r.AqlError("reshape_error", fmt.Sprintf("reshape: negative dimension %d", dims[i]), "reshape")
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
	list, _ := AsList(v)
	if list.IsNil() {
		return nil
	}
	var result []Value
	for i := 0; i < list.Len(); i++ {
		elem := list.Get(i)
		if elem.Parent.Matches(TList) && elem.Data != nil {
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

// ---- transpose ----

func arrTransposeHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	if !IsConcrete(args[0]) {
		return nil, r.AqlError("transpose_error", "transpose: expected concrete list", "transpose")
	}
	outer, _ := AsList(args[0])
	if outer.Len() == 0 {
		return []Value{NewList(nil)}, nil
	}
	first := outer.Get(0)
	if !first.Parent.Matches(TList) || !IsConcrete(first) {
		return nil, r.AqlError("transpose_error", "transpose: expected rank-2 list", "transpose")
	}
	_lst, _ := AsList(first)
	cols := _lst.Len()
	for i := 1; i < outer.Len(); i++ {
		sub := outer.Get(i)
		_subLst, _ := AsList(sub)
		if !sub.Parent.Matches(TList) || !IsConcrete(sub) || _subLst.Len() != cols {
			return nil, r.AqlError("transpose_error", "transpose: expected rectangular rank-2 list", "transpose")
		}
	}
	rows := outer.Len()
	result := make([]Value, cols)
	for c := 0; c < cols; c++ {
		row := make([]Value, rows)
		for r := 0; r < rows; r++ {
			_rowLst, _ := AsList(outer.Get(r))
			row[r] = _rowLst.Get(c)
		}
		result[c] = NewList(row)
	}
	return []Value{NewList(result)}, nil
}

// ---- reverse ----

func reverseHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	if !IsConcrete(args[0]) {
		return nil, r.AqlError("reverse_error", "reverse: expected concrete list", "reverse")
	}
	list, _ := AsList(args[0])
	n := list.Len()
	elems := make([]Value, n)
	for i := 0; i < n; i++ {
		elems[i] = list.Get(n - 1 - i)
	}
	return []Value{NewList(elems)}, nil
}

// ---- take ----

func takeHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	if !IsConcrete(args[1]) {
		return nil, r.AqlError("take_error", "take: expected concrete list", "take")
	}
	_as2, _ := args[0].AsConcreteInteger()
	n := int(_as2)
	list, _ := AsList(args[1])
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

func shedHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	if !IsConcrete(args[1]) {
		return nil, r.AqlError("shed_error", "shed: expected concrete list", "shed")
	}
	_as3, _ := args[0].AsConcreteInteger()
	n := int(_as3)
	list, _ := AsList(args[1])
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

func whereHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	if !IsConcrete(args[0]) {
		return nil, r.AqlError("where_error", "where: expected concrete list", "where")
	}
	list, _ := AsList(args[0])
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

func uniqueHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	if !IsConcrete(args[0]) {
		return nil, r.AqlError("unique_error", "unique: expected concrete list", "unique")
	}
	list, _ := AsList(args[0])
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

func gradeHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	if !IsConcrete(args[0]) {
		return nil, r.AqlError("grade_error", "grade: expected concrete list", "grade")
	}
	list, _ := AsList(args[0])
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

func atHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	if !IsConcrete(args[0]) {
		return nil, r.AqlError("at_error", "at: expected concrete indices list", "at")
	}
	if !IsConcrete(args[1]) {
		return nil, r.AqlError("at_error", "at: expected concrete data list", "at")
	}
	indices, _ := AsList(args[0])
	data, _ := AsList(args[1])
	dataLen := data.Len()
	result := make([]Value, indices.Len())
	for i := 0; i < indices.Len(); i++ {
		_as4, _ := AsInteger(indices.Get(i))
		idx := int(_as4)
		if idx < 0 || idx >= dataLen {
			return nil, r.AqlError("at_error", fmt.Sprintf("at: index %d out of bounds (length %d)", idx, dataLen), "at")
		}
		result[i] = data.Get(idx)
	}
	return []Value{NewList(result)}, nil
}

// ---- sortby ----

func sortbyHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	if !IsConcrete(args[0]) {
		return nil, r.AqlError("sortby_error", "sortby: expected concrete keys list", "sortby")
	}
	if !IsConcrete(args[1]) {
		return nil, r.AqlError("sortby_error", "sortby: expected concrete data list", "sortby")
	}
	keys, _ := AsList(args[0])
	data, _ := AsList(args[1])
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

func memberHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	if !IsConcrete(args[0]) {
		return nil, r.AqlError("member_error", "member: expected concrete needles list", "member")
	}
	if !IsConcrete(args[1]) {
		return nil, r.AqlError("member_error", "member: expected concrete haystack list", "member")
	}
	needles, _ := AsList(args[0])
	haystack, _ := AsList(args[1])
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

// ---- indexof (list overload) ----

// listIndexofHandler backs the [List, List] signature of the core
// indexof word (registered in native_string.go alongside the string
// overloads): for each needle, its index in the haystack, or the
// haystack length when absent.
func listIndexofHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	if !IsConcrete(args[0]) {
		return nil, r.AqlError("indexof_error", "indexof: expected concrete needles list", "indexof")
	}
	if !IsConcrete(args[1]) {
		return nil, r.AqlError("indexof_error", "indexof: expected concrete haystack list", "indexof")
	}
	needles, _ := AsList(args[0])
	haystack, _ := AsList(args[1])
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

func groupTwoHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	if !IsConcrete(args[0]) {
		return nil, r.AqlError("group_error", "group: expected concrete keys list", "group")
	}
	if !IsConcrete(args[1]) {
		return nil, r.AqlError("group_error", "group: expected concrete values list", "group")
	}
	keys, _ := AsList(args[0])
	values, _ := AsList(args[1])
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

func groupOneHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	if !IsConcrete(args[0]) {
		return nil, r.AqlError("group_error", "group: expected concrete list", "group")
	}
	list, _ := AsList(args[0])
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

func replicateHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	if !IsConcrete(args[0]) {
		return nil, r.AqlError("replicate_error", "replicate: expected concrete counts list", "replicate")
	}
	if !IsConcrete(args[1]) {
		return nil, r.AqlError("replicate_error", "replicate: expected concrete data list", "replicate")
	}
	counts, _ := AsList(args[0])
	data, _ := AsList(args[1])
	if counts.Len() != data.Len() {
		return nil, fmt.Errorf("replicate: counts length %d does not match data length %d", counts.Len(), data.Len())
	}
	var result []Value
	for i := 0; i < counts.Len(); i++ {
		_as5, _ := AsInteger(counts.Get(i))
		c := int(_as5)
		if c < 0 {
			return nil, r.AqlError("replicate_error", fmt.Sprintf("replicate: negative count %d at index %d", c, i), "replicate")
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

func expandHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	if !IsConcrete(args[0]) {
		return nil, r.AqlError("expand_error", "expand: expected concrete mask list", "expand")
	}
	if !IsConcrete(args[1]) {
		return nil, r.AqlError("expand_error", "expand: expected concrete data list", "expand")
	}
	mask, _ := AsList(args[0])
	data, _ := AsList(args[1])
	result := make([]Value, mask.Len())
	dataIdx := 0
	for i := 0; i < mask.Len(); i++ {
		if CoerceBoolean(mask.Get(i)) {
			if dataIdx >= data.Len() {
				return nil, r.AqlError("expand_error", "expand: not enough data elements for mask", "expand")
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

func windowHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	if !IsConcrete(args[1]) {
		return nil, r.AqlError("window_error", "window: expected concrete list", "window")
	}
	_as6, _ := args[0].AsConcreteInteger()
	size := int(_as6)
	list, _ := AsList(args[1])
	length := list.Len()
	if size <= 0 {
		return nil, r.AqlError("window_error", fmt.Sprintf("window: size must be positive, got %d", size), "window")
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

func pairsHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	if !IsConcrete(args[0]) {
		return nil, r.AqlError("pairs_error", "pairs: expected concrete list", "pairs")
	}
	list, _ := AsList(args[0])
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
		return nil, reg.AqlError("each_error", "each: expected concrete lists", "each")
	}
	_lst, _ := AsList(args[0])
	bodySlice := _lst.Slice()
	dataList, _ := AsList(args[1])

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
			return nil, reg.AqlError("each_error", fmt.Sprintf("each: element %d: body produced no result", i), "each")
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
	return []Value{NewCarrierTypedList(stk[len(stk)-1].Parent)}
}

// analyseHigherOrderBody runs a literal code-body list through a
// sub-engine in check mode, prepending the given element carrier
// type(s) so body words see realistic input carriers. Returns the
// residual carrier stack, or nil if the body is not concrete. The
// primary purpose is side-effect: any diagnostics the body produces
// (type mismatches, undefined words) are accumulated on the registry.
func analyseHigherOrderBody(r *Registry, body Value, elems ...*Type) []Value {
	if !IsConcrete(body) {
		return nil
	}
	bodyList, _ := AsList(body)
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
		r.Check.AddDiagnostic(CheckDiagnostic{
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
		return nil, reg.AqlError("fold_error", "fold: expected concrete lists", "fold")
	}
	_lst, _ := AsList(args[0])
	bodySlice := _lst.Slice()
	dataList, _ := AsList(args[1])
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
	stk := analyseHigherOrderBody(r, args[0], args[2].Parent, elem)
	if len(stk) == 0 {
		return []Value{NewCarrier(TAny)}
	}
	return []Value{stk[len(stk)-1]}
}

func foldNoInitHandler(args []Value, _ map[string]Value, _ []Value, reg *Registry) ([]Value, error) {
	if !IsConcrete(args[0]) || !IsConcrete(args[1]) {
		return nil, reg.AqlError("fold_error", "fold: expected concrete lists", "fold")
	}
	_lst, _ := AsList(args[0])
	bodySlice := _lst.Slice()
	dataList, _ := AsList(args[1])
	if dataList.Len() == 0 {
		return nil, reg.AqlError("fold_error", "fold: empty list with no initial value", "fold")
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
			return nil, reg.AqlError("fold_error", fmt.Sprintf("fold: step %d: body produced no result", i), "fold")
		}
		acc = res[len(res)-1]
	}
	return []Value{acc}, nil
}

// ---- scan ----

func scanHandler(args []Value, _ map[string]Value, _ []Value, reg *Registry) ([]Value, error) {
	if !IsConcrete(args[0]) || !IsConcrete(args[1]) {
		return nil, reg.AqlError("scan_error", "scan: expected concrete lists", "scan")
	}
	_lst, _ := AsList(args[0])
	bodySlice := _lst.Slice()
	dataList, _ := AsList(args[1])
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
			return nil, reg.AqlError("scan_error", fmt.Sprintf("scan: step %d: body produced no result", i), "scan")
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
	return []Value{NewCarrierTypedList(stk[len(stk)-1].Parent)}
}

// ---- outer ----

func outerHandler(args []Value, _ map[string]Value, _ []Value, reg *Registry) ([]Value, error) {
	if !IsConcrete(args[0]) || !IsConcrete(args[1]) || !IsConcrete(args[2]) {
		return nil, reg.AqlError("outer_error", "outer: expected concrete lists", "outer")
	}
	_lst, _ := AsList(args[0])
	bodySlice := _lst.Slice()
	left, _ := AsList(args[1])
	right, _ := AsList(args[2])

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
				return nil, reg.AqlError("outer_error", fmt.Sprintf("outer: (%d,%d): body produced no result", i, j), "outer")
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
		innerElem = stk[len(stk)-1].Parent
	}
	inner := NewCarrierTypedList(innerElem)
	return []Value{NewCarrierTypedListValue(inner)}
}

// ---- inner ----

func innerHandler(args []Value, _ map[string]Value, _ []Value, reg *Registry) ([]Value, error) {
	if !IsConcrete(args[0]) || !IsConcrete(args[1]) || !IsConcrete(args[2]) || !IsConcrete(args[3]) {
		return nil, reg.AqlError("inner_error", "inner: expected concrete lists", "inner")
	}
	_lst, _ := AsList(args[0])
	pairOp := _lst.Slice()
	_lst2, _ := AsList(args[1])
	aggOp := _lst2.Slice()
	left, _ := AsList(args[2])
	right, _ := AsList(args[3])

	// 1D case: zip then fold
	if left.Len() > 0 && !left.Get(0).Parent.Matches(TList) {
		if left.Len() != right.Len() {
			return nil, reg.AqlError("inner_error", "inner: vectors must have same length", "inner")
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
				return nil, reg.AqlError("inner_error", fmt.Sprintf("inner: pair %d: no result", i), "inner")
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
				return nil, reg.AqlError("inner_error", fmt.Sprintf("inner: fold %d: no result", i), "inner")
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
		leftRow, _ := AsList(left.Get(i))
		cols := make([]Value, len(rightCols))
		for j := 0; j < len(rightCols); j++ {
			rightCol := rightCols[j]
			if leftRow.Len() != len(rightCol) {
				return nil, reg.AqlError("inner_error", "inner: dimension mismatch", "inner")
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
					return nil, reg.AqlError("inner_error", fmt.Sprintf("inner: pair (%d,%d,%d): no result", i, j, k), "inner")
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
					return nil, reg.AqlError("inner_error", fmt.Sprintf("inner: fold (%d,%d,%d): no result", i, j, k), "inner")
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
	firstRow, _ := AsList(rows.Get(0))
	cols := firstRow.Len()
	result := make([][]Value, cols)
	for j := 0; j < cols; j++ {
		result[j] = make([]Value, rows.Len())
	}
	for i := 0; i < rows.Len(); i++ {
		row, _ := AsList(rows.Get(i))
		for j := 0; j < cols && j < row.Len(); j++ {
			result[j][i] = row.Get(j)
		}
	}
	return result
}
