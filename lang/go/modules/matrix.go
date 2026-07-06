package modules

import (
	"fmt"
	"math"

	"github.com/aql-lang/aql/eng/go"
	"github.com/aql-lang/aql/lang/go/native"
)

// TensorModuleTypes are the tensor types owned by aql:matrix-util,
// minted per import into the module's sub-registry by MintTensorTypes
// (the StreamKind / TemporalModuleTypes pattern — see
// design/OPEN-WORDS.0.md). Tensor values escape to the importer, so
// the mints draw IDs from the importing tree's counter —
// BuildMatrixModule adopts the parent's sequence before minting. The
// former global registrations (FixedIDs 2000-2002) are retired; the
// types are reachable after import as MatrixUtil.Tensor /
// MatrixUtil.Matrix / MatrixUtil.Vector, and they are what anchors
// the module's add / sub / mul word extensions under the module-scope
// user-type rule. Matrix and Vector are lattice children of Tensor —
// a Matrix is a rank-2 tensor, a Vector a rank-1 tensor — so
// `is MatrixUtil.Tensor` holds for both.
type TensorModuleTypes struct {
	Tensor *native.Type
	Matrix *native.Type
	Vector *native.Type
}

// MintTensorTypes mints the tensor family into r's type table (r is
// aql:matrix-util's sub-registry) and returns the nodes. Tensor hangs
// under the global Ideal branch — the same lattice position the
// former global registrations occupied.
func MintTensorTypes(r *native.Registry) TensorModuleTypes {
	tensor := r.Types.MintTypeWithBehavior("Tensor", native.TIdeal, tensorFormatBehavior{})
	return TensorModuleTypes{
		Tensor: tensor,
		Matrix: r.Types.MintTypeWithBehavior("Matrix", tensor, tensorFormatBehavior{}),
		Vector: r.Types.MintTypeWithBehavior("Vector", tensor, tensorFormatBehavior{}),
	}
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

// DeepClone satisfies eng.DeepCloner so the universal `clone` word
// produces an independent tensor: the Shape and Data slices are copied
// rather than shared, so mutating the clone cannot touch the original.
func (t TensorData) DeepClone() any {
	shape := make([]int, len(t.Shape))
	copy(shape, t.Shape)
	data := make([]float64, len(t.Data))
	copy(data, t.Data)
	return TensorData{Shape: shape, Data: data}
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

// tensorKindName names the tensor kind a type belongs to — by
// walking the parent chain for the kind names, so it needs no access
// to a particular import's mints (the format Behavior carries no
// state) and user refines (`def UMat refine (MatrixUtil.Matrix)`)
// report their kind through their ancestry.
func tensorKindName(vt *eng.Type) string {
	for t := vt; t != nil; t = t.Parent {
		switch t.Name {
		case "Vector", "Matrix", "Tensor":
			return t.Name
		}
	}
	return "Tensor"
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

// newMatrix builds a rank-2 tensor — a Matrix value of this import's mint.
func (tt TensorModuleTypes) newMatrix(rows, cols int, data []float64) native.Value {
	return tensorValue(tt.Matrix, TensorData{Shape: []int{rows, cols}, Data: data})
}

// maxMatrixElems caps the number of entries a constructor will allocate
// from user-supplied dimensions (~134M float64 ≈ 1 GiB). It bounds the
// memory a single `zeros`/`ones`/`fill`/`eye` call can demand.
const maxMatrixElems = 1 << 27

// validDims rejects dimensions that would make matrix allocation unsafe:
// a negative size (make([]float64, neg) panics), an int overflow in
// rows*cols, or a product beyond the element cap (OOM). Returning an
// error here keeps these user-reachable conditions out of the panicking
// allocation path — see ADR-005.
func validDims(rows, cols int) error {
	if rows < 0 || cols < 0 {
		return fmt.Errorf("matrix: dimensions must be non-negative, got %dx%d", rows, cols)
	}
	// Check the product without overflowing: rows*cols > cap ⇔ cols > cap/rows.
	if rows != 0 && cols > maxMatrixElems/rows {
		return fmt.Errorf("matrix: %dx%d (%d elements) exceeds the cap of %d", rows, cols, rows*cols, maxMatrixElems)
	}
	return nil
}

// BuildMatrixModule creates the "aql:matrix-util" native module. It registers
// Go-implemented matrix words into an isolated sub-registry and returns a
// ModuleDesc with a "matrix" export containing FnDef wrappers for each word.
func BuildMatrixModule(parent *native.Registry) (native.ModuleDesc, error) {
	subReg, err := native.DefaultRegistry()
	if err != nil {
		return native.ModuleDesc{}, err
	}
	// The tensor types escape to the importer (exported type literals,
	// the Parent tag on every tensor value, and the add/sub/mul word-
	// extension signatures), so they draw IDs from the importing tree's
	// counter — see eng TypeTable.mintID and the BuildIOModule /
	// StreamKind precedent.
	subReg.Types.AdoptSeqFrom(parent.Types)
	tt := MintTensorTypes(subReg)

	for _, n := range matrixNatives(tt) {
		subReg.RegisterNativeFunc(n)
	}

	// Install the Tensor/Matrix/Vector type-kinds into the importing
	// registry so `refine` constructs shaped types and `make`
	// instantiates them (via the exported literals — e.g.
	// `make MatrixUtil.Matrix [[1 2][3 4]]`).
	registerTensorIdeals(parent, tt)

	exports := native.NewOrderedMap()

	// Construction
	exports.Set("create", makeTypedFnDef("matrix-make", subReg, tt.Matrix, native.TList))
	exports.Set("zeros", makeTypedFnDef("matrix-zeros", subReg, tt.Matrix, native.TInteger, native.TInteger))
	exports.Set("ones", makeTypedFnDef("matrix-ones", subReg, tt.Matrix, native.TInteger, native.TInteger))
	exports.Set("eye", makeTypedFnDef("matrix-eye", subReg, tt.Matrix, native.TInteger))
	exports.Set("fill", makeTypedFnDef("matrix-fill", subReg, tt.Matrix, native.TInteger, native.TInteger, native.TNumber))

	// Shape. No `size` export: the core `size` word already reports a
	// tensor's entry count via the Sizer behavior (TensorData), so a
	// MatrixUtil.size would only shadow it — see ADR-001.
	exports.Set("rows", makeTypedFnDef("matrix-rows", subReg, native.TInteger, tt.Matrix))
	exports.Set("cols", makeTypedFnDef("matrix-cols", subReg, native.TInteger, tt.Matrix))

	// Access. elem's wrapper Params keep the user-facing positional
	// order `mat row col` (deepest→top) so the wrapper resolves as
	// before; the underlying NativeFunc sig is the reverse (top-down)
	// and is "data-last" by virtue of mat being deepest.
	exports.Set("elem", makeTypedFnDef("matrix-at", subReg, native.TFloat, tt.Matrix, native.TInteger, native.TInteger))
	exports.Set("row", makeTypedFnDef("matrix-row", subReg, native.TList, tt.Matrix, native.TInteger))
	exports.Set("col", makeTypedFnDef("matrix-col", subReg, native.TList, tt.Matrix, native.TInteger))

	// Arithmetic
	exports.Set("mat-add", makeTypedFnDef("matrix-mat-add", subReg, tt.Matrix, tt.Matrix, tt.Matrix))
	exports.Set("mat-sub", makeTypedFnDef("matrix-mat-sub", subReg, tt.Matrix, tt.Matrix, tt.Matrix))
	exports.Set("mat-mul", makeTypedFnDef("matrix-mat-mul", subReg, tt.Matrix, tt.Matrix, tt.Matrix))
	exports.Set("scale", makeTypedFnDef("matrix-scale", subReg, tt.Matrix, tt.Matrix, native.TNumber))
	exports.Set("mat-emul", makeTypedFnDef("matrix-mat-emul", subReg, tt.Matrix, tt.Matrix, tt.Matrix))

	// Transform. The flat row-major list of entries is exported as
	// `values`, not `flatten`, so it does not shadow the core `flatten`
	// word (ADR-001). `transpose` is an aql:array module word, not a
	// core word, so MatrixUtil.transpose is fine.
	exports.Set("transpose", makeTypedFnDef("matrix-transpose", subReg, tt.Matrix, tt.Matrix))
	exports.Set("values", makeTypedFnDef("matrix-flatten", subReg, native.TList, tt.Matrix))

	// Aggregation
	exports.Set("sum", makeTypedFnDef("matrix-sum", subReg, native.TFloat, tt.Matrix))
	exports.Set("tr", makeTypedFnDef("matrix-trace", subReg, native.TFloat, tt.Matrix))
	exports.Set("det", makeTypedFnDef("matrix-det", subReg, native.TFloat, tt.Matrix))

	// Vector
	exports.Set("dot", makeTypedFnDef("matrix-dot", subReg, native.TFloat, native.TList, native.TList))

	// The module-owned tensor types, exported as type literals so the
	// importer can reference them — `make MatrixUtil.Matrix [[1 2][3 4]]`,
	// `x is MatrixUtil.Tensor` — mirroring IO.StreamKind.
	exports.Set("Tensor", native.NewTypeLiteral(tt.Tensor))
	exports.Set("Matrix", native.NewTypeLiteral(tt.Matrix))
	exports.Set("Vector", native.NewTypeLiteral(tt.Vector))

	// The matrix arithmetic, exported as WORD EXTENSIONS: import
	// transplants [Matrix Matrix] overloads onto the importer's bare
	// add / sub / mul (and MatrixUtil.add etc. work namespaced). The
	// minted Matrix type is what satisfies the module-scope user-type
	// rule; mat-add / mat-sub / mat-mul stay as aliases.
	for _, ext := range TensorArithmeticExtensions(tt) {
		exports.Set(ext.Extends, native.NewFnDef(ext))
	}

	return moduleDesc(parent, "MatrixUtil", subReg, exports), nil
}

// --- Word definitions ---

// MatrixNatives is the consolidated NativeFunc slice for the matrix
// module's Go-implemented words. Replaces the per-word
// registerMatrix* functions and the master registerAllMatrixWords
// aggregator.
// matrixNatives builds the module's Go-implemented words,
// parameterised by the per-import tensor mints.
func matrixNatives(tt TensorModuleTypes) []native.NativeFunc {
	return []native.NativeFunc{
		// Construction.
		{
			Name: "matrix-make",

			Signatures: []native.Signature{{
				Args: []*native.Type{native.TList},
				Impl: native.Go(func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
					td, err := matrixFromRows(args[0])
					if err != nil {
						return nil, err
					}
					return []native.Value{tensorValue(tt.Matrix, td)}, nil
				}),
				Returns: []*native.Type{tt.Matrix}, BarrierPos: -1,
			}},
		},
		{
			// rows cols zeros → args[0]=cols (top), args[1]=rows (deeper)
			Name: "matrix-zeros",

			Signatures: []native.Signature{{
				Args: []*native.Type{native.TInteger, native.TInteger},
				Impl: native.Go(func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
					r64, err := args[1].AsConcreteInteger()
					if err != nil {
						return nil, err
					}
					c64, err := args[0].AsConcreteInteger()
					if err != nil {
						return nil, err
					}
					rows, cols := int(r64), int(c64)
					if err := validDims(rows, cols); err != nil {
						return nil, err
					}
					data := make([]float64, rows*cols)
					return []native.Value{tt.newMatrix(rows, cols, data)}, nil
				}),
				Returns: []*native.Type{tt.Matrix}, BarrierPos: -1,
			}},
		},
		{
			// rows cols ones → args[0]=cols (top), args[1]=rows (deeper)
			Name: "matrix-ones",

			Signatures: []native.Signature{{
				Args: []*native.Type{native.TInteger, native.TInteger},
				Impl: native.Go(func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
					r64, err := args[1].AsConcreteInteger()
					if err != nil {
						return nil, err
					}
					c64, err := args[0].AsConcreteInteger()
					if err != nil {
						return nil, err
					}
					rows, cols := int(r64), int(c64)
					if err := validDims(rows, cols); err != nil {
						return nil, err
					}
					data := make([]float64, rows*cols)
					for i := range data {
						data[i] = 1.0
					}
					return []native.Value{tt.newMatrix(rows, cols, data)}, nil
				}),
				Returns: []*native.Type{tt.Matrix}, BarrierPos: -1,
			}},
		},
		{
			Name: "matrix-eye",

			Signatures: []native.Signature{{
				Args: []*native.Type{native.TInteger},
				Impl: native.Go(func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
					n64, err := args[0].AsConcreteInteger()
					if err != nil {
						return nil, err
					}
					n := int(n64)
					if err := validDims(n, n); err != nil {
						return nil, err
					}
					data := make([]float64, n*n)
					for i := 0; i < n; i++ {
						data[i*n+i] = 1.0
					}
					return []native.Value{tt.newMatrix(n, n, data)}, nil
				}),
				Returns: []*native.Type{tt.Matrix}, BarrierPos: -1,
			}},
		},
		{
			Name: "matrix-fill",

			Signatures: []native.Signature{{
				Args: []*native.Type{native.TInteger, native.TInteger, native.TNumber},
				Impl: native.Go(func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
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
					if err := validDims(rows, cols); err != nil {
						return nil, err
					}
					data := make([]float64, rows*cols)
					for i := range data {
						data[i] = val
					}
					return []native.Value{tt.newMatrix(rows, cols, data)}, nil
				}),
				Returns: []*native.Type{tt.Matrix}, BarrierPos: -1,
			}},
		},
		// Shape.
		{
			Name: "matrix-rows",

			Signatures: []native.Signature{{
				Args: []*native.Type{tt.Matrix},
				Impl: native.Go(func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
					m := AsTensor(args[0])
					return []native.Value{native.NewInteger(int64(m.Rows()))}, nil
				}),
				Returns: []*native.Type{native.TInteger}, BarrierPos: -1,
			}},
		},
		{
			Name: "matrix-cols",

			Signatures: []native.Signature{{
				Args: []*native.Type{tt.Matrix},
				Impl: native.Go(func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
					m := AsTensor(args[0])
					return []native.Value{native.NewInteger(int64(m.Cols()))}, nil
				}),
				Returns: []*native.Type{native.TInteger}, BarrierPos: -1,
			}},
		},
		{
			Name: "matrix-size",

			Signatures: []native.Signature{{
				Args: []*native.Type{tt.Matrix},
				Impl: native.Go(func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
					m := AsTensor(args[0])
					return []native.Value{native.NewInteger(int64(m.Rows() * m.Cols()))}, nil
				}),
				Returns: []*native.Type{native.TList}, BarrierPos: -1,
			}},
		},
		// Access.
		{
			Name: "matrix-at",

			Signatures: []native.Signature{{
				// Data-last: [col, row, mat]. Under §1.4 stack-top-first matching,
				// `mat row col matrix-at` binds sig[0]=col (top), sig[1]=row,
				// sig[2]=mat (deepest).
				Args: []*native.Type{native.TInteger, native.TInteger, tt.Matrix},
				Impl: native.Go(func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
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
					return []native.Value{native.NewFloat(m.Data[row*m.Cols()+col])}, nil
				}),
				Returns: []*native.Type{native.TFloat}, BarrierPos: -1,
			}},
		},
		{
			Name: "matrix-row",

			Signatures: []native.Signature{{
				Args: []*native.Type{native.TInteger, tt.Matrix},
				Impl: native.Go(func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
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
						elems[j] = native.NewFloat(m.Data[row*m.Cols()+j])
					}
					return []native.Value{native.NewList(elems)}, nil
				}),
				Returns: []*native.Type{native.TList}, BarrierPos: -1,
			}},
		},
		{
			Name: "matrix-col",

			Signatures: []native.Signature{{
				Args: []*native.Type{native.TInteger, tt.Matrix},
				Impl: native.Go(func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
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
						elems[i] = native.NewFloat(m.Data[i*m.Cols()+col])
					}
					return []native.Value{native.NewList(elems)}, nil
				}),
				Returns: []*native.Type{native.TList}, BarrierPos: -1,
			}},
		},
		// Arithmetic.
		{
			Name: "matrix-mat-add",

			Signatures: []native.Signature{{
				Args:    []*native.Type{tt.Matrix, tt.Matrix},
				Impl:    native.Go(tt.matAddHandler),
				Returns: []*native.Type{tt.Matrix}, BarrierPos: -1,
			}},
		},
		{
			// a b mat-sub → args[0]=b (top), args[1]=a
			Name: "matrix-mat-sub",

			Signatures: []native.Signature{{
				Args:    []*native.Type{tt.Matrix, tt.Matrix},
				Impl:    native.Go(tt.matSubHandler),
				Returns: []*native.Type{tt.Matrix}, BarrierPos: -1,
			}},
		},
		{
			// a b mat-mul → args[0]=b (top), args[1]=a
			Name: "matrix-mat-mul",

			Signatures: []native.Signature{{
				Args:    []*native.Type{tt.Matrix, tt.Matrix},
				Impl:    native.Go(tt.matMulHandler),
				Returns: []*native.Type{tt.Matrix}, BarrierPos: -1,
			}},
		},
		{
			Name: "matrix-scale",

			Signatures: []native.Signature{{
				Args: []*native.Type{native.TNumber, tt.Matrix},
				Impl: native.Go(func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
					s, err := native.AsNumber(args[0])
					if err != nil {
						return nil, err
					}
					m := AsTensor(args[1])
					data := make([]float64, len(m.Data))
					for i := range data {
						data[i] = m.Data[i] * s
					}
					return []native.Value{tt.newMatrix(m.Rows(), m.Cols(), data)}, nil
				}),
				Returns: []*native.Type{tt.Matrix}, BarrierPos: -1,
			}},
		},
		{
			Name: "matrix-mat-emul",

			Signatures: []native.Signature{{
				Args: []*native.Type{tt.Matrix, tt.Matrix},
				Impl: native.Go(func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
					a := AsTensor(args[0])
					b := AsTensor(args[1])
					if a.Rows() != b.Rows() || a.Cols() != b.Cols() {
						return nil, fmt.Errorf("mat-emul: dimension mismatch %dx%d vs %dx%d", a.Rows(), a.Cols(), b.Rows(), b.Cols())
					}
					data := make([]float64, len(a.Data))
					for i := range data {
						data[i] = a.Data[i] * b.Data[i]
					}
					return []native.Value{tt.newMatrix(a.Rows(), a.Cols(), data)}, nil
				}),
				Returns: []*native.Type{tt.Matrix}, BarrierPos: -1,
			}},
		},
		// Transform.
		{
			Name: "matrix-transpose",

			Signatures: []native.Signature{{
				Args: []*native.Type{tt.Matrix},
				Impl: native.Go(func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
					m := AsTensor(args[0])
					data := make([]float64, len(m.Data))
					for i := 0; i < m.Rows(); i++ {
						for j := 0; j < m.Cols(); j++ {
							data[j*m.Rows()+i] = m.Data[i*m.Cols()+j]
						}
					}
					return []native.Value{tt.newMatrix(m.Cols(), m.Rows(), data)}, nil
				}),
				Returns: []*native.Type{tt.Matrix}, BarrierPos: -1,
			}},
		},
		{
			Name: "matrix-flatten",

			Signatures: []native.Signature{{
				Args: []*native.Type{tt.Matrix},
				Impl: native.Go(func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
					m := AsTensor(args[0])
					elems := make([]native.Value, len(m.Data))
					for i, v := range m.Data {
						elems[i] = native.NewFloat(v)
					}
					return []native.Value{native.NewList(elems)}, nil
				}),
				Returns: []*native.Type{native.TList}, BarrierPos: -1,
			}},
		},
		// Aggregation.
		{
			Name: "matrix-sum",

			Signatures: []native.Signature{{
				Args: []*native.Type{tt.Matrix},
				Impl: native.Go(func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
					m := AsTensor(args[0])
					s := 0.0
					for _, v := range m.Data {
						s += v
					}
					return []native.Value{native.NewFloat(s)}, nil
				}),
				Returns: []*native.Type{native.TFloat}, BarrierPos: -1,
			}},
		},
		{
			Name: "matrix-trace",

			Signatures: []native.Signature{{
				Args: []*native.Type{tt.Matrix},
				Impl: native.Go(func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
					m := AsTensor(args[0])
					if m.Rows() != m.Cols() {
						return nil, fmt.Errorf("trace: not square (%dx%d)", m.Rows(), m.Cols())
					}
					s := 0.0
					for i := 0; i < m.Rows(); i++ {
						s += m.Data[i*m.Cols()+i]
					}
					return []native.Value{native.NewFloat(s)}, nil
				}),
				Returns: []*native.Type{native.TFloat}, BarrierPos: -1,
			}},
		},
		{
			Name: "matrix-det",

			Signatures: []native.Signature{{
				Args: []*native.Type{tt.Matrix},
				Impl: native.Go(func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
					m := AsTensor(args[0])
					d, err := matDet(m)
					if err != nil {
						return nil, err
					}
					return []native.Value{native.NewFloat(d)}, nil
				}),
				Returns: []*native.Type{native.TFloat}, BarrierPos: -1,
			}},
		},
		// Vector.
		{
			Name: "matrix-dot",

			Signatures: []native.Signature{{
				Args: []*native.Type{native.TList, native.TList},
				Impl: native.Go(func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
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
					return []native.Value{native.NewFloat(s)}, nil
				}),
				Returns: []*native.Type{native.TFloat}, BarrierPos: -1,
			}},
		},
	}
}

// matAddHandler / matSubHandler / matMulHandler are the shared
// matrix-arithmetic handlers: each backs BOTH the namespaced mat-*
// word and the module's add / sub / mul word extension, so the two
// surfaces cannot drift. Arg order follows the kernel convention —
// args[1] op args[0] for the non-commutative ops, matching mat-sub /
// mat-mul's historical stack reading (`a b mat-sub` → a-b).
func (tt TensorModuleTypes) matAddHandler(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
	a := AsTensor(args[0])
	b := AsTensor(args[1])
	if a.Rows() != b.Rows() || a.Cols() != b.Cols() {
		return nil, fmt.Errorf("mat-add: dimension mismatch %dx%d vs %dx%d", a.Rows(), a.Cols(), b.Rows(), b.Cols())
	}
	data := make([]float64, len(a.Data))
	for i := range data {
		data[i] = a.Data[i] + b.Data[i]
	}
	return []native.Value{tt.newMatrix(a.Rows(), a.Cols(), data)}, nil
}

func (tt TensorModuleTypes) matSubHandler(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
	a := AsTensor(args[1])
	b := AsTensor(args[0])
	if a.Rows() != b.Rows() || a.Cols() != b.Cols() {
		return nil, fmt.Errorf("mat-sub: dimension mismatch %dx%d vs %dx%d", a.Rows(), a.Cols(), b.Rows(), b.Cols())
	}
	data := make([]float64, len(a.Data))
	for i := range data {
		data[i] = a.Data[i] - b.Data[i]
	}
	return []native.Value{tt.newMatrix(a.Rows(), a.Cols(), data)}, nil
}

func (tt TensorModuleTypes) matMulHandler(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
	a := AsTensor(args[1])
	b := AsTensor(args[0])
	result, err := matMul(a, b)
	if err != nil {
		return nil, err
	}
	return []native.Value{tensorValue(tt.Matrix, result)}, nil
}

// TensorArithmeticExtensions builds the add / sub / mul WORD-EXTENSION
// clones the module exports (design/OPEN-WORDS.0.md §6 migration 2):
// import transplants [Matrix Matrix] overloads onto the importer's
// bare add / sub / mul, anchored by the module's minted Matrix type.
// The mat-* words remain as aliases backed by the same handlers.
func TensorArithmeticExtensions(tt TensorModuleTypes) []native.FnDefInfo {
	return []native.FnDefInfo{
		native.NewWordExtension("add", []native.Signature{
			{Args: []*native.Type{tt.Matrix, tt.Matrix}, Impl: native.Go(tt.matAddHandler), Returns: []*native.Type{tt.Matrix}, BarrierPos: -1},
		}),
		native.NewWordExtension("sub", []native.Signature{
			{Args: []*native.Type{tt.Matrix, tt.Matrix}, Impl: native.Go(tt.matSubHandler), Returns: []*native.Type{tt.Matrix}, BarrierPos: -1},
		}),
		native.NewWordExtension("mul", []native.Signature{
			{Args: []*native.Type{tt.Matrix, tt.Matrix}, Impl: native.Go(tt.matMulHandler), Returns: []*native.Type{tt.Matrix}, BarrierPos: -1},
		}),
	}
}

// --- Internal helpers ---

// matrixFromRows builds a rank-2 TensorData from a list of equal-length
// row lists. Shared by the `matrix-make` word and the Matrix Ideal's
// Instantiate (see matrix_ideal.go) so `MatrixUtil.create` and
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

// ToMap / ToList implement native.IdealConverter (eng.IdealConverter) for
// the tensor kinds: ToList gives the nested row structure; ToMap gives
// {shape:[…] values:[flat]}.
func (tensorFormatBehavior) ToList(v native.Value) (native.Value, error) {
	t, ok := tensorPayload(v)
	if !ok {
		return native.NewList(nil), nil
	}
	return tensorNested(t, 0, 0, len(t.Data)), nil
}

func (tensorFormatBehavior) ToMap(v native.Value) (native.Value, error) {
	m := native.NewOrderedMap()
	t, ok := tensorPayload(v)
	if !ok {
		return native.NewMap(m), nil
	}
	shape := make([]native.Value, len(t.Shape))
	for i, d := range t.Shape {
		shape[i] = native.NewInteger(int64(d))
	}
	vals := make([]native.Value, len(t.Data))
	for i, d := range t.Data {
		vals[i] = native.NewFloat(d)
	}
	m.Set("shape", native.NewList(shape))
	m.Set("values", native.NewList(vals))
	return native.NewMap(m), nil
}

// tensorNested builds the nested-list view of t's [lo,hi) data slice for
// the dimension at the given axis. Rank-1 → flat list of Floats.
func tensorNested(t TensorData, axis, lo, hi int) native.Value {
	if axis >= len(t.Shape)-1 {
		elems := make([]native.Value, 0, hi-lo)
		for i := lo; i < hi; i++ {
			elems = append(elems, native.NewFloat(t.Data[i]))
		}
		return native.NewList(elems)
	}
	dim := t.Shape[axis]
	if dim == 0 {
		return native.NewList(nil)
	}
	stride := (hi - lo) / dim
	rows := make([]native.Value, 0, dim)
	for i := 0; i < dim; i++ {
		rows = append(rows, tensorNested(t, axis+1, lo+i*stride, lo+(i+1)*stride))
	}
	return native.NewList(rows)
}
