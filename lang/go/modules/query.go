package modules

import (
	"github.com/aql-lang/aql/lang/go/native"
)

// BuildQueryModule creates the "aql:query" native module. It registers
// the Go-implemented SQL-style query DSL words into an isolated
// sub-registry and returns a ModuleDesc with a "query" export containing
// FnDef wrappers for each word.
//
// After import, words are accessed via dot notation:
//
//	"aql:query" import
//	people query.from
//	  query.where [age gt 18]
//	  query.order [age desc]
//	  query.select [name age]
//
// Tables are resolved by name from the context store (set via
// `context set <name> <table-value>`), so the source word `query.from`
// takes a bare table name; every later word takes the running query off
// the stack and its own clause as a forward argument. The terminal
// `query.select` materializes the accumulated query into a concrete
// table.
func BuildQueryModule(parent *native.Registry) (native.ModuleDesc, error) {
	// Create an isolated sub-registry for the module's Go words.
	subReg, err := native.DefaultRegistry()
	if err != nil {
		return native.ModuleDesc{}, err
	}

	// Register the query words into the sub-registry. They are
	// deliberately absent from the global registry — query is only
	// reachable through `import "aql:query"`.
	for _, n := range native.QueryNatives {
		subReg.RegisterNativeFunc(n)
	}

	exports := native.NewOrderedMap()
	for _, w := range queryExports {
		exports.Set(w.export, makeQueryFnDef(w, subReg))
	}

	modID := parent.Modules.NextID()
	desc := native.ModuleDesc{
		ID:      modID,
		Exports: map[string]*native.OrderedMap{"query": exports},
	}
	return desc, nil
}

// qParam describes one parameter of a query word in sig order
// (top-of-stack first). quote marks a bare-name atom position that must
// be captured without evaluation (mirrors the inner native's
// QuoteArgs); noEval marks a clause-list position whose contents must
// not be auto-evaluated (mirrors the inner native's NoEvalArgs).
type qParam struct {
	typ    *native.Type
	quote  bool
	noEval bool
}

// qWord maps a module export name to the internal query native it
// delegates to, with the parameter list for its (single) signature.
type qWord struct {
	export   string
	internal string
	params   []qParam
}

// queryExports is the export table for aql:query. Each export name is
// the namespaced word (query.<export>); internal is the underlying
// native word registered in the sub-registry (identical here).
//
// Parameter order is sig order (top-of-stack first): the forward clause
// argument sits at position 0 and the upstream query builder at
// position 1, matching each handler's args indexing. The single-source
// word `from` and the flag word `distinct` take one argument.
var queryExports = []qWord{
	{export: "from", internal: "from", params: []qParam{{typ: native.TAtom, quote: true}}},
	{export: "where", internal: "where", params: []qParam{{typ: native.TList, noEval: true}, {typ: native.TList}}},
	{export: "select", internal: "select", params: []qParam{{typ: native.TList, noEval: true}, {typ: native.TList}}},
	{export: "order", internal: "order", params: []qParam{{typ: native.TList, noEval: true}, {typ: native.TList}}},
	{export: "group", internal: "group", params: []qParam{{typ: native.TList, noEval: true}, {typ: native.TList}}},
	{export: "having", internal: "having", params: []qParam{{typ: native.TList, noEval: true}, {typ: native.TList}}},
	{export: "limit", internal: "limit", params: []qParam{{typ: native.TInteger}, {typ: native.TList}}},
	{export: "offset", internal: "offset", params: []qParam{{typ: native.TInteger}, {typ: native.TList}}},
	{export: "distinct", internal: "distinct", params: []qParam{{typ: native.TList}}},
	{export: "join", internal: "join", params: []qParam{{typ: native.TAtom, quote: true}, {typ: native.TList}}},
	{export: "innerjoin", internal: "innerjoin", params: []qParam{{typ: native.TAtom, quote: true}, {typ: native.TList}}},
	{export: "leftjoin", internal: "leftjoin", params: []qParam{{typ: native.TAtom, quote: true}, {typ: native.TList}}},
	{export: "crossjoin", internal: "crossjoin", params: []qParam{{typ: native.TAtom, quote: true}, {typ: native.TList}}},
	{export: "on", internal: "on", params: []qParam{{typ: native.TList, noEval: true}, {typ: native.TList}}},
	{export: "using", internal: "using", params: []qParam{{typ: native.TList, noEval: true}, {typ: native.TList}}},
	{export: "union", internal: "union", params: []qParam{{typ: native.TList}, {typ: native.TList}}},
	{export: "unionall", internal: "unionall", params: []qParam{{typ: native.TList}, {typ: native.TList}}},
	{export: "intersect", internal: "intersect", params: []qParam{{typ: native.TList}, {typ: native.TList}}},
	{export: "except", internal: "except", params: []qParam{{typ: native.TList}, {typ: native.TList}}},
}

// makeQueryFnDef builds the trivial-delegation FnDef wrapper for one
// query word. The wrapper body is a single Word naming the inner native,
// so execFnDefLiteral short-circuits to direct dispatch on the inner
// native's matched sig (carrying its own QuoteArgs/NoEvalArgs). The
// wrapper's NoEvalArgs still suppresses auto-evaluation of clause lists
// during the wrapper's own forward collection.
func makeQueryFnDef(w qWord, subReg *native.Registry) native.Value {
	params := make([]native.FnParam, len(w.params))
	var noEvalMap map[int]bool
	for i, p := range w.params {
		params[i] = native.FnParam{Type: p.typ}
		if p.noEval {
			if noEvalMap == nil {
				noEvalMap = make(map[int]bool)
			}
			noEvalMap[i] = true
		}
	}
	return native.NewFnDef(native.FnDefInfo{
		Name: w.internal,
		Sigs: []native.FnSig{{
			Params:     params,
			Returns:    []*native.Type{native.TList},
			Body:       []native.Value{native.NewWord(w.internal)},
			NoEvalArgs: noEvalMap,
			BarrierPos: -1,
		}},
		Registry: subReg,
	})
}

// InstallQueryExports builds the query module and installs its exports
// as defs in the given registry — the test-setup convenience equivalent
// to running `"aql:query" import`.
func InstallQueryExports(r *native.Registry) error {
	desc, err := BuildQueryModule(r)
	if err != nil {
		return err
	}
	for name, exportMap := range desc.Exports {
		r.Defs.Push(name, native.NewMap(exportMap))
	}
	return nil
}
