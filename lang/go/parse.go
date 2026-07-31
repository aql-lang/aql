package lang

import (
	"github.com/boru-lang/boru/eng/go"
	engparser "github.com/boru-lang/boru/eng/go/parser"
)

// Parse tokenizes and parses boru source into a slice of engine
// Values, the same way (*Boru).Run does internally before execution.
//
// The parser itself lives in the standalone eng module
// (github.com/boru-lang/boru/eng/go/parser). This is just the
// `lang`-package public seam over it.
func Parse(src string) ([]eng.Value, error) {
	return engparser.Parse(src)
}
