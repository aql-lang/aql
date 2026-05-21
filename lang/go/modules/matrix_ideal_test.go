package modules

import (
	"strings"
	"testing"

	"github.com/aql-lang/aql/eng/go/parser"
	"github.com/aql-lang/aql/lang/go/native"
)

// tensorRegistry returns a registry with the aql:matrix module loaded
// and a parse func installed, so source-string programs can be run.
func tensorRegistry(t *testing.T) *native.Registry {
	t.Helper()
	r, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	r.SetParseFunc(parser.Parse)
	if err := InstallMatrixExports(r); err != nil {
		t.Fatal(err)
	}
	return r
}

// runTensorSrc parses and runs an AQL source string.
func runTensorSrc(t *testing.T, r *native.Registry, src string) ([]native.Value, error) {
	t.Helper()
	values, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	return native.NewTop(r).Run(values)
}

// Importing aql:matrix registers the three tensor type-kinds, with
// Matrix and Vector refining Tensor. They are absent until imported.
func TestTensorIdeals_Registered(t *testing.T) {
	r := tensorRegistry(t)
	for _, name := range []string{"Tensor", "Matrix", "Vector"} {
		if r.Ideals.Get(name) == nil {
			t.Errorf("tensor Ideal %q not registered after import", name)
		}
	}
	tensor := r.Ideals.Get("Tensor")
	if m := r.Ideals.Get("Matrix"); m == nil || m.Refines != tensor {
		t.Error("Matrix should refine Tensor")
	}
	if v := r.Ideals.Get("Vector"); v == nil || v.Refines != tensor {
		t.Error("Vector should refine Tensor")
	}
	bare, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	if bare.Ideals.Get("Tensor") != nil {
		t.Error("Tensor kind present without importing aql:matrix")
	}
}

func TestMake_Vector(t *testing.T) {
	r := tensorRegistry(t)
	res, err := runTensorSrc(t, r, "make Vector [1 2 3]")
	if err != nil {
		t.Fatal(err)
	}
	td := AsTensor(res[0])
	if td.Rank() != 1 || td.Rows() != 3 {
		t.Fatalf("got shape %v, want [3]", td.Shape)
	}
	if td.Data[0] != 1 || td.Data[2] != 3 {
		t.Errorf("data = %v, want [1 2 3]", td.Data)
	}
}

func TestMake_Matrix(t *testing.T) {
	r := tensorRegistry(t)
	res, err := runTensorSrc(t, r, "make Matrix [[1 2][3 4]]")
	if err != nil {
		t.Fatal(err)
	}
	td := AsTensor(res[0])
	if td.Rank() != 2 || td.Rows() != 2 || td.Cols() != 2 {
		t.Fatalf("got shape %v, want [2 2]", td.Shape)
	}
}

func TestMake_Tensor(t *testing.T) {
	r := tensorRegistry(t)
	// Nested-list form — the shape is inferred from the nesting.
	res, err := runTensorSrc(t, r, "make Tensor [[[1 2][3 4]][[5 6][7 8]]]")
	if err != nil {
		t.Fatal(err)
	}
	if td := AsTensor(res[0]); !shapeEqual(td.Shape, []int{2, 2, 2}) {
		t.Fatalf("nested form: got shape %v, want [2 2 2]", td.Shape)
	}
	// Explicit {shape data} map form.
	res, err = runTensorSrc(t, r, "make Tensor {shape:[2 3] data:[1 2 3 4 5 6]}")
	if err != nil {
		t.Fatal(err)
	}
	if td := AsTensor(res[0]); !shapeEqual(td.Shape, []int{2, 3}) {
		t.Fatalf("map form: got shape %v, want [2 3]", td.Shape)
	}
	// Ragged nested data is rejected.
	if _, err := runTensorSrc(t, r, "make Tensor [[1 2][3 4 5]]"); err == nil {
		t.Error("ragged tensor data should fail")
	}
}

// `type Matrix {rows cols}` constructs a shaped type that `def` binds
// and `make` checks data against.
func TestType_ShapedMatrix(t *testing.T) {
	r := tensorRegistry(t)
	res, err := runTensorSrc(t, r,
		"def Mat3 (type Matrix {rows:3 cols:3}) make Mat3 [[1 2 3][4 5 6][7 8 9]]")
	if err != nil {
		t.Fatalf("shaped make should succeed: %v", err)
	}
	if td := AsTensor(res[0]); !shapeEqual(td.Shape, []int{3, 3}) {
		t.Fatalf("got shape %v, want [3 3]", td.Shape)
	}
	// A shape mismatch against the constructed type is rejected.
	_, err = runTensorSrc(t, r,
		"def M3 (type Matrix {rows:3 cols:3}) make M3 [[1 2][3 4]]")
	if err == nil {
		t.Fatal("shape-mismatched make should fail")
	}
	if !strings.Contains(err.Error(), "does not match") {
		t.Errorf("error = %q, want a shape-mismatch message", err.Error())
	}
}

func TestType_ShapedVector(t *testing.T) {
	r := tensorRegistry(t)
	res, err := runTensorSrc(t, r, "def V5 (type Vector {len:5}) make V5 [1 2 3 4 5]")
	if err != nil {
		t.Fatalf("shaped vector make should succeed: %v", err)
	}
	if td := AsTensor(res[0]); !shapeEqual(td.Shape, []int{5}) {
		t.Fatalf("got shape %v, want [5]", td.Shape)
	}
	_, err = runTensorSrc(t, r, "def V3 (type Vector {len:3}) make V3 [1 2 3 4 5]")
	if err == nil || !strings.Contains(err.Error(), "does not match") {
		t.Errorf("length-mismatched vector make should fail with a shape error, got %v", err)
	}
}

// A constructed tensor type is recognised by the kernel as a type:
// `typeof` reports Type, not the concrete tensor kind.
func TestTensorType_TypeofIsType(t *testing.T) {
	r := tensorRegistry(t)
	res, err := runTensorSrc(t, r, "(type Matrix {rows:2 cols:2}) typeof")
	if err != nil {
		t.Fatal(err)
	}
	if got := res[0].String(); got != "Type" {
		t.Errorf("typeof a constructed Matrix type = %q, want Type", got)
	}
}

// Matrix and Vector refine Tensor, so disabling the Tensor kind makes
// the whole family unavailable for dispatch.
func TestTensorIdeals_DisableBaseDisablesFamily(t *testing.T) {
	r := tensorRegistry(t)
	r.Ideals.Get("Tensor").Enabled = false
	_, err := runTensorSrc(t, r, "make Matrix [[1 2][3 4]]")
	if err == nil || !strings.Contains(err.Error(), "not available") {
		t.Errorf("make Matrix with Tensor disabled: want a 'not available' error, got %v", err)
	}
	r.Ideals.Get("Tensor").Enabled = true
	if _, err := runTensorSrc(t, r, "make Matrix [[1 2][3 4]]"); err != nil {
		t.Errorf("make Matrix after re-enabling Tensor: %v", err)
	}
}

// A Matrix value satisfies `is Tensor` — Matrix is a lattice child of
// Tensor.
func TestMatrix_IsTensor(t *testing.T) {
	r := tensorRegistry(t)
	res, err := runTensorSrc(t, r, "make Matrix [[1 2][3 4]] is Tensor")
	if err != nil {
		t.Fatal(err)
	}
	if got := res[0].String(); got != "true" {
		t.Errorf("a made Matrix `is Tensor` = %q, want true", got)
	}
}

func TestTensor_Format(t *testing.T) {
	r := tensorRegistry(t)
	res, err := runTensorSrc(t, r, "make Matrix [[1 2 3][4 5 6]]")
	if err != nil {
		t.Fatal(err)
	}
	if got := res[0].String(); got != "Matrix(2x3)" {
		t.Errorf("Format = %q, want Matrix(2x3)", got)
	}
}
