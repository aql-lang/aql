// Package modules provides built-in native AQL modules that are imported
// using names of the form "aql:<name>". Each native module contains both
// Go-implemented words and AQL code definitions.
//
// Native modules produce a ModuleDesc with exports, just like file-based
// modules. The exported words are accessed via dot notation:
//
//	"aql:math" import
//	0.5 math.sin          # access sin via the math export
//	3 math.min 7          # min of 3 and 7
package modules

import (
	"fmt"

	"github.com/aql-lang/aql/lang/go/native"
	"github.com/aql-lang/aql/lang/go/policy"
)

// modules maps native module names to their builder functions.
// Each builder creates a sub-registry with the module's words and returns
// a ModuleDesc whose exports contain FnDef wrappers for those words.
var modules = map[string]func(parent *native.Registry) (native.ModuleDesc, error){
	"math":      BuildMathModule,
	"time":      BuildTimeModule,
	"matrix":    BuildMatrixModule,
	"decision":  BuildDecisionModule,
	"solardemo": BuildSolarDemoModule,
	"bin":       BuildBinaryModule,
	"type":      BuildTypeModule,
	"vm":        BuildVMModule,
	"report":    BuildReportModule,
	"test":      BuildTestModule,
	"rand":      BuildRandModule,
}

// Resolve resolves a native module name and returns a ModuleDesc.
// The parent registry is used to generate module IDs and inherit context.
// This function is intended to be called from the import handler.
//
// Consults the policy installed on parent (if any). The modules
// scope must allow the "import" op with the resolved module ID; if
// the policy has modules.install=false, all imports are refused with
// modules_disabled.
func Resolve(name string, parent *native.Registry) (native.ModuleDesc, error) {
	moduleID := "aql:" + name
	if pol := native.HostPolicy(parent); pol != nil {
		if !pol.Installed("modules") {
			return native.ModuleDesc{}, fmt.Errorf("modules disabled by policy %q", pol.Name())
		}
		if err := pol.Check("modules", "import", policy.Args{"module": moduleID}); err != nil {
			return native.ModuleDesc{}, err
		}
		// Per-module install:false check via the subscope.
		if !pol.Scope("modules").Scopes[moduleID].Installed() {
			return native.ModuleDesc{}, fmt.Errorf("module %s: install=false in policy %q", moduleID, pol.Name())
		}
	}
	fn, ok := modules[name]
	if !ok {
		return native.ModuleDesc{}, fmt.Errorf("unknown native module: %s", moduleID)
	}
	return fn(parent)
}

// InstallMathExports builds the math module and installs its exports as defs
// in the given registry. This is a convenience for test setup — equivalent to
// what happens when AQL code runs "aql:math" import.
func InstallMathExports(r *native.Registry) error {
	desc, err := BuildMathModule(r)
	if err != nil {
		return err
	}
	for name, exportMap := range desc.Exports {
		r.Defs.Push(name, native.NewMap(exportMap))
	}
	return nil
}

// InstallTimeExports builds the time module and installs its exports as defs.
func InstallTimeExports(r *native.Registry) error {
	desc, err := BuildTimeModule(r)
	if err != nil {
		return err
	}
	for name, exportMap := range desc.Exports {
		r.Defs.Push(name, native.NewMap(exportMap))
	}
	return nil
}

// InstallMatrixExports builds the matrix module and installs its exports as defs.
func InstallMatrixExports(r *native.Registry) error {
	desc, err := BuildMatrixModule(r)
	if err != nil {
		return err
	}
	for name, exportMap := range desc.Exports {
		r.Defs.Push(name, native.NewMap(exportMap))
	}
	return nil
}

// InstallDecisionExports builds the decision module and installs its exports as defs.
func InstallDecisionExports(r *native.Registry) error {
	desc, err := BuildDecisionModule(r)
	if err != nil {
		return err
	}
	for name, exportMap := range desc.Exports {
		r.Defs.Push(name, native.NewMap(exportMap))
	}
	return nil
}

// InstallRandExports builds the rand module and installs its exports as defs.
func InstallRandExports(r *native.Registry) error {
	desc, err := BuildRandModule(r)
	if err != nil {
		return err
	}
	for name, exportMap := range desc.Exports {
		r.Defs.Push(name, native.NewMap(exportMap))
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

// DecisionAQL returns the complete AQL source for the decision module.
// This is the single source of truth — the pure-AQL file module and
// module [...] inline tests are generated from this.
func DecisionAQL() string {
	return decisionAQL
}
