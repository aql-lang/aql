package engine

import "fmt"

// storageNatives covers the production-only Store-side container
// access — `context` plus the Store-targeted sigs of `set` and
// `get`. The non-context kernel sigs (Map / List / Object / Array)
// live in eng and are installed via eng.RegisterCoreStorage from
// register.go; these entries register ADDITIONAL signatures on top
// of those, keeping the dispatch table for `set` / `get` unified
// from the caller's perspective.
//
// `set` and `get` carry ReturnsFn closures that thread the static
// type tracker (r.RecordContextSet / r.LookupContextType) so
// check-mode can recover a typed carrier from a previous set on the
// same key. Both are context-aware machinery and therefore live
// here, not in the kernel.
var storageNatives = []NativeFunc{
	{
		Name:        "set",
		ForwardArgs: true,
		Signatures: []NativeSig{
			// Store (copy-on-write)
			{
				Args:      []*Type{TString, TAny, TStore},
				Handler:   setStoreHandler,
				Returns:   []*Type{},
				ReturnsFn: setStoreReturnsFn,
			},
			{
				Args:      []*Type{TAtom, TAny, TStore},
				QuoteArgs: map[int]bool{0: true},
				Handler:   setStoreHandler,
				Returns:   []*Type{},
				ReturnsFn: setStoreReturnsFn,
			},
		},
	},
	{
		Name:        "get",
		ForwardArgs: true,
		Signatures: []NativeSig{
			// [Key | Store] — check-mode-aware ReturnsFn picks up a
			// typed carrier from a previously-set key.
			{
				Args: []*Type{TString, TStore}, BarrierPos: 1, Handler: getStoreHandler,
				ReturnsFn: getStoreReturnsFn,
			},
			{
				Args: []*Type{TAtom, TStore}, QuoteArgs: map[int]bool{0: true}, BarrierPos: 1, Handler: getStoreHandler,
				ReturnsFn: getStoreReturnsFn,
			},
		},
	},
	{
		Name:        "context",
		ForwardArgs: true,
		Signatures: []NativeSig{{
			Args:    []*Type{},
			Handler: contextHandler,
			Returns: []*Type{TStore},
		}},
	},
}

// ---- set Store handler ----

func setStoreHandler(args []Value, _ map[string]Value, _ []Value, reg *Registry) ([]Value, error) {
	store := AsStore(args[2])
	if store == nil {
		return nil, fmt.Errorf("set: expected a Store, got %s", args[2].VType.String())
	}
	key := StoreKey(args[0])
	CowSet(store, key, args[1], reg)
	return nil, nil
}

func setStoreReturnsFn(args []Value, r *Registry) []Value {
	r.Check.RecordContextSet(StoreKey(args[0]), args[1])
	return nil
}

// ---- get Store handler ----

func getStoreHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	store := AsStore(args[1])
	if store == nil {
		return nil, fmt.Errorf("get: expected a Store, got %s", args[1].VType.String())
	}
	key := getKey(args[0])
	val, ok := store.Get(key)
	if !ok {
		return nil, fmt.Errorf("unknown key: %s", key)
	}
	return []Value{val}, nil
}

func getStoreReturnsFn(args []Value, r *Registry) []Value {
	v, _ := r.Check.LookupContextType(StoreKey(args[0]))
	return []Value{v}
}

// ---- context handler ----

// contextHandler implements the "context" word that pushes the
// current context Store onto the stack.
//
// The context is a Store (Object/Store), allowing get/set to operate on it
// directly and prototype chain resolution for nested scopes.
func contextHandler(_ []Value, _ map[string]Value, _ []Value, reg *Registry) ([]Value, error) {
	store := reg.Contexts.Top()
	if store == nil {
		return nil, fmt.Errorf("context: no active context")
	}
	return []Value{NewStoreValue(TStore, store)}, nil
}
