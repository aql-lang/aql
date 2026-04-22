package native

import (
	"fmt"

	"github.com/metsitaba/voxgig-exp/aql/internal/engine"
)

// listFunc returns the "list" native function definition.
// list has forward precedence and four signatures:
//   - [table, map]  — returns records whose fields match the map's key-value pairs
//   - [table]       — returns all records from the table
//   - [map, map]    — record type + filter: returns empty table
//   - [map]         — record type: returns empty table
func RegisterList(r *engine.Registry) {
	// Pattern for {kind:"api", ...} — matches maps where kind is literal "api".
	apiPattern := engine.NewOrderedMap()
	apiPattern.Set("kind", engine.NewString("api"))
	apiPatternVal := engine.NewMap(apiPattern)

	r.RegisterNativeFunc(engine.NativeFunc{
		Name:             "list",
		ForwardPrecedence: true,
		Signatures: []engine.NativeSig{
			// Entity object signatures (highest priority).
			{
				Args:    []engine.Type{engine.TResourceEntity, engine.TMap},
				Handler: listEntityOptsHandler,
			},
			{
				Args:    []engine.Type{engine.TResourceEntity},
				Handler: listEntityHandler,
			},
			{
				Args:     []engine.Type{engine.TMap, engine.TMap},
				Handler:  listAPIOptsHandler,
				Patterns: map[int]engine.Value{0: apiPatternVal},
			},
			{
				Args:     []engine.Type{engine.TMap},
				Handler:  listAPIHandler,
				Patterns: map[int]engine.Value{0: apiPatternVal},
			},
			{
				Args:    []engine.Type{engine.TList, engine.TMap},
				Handler: listFilterHandler,
			},
			{
				Args:    []engine.Type{engine.TList},
				Handler: listAllHandler,
			},
			{
				Args:    []engine.Type{engine.TMap, engine.TMap},
				Handler: listRecordFilterHandler,
			},
			{
				Args:    []engine.Type{engine.TMap},
				Handler: listRecordAllHandler,
			},
		},
	})
}

// listEntityHandler handles list with an Entity object instance.
func listEntityHandler(args []engine.Value, ctx map[string]engine.Value, stack []engine.Value, r *engine.Registry) ([]engine.Value, error) {
	apiMap := entityToAPIMap(args[0])
	return listAPIHandler([]engine.Value{engine.NewMap(apiMap)}, ctx, stack, r)
}

// listEntityOptsHandler handles list with an Entity object instance and an options map.
func listEntityOptsHandler(args []engine.Value, ctx map[string]engine.Value, stack []engine.Value, r *engine.Registry) ([]engine.Value, error) {
	merged := entityToAPIMapWithOpts(args[0], args[1].AsMap(), "query")
	return listAPIHandler([]engine.Value{engine.NewMap(merged)}, ctx, stack, r)
}

// listAPIOptsHandler handles list with {kind:"api",...} and an extra options map.
// The options map is merged into the query field of the API map.
func listAPIOptsHandler(args []engine.Value, ctx map[string]engine.Value, stack []engine.Value, r *engine.Registry) ([]engine.Value, error) {
	merged := mergeAPIOptions(args[0].AsMap(), args[1].AsMap(), "query")
	return listAPIHandler([]engine.Value{engine.NewMap(merged)}, ctx, stack, r)
}

// listAPIHandler handles list with {kind:"api", spec:String, entity:String}.
// It uses the UniversalManager to create/cache an SDK, then calls entity.List().
// An optional query field ({query:{...}}) is passed as the reqmatch filter.
func listAPIHandler(args []engine.Value, ctx map[string]engine.Value, stack []engine.Value, r *engine.Registry) ([]engine.Value, error) {
	apiMap := args[0].AsMap()

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
	rows := args[0].AsList().Slice()
	result := make([]engine.Value, len(rows))
	copy(result, rows)
	return []engine.Value{engine.NewList(result)}, nil
}

// listFilterHandler returns records from a table that match the given map.
// A record matches when every key-value pair in the filter map has an equal
// value in the corresponding record field.
func listFilterHandler(args []engine.Value, ctx map[string]engine.Value, stack []engine.Value, r *engine.Registry) ([]engine.Value, error) {
	rows := args[0].AsList().Slice()
	filter := args[1].AsMap()

	var matched []engine.Value
	for _, row := range rows {
		if !row.VType.Matches(engine.TMap) {
			continue
		}
		rec := row.AsMap()
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
		ai, _ := a.AsInteger()
		bi, _ := b.AsInteger()
		return ai == bi
	case a.VType.Matches(engine.TString) && b.VType.Matches(engine.TString):
		as, _ := a.AsString()
		bs, _ := b.AsString()
		return as == bs
	case a.VType.Matches(engine.TBoolean) && b.VType.Matches(engine.TBoolean):
		ab, _ := a.AsBoolean()
		bb, _ := b.AsBoolean()
		return ab == bb
	case a.VType.Equal(engine.TAtom) && b.VType.Equal(engine.TAtom):
		aa, _ := a.AsAtom()
		ba, _ := b.AsAtom()
		return aa == ba
	// Cross-type: atom and string are interchangeable for equality.
	case a.VType.Equal(engine.TAtom) && b.VType.Matches(engine.TString):
		aa, _ := a.AsAtom()
		bs, _ := b.AsString()
		return aa == bs
	case a.VType.Matches(engine.TString) && b.VType.Equal(engine.TAtom):
		as, _ := a.AsString()
		ba, _ := b.AsAtom()
		return as == ba
	default:
		return a.String() == b.String()
	}
}
