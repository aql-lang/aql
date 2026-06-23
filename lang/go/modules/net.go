package modules

import (
	"github.com/aql-lang/aql/lang/go/native"
)

// BuildNetModule creates the "aql:net" native module. It registers the
// HTTP / API access words (formerly core) into an isolated sub-registry and
// returns a ModuleDesc with a "Net" export containing FnDef wrappers.
//
// After import, the words are accessed via dot notation:
//
//	import "aql:net"
//	"https://example.com/data" Net.fetch                 # HTTP GET
//	{kind:"api", spec:"…", path:"/x"} Net.prepare         # build a request
//	{kind:"api", spec:"…", path:"/x"} Net.direct          # build and send
//
// The wrappers dispatch to the inner native's handler via the
// trivial-delegation short-circuit, which runs against the LIVE engine
// registry — so Net.fetch reaches the host network policy and Net.prepare /
// Net.direct reach the host API SDKs, exactly as the former core words did.
func BuildNetModule(parent *native.Registry) (native.ModuleDesc, error) {
	subReg, err := native.DefaultRegistry()
	if err != nil {
		return native.ModuleDesc{}, err
	}
	for _, n := range native.NetModuleNatives {
		subReg.RegisterNativeFunc(n)
	}

	exports := native.NewOrderedMap()
	for _, n := range native.NetModuleNatives {
		exports.Set(n.Name, makeModuleFnDef(n, subReg))
	}

	modID := parent.Modules.NextID()
	desc := native.ModuleDesc{
		ID:      modID,
		Exports: map[string]*native.OrderedMap{"Net": exports},
	}
	return desc, nil
}
