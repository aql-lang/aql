package modules

import (
	"github.com/aql-lang/aql/lang/go/native"
)

// w9NopParser is a do-nothing host parser handler for constructor seams
// (carried in specs, never dispatched).
func w9NopParser([]native.Value, map[string]native.Value, []native.Value, *native.Registry) ([]native.Value, error) {
	return nil, nil
}

// (The host-state collision seams died with RegisterHostParser: the
// built-in kind set is static and name-disjoint, and
// TestW8InstallBuiltinParserDuplicate drives the installer's defensive
// duplicate arm directly.)
