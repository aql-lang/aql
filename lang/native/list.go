package native

import (
	"fmt"

	"github.com/aql-lang/aql/lang/engine"
)

// The "list" word is registered via the consolidated Natives slice in
// natives.go. This file keeps the SDK/table/record-type handlers along
// with shared helpers (recordMatches, valuesEqual) used by load/update/
// remove.
//
// listEntityHandler handles list with an Entity object instance.
func listEntityHandler(args []engine.Value, ctx map[string]engine.Value, stack []engine.Value, r *engine.Registry) ([]engine.Value, error) {
	apiMap := entityToAPIMap(args[0])
	return listAPIHandler([]engine.Value{engine.NewMap(apiMap)}, ctx, stack, r)
}

// listEntityOptsHandler handles list with an Entity object instance and an options map.
// Sig is opts-first per the data-last principle: args[0]=opts, args[1]=entity.
func listEntityOptsHandler(args []engine.Value, ctx map[string]engine.Value, stack []engine.Value, r *engine.Registry) ([]engine.Value, error) {
	merged := entityToAPIMapWithOpts(args[1], engine.AsMap(args[0]), "query")
	return listAPIHandler([]engine.Value{engine.NewMap(merged)}, ctx, stack, r)
}

// listAPIOptsHandler handles list with {kind:"api",...} and an extra options map.
// The options map is merged into the query field of the API map.
// Sig is opts-first: args[0]=opts, args[1]=apiMap (pattern-matched).
func listAPIOptsHandler(args []engine.Value, ctx map[string]engine.Value, stack []engine.Value, r *engine.Registry) ([]engine.Value, error) {
	merged := mergeAPIOptions(engine.AsMap(args[1]), engine.AsMap(args[0]), "query")
	return listAPIHandler([]engine.Value{engine.NewMap(merged)}, ctx, stack, r)
}

// listAPIHandler handles list with {kind:"api", spec:String, entity:String}.
// It uses the UniversalManager to create/cache an SDK, then calls entity.List().
// An optional query field ({query:{...}}) is passed as the reqmatch filter.
func listAPIHandler(args []engine.Value, ctx map[string]engine.Value, stack []engine.Value, r *engine.Registry) ([]engine.Value, error) {
	apiMap := engine.AsMap(args[0])

	sdkInst, entityName, err := getSDK(apiMap, "list", r)
	if err != nil {
		return nil, err
	}

	ent := sdkInst.Entity(entityName, nil)
	result, err := ent.List(extractQuery(apiMap), nil)
	if err != nil {
		return nil, fmt.Errorf("list: entity %q: %w", entityName, err)
	}

	items, _ := result.([]any)
	rows, err := convertResultList(items, "list")
	if err != nil {
		return nil, err
	}

	return []engine.Value{engine.NewList(rows)}, nil
}

// listAllHandler returns all records from a table as a list.
func listAllHandler(args []engine.Value, ctx map[string]engine.Value, stack []engine.Value, r *engine.Registry) ([]engine.Value, error) {
	rows := engine.AsList(args[0]).Slice()
	result := make([]engine.Value, len(rows))
	copy(result, rows)
	return []engine.Value{engine.NewList(result)}, nil
}

// listFilterHandler returns records from a table that match the given map.
// A record matches when every key-value pair in the filter map has an equal
// value in the corresponding record field. Sig is opts-first: args[0]=filter,
// args[1]=list.
func listFilterHandler(args []engine.Value, ctx map[string]engine.Value, stack []engine.Value, r *engine.Registry) ([]engine.Value, error) {
	rows := engine.AsList(args[1]).Slice()
	filter := engine.AsMap(args[0])

	var matched []engine.Value
	for _, row := range rows {
		if !row.VType.Matches(engine.TMap) {
			continue
		}
		rec := engine.AsMap(row)
		if recordMatches(rec, filter) {
			matched = append(matched, row)
		}
	}

	if matched == nil {
		matched = []engine.Value{}
	}
	return []engine.Value{engine.NewList(matched)}, nil
}

// listRecordAllHandler handles list on a record type (not a table).
// Returns an empty table.
func listRecordAllHandler(args []engine.Value, ctx map[string]engine.Value, stack []engine.Value, r *engine.Registry) ([]engine.Value, error) {
	return []engine.Value{engine.NewList([]engine.Value{})}, nil
}

// listRecordFilterHandler handles list on a record type with a filter.
// Returns an empty table.
func listRecordFilterHandler(args []engine.Value, ctx map[string]engine.Value, stack []engine.Value, r *engine.Registry) ([]engine.Value, error) {
	return []engine.Value{engine.NewList([]engine.Value{})}, nil
}

// recordMatches reports whether all key-value pairs in filter are present
// in rec with equal values. Equality is checked by comparing Value.String()
// representations for scalar types and structural equality for others.
func recordMatches(rec engine.ReadMap, filter engine.ReadMap) bool {
	for _, key := range filter.Keys() {
		filterVal, _ := filter.Get(key)
		recVal, ok := rec.Get(key)
		if !ok {
			return false
		}
		if !valuesEqual(filterVal, recVal) {
			return false
		}
	}
	return true
}

// valuesEqual checks equality between two values using type-aware comparison.
func valuesEqual(a, b engine.Value) bool {
	switch {
	case a.VType.Matches(engine.TInteger) && b.VType.Matches(engine.TInteger):
		ai, _ := engine.AsInteger(a)
		bi, _ := engine.AsInteger(b)
		return ai == bi
	case a.VType.Matches(engine.TString) && b.VType.Matches(engine.TString):
		as, _ := engine.AsString(a)
		bs, _ := engine.AsString(b)
		return as == bs
	case a.VType.Matches(engine.TBoolean) && b.VType.Matches(engine.TBoolean):
		ab, _ := engine.AsBoolean(a)
		bb, _ := engine.AsBoolean(b)
		return ab == bb
	case a.VType.Equal(engine.TAtom) && b.VType.Equal(engine.TAtom):
		aa, _ := engine.AsAtom(a)
		ba, _ := engine.AsAtom(b)
		return aa == ba
	// Cross-type: atom and string are interchangeable for equality.
	case a.VType.Equal(engine.TAtom) && b.VType.Matches(engine.TString):
		aa, _ := engine.AsAtom(a)
		bs, _ := engine.AsString(b)
		return aa == bs
	case a.VType.Matches(engine.TString) && b.VType.Equal(engine.TAtom):
		as, _ := engine.AsString(a)
		ba, _ := engine.AsAtom(b)
		return as == ba
	default:
		return a.String() == b.String()
	}
}
