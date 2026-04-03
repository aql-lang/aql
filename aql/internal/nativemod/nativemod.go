// Package nativemod provides built-in native AQL modules that are imported
// using names of the form "aql:<name>". Each native module contains both
// Go-implemented words and AQL code definitions.
//
// Native modules produce a ModuleDesc with exports, just like file-based
// modules. The exported words are accessed via dot notation:
//
//	"aql:math" import
//	0.5 math.sin          # access sin via the math export
//	3 math.min 7          # min of 3 and 7
package nativemod

import (
	"fmt"

	"github.com/metsitaba/voxgig-exp/aql/internal/engine"
)

// modules maps native module names to their builder functions.
// Each builder creates a sub-registry with the module's words and returns
// a ModuleDesc whose exports contain FnDef wrappers for those words.
var modules = map[string]func(parent *engine.Registry) (engine.ModuleDesc, error){
	"math":      BuildMathModule,
	"time":      BuildTimeModule,
	"matrix":    BuildMatrixModule,
	"decision":  BuildDecisionModule,
	"solardemo": BuildSolarDemoModule,
}

// Resolve resolves a native module name and returns a ModuleDesc.
// The parent registry is used to generate module IDs and inherit context.
// This function is intended to be called from the import handler.
func Resolve(name string, parent *engine.Registry) (engine.ModuleDesc, error) {
	fn, ok := modules[name]
	if !ok {
		return engine.ModuleDesc{}, fmt.Errorf("unknown native module: aql:%s", name)
	}
	return fn(parent)
}

// InstallMathExports builds the math module and installs its exports as defs
// in the given registry. This is a convenience for test setup — equivalent to
// what happens when AQL code runs "aql:math" import.
func InstallMathExports(r *engine.Registry) error {
	desc, err := BuildMathModule(r)
	if err != nil {
		return err
	}
	for name, exportMap := range desc.Exports {
		r.DefStacks[name] = append(r.DefStacks[name], engine.NewMap(exportMap))
	}
	return nil
}

// InstallTimeExports builds the time module and installs its exports as defs.
func InstallTimeExports(r *engine.Registry) error {
	desc, err := BuildTimeModule(r)
	if err != nil {
		return err
	}
	for name, exportMap := range desc.Exports {
		r.DefStacks[name] = append(r.DefStacks[name], engine.NewMap(exportMap))
	}
	return nil
}

// InstallMatrixExports builds the matrix module and installs its exports as defs.
func InstallMatrixExports(r *engine.Registry) error {
	desc, err := BuildMatrixModule(r)
	if err != nil {
		return err
	}
	for name, exportMap := range desc.Exports {
		r.DefStacks[name] = append(r.DefStacks[name], engine.NewMap(exportMap))
	}
	return nil
}

// InstallDecisionExports builds the decision module and installs its exports as defs.
func InstallDecisionExports(r *engine.Registry) error {
	desc, err := BuildDecisionModule(r)
	if err != nil {
		return err
	}
	for name, exportMap := range desc.Exports {
		r.DefStacks[name] = append(r.DefStacks[name], engine.NewMap(exportMap))
	}
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
