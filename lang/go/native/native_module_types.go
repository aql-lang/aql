package native

import (
	"fmt"
	"strings"

	"github.com/aql-lang/aql/eng/go"
)

// Module / ModuleExport — the Ideal types that describe an imported module.
//
// When AQL code runs `import "aql:math"`, the bound name `Math` is a
// ModuleExport instance (one per `export "Name" {…}` declaration). A
// ModuleExport is transparent: `Math.sqrt` reads the raw exported value
// (so `Math.sqrt 16.0 → 4.0` still works), while the synthetic names
// `$module` and `$name` expose metadata:
//
//	Math.sqrt          → the exported sqrt function (raw, callable)
//	Math.$name         → 'Math'                (the export name)
//	Math.$module       → the Module instance   (Ideal/Module)
//	Math.$module.id    → 'aql:math'            (the module reference)
//
// A Module instance (Ideal/Module) is the descriptor shared by all of a
// module's ModuleExports via $module. Its normal fields are id, kind,
// file, folder, and exports (the list of export names).

// moduleExportFieldModule and …Name are the synthetic keys a ModuleExport
// answers in addition to its exported fields.
const (
	moduleExportFieldModule = "$module"
	moduleExportFieldName   = "$name"
)

// TModuleInst is Ideal/Module — the module descriptor instance type.
// (eng.TModule, alias-free Word/__MD, remains the internal carrier for the
// `module […]` → `import` plumbing; this is the user-facing descriptor.)
var TModuleInst = registerModuleType("Ideal/Module", 5000)

// TModuleExport is Ideal/ModuleExport — the per-export namespace instance
// type that `import` binds.
var TModuleExport = registerModuleType("Ideal/ModuleExport", 5001)

func registerModuleType(path string, fixedID int) *Type {
	t, err := eng.Builtin.RegisterExternalBuiltin(path, fixedID, moduleTypeBehavior{path: path})
	if err != nil {
		// lint:allow-panic — init-time builtin registration; see
		// registerTimerType in native_misc.go for rationale.
		panic(fmt.Sprintf("native: register %s: %v", path, err))
	}
	return t
}

// moduleInfo is the descriptor payload behind an Ideal/Module value.
type moduleInfo struct {
	ID      string   // module reference, e.g. "aql:math" / "./lib.aql"
	Kind    string   // "native" | "file" | "inline"
	File    string   // source file path ("" for native/inline)
	Folder  string   // source folder ("" for native/inline)
	Exports []string // export names declared by the module
}

// moduleExportInfo is the payload behind an Ideal/ModuleExport value.
type moduleExportInfo struct {
	Name   string      // the export name, e.g. "Math"
	Fields *OrderedMap // exported word → value (the raw exports)
	Module Value       // the owning Module instance (Ideal/Module)
}

// NewModuleInstance builds an Ideal/Module descriptor value.
func NewModuleInstance(info moduleInfo) Value {
	return Value{Parent: TModuleInst, Data: ExtensionPayload{Body: &info}}
}

// NewModuleExport builds an Ideal/ModuleExport value binding `name` to its
// exported fields and owning Module.
func NewModuleExport(name string, fields *OrderedMap, module Value) Value {
	if fields == nil {
		fields = NewOrderedMap()
	}
	return Value{Parent: TModuleExport, Data: ExtensionPayload{Body: &moduleExportInfo{
		Name: name, Fields: fields, Module: module,
	}}}
}

// asModuleInfo / asModuleExportInfo unwrap the ExtensionPayload.
func asModuleInfo(v Value) (*moduleInfo, bool) {
	if ep, ok := v.Data.(ExtensionPayload); ok {
		mi, ok := ep.Body.(*moduleInfo)
		return mi, ok
	}
	return nil, false
}

func asModuleExportInfo(v Value) (*moduleExportInfo, bool) {
	if ep, ok := v.Data.(ExtensionPayload); ok {
		me, ok := ep.Body.(*moduleExportInfo)
		return me, ok
	}
	return nil, false
}

// moduleExportGet resolves a key against a ModuleExport: the synthetic
// $module / $name, otherwise an exported field. ok=false when the key is
// neither synthetic nor an export.
func moduleExportGet(v Value, key string) (Value, bool) {
	me, ok := asModuleExportInfo(v)
	if !ok {
		return Value{}, false
	}
	switch key {
	case moduleExportFieldName:
		return NewString(me.Name), true
	case moduleExportFieldModule:
		return me.Module, true
	}
	if me.Fields == nil {
		return Value{}, false
	}
	return me.Fields.Get(key)
}

// moduleGet resolves a key against a Module descriptor's normal fields.
func moduleGet(v Value, key string) (Value, bool) {
	mi, ok := asModuleInfo(v)
	if !ok {
		return Value{}, false
	}
	switch key {
	case "id":
		return NewString(mi.ID), true
	case "kind":
		return NewString(mi.Kind), true
	case "file":
		return NewString(mi.File), true
	case "folder":
		return NewString(mi.Folder), true
	case "exports":
		elems := make([]Value, len(mi.Exports))
		for i, e := range mi.Exports {
			elems[i] = NewString(e)
		}
		return NewList(elems), true
	}
	return Value{}, false
}

// moduleTypeBehavior renders Module / ModuleExport values and matches
// nominally (DefaultBehavior semantics for Match/Equal).
type moduleTypeBehavior struct {
	path string
}

func (moduleTypeBehavior) Match(v Value, t *Type) bool { return eng.DefaultBehavior.Match(v, t) }
func (moduleTypeBehavior) Equal(a, b Value) bool       { return eng.DefaultBehavior.Equal(a, b) }

func (b moduleTypeBehavior) Format(v Value) string {
	if me, ok := asModuleExportInfo(v); ok {
		var keys []string
		if me.Fields != nil {
			keys = me.Fields.Keys()
		}
		return fmt.Sprintf("ModuleExport(%s){%s}", me.Name, strings.Join(keys, " "))
	}
	if mi, ok := asModuleInfo(v); ok {
		return fmt.Sprintf("Module(%s)", mi.ID)
	}
	return b.path
}

// ---- get / getr handlers for Module and ModuleExport ----

func getModuleExportHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	if !IsConcrete(args[1]) {
		return nil, r.AqlError("get_error", "get: cannot access property on type literal", "get")
	}
	if val, ok := moduleExportGet(args[1], getKey(args[0])); ok {
		return []Value{val}, nil
	}
	return []Value{NewTypeLiteral(TNone)}, nil
}

func getrModuleExportHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	if !IsConcrete(args[1]) {
		return nil, r.AqlError("getr_error", "getr: cannot access property on type literal", "getr")
	}
	k := getKey(args[0])
	if val, ok := moduleExportGet(args[1], k); ok {
		return []Value{val}, nil
	}
	return nil, r.AqlError("getr_error", fmt.Sprintf("getr: export %q not found in module", k), "getr")
}

func getModuleInstHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	if !IsConcrete(args[1]) {
		return nil, r.AqlError("get_error", "get: cannot access property on type literal", "get")
	}
	if val, ok := moduleGet(args[1], getKey(args[0])); ok {
		return []Value{val}, nil
	}
	return []Value{NewTypeLiteral(TNone)}, nil
}

func getrModuleInstHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	if !IsConcrete(args[1]) {
		return nil, r.AqlError("getr_error", "getr: cannot access property on type literal", "getr")
	}
	k := getKey(args[0])
	if val, ok := moduleGet(args[1], k); ok {
		return []Value{val}, nil
	}
	return nil, r.AqlError("getr_error", fmt.Sprintf("getr: field %q not found in Module", k), "getr")
}

// NewModuleInstance2 is an exported constructor (string fields) for hosts
// and tests; mirrors NewModuleInstance with an unexported moduleInfo.
func NewModuleInstance2(id, kind, file, folder string, exports []string) Value {
	return NewModuleInstance(moduleInfo{ID: id, Kind: kind, File: file, Folder: folder, Exports: exports})
}
