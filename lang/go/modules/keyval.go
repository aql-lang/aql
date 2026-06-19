package modules

import (
	"github.com/aql-lang/aql/lang/go/native"
)

// BuildKeyValModule creates the "aql:keyval" native module. It registers
// the key-value store words into an isolated sub-registry and returns a
// ModuleDesc with a "KV" export of FnDef wrappers.
//
// After import, the words are accessed via dot notation:
//
//	"aql:keyval" import
//	def kv (KV.open "memory")
//	kv KV.set "user:1" {name:'ann'}
//	kv KV.get "user:1"          # → {name:'ann'}
//	kv KV.keys "user:"          # → ['user:1']
//	kv KV.close
//
// The default backend is in-memory; external backends (file, Redis,
// MongoDB, S3) are selected by the open kind and are unavailable in the
// wasm build. The namespace is "KV" (not "KeyVal") to avoid colliding
// with the Node/Map/KeyVal map-iteration type.
func BuildKeyValModule(parent *native.Registry) (native.ModuleDesc, error) {
	subReg, err := native.DefaultRegistry()
	if err != nil {
		return native.ModuleDesc{}, err
	}
	for _, n := range native.KeyValModuleNatives {
		subReg.RegisterNativeFunc(n)
	}

	exports := native.NewOrderedMap()
	for _, n := range native.KeyValModuleNatives {
		exports.Set(n.Name, makeModuleFnDef(n, subReg))
	}

	modID := parent.Modules.NextID()
	desc := native.ModuleDesc{
		ID:      modID,
		Exports: map[string]*native.OrderedMap{"KV": exports},
	}
	return desc, nil
}
