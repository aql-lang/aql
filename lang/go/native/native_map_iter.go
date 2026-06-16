package native

import "fmt"

// Map overloads for the higher-order words each / for-each / fold / scan (and
// the Function form of filter, in filter.go), plus the keys / vals projections.
//
// A Map is iterated entry by entry in insertion order. Two body forms, mirrored
// across every word and SPLIT BY SIGNATURE (never by sniffing the body value's
// type — a compiled-closure quotation body and a lambda value both have
// Parent=TFunction, so only the matched signature discriminates them):
//
//   - quotation `[body]` (sig `[TList, TMap]`) — the entry's VALUE is pushed on
//     the stack; the body runs through the InvokeBody seam, so a compiled
//     closure runs VM-native and a raw token list runs a sub-engine (identical
//     to the historical New(reg).Run path). The result keeps the map shape.
//     e.g. `{a:1 b:2} each [mul 10]`.
//   - lambda `(kv => …)` (sig `[TFunction, TMap]`) — the body is handed a
//     KeyVal {k v i n} so it can use the key/index/total; it runs through
//     CallAQL. The result still keeps the map shape.
//     e.g. `{a:1 b:2} each (kv => [kv.v mul 10])`.
//
// each → Map (values transformed, keys kept); for-each → nothing;
// fold → the accumulator; scan → Map (running fold, keys kept);
// filter → Map (entries kept by a Boolean predicate).
// To leave the map shape and get a List, use keys / vals (or StructUtil.items).

// mapBody runs an iteration body once per map entry. The form is fixed at
// construction by the matched signature, not inferred from the body value.
type mapBody struct {
	lambda bool
	body   Value // quotation list / compiled closure, or the lambda fn
}

// newQuoteBody prepares a quotation/closure body: each entry's VALUE is its sole
// input, run through InvokeBody (VM-native for a compiled closure, sub-engine
// for a raw token list — byte-identical to the old runQuotationBody path).
func newQuoteBody(body Value) mapBody {
	return mapBody{body: body}
}

// newLambdaBody prepares a Function body handed a KeyVal per entry. It runs
// through invokeCallback: a compiled closure VM-native via InvokeBody, an
// interpreter FnDefInfo lambda via CallAQL with its captures.
func newLambdaBody(body Value) mapBody {
	return mapBody{lambda: true, body: body}
}

// value runs the body for one entry with no accumulator. ok=false when the body
// left the stack empty.
func (mb mapBody) value(reg *Registry, k string, v Value, i, n int64) (Value, bool, error) {
	if mb.lambda {
		return mb.callLambda(reg, []Value{NewKeyVal(k, v, i, n)})
	}
	return topOfRun(InvokeBody(reg, mb.body, []Value{v}))
}

// fold runs the body for one entry with an accumulator. The quotation form
// pushes the accumulator first and the value on top (same stack order as list
// fold: a 2-arg word sees value=top, acc=deeper); the lambda receives
// (accumulator, KeyVal).
func (mb mapBody) fold(reg *Registry, acc Value, k string, v Value, i, n int64) (Value, bool, error) {
	if mb.lambda {
		return mb.callLambda(reg, []Value{acc, NewKeyVal(k, v, i, n)})
	}
	return topOfRun(InvokeBody(reg, mb.body, []Value{acc, v}))
}

func (mb mapBody) callLambda(reg *Registry, args []Value) (Value, bool, error) {
	return topOfRun(invokeCallback(reg, mb.body, args))
}

// topOfRun returns the top of a body's residual stack (ok=false when empty).
func topOfRun(res []Value, err error) (Value, bool, error) {
	if err != nil {
		return Value{}, false, err
	}
	if len(res) == 0 {
		return Value{}, false, nil
	}
	return res[len(res)-1], true, nil
}

// requireConcreteMap unwraps a concrete Map arg or returns a clear error.
func requireConcreteMap(reg *Registry, v Value, word string) (ReadMap, error) {
	if !IsConcrete(v) || !v.Parent.ConformsTo(TMap) {
		return nil, reg.AqlError(word+"_error", word+": expected a concrete map", word)
	}
	m, _ := AsMap(v)
	if m == nil {
		return nil, reg.AqlError(word+"_error", word+": expected a concrete map", word)
	}
	return m, nil
}

// ---- each ----

// eachMapWith maps a body over a map's values, keeping the keys — `mapValues`.
func eachMapWith(reg *Registry, mb mapBody, data ReadMap) ([]Value, error) {
	keys := data.Keys()
	n := int64(len(keys))
	out := NewOrderedMap()
	for idx, k := range keys {
		v, _ := data.Get(k)
		res, ok, err := mb.value(reg, k, v, int64(idx), n)
		if err != nil {
			return nil, fmt.Errorf("each: key %q: %w", k, err)
		}
		if !ok {
			return nil, reg.AqlError("each_error", fmt.Sprintf("each: key %q: body produced no result", k), "each")
		}
		out.Set(k, res)
	}
	return []Value{NewMap(out)}, nil
}

func eachMapQuoteHandler(args []Value, _ map[string]Value, _ []Value, reg *Registry) ([]Value, error) {
	data, err := requireConcreteMap(reg, args[1], "each")
	if err != nil {
		return nil, err
	}
	return eachMapWith(reg, newQuoteBody(args[0]), data)
}

func eachMapLambdaHandler(args []Value, _ map[string]Value, _ []Value, reg *Registry) ([]Value, error) {
	data, err := requireConcreteMap(reg, args[1], "each")
	if err != nil {
		return nil, err
	}
	return eachMapWith(reg, newLambdaBody(args[0]), data)
}

// ---- for-each ----

// forEachMapWith runs the body once per entry for side effects, producing
// nothing.
func forEachMapWith(reg *Registry, mb mapBody, data ReadMap) ([]Value, error) {
	keys := data.Keys()
	n := int64(len(keys))
	for idx, k := range keys {
		v, _ := data.Get(k)
		if _, _, err := mb.value(reg, k, v, int64(idx), n); err != nil {
			return nil, fmt.Errorf("for-each: key %q: %w", k, err)
		}
	}
	return nil, nil
}

func forEachMapQuoteHandler(args []Value, _ map[string]Value, _ []Value, reg *Registry) ([]Value, error) {
	data, err := requireConcreteMap(reg, args[1], "for-each")
	if err != nil {
		return nil, err
	}
	return forEachMapWith(reg, newQuoteBody(args[0]), data)
}

func forEachMapLambdaHandler(args []Value, _ map[string]Value, _ []Value, reg *Registry) ([]Value, error) {
	data, err := requireConcreteMap(reg, args[1], "for-each")
	if err != nil {
		return nil, err
	}
	return forEachMapWith(reg, newLambdaBody(args[0]), data)
}

// ---- fold ----

// foldMapWith threads the accumulator from `start` onward over the map's
// entries, passing each entry's original index/total to a lambda's KeyVal.
func foldMapWith(reg *Registry, mb mapBody, acc Value, data ReadMap, start int) ([]Value, error) {
	keys := data.Keys()
	n := int64(len(keys))
	for idx := start; idx < len(keys); idx++ {
		k := keys[idx]
		v, _ := data.Get(k)
		res, ok, err := mb.fold(reg, acc, k, v, int64(idx), n)
		if err != nil {
			return nil, fmt.Errorf("fold: key %q: %w", k, err)
		}
		if !ok {
			return nil, reg.AqlError("fold_error", fmt.Sprintf("fold: key %q: body produced no result", k), "fold")
		}
		acc = res
	}
	return []Value{acc}, nil
}

// foldMapInitQuoteHandler reduces a map's entries with an explicit seed —
// `init fold [body] {map}`. Backs `[TList, TMap, TAny]`.
func foldMapInitQuoteHandler(args []Value, _ map[string]Value, _ []Value, reg *Registry) ([]Value, error) {
	data, err := requireConcreteMap(reg, args[1], "fold")
	if err != nil {
		return nil, err
	}
	return foldMapWith(reg, newQuoteBody(args[0]), args[2], data, 0)
}

// foldMapInitLambdaHandler is the lambda form of `init fold (kv => …) {map}`.
// Backs `[TFunction, TMap, TAny]`.
func foldMapInitLambdaHandler(args []Value, _ map[string]Value, _ []Value, reg *Registry) ([]Value, error) {
	data, err := requireConcreteMap(reg, args[1], "fold")
	if err != nil {
		return nil, err
	}
	return foldMapWith(reg, newLambdaBody(args[0]), args[2], data, 0)
}

// foldMapNoInit seeds the accumulator from the first value, then folds the rest.
func foldMapNoInit(reg *Registry, mb mapBody, data ReadMap) ([]Value, error) {
	keys := data.Keys()
	if len(keys) == 0 {
		return nil, reg.AqlError("fold_error", "fold: empty map with no initial value", "fold")
	}
	first, _ := data.Get(keys[0])
	return foldMapWith(reg, mb, first, data, 1)
}

// foldMapNoInitQuoteHandler reduces a map's entries, seeding from the first
// value — `fold [body] {map}`. Backs `[TList, TMap]`.
func foldMapNoInitQuoteHandler(args []Value, _ map[string]Value, _ []Value, reg *Registry) ([]Value, error) {
	data, err := requireConcreteMap(reg, args[1], "fold")
	if err != nil {
		return nil, err
	}
	return foldMapNoInit(reg, newQuoteBody(args[0]), data)
}

// foldMapNoInitLambdaHandler is the lambda form of `fold (kv => …) {map}`.
// Backs `[TFunction, TMap]`.
func foldMapNoInitLambdaHandler(args []Value, _ map[string]Value, _ []Value, reg *Registry) ([]Value, error) {
	data, err := requireConcreteMap(reg, args[1], "fold")
	if err != nil {
		return nil, err
	}
	return foldMapNoInit(reg, newLambdaBody(args[0]), data)
}

// ---- scan ----

// scanMapWith is the running (prefix) fold over a map's values: the first value
// seeds the accumulator and is the first output, then each later entry's body
// result becomes that key's output. Keeps the map shape (keys preserved).
func scanMapWith(reg *Registry, mb mapBody, data ReadMap) ([]Value, error) {
	keys := data.Keys()
	out := NewOrderedMap()
	if len(keys) == 0 {
		return []Value{NewMap(out)}, nil
	}
	n := int64(len(keys))
	acc, _ := data.Get(keys[0])
	out.Set(keys[0], acc) // first value seeds and is the first output
	for idx := 1; idx < len(keys); idx++ {
		k := keys[idx]
		v, _ := data.Get(k)
		res, ok, err := mb.fold(reg, acc, k, v, int64(idx), n)
		if err != nil {
			return nil, fmt.Errorf("scan: key %q: %w", k, err)
		}
		if !ok {
			return nil, reg.AqlError("scan_error", fmt.Sprintf("scan: key %q: body produced no result", k), "scan")
		}
		acc = res
		out.Set(k, acc)
	}
	return []Value{NewMap(out)}, nil
}

func scanMapQuoteHandler(args []Value, _ map[string]Value, _ []Value, reg *Registry) ([]Value, error) {
	data, err := requireConcreteMap(reg, args[1], "scan")
	if err != nil {
		return nil, err
	}
	return scanMapWith(reg, newQuoteBody(args[0]), data)
}

func scanMapLambdaHandler(args []Value, _ map[string]Value, _ []Value, reg *Registry) ([]Value, error) {
	data, err := requireConcreteMap(reg, args[1], "scan")
	if err != nil {
		return nil, err
	}
	return scanMapWith(reg, newLambdaBody(args[0]), data)
}

// keysHandler returns a map's keys as a list, in insertion order.
func keysHandler(args []Value, _ map[string]Value, _ []Value, reg *Registry) ([]Value, error) {
	data, err := requireConcreteMap(reg, args[0], "keys")
	if err != nil {
		return nil, err
	}
	ks := data.Keys()
	out := make([]Value, len(ks))
	for i, k := range ks {
		out[i] = NewString(k)
	}
	return []Value{NewList(out)}, nil
}

// valsHandler returns a map's values as a list, in insertion order.
func valsHandler(args []Value, _ map[string]Value, _ []Value, reg *Registry) ([]Value, error) {
	data, err := requireConcreteMap(reg, args[0], "vals")
	if err != nil {
		return nil, err
	}
	ks := data.Keys()
	out := make([]Value, len(ks))
	for i, k := range ks {
		v, _ := data.Get(k)
		out[i] = v
	}
	return []Value{NewList(out)}, nil
}

// mapNatives are the standalone map-projection words. The each / for-each /
// fold / scan Map overloads live on those words' own signatures
// (native_array.go); the filter Function-over-map path lives in filter.go.
var mapNatives = []NativeFunc{
	{
		Name: "keys",
		Signatures: []NativeSig{
			{Args: []*Type{TMap}, Handler: keysHandler, Returns: []*Type{TList}, BarrierPos: -1},
		},
	},
	{
		Name: "vals",
		Signatures: []NativeSig{
			{Args: []*Type{TMap}, Handler: valsHandler, Returns: []*Type{TList}, BarrierPos: -1},
		},
	},
}
