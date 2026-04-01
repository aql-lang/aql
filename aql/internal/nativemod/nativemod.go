// Package nativemod provides built-in native AQL modules that are imported
// using names of the form "aql:<name>". Each native module contains both
// Go-implemented words and (optionally) AQL code definitions.
//
// Native modules are registered into the engine registry when imported,
// making their words available as regular AQL words in the current scope.
package nativemod

import (
	"fmt"

	"github.com/metsitaba/voxgig-exp/aql/internal/engine"
)

// modules maps native module names to their registration functions.
var modules = map[string]func(r *engine.Registry){
	"math": RegisterMath,
}

// Resolve resolves a native module name and registers its words into the
// given registry. Returns an error if the module name is unknown.
// This function is intended to be assigned to Registry.NativeModResolver.
func Resolve(name string, r *engine.Registry) error {
	fn, ok := modules[name]
	if !ok {
		return fmt.Errorf("unknown native module: aql:%s", name)
	}
	fn(r)
	return nil
}

// Names returns the list of available native module names.
func Names() []string {
	names := make([]string, 0, len(modules))
	for name := range modules {
		names = append(names, name)
	}
	return names
}
