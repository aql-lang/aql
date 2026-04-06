package engine

import (
	"fmt"
	"sync"
)

// registerAwait registers the "await" word.
//
// await: [(make Options {mode:'all'})? parallels:[:Any]/q] → results
//
// Runs each element of the parallels list in parallel using do semantics.
// Each element should be a list (code body) that is evaluated in its own
// sub-engine goroutine. Elements that produce an error value "reject".
//
// Modes (correspond to JS Promise utility functions):
//
//   - 'all (default): waits for all to succeed. If any reject, returns the
//     first error. Otherwise returns a list of all results.
//   - 'full (JS allSettled): waits for all to complete. Returns a list of
//     maps, each with {status:'ok, value:...} or {status:'error, value:...}.
//   - 'first (JS race): returns the result of whichever finishes first,
//     whether success or error.
//   - 'any: returns the first successful result. If all reject, returns
//     the last error.
func registerAwait(r *Registry) {
	awaitHandler := func(mode string, parallels Value) ([]Value, error) {
		if parallels.Data == nil {
			return nil, fmt.Errorf("await: parallels must be a concrete list, got type literal")
		}
		elems := parallels.AsList().Slice()
		if len(elems) == 0 {
			return []Value{NewList([]Value{})}, nil
		}

		switch mode {
		case "all":
			return awaitAll(r, elems)
		case "full":
			return awaitFull(r, elems)
		case "first":
			return awaitFirst(r, elems)
		case "any":
			return awaitAny(r, elems)
		default:
			return nil, fmt.Errorf("await: unknown mode %q, expected all, full, first, or any", mode)
		}
	}

	withOptsHandler := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		// args[0] = Options, args[1] = List (parallels)
		mode := "all"
		if oi, err := args[0].AsOptionsType(); err == nil {
			if v, ok := oi.Fields.Get("mode"); ok {
				if s, err := v.AsString(); err == nil {
					mode = s
				} else if a, err := v.AsAtom(); err == nil {
					mode = a
				}
			}
		} else if optsMap := args[0].AsMap(); optsMap != nil {
			if v, ok := optsMap.Get("mode"); ok {
				if s, err := v.AsString(); err == nil {
					mode = s
				} else if a, err := v.AsAtom(); err == nil {
					mode = a
				}
			}
		}
		return awaitHandler(mode, args[1])
	}

	defaultHandler := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		return awaitHandler("all", args[0])
	}

	r.RegisterNativeFunc(NativeFunc{
		Name:              "await",
		ForwardPrecedence: true,
		Signatures: []NativeSig{
			{
				Args:       []Type{TOptions, TList},
				NoEvalArgs: map[int]bool{1: true},
				Handler:    withOptsHandler,
			},
			{
				Args:       []Type{TList},
				NoEvalArgs: map[int]bool{0: true},
				Handler:    defaultHandler,
			},
		},
	})
}

// parallelResult holds the outcome of one parallel branch.
type parallelResult struct {
	index  int
	values []Value
	err    bool // true if the branch produced an error value or runtime error
}

// runParallelBranch executes one element with do semantics.
// If the element is a list, it runs as a sub-program.
// Otherwise, it is returned as a single value.
func runParallelBranch(r *Registry, elem Value) parallelResult {
	if elem.VType.Matches(TList) && elem.Data != nil && !elem.IsTypedList() && !elem.IsTableType() {
		sub := New(r)
		body := elem.AsList().Slice()
		input := make([]Value, len(body))
		copy(input, body)
		result, runErr := sub.Run(input)
		if runErr != nil {
			return parallelResult{values: []Value{NewError(runErr)}, err: true}
		}
		// Check if the result is a single error value.
		if len(result) == 1 && result[0].IsError() {
			return parallelResult{values: result, err: true}
		}
		return parallelResult{values: result}
	}
	// Non-list element: just return it as-is.
	return parallelResult{values: []Value{elem}}
}

// awaitAll waits for all branches to succeed. Returns the first error if any reject.
func awaitAll(r *Registry, elems []Value) ([]Value, error) {
	results := make([]parallelResult, len(elems))
	var wg sync.WaitGroup
	wg.Add(len(elems))

	for i, elem := range elems {
		go func(idx int, e Value) {
			defer wg.Done()
			pr := runParallelBranch(r, e)
			pr.index = idx
			results[idx] = pr
		}(i, elem)
	}
	wg.Wait()

	// If any rejected, return the first error.
	for _, pr := range results {
		if pr.err {
			return pr.values, nil
		}
	}

	// Collect all results into a list. Each branch's result is unwrapped
	// if it produced exactly one value.
	out := make([]Value, len(results))
	for i, pr := range results {
		if len(pr.values) == 1 {
			out[i] = pr.values[0]
		} else {
			out[i] = NewList(pr.values)
		}
	}
	return []Value{NewList(out)}, nil
}

// awaitFull waits for all branches to complete and returns a list of
// {status:'ok, value:...} or {status:'error, value:...} maps.
func awaitFull(r *Registry, elems []Value) ([]Value, error) {
	results := make([]parallelResult, len(elems))
	var wg sync.WaitGroup
	wg.Add(len(elems))

	for i, elem := range elems {
		go func(idx int, e Value) {
			defer wg.Done()
			pr := runParallelBranch(r, e)
			pr.index = idx
			results[idx] = pr
		}(i, elem)
	}
	wg.Wait()

	out := make([]Value, len(results))
	for i, pr := range results {
		m := NewOrderedMap()
		if pr.err {
			m.Set("status", NewAtom("error"))
		} else {
			m.Set("status", NewAtom("ok"))
		}
		if len(pr.values) == 1 {
			m.Set("value", pr.values[0])
		} else {
			m.Set("value", NewList(pr.values))
		}
		out[i] = NewMap(m)
	}
	return []Value{NewList(out)}, nil
}

// awaitFirst returns the result of whichever branch finishes first.
func awaitFirst(r *Registry, elems []Value) ([]Value, error) {
	ch := make(chan parallelResult, len(elems))
	for i, elem := range elems {
		go func(idx int, e Value) {
			pr := runParallelBranch(r, e)
			pr.index = idx
			ch <- pr
		}(i, elem)
	}
	first := <-ch
	return first.values, nil
}

// awaitAny returns the first successful result. If all reject, returns
// the last error.
func awaitAny(r *Registry, elems []Value) ([]Value, error) {
	type indexedResult struct {
		pr    parallelResult
		order int // completion order
	}

	ch := make(chan indexedResult, len(elems))
	for i, elem := range elems {
		go func(idx int, e Value) {
			pr := runParallelBranch(r, e)
			pr.index = idx
			ch <- indexedResult{pr: pr}
		}(i, elem)
	}

	var lastErr parallelResult
	errCount := 0
	for range elems {
		ir := <-ch
		if !ir.pr.err {
			return ir.pr.values, nil
		}
		lastErr = ir.pr
		errCount++
	}
	// All rejected — return the last error.
	return lastErr.values, nil
}
