package engine

import "fmt"

// registerEach registers the "each" word.
// each applies a quoted code body to each element of a data list, collecting results.
//   each [dup add] [1 2 3]  →  [2 4 6]
func registerEach(r *Registry) {
	r.RegisterNativeFunc(NativeFunc{
		Name:              "each",
		ForwardPrecedence: true,
		Signatures: []NativeSig{{
			Args:       []Type{TList, TList},
			NoEvalArgs: map[int]bool{0: true},
			Handler: func(args []Value, _ map[string]Value, _ []Value, reg *Registry) ([]Value, error) {
				if args[0].Data == nil || args[1].Data == nil {
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
			},
			// each returns a list whose element type is whatever the
			// body's top-of-stack produces. Pass the concrete data
			// list's element carrier into the body so diagnostics
			// fire against realistic types.
			ReturnsFn: func(args []Value) []Value {
				elem := dataListElemType(args[1])
				stk := analyseHigherOrderBody(r, args[0], elem)
				if len(stk) == 0 {
					return []Value{NewCarrier(TList)}
				}
				return []Value{NewCarrierTypedList(stk[len(stk)-1].VType)}
			},
		}},
	})
}

// dataListElemType inspects a data list Value and returns a
// conservative element type. Handles three cases:
//
//  1. Typed-list carrier (ChildTypeInfo{Child: ...}): return the
//     child carrier's VType directly.
//  2. Concrete list with real elements: join all element VTypes
//     via commonAncestorType.
//  3. Empty / non-concrete / nil-data list: TAny.
func dataListElemType(data Value) Type {
	if data.Data == nil {
		return TAny
	}
	if ct, ok := data.Data.(ChildTypeInfo); ok {
		return ct.Child.VType
	}
	list := data.AsList()
	if list.IsNil() || list.Len() == 0 {
		return TAny
	}
	t := list.Get(0).VType
	for i := 1; i < list.Len(); i++ {
		t = commonAncestorType(t, list.Get(i).VType)
		if t.Equal(TAny) {
			break
		}
	}
	return t
}

// analyseHigherOrderBody runs a literal code-body list through a
// sub-engine in check mode, prepending the given element carrier
// type(s) so body words see realistic input carriers. Returns the
// residual carrier stack, or nil if the body is not concrete. The
// primary purpose is side-effect: any diagnostics the body produces
// (type mismatches, undefined words) are accumulated on the registry.
func analyseHigherOrderBody(r *Registry, body Value, elems ...Type) []Value {
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
		r.addCheckDiagnostic(CheckDiagnostic{
			Code:   "body_error",
			Detail: "higher-order body analysis error: " + err.Error(),
		})
		return nil
	}
	return result
}

// registerFold registers the "fold" word with two signatures.
// With initial value:  0 fold [add] [1 2 3]  →  6
// Without initial:     fold [add] [1 2 3]    →  6  (uses first element as init)
func registerFold(r *Registry) {
	r.RegisterNativeFunc(NativeFunc{
		Name:              "fold",
		ForwardPrecedence: true,
		Signatures: []NativeSig{
			{
				// With initial value: init body data → result
				Args:       []Type{TAny, TList, TList},
				NoEvalArgs: map[int]bool{1: true},
				Handler: func(args []Value, _ map[string]Value, _ []Value, reg *Registry) ([]Value, error) {
					// args[0]=init, args[1]=body, args[2]=data
					if args[1].Data == nil || args[2].Data == nil {
						return nil, fmt.Errorf("fold: expected concrete lists")
					}
					init := args[0]
					bodySlice := args[1].AsList().Slice()
					dataList := args[2].AsList()
					return doFold(reg, init, bodySlice, dataList)
				},
				// Fold result type is the body's output. Analyse the
				// body once with (init, element) as carrier inputs;
				// use the residual top-of-stack carrier as the
				// result. A proper fixed-point would iterate until
				// the accumulator type stabilises — one pass is a
				// close approximation for bounded-lattice types.
				ReturnsFn: func(args []Value) []Value {
					elem := dataListElemType(args[2])
					stk := analyseHigherOrderBody(r, args[1], args[0].VType, elem)
					if len(stk) == 0 {
						return []Value{NewCarrier(TAny)}
					}
					return []Value{stk[len(stk)-1]}
				},
			},
			{
				// Without initial: body data → result (uses first element as init)
				Args:       []Type{TList, TList},
				NoEvalArgs: map[int]bool{0: true},
				Handler: func(args []Value, _ map[string]Value, _ []Value, reg *Registry) ([]Value, error) {
					if args[0].Data == nil || args[1].Data == nil {
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
					restList := ReadList{elems: rest}
					return doFold(reg, init, bodySlice, restList)
				},
				// No init — accumulator type and element type both
				// come from the data list.
				ReturnsFn: func(args []Value) []Value {
					elem := dataListElemType(args[1])
					stk := analyseHigherOrderBody(r, args[0], elem, elem)
					if len(stk) == 0 {
						return []Value{NewCarrier(TAny)}
					}
					return []Value{stk[len(stk)-1]}
				},
			},
		},
	})
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

// registerScan registers the "scan" word.
// scan is a running reduction: first element is the initial accumulator,
// each step produces an intermediate result.
//   scan [add] [1 2 3 4]  →  [1 3 6 10]
func registerScan(r *Registry) {
	r.RegisterNativeFunc(NativeFunc{
		Name:              "scan",
		ForwardPrecedence: true,
		Signatures: []NativeSig{{
			Args:       []Type{TList, TList},
			NoEvalArgs: map[int]bool{0: true},
			Handler: func(args []Value, _ map[string]Value, _ []Value, reg *Registry) ([]Value, error) {
				if args[0].Data == nil || args[1].Data == nil {
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
			},
			ReturnsFn: func(args []Value) []Value {
				elem := dataListElemType(args[1])
				stk := analyseHigherOrderBody(r, args[0], elem, elem)
				if len(stk) == 0 {
					return []Value{NewCarrier(TList)}
				}
				return []Value{NewCarrierTypedList(stk[len(stk)-1].VType)}
			},
		}},
	})
}

// registerOuter registers the "outer" word.
// outer applies an operation body to every pair (l, r) from left and right arrays,
// producing a 2D nested list.
//   outer [add] [1 2] [10 20]  →  [[11 21] [12 22]]
func registerOuter(r *Registry) {
	r.RegisterNativeFunc(NativeFunc{
		Name:              "outer",
		ForwardPrecedence: true,
		Signatures: []NativeSig{{
			Args:       []Type{TList, TList, TList},
			NoEvalArgs: map[int]bool{0: true},
			Handler: func(args []Value, _ map[string]Value, _ []Value, reg *Registry) ([]Value, error) {
			if args[0].Data == nil || args[1].Data == nil || args[2].Data == nil {
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
			},
			ReturnsFn: func(args []Value) []Value {
				leftElem := dataListElemType(args[1])
				rightElem := dataListElemType(args[2])
				// outer produces a 2D list (rows of body results).
				// The carrier model flattens that to TList (list of
				// lists); refining to a nested typed list is
				// future work.
				analyseHigherOrderBody(r, args[0], leftElem, rightElem)
				return []Value{NewCarrierTypedList(TList)}
			},
		}},
	})
}

// registerInner registers the "inner" word.
// inner performs an inner-product-style operation using a pair-op and an aggregate-op.
// For 1D vectors: zip with pair-op, then fold with agg-op.
// For 2D matrices: matrix inner product (left rows × right columns).
//   inner [mul] [add] [1 2 3] [4 5 6]  →  32  (dot product)
func registerInner(r *Registry) {
	r.RegisterNativeFunc(NativeFunc{
		Name:              "inner",
		ForwardPrecedence: true,
		Signatures: []NativeSig{{
			Args:       []Type{TList, TList, TList, TList},
			NoEvalArgs: map[int]bool{0: true, 1: true},
			Handler: func(args []Value, _ map[string]Value, _ []Value, reg *Registry) ([]Value, error) {
			if args[0].Data == nil || args[1].Data == nil || args[2].Data == nil || args[3].Data == nil {
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
			},
			ReturnsFn: func(args []Value) []Value {
				leftElem := dataListElemType(args[2])
				rightElem := dataListElemType(args[3])
				// pair op consumes (left-elem, right-elem); agg
				// consumes (accumulator, pair-result). Without
				// carrier list element tracking we use the pair
				// output as TAny for the agg input.
				analyseHigherOrderBody(r, args[0], leftElem, rightElem)
				analyseHigherOrderBody(r, args[1], TAny, TAny)
				return []Value{NewCarrier(TList)}
			},
		}},
	})
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
