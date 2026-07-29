package native

import (
	"fmt"
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

// branchSharingViolation reports the binding name and container kind of
// the first STATEFUL MUTABLE container an `await` branch body can reach
// from the parent scope, or ("", "") when every branch is clean.
//
// ForkConcurrent isolates execution SCOPES — a `def` or `context set` on
// a branch stays private — and its header says exactly that. It does not
// isolate bound VALUES: cloning a binding table copies Values, and a
// FlexMap / FlexList / Store / class-instance / Table Value holds a
// POINTER to shared state. Two branches mutating one such container race
// on `OrderedMap.Set`, an unsynchronised map assign plus a slice append,
// which is undefined behaviour and not merely a surprise.
//
// The rule is `send`'s — refuse a mutable container at a concurrency
// boundary — but the PREDICATE has to be its own, and the reason is worth
// stating because it is easy to get wrong by reaching for the obvious
// reuse. `send` asks "can this be COPIED across?" and deep-copies what it
// admits (CloneValue at the boundary), so it only refuses containers a
// copy cannot preserve. `await` asks "can this be SHARED across?" and
// copies nothing, so it must refuse anything mutated in place — a
// strictly broader set.
//
// Concretely, sendableViolation misses FlexMap: a FlexMap is
// `MapPayload{*OrderedMap}` with a TFlexMap tag, payload-identical to an
// immutable Map, so a payload-keyed switch reads it as a plain map. That
// is harmless for `send` (the clone makes it safe either way) and fatal
// here. So this discriminates on the TYPE TAG, which is what actually
// separates in-place mutation from copy-on-write.
//
// LIMITS, stated rather than implied — this is a boundary rule, not a
// proof of non-aliasing. Reachability is what a branch body REFERENCES,
// followed one level into any referenced function (its captures and its
// body). Two things it does not see:
//
//   - a container reached only through a longer chain of calls;
//   - a binding that is not in `r.Defs` at await time. Compiled, some
//     fn-body locals live in frame slots rather than the def table, so a
//     lambda defined INSIDE a fn body can be unbound here.
//
// The second is an engine-mode divergence in the check itself, and it is
// worse than "some shapes are missed" — it misses exactly the dangerous
// ones. With `def m (make FlexMap {a:0})` inside a fn body and both
// branches invoking the same fn-local lambda:
//
//	lambda body     compiled     interpreted
//	[m]             refused      refused
//	[m get a]       refused      refused
//	[m size]        ALLOWED      refused
//	[m set a 1]     ALLOWED      refused   <-- real DATA RACE under -race
//
// So the compiled path refuses the harmless shapes and permits the
// mutating one, which is undefined behaviour on the path that ships by
// default. Naming `m` directly in the branch is caught on both paths, as
// is the same lambda at MODULE level, so the hole is bounded — but it is
// a live memory-safety hole, not a cosmetic divergence.
//
// Pinned by TestAwaitRefusalMissesCompiledFnLocalLambda (which uses the
// non-racing `[m size]` shape, so `make test-race` stays usable) and by
// TestAwaitRefusalCatchesFnLocalLambdaReads (the refused half). Closing
// it means reaching compiled frame locals from a native handler, which is
// a different layer's job. See design/verse-report-defects-investigation.0.md §A.
func sharedMutableKind(v Value) string {
	if v.Parent == nil {
		return ""
	}
	switch {
	case v.Parent.ConformsTo(TFlexMap), v.Parent.ConformsTo(TWeakFlexMap):
		return "FlexMap"
	case v.Parent.ConformsTo(TFlexList), v.Parent.ConformsTo(TWeakFlexList):
		return "FlexList"
	case v.Parent.ConformsTo(TStore):
		return "Store"
	case v.Parent.ConformsTo(TTable):
		return "Table"
	}
	switch d := v.Data.(type) {
	case ClassInstanceInfo:
		return "Object"
	case ListPayload:
		for _, e := range d.Elems {
			if k := sharedMutableKind(e); k != "" {
				return k
			}
		}
	case MapPayload:
		if d.M != nil {
			for _, key := range d.M.Keys() {
				e, _ := d.M.Get(key)
				if k := sharedMutableKind(e); k != "" {
					return k
				}
			}
		}
	}
	return ""
}

// branchTokens returns a branch element's body tokens for either shape:
// the raw token list (interpreted) or the AQLImpl body carried by a
// compiled fn-value (the shape runParallelBranch unwraps via RunUnit).
func branchTokens(elem Value) []Value {
	if fd, ok := elem.Data.(eng.FnDefInfo); ok {
		for i := range fd.Signatures {
			if a, isAQL := fd.Signatures[i].Impl.(*eng.AQLImpl); isAQL {
				return a.Body
			}
		}
		return nil
	}
	body, err := AsList(elem)
	if err != nil || body.IsNil() {
		return nil
	}
	return body.Slice()
}

func branchSharingViolation(r *Registry, elems []Value) (string, string) {
	seen := map[string]bool{}
	var offender, kind string
	inspect := func(name string, v Value) bool {
		if bad := sharedMutableKind(v); bad != "" {
			offender, kind = name, bad
			return true
		}
		return false
	}
	// A branch body arrives in one of two shapes, and checking only the
	// obvious one is how this check first shipped inert: interpreted, the
	// element IS the token list; COMPILED, it is a synthetic fn-value
	// carrier holding the tokens in its AQLImpl body (the
	// CompileStoresBodyList spawn pattern runParallelBranch unwraps). Miss
	// the second and the boundary is unguarded on the default path — the
	// only path most programs take.
	var work [][]Value
	for _, elem := range elems {
		if toks := branchTokens(elem); len(toks) > 0 {
			work = append(work, toks)
		}
	}
	// Referenced FUNCTIONS are followed into, because both ways a lambda
	// can reach a container cross the boundary just as surely as a
	// directly-named one — and they are different mechanisms:
	//
	//   - CAPTURED, when the container is a binding of an enclosing fn.
	//     It travels inside the Function value.
	//   - DYNAMIC, when the container is module-level. Nothing is
	//     captured (CLAUDE.md "What gets captured": a module-level def
	//     stays dynamic), so it is reachable ONLY through the body's
	//     word references — which is exactly the shape `def w ([] => [m
	//     set a 1])` takes at the top level, the one a user is most
	//     likely to write.
	//
	// `seen` bounds the walk, so a recursive or mutually-recursive fn
	// terminates rather than looping.
	for len(work) > 0 && offender == "" {
		tokens := work[0]
		work = work[1:]
		WalkBodyWords(tokens, func(w WordInfo, _ Value) {
			if offender != "" || seen[w.Name] {
				return
			}
			seen[w.Name] = true
			bound, ok := r.Defs.Top(w.Name)
			if !ok {
				return
			}
			if inspect(w.Name, bound) {
				return
			}
			if fd, isFn := bound.Data.(FnDefInfo); isFn {
				for _, c := range fd.Captured {
					if inspect(c.Name, c.Value) {
						return
					}
				}
				for i := range fd.Signatures {
					if a, isAQL := fd.Signatures[i].Impl.(*eng.AQLImpl); isAQL {
						work = append(work, a.Body)
					}
				}
			}
		})
	}
	return offender, kind
}

// makeBranchForks builds one isolated registry per branch so the
// branches can run concurrently without racing on the shared registry's
// mutable stacks. A single SyncWriter is shared across the forks so
// concurrent prints to the parent's output are serialized rather than
// interleaved or raced. Forking happens here, on the dispatching
// goroutine, while the parent is not yet parked — ForkConcurrent's
// reads of the parent state are therefore race-free.
//
// It also enforces the branch boundary: a reachable mutable container is
// REFUSED rather than shared or silently copied. Refusing is what makes
// the documented guarantee ("writes to mutable objects inside one branch
// do not bleed into the others") true and checkable, and it is the answer
// `send` already gives at the process boundary. Deep-copying instead
// would make the same programs work silently, but it hides the sharing
// the author wrote and pays a per-branch copy for every container in
// scope.
func makeBranchForks(r *Registry, elems []Value) ([]*Registry, error) {
	if name, kind := branchSharingViolation(r, elems); name != "" {
		return nil, r.AqlErrorHint("not_sendable",
			fmt.Sprintf("await: branch reaches `%s`, a mutable %s — a stateful "+
				"container cannot be shared across concurrent branches", name, kind),
			"await",
			"branches run on separate goroutines, so an in-place write to a "+
				"shared container is a data race; pass an immutable List/Map, or "+
				"build the container inside each branch and combine the results")
	}
	out := NewSyncWriter(r.Output)
	errOut := NewSyncWriter(r.ErrOutput)
	forks := make([]*Registry, len(elems))
	for i := range forks {
		f := r.ForkConcurrent()
		f.Output = out
		f.ErrOutput = errOut
		forks[i] = f
	}
	return forks, nil
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
	forks, ferr := makeBranchForks(r, elems)
	if ferr != nil {
		return nil, ferr
	}
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
	forks, ferr := makeBranchForks(r, elems)
	if ferr != nil {
		return nil, ferr
	}
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
	forks, ferr := makeBranchForks(r, elems)
	if ferr != nil {
		return nil, ferr
	}
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

	forks, ferr := makeBranchForks(r, elems)
	if ferr != nil {
		return nil, ferr
	}
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
