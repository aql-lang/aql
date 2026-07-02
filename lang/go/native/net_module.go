package native

// NetModuleNatives holds the HTTP / API access words that were moved out of
// the core registry into the loadable `aql:net` module (namespace `Net`).
// They are registered ONLY into that module's sub-registry by
// modules.BuildNetModule — deliberately absent from the global registry. The
// handlers themselves still live in their feature files (fetch.go's
// fetch*Handler, the API prepare/direct handlers in this package).
//
// At dispatch the module FnDef wrapper short-circuits to the inner native's
// handler, which runs against the LIVE engine registry (execMatch passes
// e.registry, not the sub-registry) — so Net.fetch reaches the host network
// policy and Net.prepare / Net.direct reach the host API SDKs, exactly as the
// former core words did.
//
//	fetch    perform an HTTP request (URL string and/or options map)
//	prepare  build a request from an API descriptor without sending
//	direct   build-and-send a request against an API descriptor
//
// prepare/direct carry the shared API-descriptor pattern on arg 0 (the inner
// native's signature drives dot-access dispatch, so the pattern is preserved
// even though the wrapper FnSig does not copy it).
var NetModuleNatives = []NativeFunc{
	{
		Name: "fetch",
		Signatures: []Signature{
			{Args: []*Type{TString, TMap}, Impl: Go(fetchStringMapHandler), BarrierPos: -1},
			{Args: []*Type{TMap}, Impl: Go(fetchMapHandler), BarrierPos: -1},
			{Args: []*Type{TString}, Impl: Go(fetchStringHandler), BarrierPos: -1},
		},
	},
	{
		Name: "prepare",
		Signatures: []Signature{
			{Args: []*Type{TMap}, Impl: Go(prepareAPIHandler), Patterns: map[int]Value{0: apiPatternValue()}, BarrierPos: -1},
		},
	},
	{
		Name: "direct",
		Signatures: []Signature{
			{Args: []*Type{TMap}, Impl: Go(directAPIHandler), Patterns: map[int]Value{0: apiPatternValue()}, BarrierPos: -1},
		},
	},
}
