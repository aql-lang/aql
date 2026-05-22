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
		Name:        "implies",
		ForwardArgs: true,
		Signatures: []NativeSig{
			{Args: []*Type{TBoolean, TBoolean}, Handler: impliesHandler, Returns: []*Type{TBoolean}},
			{Args: []*Type{TAny, TAny}, Handler: impliesHandler, Returns: []*Type{TBoolean}},
		},
	},

	// ---- control flow ----
	{
		Name:        "quote",
		ForwardArgs: true,
		Signatures: []NativeSig{
			{
				// /q captures the upcoming Word as an Atom for us; the
				// handler just marks it Quoted=true.
				Args:      []*Type{TAtom},
				QuoteArgs: map[int]bool{0: true},
				Handler:   quoteWordHandler,
				Returns:   []*Type{TAtom},
			},
			{
				Args:           []*Type{TAny},
				NoEvalArgs:     map[int]bool{0: true},
				Handler:        quoteAnyHandler,
				RunInCheckMode: true,
				ReturnsFn:      ReturnsIdentity(0),
			},
		},
	},

	// ---- file ops ----
	{
		Name:        "folder",
		ForwardArgs: true,
		Signatures: []NativeSig{
			{Args: []*Type{TOptions, TPath}, Handler: folderOptsHandler, Returns: []*Type{TList}},
			{Args: []*Type{TPath}, Handler: folderHandler, Returns: []*Type{TList}},
		},
	},

	// ---- string slice ----
	stringSliceNative(),

	// ---- stack ----
	{
		Name:        "stack",
		ForwardArgs: false,
		Signatures: []NativeSig{{
			Args:             []*Type{TInteger},
			FullStack:        true,
			Handler:          stackCollectHandler,
			CheckFullStackFn: stackCollectCheckFullStackFn,
		}},
	},

	// ---- temporal ----
	{
		Name:        "now",
		ForwardArgs: false,
		Signatures: []NativeSig{{
			Args:    []*Type{},
			Handler: nowHandler,
			Returns: []*Type{TInstant},
		}},
	},
	{
		Name:        "sleep",
		ForwardArgs: true,
		Signatures: []NativeSig{{
			Args:    []*Type{TInteger},
			Handler: sleepHandler,
			Returns: []*Type{},
		}},
	},
	{
		Name:        "interval",
		ForwardArgs: true,
		Signatures: []NativeSig{
			{Args: []*Type{TInteger, TList}, QuoteArgs: map[int]bool{1: true}, Handler: intervalListHandler, Returns: []*Type{TInterval}},
			{Args: []*Type{TInteger, TAtom}, QuoteArgs: map[int]bool{1: true}, Handler: intervalAtomHandler, Returns: []*Type{TInterval}},
		},
	},
	{
		Name:        "cancel",
		ForwardArgs: true,
		Signatures: []NativeSig{
			{Args: []*Type{TTimeout}, Handler: cancelTimeoutHandler, Returns: []*Type{}},
			{Args: []*Type{TInterval}, Handler: cancelIntervalHandler, Returns: []*Type{}},
		},
	},

	// ---- list (table query) ----
	{
		Name:        "list",
		ForwardArgs: true,
		Signatures: []NativeSig{
			{Args: []*Type{TMap, TResourceEntity}, Handler: listEntityOptsHandler},
			{Args: []*Type{TResourceEntity}, Handler: listEntityHandler},
			{Args: []*Type{TMap, TMap}, Handler: listAPIOptsHandler, Patterns: map[int]Value{1: apiPatternValue()}},
			{Args: []*Type{TMap}, Handler: listAPIHandler, Patterns: map[int]Value{0: apiPatternValue()}},
			{Args: []*Type{TMap, TList}, Handler: listFilterHandler},
			{Args: []*Type{TList}, Handler: listAllHandler},
			{Args: []*Type{TMap, TMap}, Handler: listRecordFilterHandler},
			{Args: []*Type{TMap}, Handler: listRecordAllHandler},
		},
	},

	// ---- create ----
	{
		Name:        "create",
		ForwardArgs: true,
		Signatures: []NativeSig{
			{Args: []*Type{TMap, TResourceEntity}, Handler: createEntityOptsHandler},
			{Args: []*Type{TResourceEntity}, Handler: createEntityHandler},
			{Args: []*Type{TMap, TMap}, Handler: createAPIOptsHandler, Patterns: map[int]Value{1: apiPatternValue()}},
			{Args: []*Type{TMap}, Handler: createAPIHandler, Patterns: map[int]Value{0: apiPatternValue()}},
			{Args: []*Type{TMap, TList}, Handler: createHandler},
			{Args: []*Type{TMap, TMap}, Handler: createRecordHandler},
		},
	},

	// ---- load ----
	{
		Name:        "load",
		ForwardArgs: true,
		Signatures: []NativeSig{
			{Args: []*Type{TMap, TResourceEntity}, Handler: loadEntityOptsHandler},
			{Args: []*Type{TResourceEntity}, Handler: loadEntityHandler},
			{Args: []*Type{TMap, TMap}, Handler: loadAPIOptsHandler, Patterns: map[int]Value{1: apiPatternValue()}},
			{Args: []*Type{TMap}, Handler: loadAPIHandler, Patterns: map[int]Value{0: apiPatternValue()}},
			{Args: []*Type{TMap, TList}, Handler: loadHandler},
			{Args: []*Type{TMap, TMap}, Handler: loadRecordHandler},
		},
	},

	// ---- update ----
	{
		Name:        "update",
		ForwardArgs: true,
		Signatures: []NativeSig{
			{Args: []*Type{TMap, TResourceEntity}, Handler: updateEntityOptsHandler},
			{Args: []*Type{TResourceEntity}, Handler: updateEntityHandler},
			{Args: []*Type{TMap, TMap}, Handler: updateAPIOptsHandler, Patterns: map[int]Value{1: apiPatternValue()}},
			{Args: []*Type{TMap}, Handler: updateAPIHandler, Patterns: map[int]Value{0: apiPatternValue()}},
			{Args: []*Type{TMap, TList}, Handler: updateHandler},
			{Args: []*Type{TMap, TMap}, Handler: updateRecordHandler},
		},
	},

	// ---- remove ----
	{
		Name:        "remove",
		ForwardArgs: true,
		Signatures: []NativeSig{
			{Args: []*Type{TMap, TResourceEntity}, Handler: removeEntityOptsHandler},
			{Args: []*Type{TResourceEntity}, Handler: removeEntityHandler},
			{Args: []*Type{TMap, TMap}, Handler: removeAPIOptsHandler, Patterns: map[int]Value{1: apiPatternValue()}},
			{Args: []*Type{TMap}, Handler: removeAPIHandler, Patterns: map[int]Value{0: apiPatternValue()}},
			{Args: []*Type{TMap, TList}, Handler: removeHandler},
			{Args: []*Type{TMap, TMap}, Handler: removeRecordHandler},
		},
	},

	// ---- transform ----
	{
		Name:        "transform",
		ForwardArgs: true,
		Signatures: []NativeSig{
			{Args: []*Type{TMap, TAny}, Handler: transformHandler},
		},
	},

	// ---- merge ----
	{
		Name:        "merge",
		ForwardArgs: true,
		Signatures: []NativeSig{
			{Args: []*Type{TList, TMap}, Handler: mergeListMapHandler},
			{Args: []*Type{TMap, TList}, Handler: mergeMapListHandler},
			{Args: []*Type{TAny, TAny}, Handler: mergeHandler},
		},
	},

	// ---- validate ----
	{
		Name:        "validate",
		ForwardArgs: true,
		Signatures: []NativeSig{
			{Args: []*Type{TMap, TAny}, Handler: validateHandler},
		},
	},

	// ---- getpath ----
	{
		Name:        "getpath",
		ForwardArgs: true,
		Signatures: []NativeSig{
			{Args: []*Type{TString, TAny}, Handler: getpathHandler},
		},
	},

	// ---- setpath ----
	{
		Name:        "setpath",
		ForwardArgs: true,
		Signatures: []NativeSig{
			{Args: []*Type{TString, TAny, TAny}, Handler: setpathHandler},
			{Args: []*Type{TAny, TString, TAny}, Handler: setpathHandler},
		},
	},

	// ---- inject ----
	{
		Name:        "inject",
		ForwardArgs: true,
		Signatures: []NativeSig{
			{Args: []*Type{TAny, TAny}, Handler: injectHandler},
		},
	},

	// ---- clone ----
	{
		Name:        "clone",
		ForwardArgs: false,
		Signatures: []NativeSig{
			{Args: []*Type{TAny}, Handler: cloneHandler},
		},
	},

	// ---- walk ----
	{
		Name:        "walk",
		ForwardArgs: false,
		Signatures: []NativeSig{
			{Args: []*Type{TFunction, TFunction, TAny}, Handler: walkBeforeAfterHandler},
			{Args: []*Type{TFunction, TAny}, Handler: walkBeforeHandler},
			{Args: []*Type{TAny}, Handler: walkHandler},
		},
	},

	// ---- selector ----
	{
		Name:        "selector",
		ForwardArgs: true,
		Signatures: []NativeSig{
			{Args: []*Type{TMap, TAny}, Handler: selectorHandler},
		},
	},

	// ---- size ----
	{
		Name:        "size",
		ForwardArgs: true,
		Signatures: []NativeSig{
			{Args: []*Type{TAny}, Handler: sizeHandler},
		},
	},

	// ---- pad ----
	{
		Name:        "pad",
		ForwardArgs: true,
		Signatures: []NativeSig{
			{Args: []*Type{TInteger, TAny}, Handler: padWidthHandler},
			{Args: []*Type{TAny}, Handler: padDefaultHandler},
		},
	},

	// ---- items ----
	{
		Name:        "items",
		ForwardArgs: true,
		Signatures: []NativeSig{
			{Args: []*Type{TAny}, Handler: itemsHandler},
		},
	},

	// ---- fetch ----
	{
		Name:        "fetch",
		ForwardArgs: true,
		Signatures: []NativeSig{
			{Args: []*Type{TString, TMap}, Handler: fetchStringMapHandler},
			{Args: []*Type{TMap}, Handler: fetchMapHandler},
			{Args: []*Type{TString}, Handler: fetchStringHandler},
		},
	},

	// ---- prepare ----
	{
		Name:        "prepare",
		ForwardArgs: true,
		Signatures: []NativeSig{
			{Args: []*Type{TMap}, Handler: prepareAPIHandler, Patterns: map[int]Value{0: apiPatternValue()}},
		},
	},

	// ---- direct ----
	{
		Name:        "direct",
		ForwardArgs: true,
		Signatures: []NativeSig{
			{Args: []*Type{TMap}, Handler: directAPIHandler, Patterns: map[int]Value{0: apiPatternValue()}},
		},
	},

	// ---- flatten ----
	{
		Name:        "flatten",
		ForwardArgs: true,
		Signatures: []NativeSig{
			{Args: []*Type{TInteger, TList}, Handler: flattenDepthHandler},
			{Args: []*Type{TList}, Handler: flattenDefaultHandler},
		},
	},

	// ---- filter ----
	{
		Name:        "filter",
		ForwardArgs: true,
		Signatures: []NativeSig{
			{Args: []*Type{TFunction, TAny}, Handler: filterHandler},
		},
	},

	// ---- join ----
	{
		Name:        "join",
		ForwardArgs: true,
		Signatures: []NativeSig{
			{Args: []*Type{TString, TList}, Handler: joinSepHandler},
			{Args: []*Type{TList}, Handler: joinDefaultHandler},
		},
	},

	// ---- jsonify ----
	{
		Name:        "jsonify",
		ForwardArgs: true,
		Signatures: []NativeSig{
			{Args: []*Type{TMap, TAny}, Handler: jsonifyFlagsHandler},
			{Args: []*Type{TAny}, Handler: jsonifyDefaultHandler},
		},
	},

	// ---- listops (push/pop/unshift/shift) ----
	{
		Name:        "push",
		ForwardArgs: true,
		Signatures: []NativeSig{
			{Args: []*Type{TAny, TList}, Handler: pushHandler},
		},
	},
	{
		Name:        "pop",
		ForwardArgs: true,
		Signatures: []NativeSig{
			{Args: []*Type{TList}, Handler: popHandler},
		},
	},
	{
		Name:        "unshift",
		ForwardArgs: true,
		Signatures: []NativeSig{
			{Args: []*Type{TAny, TList}, Handler: unshiftHandler},
		},
	},
	{
		Name:        "shift",
		ForwardArgs: true,
		Signatures: []NativeSig{
			{Args: []*Type{TList}, Handler: shiftHandler},
		},
	},

	// ---- istype ----
	{
		Name:        "istype",
		ForwardArgs: true,
		Signatures: []NativeSig{
			{Args: []*Type{TAny}, Handler: istypeHandler},
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
		Name:        "slice",
		ForwardArgs: true,
		Signatures: []NativeSig{
			{Args: []*Type{TInteger, TInteger, TString}, Handler: sliceStartEndHandler, Returns: []*Type{TString}},
			{Args: []*Type{TInteger, TInteger, TList}, Handler: sliceStartEndHandler, Returns: []*Type{TList}},
			{Args: []*Type{TInteger, TString}, Handler: sliceStartHandler, Returns: []*Type{TString}},
			{Args: []*Type{TInteger, TList}, Handler: sliceStartHandler, Returns: []*Type{TList}},
			{Args: []*Type{TString}, Handler: sliceAllHandler, Returns: []*Type{TString}},
			{Args: []*Type{TList}, Handler: sliceAllHandler, Returns: []*Type{TList}},
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
