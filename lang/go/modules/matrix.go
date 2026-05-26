package modules

import (
	"fmt"
	"math"

	"github.com/aql-lang/aql/eng/go"
	"github.com/aql-lang/aql/lang/go/native"
)

// TTensor, TMatrix and TVector are the Node/Tensor[/Matrix | /Vector]
// type identities. They live under Node, the container family — a
// tensor is a structured collection, not a scalar — which also keeps a
// bare Matrix / Vector type literal from matching `make`'s scalar-cast
// overload. Matrix and Vector are lattice children of Tensor — a
// Matrix is a rank-2 tensor, a Vector a rank-1 tensor — so `is Tensor`
// holds for both. The matrix native module owns them; the
// var initialiser below registers them into the global eng.Builtin
// table so any package-level var referencing them (MatrixNatives,
// whose signatures embed TMatrix) sees non-nil pointers at slice-init
// time — Go resolves var-init dependencies before declaration order.
//
// FixedIDs 2000-2002 come from the documented matrix-module range
// (2000-2999); TMatrix keeps its historical 2000 so serialised Value
// IDs stay wire-compatible.
var TTensor, TMatrix, TVector = registerTensorTypes()

func registerTensorTypes() (*eng.Type, *eng.Type, *eng.Type) {
	// Tensor first — Matrix and Vector register as its lattice
	// children and so need it present in eng.Builtin.
	tensor, err := eng.Builtin.RegisterExternalBuiltin("Ideal/Tensor", 2001, tensorFormatBehavior{})
	if err != nil {
		// lint:allow-panic — init-time builtin registration; see
		// registerTimerType in native/native_misc.go for rationale.
		panic(fmt.Sprintf("matrix: register TTensor: %v", err))
	}
	// Tensor inherits Ideal's unified Rank from RegisterExternalBuiltin
	// (external types take no positional slot — see builtinDecls in
	// eng/go/typetable.go); Matrix and Vector inherit it in turn.
	matrix, err := eng.Builtin.RegisterExternalBuiltin("Ideal/Tensor/Matrix", 2000, tensorFormatBehavior{})
	if err != nil {
		// lint:allow-panic — see above.
		panic(fmt.Sprintf("matrix: register TMatrix: %v", err))
	}
	vector, err := eng.Builtin.RegisterExternalBuiltin("Ideal/Tensor/Vector", 2002, tensorFormatBehavior{})
	if err != nil {
		// lint:allow-panic — see above.
		panic(fmt.Sprintf("matrix: register TVector: %v", err))
	}
	return tensor, matrix, vector
}

// TensorData is a concrete tensor value: a dense float64 array stored
// in row-major (C-contiguous) order with an explicit shape. The rank
// is len(Shape) — a Matrix is a rank-2 TensorData, a Vector rank-1.
// It replaces the former kernel MatrixData{Data,Rows,Cols} payload:
// the tensor types now carry their payload through ExtensionPayload,
// the host escape hatch, rather than a dedicated kernel variant.
type TensorData struct {
	Shape []int
	Data  []float64
}

// Rank reports the number of dimensions.
func (t TensorData) Rank() int { return len(t.Shape) }

// Rows reports the first dimension, or 0 for a rank-0 tensor.
func (t TensorData) Rows() int {
	if len(t.Shape) > 0 {
		return t.Shape[0]
	}
	return 0
}

// Cols reports the second dimension, or 0 for rank < 2.
func (t TensorData) Cols() int {
	if len(t.Shape) > 1 {
		return t.Shape[1]
	}
	return 0
}

// tensorFormatBehavior renders a tensor as "Kind(d0xd1x…)" —
// "Matrix(2x3)", "Vector(3)", "Tensor(2x3x4)". Shared by all three
// tensor types; Match and Equal defer to the kernel default. Replaces
// the former kernel-installed matrix Behavior.
type tensorFormatBehavior struct{}

func (tensorFormatBehavior) Match(v native.Value, t *native.Type) bool {
	return native.DefaultBehavior.Match(v, t)
}

func (tensorFormatBehavior) Equal(a, b native.Value) bool {
	return native.DefaultBehavior.Equal(a, b)
}

func (tensorFormatBehavior) Format(v native.Value) string {
	kind := tensorKindName(v.Parent)
	if td, ok := tensorPayload(v); ok {
		return kind + "(" + shapeString(td.Shape) + ")"
	}
	return kind
}

// Size of a tensor is its entry count — the number of scalars in the
// dense array, so a 3x3 Matrix sizes to 9 and a length-5 Vector to 5.
// This satisfies the kernel's eng.Sizer capability, which SizeOf (the
// `size` word) consults.
func (tensorFormatBehavior) Size(v native.Value) int {
	return len(AsTensor(v).Data)
}

// tensorKindName names the tensor kind a type belongs to.
func tensorKindName(vt *eng.Type) string {
	switch {
	case vt == nil:
		return "Tensor"
	case vt.Matches(TVector):
		return "Vector"
	case vt.Matches(TMatrix):
		return "Matrix"
	default:
		return "Tensor"
	}
}

// shapeString renders a shape as "d0xd1x…".
func shapeString(shape []int) string {
	if len(shape) == 0 {
		return "scalar"
	}
	out := ""
	for i, d := range shape {
		if i > 0 {
			out += "x"
		}
		out += fmt.Sprintf("%d", d)
	}
	return out
}

// tensorPayload extracts the TensorData payload from a tensor value.
func tensorPayload(v native.Value) (TensorData, bool) {
	if ep, ok := v.Data.(eng.ExtensionPayload); ok {
		td, ok := ep.Body.(TensorData)
		return td, ok
	}
	return TensorData{}, false
}

// AsTensor extracts the TensorData payload from a tensor value,
// returning the zero TensorData when v carries no tensor payload.
func AsTensor(v native.Value) TensorData {
	td, _ := tensorPayload(v)
	return td
}

// tensorValue wraps a TensorData as a value of the given tensor type.
func tensorValue(vt *eng.Type, td TensorData) native.Value {
	return eng.NewExtension(vt, td)
}

// newMatrix builds a rank-2 tensor — a Matrix value.
func newMatrix(rows, cols int, data []float64) native.Value {
	return tensorValue(TMatrix, TensorData{Shape: []int{rows, cols}, Data: data})
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

	// Install the Tensor/Matrix/Vector type-kinds into the importing
	// registry so `type` constructs and `make` instantiates them.
	registerTensorIdeals(parent)

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
			Body:    []native.Value{native.NewWord(wordName)}, BarrierPos: -1,
		}},
		Registry: subReg,
	})
}

func makeIntIntToMatrixFnDef(wordName string, subReg *native.Registry) native.Value {
	return native.NewFnDef(native.FnDefInfo{Name: wordName,
		Sigs: []native.FnSig{{
			Params:  []native.FnParam{{Type: native.TInteger}, {Type: native.TInteger}},
			Returns: []*native.Type{TMatrix},
			Body:    []native.Value{native.NewWord(wordName)}, BarrierPos: -1,
		}},
		Registry: subReg,
	})
}

func makeIntToMatrixFnDef(wordName string, subReg *native.Registry) native.Value {
	return native.NewFnDef(native.FnDefInfo{Name: wordName,
		Sigs: []native.FnSig{{
			Params:  []native.FnParam{{Type: native.TInteger}},
			Returns: []*native.Type{TMatrix},
			Body:    []native.Value{native.NewWord(wordName)}, BarrierPos: -1,
		}},
		Registry: subReg,
	})
}

func makeIntIntNumToMatrixFnDef(wordName string, subReg *native.Registry) native.Value {
	return native.NewFnDef(native.FnDefInfo{Name: wordName,
		Sigs: []native.FnSig{{
			Params:  []native.FnParam{{Type: native.TInteger}, {Type: native.TInteger}, {Type: native.TNumber}},
			Returns: []*native.Type{TMatrix},
			Body:    []native.Value{native.NewWord(wordName)}, BarrierPos: -1,
		}},
		Registry: subReg,
	})
}

func makeMatrixToIntFnDef(wordName string, subReg *native.Registry) native.Value {
	return native.NewFnDef(native.FnDefInfo{Name: wordName,
		Sigs: []native.FnSig{{
			Params:  []native.FnParam{{Type: TMatrix}},
			Returns: []*native.Type{native.TInteger},
			Body:    []native.Value{native.NewWord(wordName)}, BarrierPos: -1,
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
			Body:    []native.Value{native.NewWord(wordName)}, BarrierPos: -1,
		}},
		Registry: subReg,
	})
}

func makeMatrixIntToListFnDef(wordName string, subReg *native.Registry) native.Value {
	return native.NewFnDef(native.FnDefInfo{Name: wordName,
		Sigs: []native.FnSig{{
			Params:  []native.FnParam{{Type: TMatrix}, {Type: native.TInteger}},
			Returns: []*native.Type{native.TList},
			Body:    []native.Value{native.NewWord(wordName)}, BarrierPos: -1,
		}},
		Registry: subReg,
	})
}

func makeMatrixMatrixToMatrixFnDef(wordName string, subReg *native.Registry) native.Value {
	return native.NewFnDef(native.FnDefInfo{Name: wordName,
		Sigs: []native.FnSig{{
			Params:  []native.FnParam{{Type: TMatrix}, {Type: TMatrix}},
			Returns: []*native.Type{TMatrix},
			Body:    []native.Value{native.NewWord(wordName)}, BarrierPos: -1,
		}},
		Registry: subReg,
	})
}

func makeMatrixNumToMatrixFnDef(wordName string, subReg *native.Registry) native.Value {
	return native.NewFnDef(native.FnDefInfo{Name: wordName,
		Sigs: []native.FnSig{{
			Params:  []native.FnParam{{Type: TMatrix}, {Type: native.TNumber}},
			Returns: []*native.Type{TMatrix},
			Body:    []native.Value{native.NewWord(wordName)}, BarrierPos: -1,
		}},
		Registry: subReg,
	})
}

func makeUnaryMatrixFnDef(wordName string, subReg *native.Registry) native.Value {
	return native.NewFnDef(native.FnDefInfo{Name: wordName,
		Sigs: []native.FnSig{{
			Params:  []native.FnParam{{Type: TMatrix}},
			Returns: []*native.Type{TMatrix},
			Body:    []native.Value{native.NewWord(wordName)}, BarrierPos: -1,
		}},
		Registry: subReg,
	})
}

func makeMatrixToListFnDef(wordName string, subReg *native.Registry) native.Value {
	return native.NewFnDef(native.FnDefInfo{Name: wordName,
		Sigs: []native.FnSig{{
			Params:  []native.FnParam{{Type: TMatrix}},
			Returns: []*native.Type{native.TList},
			Body:    []native.Value{native.NewWord(wordName)}, BarrierPos: -1,
		}},
		Registry: subReg,
	})
}

func makeMatrixToDecFnDef(wordName string, subReg *native.Registry) native.Value {
	return native.NewFnDef(native.FnDefInfo{Name: wordName,
		Sigs: []native.FnSig{{
			Params:  []native.FnParam{{Type: TMatrix}},
			Returns: []*native.Type{native.TDecimal},
			Body:    []native.Value{native.NewWord(wordName)}, BarrierPos: -1,
		}},
		Registry: subReg,
	})
}

func makeListListToDecFnDef(wordName string, subReg *native.Registry) native.Value {
	return native.NewFnDef(native.FnDefInfo{Name: wordName,
		Sigs: []native.FnSig{{
			Params:  []native.FnParam{{Type: native.TList}, {Type: native.TList}},
			Returns: []*native.Type{native.TDecimal},
			Body:    []native.Value{native.NewWord(wordName)}, BarrierPos: -1,
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
				td, err := matrixFromRows(args[0])
				if err != nil {
					return nil, err
				}
				return []native.Value{tensorValue(TMatrix, td)}, nil
			},
			Returns: []*native.Type{TMatrix}, BarrierPos: -1,
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
				return []native.Value{newMatrix(rows, cols, data)}, nil
			},
			Returns: []*native.Type{TMatrix}, BarrierPos: -1,
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
				return []native.Value{newMatrix(rows, cols, data)}, nil
			},
			Returns: []*native.Type{TMatrix}, BarrierPos: -1,
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
				return []native.Value{newMatrix(n, n, data)}, nil
			},
			Returns: []*native.Type{TMatrix}, BarrierPos: -1,
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
				return []native.Value{newMatrix(rows, cols, data)}, nil
			},
			Returns: []*native.Type{TMatrix}, BarrierPos: -1,
		}},
	},
	// Shape.
	{
		Name:        "matrix-rows",
		ForwardArgs: true,
		Signatures: []native.NativeSig{{
			Args: []*native.Type{TMatrix},
			Handler: func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
				m := AsTensor(args[0])
				return []native.Value{native.NewInteger(int64(m.Rows()))}, nil
			},
			Returns: []*native.Type{native.TInteger}, BarrierPos: -1,
		}},
	},
	{
		Name:        "matrix-cols",
		ForwardArgs: true,
		Signatures: []native.NativeSig{{
			Args: []*native.Type{TMatrix},
			Handler: func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
				m := AsTensor(args[0])
				return []native.Value{native.NewInteger(int64(m.Cols()))}, nil
			},
			Returns: []*native.Type{native.TInteger}, BarrierPos: -1,
		}},
	},
	{
		Name:        "matrix-size",
		ForwardArgs: true,
		Signatures: []native.NativeSig{{
			Args: []*native.Type{TMatrix},
			Handler: func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
				m := AsTensor(args[0])
				return []native.Value{native.NewInteger(int64(m.Rows() * m.Cols()))}, nil
			},
			Returns: []*native.Type{native.TList}, BarrierPos: -1,
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
				m := AsTensor(args[2])
				row, col := int(r64), int(c64)
				if row < 0 || row >= m.Rows() || col < 0 || col >= m.Cols() {
					return nil, fmt.Errorf("at: index (%d,%d) out of bounds for %dx%d matrix", row, col, m.Rows(), m.Cols())
				}
				return []native.Value{native.NewDecimal(m.Data[row*m.Cols()+col])}, nil
			},
			Returns: []*native.Type{native.TDecimal}, BarrierPos: -1,
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
				m := AsTensor(args[1])
				row := int(r64)
				if row < 0 || row >= m.Rows() {
					return nil, fmt.Errorf("row: index %d out of bounds for %d rows", row, m.Rows())
				}
				elems := make([]native.Value, m.Cols())
				for j := 0; j < m.Cols(); j++ {
					elems[j] = native.NewDecimal(m.Data[row*m.Cols()+j])
				}
				return []native.Value{native.NewList(elems)}, nil
			},
			Returns: []*native.Type{native.TList}, BarrierPos: -1,
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
				m := AsTensor(args[1])
				col := int(c64)
				if col < 0 || col >= m.Cols() {
					return nil, fmt.Errorf("col: index %d out of bounds for %d cols", col, m.Cols())
				}
				elems := make([]native.Value, m.Rows())
				for i := 0; i < m.Rows(); i++ {
					elems[i] = native.NewDecimal(m.Data[i*m.Cols()+col])
				}
				return []native.Value{native.NewList(elems)}, nil
			},
			Returns: []*native.Type{native.TList}, BarrierPos: -1,
		}},
	},
	// Arithmetic.
	{
		Name:        "matrix-mat-add",
		ForwardArgs: true,
		Signatures: []native.NativeSig{{
			Args: []*native.Type{TMatrix, TMatrix},
			Handler: func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
				a := AsTensor(args[0])
				b := AsTensor(args[1])
				if a.Rows() != b.Rows() || a.Cols() != b.Cols() {
					return nil, fmt.Errorf("mat-add: dimension mismatch %dx%d vs %dx%d", a.Rows(), a.Cols(), b.Rows(), b.Cols())
				}
				data := make([]float64, len(a.Data))
				for i := range data {
					data[i] = a.Data[i] + b.Data[i]
				}
				return []native.Value{newMatrix(a.Rows(), a.Cols(), data)}, nil
			},
			Returns: []*native.Type{TMatrix}, BarrierPos: -1,
		}},
	},
	{
		// a b mat-sub → args[0]=b (top), args[1]=a
		Name:        "matrix-mat-sub",
		ForwardArgs: true,
		Signatures: []native.NativeSig{{
			Args: []*native.Type{TMatrix, TMatrix},
			Handler: func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
				a := AsTensor(args[1])
				b := AsTensor(args[0])
				if a.Rows() != b.Rows() || a.Cols() != b.Cols() {
					return nil, fmt.Errorf("mat-sub: dimension mismatch %dx%d vs %dx%d", a.Rows(), a.Cols(), b.Rows(), b.Cols())
				}
				data := make([]float64, len(a.Data))
				for i := range data {
					data[i] = a.Data[i] - b.Data[i]
				}
				return []native.Value{newMatrix(a.Rows(), a.Cols(), data)}, nil
			},
			Returns: []*native.Type{TMatrix}, BarrierPos: -1,
		}},
	},
	{
		// a b mat-mul → args[0]=b (top), args[1]=a
		Name:        "matrix-mat-mul",
		ForwardArgs: true,
		Signatures: []native.NativeSig{{
			Args: []*native.Type{TMatrix, TMatrix},
			Handler: func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
				a := AsTensor(args[1])
				b := AsTensor(args[0])
				result, err := matMul(a, b)
				if err != nil {
					return nil, err
				}
				return []native.Value{tensorValue(TMatrix, result)}, nil
			},
			Returns: []*native.Type{TMatrix}, BarrierPos: -1,
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
				m := AsTensor(args[1])
				data := make([]float64, len(m.Data))
				for i := range data {
					data[i] = m.Data[i] * s
				}
				return []native.Value{newMatrix(m.Rows(), m.Cols(), data)}, nil
			},
			Returns: []*native.Type{TMatrix}, BarrierPos: -1,
		}},
	},
	{
		Name:        "matrix-mat-emul",
		ForwardArgs: true,
		Signatures: []native.NativeSig{{
			Args: []*native.Type{TMatrix, TMatrix},
			Handler: func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
				a := AsTensor(args[0])
				b := AsTensor(args[1])
				if a.Rows() != b.Rows() || a.Cols() != b.Cols() {
					return nil, fmt.Errorf("mat-emul: dimension mismatch %dx%d vs %dx%d", a.Rows(), a.Cols(), b.Rows(), b.Cols())
				}
				data := make([]float64, len(a.Data))
				for i := range data {
					data[i] = a.Data[i] * b.Data[i]
				}
				return []native.Value{newMatrix(a.Rows(), a.Cols(), data)}, nil
			},
			Returns: []*native.Type{TMatrix}, BarrierPos: -1,
		}},
	},
	// Transform.
	{
		Name:        "matrix-transpose",
		ForwardArgs: true,
		Signatures: []native.NativeSig{{
			Args: []*native.Type{TMatrix},
			Handler: func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
				m := AsTensor(args[0])
				data := make([]float64, len(m.Data))
				for i := 0; i < m.Rows(); i++ {
					for j := 0; j < m.Cols(); j++ {
						data[j*m.Rows()+i] = m.Data[i*m.Cols()+j]
					}
				}
				return []native.Value{newMatrix(m.Cols(), m.Rows(), data)}, nil
			},
			Returns: []*native.Type{TMatrix}, BarrierPos: -1,
		}},
	},
	{
		Name:        "matrix-flatten",
		ForwardArgs: true,
		Signatures: []native.NativeSig{{
			Args: []*native.Type{TMatrix},
			Handler: func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
				m := AsTensor(args[0])
				elems := make([]native.Value, len(m.Data))
				for i, v := range m.Data {
					elems[i] = native.NewDecimal(v)
				}
				return []native.Value{native.NewList(elems)}, nil
			},
			Returns: []*native.Type{native.TList}, BarrierPos: -1,
		}},
	},
	// Aggregation.
	{
		Name:        "matrix-sum",
		ForwardArgs: true,
		Signatures: []native.NativeSig{{
			Args: []*native.Type{TMatrix},
			Handler: func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
				m := AsTensor(args[0])
				s := 0.0
				for _, v := range m.Data {
					s += v
				}
				return []native.Value{native.NewDecimal(s)}, nil
			},
			Returns: []*native.Type{native.TDecimal}, BarrierPos: -1,
		}},
	},
	{
		Name:        "matrix-trace",
		ForwardArgs: true,
		Signatures: []native.NativeSig{{
			Args: []*native.Type{TMatrix},
			Handler: func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
				m := AsTensor(args[0])
				if m.Rows() != m.Cols() {
					return nil, fmt.Errorf("trace: not square (%dx%d)", m.Rows(), m.Cols())
				}
				s := 0.0
				for i := 0; i < m.Rows(); i++ {
					s += m.Data[i*m.Cols()+i]
				}
				return []native.Value{native.NewDecimal(s)}, nil
			},
			Returns: []*native.Type{native.TDecimal}, BarrierPos: -1,
		}},
	},
	{
		Name:        "matrix-det",
		ForwardArgs: true,
		Signatures: []native.NativeSig{{
			Args: []*native.Type{TMatrix},
			Handler: func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
				m := AsTensor(args[0])
				d, err := matDet(m)
				if err != nil {
					return nil, err
				}
				return []native.Value{native.NewDecimal(d)}, nil
			},
			Returns: []*native.Type{native.TDecimal}, BarrierPos: -1,
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
			Returns: []*native.Type{native.TDecimal}, BarrierPos: -1,
		}},
	},
}

// --- Internal helpers ---

// matrixFromRows builds a rank-2 TensorData from a list of equal-length
// row lists. Shared by the `matrix-make` word and the Matrix Ideal's
// Instantiate (see matrix_ideal.go) so `matrix.create` and
// `make Matrix …` agree.
func matrixFromRows(v native.Value) (TensorData, error) {
	rl, _ := native.AsList(v)
	if rl.IsNil() {
		return TensorData{}, fmt.Errorf("make: expected list of row lists")
	}
	rows := rl.Len()
	if rows == 0 {
		return TensorData{}, fmt.Errorf("make: empty list")
	}
	firstRow, _ := native.AsList(rl.Get(0))
	if firstRow.IsNil() {
		return TensorData{}, fmt.Errorf("make: first element is not a list")
	}
	cols := firstRow.Len()
	if cols == 0 {
		return TensorData{}, fmt.Errorf("make: first row is empty")
	}
	data := make([]float64, 0, rows*cols)
	for i := 0; i < rows; i++ {
		row, _ := native.AsList(rl.Get(i))
		if row.IsNil() {
			return TensorData{}, fmt.Errorf("make: row %d is not a list", i)
		}
		if row.Len() != cols {
			return TensorData{}, fmt.Errorf("make: row %d has %d elements, expected %d", i, row.Len(), cols)
		}
		for j := 0; j < cols; j++ {
			n, err := native.AsNumber(row.Get(j))
			if err != nil {
				return TensorData{}, err
			}
			data = append(data, n)
		}
	}
	return TensorData{Shape: []int{rows, cols}, Data: data}, nil
}

func matMul(a, b TensorData) (TensorData, error) {
	if a.Cols() != b.Rows() {
		return TensorData{}, fmt.Errorf("mat-mul: dimension mismatch %dx%d * %dx%d", a.Rows(), a.Cols(), b.Rows(), b.Cols())
	}
	rows, cols, inner := a.Rows(), b.Cols(), a.Cols()
	result := make([]float64, rows*cols)
	for i := 0; i < rows; i++ {
		for j := 0; j < cols; j++ {
			sum := 0.0
			for k := 0; k < inner; k++ {
				sum += a.Data[i*inner+k] * b.Data[k*cols+j]
			}
			result[i*cols+j] = sum
		}
	}
	return TensorData{Shape: []int{rows, cols}, Data: result}, nil
}

func matDet(m TensorData) (float64, error) {
	if m.Rows() != m.Cols() {
		return 0, fmt.Errorf("det: not square (%dx%d)", m.Rows(), m.Cols())
	}
	n := m.Rows()
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
