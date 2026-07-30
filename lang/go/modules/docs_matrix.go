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

	// A Matrix is an opaque handle whose Format is just its dimensions, so
	// nearly every example needs a `values` call to show the contents —
	// which the generated permutations never do. Results are from verified
	// lang/spec/module-matrix.tsv rows.
	registerExamples("aql:matrix-util", map[string][]string{
		"create": {
			`MatrixUtil.create [[1 2] [3 4]]                  ;# Matrix(2x2) — an opaque handle`,
			`MatrixUtil.values (MatrixUtil.create [[1 2] [3 4]])   ;# [1.0 2.0 3.0 4.0] — read it back, row-major`,
		},
		"values": {`MatrixUtil.values (MatrixUtil.create [[1 2] [3 4]])   ;# [1.0 2.0 3.0 4.0] — always Floats`},
		"zeros":  {`MatrixUtil.zeros 3 2                             ;# Matrix(2x3) — COLS then ROWS`},
		"ones":   {`MatrixUtil.ones 2 2                              ;# Matrix(2x2)`},
		"eye":    {`MatrixUtil.eye 2                                 ;# the 2x2 identity`},
		"fill":   {`MatrixUtil.fill 2 2 5                            ;# Matrix(2x2), every cell 5`},
		"rows":   {`MatrixUtil.rows (MatrixUtil.create [[1 2 3] [4 5 6]])   ;# 2`},
		"cols":   {`MatrixUtil.cols (MatrixUtil.create [[1 2 3] [4 5 6]])   ;# 3`},
		"elem":   {`(MatrixUtil.create [[1 2] [3 4]]) MatrixUtil.elem 1 0   ;# 2.0`},
		"row":    {`(MatrixUtil.create [[1 2 3] [4 5 6]]) MatrixUtil.row 1  ;# [4.0 5.0 6.0]`},
		"col":    {`(MatrixUtil.create [[1 2 3] [4 5 6]]) MatrixUtil.col 2  ;# [3.0 6.0]`},
		"mat-mul": {
			`MatrixUtil.mat-mul (MatrixUtil.create [[1 2] [3 4]]) (MatrixUtil.create [[5 6] [7 8]])`,
			`;# Matrix(2x2) — true matrix product. For element-wise use mat-emul.`,
		},
		"mat-emul": {
			`MatrixUtil.mat-emul (MatrixUtil.create [[1 2] [3 4]]) (MatrixUtil.create [[5 6] [7 8]])`,
			`;# Matrix(2x2) — ELEMENT-wise, not the matrix product.`,
		},
		"mat-add":   {`MatrixUtil.mat-add (MatrixUtil.create [[1 2] [3 4]]) (MatrixUtil.create [[10 20] [30 40]])`},
		"mat-sub":   {`MatrixUtil.mat-sub (MatrixUtil.create [[10 20] [30 40]]) (MatrixUtil.create [[1 2] [3 4]])`},
		"scale":     {`MatrixUtil.scale 2 (MatrixUtil.create [[1 2] [3 4]])     ;# every cell doubled`},
		"transpose": {`MatrixUtil.transpose (MatrixUtil.create [[1 2 3] [4 5 6]])   ;# Matrix(3x2)`},
		"sum":       {`MatrixUtil.sum (MatrixUtil.create [[1 2] [3 4]])         ;# 10.0`},
		"tr":        {`MatrixUtil.tr (MatrixUtil.create [[1 2] [3 4]])          ;# 5.0 — the trace`},
		"det":       {`MatrixUtil.det (MatrixUtil.create [[1 2] [3 4]])         ;# -2.0`},
		"dot":       {`MatrixUtil.dot [1 2 3] [4 5 6]                   ;# 32.0 — plain LISTS, not Matrices`},
		"add": {
			`MatrixUtil.values (MatrixUtil.add (MatrixUtil.eye 2) (MatrixUtil.eye 2))   ;# [2.0 0.0 0.0 2.0]`,
			`;# add/sub/mul are also installed on the core operators, so`,
			`;# ` + "`(MatrixUtil.eye 2) add (MatrixUtil.eye 2)`" + ` works too.`,
		},
	})
}
