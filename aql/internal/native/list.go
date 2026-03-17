package native

import (
	"fmt"

	"github.com/metsitaba/voxgig-exp/aql/internal/engine"
)

// listFunc returns the "list" native function definition.
// list has suffix precedence and four signatures:
//   - [table, map]  — returns records whose fields match the map's key-value pairs
//   - [table]       — returns all records from the table
//   - [map, map]    — record type + filter: returns empty table
//   - [map]         — record type: returns empty table
func listFunc() NativeFunc {
	// Pattern for {kind:"api", ...} — matches maps where kind is literal "api".
	apiPattern := engine.NewOrderedMap()
	apiPattern.Set("kind", engine.NewString("api"))
	apiPatternVal := engine.NewMap(apiPattern)

	return NativeFunc{
		Name:             "list",
		SuffixPrecedence: true,
		Signatures: []NativeSig{
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
	}
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
	rows := args[0].AsList()
	result := make([]engine.Value, len(rows))
	copy(result, rows)
	return []engine.Value{engine.NewList(result)}, nil
}

// listFilterHandler returns records from a table that match the given map.
// A record matches when every key-value pair in the filter map has an equal
// value in the corresponding record field.
func listFilterHandler(args []engine.Value, ctx map[string]engine.Value, stack []engine.Value, r *engine.Registry) ([]engine.Value, error) {
	rows := args[0].AsList()
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
func recordMatches(rec *engine.OrderedMap, filter *engine.OrderedMap) bool {
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
		return a.AsInteger() == b.AsInteger()
	case a.VType.Matches(engine.TString) && b.VType.Matches(engine.TString):
		return a.AsString() == b.AsString()
	case a.VType.Matches(engine.TBoolean) && b.VType.Matches(engine.TBoolean):
		return a.AsBoolean() == b.AsBoolean()
	case a.VType.Equal(engine.TAtom) && b.VType.Equal(engine.TAtom):
		return a.AsAtom() == b.AsAtom()
	// Cross-type: atom and string are interchangeable for equality.
	case a.VType.Equal(engine.TAtom) && b.VType.Matches(engine.TString):
		return a.AsAtom() == b.AsString()
	case a.VType.Matches(engine.TString) && b.VType.Equal(engine.TAtom):
		return a.AsString() == b.AsAtom()
	default:
		return a.String() == b.String()
	}
}
