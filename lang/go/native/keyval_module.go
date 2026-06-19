package native

import (
	"fmt"

	"github.com/aql-lang/aql/lang/go/policy"
)

// KeyValModuleNatives holds the key-value store words surfaced through
// the loadable aql:keyval module (namespace KV). Registered ONLY into
// that module's sub-registry by modules.BuildKeyValModule.
//
//	open    open a store              KV.open "memory"
//	                                   KV.open "redis" "redis://host:6379"
//	get     read a key (None on miss) kv KV.get "user:1"
//	set     write a key               kv KV.set "user:1" {name:'ann'}
//	del     delete a key (was-present) kv KV.del "user:1"
//	has     membership test           kv KV.has "user:1"
//	keys    keys (optional prefix)    kv KV.keys "" ; kv KV.keys "user:"
//	close   close the store           kv KV.close
//
// All inner sigs use BarrierPos -1 (module-wrapper swap-form dispatch).
// Reads are gated through the `keyval` policy scope; writes additionally
// require the mutate global.
var KeyValModuleNatives = []NativeFunc{
	{
		Name: "open",
		Signatures: []NativeSig{
			{Args: []*Type{TString, TString}, Handler: kvOpenHandler, Returns: []*Type{TKVStore}, BarrierPos: -1},
			{Args: []*Type{TString}, Handler: kvOpenKindHandler, Returns: []*Type{TKVStore}, BarrierPos: -1},
		},
	},
	{
		Name: "get",
		Signatures: []NativeSig{
			{Args: []*Type{TString, TKVStore}, Handler: kvGetHandler, Returns: []*Type{TAny}, BarrierPos: -1},
		},
	},
	{
		Name: "set",
		Signatures: []NativeSig{
			{Args: []*Type{TString, TAny, TKVStore}, Handler: kvSetHandler, Returns: []*Type{TKVStore}, BarrierPos: -1},
		},
	},
	{
		Name: "del",
		Signatures: []NativeSig{
			{Args: []*Type{TString, TKVStore}, Handler: kvDelHandler, Returns: []*Type{TBoolean}, BarrierPos: -1},
		},
	},
	{
		Name: "has",
		Signatures: []NativeSig{
			{Args: []*Type{TString, TKVStore}, Handler: kvHasHandler, Returns: []*Type{TBoolean}, BarrierPos: -1},
		},
	},
	{
		Name: "keys",
		Signatures: []NativeSig{
			{Args: []*Type{TString, TKVStore}, Handler: kvKeysHandler, Returns: []*Type{TList}, BarrierPos: -1},
			{Args: []*Type{TKVStore}, Handler: kvKeysAllHandler, Returns: []*Type{TList}, BarrierPos: -1},
		},
	},
	{
		Name: "close",
		Signatures: []NativeSig{
			{Args: []*Type{TKVStore}, Handler: kvCloseHandler, Returns: []*Type{}, BarrierPos: -1},
		},
	},
}

// checkKVPolicy gates a keyval-scope operation. A nil registry or absent
// policy is the allow-everything default; keyval.install=false produces
// capability_not_installed.
func checkKVPolicy(r *Registry, op string, args policy.Args) error {
	if r == nil {
		return nil
	}
	pol := HostPolicy(r)
	if pol == nil {
		return nil
	}
	if !pol.Installed("keyval") {
		return &policy.Denied{
			Code:    policy.CodeCapabilityNotInstalled,
			Scope:   "keyval",
			Op:      op,
			Profile: pol.Name(),
			Blame:   "keyval.install=false",
			Args:    args,
		}
	}
	return pol.Check("keyval", op, args)
}

func kvStoreArg(r *Registry, v Value, word string) (*KVStore, error) {
	s, ok := AsKVStore(v)
	if !ok {
		return nil, r.AqlError("keyval_error", word+": expected a key-value store, got "+v.Parent.String(), word)
	}
	return s, nil
}

func kvOpen(r *Registry, kind, dsn string) ([]Value, error) {
	if err := checkKVPolicy(r, "open", policy.Args{"kind": kind}); err != nil {
		return nil, err
	}
	s, err := OpenKVStore(kind, dsn)
	if err != nil {
		return nil, r.AqlError("keyval_error", "open: "+err.Error(), "open")
	}
	return []Value{NewKVStore(s)}, nil
}

func kvOpenHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	kind, err := args[0].AsConcreteString()
	if err != nil {
		return nil, fmt.Errorf("kv open: %w", err)
	}
	dsn, err := args[1].AsConcreteString()
	if err != nil {
		return nil, fmt.Errorf("kv open: %w", err)
	}
	return kvOpen(r, kind, dsn)
}

func kvOpenKindHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	kind, err := args[0].AsConcreteString()
	if err != nil {
		return nil, fmt.Errorf("kv open: %w", err)
	}
	return kvOpen(r, kind, "")
}

func kvGetHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	key := StoreKey(args[0])
	s, err := kvStoreArg(r, args[1], "get")
	if err != nil {
		return nil, err
	}
	if err := checkKVPolicy(r, "read", policy.Args{"kind": s.Kind, "key": key}); err != nil {
		return nil, err
	}
	v, ok, err := s.Backend.Get(key)
	if err != nil {
		return nil, r.AqlError("keyval_error", "get: "+err.Error(), "get")
	}
	if !ok {
		return []Value{NewNone()}, nil
	}
	return []Value{v}, nil
}

func kvSetHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	key := StoreKey(args[0])
	s, err := kvStoreArg(r, args[2], "set")
	if err != nil {
		return nil, err
	}
	if err := checkKVPolicy(r, "write", policy.Args{"kind": s.Kind, "key": key}); err != nil {
		return nil, err
	}
	if err := s.Backend.Set(key, args[1]); err != nil {
		return nil, r.AqlError("keyval_error", "set: "+err.Error(), "set")
	}
	// Return the store so calls can chain in a pipeline.
	return []Value{args[2]}, nil
}

func kvDelHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	key := StoreKey(args[0])
	s, err := kvStoreArg(r, args[1], "del")
	if err != nil {
		return nil, err
	}
	if err := checkKVPolicy(r, "write", policy.Args{"kind": s.Kind, "key": key}); err != nil {
		return nil, err
	}
	existed, err := s.Backend.Delete(key)
	if err != nil {
		return nil, r.AqlError("keyval_error", "del: "+err.Error(), "del")
	}
	return []Value{NewBoolean(existed)}, nil
}

func kvHasHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	key := StoreKey(args[0])
	s, err := kvStoreArg(r, args[1], "has")
	if err != nil {
		return nil, err
	}
	if err := checkKVPolicy(r, "read", policy.Args{"kind": s.Kind, "key": key}); err != nil {
		return nil, err
	}
	ok, err := s.Backend.Has(key)
	if err != nil {
		return nil, r.AqlError("keyval_error", "has: "+err.Error(), "has")
	}
	return []Value{NewBoolean(ok)}, nil
}

func kvKeysHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	prefix, err := args[0].AsConcreteString()
	if err != nil {
		return nil, fmt.Errorf("kv keys: %w", err)
	}
	return kvKeys(r, prefix, args[1])
}

func kvKeysAllHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	return kvKeys(r, "", args[0])
}

func kvKeys(r *Registry, prefix string, storeV Value) ([]Value, error) {
	s, err := kvStoreArg(r, storeV, "keys")
	if err != nil {
		return nil, err
	}
	if err := checkKVPolicy(r, "read", policy.Args{"kind": s.Kind}); err != nil {
		return nil, err
	}
	keys, err := s.Backend.Keys(prefix)
	if err != nil {
		return nil, r.AqlError("keyval_error", "keys: "+err.Error(), "keys")
	}
	out := make([]Value, len(keys))
	for i, k := range keys {
		out[i] = NewString(k)
	}
	return []Value{NewList(out)}, nil
}

func kvCloseHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	s, err := kvStoreArg(r, args[0], "close")
	if err != nil {
		return nil, err
	}
	if err := s.Close(); err != nil {
		return nil, r.AqlError("keyval_error", "close: "+err.Error(), "close")
	}
	return []Value{}, nil
}
