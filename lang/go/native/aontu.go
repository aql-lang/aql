package native

import (
	aontu "github.com/rjrodger/aontu/go"
)

// The aontu parser is the upstream Go port of rjrodger's aontu
// (github.com/rjrodger/aontu, the go/ module) — a CUE-inspired
// unification-based configuration dialect. It is wired in as a built-in
// `parse` kind by aql:parselang (see modules/parselang.go), so
// `import "aql:parselang"` then `parse aontu '<text>'` parses, unifies, and
// generates a Node data structure of Maps and Lists — the resolved
// configuration, exactly like the JSON family.

// AontuParse parses, unifies, and generates aontu source into generic Go
// data (map[string]any, []any, and scalars), or an error. opts is accepted
// for interface uniformity with the tabnas parser family (TabnasParser);
// aontu's `parse`/`read` options are not yet surfaced, so it is ignored. It
// is the decoder the aql:parselang `aontu` kind is built from.
//
// Generate runs the full aontu pipeline (parse → unify → generate), so a
// conflict, an unresolvable reference, or a value that does not fully
// resolve to concrete data surfaces as an error.
func AontuParse(src string, _ map[string]any) (any, error) {
	return aontu.New().Generate(src)
}
