package native

import (
	"fmt"
	"time"

	voxgigstruct "github.com/voxgig/struct"
)

// Natives is the consolidated NativeFunc slice for the data-manipulation
// words owned by the native package. It replaces the per-word RegisterFoo
// functions and their aggregator (registerAll). The public Register entry
// point in native.go installs every entry into a registry.
var Natives = []NativeFunc{
	// `implies` (with nand/nor/iff/xnor) moved to the aql:logic-util module —
	// see native/logic_module.go.

	// ---- control flow ----
	{
		Name: "quote",

		Signatures: []NativeSig{
			{
				// /q captures the upcoming Word as an Atom for us; the
				// handler just marks it Quoted=true.
				Args:      []*Type{TAtom},
				QuoteArgs: map[int]bool{0: true},
				Handler:   quoteWordHandler,
				Returns:   []*Type{TAtom}, BarrierPos: -1,
			},
			{
				Args:           []*Type{TAny},
				NoEvalArgs:     map[int]bool{0: true},
				Handler:        quoteAnyHandler,
				RunInCheckMode: true,
				ReturnsFn:      ReturnsIdentity(0), BarrierPos: -1,
			},
		},
	},

	// `codequote` is `quote`'s code-capturing sibling: it also captures a
	// forward *paren* RAW (as a ParenExpr value) instead of evaluating it.
	// `quote (expr)` evaluates expr then quotes the result (the inert-value
	// idiom); `codequote (expr)` keeps the paren as code — the structural
	// quotability the macro layer wants. Words → atoms and lists → raw list
	// behave exactly like `quote`. See design/PAREN-REPRESENTATION.9.md §2.2.
	{
		Name: "codequote",

		Signatures: []NativeSig{
			{
				Args:      []*Type{TAtom},
				QuoteArgs: map[int]bool{0: true},
				Handler:   quoteWordHandler,
				Returns:   []*Type{TAtom}, BarrierPos: -1,
			},
			{
				Args:           []*Type{TAny},
				NoEvalArgs:     map[int]bool{0: true},
				RawParens:      map[int]bool{0: true},
				Handler:        quoteAnyHandler,
				RunInCheckMode: true,
				ReturnsFn:      ReturnsIdentity(0), BarrierPos: -1,
			},
		},
	},

	// `reach <receiver> [keys]` builds a first-class Reach value (a lens)
	// programmatically: an inert (non-evaluating) dot-access over the
	// receiver with literal `get` segments. Unlike a parsed m.a.b (which
	// evaluates eagerly), a constructed reach is data you can inspect,
	// pass, and convert (see design/REACH.10.md §7). getr/computed segments
	// come from source via codequote; the list-encoding for them is TBD.
	{
		Name: "reach",

		Signatures: []NativeSig{
			{
				Args:       []*Type{TAny, TList},
				NoEvalArgs: map[int]bool{1: true},
				Handler:    reachHandler,
				Returns:    []*Type{TReach}, BarrierPos: -1,
			},
		},
	},

	// `word <value>` wraps its argument (unevaluated) in an __SP splice
	// marker. When the marker reaches the stack pointer its payload is
	// spliced in: a plain list contributes its top-level elements, any
	// other value contributes itself, and the result is re-stepped against
	// the live stack. `def name word value` binds the marker so a later
	// reference splices. The arg is NoEvalArgs so the body is stored raw.
	{
		Name: "word",

		Signatures: []NativeSig{
			{
				Args:       []*Type{TAny},
				NoEvalArgs: map[int]bool{0: true},
				Handler:    wordHandler,
				Returns:    []*Type{TAny}, BarrierPos: -1,
			},
		},
	},

	// `folder` (filesystem op) moved to the aql:io module — see io_module.go.

	// ---- string slice ----
	stringSliceNative(),

	// ---- stack ----
	{
		Name: "stack",

		Signatures: []NativeSig{{
			Args:             []*Type{TInteger},
			FullStack:        true,
			Handler:          stackCollectHandler,
			CheckFullStackFn: stackCollectCheckFullStackFn, BarrierPos: 0,
		}},
	},

	// now / sleep / interval / cancel (with timeout / await) moved to the
	// aql:time-util module — see native/time_async_module.go.

	// ---- list (table query) ----
	{
		Name: "list",

		Signatures: []NativeSig{
			{Args: []*Type{TMap, TResourceEntity}, Handler: listEntityOptsHandler, BarrierPos: -1},
			{Args: []*Type{TResourceEntity}, Handler: listEntityHandler, BarrierPos: -1},
			{Args: []*Type{TMap, TMap}, Handler: listAPIOptsHandler, Patterns: map[int]Value{1: apiPatternValue()}, BarrierPos: -1},
			{Args: []*Type{TMap}, Handler: listAPIHandler, Patterns: map[int]Value{0: apiPatternValue()}, BarrierPos: -1},
			{Args: []*Type{TMap, TList}, Handler: listFilterHandler, BarrierPos: -1},
			{Args: []*Type{TList}, Handler: listAllHandler, BarrierPos: -1},
			{Args: []*Type{TMap, TMap}, Handler: listRecordFilterHandler, BarrierPos: -1},
			{Args: []*Type{TMap}, Handler: listRecordAllHandler, BarrierPos: -1},
		},
	},

	// ---- create ----
	{
		Name: "create",

		Signatures: []NativeSig{
			{Args: []*Type{TMap, TResourceEntity}, Handler: createEntityOptsHandler, BarrierPos: -1},
			{Args: []*Type{TResourceEntity}, Handler: createEntityHandler, BarrierPos: -1},
			{Args: []*Type{TMap, TMap}, Handler: createAPIOptsHandler, Patterns: map[int]Value{1: apiPatternValue()}, BarrierPos: -1},
			{Args: []*Type{TMap}, Handler: createAPIHandler, Patterns: map[int]Value{0: apiPatternValue()}, BarrierPos: -1},
			{Args: []*Type{TMap, TList}, Handler: createHandler, BarrierPos: -1},
			{Args: []*Type{TMap, TMap}, Handler: createRecordHandler, BarrierPos: -1},
		},
	},

	// ---- load ----
	{
		Name: "load",

		Signatures: []NativeSig{
			{Args: []*Type{TMap, TResourceEntity}, Handler: loadEntityOptsHandler, BarrierPos: -1},
			{Args: []*Type{TResourceEntity}, Handler: loadEntityHandler, BarrierPos: -1},
			{Args: []*Type{TMap, TMap}, Handler: loadAPIOptsHandler, Patterns: map[int]Value{1: apiPatternValue()}, BarrierPos: -1},
			{Args: []*Type{TMap}, Handler: loadAPIHandler, Patterns: map[int]Value{0: apiPatternValue()}, BarrierPos: -1},
			{Args: []*Type{TMap, TList}, Handler: loadHandler, BarrierPos: -1},
			{Args: []*Type{TMap, TMap}, Handler: loadRecordHandler, BarrierPos: -1},
		},
	},

	// ---- update ----
	{
		Name: "update",

		Signatures: []NativeSig{
			{Args: []*Type{TMap, TResourceEntity}, Handler: updateEntityOptsHandler, BarrierPos: -1},
			{Args: []*Type{TResourceEntity}, Handler: updateEntityHandler, BarrierPos: -1},
			{Args: []*Type{TMap, TMap}, Handler: updateAPIOptsHandler, Patterns: map[int]Value{1: apiPatternValue()}, BarrierPos: -1},
			{Args: []*Type{TMap}, Handler: updateAPIHandler, Patterns: map[int]Value{0: apiPatternValue()}, BarrierPos: -1},
			{Args: []*Type{TMap, TList}, Handler: updateHandler, BarrierPos: -1},
			{Args: []*Type{TMap, TMap}, Handler: updateRecordHandler, BarrierPos: -1},
		},
	},

	// ---- remove ----
	{
		Name: "remove",

		Signatures: []NativeSig{
			{Args: []*Type{TMap, TResourceEntity}, Handler: removeEntityOptsHandler, BarrierPos: -1},
			{Args: []*Type{TResourceEntity}, Handler: removeEntityHandler, BarrierPos: -1},
			{Args: []*Type{TMap, TMap}, Handler: removeAPIOptsHandler, Patterns: map[int]Value{1: apiPatternValue()}, BarrierPos: -1},
			{Args: []*Type{TMap}, Handler: removeAPIHandler, Patterns: map[int]Value{0: apiPatternValue()}, BarrierPos: -1},
			{Args: []*Type{TMap, TList}, Handler: removeHandler, BarrierPos: -1},
			{Args: []*Type{TMap, TMap}, Handler: removeRecordHandler, BarrierPos: -1},
		},
	},

	// ---- size ----
	{
		Name: "size",

		Signatures: []NativeSig{
			{Args: []*Type{TAny}, Handler: sizeHandler, BarrierPos: -1, Returns: []*Type{TInteger}},
		},
	},

	// `pad` (with the rest of the string words) moved to the aql:string-util
	// module — see native/string_module.go.

	// fetch / prepare / direct moved to the aql:net module — see net_module.go.

	// ---- flatten ----
	{
		Name: "flatten",

		Signatures: []NativeSig{
			{Args: []*Type{TInteger, TList}, Handler: flattenDepthHandler, BarrierPos: -1},
			{Args: []*Type{TList}, Handler: flattenDefaultHandler, BarrierPos: -1},
		},
	},

	// ---- filter ----
	{
		Name: "filter",

		Signatures: []NativeSig{
			{Args: []*Type{TFunction, TAny}, Handler: filterHandler, BarrierPos: -1},
			// Lens form: `filter $.active xs` keeps the elements whose reach
			// applies to a truthy value (the reach reads the ELEMENT, not the
			// {key,value} wrapper the Function form receives).
			{Args: []*Type{TReach, TAny}, Handler: filterReachHandler, BarrierPos: -1},
			// Quotation form: `filter [body] xs` runs the quoted body once
			// per element (element pushed first, like each/fold) and keeps
			// the elements whose body result is Boolean true.
			{Args: []*Type{TList, TAny}, NoEvalArgs: map[int]bool{0: true}, Handler: filterBodyHandler, BarrierPos: -1},
		},
	},

	// ---- join ----
	{
		Name: "join",

		Signatures: []NativeSig{
			{Args: []*Type{TString, TList}, Handler: joinSepHandler, Returns: []*Type{TString}, BarrierPos: -1},
			{Args: []*Type{TList}, Handler: joinDefaultHandler, Returns: []*Type{TString}, BarrierPos: -1},
		},
	},

	// jsonify moved to the aql:struct module — see struct_module.go.

	// ---- listops (push/pop/unshift/shift) ----
	// Each word carries a FlexList sig alongside the plain List sig.
	// The FlexList sig is more specific (deeper lattice Rank) so it
	// wins for flex inputs and mutates IN PLACE, returning the same
	// node (pop/shift additionally return the removed element on
	// top). Plain lists keep the immutable new-copy semantics.
	{
		Name: "push",

		Signatures: []NativeSig{
			{Args: []*Type{TAny, TFlexList}, Handler: pushFlexHandler, Returns: []*Type{TFlexList}, BarrierPos: -1},
			{Args: []*Type{TAny, TList}, Handler: pushHandler, BarrierPos: -1},
		},
	},
	{
		Name: "pop",

		Signatures: []NativeSig{
			{Args: []*Type{TFlexList}, Handler: popFlexHandler, Returns: []*Type{TFlexList, TAny}, BarrierPos: -1},
			{Args: []*Type{TList}, Handler: popHandler, Returns: []*Type{TList, TAny}, BarrierPos: -1},
		},
	},
	{
		Name: "unshift",

		Signatures: []NativeSig{
			{Args: []*Type{TAny, TFlexList}, Handler: unshiftFlexHandler, Returns: []*Type{TFlexList}, BarrierPos: -1},
			{Args: []*Type{TAny, TList}, Handler: unshiftHandler, Returns: []*Type{TList}, BarrierPos: -1},
		},
	},
	{
		Name: "shift",

		Signatures: []NativeSig{
			{Args: []*Type{TFlexList}, Handler: shiftFlexHandler, Returns: []*Type{TFlexList, TAny}, BarrierPos: -1},
			{Args: []*Type{TList}, Handler: shiftHandler, Returns: []*Type{TList, TAny}, BarrierPos: -1},
		},
	},

	// ---- istype ----
	{
		Name: "istype",

		Signatures: []NativeSig{
			{Args: []*Type{TAny}, Handler: istypeHandler, BarrierPos: -1},
		},
	},
}

// apiPatternValue returns the pattern map {kind:"api"} used by signature
// matching to discriminate API maps from plain maps.
func apiPatternValue() Value {
	apiPattern := NewOrderedMap()
	apiPattern.Set("kind", NewString("api"))
	return NewMap(apiPattern)
}

// stringSliceNative builds the "slice" NativeFunc covering substring and
// sublist extraction with three forward-first signatures (3-arg
// start+end+data, 2-arg start+data, 1-arg data) for both String and List
// inputs.
func stringSliceNative() NativeFunc {
	return NativeFunc{
		Name: "slice",

		Signatures: []NativeSig{
			{Args: []*Type{TInteger, TInteger, TString}, Handler: sliceStartEndHandler, Returns: []*Type{TString}, BarrierPos: -1},
			{Args: []*Type{TInteger, TInteger, TList}, Handler: sliceStartEndHandler, Returns: []*Type{TList}, BarrierPos: -1},
			{Args: []*Type{TInteger, TString}, Handler: sliceStartHandler, Returns: []*Type{TString}, BarrierPos: -1},
			{Args: []*Type{TInteger, TList}, Handler: sliceStartHandler, Returns: []*Type{TList}, BarrierPos: -1},
			{Args: []*Type{TString}, Handler: sliceAllHandler, Returns: []*Type{TString}, BarrierPos: -1},
			{Args: []*Type{TList}, Handler: sliceAllHandler, Returns: []*Type{TList}, BarrierPos: -1},
		},
	}
}

// sliceStartEndHandler implements `slice start end data` (forward-first:
// args[0]=start, args[1]=end, args[2]=data). Used by both string and
// list overloads.
func sliceStartEndHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	_as0, _ := args[0].AsConcreteInteger()
	start := int(_as0)
	_as1, _ := args[1].AsConcreteInteger()
	end := int(_as1)
	data := valueToSliceArg(args[2])
	result := voxgigstruct.Slice(data, start, end)
	return sliceResult(result)
}

// sliceStartHandler implements `slice start data` (forward-first:
// args[0]=start, args[1]=data). Slices from start to the end of the
// input.
func sliceStartHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	_as0, _ := args[0].AsConcreteInteger()
	s := int(_as0)
	data := valueToSliceArg(args[1])
	result := voxgigstruct.Slice(data, s)
	return sliceResult(result)
}

// sliceAllHandler implements `slice data` — the identity/copy form that
// returns the input unchanged through voxgigstruct.Slice.
func sliceAllHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	data := valueToSliceArg(args[0])
	result := voxgigstruct.Slice(data)
	return sliceResult(result)
}

// ---- handlers extracted from per-word RegisterFoo closures ----

// impliesHandler implements the "implies" boolean operator. Args[1] is
// the antecedent (left), args[0] is the consequent (right): !left||right.
func impliesHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	left := CoerceBoolean(args[1])
	right := CoerceBoolean(args[0])
	return []Value{NewBoolean(!left || right)}, nil
}

// quoteWordHandler marks the captured atom (already converted from the
// upcoming Word by /q) as Quoted=true and, when its name is currently bound,
// snapshots the binding as the atom's referent — so a quoted name records
// what it referred to at quote time.
func quoteWordHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	v := args[0]
	v.Quoted = true
	return []Value{captureAtomReferent(v, r)}, nil
}

// quoteAnyHandler returns the value with Quoted=true, suppressing
// downstream auto-evaluation. Lists/maps are left structurally intact. An
// atom value additionally captures its referent (see quoteWordHandler).
func quoteAnyHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	v := args[0]
	v.Quoted = true
	return []Value{captureAtomReferent(v, r)}, nil
}

// captureAtomReferent snapshots the current binding of an atom's name onto
// the atom as its referent, when the value is an atom that has none yet and
// the name is bound. Non-atoms and already-captured atoms are returned
// unchanged.
func captureAtomReferent(v Value, r *Registry) Value {
	name, err := AsAtom(v)
	if err != nil {
		return v
	}
	if _, has := AtomReferent(v); has {
		return v
	}
	if bound, ok := r.Defs.Top(name); ok {
		return SetAtomReferent(v, bound)
	}
	return v
}

// reachHandler builds an inert Reach over a receiver value (args[0]) and a
// key list (args[1], NOT evaluated). The list encodes one segment per key:
//
//   - a bare word / atom / string / number → a `get` (lenient) segment
//   - a `!` marker element → the NEXT key is a `getr` (strict) segment
//     (so `reach m [a !b]` ≡ m.a!.b — `!b` lexes as `! b`)
//   - a `(expr)` paren element → a computed segment (key evaluated at apply)
//
// e.g. `reach m [a !b (k)]` → an inert m.a!.b.(k) lens (data).
func reachHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	keys, err := RequireConcreteList(args[1], "reach")
	if err != nil {
		return nil, err
	}
	segs, err := decodeReachSegments(keys.Slice(), r)
	if err != nil {
		return nil, err
	}
	return []Value{NewReach(ReachInfo{Receiver: []Value{args[0]}, Segments: segs, Eval: false})}, nil
}

// decodeReachSegments turns a `reach` key list into Reach segments (see
// reachHandler for the encoding).
func decodeReachSegments(elems []Value, r *Registry) ([]ReachSeg, error) {
	segs := make([]ReachSeg, 0, len(elems))
	getrNext := false
	for _, el := range elems {
		if isReachBang(el) {
			getrNext = true
			continue
		}
		seg := ReachSeg{Getr: getrNext}
		getrNext = false
		if IsParenExpr(el) {
			toks, _ := AsParenExpr(el)
			seg.Computed = true
			seg.KeyExpr = toks
		} else {
			seg.KeyLit = el
		}
		segs = append(segs, seg)
	}
	if getrNext {
		return nil, r.AqlError("reach_error", "reach: trailing `!` with no following key", "reach")
	}
	return segs, nil
}

// isReachBang reports whether a key-list element is the `!` getr marker (a
// bare `!` lexes to a word; an atom `!` covers the quoted form).
func isReachBang(v Value) bool {
	if IsWord(v) {
		w, _ := AsWord(v)
		return w.Name == "!"
	}
	if IsAtom(v) {
		a, _ := AsAtom(v)
		return a == "!"
	}
	return false
}

// wordHandler wraps its (unevaluated) argument in an __SP splice marker. The
// splice itself happens later, when the marker reaches the engine pointer.
func wordHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	return []Value{NewSplice(args[0])}, nil
}

// folderOptsHandler implements `folder` with a leading {parents:bool} options map.
func folderOptsHandler(args []Value, _ map[string]Value, _ []Value, reg *Registry) ([]Value, error) {
	optsVal := args[0]
	pathVal := args[1]
	if !IsPath(pathVal) {
		return nil, fmt.Errorf("folder: expected Path, got %s", pathVal.Parent.String())
	}
	parents := true
	if optsMap, _ := AsMap(optsVal); optsMap != nil {
		if v, ok := optsMap.Get("parents"); ok && v.Parent.ConformsTo(TBoolean) {
			parents, _ = AsBoolean(v)
		}
	}
	_as0, _ := AsPath(pathVal)
	return doFolder(_as0, parents, reg)
}

// folderHandler implements `folder` with a single Path arg (parents=true).
func folderHandler(args []Value, _ map[string]Value, _ []Value, reg *Registry) ([]Value, error) {
	pathVal := args[0]
	if !IsPath(pathVal) {
		return nil, fmt.Errorf("folder: expected Path, got %s", pathVal.Parent.String())
	}
	_as1, _ := AsPath(pathVal)
	return doFolder(_as1, true, reg)
}

// doFolder is the shared body for both folder signatures; resolves and
// creates a directory via the configured FileOps.
func doFolder(p PathInfo, parents bool, reg *Registry) ([]Value, error) {
	ops := EffectiveFileOps(reg)
	pathStr := p.String()

	if parents {
		if err := ops.MkdirAll(pathStr, 0755); err != nil {
			return []Value{NewError(fmt.Errorf("folder: %w", err))}, nil
		}
	} else {
		resolved, err := ops.ResolvePath(pathStr)
		if err != nil {
			return []Value{NewError(fmt.Errorf("folder: %w", err))}, nil
		}
		if err := ops.MkdirAll(resolved, 0755); err != nil {
			return []Value{NewError(fmt.Errorf("folder: %w", err))}, nil
		}
	}

	return []Value{NewPath(p.Parts, p.Abs)}, nil
}

// stackCollectHandler runs at execution time: wraps the top N stack
// entries into a list, preserving the rest of the stack underneath.
func stackCollectHandler(args []Value, _ map[string]Value, stack []Value, _ *Registry) ([]Value, error) {
	_as0, _ := args[0].AsConcreteInteger()
	n := int(_as0)
	if n < 0 || n > len(stack) {
		return nil, fmt.Errorf("stack: count %d out of range (stack depth %d)", n, len(stack))
	}
	items := make([]Value, n)
	copy(items, stack[len(stack)-n:])
	return append(stack, NewList(items)), nil
}

// stackCollectCheckFullStackFn is the check-mode model for `stack N`:
// we don't know N statically, so produce a typed-list carrier whose
// element type joins all preserved stack carriers, leaving the original
// stack intact below it.
func stackCollectCheckFullStackFn(_ []Value, stack []Value, _ *Registry) []Value {
	elem := TAny
	if len(stack) > 0 {
		elem = stack[0].Parent
		for i := 1; i < len(stack); i++ {
			elem = CommonAncestorType(elem, stack[i].Parent)
		}
	}
	return append(append([]Value(nil), stack...), NewCarrierTypedList(elem))
}

// nowHandler returns the current instant as an Instant value, read from
// the registry's Clock capability (a wall clock by default; a FixedClock
// under test/spec) so temporal output is reproducible when frozen.
func nowHandler(_ []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	return []Value{NewInstant(EffectiveClock(r).Now())}, nil
}

// sleepHandler pauses the current goroutine for the given milliseconds.
func sleepHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	ms, _ := args[0].AsConcreteInteger()
	if ms < 0 {
		return nil, r.AqlError("sleep_error", fmt.Sprintf("sleep: milliseconds must be non-negative, got %d", ms), "sleep")
	}
	time.Sleep(time.Duration(ms) * time.Millisecond)
	return nil, nil
}

// intervalListHandler / intervalAtomHandler schedule a repeated callback
// (a quoted code list or word) at the given millisecond interval.
func intervalListHandler(args []Value, ctx map[string]Value, stack []Value, r *Registry) ([]Value, error) {
	return startInterval(args, r, true)
}

func intervalAtomHandler(args []Value, ctx map[string]Value, stack []Value, r *Registry) ([]Value, error) {
	return startInterval(args, r, false)
}

func startInterval(args []Value, r *Registry, isList bool) ([]Value, error) {
	ms, _ := args[0].AsConcreteInteger()
	if ms <= 0 {
		return nil, r.AqlError("interval_error", fmt.Sprintf("interval: milliseconds must be positive, got %d", ms), "interval")
	}
	callback := args[1]

	id := GenerateID("T_")
	ticker := time.NewTicker(time.Duration(ms) * time.Millisecond)
	done := make(chan struct{})

	// Fork now, on the scheduling goroutine, so every tick runs the
	// callback on an isolated registry and never races the main
	// interpreter. The fork is reused across ticks; its private scopes
	// persist between invocations like a long-lived handler's state.
	fork := r.ForkConcurrent()
	go func() {
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				RunTimerCallback(fork, callback, isList)
			}
		}
	}()

	info := &IntervalInfo{
		ID:     id,
		Ms:     ms,
		Ticker: ticker,
		Done:   done,
	}
	return []Value{NewInterval(info)}, nil
}

// cancelTimeoutHandler stops a pending Timeout timer.
func cancelTimeoutHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	ti, ok := args[0].Data.(*TimeoutInfo)
	if !ok {
		return nil, r.AqlError("cancel-timeout_error", fmt.Sprintf("cancel-timeout: not a Timeout value (got %s)", args[0].Parent), "cancel-timeout")
	}
	if ti.Timer != nil {
		ti.Timer.Stop()
		ti.Timer = nil
	}
	return nil, nil
}

// cancelIntervalHandler stops a running Interval ticker and signals its
// goroutine to exit.
func cancelIntervalHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	ii, ok := args[0].Data.(*IntervalInfo)
	if !ok {
		return nil, r.AqlError("cancel-interval_error", fmt.Sprintf("cancel-interval: not an Interval value (got %s)", args[0].Parent), "cancel-interval")
	}
	if ii.Ticker != nil {
		ii.Ticker.Stop()
		close(ii.Done)
		ii.Ticker = nil
	}
	return nil, nil
}
