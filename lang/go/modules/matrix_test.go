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
		native.NewOpenParen(), native.NewWord("matrix"), native.NewWord("get"), native.NewWord(word), native.NewCloseParen(),
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
	matExport, ok := desc.Exports["matrix"]
	if !ok {
		t.Fatal("expected 'matrix' export")
	}
	expected := []string{
		"create", "zeros", "ones", "eye", "fill",
		"rows", "cols", "size",
		"elem", "row", "col",
		"mat-add", "mat-sub", "mat-mul", "scale", "mat-emul",
		"transpose", "flatten",
		"sum", "tr", "det",
		"dot",
	}
	for _, name := range expected {
		if _, ok := matExport.Get(name); !ok {
			t.Errorf("expected %q in matrix export map", name)
		}
	}
}

// --- Construction: matrix.eye ---

func TestMatrixEye(t *testing.T) {
	r := matrixRegistry(t)
	// 3 matrix.eye → 3x3 identity
	input := append([]native.Value{native.NewInteger(3)}, matGet("eye")...)
	result := runAQL(t, r, input)
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	m := AsMatrix(result[0])
	if m.Rows != 3 || m.Cols != 3 {
		t.Fatalf("expected 3x3, got %dx%d", m.Rows, m.Cols)
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

// --- Construction: matrix.zeros ---

func TestMatrixZeros(t *testing.T) {
	r := matrixRegistry(t)
	input := append([]native.Value{native.NewInteger(2), native.NewInteger(3)}, matGet("zeros")...)
	result := runAQL(t, r, input)
	m := AsMatrix(result[0])
	if m.Rows != 2 || m.Cols != 3 {
		t.Fatalf("expected 2x3, got %dx%d", m.Rows, m.Cols)
	}
	for i, v := range m.Data {
		if v != 0.0 {
			t.Errorf("zeros[%d] = %v, want 0.0", i, v)
		}
	}
}

// --- Construction: matrix.ones ---

func TestMatrixOnes(t *testing.T) {
	r := matrixRegistry(t)
	input := append([]native.Value{native.NewInteger(2), native.NewInteger(2)}, matGet("ones")...)
	result := runAQL(t, r, input)
	m := AsMatrix(result[0])
	for i, v := range m.Data {
		if v != 1.0 {
			t.Errorf("ones[%d] = %v, want 1.0", i, v)
		}
	}
}

// --- Shape ---

func TestMatrixRows(t *testing.T) {
	r := matrixRegistry(t)
	mat := NewMatrix(native.MatrixData{Data: make([]float64, 6), Rows: 2, Cols: 3})
	input := append([]native.Value{mat}, matGet("rows")...)
	result := runAQL(t, r, input)
	v, _ := native.AsInteger(result[0])
	if v != 2 {
		t.Errorf("rows = %v, want 2", result[0])
	}
}

func TestMatrixCols(t *testing.T) {
	r := matrixRegistry(t)
	mat := NewMatrix(native.MatrixData{Data: make([]float64, 6), Rows: 2, Cols: 3})
	input := append([]native.Value{mat}, matGet("cols")...)
	result := runAQL(t, r, input)
	v, _ := native.AsInteger(result[0])
	if v != 3 {
		t.Errorf("cols = %v, want 3", result[0])
	}
}

func TestMatrixSize(t *testing.T) {
	r := matrixRegistry(t)
	mat := NewMatrix(native.MatrixData{Data: make([]float64, 6), Rows: 2, Cols: 3})
	input := append([]native.Value{mat}, matGet("size")...)
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
	mat := NewMatrix(native.MatrixData{Data: []float64{1, 2, 3, 4}, Rows: 2, Cols: 2})
	// mat 1 0 matrix.at → element at row 1, col 0 = 3
	input := append([]native.Value{mat, native.NewInteger(1), native.NewInteger(0)}, matGet("elem")...)
	result := runAQL(t, r, input)
	v, _ := native.AsNumber(result[0])
	if v != 3.0 {
		t.Errorf("at(1,0) = %v, want 3.0", result[0])
	}
}

func TestMatrixRow(t *testing.T) {
	r := matrixRegistry(t)
	mat := NewMatrix(native.MatrixData{Data: []float64{1, 2, 3, 4, 5, 6}, Rows: 2, Cols: 3})
	// mat 1 matrix.row → [4, 5, 6]
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
	mat := NewMatrix(native.MatrixData{Data: []float64{1, 2, 3, 4, 5, 6}, Rows: 2, Cols: 3})
	// mat 1 matrix.col → [2, 5]
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
	mat := NewMatrix(native.MatrixData{Data: []float64{1, 2, 3, 4}, Rows: 2, Cols: 2})
	input := append([]native.Value{mat, native.NewInteger(3)}, matGet("scale")...)
	result := runAQL(t, r, input)
	m := AsMatrix(result[0])
	expected := []float64{3, 6, 9, 12}
	for i, v := range m.Data {
		if v != expected[i] {
			t.Errorf("scale[%d] = %v, want %v", i, v, expected[i])
		}
	}
}

func TestMatrixAdd(t *testing.T) {
	r := matrixRegistry(t)
	a := NewMatrix(native.MatrixData{Data: []float64{1, 2, 3, 4}, Rows: 2, Cols: 2})
	b := NewMatrix(native.MatrixData{Data: []float64{10, 20, 30, 40}, Rows: 2, Cols: 2})
	input := append([]native.Value{a, b}, matGet("mat-add")...)
	result := runAQL(t, r, input)
	m := AsMatrix(result[0])
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
	a := NewMatrix(native.MatrixData{Data: []float64{1, 2, 3, 4}, Rows: 2, Cols: 2})
	b := NewMatrix(native.MatrixData{Data: []float64{5, 6, 7, 8}, Rows: 2, Cols: 2})
	input := append([]native.Value{a, b}, matGet("mat-mul")...)
	result := runAQL(t, r, input)
	m := AsMatrix(result[0])
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
	a := NewMatrix(native.MatrixData{Data: []float64{1, 2, 3, 4, 5, 6}, Rows: 2, Cols: 3})
	b := NewMatrix(native.MatrixData{Data: []float64{1, 1, 1}, Rows: 3, Cols: 1})
	input := append([]native.Value{a, b}, matGet("mat-mul")...)
	result := runAQL(t, r, input)
	m := AsMatrix(result[0])
	if m.Rows != 2 || m.Cols != 1 {
		t.Fatalf("expected 2x1, got %dx%d", m.Rows, m.Cols)
	}
	if m.Data[0] != 6.0 || m.Data[1] != 15.0 {
		t.Errorf("mat-mul result = %v, want [6, 15]", m.Data)
	}
}

// --- Transform ---

func TestMatrixTranspose(t *testing.T) {
	r := matrixRegistry(t)
	// [[1,2,3],[4,5,6]] → [[1,4],[2,5],[3,6]]
	mat := NewMatrix(native.MatrixData{Data: []float64{1, 2, 3, 4, 5, 6}, Rows: 2, Cols: 3})
	input := append([]native.Value{mat}, matGet("transpose")...)
	result := runAQL(t, r, input)
	m := AsMatrix(result[0])
	if m.Rows != 3 || m.Cols != 2 {
		t.Fatalf("expected 3x2, got %dx%d", m.Rows, m.Cols)
	}
	expected := []float64{1, 4, 2, 5, 3, 6}
	for i, v := range m.Data {
		if v != expected[i] {
			t.Errorf("transpose[%d] = %v, want %v", i, v, expected[i])
		}
	}
}

func TestMatrixFlatten(t *testing.T) {
	r := matrixRegistry(t)
	mat := NewMatrix(native.MatrixData{Data: []float64{1, 2, 3, 4}, Rows: 2, Cols: 2})
	input := append([]native.Value{mat}, matGet("flatten")...)
	result := runAQL(t, r, input)
	list, _ := native.AsList(result[0])
	if list.Len() != 4 {
		t.Fatalf("flatten length = %d, want 4", list.Len())
	}
	for i := 0; i < 4; i++ {
		v, _ := native.AsNumber(list.Get(i))
		if v != float64(i+1) {
			t.Errorf("flatten[%d] = %v, want %v", i, v, float64(i+1))
		}
	}
}

// --- Aggregation ---

func TestMatrixSum(t *testing.T) {
	r := matrixRegistry(t)
	mat := NewMatrix(native.MatrixData{Data: []float64{1, 2, 3, 4}, Rows: 2, Cols: 2})
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
	mat := NewMatrix(native.MatrixData{Data: []float64{1, 2, 3, 4}, Rows: 2, Cols: 2})
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
	mat := NewMatrix(native.MatrixData{Data: []float64{1, 2, 3, 4}, Rows: 2, Cols: 2})
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
	mat := NewMatrix(native.MatrixData{Data: []float64{6, 1, 1, 4, -2, 5, 2, 8, 7}, Rows: 3, Cols: 3})
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
	a := native.NewList([]native.Value{native.NewDecimal(1), native.NewDecimal(2), native.NewDecimal(3)})
	b := native.NewList([]native.Value{native.NewDecimal(4), native.NewDecimal(5), native.NewDecimal(6)})
	// [1,2,3] . [4,5,6] = 4+10+18 = 32
	input := append([]native.Value{a, b}, matGet("dot")...)
	result := runAQL(t, r, input)
	v, _ := native.AsNumber(result[0])
	if v != 32.0 {
		t.Errorf("dot = %v, want 32.0", result[0])
	}
}

// --- matrix.make from list of rows ---

func TestMatrixMakeFromRows(t *testing.T) {
	r := matrixRegistry(t)
	rows := native.NewList([]native.Value{
		native.NewList([]native.Value{native.NewInteger(1), native.NewInteger(2)}),
		native.NewList([]native.Value{native.NewInteger(3), native.NewInteger(4)}),
	})
	input := append([]native.Value{rows}, matGet("create")...)
	result := runAQL(t, r, input)
	m := AsMatrix(result[0])
	if m.Rows != 2 || m.Cols != 2 {
		t.Fatalf("expected 2x2, got %dx%d", m.Rows, m.Cols)
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
	a := NewMatrix(native.MatrixData{Data: []float64{1, 2, 3, 4}, Rows: 2, Cols: 2})
	eye := NewMatrix(native.MatrixData{Data: []float64{1, 0, 0, 1}, Rows: 2, Cols: 2})
	input := append([]native.Value{a, eye}, matGet("mat-mul")...)
	result := runAQL(t, r, input)
	m := AsMatrix(result[0])
	expected := []float64{1, 2, 3, 4}
	for i, v := range m.Data {
		if v != expected[i] {
			t.Errorf("A*I[%d] = %v, want %v", i, v, expected[i])
		}
	}
}
