package modules

import (
	"github.com/boru-lang/boru/lang/go/native"
)

// BuildNetModule creates the "boru:net" native module. It registers the
// HTTP / API access words (formerly core) into an isolated sub-registry and
// returns a ModuleDesc with a "Net" export containing FnDef wrappers.
//
// After import, the words are accessed via dot notation:
//
//	import "boru:net"
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
	// Stamp this module's provenance onto the types it mints, as
	// boru:matrix-util / boru:time-util / boru:io already do. Beyond
	// consistency it is load-bearing: a word extension may only be
	// anchored on a nominal type the extending scope OWNS
	// (requireOwnedAnchor), so without this the accessor overloads below
	// are refused with extend_owner.
	subReg.Types.MintOwner = "boru:net"
	ft := native.MintFetchTypes(subReg)
	netNatives := native.NetModuleNatives(ft)
	netNatives = append(netNatives, socketNatives()...) // Tier-1 sockets (net_socket.go)
	netNatives = append(netNatives, codecNatives()...)  // Tier-2 codecs + listen/connect (net_codec.go)
	for _, n := range netNatives {
		subReg.RegisterNativeFunc(n)
	}

	exports := delegatingExports(netNatives, subReg)
	// The module-owned Fetch types, exported as type literals —
	// `resp is Net.Response` — mirroring IO.StreamKind.
	exports.Set("Fetch", native.NewTypeLiteral(ft.Fetch))
	exports.Set("Request", native.NewTypeLiteral(ft.Request))
	exports.Set("Response", native.NewTypeLiteral(ft.Response))
	// The global socket handle types, as literals for `x is Net.Socket`.
	exports.Set("Socket", native.NewTypeLiteral(TSocket))
	exports.Set("Listener", native.NewTypeLiteral(TListener))
	// Accessor overloads, exported as WORD EXTENSIONS: import transplants
	// them onto the importer's bare dot/get/dotr/getr/has so a Response
	// answers `.status` directly. Same mechanism boru:matrix-util uses for
	// add/sub/mul, anchored on this import's minted Fetch type.
	for _, ext := range native.FetchAccessorExtensions(ft) {
		exports.Set(ext.Extends, native.NewFnDef(ext))
	}
	// Built-in codec values (plain {decode encode} maps whose functions
	// carry Go handlers): `listen {tcp: 8080 codec: Net.http} svc`.
	exports.Set("lines", makeLinesCodec())
	exports.Set("json-lines", makeJSONLinesCodec())
	exports.Set("http", makeHTTPCodec())

	return moduleDesc(parent, "Net", subReg, exports), nil
}
