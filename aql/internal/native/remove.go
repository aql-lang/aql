package native

import (
	"fmt"

	"github.com/metsitaba/voxgig-exp/aql/internal/engine"
)

// removeFunc returns the "remove" native function definition.
// remove has suffix precedence and two signatures:
//   - [table, map] — removes the record whose "id" matches the map's "id" field
//   - [map, map]   — record type + filter: returns empty table
func removeFunc() NativeFunc {
	return NativeFunc{
		Name:             "remove",
		SuffixPrecedence: true,
		Signatures: []NativeSig{
			{
				Args:    []engine.Type{engine.TList, engine.TMap},
				Handler: removeHandler,
			},
			{
				Args:    []engine.Type{engine.TMap, engine.TMap},
				Handler: removeRecordHandler,
			},
		},
	}
}

// removeRecordHandler handles remove on a record type (not a table).
// Returns an empty table.
func removeRecordHandler(args []engine.Value, ctx map[string]engine.Value, stack []engine.Value, r *engine.Registry) ([]engine.Value, error) {
	return []engine.Value{engine.NewList([]engine.Value{})}, nil
}

// removeHandler finds a record by its "id" field and removes it from the table.
// Returns the updated table. The map must contain an "id" field.
func removeHandler(args []engine.Value, ctx map[string]engine.Value, stack []engine.Value, r *engine.Registry) ([]engine.Value, error) {
	rows := args[0].AsList()
	filter := args[1].AsMap()

	idVal, ok := filter.Get("id")
	if !ok {
		return nil, fmt.Errorf("remove: filter must contain an \"id\" field")
	}
	id := idVal.AsString()

	found := false
	var result []engine.Value
	for _, row := range rows {
		if !row.VType.Matches(engine.TMap) {
			result = append(result, row)
			continue
		}
		rec := row.AsMap()
		existing, ok := rec.Get("id")
		if ok && existing.AsString() == id {
			found = true
			continue // skip this record
		}
		result = append(result, row)
	}

	if !found {
		return nil, fmt.Errorf("remove: no record found with id %q", id)
	}

	if result == nil {
		result = []engine.Value{}
	}
	return []engine.Value{engine.NewList(result)}, nil
}
