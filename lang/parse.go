package lang

import (
	"github.com/aql-lang/aql/eng"
	engparser "github.com/aql-lang/aql/eng/parser"
)

// Parse tokenizes and parses AQL source into a slice of engine
// Values, the same way (*AQL).Run does internally before execution.
//
// The parser itself lives in the standalone eng module
// (github.com/aql-lang/aql/eng/parser). This is just the
// `lang`-package public seam over it.
func Parse(src string) ([]eng.Value, error) {
	return engparser.Parse(src)
}
