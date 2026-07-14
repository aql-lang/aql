package native

import (
	"sync"

	eng "github.com/aql-lang/aql/eng/go"
)

// The "await" word lives in miscNatives (native_misc.go). Below
// are the parallel-branch helpers used by the dispatcher.

// parallelResult holds the outcome of one parallel branch.
type parallelResult struct {
	index  int
	values []Value
	err    bool // true if the branch produced an error value or runtime error
}

// makeBranchForks builds one isolated registry per branch so the
// branches can run concurrently without racing on the shared registry's
// mutable stacks. A single SyncWriter is shared across the forks so
// concurrent prints to the parent's output are serialized rather than
// interleaved or raced. Forking happens here, on the dispatching
// goroutine, while the parent is not yet parked — ForkConcurrent's
// reads of the parent state are therefore race-free.
func makeBranchForks(r *Registry, n int) []*Registry {
	out := NewSyncWriter(r.Output)
	errOut := NewSyncWriter(r.ErrOutput)
	forks := make([]*Registry, n)
	for i := range forks {
		f := r.ForkConcurrent()
		f.Output = out
		f.ErrOutput = errOut
		forks[i] = f
	}
	return forks
}

// runParallelBranch executes one element with do semantics on its own
// isolated registry fork. A COMPILED branch body arrives as a synthetic
// fn-value carrier with a CompiledFnRef (CompileStoresBodyList — the spawn
// pattern) and runs via RunUnit on the fork; a raw list runs as an
// interpreter sub-program; anything else is returned as a single value.
func runParallelBranch(reg *Registry, elem Value) parallelResult {
	if fd, ok := elem.Data.(eng.FnDefInfo); ok {
		for i := range fd.Signatures {
			ref := fd.Signatures[i].CompiledRef()
			if ref == nil || ref.Prog == nil {
				continue
			}
			effectsAt := reg.Effects.Count()
			result, runErr := eng.RunUnit(ref, reg, nil)
			if eng.IsInternalError(runErr) && reg.Effects.Count() == effectsAt {
				// A VM soundness bail with NO observable effect: re-run the raw
				// tokens on the interpreter, exactly as the branch would have run
				// without the stamp (the C1 fence — see InvokeCallback).
				if a, isAQL := fd.Signatures[i].Impl.(*eng.AQLImpl); isAQL {
					return interpretBranchBody(reg, a.Body)
				}
			}
			return branchOutcome(result, runErr)
		}
	}
	if elem.Parent.ConformsTo(TList) && elem.Data != nil && !IsTypedList(elem) && !IsTableType(elem) {
		_lst, _ := AsList(elem)
		return interpretBranchBody(reg, _lst.Slice())
	}
	// Non-list element: just return it as-is.
	return parallelResult{values: []Value{elem}}
}

// interpretBranchBody runs a branch's raw token list on a fresh interpreter
// sub-engine over the fork — the pre-stamping branch path, byte-identical.
func interpretBranchBody(reg *Registry, body []Value) parallelResult {
	sub := New(reg)
	input := make([]Value, len(body))
	copy(input, body)
	result, runErr := sub.Run(input)
	return branchOutcome(result, runErr)
}

// branchOutcome maps a branch run's (result, error) to a parallelResult —
// one shared mapping so the compiled and interpreted paths agree exactly.
func branchOutcome(result []Value, runErr error) parallelResult {
	if runErr != nil {
		return parallelResult{values: []Value{NewError(runErr)}, err: true}
	}
	if len(result) == 1 && IsError(result[0]) {
		return parallelResult{values: result, err: true}
	}
	return parallelResult{values: result}
}

// awaitAll waits for all branches to succeed. Returns the first error if any reject.
func awaitAll(r *Registry, elems []Value) ([]Value, error) {
	results := make([]parallelResult, len(elems))
	forks := makeBranchForks(r, len(elems))
	var wg sync.WaitGroup
	wg.Add(len(elems))

	for i, elem := range elems {
		go func(idx int, e Value, reg *Registry) {
			defer wg.Done()
			pr := runParallelBranch(reg, e)
			pr.index = idx
			results[idx] = pr
		}(i, elem, forks[i])
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
	forks := makeBranchForks(r, len(elems))
	var wg sync.WaitGroup
	wg.Add(len(elems))

	for i, elem := range elems {
		go func(idx int, e Value, reg *Registry) {
			defer wg.Done()
			pr := runParallelBranch(reg, e)
			pr.index = idx
			results[idx] = pr
		}(i, elem, forks[i])
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
	forks := makeBranchForks(r, len(elems))
	ch := make(chan parallelResult, len(elems))
	for i, elem := range elems {
		go func(idx int, e Value, reg *Registry) {
			pr := runParallelBranch(reg, e)
			pr.index = idx
			ch <- pr
		}(i, elem, forks[i])
	}
	first := <-ch
	return first.values, nil
}

// awaitAny returns the first successful result. If all reject, returns
// the last error.
func awaitAny(r *Registry, elems []Value) ([]Value, error) {
	type indexedResult struct {
		pr parallelResult
	}

	forks := makeBranchForks(r, len(elems))
	ch := make(chan indexedResult, len(elems))
	for i, elem := range elems {
		go func(idx int, e Value, reg *Registry) {
			pr := runParallelBranch(reg, e)
			pr.index = idx
			ch <- indexedResult{pr: pr}
		}(i, elem, forks[i])
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
