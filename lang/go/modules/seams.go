package modules

import (
	"regexp"

	"github.com/boru-lang/boru/lang/go/native"
)

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
	// regexpCompile seams regexp.Compile for gexCompile's failure arm —
	// gexSpecToRegexp QuoteMetas every literal rune, so the translated
	// pattern is always valid and only a seam can observe the propagation.
	regexpCompile = regexp.Compile
	// transplantExtension seams native.TransplantExtension for the
	// Install*Exports transplant-failure arms — the extensions the time /
	// matrix builders export always transplant cleanly onto a fresh
	// default registry, so only a seam can observe the propagation.
	transplantExtension = native.TransplantExtension
)
