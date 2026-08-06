package eng

import "testing"

// TestVmDeferAltAttaches was dropped by the four-piece carve triage and is
// restored here: vmDeferAlt is unexported in eng, so eng is the only module
// that can hold it. The helpers covRegistry (compile_pipeline_cov_test.go)
// and seam7Dbg (vm_seam7_test.go) already live in this test package.
func TestVmDeferAltAttaches(t *testing.T) {
	r := covRegistry(t, nil)
	plain := vmDeferAlt(r, seam7Dbg, 0, "vm:poly-no-match", "x", nil)
	if ae, ok := plain.(*BoruError); !ok || ae.Code != "internal_error" || ae.DeferAlt != nil {
		t.Errorf("a nil alt must be a plain defer, got %v", plain)
	}
	alt := &BoruError{Code: "signature_error", Detail: "d"}
	err := vmDeferAlt(r, seam7Dbg, 0, "vm:poly-no-match", "x", alt)
	ae, ok := err.(*BoruError)
	if !ok || ae.Code != "internal_error" || ae.DeferAlt != alt {
		t.Errorf("the alt must ride the internal defer, got %v", err)
	}
}
