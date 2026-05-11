package lang

import (
	"github.com/metsitaba/voxgig-exp/eng"
	"github.com/metsitaba/voxgig-exp/lang/internal/parser"
)

// Parse tokenizes and parses AQL source into a slice of engine
// Values, the same way (*AQL).Run does internally before execution.
//
// It is exported so external packages — notably the eng kernel's
// spec test harness in eng/test — can build test inputs with the
// real production parser rather than reimplementing tokenization.
// The parser itself stays in lang/internal/parser; this is the
// public seam.
func Parse(src string) ([]eng.Value, error) {
	return parser.Parse(src)
}
