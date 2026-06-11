package native

import (
	"fmt"

	jsonic "github.com/jsonicjs/jsonic/go"
)

// parseTextHandler implements `StructUtil.parse` — jsonic/JSON text →
// data, the decode complement of jsonify (design/PARSING.10.md §2).
// The input parses in DATA context, the way a map literal's interior
// parses: unquoted text becomes strings, numbers become numbers,
// true/false booleans. Nothing is evaluated and nothing dispatches —
// `StructUtil.parse "{val:'if'}"` yields a map holding the STRING
// 'if'. The jsonic superset is accepted (unquoted keys, optional
// commas), so strict JSON parses too. Malformed input raises
// [aql/parse_error] — loud, never a silent none.
func parseTextHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	text, err := AsString(args[0])
	if err != nil {
		return nil, r.AqlError("parse_error",
			fmt.Sprintf("parse: argument must be a string, got %s", args[0].String()), "parse")
	}
	result, perr := jsonic.Make().Parse(text)
	if perr != nil {
		return nil, r.AqlError("parse_error",
			fmt.Sprintf("parse: invalid jsonic/JSON: %v", perr), "parse")
	}
	if result == nil {
		return nil, r.AqlError("parse_error",
			"parse: input is empty (no value to decode)", "parse")
	}
	v, cerr := jsonicToValue(result)
	if cerr != nil {
		return nil, r.AqlError("parse_error",
			fmt.Sprintf("parse: %v", cerr), "parse")
	}
	return []Value{v}, nil
}
