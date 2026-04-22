package native

import (
	"github.com/metsitaba/voxgig-exp/aql/internal/engine"
)

// Register installs all native functions into the given registry and sets
// ModuleInitFunc so that module sub-registries automatically get the same
// words.
func Register(r *engine.Registry) {
	registerAll(r)
	r.ModuleInitFunc = func(child *engine.Registry) {
		registerAll(child)
	}
}

// registerAll calls every RegisterXxx function owned by this package.
func registerAll(r *engine.Registry) {
	// Words successfully relocated from engine. Words that are exercised
	// by engine-internal tests (stack ops, most definitions, record/table
	// etc.) must stay in engine because those tests are `package engine`
	// and cannot import native (Go import cycle).

	// Boolean (implies)
	RegisterImplies(r)

	// Control flow (quote)
	RegisterQuote(r)

	// I/O (folder)
	RegisterFolder(r)

	// String (slice)
	RegisterStringSlice(r)

	// Stack (StackCollect)
	RegisterStackCollect(r)

	// Temporal (now, sleep, interval, cancel)
	RegisterNow(r)
	RegisterSleep(r)
	RegisterInterval(r)
	RegisterCancel(r)

	// Data manipulation (always lived in native)
	RegisterList(r)
	RegisterCreate(r)
	RegisterLoad(r)
	RegisterUpdate(r)
	RegisterRemove(r)
	RegisterTransform(r)
	RegisterMerge(r)
	RegisterValidate(r)
	RegisterGetpath(r)
	RegisterSetpath(r)
	RegisterInject(r)
	RegisterClone(r)
	RegisterWalk(r)
	RegisterSelector(r)
	RegisterSize(r)
	RegisterPad(r)
	RegisterItems(r)
	RegisterFetch(r)
	RegisterPrepare(r)
	RegisterDirect(r)
	RegisterFlatten(r)
	RegisterFilter(r)
	RegisterJoin(r)
	RegisterJsonify(r)
	RegisterPush(r)
	RegisterPop(r)
	RegisterUnshift(r)
	RegisterShift(r)
	RegisterIstype(r)
}
