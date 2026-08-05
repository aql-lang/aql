package core

// Seam-7 (cluster A4): in-package unit tests for previously-unreached
// blocks in define_type.go, sigimpl.go and deadsig.go. Direct calls to the
// guard / error / marker arms per design/TEST-SEAMS.10.md.

import "testing"

// --- define_type.go -------------------------------------------------------

// --- sigimpl.go -----------------------------------------------------------

func TestS7SigImplMarkersAndNilImpl(t *testing.T) {
	(&GoImpl{}).sigImpl()   // zero-statement seal marker
	(&BoruImpl{}).sigImpl() // zero-statement seal marker
	// A signature with no implementation returns a nil dispatch handler.
	var s Signature
	if s.DispatchHandler() != nil {
		t.Error("a nil-Impl signature must have a nil dispatch handler")
	}
}

// --- deadsig.go -----------------------------------------------------------
