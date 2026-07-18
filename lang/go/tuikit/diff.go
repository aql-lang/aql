package tuikit

// Span is one damaged run: Cells replace the row-Y cells starting at X.
type Span struct {
	X, Y  int
	Cells []Cell
}

// DiffFrames reports the damage between two frames as row-wise spans of
// changed cells — the shared helper backends diff with (TUI.0.md §4.2).
// A dimension change (or a nil prev) is total damage: one span per row
// of next.
func DiffFrames(prev, next *Frame) []Span {
	if prev == nil || prev.Cols != next.Cols || prev.Rows != next.Rows {
		spans := make([]Span, 0, next.Rows)
		for y := 0; y < next.Rows; y++ {
			row := make([]Cell, next.Cols)
			copy(row, next.Cells[y*next.Cols:(y+1)*next.Cols])
			spans = append(spans, Span{X: 0, Y: y, Cells: row})
		}
		return spans
	}
	var spans []Span
	for y := 0; y < next.Rows; y++ {
		x := 0
		for x < next.Cols {
			idx := y*next.Cols + x
			if prev.Cells[idx] == next.Cells[idx] {
				x++
				continue
			}
			start := x
			for x < next.Cols && prev.Cells[y*next.Cols+x] != next.Cells[y*next.Cols+x] {
				x++
			}
			run := make([]Cell, x-start)
			copy(run, next.Cells[y*next.Cols+start:y*next.Cols+x])
			spans = append(spans, Span{X: start, Y: y, Cells: run})
		}
	}
	return spans
}
