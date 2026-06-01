package native

import (
	"fmt"
	"sort"
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

// TModuleInst is Ideal/Module — the module descriptor instance type. It is
// also the value `module […]` produces and that `import` consumes (it
// carries the full ModuleDesc), so there is no separate internal carrier
// type.
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

// moduleExportInfo is the payload behind an Ideal/ModuleExport value.
type moduleExportInfo struct {
	Name   string      // the export name, e.g. "Math"
	Fields *OrderedMap // exported word → value (the raw exports)
	Module Value       // the owning Module instance (Ideal/Module)
}

// NewModuleInstance builds an Ideal/Module value wrapping a ModuleDesc. The
// descriptor metadata (Ref/Kind/File/Folder) and the export names are
// surfaced as the id/kind/file/folder/exports fields; the carried Exports
// map is what `import` installs.
func NewModuleInstance(desc ModuleDesc) Value {
	return Value{Parent: TModuleInst, Data: ExtensionPayload{Body: desc}}
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

// asModuleDesc / asModuleExportInfo unwrap the ExtensionPayload.
func asModuleDesc(v Value) (ModuleDesc, bool) {
	if ep, ok := v.Data.(ExtensionPayload); ok {
		d, ok := ep.Body.(ModuleDesc)
		return d, ok
	}
	return ModuleDesc{}, false
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

// moduleKind returns a Module's kind, defaulting "" to "inline".
func moduleKind(desc ModuleDesc) string {
	if desc.Kind == "" {
		return "inline"
	}
	return desc.Kind
}

// moduleExportNames returns a Module's export names, sorted for determinism.
func moduleExportNames(desc ModuleDesc) []string {
	names := make([]string, 0, len(desc.Exports))
	for name := range desc.Exports {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// moduleGet resolves a key against a Module descriptor's normal fields.
func moduleGet(v Value, key string) (Value, bool) {
	desc, ok := asModuleDesc(v)
	if !ok {
		return Value{}, false
	}
	switch key {
	case "id":
		return NewString(desc.Ref), true
	case "kind":
		return NewString(moduleKind(desc)), true
	case "file":
		return NewString(desc.File), true
	case "folder":
		return NewString(desc.Folder), true
	case "exports":
		names := moduleExportNames(desc)
		elems := make([]Value, len(names))
		for i, e := range names {
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
	if desc, ok := asModuleDesc(v); ok {
		ref := desc.Ref
		if ref == "" {
			ref = moduleKind(desc)
		}
		return fmt.Sprintf("Module(%s)", ref)
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

// AsModuleDesc unwraps the ModuleDesc carried by an Ideal/Module value
// (what `module […]` produces and `import` consumes). Exported for hosts
// and integration tests; ok=false for non-Module values.
func AsModuleDesc(v Value) (ModuleDesc, bool) { return asModuleDesc(v) }

// ToMap / ToList implement eng.IdealConverter for Module / ModuleExport.
// A ModuleExport projects to its exported fields; a Module projects to its
// descriptor (id/kind/file/folder/exports).
func (moduleTypeBehavior) ToMap(v Value) (Value, error) {
	if me, ok := asModuleExportInfo(v); ok {
		out := NewOrderedMap()
		if me.Fields != nil {
			for _, k := range me.Fields.Keys() {
				val, _ := me.Fields.Get(k)
				out.Set(k, val)
			}
		}
		return NewMap(out), nil
	}
	if desc, ok := asModuleDesc(v); ok {
		out := NewOrderedMap()
		out.Set("id", NewString(desc.Ref))
		out.Set("kind", NewString(moduleKind(desc)))
		out.Set("file", NewString(desc.File))
		out.Set("folder", NewString(desc.Folder))
		names := moduleExportNames(desc)
		elems := make([]Value, len(names))
		for i, n := range names {
			elems[i] = NewString(n)
		}
		out.Set("exports", NewList(elems))
		return NewMap(out), nil
	}
	return NewMap(NewOrderedMap()), nil
}

func (moduleTypeBehavior) ToList(v Value) (Value, error) {
	if me, ok := asModuleExportInfo(v); ok {
		var vals []Value
		if me.Fields != nil {
			for _, k := range me.Fields.Keys() {
				val, _ := me.Fields.Get(k)
				vals = append(vals, val)
			}
		}
		return NewList(vals), nil
	}
	if desc, ok := asModuleDesc(v); ok {
		names := moduleExportNames(desc)
		elems := make([]Value, len(names))
		for i, n := range names {
			elems[i] = NewString(n)
		}
		return NewList(elems), nil
	}
	return NewList(nil), nil
}
