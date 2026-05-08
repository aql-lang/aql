package native

import (
	"fmt"
	"time"

	"github.com/metsitaba/voxgig-exp/aql/internal/engine"
	voxgigstruct "github.com/voxgig/struct"
)

// Natives is the consolidated NativeFunc slice for the data-manipulation
// words owned by the native package. It replaces the per-word RegisterFoo
// functions and their aggregator (registerAll). The public Register entry
// point in native.go installs every entry into a registry.
var Natives = []engine.NativeFunc{
	// ---- boolean ----
	{
		Name:              "implies",
		ForwardPrecedence: true,
		Signatures: []engine.NativeSig{
			{Args: []engine.Type{engine.TBoolean, engine.TBoolean}, Handler: impliesHandler, Returns: []engine.Type{engine.TBoolean}},
			{Args: []engine.Type{engine.TAny, engine.TAny}, Handler: impliesHandler, Returns: []engine.Type{engine.TBoolean}},
		},
	},

	// ---- control flow ----
	{
		Name:              "quote",
		ForwardPrecedence: true,
		Signatures: []engine.NativeSig{
			{
				Args:    []engine.Type{engine.TWord},
				Handler: quoteWordHandler,
				Returns: []engine.Type{engine.TAtom},
			},
			{
				Args:           []engine.Type{engine.TAny},
				NoEvalArgs:     map[int]bool{0: true},
				Handler:        quoteAnyHandler,
				RunInCheckMode: true,
				ReturnsFn:      engine.ReturnsIdentity(0),
			},
		},
	},

	// ---- file ops ----
	{
		Name:              "folder",
		ForwardPrecedence: true,
		Signatures: []engine.NativeSig{
			{Args: []engine.Type{engine.TOptions, engine.TPath}, Handler: folderOptsHandler, Returns: []engine.Type{engine.TList}},
			{Args: []engine.Type{engine.TPath}, Handler: folderHandler, Returns: []engine.Type{engine.TList}},
		},
	},

	// ---- string slice ----
	stringSliceNative(),

	// ---- stack ----
	{
		Name:              "stack",
		ForwardPrecedence: false,
		Signatures: []engine.NativeSig{{
			Args:             []engine.Type{engine.TInteger},
			FullStack:        true,
			Handler:          stackCollectHandler,
			CheckFullStackFn: stackCollectCheckFullStackFn,
		}},
	},

	// ---- temporal ----
	{
		Name:              "now",
		ForwardPrecedence: false,
		Signatures: []engine.NativeSig{{
			Args:    []engine.Type{},
			Handler: nowHandler,
			Returns: []engine.Type{engine.TInstant},
		}},
	},
	{
		Name:              "sleep",
		ForwardPrecedence: true,
		Signatures: []engine.NativeSig{{
			Args:    []engine.Type{engine.TInteger},
			Handler: sleepHandler,
			Returns: []engine.Type{},
		}},
	},
	{
		Name:              "interval",
		ForwardPrecedence: true,
		Signatures: []engine.NativeSig{
			{Args: []engine.Type{engine.TInteger, engine.TList}, QuoteArgs: map[int]bool{1: true}, Handler: intervalListHandler, Returns: []engine.Type{engine.TInterval}},
			{Args: []engine.Type{engine.TInteger, engine.TAtom}, QuoteArgs: map[int]bool{1: true}, Handler: intervalAtomHandler, Returns: []engine.Type{engine.TInterval}},
		},
	},
	{
		Name:              "cancel",
		ForwardPrecedence: true,
		Signatures: []engine.NativeSig{
			{Args: []engine.Type{engine.TTimeout}, Handler: cancelTimeoutHandler, Returns: []engine.Type{}},
			{Args: []engine.Type{engine.TInterval}, Handler: cancelIntervalHandler, Returns: []engine.Type{}},
		},
	},

	// ---- list (table query) ----
	{
		Name:              "list",
		ForwardPrecedence: true,
		Signatures: []engine.NativeSig{
			{Args: []engine.Type{engine.TMap, engine.TResourceEntity}, Handler: listEntityOptsHandler},
			{Args: []engine.Type{engine.TResourceEntity}, Handler: listEntityHandler},
			{Args: []engine.Type{engine.TMap, engine.TMap}, Handler: listAPIOptsHandler, Patterns: map[int]engine.Value{1: apiPatternValue()}},
			{Args: []engine.Type{engine.TMap}, Handler: listAPIHandler, Patterns: map[int]engine.Value{0: apiPatternValue()}},
			{Args: []engine.Type{engine.TMap, engine.TList}, Handler: listFilterHandler},
			{Args: []engine.Type{engine.TList}, Handler: listAllHandler},
			{Args: []engine.Type{engine.TMap, engine.TMap}, Handler: listRecordFilterHandler},
			{Args: []engine.Type{engine.TMap}, Handler: listRecordAllHandler},
		},
	},

	// ---- create ----
	{
		Name:              "create",
		ForwardPrecedence: true,
		Signatures: []engine.NativeSig{
			{Args: []engine.Type{engine.TMap, engine.TResourceEntity}, Handler: createEntityOptsHandler},
			{Args: []engine.Type{engine.TResourceEntity}, Handler: createEntityHandler},
			{Args: []engine.Type{engine.TMap, engine.TMap}, Handler: createAPIOptsHandler, Patterns: map[int]engine.Value{1: apiPatternValue()}},
			{Args: []engine.Type{engine.TMap}, Handler: createAPIHandler, Patterns: map[int]engine.Value{0: apiPatternValue()}},
			{Args: []engine.Type{engine.TMap, engine.TList}, Handler: createHandler},
			{Args: []engine.Type{engine.TMap, engine.TMap}, Handler: createRecordHandler},
		},
	},

	// ---- load ----
	{
		Name:              "load",
		ForwardPrecedence: true,
		Signatures: []engine.NativeSig{
			{Args: []engine.Type{engine.TMap, engine.TResourceEntity}, Handler: loadEntityOptsHandler},
			{Args: []engine.Type{engine.TResourceEntity}, Handler: loadEntityHandler},
			{Args: []engine.Type{engine.TMap, engine.TMap}, Handler: loadAPIOptsHandler, Patterns: map[int]engine.Value{1: apiPatternValue()}},
			{Args: []engine.Type{engine.TMap}, Handler: loadAPIHandler, Patterns: map[int]engine.Value{0: apiPatternValue()}},
			{Args: []engine.Type{engine.TMap, engine.TList}, Handler: loadHandler},
			{Args: []engine.Type{engine.TMap, engine.TMap}, Handler: loadRecordHandler},
		},
	},

	// ---- update ----
	{
		Name:              "update",
		ForwardPrecedence: true,
		Signatures: []engine.NativeSig{
			{Args: []engine.Type{engine.TMap, engine.TResourceEntity}, Handler: updateEntityOptsHandler},
			{Args: []engine.Type{engine.TResourceEntity}, Handler: updateEntityHandler},
			{Args: []engine.Type{engine.TMap, engine.TMap}, Handler: updateAPIOptsHandler, Patterns: map[int]engine.Value{1: apiPatternValue()}},
			{Args: []engine.Type{engine.TMap}, Handler: updateAPIHandler, Patterns: map[int]engine.Value{0: apiPatternValue()}},
			{Args: []engine.Type{engine.TMap, engine.TList}, Handler: updateHandler},
			{Args: []engine.Type{engine.TMap, engine.TMap}, Handler: updateRecordHandler},
		},
	},

	// ---- remove ----
	{
		Name:              "remove",
		ForwardPrecedence: true,
		Signatures: []engine.NativeSig{
			{Args: []engine.Type{engine.TMap, engine.TResourceEntity}, Handler: removeEntityOptsHandler},
			{Args: []engine.Type{engine.TResourceEntity}, Handler: removeEntityHandler},
			{Args: []engine.Type{engine.TMap, engine.TMap}, Handler: removeAPIOptsHandler, Patterns: map[int]engine.Value{1: apiPatternValue()}},
			{Args: []engine.Type{engine.TMap}, Handler: removeAPIHandler, Patterns: map[int]engine.Value{0: apiPatternValue()}},
			{Args: []engine.Type{engine.TMap, engine.TList}, Handler: removeHandler},
			{Args: []engine.Type{engine.TMap, engine.TMap}, Handler: removeRecordHandler},
		},
	},

	// ---- transform ----
	{
		Name:              "transform",
		ForwardPrecedence: true,
		Signatures: []engine.NativeSig{
			{Args: []engine.Type{engine.TMap, engine.TAny}, Handler: transformHandler},
		},
	},

	// ---- merge ----
	{
		Name:              "merge",
		ForwardPrecedence: true,
		Signatures: []engine.NativeSig{
			{Args: []engine.Type{engine.TList, engine.TMap}, Handler: mergeListMapHandler},
			{Args: []engine.Type{engine.TMap, engine.TList}, Handler: mergeMapListHandler},
			{Args: []engine.Type{engine.TAny, engine.TAny}, Handler: mergeHandler},
		},
	},

	// ---- validate ----
	{
		Name:              "validate",
		ForwardPrecedence: true,
		Signatures: []engine.NativeSig{
			{Args: []engine.Type{engine.TMap, engine.TAny}, Handler: validateHandler},
		},
	},

	// ---- getpath ----
	{
		Name:              "getpath",
		ForwardPrecedence: true,
		Signatures: []engine.NativeSig{
			{Args: []engine.Type{engine.TString, engine.TAny}, Handler: getpathHandler},
		},
	},

	// ---- setpath ----
	{
		Name:              "setpath",
		ForwardPrecedence: true,
		Signatures: []engine.NativeSig{
			{Args: []engine.Type{engine.TString, engine.TAny, engine.TAny}, Handler: setpathHandler},
			{Args: []engine.Type{engine.TAny, engine.TString, engine.TAny}, Handler: setpathHandler},
		},
	},

	// ---- inject ----
	{
		Name:              "inject",
		ForwardPrecedence: true,
		Signatures: []engine.NativeSig{
			{Args: []engine.Type{engine.TAny, engine.TAny}, Handler: injectHandler},
		},
	},

	// ---- clone ----
	{
		Name:              "clone",
		ForwardPrecedence: false,
		Signatures: []engine.NativeSig{
			{Args: []engine.Type{engine.TAny}, Handler: cloneHandler},
		},
	},

	// ---- walk ----
	{
		Name:              "walk",
		ForwardPrecedence: false,
		Signatures: []engine.NativeSig{
			{Args: []engine.Type{engine.TFunction, engine.TFunction, engine.TAny}, Handler: walkBeforeAfterHandler},
			{Args: []engine.Type{engine.TFunction, engine.TAny}, Handler: walkBeforeHandler},
			{Args: []engine.Type{engine.TAny}, Handler: walkHandler},
		},
	},

	// ---- selector ----
	{
		Name:              "selector",
		ForwardPrecedence: true,
		Signatures: []engine.NativeSig{
			{Args: []engine.Type{engine.TMap, engine.TAny}, Handler: selectorHandler},
		},
	},

	// ---- size ----
	{
		Name:              "size",
		ForwardPrecedence: true,
		Signatures: []engine.NativeSig{
			{Args: []engine.Type{engine.TAny}, Handler: sizeHandler},
		},
	},

	// ---- pad ----
	{
		Name:              "pad",
		ForwardPrecedence: true,
		Signatures: []engine.NativeSig{
			{Args: []engine.Type{engine.TInteger, engine.TAny}, Handler: padWidthHandler},
			{Args: []engine.Type{engine.TAny}, Handler: padDefaultHandler},
		},
	},

	// ---- items ----
	{
		Name:              "items",
		ForwardPrecedence: true,
		Signatures: []engine.NativeSig{
			{Args: []engine.Type{engine.TAny}, Handler: itemsHandler},
		},
	},

	// ---- fetch ----
	{
		Name:              "fetch",
		ForwardPrecedence: true,
		Signatures: []engine.NativeSig{
			{Args: []engine.Type{engine.TString, engine.TMap}, Handler: fetchStringMapHandler},
			{Args: []engine.Type{engine.TMap}, Handler: fetchMapHandler},
			{Args: []engine.Type{engine.TString}, Handler: fetchStringHandler},
		},
	},

	// ---- prepare ----
	{
		Name:              "prepare",
		ForwardPrecedence: true,
		Signatures: []engine.NativeSig{
			{Args: []engine.Type{engine.TMap}, Handler: prepareAPIHandler, Patterns: map[int]engine.Value{0: apiPatternValue()}},
		},
	},

	// ---- direct ----
	{
		Name:              "direct",
		ForwardPrecedence: true,
		Signatures: []engine.NativeSig{
			{Args: []engine.Type{engine.TMap}, Handler: directAPIHandler, Patterns: map[int]engine.Value{0: apiPatternValue()}},
		},
	},

	// ---- flatten ----
	{
		Name:              "flatten",
		ForwardPrecedence: true,
		Signatures: []engine.NativeSig{
			{Args: []engine.Type{engine.TInteger, engine.TList}, Handler: flattenDepthHandler},
			{Args: []engine.Type{engine.TList}, Handler: flattenDefaultHandler},
		},
	},

	// ---- filter ----
	{
		Name:              "filter",
		ForwardPrecedence: true,
		Signatures: []engine.NativeSig{
			{Args: []engine.Type{engine.TFunction, engine.TAny}, Handler: filterHandler},
		},
	},

	// ---- join ----
	{
		Name:              "join",
		ForwardPrecedence: true,
		Signatures: []engine.NativeSig{
			{Args: []engine.Type{engine.TString, engine.TList}, Handler: joinSepHandler},
			{Args: []engine.Type{engine.TList}, Handler: joinDefaultHandler},
		},
	},

	// ---- jsonify ----
	{
		Name:              "jsonify",
		ForwardPrecedence: true,
		Signatures: []engine.NativeSig{
			{Args: []engine.Type{engine.TMap, engine.TAny}, Handler: jsonifyFlagsHandler},
			{Args: []engine.Type{engine.TAny}, Handler: jsonifyDefaultHandler},
		},
	},

	// ---- listops (push/pop/unshift/shift) ----
	{
		Name:              "push",
		ForwardPrecedence: true,
		Signatures: []engine.NativeSig{
			{Args: []engine.Type{engine.TAny, engine.TList}, Handler: pushHandler},
		},
	},
	{
		Name:              "pop",
		ForwardPrecedence: true,
		Signatures: []engine.NativeSig{
			{Args: []engine.Type{engine.TList}, Handler: popHandler},
		},
	},
	{
		Name:              "unshift",
		ForwardPrecedence: true,
		Signatures: []engine.NativeSig{
			{Args: []engine.Type{engine.TAny, engine.TList}, Handler: unshiftHandler},
		},
	},
	{
		Name:              "shift",
		ForwardPrecedence: true,
		Signatures: []engine.NativeSig{
			{Args: []engine.Type{engine.TList}, Handler: shiftHandler},
		},
	},

	// ---- istype ----
	{
		Name:              "istype",
		ForwardPrecedence: true,
		Signatures: []engine.NativeSig{
			{Args: []engine.Type{engine.TAny}, Handler: istypeHandler},
		},
	},
}

// apiPatternValue returns the pattern map {kind:"api"} used by signature
// matching to discriminate API maps from plain maps.
func apiPatternValue() engine.Value {
	apiPattern := engine.NewOrderedMap()
	apiPattern.Set("kind", engine.NewString("api"))
	return engine.NewMap(apiPattern)
}

// stringSliceNative builds the "slice" NativeFunc covering substring and
// sublist extraction with three forward-first signatures (3-arg
// start+end+data, 2-arg start+data, 1-arg data) for both String and List
// inputs.
func stringSliceNative() engine.NativeFunc {
	return engine.NativeFunc{
		Name:              "slice",
		ForwardPrecedence: true,
		Signatures: []engine.NativeSig{
			{Args: []engine.Type{engine.TInteger, engine.TInteger, engine.TString}, Handler: sliceStartEndHandler, Returns: []engine.Type{engine.TString}},
			{Args: []engine.Type{engine.TInteger, engine.TInteger, engine.TList}, Handler: sliceStartEndHandler, Returns: []engine.Type{engine.TList}},
			{Args: []engine.Type{engine.TInteger, engine.TString}, Handler: sliceStartHandler, Returns: []engine.Type{engine.TString}},
			{Args: []engine.Type{engine.TInteger, engine.TList}, Handler: sliceStartHandler, Returns: []engine.Type{engine.TList}},
			{Args: []engine.Type{engine.TString}, Handler: sliceAllHandler, Returns: []engine.Type{engine.TString}},
			{Args: []engine.Type{engine.TList}, Handler: sliceAllHandler, Returns: []engine.Type{engine.TList}},
		},
	}
}

// sliceStartEndHandler implements `slice start end data` (forward-first:
// args[0]=start, args[1]=end, args[2]=data). Used by both string and
// list overloads.
func sliceStartEndHandler(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
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
func sliceStartHandler(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
	_as0, _ := args[0].AsConcreteInteger()
	s := int(_as0)
	data := valueToSliceArg(args[1])
	result := voxgigstruct.Slice(data, s)
	return sliceResult(result)
}

// sliceAllHandler implements `slice data` — the identity/copy form that
// returns the input unchanged through voxgigstruct.Slice.
func sliceAllHandler(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
	data := valueToSliceArg(args[0])
	result := voxgigstruct.Slice(data)
	return sliceResult(result)
}

// ---- handlers extracted from per-word RegisterFoo closures ----

// impliesHandler implements the "implies" boolean operator. Args[1] is
// the antecedent (left), args[0] is the consequent (right): !left||right.
func impliesHandler(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
	left := engine.CoerceBoolean(args[1])
	right := engine.CoerceBoolean(args[0])
	return []engine.Value{engine.NewBoolean(!left || right)}, nil
}

// quoteWordHandler converts the upcoming Word literal to a quoted Atom.
func quoteWordHandler(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
	w, _ := args[0].AsWord()
	v := engine.NewAtom(w.Name)
	v.Quoted = true
	return []engine.Value{v}, nil
}

// quoteAnyHandler returns the value with Quoted=true, suppressing
// downstream auto-evaluation. Lists/maps are left structurally intact.
func quoteAnyHandler(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
	v := args[0]
	v.Quoted = true
	return []engine.Value{v}, nil
}

// folderOptsHandler implements `folder` with a leading {parents:bool} options map.
func folderOptsHandler(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, reg *engine.Registry) ([]engine.Value, error) {
	optsVal := args[0]
	pathVal := args[1]
	if !pathVal.IsPath() {
		return nil, fmt.Errorf("folder: expected Path, got %s", pathVal.VType.String())
	}
	parents := true
	if optsMap := optsVal.AsMap(); optsMap != nil {
		if v, ok := optsMap.Get("parents"); ok && v.VType.Matches(engine.TBoolean) {
			parents, _ = v.AsBoolean()
		}
	}
	_as0, _ := pathVal.AsPath()
	return doFolder(_as0, parents, reg)
}

// folderHandler implements `folder` with a single Path arg (parents=true).
func folderHandler(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, reg *engine.Registry) ([]engine.Value, error) {
	pathVal := args[0]
	if !pathVal.IsPath() {
		return nil, fmt.Errorf("folder: expected Path, got %s", pathVal.VType.String())
	}
	_as1, _ := pathVal.AsPath()
	return doFolder(_as1, true, reg)
}

// doFolder is the shared body for both folder signatures; resolves and
// creates a directory via the configured FileOps.
func doFolder(p engine.PathInfo, parents bool, reg *engine.Registry) ([]engine.Value, error) {
	ops := engine.EffectiveFileOps(reg)
	pathStr := p.String()

	if parents {
		if err := ops.MkdirAll(pathStr, 0755); err != nil {
			return []engine.Value{engine.NewError(fmt.Errorf("folder: %w", err))}, nil
		}
	} else {
		resolved, err := ops.ResolvePath(pathStr)
		if err != nil {
			return []engine.Value{engine.NewError(fmt.Errorf("folder: %w", err))}, nil
		}
		if err := ops.MkdirAll(resolved, 0755); err != nil {
			return []engine.Value{engine.NewError(fmt.Errorf("folder: %w", err))}, nil
		}
	}

	return []engine.Value{engine.NewPath(p.Parts, p.Abs)}, nil
}

// stackCollectHandler runs at execution time: wraps the top N stack
// entries into a list, preserving the rest of the stack underneath.
func stackCollectHandler(args []engine.Value, _ map[string]engine.Value, stack []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
	_as0, _ := args[0].AsConcreteInteger()
	n := int(_as0)
	if n < 0 || n > len(stack) {
		return nil, fmt.Errorf("stack: count %d out of range (stack depth %d)", n, len(stack))
	}
	items := make([]engine.Value, n)
	copy(items, stack[len(stack)-n:])
	return append(stack, engine.NewList(items)), nil
}

// stackCollectCheckFullStackFn is the check-mode model for `stack N`:
// we don't know N statically, so produce a typed-list carrier whose
// element type joins all preserved stack carriers, leaving the original
// stack intact below it.
func stackCollectCheckFullStackFn(_ []engine.Value, stack []engine.Value, _ *engine.Registry) []engine.Value {
	var elem engine.Type = engine.TAny
	if len(stack) > 0 {
		elem = stack[0].VType
		for i := 1; i < len(stack); i++ {
			elem = engine.CommonAncestorType(elem, stack[i].VType)
		}
	}
	return append(append([]engine.Value(nil), stack...), engine.NewCarrierTypedList(elem))
}

// nowHandler returns the current UTC instant as an Instant value.
func nowHandler(_ []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
	return []engine.Value{engine.NewInstant(time.Now())}, nil
}

// sleepHandler pauses the current goroutine for the given milliseconds.
func sleepHandler(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
	ms, _ := args[0].AsConcreteInteger()
	if ms < 0 {
		return nil, fmt.Errorf("sleep: milliseconds must be non-negative, got %d", ms)
	}
	time.Sleep(time.Duration(ms) * time.Millisecond)
	return nil, nil
}

// intervalListHandler / intervalAtomHandler schedule a repeated callback
// (a quoted code list or word) at the given millisecond interval.
func intervalListHandler(args []engine.Value, ctx map[string]engine.Value, stack []engine.Value, r *engine.Registry) ([]engine.Value, error) {
	return startInterval(args, r, true)
}

func intervalAtomHandler(args []engine.Value, ctx map[string]engine.Value, stack []engine.Value, r *engine.Registry) ([]engine.Value, error) {
	return startInterval(args, r, false)
}

func startInterval(args []engine.Value, r *engine.Registry, isList bool) ([]engine.Value, error) {
	ms, _ := args[0].AsConcreteInteger()
	if ms <= 0 {
		return nil, fmt.Errorf("interval: milliseconds must be positive, got %d", ms)
	}
	callback := args[1]

	id := engine.GenerateID("T_")
	ticker := time.NewTicker(time.Duration(ms) * time.Millisecond)
	done := make(chan struct{})

	go func() {
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				engine.RunTimerCallback(r, callback, isList)
			}
		}
	}()

	info := &engine.IntervalInfo{
		ID:     id,
		Ms:     ms,
		Ticker: ticker,
		Done:   done,
	}
	return []engine.Value{engine.NewInterval(info)}, nil
}

// cancelTimeoutHandler stops a pending Timeout timer.
func cancelTimeoutHandler(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
	ti, err := args[0].AsTimeout()
	if err != nil {
		return nil, err
	}
	if ti.Timer != nil {
		ti.Timer.Stop()
		ti.Timer = nil
	}
	return nil, nil
}

// cancelIntervalHandler stops a running Interval ticker and signals its
// goroutine to exit.
func cancelIntervalHandler(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
	ii, err := args[0].AsInterval()
	if err != nil {
		return nil, err
	}
	if ii.Ticker != nil {
		ii.Ticker.Stop()
		close(ii.Done)
		ii.Ticker = nil
	}
	return nil, nil
}
