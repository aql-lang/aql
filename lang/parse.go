package lang

import (
	"github.com/metsitaba/voxgig-exp/eng"
	engparser "github.com/metsitaba/voxgig-exp/eng/parser"
)

// Parse tokenizes and parses AQL source into a slice of engine
// Values, the same way (*AQL).Run does internally before execution.
//
// The parser itself lives in the standalone eng module
// (github.com/metsitaba/voxgig-exp/eng/parser). This is just the
// `lang`-package public seam over it.
func Parse(src string) ([]eng.Value, error) {
	return engparser.Parse(src)
}
