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

	registerAllMatrixWords(subReg)

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

// --- Word registration ---

func registerAllMatrixWords(r *engine.Registry) {
	registerMatrixMake(r)
	registerMatrixZeros(r)
	registerMatrixOnes(r)
	registerMatrixEye(r)
	registerMatrixFill(r)

	registerMatrixRows(r)
	registerMatrixCols(r)
	registerMatrixSize(r)

	registerMatrixAt(r)
	registerMatrixRow(r)
	registerMatrixCol(r)

	registerMatrixAdd(r)
	registerMatrixSub(r)
	registerMatrixMul(r)
	registerMatrixScale(r)
	registerMatrixEmul(r)

	registerMatrixTranspose(r)
	registerMatrixFlatten(r)

	registerMatrixSum(r)
	registerMatrixTrace(r)
	registerMatrixDet(r)

	registerMatrixDot(r)
}

// --- Construction ---

func registerMatrixMake(r *engine.Registry) {
	r.Register("matrix-make", engine.Signature{
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
			// Determine cols from first row
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
	})
}

func registerMatrixZeros(r *engine.Registry) {
	// rows cols zeros → args[0]=cols (top), args[1]=rows (deeper)
	r.Register("matrix-zeros", engine.Signature{
		Args: []engine.Type{engine.TInteger, engine.TInteger},
		Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
			r64, err := args[1].AsInteger()
			if err != nil {
				return nil, err
			}
			c64, err := args[0].AsInteger()
			if err != nil {
				return nil, err
			}
			rows, cols := int(r64), int(c64)
			data := make([]float64, rows*cols)
			return []engine.Value{engine.NewMatrix(engine.MatrixData{Data: data, Rows: rows, Cols: cols})}, nil
		},
		Returns: []engine.Type{engine.TMatrix},
	})
}

func registerMatrixOnes(r *engine.Registry) {
	// rows cols ones → args[0]=cols (top), args[1]=rows (deeper)
	r.Register("matrix-ones", engine.Signature{
		Args: []engine.Type{engine.TInteger, engine.TInteger},
		Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
			r64, err := args[1].AsInteger()
			if err != nil {
				return nil, err
			}
			c64, err := args[0].AsInteger()
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
	})
}

func registerMatrixEye(r *engine.Registry) {
	r.Register("matrix-eye", engine.Signature{
		Args: []engine.Type{engine.TInteger},
		Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
			n64, err := args[0].AsInteger()
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
	})
}

func registerMatrixFill(r *engine.Registry) {
	r.Register("matrix-fill", engine.Signature{
		Args: []engine.Type{engine.TInteger, engine.TInteger, engine.TNumber},
		Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
			r64, err := args[0].AsInteger()
			if err != nil {
				return nil, err
			}
			c64, err := args[1].AsInteger()
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
	})
}

// --- Shape ---

func registerMatrixRows(r *engine.Registry) {
	r.Register("matrix-rows", engine.Signature{
		Args: []engine.Type{engine.TMatrix},
		Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
			m := args[0].AsMatrix()
			return []engine.Value{engine.NewInteger(int64(m.Rows))}, nil
		},
		Returns: []engine.Type{engine.TInteger},
	})
}

func registerMatrixCols(r *engine.Registry) {
	r.Register("matrix-cols", engine.Signature{
		Args: []engine.Type{engine.TMatrix},
		Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
			m := args[0].AsMatrix()
			return []engine.Value{engine.NewInteger(int64(m.Cols))}, nil
		},
		Returns: []engine.Type{engine.TInteger},
	})
}

func registerMatrixSize(r *engine.Registry) {
	r.Register("matrix-size", engine.Signature{
		Args: []engine.Type{engine.TMatrix},
		Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
			m := args[0].AsMatrix()
			return []engine.Value{engine.NewInteger(int64(m.Rows * m.Cols))}, nil
		},
		Returns: []engine.Type{engine.TList},
	})
}

// --- Access ---

func registerMatrixAt(r *engine.Registry) {
	r.Register("matrix-at", engine.Signature{
		Args: []engine.Type{engine.TMatrix, engine.TInteger, engine.TInteger},
		Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
			m := args[0].AsMatrix()
			r64, err := args[1].AsInteger()
			if err != nil {
				return nil, err
			}
			c64, err := args[2].AsInteger()
			if err != nil {
				return nil, err
			}
			row, col := int(r64), int(c64)
			if row < 0 || row >= m.Rows || col < 0 || col >= m.Cols {
				return nil, fmt.Errorf("at: index (%d,%d) out of bounds for %dx%d matrix", row, col, m.Rows, m.Cols)
			}
			return []engine.Value{engine.NewDecimal(m.Data[row*m.Cols+col])}, nil
		},
		Returns: []engine.Type{engine.TDecimal},
	})
}

func registerMatrixRow(r *engine.Registry) {
	r.Register("matrix-row", engine.Signature{
		Args: []engine.Type{engine.TMatrix, engine.TInteger},
		Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
			m := args[0].AsMatrix()
			r64, err := args[1].AsInteger()
			if err != nil {
				return nil, err
			}
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
	})
}

func registerMatrixCol(r *engine.Registry) {
	r.Register("matrix-col", engine.Signature{
		Args: []engine.Type{engine.TMatrix, engine.TInteger},
		Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
			m := args[0].AsMatrix()
			c64, err := args[1].AsInteger()
			if err != nil {
				return nil, err
			}
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
	})
}

// --- Arithmetic ---

func registerMatrixAdd(r *engine.Registry) {
	r.Register("matrix-mat-add", engine.Signature{
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
	})
}

func registerMatrixSub(r *engine.Registry) {
	// a b mat-sub → args[0]=b (top), args[1]=a
	r.Register("matrix-mat-sub", engine.Signature{
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
	})
}

func registerMatrixMul(r *engine.Registry) {
	// a b mat-mul → args[0]=b (top), args[1]=a
	r.Register("matrix-mat-mul", engine.Signature{
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
	})
}

func registerMatrixScale(r *engine.Registry) {
	r.Register("matrix-scale", engine.Signature{
		Args: []engine.Type{engine.TMatrix, engine.TNumber},
		Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
			m := args[0].AsMatrix()
			s, err := args[1].AsNumber()
			if err != nil {
				return nil, err
			}
			data := make([]float64, len(m.Data))
			for i := range data {
				data[i] = m.Data[i] * s
			}
			return []engine.Value{engine.NewMatrix(engine.MatrixData{Data: data, Rows: m.Rows, Cols: m.Cols})}, nil
		},
		Returns: []engine.Type{engine.TMatrix},
	})
}

func registerMatrixEmul(r *engine.Registry) {
	r.Register("matrix-mat-emul", engine.Signature{
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
	})
}

// --- Transform ---

func registerMatrixTranspose(r *engine.Registry) {
	r.Register("matrix-transpose", engine.Signature{
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
	})
}

func registerMatrixFlatten(r *engine.Registry) {
	r.Register("matrix-flatten", engine.Signature{
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
	})
}

// --- Aggregation ---

func registerMatrixSum(r *engine.Registry) {
	r.Register("matrix-sum", engine.Signature{
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
	})
}

func registerMatrixTrace(r *engine.Registry) {
	r.Register("matrix-trace", engine.Signature{
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
	})
}

func registerMatrixDet(r *engine.Registry) {
	r.Register("matrix-det", engine.Signature{
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
	})
}

// --- Vector ---

func registerMatrixDot(r *engine.Registry) {
	r.Register("matrix-dot", engine.Signature{
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
	})
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
