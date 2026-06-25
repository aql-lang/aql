package modules

import (
	"math"
	"testing"

	"github.com/aql-lang/aql/lang/go/native"
)

// matrixRegistry returns a registry with the aql:matrix module loaded.
func matrixRegistry(t *testing.T) *native.Registry {
	t.Helper()
	r, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	if err := InstallMatrixExports(r); err != nil {
		t.Fatal(err)
	}
	return r
}

// matGet is a shorthand: ( matrix get <word> )
func matGet(word string) []native.Value {
	return []native.Value{
		native.NewOpenParen(), native.NewWord("MatrixUtil"), native.NewWord("dot"), native.NewWord(word), native.NewCloseParen(),
	}
}

// --- Module structure ---

func TestMatrixModuleExports(t *testing.T) {
	r, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	desc, err := BuildMatrixModule(r)
	if err != nil {
		t.Fatal(err)
	}
	matExport, ok := desc.Exports["MatrixUtil"]
	if !ok {
		t.Fatal("expected 'matrix' export")
	}
	expected := []string{
		"create", "zeros", "ones", "eye", "fill",
		"rows", "cols",
		"elem", "row", "col",
		"mat-add", "mat-sub", "mat-mul", "scale", "mat-emul",
		"transpose", "values",
		"sum", "tr", "det",
		"dot",
	}
	for _, name := range expected {
		if _, ok := matExport.Get(name); !ok {
			t.Errorf("expected %q in matrix export map", name)
		}
	}
}

// TestTensorSize covers the Sizer behaviour: the kernel `size` word
// (eng.SizeOf) reports a tensor's entry count.
func TestTensorSize(t *testing.T) {
	m := tensorValue(TMatrix, TensorData{Shape: []int{2, 3}, Data: []float64{1, 2, 3, 4, 5, 6}})
	if got := native.SizeOf(m); got != 6 {
		t.Errorf("SizeOf(2x3 matrix) = %d, want 6", got)
	}
	v := tensorValue(TVector, TensorData{Shape: []int{4}, Data: []float64{1, 2, 3, 4}})
	if got := native.SizeOf(v); got != 4 {
		t.Errorf("SizeOf(vector of 4) = %d, want 4", got)
	}
}

// --- Construction: MatrixUtil.eye ---

func TestMatrixEye(t *testing.T) {
	r := matrixRegistry(t)
	// 3 MatrixUtil.eye → 3x3 identity
	input := append([]native.Value{native.NewInteger(3)}, matGet("eye")...)
	result := runAQL(t, r, input)
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	m := AsTensor(result[0])
	if m.Rows() != 3 || m.Cols() != 3 {
		t.Fatalf("expected 3x3, got %dx%d", m.Rows(), m.Cols())
	}
	// Check diagonal is 1, off-diagonal is 0
	for i := 0; i < 3; i++ {
		for j := 0; j < 3; j++ {
			val := m.Data[i*3+j]
			if i == j && val != 1.0 {
				t.Errorf("eye[%d][%d] = %v, want 1.0", i, j, val)
			}
			if i != j && val != 0.0 {
				t.Errorf("eye[%d][%d] = %v, want 0.0", i, j, val)
			}
		}
	}
}

// --- Construction: MatrixUtil.zeros ---

func TestMatrixZeros(t *testing.T) {
	r := matrixRegistry(t)
	input := append([]native.Value{native.NewInteger(2), native.NewInteger(3)}, matGet("zeros")...)
	result := runAQL(t, r, input)
	m := AsTensor(result[0])
	if m.Rows() != 2 || m.Cols() != 3 {
		t.Fatalf("expected 2x3, got %dx%d", m.Rows(), m.Cols())
	}
	for i, v := range m.Data {
		if v != 0.0 {
			t.Errorf("zeros[%d] = %v, want 0.0", i, v)
		}
	}
}

// --- Construction: MatrixUtil.ones ---

func TestMatrixOnes(t *testing.T) {
	r := matrixRegistry(t)
	input := append([]native.Value{native.NewInteger(2), native.NewInteger(2)}, matGet("ones")...)
	result := runAQL(t, r, input)
	m := AsTensor(result[0])
	for i, v := range m.Data {
		if v != 1.0 {
			t.Errorf("ones[%d] = %v, want 1.0", i, v)
		}
	}
}

// --- Shape ---

func TestMatrixRows(t *testing.T) {
	r := matrixRegistry(t)
	mat := newMatrix(2, 3, make([]float64, 6))
	input := append([]native.Value{mat}, matGet("rows")...)
	result := runAQL(t, r, input)
	v, _ := native.AsInteger(result[0])
	if v != 2 {
		t.Errorf("rows = %v, want 2", result[0])
	}
}

func TestMatrixCols(t *testing.T) {
	r := matrixRegistry(t)
	mat := newMatrix(2, 3, make([]float64, 6))
	input := append([]native.Value{mat}, matGet("cols")...)
	result := runAQL(t, r, input)
	v, _ := native.AsInteger(result[0])
	if v != 3 {
		t.Errorf("cols = %v, want 3", result[0])
	}
}

// The core `size` word reports a matrix's entry count via the Sizer
// behavior — there is no MatrixUtil.size export (ADR-001: it would only
// shadow the core word).
func TestMatrixSize(t *testing.T) {
	r := matrixRegistry(t)
	mat := newMatrix(2, 3, make([]float64, 6))
	input := append([]native.Value{mat}, native.NewWord("size"))
	result := runAQL(t, r, input)
	v, _ := native.AsInteger(result[0])
	if v != 6 {
		t.Errorf("size = %v, want 6", result[0])
	}
}

// --- Access ---

func TestMatrixAt(t *testing.T) {
	r := matrixRegistry(t)
	// 2x2 matrix: [[1,2],[3,4]]
	mat := newMatrix(2, 2, []float64{1, 2, 3, 4})
	// mat 1 0 MatrixUtil.at → element at row 1, col 0 = 3
	input := append([]native.Value{mat, native.NewInteger(1), native.NewInteger(0)}, matGet("elem")...)
	result := runAQL(t, r, input)
	v, _ := native.AsNumber(result[0])
	if v != 3.0 {
		t.Errorf("at(1,0) = %v, want 3.0", result[0])
	}
}

func TestMatrixRow(t *testing.T) {
	r := matrixRegistry(t)
	mat := newMatrix(2, 3, []float64{1, 2, 3, 4, 5, 6})
	// mat 1 MatrixUtil.row → [4, 5, 6]
	input := append([]native.Value{mat, native.NewInteger(1)}, matGet("row")...)
	result := runAQL(t, r, input)
	list, _ := native.AsList(result[0])
	if list.Len() != 3 {
		t.Fatalf("row length = %d, want 3", list.Len())
	}
	v0, _ := native.AsNumber(list.Get(0))
	v1, _ := native.AsNumber(list.Get(1))
	v2, _ := native.AsNumber(list.Get(2))
	if v0 != 4.0 || v1 != 5.0 || v2 != 6.0 {
		t.Errorf("row(1) = %v, want [4,5,6]", result[0])
	}
}

func TestMatrixCol(t *testing.T) {
	r := matrixRegistry(t)
	mat := newMatrix(2, 3, []float64{1, 2, 3, 4, 5, 6})
	// mat 1 MatrixUtil.col → [2, 5]
	input := append([]native.Value{mat, native.NewInteger(1)}, matGet("col")...)
	result := runAQL(t, r, input)
	list, _ := native.AsList(result[0])
	if list.Len() != 2 {
		t.Fatalf("col length = %d, want 2", list.Len())
	}
	v0, _ := native.AsNumber(list.Get(0))
	v1, _ := native.AsNumber(list.Get(1))
	if v0 != 2.0 || v1 != 5.0 {
		t.Errorf("col(1) = %v, want [2,5]", result[0])
	}
}

// --- Arithmetic ---

func TestMatrixScale(t *testing.T) {
	r := matrixRegistry(t)
	mat := newMatrix(2, 2, []float64{1, 2, 3, 4})
	input := append([]native.Value{mat, native.NewInteger(3)}, matGet("scale")...)
	result := runAQL(t, r, input)
	m := AsTensor(result[0])
	expected := []float64{3, 6, 9, 12}
	for i, v := range m.Data {
		if v != expected[i] {
			t.Errorf("scale[%d] = %v, want %v", i, v, expected[i])
		}
	}
}

func TestMatrixAdd(t *testing.T) {
	r := matrixRegistry(t)
	a := newMatrix(2, 2, []float64{1, 2, 3, 4})
	b := newMatrix(2, 2, []float64{10, 20, 30, 40})
	input := append([]native.Value{a, b}, matGet("mat-add")...)
	result := runAQL(t, r, input)
	m := AsTensor(result[0])
	expected := []float64{11, 22, 33, 44}
	for i, v := range m.Data {
		if v != expected[i] {
			t.Errorf("mat-add[%d] = %v, want %v", i, v, expected[i])
		}
	}
}

func TestMatrixMul(t *testing.T) {
	r := matrixRegistry(t)
	// [[1,2],[3,4]] * [[5,6],[7,8]] = [[19,22],[43,50]]
	a := newMatrix(2, 2, []float64{1, 2, 3, 4})
	b := newMatrix(2, 2, []float64{5, 6, 7, 8})
	input := append([]native.Value{a, b}, matGet("mat-mul")...)
	result := runAQL(t, r, input)
	m := AsTensor(result[0])
	expected := []float64{19, 22, 43, 50}
	for i, v := range m.Data {
		if v != expected[i] {
			t.Errorf("mat-mul[%d] = %v, want %v", i, v, expected[i])
		}
	}
}

func TestMatrixMulRectangular(t *testing.T) {
	r := matrixRegistry(t)
	// 2x3 * 3x1 = 2x1
	a := newMatrix(2, 3, []float64{1, 2, 3, 4, 5, 6})
	b := newMatrix(3, 1, []float64{1, 1, 1})
	input := append([]native.Value{a, b}, matGet("mat-mul")...)
	result := runAQL(t, r, input)
	m := AsTensor(result[0])
	if m.Rows() != 2 || m.Cols() != 1 {
		t.Fatalf("expected 2x1, got %dx%d", m.Rows(), m.Cols())
	}
	if m.Data[0] != 6.0 || m.Data[1] != 15.0 {
		t.Errorf("mat-mul result = %v, want [6, 15]", m.Data)
	}
}

// --- Transform ---

func TestMatrixTranspose(t *testing.T) {
	r := matrixRegistry(t)
	// [[1,2,3],[4,5,6]] → [[1,4],[2,5],[3,6]]
	mat := newMatrix(2, 3, []float64{1, 2, 3, 4, 5, 6})
	input := append([]native.Value{mat}, matGet("transpose")...)
	result := runAQL(t, r, input)
	m := AsTensor(result[0])
	if m.Rows() != 3 || m.Cols() != 2 {
		t.Fatalf("expected 3x2, got %dx%d", m.Rows(), m.Cols())
	}
	expected := []float64{1, 4, 2, 5, 3, 6}
	for i, v := range m.Data {
		if v != expected[i] {
			t.Errorf("transpose[%d] = %v, want %v", i, v, expected[i])
		}
	}
}

// MatrixUtil.values returns the row-major list of entries. Named `values`,
// not `flatten`, so it does not shadow the core flatten word (ADR-001).
func TestMatrixValues(t *testing.T) {
	r := matrixRegistry(t)
	mat := newMatrix(2, 2, []float64{1, 2, 3, 4})
	input := append([]native.Value{mat}, matGet("values")...)
	result := runAQL(t, r, input)
	list, _ := native.AsList(result[0])
	if list.Len() != 4 {
		t.Fatalf("values length = %d, want 4", list.Len())
	}
	for i := 0; i < 4; i++ {
		v, _ := native.AsNumber(list.Get(i))
		if v != float64(i+1) {
			t.Errorf("values[%d] = %v, want %v", i, v, float64(i+1))
		}
	}
}

// --- Aggregation ---

func TestMatrixSum(t *testing.T) {
	r := matrixRegistry(t)
	mat := newMatrix(2, 2, []float64{1, 2, 3, 4})
	input := append([]native.Value{mat}, matGet("sum")...)
	result := runAQL(t, r, input)
	v, _ := native.AsNumber(result[0])
	if v != 10.0 {
		t.Errorf("sum = %v, want 10.0", result[0])
	}
}

func TestMatrixTrace(t *testing.T) {
	r := matrixRegistry(t)
	// trace([[1,2],[3,4]]) = 1+4 = 5
	mat := newMatrix(2, 2, []float64{1, 2, 3, 4})
	input := append([]native.Value{mat}, matGet("tr")...)
	result := runAQL(t, r, input)
	v, _ := native.AsNumber(result[0])
	if v != 5.0 {
		t.Errorf("trace = %v, want 5.0", result[0])
	}
}

func TestMatrixDet(t *testing.T) {
	r := matrixRegistry(t)
	// det([[1,2],[3,4]]) = 1*4 - 2*3 = -2
	mat := newMatrix(2, 2, []float64{1, 2, 3, 4})
	input := append([]native.Value{mat}, matGet("det")...)
	result := runAQL(t, r, input)
	v, _ := native.AsNumber(result[0])
	if math.Abs(v-(-2.0)) > 1e-10 {
		t.Errorf("det = %v, want -2.0", result[0])
	}
}

func TestMatrixDet3x3(t *testing.T) {
	r := matrixRegistry(t)
	// det([[6,1,1],[4,-2,5],[2,8,7]]) = 6(-2*7-5*8) - 1(4*7-5*2) + 1(4*8-(-2)*2)
	// = 6(-14-40) - 1(28-10) + 1(32+4) = 6(-54) - 18 + 36 = -324-18+36 = -306
	mat := newMatrix(3, 3, []float64{6, 1, 1, 4, -2, 5, 2, 8, 7})
	input := append([]native.Value{mat}, matGet("det")...)
	result := runAQL(t, r, input)
	v, _ := native.AsNumber(result[0])
	if math.Abs(v-(-306.0)) > 1e-6 {
		t.Errorf("det = %v, want -306.0", result[0])
	}
}

func TestMatrixDetIdentity(t *testing.T) {
	r := matrixRegistry(t)
	// det(I) = 1
	input := append([]native.Value{native.NewInteger(4)}, matGet("eye")...)
	eye := runAQL(t, r, input)
	input2 := append([]native.Value{eye[0]}, matGet("det")...)
	result := runAQL(t, r, input2)
	v, _ := native.AsNumber(result[0])
	if math.Abs(v-1.0) > 1e-10 {
		t.Errorf("det(I) = %v, want 1.0", result[0])
	}
}

// --- Vector ---

func TestMatrixDot(t *testing.T) {
	r := matrixRegistry(t)
	a := native.NewList([]native.Value{native.NewFloat(1), native.NewFloat(2), native.NewFloat(3)})
	b := native.NewList([]native.Value{native.NewFloat(4), native.NewFloat(5), native.NewFloat(6)})
	// [1,2,3] . [4,5,6] = 4+10+18 = 32
	input := append([]native.Value{a, b}, matGet("dot")...)
	result := runAQL(t, r, input)
	v, _ := native.AsNumber(result[0])
	if v != 32.0 {
		t.Errorf("dot = %v, want 32.0", result[0])
	}
}

// --- MatrixUtil.make from list of rows ---

func TestMatrixMakeFromRows(t *testing.T) {
	r := matrixRegistry(t)
	rows := native.NewList([]native.Value{
		native.NewList([]native.Value{native.NewInteger(1), native.NewInteger(2)}),
		native.NewList([]native.Value{native.NewInteger(3), native.NewInteger(4)}),
	})
	input := append([]native.Value{rows}, matGet("create")...)
	result := runAQL(t, r, input)
	m := AsTensor(result[0])
	if m.Rows() != 2 || m.Cols() != 2 {
		t.Fatalf("expected 2x2, got %dx%d", m.Rows(), m.Cols())
	}
	expected := []float64{1, 2, 3, 4}
	for i, v := range m.Data {
		if v != expected[i] {
			t.Errorf("make[%d] = %v, want %v", i, v, expected[i])
		}
	}
}

// --- Identity multiplication ---

func TestMatrixMulIdentity(t *testing.T) {
	r := matrixRegistry(t)
	a := newMatrix(2, 2, []float64{1, 2, 3, 4})
	eye := newMatrix(2, 2, []float64{1, 0, 0, 1})
	input := append([]native.Value{a, eye}, matGet("mat-mul")...)
	result := runAQL(t, r, input)
	m := AsTensor(result[0])
	expected := []float64{1, 2, 3, 4}
	for i, v := range m.Data {
		if v != expected[i] {
			t.Errorf("A*I[%d] = %v, want %v", i, v, expected[i])
		}
	}
}

// TestValidDims pins the matrix-constructor dimension guard: negative,
// overflowing, and oversized dimensions return errors instead of
// panicking in make([]float64, …) or allocating gigabytes. See ADR-005.
func TestValidDims(t *testing.T) {
	if err := validDims(3, 4); err != nil {
		t.Errorf("validDims(3,4) = %v, want nil", err)
	}
	if err := validDims(0, 0); err != nil {
		t.Errorf("validDims(0,0) = %v, want nil", err)
	}
	for _, d := range [][2]int{{-1, 5}, {5, -1}, {-3, -3}} {
		if err := validDims(d[0], d[1]); err == nil {
			t.Errorf("validDims(%d,%d) = nil, want non-negative error", d[0], d[1])
		}
	}
	if err := validDims(100000, 100000); err == nil {
		t.Error("validDims(100000,100000) = nil, want element-cap error")
	}
	// Overflow guard: rows*cols would wrap to a small/negative int.
	if err := validDims(1<<40, 1<<40); err == nil {
		t.Error("validDims(2^40,2^40) = nil, want overflow rejection")
	}
}

// TestMatrixConstructorsRejectBadDims drives the guard through the real
// words and asserts a negative dimension returns an error rather than
// reaching the panicking make([]float64, neg) path.
func TestMatrixConstructorsRejectBadDims(t *testing.T) {
	r := matrixRegistry(t)
	cases := []struct {
		word string
		args []native.Value
	}{
		{"zeros", []native.Value{native.NewInteger(5), native.NewInteger(-1)}},
		{"ones", []native.Value{native.NewInteger(-2), native.NewInteger(5)}},
		{"eye", []native.Value{native.NewInteger(-3)}},
	}
	for _, tc := range cases {
		input := append(append([]native.Value{}, tc.args...), matGet(tc.word)...)
		e := native.New(r)
		if _, err := e.Run(input); err == nil {
			t.Errorf("MatrixUtil.%s with negative dim: expected error, got nil", tc.word)
		}
	}
}
