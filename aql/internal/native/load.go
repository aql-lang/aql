package native

import (
	"fmt"

	"github.com/metsitaba/voxgig-exp/aql/internal/engine"
)

// loadFunc returns the "load" native function definition.
// load has suffix precedence and one signature:
//   - [table, map] — finds a single record by matching the map's key-value pairs (typically {id:"..."})
func loadFunc() NativeFunc {
	return NativeFunc{
		Name:             "load",
		SuffixPrecedence: true,
		Signatures: []NativeSig{
			{
				Args:    []engine.Type{engine.TList, engine.TMap},
				Handler: loadHandler,
			},
		},
	}
}

// loadHandler finds and returns a single record matching the filter.
// Returns an error if no matching record is found.
func loadHandler(args []engine.Value, ctx map[string]engine.Value, stack []engine.Value, r *engine.Registry) ([]engine.Value, error) {
	rows := args[0].AsList()
	filter := args[1].AsMap()

	for _, row := range rows {
		if !row.VType.Matches(engine.TMap) {
			continue
		}
		rec := row.AsMap()
		if recordMatches(rec, filter) {
			return []engine.Value{row}, nil
		}
	}

	return nil, fmt.Errorf("load: no record found matching filter")
}
