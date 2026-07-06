package native

import (
	"encoding/json"

	udk "voxgiguniversalsdk"
)

// Shared test seams (design/TEST-SEAMS.10.md). Every var here defaults
// to the real implementation and is unexported — a seam is not API.
// Production code never checks "is the seam set"; tests that swap one
// must restore it via t.Cleanup and must not run in t.Parallel with
// other tests reading the same seam. Single-consumer seams live next
// to their consumer; this file holds only the seams shared by several
// files of the package.

// jsonMarshal seams encoding/json.Marshal for the marshal-failure arms
// (renderJSON in log_module.go, jsonRoundTrip in tabnas.go).
var jsonMarshal = json.Marshal

// newSubRegistry seams DefaultRegistry for the init-error arms of the
// internal sub-registry constructions (buildLoggerInstance in
// log_logger.go, buildMeterInstance in log_metrics.go, span state in
// log_span.go, RunModuleBody in native_module_module.go).
var newSubRegistry = DefaultRegistry

// structConvert seams anyToValue for the convert-back failure arms of the
// voxgig-struct word handlers (filter / flatten / getpath / inject / merge /
// items). Their inputs come from valueToAny, which only produces JSON-shaped
// Go values that anyToValue always accepts — but the voxgig-struct library
// owns that round-trip guarantee, not this package.
var structConvert = anyToValue

// newEngRegistry seams eng.NewRegistry for the kernel-init error arm of
// DefaultRegistryWithPolicy (setup.go) — eng.NewRegistry fails only on
// an init-time builtin-table error, which is unreachable from here.
var newEngRegistry = NewRegistry

// newSQLiteStoreFn seams NewSQLiteStore for the store-init error arm of
// DefaultRegistryWithPolicy (setup.go) — sql.Open with the compiled-in
// driver cannot fail in production.
var newSQLiteStoreFn = NewSQLiteStore

// sdkClient is the narrow surface the API-word handlers (create / load /
// list / update / remove / prepare / direct) consume from the host SDK
// (seam shape 3, design/TEST-SEAMS.10.md). *udk.UniversalSDK satisfies
// it structurally; tests plant a fake into r.SDKCache, which getSDK
// (sdk_helper.go) consults before falling back to the real manager.
type sdkClient interface {
	Entity(name string, entopts map[string]any) udk.UniversalEntity
	Prepare(fetchargs map[string]any) (map[string]any, error)
	Direct(fetchargs map[string]any) (map[string]any, error)
}
