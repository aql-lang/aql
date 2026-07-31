package native

import (
	"fmt"
	"sort"
	"sync"

	eng "github.com/boru-lang/boru/eng/go"
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
// body), plus a scope-wide fallback when a reference cannot be resolved
// at all (branchRefIsOpaque / scopeSharedMutable).
//
// What it does NOT see: a container reached only through a longer chain
// of calls, and one held in a VM frame slot with nothing mutable visible
// in `r.Defs` to trip the opaque fallback.
//
// The fn-local hole this used to have is CLOSED. Compiled, `def w ([] =>
// [m set a 1])` inside a fn body puts `w` in a frame slot, so the walk
// dead-ended at `w`; the check then refused the harmless shapes (`[m]`,
// `[m get a]`) and permitted the mutating one — a real data race under
// `-race`, on the default path. All four shapes now refuse in both
// engines, pinned by TestAwaitRefusesFnLocalLambdaInBothEngines, with
// TestAwaitOpaqueRefIsNotRefusedWithoutAMutable holding the other side so
// the fallback cannot grow into refusing map keys and branch-locals.
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
// the raw token list (interpreted) or the BORUImpl body carried by a
// compiled fn-value (the shape runParallelBranch unwraps via RunUnit).
func branchTokens(elem Value) []Value {
	if fd, ok := elem.Data.(eng.FnDefInfo); ok {
		for i := range fd.Signatures {
			if a, isBORU := fd.Signatures[i].Impl.(*eng.BORUImpl); isBORU {
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

// branchRefIsOpaque reports whether a branch-body word resolves to
// NOTHING the check can inspect.
//
// It is consulted only for a name the walk already failed to resolve in
// `r.Defs`, and that failure is the whole signal: compiled, a fn-body
// local is exactly what it looks like — `def w ([] => …)` inside a fn
// body lives in a VM frame slot, not in r.Defs, and a native handler has
// no path to frame locals.
//
// The one exception is a bare LITERAL. `true` / `false` / `none` arrive
// as Words and never resolve, but they are values, not bindings — a
// branch body of `[true]` reaches nothing. Without this case an
// `await [[true] [false]]` with any FlexMap in scope would be refused
// outright.
//
// Registered words and type names deliberately have no case here. Both
// were tried, and both are unreachable from this call site: every
// registered word also carries a `r.Defs` binding (and `undef` of a
// builtin is refused as `reserved_word`), while a kernel type name is
// converted by the parser into a type literal and never arrives as a
// Word at all. Measured across the whole Go test corpus, every name
// reaching this function misses both lookups. A guard that cannot fire
// is worse than no guard: it reads as protection that isn't there.
func branchRefIsOpaque(name string) bool {
	switch name {
	case "true", "false", "none":
		return false
	}
	return true
}

// scopeSharedMutable returns the first binding in scope that is an
// in-place-mutable container. It answers the only question that matters
// once a branch reference has gone opaque: is there anything here for it
// to reach?
func scopeSharedMutable(r *Registry) (string, string) {
	names := r.Defs.Names()
	sort.Strings(names) // deterministic: the same program names the same binding
	for _, n := range names {
		if v, ok := r.Defs.Top(n); ok {
			if bad := sharedMutableKind(v); bad != "" {
				return n, bad
			}
		}
	}
	return "", ""
}

func branchSharingViolation(r *Registry, elems []Value) (string, string) {
	seen := map[string]bool{}
	var offender, kind string
	// opaque records a branch reference the check could not resolve. It is
	// not a violation by itself — a map key (`m set a 1` walks `a` as a bare
	// Word) and a branch-local `def` are both opaque and both harmless.
	var opaque string
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
	// carrier holding the tokens in its BORUImpl body (the
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
				if branchRefIsOpaque(w.Name) {
					opaque = w.Name
				}
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
					if a, isBORU := fd.Signatures[i].Impl.(*eng.BORUImpl); isBORU {
						work = append(work, a.Body)
					}
				}
			}
		})
	}
	if offender != "" {
		return offender, kind
	}
	// Nothing was reached DIRECTLY, but a reference went opaque. The check
	// cannot bound what that reference reaches, so fall back to the only
	// sound question left: is there a mutable container in scope at all?
	//
	// This is what closes the compiled fn-local hole. `def w ([] => [m set
	// a 1])` inside a fn body compiles `w` into a frame slot, so the walk
	// dead-ends at `w` — while `m` sits in r.Defs the whole time, one step
	// out of reach. Two branches then wrote the same OrderedMap
	// concurrently on the DEFAULT path: a real data race, not a
	// theoretical one.
	//
	// It is deliberately conditional on something mutable existing. An
	// unconditional "opaque reference => refuse" would reject the two
	// shapes the docs promote — `[m set a 1]` over an IMMUTABLE map (the
	// key `a` walks as a bare Word) and `[def a (make FlexMap {}) a set k
	// 1]` (branch-local, private by construction) — because a map key and
	// a branch-local binding are indistinguishable from a fn-local
	// function here.
	//
	// The residual imprecision is stated rather than hidden: a mutable
	// container in scope that the opaque reference could not actually have
	// reached is refused anyway. That is the conservative direction, and
	// it is the direction this boundary already chose over cloning.
	if opaque != "" {
		if name, bad := scopeSharedMutable(r); name != "" {
			return name, bad
		}
	}
	return "", ""
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
		return nil, r.BoruErrorHint("not_sendable",
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
				if a, isBORU := fd.Signatures[i].Impl.(*eng.BORUImpl); isBORU {
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
