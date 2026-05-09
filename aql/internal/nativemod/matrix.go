package nativemod

import (
	"fmt"
	"math"

	"github.com/metsitaba/voxgig-exp/aql/internal/engine"
)

// BuildMatrixModule creates the "aql:matrix" native module. It registers
// Go-implemented matrix words into an isolated sub-registry and returns a
// ModuleDesc with a "matrix" export containing FnDef wrappers for each word.
func BuildMatrixModule(parent *engine.Registry) (engine.ModuleDesc, error) {
	subReg, err := engine.DefaultRegistry()
	if err != nil {
		return engine.ModuleDesc{}, err
	}

	for _, n := range MatrixNatives {
		subReg.RegisterNativeFunc(n)
	}

	exports := engine.NewOrderedMap()

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

	modID := parent.NextModuleID()
	desc := engine.ModuleDesc{
		ID:      modID,
		Exports: map[string]*engine.OrderedMap{"matrix": exports},
	}
	return desc, nil
}

// --- FnDef helpers ---

func makeListToMatrixFnDef(wordName string, subReg *engine.Registry) engine.Value {
	return engine.NewFnDef(engine.FnDefInfo{Name: wordName,
		Sigs: []engine.FnSig{{
			Params:  []engine.FnParam{{Type: engine.TList}},
			Returns: []engine.Type{engine.TMatrix},
			Body:    []engine.Value{engine.NewWord(wordName)},
		}},
		Registry: subReg,
	})
}

func makeIntIntToMatrixFnDef(wordName string, subReg *engine.Registry) engine.Value {
	return engine.NewFnDef(engine.FnDefInfo{Name: wordName,
		Sigs: []engine.FnSig{{
			Params:  []engine.FnParam{{Type: engine.TInteger}, {Type: engine.TInteger}},
			Returns: []engine.Type{engine.TMatrix},
			Body:    []engine.Value{engine.NewWord(wordName)},
		}},
		Registry: subReg,
	})
}

func makeIntToMatrixFnDef(wordName string, subReg *engine.Registry) engine.Value {
	return engine.NewFnDef(engine.FnDefInfo{Name: wordName,
		Sigs: []engine.FnSig{{
			Params:  []engine.FnParam{{Type: engine.TInteger}},
			Returns: []engine.Type{engine.TMatrix},
			Body:    []engine.Value{engine.NewWord(wordName)},
		}},
		Registry: subReg,
	})
}

func makeIntIntNumToMatrixFnDef(wordName string, subReg *engine.Registry) engine.Value {
	return engine.NewFnDef(engine.FnDefInfo{Name: wordName,
		Sigs: []engine.FnSig{{
			Params:  []engine.FnParam{{Type: engine.TInteger}, {Type: engine.TInteger}, {Type: engine.TNumber}},
			Returns: []engine.Type{engine.TMatrix},
			Body:    []engine.Value{engine.NewWord(wordName)},
		}},
		Registry: subReg,
	})
}

func makeMatrixToIntFnDef(wordName string, subReg *engine.Registry) engine.Value {
	return engine.NewFnDef(engine.FnDefInfo{Name: wordName,
		Sigs: []engine.FnSig{{
			Params:  []engine.FnParam{{Type: engine.TMatrix}},
			Returns: []engine.Type{engine.TInteger},
			Body:    []engine.Value{engine.NewWord(wordName)},
		}},
		Registry: subReg,
	})
}

func makeMatrixIntIntToDecFnDef(wordName string, subReg *engine.Registry) engine.Value {
	// FnDef.Params is matched deepest-first against the user stack. Keep the
	// user-facing positional order `mat row col` (deepest→top) so the
	// wrapper resolves correctly; the underlying NativeFunc sig is the
	// reverse (top-down = stack[N-1], stack[N-2], …, stack[0]) and is
	// "data-last" by virtue of mat being deepest.
	return engine.NewFnDef(engine.FnDefInfo{Name: wordName,
		Sigs: []engine.FnSig{{
			Params:  []engine.FnParam{{Type: engine.TMatrix}, {Type: engine.TInteger}, {Type: engine.TInteger}},
			Returns: []engine.Type{engine.TDecimal},
			Body:    []engine.Value{engine.NewWord(wordName)},
		}},
		Registry: subReg,
	})
}

func makeMatrixIntToListFnDef(wordName string, subReg *engine.Registry) engine.Value {
	return engine.NewFnDef(engine.FnDefInfo{Name: wordName,
		Sigs: []engine.FnSig{{
			Params:  []engine.FnParam{{Type: engine.TMatrix}, {Type: engine.TInteger}},
			Returns: []engine.Type{engine.TList},
			Body:    []engine.Value{engine.NewWord(wordName)},
		}},
		Registry: subReg,
	})
}

func makeMatrixMatrixToMatrixFnDef(wordName string, subReg *engine.Registry) engine.Value {
	return engine.NewFnDef(engine.FnDefInfo{Name: wordName,
		Sigs: []engine.FnSig{{
			Params:  []engine.FnParam{{Type: engine.TMatrix}, {Type: engine.TMatrix}},
			Returns: []engine.Type{engine.TMatrix},
			Body:    []engine.Value{engine.NewWord(wordName)},
		}},
		Registry: subReg,
	})
}

func makeMatrixNumToMatrixFnDef(wordName string, subReg *engine.Registry) engine.Value {
	return engine.NewFnDef(engine.FnDefInfo{Name: wordName,
		Sigs: []engine.FnSig{{
			Params:  []engine.FnParam{{Type: engine.TMatrix}, {Type: engine.TNumber}},
			Returns: []engine.Type{engine.TMatrix},
			Body:    []engine.Value{engine.NewWord(wordName)},
		}},
		Registry: subReg,
	})
}

func makeUnaryMatrixFnDef(wordName string, subReg *engine.Registry) engine.Value {
	return engine.NewFnDef(engine.FnDefInfo{Name: wordName,
		Sigs: []engine.FnSig{{
			Params:  []engine.FnParam{{Type: engine.TMatrix}},
			Returns: []engine.Type{engine.TMatrix},
			Body:    []engine.Value{engine.NewWord(wordName)},
		}},
		Registry: subReg,
	})
}

func makeMatrixToListFnDef(wordName string, subReg *engine.Registry) engine.Value {
	return engine.NewFnDef(engine.FnDefInfo{Name: wordName,
		Sigs: []engine.FnSig{{
			Params:  []engine.FnParam{{Type: engine.TMatrix}},
			Returns: []engine.Type{engine.TList},
			Body:    []engine.Value{engine.NewWord(wordName)},
		}},
		Registry: subReg,
	})
}

func makeMatrixToDecFnDef(wordName string, subReg *engine.Registry) engine.Value {
	return engine.NewFnDef(engine.FnDefInfo{Name: wordName,
		Sigs: []engine.FnSig{{
			Params:  []engine.FnParam{{Type: engine.TMatrix}},
			Returns: []engine.Type{engine.TDecimal},
			Body:    []engine.Value{engine.NewWord(wordName)},
		}},
		Registry: subReg,
	})
}

func makeListListToDecFnDef(wordName string, subReg *engine.Registry) engine.Value {
	return engine.NewFnDef(engine.FnDefInfo{Name: wordName,
		Sigs: []engine.FnSig{{
			Params:  []engine.FnParam{{Type: engine.TList}, {Type: engine.TList}},
			Returns: []engine.Type{engine.TDecimal},
			Body:    []engine.Value{engine.NewWord(wordName)},
		}},
		Registry: subReg,
	})
}

// --- Word definitions ---

// MatrixNatives is the consolidated NativeFunc slice for the matrix
// module's Go-implemented words. Replaces the per-word
// registerMatrix* functions and the master registerAllMatrixWords
// aggregator.
var MatrixNatives = []engine.NativeFunc{
	// Construction.
	{
		Name:              "matrix-make",
		ForwardPrecedence: true,
		Signatures: []engine.NativeSig{{
			Args: []engine.Type{engine.TList},
			Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
				rl := args[0].AsList()
				if rl.IsNil() {
					return nil, fmt.Errorf("make: expected list of row lists")
				}
				rows := rl.Len()
				if rows == 0 {
					return nil, fmt.Errorf("make: empty list")
				}
				firstRow := rl.Get(0).AsList()
				if firstRow.IsNil() {
					return nil, fmt.Errorf("make: first element is not a list")
				}
				cols := firstRow.Len()
				if cols == 0 {
					return nil, fmt.Errorf("make: first row is empty")
				}
				data := make([]float64, 0, rows*cols)
				for i := 0; i < rows; i++ {
					row := rl.Get(i).AsList()
					if row.IsNil() {
						return nil, fmt.Errorf("make: row %d is not a list", i)
					}
					if row.Len() != cols {
						return nil, fmt.Errorf("make: row %d has %d elements, expected %d", i, row.Len(), cols)
					}
					for j := 0; j < cols; j++ {
						n, err := row.Get(j).AsNumber()
						if err != nil {
							return nil, err
						}
						data = append(data, n)
					}
				}
				return []engine.Value{engine.NewMatrix(engine.MatrixData{Data: data, Rows: rows, Cols: cols})}, nil
			},
			Returns: []engine.Type{engine.TMatrix},
		}},
	},
	{
		// rows cols zeros → args[0]=cols (top), args[1]=rows (deeper)
		Name:              "matrix-zeros",
		ForwardPrecedence: true,
		Signatures: []engine.NativeSig{{
			Args: []engine.Type{engine.TInteger, engine.TInteger},
			Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
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
				return []engine.Value{engine.NewMatrix(engine.MatrixData{Data: data, Rows: rows, Cols: cols})}, nil
			},
			Returns: []engine.Type{engine.TMatrix},
		}},
	},
	{
		// rows cols ones → args[0]=cols (top), args[1]=rows (deeper)
		Name:              "matrix-ones",
		ForwardPrecedence: true,
		Signatures: []engine.NativeSig{{
			Args: []engine.Type{engine.TInteger, engine.TInteger},
			Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
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
				return []engine.Value{engine.NewMatrix(engine.MatrixData{Data: data, Rows: rows, Cols: cols})}, nil
			},
			Returns: []engine.Type{engine.TMatrix},
		}},
	},
	{
		Name:              "matrix-eye",
		ForwardPrecedence: true,
		Signatures: []engine.NativeSig{{
			Args: []engine.Type{engine.TInteger},
			Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
				n64, err := args[0].AsConcreteInteger()
				if err != nil {
					return nil, err
				}
				n := int(n64)
				data := make([]float64, n*n)
				for i := 0; i < n; i++ {
					data[i*n+i] = 1.0
				}
				return []engine.Value{engine.NewMatrix(engine.MatrixData{Data: data, Rows: n, Cols: n})}, nil
			},
			Returns: []engine.Type{engine.TMatrix},
		}},
	},
	{
		Name:              "matrix-fill",
		ForwardPrecedence: true,
		Signatures: []engine.NativeSig{{
			Args: []engine.Type{engine.TInteger, engine.TInteger, engine.TNumber},
			Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
				r64, err := args[0].AsConcreteInteger()
				if err != nil {
					return nil, err
				}
				c64, err := args[1].AsConcreteInteger()
				if err != nil {
					return nil, err
				}
				val, err := args[2].AsNumber()
				if err != nil {
					return nil, err
				}
				rows, cols := int(r64), int(c64)
				data := make([]float64, rows*cols)
				for i := range data {
					data[i] = val
				}
				return []engine.Value{engine.NewMatrix(engine.MatrixData{Data: data, Rows: rows, Cols: cols})}, nil
			},
			Returns: []engine.Type{engine.TMatrix},
		}},
	},
	// Shape.
	{
		Name:              "matrix-rows",
		ForwardPrecedence: true,
		Signatures: []engine.NativeSig{{
			Args: []engine.Type{engine.TMatrix},
			Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
				m := args[0].AsMatrix()
				return []engine.Value{engine.NewInteger(int64(m.Rows))}, nil
			},
			Returns: []engine.Type{engine.TInteger},
		}},
	},
	{
		Name:              "matrix-cols",
		ForwardPrecedence: true,
		Signatures: []engine.NativeSig{{
			Args: []engine.Type{engine.TMatrix},
			Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
				m := args[0].AsMatrix()
				return []engine.Value{engine.NewInteger(int64(m.Cols))}, nil
			},
			Returns: []engine.Type{engine.TInteger},
		}},
	},
	{
		Name:              "matrix-size",
		ForwardPrecedence: true,
		Signatures: []engine.NativeSig{{
			Args: []engine.Type{engine.TMatrix},
			Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
				m := args[0].AsMatrix()
				return []engine.Value{engine.NewInteger(int64(m.Rows * m.Cols))}, nil
			},
			Returns: []engine.Type{engine.TList},
		}},
	},
	// Access.
	{
		Name:              "matrix-at",
		ForwardPrecedence: true,
		Signatures: []engine.NativeSig{{
			// Data-last: [col, row, mat]. Under §1.4 stack-top-first matching,
			// `mat row col matrix-at` binds sig[0]=col (top), sig[1]=row,
			// sig[2]=mat (deepest).
			Args: []engine.Type{engine.TInteger, engine.TInteger, engine.TMatrix},
			Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
				c64, err := args[0].AsConcreteInteger()
				if err != nil {
					return nil, err
				}
				r64, err := args[1].AsConcreteInteger()
				if err != nil {
					return nil, err
				}
				m := args[2].AsMatrix()
				row, col := int(r64), int(c64)
				if row < 0 || row >= m.Rows || col < 0 || col >= m.Cols {
					return nil, fmt.Errorf("at: index (%d,%d) out of bounds for %dx%d matrix", row, col, m.Rows, m.Cols)
				}
				return []engine.Value{engine.NewDecimal(m.Data[row*m.Cols+col])}, nil
			},
			Returns: []engine.Type{engine.TDecimal},
		}},
	},
	{
		Name:              "matrix-row",
		ForwardPrecedence: true,
		Signatures: []engine.NativeSig{{
			Args: []engine.Type{engine.TInteger, engine.TMatrix},
			Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
				r64, err := args[0].AsConcreteInteger()
				if err != nil {
					return nil, err
				}
				m := args[1].AsMatrix()
				row := int(r64)
				if row < 0 || row >= m.Rows {
					return nil, fmt.Errorf("row: index %d out of bounds for %d rows", row, m.Rows)
				}
				elems := make([]engine.Value, m.Cols)
				for j := 0; j < m.Cols; j++ {
					elems[j] = engine.NewDecimal(m.Data[row*m.Cols+j])
				}
				return []engine.Value{engine.NewList(elems)}, nil
			},
			Returns: []engine.Type{engine.TList},
		}},
	},
	{
		Name:              "matrix-col",
		ForwardPrecedence: true,
		Signatures: []engine.NativeSig{{
			Args: []engine.Type{engine.TInteger, engine.TMatrix},
			Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
				c64, err := args[0].AsConcreteInteger()
				if err != nil {
					return nil, err
				}
				m := args[1].AsMatrix()
				col := int(c64)
				if col < 0 || col >= m.Cols {
					return nil, fmt.Errorf("col: index %d out of bounds for %d cols", col, m.Cols)
				}
				elems := make([]engine.Value, m.Rows)
				for i := 0; i < m.Rows; i++ {
					elems[i] = engine.NewDecimal(m.Data[i*m.Cols+col])
				}
				return []engine.Value{engine.NewList(elems)}, nil
			},
			Returns: []engine.Type{engine.TList},
		}},
	},
	// Arithmetic.
	{
		Name:              "matrix-mat-add",
		ForwardPrecedence: true,
		Signatures: []engine.NativeSig{{
			Args: []engine.Type{engine.TMatrix, engine.TMatrix},
			Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
				a := args[0].AsMatrix()
				b := args[1].AsMatrix()
				if a.Rows != b.Rows || a.Cols != b.Cols {
					return nil, fmt.Errorf("mat-add: dimension mismatch %dx%d vs %dx%d", a.Rows, a.Cols, b.Rows, b.Cols)
				}
				data := make([]float64, len(a.Data))
				for i := range data {
					data[i] = a.Data[i] + b.Data[i]
				}
				return []engine.Value{engine.NewMatrix(engine.MatrixData{Data: data, Rows: a.Rows, Cols: a.Cols})}, nil
			},
			Returns: []engine.Type{engine.TMatrix},
		}},
	},
	{
		// a b mat-sub → args[0]=b (top), args[1]=a
		Name:              "matrix-mat-sub",
		ForwardPrecedence: true,
		Signatures: []engine.NativeSig{{
			Args: []engine.Type{engine.TMatrix, engine.TMatrix},
			Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
				a := args[1].AsMatrix()
				b := args[0].AsMatrix()
				if a.Rows != b.Rows || a.Cols != b.Cols {
					return nil, fmt.Errorf("mat-sub: dimension mismatch %dx%d vs %dx%d", a.Rows, a.Cols, b.Rows, b.Cols)
				}
				data := make([]float64, len(a.Data))
				for i := range data {
					data[i] = a.Data[i] - b.Data[i]
				}
				return []engine.Value{engine.NewMatrix(engine.MatrixData{Data: data, Rows: a.Rows, Cols: a.Cols})}, nil
			},
			Returns: []engine.Type{engine.TMatrix},
		}},
	},
	{
		// a b mat-mul → args[0]=b (top), args[1]=a
		Name:              "matrix-mat-mul",
		ForwardPrecedence: true,
		Signatures: []engine.NativeSig{{
			Args: []engine.Type{engine.TMatrix, engine.TMatrix},
			Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
				a := args[1].AsMatrix()
				b := args[0].AsMatrix()
				result, err := matMul(a, b)
				if err != nil {
					return nil, err
				}
				return []engine.Value{engine.NewMatrix(result)}, nil
			},
			Returns: []engine.Type{engine.TMatrix},
		}},
	},
	{
		Name:              "matrix-scale",
		ForwardPrecedence: true,
		Signatures: []engine.NativeSig{{
			Args: []engine.Type{engine.TNumber, engine.TMatrix},
			Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
				s, err := args[0].AsNumber()
				if err != nil {
					return nil, err
				}
				m := args[1].AsMatrix()
				data := make([]float64, len(m.Data))
				for i := range data {
					data[i] = m.Data[i] * s
				}
				return []engine.Value{engine.NewMatrix(engine.MatrixData{Data: data, Rows: m.Rows, Cols: m.Cols})}, nil
			},
			Returns: []engine.Type{engine.TMatrix},
		}},
	},
	{
		Name:              "matrix-mat-emul",
		ForwardPrecedence: true,
		Signatures: []engine.NativeSig{{
			Args: []engine.Type{engine.TMatrix, engine.TMatrix},
			Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
				a := args[0].AsMatrix()
				b := args[1].AsMatrix()
				if a.Rows != b.Rows || a.Cols != b.Cols {
					return nil, fmt.Errorf("mat-emul: dimension mismatch %dx%d vs %dx%d", a.Rows, a.Cols, b.Rows, b.Cols)
				}
				data := make([]float64, len(a.Data))
				for i := range data {
					data[i] = a.Data[i] * b.Data[i]
				}
				return []engine.Value{engine.NewMatrix(engine.MatrixData{Data: data, Rows: a.Rows, Cols: a.Cols})}, nil
			},
			Returns: []engine.Type{engine.TMatrix},
		}},
	},
	// Transform.
	{
		Name:              "matrix-transpose",
		ForwardPrecedence: true,
		Signatures: []engine.NativeSig{{
			Args: []engine.Type{engine.TMatrix},
			Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
				m := args[0].AsMatrix()
				data := make([]float64, len(m.Data))
				for i := 0; i < m.Rows; i++ {
					for j := 0; j < m.Cols; j++ {
						data[j*m.Rows+i] = m.Data[i*m.Cols+j]
					}
				}
				return []engine.Value{engine.NewMatrix(engine.MatrixData{Data: data, Rows: m.Cols, Cols: m.Rows})}, nil
			},
			Returns: []engine.Type{engine.TMatrix},
		}},
	},
	{
		Name:              "matrix-flatten",
		ForwardPrecedence: true,
		Signatures: []engine.NativeSig{{
			Args: []engine.Type{engine.TMatrix},
			Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
				m := args[0].AsMatrix()
				elems := make([]engine.Value, len(m.Data))
				for i, v := range m.Data {
					elems[i] = engine.NewDecimal(v)
				}
				return []engine.Value{engine.NewList(elems)}, nil
			},
			Returns: []engine.Type{engine.TList},
		}},
	},
	// Aggregation.
	{
		Name:              "matrix-sum",
		ForwardPrecedence: true,
		Signatures: []engine.NativeSig{{
			Args: []engine.Type{engine.TMatrix},
			Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
				m := args[0].AsMatrix()
				s := 0.0
				for _, v := range m.Data {
					s += v
				}
				return []engine.Value{engine.NewDecimal(s)}, nil
			},
			Returns: []engine.Type{engine.TDecimal},
		}},
	},
	{
		Name:              "matrix-trace",
		ForwardPrecedence: true,
		Signatures: []engine.NativeSig{{
			Args: []engine.Type{engine.TMatrix},
			Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
				m := args[0].AsMatrix()
				if m.Rows != m.Cols {
					return nil, fmt.Errorf("trace: not square (%dx%d)", m.Rows, m.Cols)
				}
				s := 0.0
				for i := 0; i < m.Rows; i++ {
					s += m.Data[i*m.Cols+i]
				}
				return []engine.Value{engine.NewDecimal(s)}, nil
			},
			Returns: []engine.Type{engine.TDecimal},
		}},
	},
	{
		Name:              "matrix-det",
		ForwardPrecedence: true,
		Signatures: []engine.NativeSig{{
			Args: []engine.Type{engine.TMatrix},
			Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
				m := args[0].AsMatrix()
				d, err := matDet(m)
				if err != nil {
					return nil, err
				}
				return []engine.Value{engine.NewDecimal(d)}, nil
			},
			Returns: []engine.Type{engine.TDecimal},
		}},
	},
	// Vector.
	{
		Name:              "matrix-dot",
		ForwardPrecedence: true,
		Signatures: []engine.NativeSig{{
			Args: []engine.Type{engine.TList, engine.TList},
			Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
				a := args[0].AsList()
				b := args[1].AsList()
				if a.IsNil() || b.IsNil() {
					return nil, fmt.Errorf("dot: expected two lists")
				}
				if a.Len() != b.Len() {
					return nil, fmt.Errorf("dot: length mismatch %d vs %d", a.Len(), b.Len())
				}
				s := 0.0
				for i := 0; i < a.Len(); i++ {
					av, err := a.Get(i).AsNumber()
					if err != nil {
						return nil, err
					}
					bv, err := b.Get(i).AsNumber()
					if err != nil {
						return nil, err
					}
					s += av * bv
				}
				return []engine.Value{engine.NewDecimal(s)}, nil
			},
			Returns: []engine.Type{engine.TDecimal},
		}},
	},
}

// --- Internal helpers ---

func matMul(a, b engine.MatrixData) (engine.MatrixData, error) {
	if a.Cols != b.Rows {
		return engine.MatrixData{}, fmt.Errorf("mat-mul: dimension mismatch %dx%d * %dx%d", a.Rows, a.Cols, b.Rows, b.Cols)
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
	return engine.MatrixData{Data: result, Rows: a.Rows, Cols: b.Cols}, nil
}

func matDet(m engine.MatrixData) (float64, error) {
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
