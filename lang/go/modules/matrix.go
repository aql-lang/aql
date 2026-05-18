package modules

import (
	"fmt"
	"math"

	"github.com/aql-lang/aql/eng/go"
	"github.com/aql-lang/aql/lang/go/native"
)

// TMatrix is the Scalar/Number/Matrix type identity. Owned by the
// matrix native module — the kernel doesn't carry this type; the
// module registers it via eng.Builtin.RegisterExternalBuiltin in
// the var initialiser below so that any other package-level var
// referencing TMatrix (notably MatrixNatives, whose signatures
// embed TMatrix) sees a non-nil pointer at slice-init time. Go
// package-init order resolves dependencies before declaration order;
// declaring TMatrix's initialiser to call the registration helper
// gives MatrixNatives a hard dependency on TMatrix.
//
// FixedID 2000 comes from the documented
// lang/go/internal/nativemod/matrix range (2000-2999).
var TMatrix = registerMatrixType()

func registerMatrixType() *eng.Type {
	t, err := eng.Builtin.RegisterExternalBuiltin("Scalar/Number/Matrix", 2000, matrixFormatBehavior{})
	if err != nil {
		// lint:allow-panic — init-time builtin registration; see
		// registerTimerType in engine/native_misc.go for rationale.
		panic(fmt.Sprintf("matrix: register TMatrix: %v", err))
	}
	return t
}

// matrixFormatBehavior renders a Matrix as "Matrix(rowsxcols)".
// Lives in matrix.go (this file) post Step 8 — the kernel
// previously installed an identical Behavior in
// eng/coretype_format_behaviors.go; it has been removed in favour
// of the module-owned one declared here and attached at registration.
type matrixFormatBehavior struct{}

func (matrixFormatBehavior) Match(v native.Value, t *native.Type) bool {
	return native.DefaultBehavior.Match(v, t)
}
func (matrixFormatBehavior) Equal(a, b native.Value) bool {
	return native.DefaultBehavior.Equal(a, b)
}
func (matrixFormatBehavior) Format(v native.Value) string {
	if mp, ok := v.Data.(native.MatrixData); ok {
		return fmt.Sprintf("Matrix(%dx%d)", mp.Rows, mp.Cols)
	}
	return "Matrix(nil)"
}

// NewMatrix constructs a Matrix value carrying the given MatrixData
// payload. Moved out of eng at Step 8 — the kernel no longer carries
// a constructor for a type it doesn't own.
func NewMatrix(m native.MatrixData) native.Value {
	return native.NewValueRaw(TMatrix, m)
}

// AsMatrix extracts the MatrixData payload from a Matrix value.
// Moved out of eng at Step 6/7 — the kernel no longer carries an
// accessor method for a type it doesn't own. The implementation
// is identical to the previous method on Value.
func AsMatrix(v native.Value) native.MatrixData {
	if m, ok := v.Data.(native.MatrixData); ok {
		return m
	}
	return native.MatrixData{}
}

// BuildMatrixModule creates the "aql:matrix" native module. It registers
// Go-implemented matrix words into an isolated sub-registry and returns a
// ModuleDesc with a "matrix" export containing FnDef wrappers for each word.
func BuildMatrixModule(parent *native.Registry) (native.ModuleDesc, error) {
	subReg, err := native.DefaultRegistry()
	if err != nil {
		return native.ModuleDesc{}, err
	}

	for _, n := range MatrixNatives {
		subReg.RegisterNativeFunc(n)
	}

	exports := native.NewOrderedMap()

	// Construction
	exports.Set("create", makeListToMatrixFnDef("matrix-make", subReg))
	exports.Set("zeros", makeIntIntToMatrixFnDef("matrix-zeros", subReg))
	exports.Set("ones", makeIntIntToMatrixFnDef("matrix-ones", subReg))
	exports.Set("eye", makeIntToMatrixFnDef("matrix-eye", subReg))
	exports.Set("fill", makeIntIntNumToMatrixFnDef("matrix-fill", subReg))

	// Shape
	exports.Set("rows", makeMatrixToIntFnDef("matrix-rows", subReg))
	exports.Set("cols", makeMatrixToIntFnDef("matrix-cols", subReg))
	exports.Set("size", makeMatrixToIntFnDef("matrix-size", subReg))

	// Access
	exports.Set("elem", makeMatrixIntIntToDecFnDef("matrix-at", subReg))
	exports.Set("row", makeMatrixIntToListFnDef("matrix-row", subReg))
	exports.Set("col", makeMatrixIntToListFnDef("matrix-col", subReg))

	// Arithmetic
	exports.Set("mat-add", makeMatrixMatrixToMatrixFnDef("matrix-mat-add", subReg))
	exports.Set("mat-sub", makeMatrixMatrixToMatrixFnDef("matrix-mat-sub", subReg))
	exports.Set("mat-mul", makeMatrixMatrixToMatrixFnDef("matrix-mat-mul", subReg))
	exports.Set("scale", makeMatrixNumToMatrixFnDef("matrix-scale", subReg))
	exports.Set("mat-emul", makeMatrixMatrixToMatrixFnDef("matrix-mat-emul", subReg))

	// Transform
	exports.Set("transpose", makeUnaryMatrixFnDef("matrix-transpose", subReg))
	exports.Set("flatten", makeMatrixToListFnDef("matrix-flatten", subReg))

	// Aggregation
	exports.Set("sum", makeMatrixToDecFnDef("matrix-sum", subReg))
	exports.Set("tr", makeMatrixToDecFnDef("matrix-trace", subReg))
	exports.Set("det", makeMatrixToDecFnDef("matrix-det", subReg))

	// Vector
	exports.Set("dot", makeListListToDecFnDef("matrix-dot", subReg))

	modID := parent.Modules.NextID()
	desc := native.ModuleDesc{
		ID:      modID,
		Exports: map[string]*native.OrderedMap{"matrix": exports},
	}
	return desc, nil
}

// --- FnDef helpers ---

func makeListToMatrixFnDef(wordName string, subReg *native.Registry) native.Value {
	return native.NewFnDef(native.FnDefInfo{Name: wordName,
		Sigs: []native.FnSig{{
			Params:  []native.FnParam{{Type: native.TList}},
			Returns: []*native.Type{TMatrix},
			Body:    []native.Value{native.NewWord(wordName)},
		}},
		Registry: subReg,
	})
}

func makeIntIntToMatrixFnDef(wordName string, subReg *native.Registry) native.Value {
	return native.NewFnDef(native.FnDefInfo{Name: wordName,
		Sigs: []native.FnSig{{
			Params:  []native.FnParam{{Type: native.TInteger}, {Type: native.TInteger}},
			Returns: []*native.Type{TMatrix},
			Body:    []native.Value{native.NewWord(wordName)},
		}},
		Registry: subReg,
	})
}

func makeIntToMatrixFnDef(wordName string, subReg *native.Registry) native.Value {
	return native.NewFnDef(native.FnDefInfo{Name: wordName,
		Sigs: []native.FnSig{{
			Params:  []native.FnParam{{Type: native.TInteger}},
			Returns: []*native.Type{TMatrix},
			Body:    []native.Value{native.NewWord(wordName)},
		}},
		Registry: subReg,
	})
}

func makeIntIntNumToMatrixFnDef(wordName string, subReg *native.Registry) native.Value {
	return native.NewFnDef(native.FnDefInfo{Name: wordName,
		Sigs: []native.FnSig{{
			Params:  []native.FnParam{{Type: native.TInteger}, {Type: native.TInteger}, {Type: native.TNumber}},
			Returns: []*native.Type{TMatrix},
			Body:    []native.Value{native.NewWord(wordName)},
		}},
		Registry: subReg,
	})
}

func makeMatrixToIntFnDef(wordName string, subReg *native.Registry) native.Value {
	return native.NewFnDef(native.FnDefInfo{Name: wordName,
		Sigs: []native.FnSig{{
			Params:  []native.FnParam{{Type: TMatrix}},
			Returns: []*native.Type{native.TInteger},
			Body:    []native.Value{native.NewWord(wordName)},
		}},
		Registry: subReg,
	})
}

func makeMatrixIntIntToDecFnDef(wordName string, subReg *native.Registry) native.Value {
	// FnDef.Params is matched deepest-first against the user stack. Keep the
	// user-facing positional order `mat row col` (deepest→top) so the
	// wrapper resolves correctly; the underlying NativeFunc sig is the
	// reverse (top-down = stack[N-1], stack[N-2], …, stack[0]) and is
	// "data-last" by virtue of mat being deepest.
	return native.NewFnDef(native.FnDefInfo{Name: wordName,
		Sigs: []native.FnSig{{
			Params:  []native.FnParam{{Type: TMatrix}, {Type: native.TInteger}, {Type: native.TInteger}},
			Returns: []*native.Type{native.TDecimal},
			Body:    []native.Value{native.NewWord(wordName)},
		}},
		Registry: subReg,
	})
}

func makeMatrixIntToListFnDef(wordName string, subReg *native.Registry) native.Value {
	return native.NewFnDef(native.FnDefInfo{Name: wordName,
		Sigs: []native.FnSig{{
			Params:  []native.FnParam{{Type: TMatrix}, {Type: native.TInteger}},
			Returns: []*native.Type{native.TList},
			Body:    []native.Value{native.NewWord(wordName)},
		}},
		Registry: subReg,
	})
}

func makeMatrixMatrixToMatrixFnDef(wordName string, subReg *native.Registry) native.Value {
	return native.NewFnDef(native.FnDefInfo{Name: wordName,
		Sigs: []native.FnSig{{
			Params:  []native.FnParam{{Type: TMatrix}, {Type: TMatrix}},
			Returns: []*native.Type{TMatrix},
			Body:    []native.Value{native.NewWord(wordName)},
		}},
		Registry: subReg,
	})
}

func makeMatrixNumToMatrixFnDef(wordName string, subReg *native.Registry) native.Value {
	return native.NewFnDef(native.FnDefInfo{Name: wordName,
		Sigs: []native.FnSig{{
			Params:  []native.FnParam{{Type: TMatrix}, {Type: native.TNumber}},
			Returns: []*native.Type{TMatrix},
			Body:    []native.Value{native.NewWord(wordName)},
		}},
		Registry: subReg,
	})
}

func makeUnaryMatrixFnDef(wordName string, subReg *native.Registry) native.Value {
	return native.NewFnDef(native.FnDefInfo{Name: wordName,
		Sigs: []native.FnSig{{
			Params:  []native.FnParam{{Type: TMatrix}},
			Returns: []*native.Type{TMatrix},
			Body:    []native.Value{native.NewWord(wordName)},
		}},
		Registry: subReg,
	})
}

func makeMatrixToListFnDef(wordName string, subReg *native.Registry) native.Value {
	return native.NewFnDef(native.FnDefInfo{Name: wordName,
		Sigs: []native.FnSig{{
			Params:  []native.FnParam{{Type: TMatrix}},
			Returns: []*native.Type{native.TList},
			Body:    []native.Value{native.NewWord(wordName)},
		}},
		Registry: subReg,
	})
}

func makeMatrixToDecFnDef(wordName string, subReg *native.Registry) native.Value {
	return native.NewFnDef(native.FnDefInfo{Name: wordName,
		Sigs: []native.FnSig{{
			Params:  []native.FnParam{{Type: TMatrix}},
			Returns: []*native.Type{native.TDecimal},
			Body:    []native.Value{native.NewWord(wordName)},
		}},
		Registry: subReg,
	})
}

func makeListListToDecFnDef(wordName string, subReg *native.Registry) native.Value {
	return native.NewFnDef(native.FnDefInfo{Name: wordName,
		Sigs: []native.FnSig{{
			Params:  []native.FnParam{{Type: native.TList}, {Type: native.TList}},
			Returns: []*native.Type{native.TDecimal},
			Body:    []native.Value{native.NewWord(wordName)},
		}},
		Registry: subReg,
	})
}

// --- Word definitions ---

// MatrixNatives is the consolidated NativeFunc slice for the matrix
// module's Go-implemented words. Replaces the per-word
// registerMatrix* functions and the master registerAllMatrixWords
// aggregator.
var MatrixNatives = []native.NativeFunc{
	// Construction.
	{
		Name:        "matrix-make",
		ForwardArgs: true,
		Signatures: []native.NativeSig{{
			Args: []*native.Type{native.TList},
			Handler: func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
				rl, _ := native.AsList(args[0])
				if rl.IsNil() {
					return nil, fmt.Errorf("make: expected list of row lists")
				}
				rows := rl.Len()
				if rows == 0 {
					return nil, fmt.Errorf("make: empty list")
				}
				firstRow, _ := native.AsList(rl.Get(0))
				if firstRow.IsNil() {
					return nil, fmt.Errorf("make: first element is not a list")
				}
				cols := firstRow.Len()
				if cols == 0 {
					return nil, fmt.Errorf("make: first row is empty")
				}
				data := make([]float64, 0, rows*cols)
				for i := 0; i < rows; i++ {
					row, _ := native.AsList(rl.Get(i))
					if row.IsNil() {
						return nil, fmt.Errorf("make: row %d is not a list", i)
					}
					if row.Len() != cols {
						return nil, fmt.Errorf("make: row %d has %d elements, expected %d", i, row.Len(), cols)
					}
					for j := 0; j < cols; j++ {
						n, err := native.AsNumber(row.Get(j))
						if err != nil {
							return nil, err
						}
						data = append(data, n)
					}
				}
				return []native.Value{NewMatrix(native.MatrixData{Data: data, Rows: rows, Cols: cols})}, nil
			},
			Returns: []*native.Type{TMatrix},
		}},
	},
	{
		// rows cols zeros → args[0]=cols (top), args[1]=rows (deeper)
		Name:        "matrix-zeros",
		ForwardArgs: true,
		Signatures: []native.NativeSig{{
			Args: []*native.Type{native.TInteger, native.TInteger},
			Handler: func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
				r64, err := args[1].AsConcreteInteger()
				if err != nil {
					return nil, err
				}
				c64, err := args[0].AsConcreteInteger()
				if err != nil {
					return nil, err
				}
				rows, cols := int(r64), int(c64)
				data := make([]float64, rows*cols)
				return []native.Value{NewMatrix(native.MatrixData{Data: data, Rows: rows, Cols: cols})}, nil
			},
			Returns: []*native.Type{TMatrix},
		}},
	},
	{
		// rows cols ones → args[0]=cols (top), args[1]=rows (deeper)
		Name:        "matrix-ones",
		ForwardArgs: true,
		Signatures: []native.NativeSig{{
			Args: []*native.Type{native.TInteger, native.TInteger},
			Handler: func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
				r64, err := args[1].AsConcreteInteger()
				if err != nil {
					return nil, err
				}
				c64, err := args[0].AsConcreteInteger()
				if err != nil {
					return nil, err
				}
				rows, cols := int(r64), int(c64)
				data := make([]float64, rows*cols)
				for i := range data {
					data[i] = 1.0
				}
				return []native.Value{NewMatrix(native.MatrixData{Data: data, Rows: rows, Cols: cols})}, nil
			},
			Returns: []*native.Type{TMatrix},
		}},
	},
	{
		Name:        "matrix-eye",
		ForwardArgs: true,
		Signatures: []native.NativeSig{{
			Args: []*native.Type{native.TInteger},
			Handler: func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
				n64, err := args[0].AsConcreteInteger()
				if err != nil {
					return nil, err
				}
				n := int(n64)
				data := make([]float64, n*n)
				for i := 0; i < n; i++ {
					data[i*n+i] = 1.0
				}
				return []native.Value{NewMatrix(native.MatrixData{Data: data, Rows: n, Cols: n})}, nil
			},
			Returns: []*native.Type{TMatrix},
		}},
	},
	{
		Name:        "matrix-fill",
		ForwardArgs: true,
		Signatures: []native.NativeSig{{
			Args: []*native.Type{native.TInteger, native.TInteger, native.TNumber},
			Handler: func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
				r64, err := args[0].AsConcreteInteger()
				if err != nil {
					return nil, err
				}
				c64, err := args[1].AsConcreteInteger()
				if err != nil {
					return nil, err
				}
				val, err := native.AsNumber(args[2])
				if err != nil {
					return nil, err
				}
				rows, cols := int(r64), int(c64)
				data := make([]float64, rows*cols)
				for i := range data {
					data[i] = val
				}
				return []native.Value{NewMatrix(native.MatrixData{Data: data, Rows: rows, Cols: cols})}, nil
			},
			Returns: []*native.Type{TMatrix},
		}},
	},
	// Shape.
	{
		Name:        "matrix-rows",
		ForwardArgs: true,
		Signatures: []native.NativeSig{{
			Args: []*native.Type{TMatrix},
			Handler: func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
				m := AsMatrix(args[0])
				return []native.Value{native.NewInteger(int64(m.Rows))}, nil
			},
			Returns: []*native.Type{native.TInteger},
		}},
	},
	{
		Name:        "matrix-cols",
		ForwardArgs: true,
		Signatures: []native.NativeSig{{
			Args: []*native.Type{TMatrix},
			Handler: func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
				m := AsMatrix(args[0])
				return []native.Value{native.NewInteger(int64(m.Cols))}, nil
			},
			Returns: []*native.Type{native.TInteger},
		}},
	},
	{
		Name:        "matrix-size",
		ForwardArgs: true,
		Signatures: []native.NativeSig{{
			Args: []*native.Type{TMatrix},
			Handler: func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
				m := AsMatrix(args[0])
				return []native.Value{native.NewInteger(int64(m.Rows * m.Cols))}, nil
			},
			Returns: []*native.Type{native.TList},
		}},
	},
	// Access.
	{
		Name:        "matrix-at",
		ForwardArgs: true,
		Signatures: []native.NativeSig{{
			// Data-last: [col, row, mat]. Under §1.4 stack-top-first matching,
			// `mat row col matrix-at` binds sig[0]=col (top), sig[1]=row,
			// sig[2]=mat (deepest).
			Args: []*native.Type{native.TInteger, native.TInteger, TMatrix},
			Handler: func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
				c64, err := args[0].AsConcreteInteger()
				if err != nil {
					return nil, err
				}
				r64, err := args[1].AsConcreteInteger()
				if err != nil {
					return nil, err
				}
				m := AsMatrix(args[2])
				row, col := int(r64), int(c64)
				if row < 0 || row >= m.Rows || col < 0 || col >= m.Cols {
					return nil, fmt.Errorf("at: index (%d,%d) out of bounds for %dx%d matrix", row, col, m.Rows, m.Cols)
				}
				return []native.Value{native.NewDecimal(m.Data[row*m.Cols+col])}, nil
			},
			Returns: []*native.Type{native.TDecimal},
		}},
	},
	{
		Name:        "matrix-row",
		ForwardArgs: true,
		Signatures: []native.NativeSig{{
			Args: []*native.Type{native.TInteger, TMatrix},
			Handler: func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
				r64, err := args[0].AsConcreteInteger()
				if err != nil {
					return nil, err
				}
				m := AsMatrix(args[1])
				row := int(r64)
				if row < 0 || row >= m.Rows {
					return nil, fmt.Errorf("row: index %d out of bounds for %d rows", row, m.Rows)
				}
				elems := make([]native.Value, m.Cols)
				for j := 0; j < m.Cols; j++ {
					elems[j] = native.NewDecimal(m.Data[row*m.Cols+j])
				}
				return []native.Value{native.NewList(elems)}, nil
			},
			Returns: []*native.Type{native.TList},
		}},
	},
	{
		Name:        "matrix-col",
		ForwardArgs: true,
		Signatures: []native.NativeSig{{
			Args: []*native.Type{native.TInteger, TMatrix},
			Handler: func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
				c64, err := args[0].AsConcreteInteger()
				if err != nil {
					return nil, err
				}
				m := AsMatrix(args[1])
				col := int(c64)
				if col < 0 || col >= m.Cols {
					return nil, fmt.Errorf("col: index %d out of bounds for %d cols", col, m.Cols)
				}
				elems := make([]native.Value, m.Rows)
				for i := 0; i < m.Rows; i++ {
					elems[i] = native.NewDecimal(m.Data[i*m.Cols+col])
				}
				return []native.Value{native.NewList(elems)}, nil
			},
			Returns: []*native.Type{native.TList},
		}},
	},
	// Arithmetic.
	{
		Name:        "matrix-mat-add",
		ForwardArgs: true,
		Signatures: []native.NativeSig{{
			Args: []*native.Type{TMatrix, TMatrix},
			Handler: func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
				a := AsMatrix(args[0])
				b := AsMatrix(args[1])
				if a.Rows != b.Rows || a.Cols != b.Cols {
					return nil, fmt.Errorf("mat-add: dimension mismatch %dx%d vs %dx%d", a.Rows, a.Cols, b.Rows, b.Cols)
				}
				data := make([]float64, len(a.Data))
				for i := range data {
					data[i] = a.Data[i] + b.Data[i]
				}
				return []native.Value{NewMatrix(native.MatrixData{Data: data, Rows: a.Rows, Cols: a.Cols})}, nil
			},
			Returns: []*native.Type{TMatrix},
		}},
	},
	{
		// a b mat-sub → args[0]=b (top), args[1]=a
		Name:        "matrix-mat-sub",
		ForwardArgs: true,
		Signatures: []native.NativeSig{{
			Args: []*native.Type{TMatrix, TMatrix},
			Handler: func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
				a := AsMatrix(args[1])
				b := AsMatrix(args[0])
				if a.Rows != b.Rows || a.Cols != b.Cols {
					return nil, fmt.Errorf("mat-sub: dimension mismatch %dx%d vs %dx%d", a.Rows, a.Cols, b.Rows, b.Cols)
				}
				data := make([]float64, len(a.Data))
				for i := range data {
					data[i] = a.Data[i] - b.Data[i]
				}
				return []native.Value{NewMatrix(native.MatrixData{Data: data, Rows: a.Rows, Cols: a.Cols})}, nil
			},
			Returns: []*native.Type{TMatrix},
		}},
	},
	{
		// a b mat-mul → args[0]=b (top), args[1]=a
		Name:        "matrix-mat-mul",
		ForwardArgs: true,
		Signatures: []native.NativeSig{{
			Args: []*native.Type{TMatrix, TMatrix},
			Handler: func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
				a := AsMatrix(args[1])
				b := AsMatrix(args[0])
				result, err := matMul(a, b)
				if err != nil {
					return nil, err
				}
				return []native.Value{NewMatrix(result)}, nil
			},
			Returns: []*native.Type{TMatrix},
		}},
	},
	{
		Name:        "matrix-scale",
		ForwardArgs: true,
		Signatures: []native.NativeSig{{
			Args: []*native.Type{native.TNumber, TMatrix},
			Handler: func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
				s, err := native.AsNumber(args[0])
				if err != nil {
					return nil, err
				}
				m := AsMatrix(args[1])
				data := make([]float64, len(m.Data))
				for i := range data {
					data[i] = m.Data[i] * s
				}
				return []native.Value{NewMatrix(native.MatrixData{Data: data, Rows: m.Rows, Cols: m.Cols})}, nil
			},
			Returns: []*native.Type{TMatrix},
		}},
	},
	{
		Name:        "matrix-mat-emul",
		ForwardArgs: true,
		Signatures: []native.NativeSig{{
			Args: []*native.Type{TMatrix, TMatrix},
			Handler: func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
				a := AsMatrix(args[0])
				b := AsMatrix(args[1])
				if a.Rows != b.Rows || a.Cols != b.Cols {
					return nil, fmt.Errorf("mat-emul: dimension mismatch %dx%d vs %dx%d", a.Rows, a.Cols, b.Rows, b.Cols)
				}
				data := make([]float64, len(a.Data))
				for i := range data {
					data[i] = a.Data[i] * b.Data[i]
				}
				return []native.Value{NewMatrix(native.MatrixData{Data: data, Rows: a.Rows, Cols: a.Cols})}, nil
			},
			Returns: []*native.Type{TMatrix},
		}},
	},
	// Transform.
	{
		Name:        "matrix-transpose",
		ForwardArgs: true,
		Signatures: []native.NativeSig{{
			Args: []*native.Type{TMatrix},
			Handler: func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
				m := AsMatrix(args[0])
				data := make([]float64, len(m.Data))
				for i := 0; i < m.Rows; i++ {
					for j := 0; j < m.Cols; j++ {
						data[j*m.Rows+i] = m.Data[i*m.Cols+j]
					}
				}
				return []native.Value{NewMatrix(native.MatrixData{Data: data, Rows: m.Cols, Cols: m.Rows})}, nil
			},
			Returns: []*native.Type{TMatrix},
		}},
	},
	{
		Name:        "matrix-flatten",
		ForwardArgs: true,
		Signatures: []native.NativeSig{{
			Args: []*native.Type{TMatrix},
			Handler: func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
				m := AsMatrix(args[0])
				elems := make([]native.Value, len(m.Data))
				for i, v := range m.Data {
					elems[i] = native.NewDecimal(v)
				}
				return []native.Value{native.NewList(elems)}, nil
			},
			Returns: []*native.Type{native.TList},
		}},
	},
	// Aggregation.
	{
		Name:        "matrix-sum",
		ForwardArgs: true,
		Signatures: []native.NativeSig{{
			Args: []*native.Type{TMatrix},
			Handler: func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
				m := AsMatrix(args[0])
				s := 0.0
				for _, v := range m.Data {
					s += v
				}
				return []native.Value{native.NewDecimal(s)}, nil
			},
			Returns: []*native.Type{native.TDecimal},
		}},
	},
	{
		Name:        "matrix-trace",
		ForwardArgs: true,
		Signatures: []native.NativeSig{{
			Args: []*native.Type{TMatrix},
			Handler: func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
				m := AsMatrix(args[0])
				if m.Rows != m.Cols {
					return nil, fmt.Errorf("trace: not square (%dx%d)", m.Rows, m.Cols)
				}
				s := 0.0
				for i := 0; i < m.Rows; i++ {
					s += m.Data[i*m.Cols+i]
				}
				return []native.Value{native.NewDecimal(s)}, nil
			},
			Returns: []*native.Type{native.TDecimal},
		}},
	},
	{
		Name:        "matrix-det",
		ForwardArgs: true,
		Signatures: []native.NativeSig{{
			Args: []*native.Type{TMatrix},
			Handler: func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
				m := AsMatrix(args[0])
				d, err := matDet(m)
				if err != nil {
					return nil, err
				}
				return []native.Value{native.NewDecimal(d)}, nil
			},
			Returns: []*native.Type{native.TDecimal},
		}},
	},
	// Vector.
	{
		Name:        "matrix-dot",
		ForwardArgs: true,
		Signatures: []native.NativeSig{{
			Args: []*native.Type{native.TList, native.TList},
			Handler: func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
				a, _ := native.AsList(args[0])
				b, _ := native.AsList(args[1])
				if a.IsNil() || b.IsNil() {
					return nil, fmt.Errorf("dot: expected two lists")
				}
				if a.Len() != b.Len() {
					return nil, fmt.Errorf("dot: length mismatch %d vs %d", a.Len(), b.Len())
				}
				s := 0.0
				for i := 0; i < a.Len(); i++ {
					av, err := native.AsNumber(a.Get(i))
					if err != nil {
						return nil, err
					}
					bv, err := native.AsNumber(b.Get(i))
					if err != nil {
						return nil, err
					}
					s += av * bv
				}
				return []native.Value{native.NewDecimal(s)}, nil
			},
			Returns: []*native.Type{native.TDecimal},
		}},
	},
}

// --- Internal helpers ---

func matMul(a, b native.MatrixData) (native.MatrixData, error) {
	if a.Cols != b.Rows {
		return native.MatrixData{}, fmt.Errorf("mat-mul: dimension mismatch %dx%d * %dx%d", a.Rows, a.Cols, b.Rows, b.Cols)
	}
	result := make([]float64, a.Rows*b.Cols)
	for i := 0; i < a.Rows; i++ {
		for j := 0; j < b.Cols; j++ {
			sum := 0.0
			for k := 0; k < a.Cols; k++ {
				sum += a.Data[i*a.Cols+k] * b.Data[k*b.Cols+j]
			}
			result[i*b.Cols+j] = sum
		}
	}
	return native.MatrixData{Data: result, Rows: a.Rows, Cols: b.Cols}, nil
}

func matDet(m native.MatrixData) (float64, error) {
	if m.Rows != m.Cols {
		return 0, fmt.Errorf("det: not square (%dx%d)", m.Rows, m.Cols)
	}
	n := m.Rows
	// Copy data for in-place elimination
	a := make([]float64, len(m.Data))
	copy(a, m.Data)
	det := 1.0
	for i := 0; i < n; i++ {
		// Find pivot
		maxVal := math.Abs(a[i*n+i])
		maxRow := i
		for k := i + 1; k < n; k++ {
			if math.Abs(a[k*n+i]) > maxVal {
				maxVal = math.Abs(a[k*n+i])
				maxRow = k
			}
		}
		if maxVal < 1e-12 {
			return 0, nil // singular
		}
		if maxRow != i {
			for j := 0; j < n; j++ {
				a[i*n+j], a[maxRow*n+j] = a[maxRow*n+j], a[i*n+j]
			}
			det *= -1
		}
		det *= a[i*n+i]
		for k := i + 1; k < n; k++ {
			factor := a[k*n+i] / a[i*n+i]
			for j := i; j < n; j++ {
				a[k*n+j] -= factor * a[i*n+j]
			}
		}
	}
	return det, nil
}
