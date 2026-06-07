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
		"mat-add":   "Element-wise sum of two matrices.",
		"mat-emul":  "Element-wise (Hadamard) product of two matrices.",
		"mat-mul":   "Matrix product of two matrices.",
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
