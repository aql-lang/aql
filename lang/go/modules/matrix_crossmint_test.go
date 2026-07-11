package modules

import (
	"testing"

	"github.com/aql-lang/aql/lang/go/native"
)

// TestTensorCrossMintMatch drives tensorFormatBehavior.Match's structural
// arms directly: every `import "aql:matrix-util"` mints its own tensor
// family, and a VALUE (or check-mode carrier) from one mint must match
// another mint's nodes by payload rank / family kind — the cross-import
// dispatch fix — while non-tensor values and rank mismatches stay out.
func TestTensorCrossMintMatch(t *testing.T) {
	rA, err := newDefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	// Both mints come from ONE registry so their seq-derived type IDs
	// genuinely differ — the real per-import shape (sub-registries adopt
	// the parent's seq). Two FRESH registries would mint IDENTICAL IDs,
	// making the "cross-mint" pairs match nominally and never reach the
	// structural arms this test exists to pin.
	ttA := MintTensorTypes(rA)
	ttB := MintTensorTypes(rA)
	if ttA.Matrix.Equal(ttB.Matrix) || ttA.Tensor.Equal(ttB.Tensor) {
		t.Fatal("test setup degenerated: the two mints must have distinct lattice identities")
	}
	b := tensorFormatBehavior{}

	matA := ttA.newMatrix(2, 2, []float64{1, 2, 3, 4})
	vecA := tensorValue(ttA.Vector, TensorData{Shape: []int{3}, Data: []float64{1, 2, 3}})

	// Concrete payload arms: rank decides, including the Tensor default.
	if !b.Match(matA, ttB.Matrix) {
		t.Fatal("cross-mint matrix must match the Matrix node")
	}
	if b.Match(matA, ttB.Vector) {
		t.Fatal("a rank-2 tensor is not a Vector")
	}
	if !b.Match(vecA, ttB.Vector) {
		t.Fatal("cross-mint vector must match the Vector node")
	}
	if !b.Match(matA, ttB.Tensor) || !b.Match(vecA, ttB.Tensor) {
		t.Fatal("every tensor matches the Tensor node")
	}

	// Carrier (check-mode) arms: family kind decides.
	if !b.Match(native.NewCarrier(ttA.Matrix), ttB.Matrix) {
		t.Fatal("cross-mint Matrix carrier must match")
	}
	if b.Match(native.NewCarrier(ttA.Vector), ttB.Matrix) {
		t.Fatal("a Vector carrier is not a Matrix")
	}
	if !b.Match(native.NewCarrier(ttA.Vector), ttB.Tensor) {
		t.Fatal("a Vector carrier is a Tensor")
	}

	// Non-tensor values stay out (isTensorFamilyNode false path).
	if b.Match(native.NewInteger(3), ttB.Matrix) {
		t.Fatal("an integer is not a Matrix")
	}
	if b.Match(native.NewCarrier(native.TInteger), ttB.Matrix) {
		t.Fatal("an Integer carrier is not a Matrix")
	}

	// Equal delegates to the kernel default (identity on same value).
	if !b.Equal(matA, matA) {
		t.Fatal("a tensor equals itself")
	}
	if b.Equal(matA, vecA) {
		t.Fatal("distinct tensors are not equal")
	}
}
