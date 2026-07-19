package tuikit

// Style is the cell-attribute set carried by every painted cell —
// the Go carrier of the style maps of TUI.0.md §3.3. Colors are kept as
// the strings AQL programs wrote ("red", "#rrggbb", a 256-palette index
// in decimal); interpretation/degradation is the painting backend's job.
type Style struct {
	FG, BG    string
	Bold      bool
	Italic    bool
	Underline bool
	Reverse   bool
}

// Cell is one screen cell: a single grapheme plus its display width and
// style. An empty Content renders as a blank; Width -1 marks the
// continuation cell shadowed by a wide rune in the cell to its left
// (skipped when a frame is projected to row strings).
type Cell struct {
	Content string
	Width   int8
	Style   Style
}

// Frame is a row-major grid of cells — the unit Present consumes
// (TUI.0.md §4.2). len(Cells) == Cols*Rows always.
type Frame struct {
	Cols, Rows int
	Cells      []Cell
}

// NewFrame returns a blank Cols×Rows frame. Non-positive dimensions are
// clamped to zero (an empty frame is legal and paints nothing).
func NewFrame(cols, rows int) *Frame {
	if cols < 0 {
		cols = 0
	}
	if rows < 0 {
		rows = 0
	}
	return &Frame{Cols: cols, Rows: rows, Cells: make([]Cell, cols*rows)}
}

// At returns the cell at (x, y), reporting false when out of bounds.
func (f *Frame) At(x, y int) (Cell, bool) {
	if x < 0 || y < 0 || x >= f.Cols || y >= f.Rows {
		return Cell{}, false
	}
	return f.Cells[y*f.Cols+x], true
}

// Set writes the cell at (x, y), reporting false (and writing nothing)
// when out of bounds — drawing words clip silently at the edges.
func (f *Frame) Set(x, y int, c Cell) bool {
	if x < 0 || y < 0 || x >= f.Cols || y >= f.Rows {
		return false
	}
	f.Cells[y*f.Cols+x] = c
	return true
}

// Clone returns an independent copy. Present receivers that retain
// frames (the virtual backend's history) store clones so later drawing
// into the source grid cannot mutate what was already presented.
func (f *Frame) Clone() *Frame {
	c := &Frame{Cols: f.Cols, Rows: f.Rows, Cells: make([]Cell, len(f.Cells))}
	copy(c.Cells, f.Cells)
	return c
}

// renderRows projects a frame to one string per row, using a space for
// blank cells and skipping wide-rune continuation cells — the assertion
// surface tests golden-match against ("漢字" stays a contiguous
// substring even though it spans four columns).
func renderRows(f *Frame) []string {
	rows := make([]string, f.Rows)
	for y := 0; y < f.Rows; y++ {
		row := make([]byte, 0, f.Cols)
		for x := 0; x < f.Cols; x++ {
			c := f.Cells[y*f.Cols+x]
			switch {
			case c.Width < 0:
				// continuation of the wide rune to the left
			case c.Content == "":
				row = append(row, ' ')
			default:
				row = append(row, c.Content...)
			}
		}
		rows[y] = string(row)
	}
	return rows
}
