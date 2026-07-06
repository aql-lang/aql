package native

import "testing"

// Smoke coverage for the shared seam5 drivers themselves: each entry
// point runs a positive program and rejects a malformed one, so the
// helpers stay exercised even when individual seam5 suites come and go.
func TestSeam5DriversSmoke(t *testing.T) {
	r := seam5Reg(t)

	out, err := seam5Run(r, `1 add 2`)
	if err != nil || len(out) != 1 || out[0].String() != "3" {
		t.Fatalf("seam5Run: %v %v", out, err)
	}
	if _, err := seam5Run(r, `1 add (`); err == nil {
		t.Error("seam5Run malformed source: want parse error")
	}

	if _, err := seam5Check(seam5Reg(t), `1 add 2`); err != nil {
		t.Fatalf("seam5Check: %v", err)
	}
	if _, err := seam5Check(seam5Reg(t), `1 add (`); err == nil {
		t.Error("seam5Check malformed source: want parse error")
	}

	if _, err := seam5CompileCheck(seam5Reg(t), `1 add 2`); err != nil {
		t.Fatalf("seam5CompileCheck: %v", err)
	}
	if _, err := seam5CompileCheck(seam5Reg(t), `1 add (`); err == nil {
		t.Error("seam5CompileCheck malformed source: want parse error")
	}
}
