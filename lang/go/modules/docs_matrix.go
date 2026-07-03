package modules

func init() {
	registerDocs("aql:matrix-util", map[string]string{
		"col":       "One column of a matrix as a list.",
		"cols":      "Number of columns in a matrix.",
		"create":    "Build a matrix from a list of equal-length rows.",
		"det":       "Determinant of a square matrix.",
		"dot":       "Dot product of two equal-length vectors.",
		"elem":      "A single matrix entry at the given row and column.",
		"eye":       "Identity matrix of the given size.",
		"fill":      "A rows-by-cols matrix filled with a constant value.",
		"add":       "Matrix plus Matrix, element-wise (word extension of core add).",
		"sub":       "Matrix minus Matrix, element-wise (word extension of core sub).",
		"mul":       "Matrix product (word extension of core mul).",
		"mat-add":   "Element-wise sum of two matrices.",
		"mat-emul":  "Element-wise (Hadamard) product of two matrices.",
		"mat-mul":   "Matrix product of two matrices. Non-commutative: like every binary op, the stack/swap form 'A mat-mul B' computes A·B, while the forward form 'mat-mul A B' binds operands in reverse and computes B·A.",
		"mat-sub":   "Element-wise difference of two matrices.",
		"ones":      "All-one matrix (forward args are cols then rows).",
		"row":       "One row of a matrix as a list.",
		"rows":      "Number of rows in a matrix.",
		"scale":     "Multiply every entry of a matrix by a scalar.",
		"sum":       "Total of all entries in a matrix.",
		"tr":        "Trace: sum of the diagonal entries.",
		"transpose": "Swap the rows and columns of a matrix.",
		"values":    "The row-major flat list of matrix entries.",
		"zeros":     "All-zero matrix (forward args are cols then rows).",
	})
}
