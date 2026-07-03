package modules

import (
	"github.com/aql-lang/aql/lang/go/native"
)

// BuildLogModule creates the "aql:log" native module (namespace `Log`).
// It registers the logging words into an isolated sub-registry and
// returns a ModuleDesc whose "Log" export carries FnDef wrappers.
//
// After import, the words are accessed via dot notation:
//
//	import "aql:log"
//	Log.info "server started"                       # INFO record → console
//	Log.warn "slow query" {ms:1200 table:"orders"}   # …with structured fields
//	warn/q Log.set-level                              # raise the global threshold
//	memory/q Log.add-sink   Log.dump                  # capture & inspect records
//
// Phase 1 ships the traditional-logging layer: six severity levels, a
// global level threshold, structured fields, a console sink built on
// Go's standard `log` package, plus `memory` and `null` sinks. The
// OpenTelemetry abstraction (provider hooks, spans) lands in later
// phases — see design/LOG-MODULE.10.md.
//
// The wrappers dispatch to the inner native's handler via the
// trivial-delegation short-circuit, which runs against the LIVE engine
// registry — so the handlers reach the host ErrOutput, Clock, and
// Policy exactly as the core `print` word does.
func BuildLogModule(parent *native.Registry) (native.ModuleDesc, error) {
	subReg, err := native.DefaultRegistry()
	if err != nil {
		return native.ModuleDesc{}, err
	}

	// One sink registry per import, captured by every word closure (the
	// aql:rand state pattern). A host may pre-install one via
	// SetHostLogSinks(parent, …) before `import "aql:log"` runs — e.g. to
	// attach an OTel sink — and we honour it; otherwise a default
	// (console attached) is created AND installed on the parent, so a host
	// that calls RegisterHostLogSink AFTER importing reaches this same
	// registry (HostLogSinks would otherwise return nil and the late sink
	// would land on a second registry the imported Log.* words never use).
	lsr := native.HostLogSinks(parent)
	if lsr == nil {
		lsr = native.NewLogSinkRegistry()
		native.SetHostLogSinks(parent, lsr)
	}
	logNatives := native.LogModuleNativeFuncs(lsr)
	for _, n := range logNatives {
		subReg.RegisterNativeFunc(n)
	}

	exports := native.NewOrderedMap()
	// Export names are the dotted suffixes users type; the inner native
	// names carry a `log-` prefix to stay clear of any same-named core
	// word in the sub-registry.
	exportName := map[string]string{
		"log-trace":        "trace",
		"log-debug":        "debug",
		"log-info":         "info",
		"log-warn":         "warn",
		"log-error":        "error",
		"log-fatal":        "fatal",
		"log-log":          "log",
		"log-set-level":    "set-level",
		"log-get-level":    "get-level",
		"log-enabled":      "enabled",
		"log-set-format":   "set-format",
		"log-get-format":   "get-format",
		"log-add-sink":     "add-sink",
		"log-remove-sink":  "remove-sink",
		"log-sinks":        "sinks",
		"log-dump":         "dump",
		"log-clear":        "clear",
		"log-logger":       "logger",
		"log-with":         "with",
		"log-register":     "register",
		"log-span":         "span",
		"log-with-span":    "with-span",
		"log-end":          "end-span",
		"log-current-span": "current-span",
		"log-traces":       "traces",
		"log-counter":      "counter",
		"log-gauge":        "gauge",
		"log-histogram":    "histogram",
		"log-measurements": "measurements",
	}
	for _, n := range logNatives {
		exports.Set(exportName[n.Name], makeModuleFnDef(n, subReg))
	}

	modID := parent.Modules.NextID()
	return native.ModuleDesc{
		Src:     subReg,
		ID:      modID,
		Exports: map[string]*native.OrderedMap{"Log": exports},
	}, nil
}
