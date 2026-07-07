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
	subReg, err := newDefaultRegistry()
	if err != nil {
		return native.ModuleDesc{}, err
	}
	// Response values escape to the importer, so the Fetch-family mints
	// draw IDs from the importing tree's counter.
	subReg.Types.AdoptSeqFrom(parent.Types)
	ft := native.MintFetchTypes(subReg)
	netNatives := native.NetModuleNatives(ft)
	for _, n := range netNatives {
		subReg.RegisterNativeFunc(n)
	}

	exports := delegatingExports(netNatives, subReg)
	// The module-owned Fetch types, exported as type literals —
	// `resp is Net.Response` — mirroring IO.StreamKind.
	exports.Set("Fetch", native.NewTypeLiteral(ft.Fetch))
	exports.Set("Request", native.NewTypeLiteral(ft.Request))
	exports.Set("Response", native.NewTypeLiteral(ft.Response))

	return moduleDesc(parent, "Net", subReg, exports), nil
}
