package modules

import "github.com/aql-lang/aql/lang/go/native"

// Test seams (design/TEST-SEAMS.10.md). Each var defaults to the real
// implementation and is swapped only by tests (with a t.Cleanup restore)
// to drive the registry-construction error arms of the Build*Module
// constructors and the vm sub-engine — arms that are otherwise
// unreachable because the default registry construction cannot fail
// once the process has started.
var (
	// newDefaultRegistry seams native.DefaultRegistry for the module
	// constructors' init-error arms.
	newDefaultRegistry = native.DefaultRegistry
	// newDefaultRegistryWithPolicy seams native.DefaultRegistryWithPolicy
	// for the vm sub-engine's init-error arm.
	newDefaultRegistryWithPolicy = native.DefaultRegistryWithPolicy
)
