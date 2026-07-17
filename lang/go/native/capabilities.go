package native

import (
	"github.com/aql-lang/aql/eng/go"

	"github.com/aql-lang/aql/lang/go/capabilities"
	"github.com/aql-lang/aql/lang/go/policy"
)

// Host-side capability keys. The host installs implementations under
// these names on eng.Registry; word handlers retrieve them through
// the typed accessors in this file. aqleng itself never sees them.
const (
	CapFileOps        = "engine.fileops"         // active capabilities.FileOps
	CapMemFileOps     = "engine.fileops.mem"     // lazily created in-memory FileOps
	CapOverlayFileOps = "engine.fileops.overlay" // lazily created mem-over-host overlay
	CapFormats        = "engine.formats"         // map[string]Format read/write registry
	CapExtensions     = "engine.extensions"      // map[string]string file-ext→format-name
	CapSQLite         = "engine.sqlite"          // *SQLiteStore
	CapPolicy         = "engine.policy"          // policy.Policy enforcing permissions
	CapClock          = "engine.clock"           // capabilities.Clock (the time source)
	CapLogSinks       = "engine.logsinks"        // *LogSinkRegistry (aql:log fan-out sinks)
	CapDebugOps       = "engine.debugops"        // capabilities.DebugOps (interactive stepping)
)

// EffectiveDebugOps returns the installed DebugOps capability, or (nil,
// false) when none is installed. Debug.step uses it for an interactive
// step controller; with no DebugOps it falls back to printing the trace.
func EffectiveDebugOps(r *Registry) (capabilities.DebugOps, bool) {
	if ops, ok, _ := eng.Cap[capabilities.DebugOps](r, CapDebugOps); ok && ops != nil {
		return ops, true
	}
	return nil, false
}

// SetHostDebugOps installs a DebugOps capability (used by the REPL/TTY
// host and by tests supplying a scripted controller).
func SetHostDebugOps(r *Registry, ops capabilities.DebugOps) {
	if r == nil || ops == nil {
		return
	}
	_ = r.Capabilities.Set(CapDebugOps, ops)
}

// EffectiveClock returns the time source for the current invocation. The
// host (or the spec runner) may install a Clock under CapClock — e.g. a
// capabilities.FixedClock for deterministic tests. When none is installed,
// a real wall clock is used. Never returns nil.
func EffectiveClock(r *Registry) capabilities.Clock {
	if clk, ok, _ := eng.Cap[capabilities.Clock](r, CapClock); ok && clk != nil {
		return clk
	}
	return capabilities.WallClock{}
}

// SetHostClock installs a Clock capability (used by the spec runner and
// host embedders to freeze time).
func SetHostClock(r *Registry, clk capabilities.Clock) {
	if r == nil {
		return
	}
	_ = r.Capabilities.Set(CapClock, clk)
}

// HostLogSinks returns the LogSinkRegistry installed on r, or nil if
// none has been created yet. Most callers want LogSinkRegistryFor,
// which lazily creates and installs a default registry (console sink
// attached) on first use — mirroring how `print` needs no host wiring.
func HostLogSinks(r *Registry) *LogSinkRegistry {
	lsr, _, _ := eng.Cap[*LogSinkRegistry](r, CapLogSinks)
	return lsr
}

// SetHostLogSinks installs a LogSinkRegistry as the active sink
// registry. When a policy uninstalls the "log" scope (install:false)
// the slot is left empty so emission is a silent no-op — logging must
// never crash a program. Hosts call this to pre-attach an OTel/Datadog
// sink before the AQL program runs `import "aql:log"`.
func SetHostLogSinks(r *Registry, lsr *LogSinkRegistry) {
	if r == nil {
		return
	}
	if pol := HostPolicy(r); pol != nil && !pol.Installed("log") {
		_, _ = r.Capabilities.Delete(CapLogSinks)
		return
	}
	_ = r.Capabilities.Set(CapLogSinks, lsr)
}

// HostPolicy returns the policy installed on r, or nil if none. A
// nil result means "no permissions configured" — the engine and
// every capability wrapper treat that as allow-everything (the
// default opt-in posture).
func HostPolicy(r *Registry) policy.Policy {
	p, _, _ := eng.Cap[policy.Policy](r, CapPolicy)
	return p
}

// SetHostPolicy installs a Policy as a capability. Must be called
// before SetHostX hooks if those hooks should auto-wrap the
// capability with the policy. RewrapCapabilities can re-apply
// wrapping after a late SetHostPolicy.
func SetHostPolicy(r *Registry, p policy.Policy) {
	if p == nil {
		return
	}
	_ = r.Capabilities.Set(CapPolicy, p)
}

// HostFileOps returns the FileOps installed on r. When a policy has
// uninstalled the fileops capability (via install:false), the slot
// is empty — in that case HostFileOps returns notInstalledFileOps,
// a stub whose ReadFile/WriteFile/MkdirAll return a
// capability_not_installed error rather than a nil deref. Word
// handlers that need filesystem access can call without a nil
// guard and surface the error to the user.
//
// Returns nil only when r itself is nil (a misconfigured registry).
func HostFileOps(r *Registry) capabilities.FileOps {
	if r == nil {
		return nil
	}
	ops, ok, _ := eng.Cap[capabilities.FileOps](r, CapFileOps)
	if !ok || ops == nil {
		return notInstalledFileOps{}
	}
	return ops
}

// SetHostFileOps installs the active fileops capability and re-wires
// any registered jsonic-format multisource resolver to use it.
//
// When a policy is present on r and its fileops scope has
// install=false, the capability slot is left empty so word handlers
// that try to reach it produce capability_not_installed errors.
// When a policy is present without install=false, the FileOps is
// wrapped with permissionedFileOps so each call is gated.
func SetHostFileOps(r *Registry, ops capabilities.FileOps) {
	if pol := HostPolicy(r); pol != nil {
		if !pol.Installed("fileops") {
			// Uninstall: remove the slot so HostFileOps returns nil.
			_, _ = r.Capabilities.Delete(CapFileOps)
			return
		}
		ops = NewPermissionedFileOps(ops, pol)
	}
	_ = r.Capabilities.Set(CapFileOps, ops)
	if formats := HostFormats(r); formats != nil {
		if jf, ok := formats["jsonic"].(*JsonicFormat); ok {
			jf.Resolver = MakeFileOpsResolver(ops)
		}
	}
}

// HostFormats returns the format registry installed on r, or nil if
// none. The map is owned by the host and may be mutated in place to
// register or replace individual formats.
func HostFormats(r *Registry) map[string]Format {
	formats, _, _ := eng.Cap[map[string]Format](r, CapFormats)
	return formats
}

// SetHostFormats installs the format registry as a single capability.
// When a policy has formats.install=false, the slot is removed so
// HostFormats returns nil and read/write handlers raise
// capability_not_installed.
func SetHostFormats(r *Registry, formats map[string]Format) {
	if pol := HostPolicy(r); pol != nil && !pol.Installed("formats") {
		_, _ = r.Capabilities.Delete(CapFormats)
		return
	}
	_ = r.Capabilities.Set(CapFormats, formats)
}

// HostExtensions returns the file-extension→format-name map installed on
// r, or nil if none. The map is owned by the host and may be mutated in
// place (e.g. by RegisterFormat) to add or override extension mappings.
// Keys are lowercase, without the leading dot.
func HostExtensions(r *Registry) map[string]string {
	exts, _, _ := eng.Cap[map[string]string](r, CapExtensions)
	return exts
}

// SetHostExtensions installs the extension→format map. It shares the
// `formats` policy scope with SetHostFormats: when formats.install=false
// the slot is removed so read falls back to text for every extension.
func SetHostExtensions(r *Registry, exts map[string]string) {
	if pol := HostPolicy(r); pol != nil && !pol.Installed("formats") {
		_, _ = r.Capabilities.Delete(CapExtensions)
		return
	}
	_ = r.Capabilities.Set(CapExtensions, exts)
}

// HostSQLite returns the SQLite store installed on r, or nil if none.
func HostSQLite(r *Registry) *SQLiteStore {
	store, _, _ := eng.Cap[*SQLiteStore](r, CapSQLite)
	return store
}

// SetHostSQLite installs the SQLite store as a capability. When a
// policy has sqlite.install=false, the slot is removed so HostSQLite
// returns nil and sqlite-* word handlers raise capability_not_installed.
func SetHostSQLite(r *Registry, store *SQLiteStore) {
	if pol := HostPolicy(r); pol != nil && !pol.Installed("sqlite") {
		_, _ = r.Capabilities.Delete(CapSQLite)
		return
	}
	_ = r.Capabilities.Set(CapSQLite, store)
}

// EffectiveFileOps returns the fileops to use for the current
// invocation, switched by flags on the active context store's __sys.fs:
//
//   - {mem: true} — a fully in-memory FileOps (hermetic; nothing
//     touches the host filesystem);
//   - {overlay: true} — a UNION of a writable in-memory upper over the
//     host fileops as a read-only lower (capabilities.OverlayFileOps):
//     reads fall through to real files, every mutation lands in memory,
//     and deletes are whiteouts — the partially-in-memory configuration
//     unit tests use to exercise real fixtures without mutating them.
//     {mem: true} wins when both flags are set (it is the stricter
//     isolation).
//
// Each variant is cached as a capability on first use so state persists
// across invocations; otherwise the regular host fileops is returned.
//
// Never returns nil: if the policy has uninstalled the fileops
// capability, HostFileOps returns notInstalledFileOps so consumers
// can call methods without a nil guard and get a clean error.
// Returns nil only when r itself is nil (a misconfigured registry).
//
// This logic used to live on eng.Registry; it now lives here
// because aqleng has no fileops concept.
func EffectiveFileOps(r *Registry) capabilities.FileOps {
	if r == nil {
		return nil
	}
	fsStore := sysFsStore(r)
	if fsStore == nil {
		return HostFileOps(r)
	}
	if fsFlag(fsStore, "mem") {
		if mem, _, _ := eng.Cap[capabilities.FileOps](r, CapMemFileOps); mem != nil {
			return mem
		}
		mem := capabilities.NewMem()
		_ = r.Capabilities.Set(CapMemFileOps, mem)
		return mem
	}
	if fsFlag(fsStore, "overlay") {
		if ov, _, _ := eng.Cap[capabilities.FileOps](r, CapOverlayFileOps); ov != nil {
			return ov
		}
		ov := capabilities.NewOverlay(capabilities.NewMem(), HostFileOps(r))
		_ = r.Capabilities.Set(CapOverlayFileOps, ov)
		return ov
	}
	return HostFileOps(r)
}

// sysFsStore walks the active context store to __sys.fs, or nil when any
// hop is absent or not a store.
func sysFsStore(r *Registry) *StoreInstanceInfo {
	store := r.Contexts.Top()
	if store == nil {
		return nil
	}
	sysVal, ok := store.Get("__sys")
	if !ok {
		return nil
	}
	sysStore, ok := sysVal.Data.(*StoreInstanceInfo)
	if !ok {
		return nil
	}
	fsVal, ok := sysStore.Get("fs")
	if !ok {
		return nil
	}
	fsStore, ok := fsVal.Data.(*StoreInstanceInfo)
	if !ok {
		return nil
	}
	return fsStore
}

// fsFlag reads one boolean toggle off the __sys.fs store.
func fsFlag(fsStore *StoreInstanceInfo, key string) bool {
	v, ok := fsStore.Get(key)
	if !ok {
		return false
	}
	asBool, _ := AsBoolean(v)
	return v.Parent.ConformsTo(TBoolean) && asBool
}
