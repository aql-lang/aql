package engine

import (
	"github.com/aql-lang/aql/eng/go"

	"github.com/aql-lang/aql/lang/go/internal/fileops"
)

// Host-side capability keys. The host installs implementations under
// these names on eng.Registry; word handlers retrieve them through
// the typed accessors in this file. aqleng itself never sees them.
const (
	CapFileOps    = "engine.fileops"     // active fileops.FileOps
	CapMemFileOps = "engine.fileops.mem" // lazily created in-memory fileops
	CapFormats    = "engine.formats"     // map[string]Format read/write registry
	CapSQLite     = "engine.sqlite"      // *SQLiteStore
)

// HostFileOps returns the FileOps installed on r, or nil if none.
// Word handlers that need filesystem access call EffectiveFileOps
// instead — it honours the __sys.fs.mem switch.
//
// The eng.Cap error (nil registry) is discarded: the wrappers are
// only ever called on initialised registries in practice, and the
// callers expect a single-value signature. A misconfigured registry
// surfaces at lower-level capability checks before reaching here.
func HostFileOps(r *Registry) fileops.FileOps {
	ops, _, _ := eng.Cap[fileops.FileOps](r, CapFileOps)
	return ops
}

// SetHostFileOps installs the active fileops capability and re-wires
// any registered jsonic-format multisource resolver to use it.
func SetHostFileOps(r *Registry, ops fileops.FileOps) {
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
func SetHostFormats(r *Registry, formats map[string]Format) {
	_ = r.Capabilities.Set(CapFormats, formats)
}

// HostSQLite returns the SQLite store installed on r, or nil if none.
func HostSQLite(r *Registry) *SQLiteStore {
	store, _, _ := eng.Cap[*SQLiteStore](r, CapSQLite)
	return store
}

// SetHostSQLite installs the SQLite store as a capability.
func SetHostSQLite(r *Registry, store *SQLiteStore) {
	_ = r.Capabilities.Set(CapSQLite, store)
}

// EffectiveFileOps returns the fileops to use for the current
// invocation. When __sys.fs.mem is set on the active context store the
// in-memory variant is returned (and cached as a capability on first
// use); otherwise the regular host fileops is returned.
//
// This logic used to live on eng.Registry; it now lives here
// because aqleng has no fileops concept.
func EffectiveFileOps(r *Registry) fileops.FileOps {
	if r == nil {
		return nil
	}
	store := r.Contexts.Top()
	if store == nil {
		return HostFileOps(r)
	}
	sysVal, ok := store.Get("__sys")
	if !ok {
		return HostFileOps(r)
	}
	sysStore, ok := sysVal.Data.(*StoreInstanceInfo)
	if !ok {
		return HostFileOps(r)
	}
	fsVal, ok := sysStore.Get("fs")
	if !ok {
		return HostFileOps(r)
	}
	fsStore, ok := fsVal.Data.(*StoreInstanceInfo)
	if !ok {
		return HostFileOps(r)
	}
	memVal, ok := fsStore.Get("mem")
	if !ok {
		return HostFileOps(r)
	}
	asBool, _ := AsBoolean(memVal)
	if memVal.VType.Matches(TBoolean) && asBool {
		if mem, _, _ := eng.Cap[fileops.FileOps](r, CapMemFileOps); mem != nil {
			return mem
		}
		mem := fileops.NewMem()
		_ = r.Capabilities.Set(CapMemFileOps, mem)
		return mem
	}
	return HostFileOps(r)
}
