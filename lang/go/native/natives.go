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
	// ---- boolean ----
	{
		Name: "implies",

		Signatures: []NativeSig{
			{Args: []*Type{TBoolean, TBoolean}, Handler: impliesHandler, Returns: []*Type{TBoolean}, BarrierPos: -1},
			{Args: []*Type{TAny, TAny}, Handler: impliesHandler, Returns: []*Type{TBoolean}, BarrierPos: -1},
		},
	},

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

	// ---- file ops ----
	{
		Name: "folder",

		Signatures: []NativeSig{
			{Args: []*Type{TOptions, TPath}, Handler: folderOptsHandler, Returns: []*Type{TList}, BarrierPos: -1},
			{Args: []*Type{TPath}, Handler: folderHandler, Returns: []*Type{TList}, BarrierPos: -1},
		},
	},

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

	// ---- temporal ----
	{
		Name: "now",

		Signatures: []NativeSig{{
			Args:    []*Type{},
			Handler: nowHandler,
			Returns: []*Type{TInstant}, BarrierPos: 0,
		}},
	},
	{
		Name: "sleep",

		Signatures: []NativeSig{{
			Args:    []*Type{TInteger},
			Handler: sleepHandler,
			Returns: []*Type{}, BarrierPos: -1,
		}},
	},
	{
		Name: "interval",

		Signatures: []NativeSig{
			{Args: []*Type{TInteger, TList}, QuoteArgs: map[int]bool{1: true}, Handler: intervalListHandler, Returns: []*Type{TInterval}, BarrierPos: -1},
			{Args: []*Type{TInteger, TAtom}, QuoteArgs: map[int]bool{1: true}, Handler: intervalAtomHandler, Returns: []*Type{TInterval}, BarrierPos: -1},
		},
	},
	{
		Name: "cancel",

		Signatures: []NativeSig{
			{Args: []*Type{TTimeout}, Handler: cancelTimeoutHandler, Returns: []*Type{}, BarrierPos: -1},
			{Args: []*Type{TInterval}, Handler: cancelIntervalHandler, Returns: []*Type{}, BarrierPos: -1},
		},
	},

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

	// ---- transform ----
	{
		Name: "transform",

		Signatures: []NativeSig{
			{Args: []*Type{TMap, TAny}, Handler: transformHandler, BarrierPos: -1},
		},
	},

	// ---- merge ----
	{
		Name: "merge",

		Signatures: []NativeSig{
			{Args: []*Type{TList, TMap}, Handler: mergeListMapHandler, BarrierPos: -1},
			{Args: []*Type{TMap, TList}, Handler: mergeMapListHandler, BarrierPos: -1},
			{Args: []*Type{TAny, TAny}, Handler: mergeHandler, BarrierPos: -1},
		},
	},

	// ---- validate ----
	{
		Name: "validate",

		Signatures: []NativeSig{
			{Args: []*Type{TMap, TAny}, Handler: validateHandler, BarrierPos: -1},
		},
	},

	// ---- getpath ----
	{
		Name: "getpath",

		Signatures: []NativeSig{
			{Args: []*Type{TString, TAny}, Handler: getpathHandler, BarrierPos: -1},
		},
	},

	// ---- setpath ----
	{
		Name: "setpath",

		Signatures: []NativeSig{
			{Args: []*Type{TString, TAny, TAny}, Handler: setpathHandler, BarrierPos: -1},
			{Args: []*Type{TAny, TString, TAny}, Handler: setpathHandler, BarrierPos: -1},
		},
	},

	// ---- inject ----
	{
		Name: "inject",

		Signatures: []NativeSig{
			{Args: []*Type{TAny, TAny}, Handler: injectHandler, BarrierPos: -1},
		},
	},

	// ---- clone ----
	{
		Name: "clone",

		Signatures: []NativeSig{
			{Args: []*Type{TAny}, Handler: cloneHandler, BarrierPos: 0},
		},
	},

	// ---- walk ----
	{
		Name: "walk",

		Signatures: []NativeSig{
			{Args: []*Type{TFunction, TFunction, TAny}, Handler: walkBeforeAfterHandler, BarrierPos: 0},
			{Args: []*Type{TFunction, TAny}, Handler: walkBeforeHandler, BarrierPos: 0},
			{Args: []*Type{TAny}, Handler: walkHandler, BarrierPos: 0},
		},
	},

	// ---- selector ----
	{
		Name: "selector",

		Signatures: []NativeSig{
			{Args: []*Type{TMap, TAny}, Handler: selectorHandler, BarrierPos: -1},
		},
	},

	// ---- size ----
	{
		Name: "size",

		Signatures: []NativeSig{
			{Args: []*Type{TAny}, Handler: sizeHandler, BarrierPos: -1},
		},
	},

	// ---- pad ----
	{
		Name: "pad",

		Signatures: []NativeSig{
			{Args: []*Type{TInteger, TAny}, Handler: padWidthHandler, BarrierPos: -1},
			{Args: []*Type{TAny}, Handler: padDefaultHandler, BarrierPos: -1},
		},
	},

	// ---- items ----
	{
		Name: "items",

		Signatures: []NativeSig{
			{Args: []*Type{TAny}, Handler: itemsHandler, BarrierPos: -1},
		},
	},

	// ---- fetch ----
	{
		Name: "fetch",

		Signatures: []NativeSig{
			{Args: []*Type{TString, TMap}, Handler: fetchStringMapHandler, BarrierPos: -1},
			{Args: []*Type{TMap}, Handler: fetchMapHandler, BarrierPos: -1},
			{Args: []*Type{TString}, Handler: fetchStringHandler, BarrierPos: -1},
		},
	},

	// ---- prepare ----
	{
		Name: "prepare",

		Signatures: []NativeSig{
			{Args: []*Type{TMap}, Handler: prepareAPIHandler, Patterns: map[int]Value{0: apiPatternValue()}, BarrierPos: -1},
		},
	},

	// ---- direct ----
	{
		Name: "direct",

		Signatures: []NativeSig{
			{Args: []*Type{TMap}, Handler: directAPIHandler, Patterns: map[int]Value{0: apiPatternValue()}, BarrierPos: -1},
		},
	},

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
		},
	},

	// ---- join ----
	{
		Name: "join",

		Signatures: []NativeSig{
			{Args: []*Type{TString, TList}, Handler: joinSepHandler, BarrierPos: -1},
			{Args: []*Type{TList}, Handler: joinDefaultHandler, BarrierPos: -1},
		},
	},

	// ---- jsonify ----
	{
		Name: "jsonify",

		Signatures: []NativeSig{
			{Args: []*Type{TMap, TAny}, Handler: jsonifyFlagsHandler, BarrierPos: -1},
			{Args: []*Type{TAny}, Handler: jsonifyDefaultHandler, BarrierPos: -1},
		},
	},

	// ---- listops (push/pop/unshift/shift) ----
	{
		Name: "push",

		Signatures: []NativeSig{
			{Args: []*Type{TAny, TList}, Handler: pushHandler, BarrierPos: -1},
		},
	},
	{
		Name: "pop",

		Signatures: []NativeSig{
			{Args: []*Type{TList}, Handler: popHandler, BarrierPos: -1},
		},
	},
	{
		Name: "unshift",

		Signatures: []NativeSig{
			{Args: []*Type{TAny, TList}, Handler: unshiftHandler, BarrierPos: -1},
		},
	},
	{
		Name: "shift",

		Signatures: []NativeSig{
			{Args: []*Type{TList}, Handler: shiftHandler, BarrierPos: -1},
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
// upcoming Word by /q) as Quoted=true.
func quoteWordHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	v := args[0]
	v.Quoted = true
	return []Value{v}, nil
}

// quoteAnyHandler returns the value with Quoted=true, suppressing
// downstream auto-evaluation. Lists/maps are left structurally intact.
func quoteAnyHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	v := args[0]
	v.Quoted = true
	return []Value{v}, nil
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
		if v, ok := optsMap.Get("parents"); ok && v.Parent.Matches(TBoolean) {
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

// nowHandler returns the current UTC instant as an Instant value.
func nowHandler(_ []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	return []Value{NewInstant(time.Now())}, nil
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

	go func() {
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				RunTimerCallback(r, callback, isList)
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
