package lang

import (
	core "github.com/boru-lang/boru/core/go"
	engparser "github.com/boru-lang/boru/parser/go"
)

// Parse tokenizes and parses boru source into a slice of engine
// Values, the same way (*Boru).Run does internally before execution.
//
// The parser itself lives in its own standalone module
// (github.com/boru-lang/boru/parser/go). This is just the
// `lang`-package public seam over it.
func Parse(src string) ([]core.Value, error) {
	return engparser.Parse(src)
}
