package tuikit

import (
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"
)

// The tree→Frame renderer of TUI.0.md §3: consumes the widget tree as
// ValueToAny-shaped data (the SAME shape the boru:tui module projects
// from BORU values and the remote protocol carries as JSON — one
// rendering code path for local and attached clients), lays out with
// the §3.2 constraint solver, and paints the nine §3.1 widget kinds
// into a Frame. Pure: no engine, no terminal.

// Rect is a paint region in frame coordinates.
type Rect struct{ X, Y, W, H int }

// CursorPos is where the hardware cursor should sit after a render —
// set when exactly one `input` in the tree has focus:true (§3.4).
type CursorPos struct{ X, Y int }

// RenderResult carries one rendered frame plus the cursor verdict.
type RenderResult struct {
	Frame  *Frame
	Cursor *CursorPos
}

// viewport virtual-canvas caps: a scrolled child lays out into an
// offscreen canvas at its intrinsic size, bounded so a runaway fit
// cannot allocate unboundedly.
const (
	viewportMaxCols = 1024
	viewportMaxRows = 4096
)

// Render lays the widget tree out into a cols×rows frame.
func Render(tree any, cols, rows int) (*RenderResult, error) {
	rd := &renderer{frame: NewFrame(cols, rows)}
	root := Rect{X: 0, Y: 0, W: rd.frame.Cols, H: rd.frame.Rows}
	if err := rd.paint(tree, root, Style{}); err != nil {
		return nil, err
	}
	res := &RenderResult{Frame: rd.frame}
	if rd.focused == 1 {
		res.Cursor = &rd.cursor
	}
	return res, nil
}

type renderer struct {
	frame   *Frame
	focused int
	cursor  CursorPos
}

var widgetKinds = map[string]bool{
	"text": true, "rows": true, "cols": true, "box": true,
	"list-view": true, "table": true, "input": true,
	"viewport": true, "spacer": true,
}

// parseWidget validates one tree node: a map with a known w: kind.
func parseWidget(v any) (string, map[string]any, error) {
	m, ok := asNode(v)
	if !ok {
		return "", nil, fmt.Errorf("bad widget: a tree node must be a Map, got %T", v)
	}
	w, has, err := nodeStr(m, "w")
	if err != nil || !has {
		return "", nil, fmt.Errorf("bad widget: a tree node needs a w: kind String")
	}
	if !widgetKinds[w] {
		return "", nil, fmt.Errorf("bad widget: unknown kind %q", w)
	}
	return w, m, nil
}

// styleOver merges a node's style: map over the inherited style — only
// the keys present override (§3.3 inheritance).
func styleOver(inherited Style, m map[string]any) (Style, error) {
	v, ok := m["style"]
	if !ok {
		return inherited, nil
	}
	sm, ok := asNode(v)
	if !ok {
		return Style{}, fmt.Errorf("bad widget: style: must be a Map")
	}
	st := inherited
	if s, has, err := nodeStr(sm, "fg"); err != nil {
		return Style{}, fmt.Errorf("bad widget: style %v", err)
	} else if has {
		st.FG = s
	}
	if s, has, err := nodeStr(sm, "bg"); err != nil {
		return Style{}, fmt.Errorf("bad widget: style %v", err)
	} else if has {
		st.BG = s
	}
	for key, dst := range map[string]*bool{
		"bold": &st.Bold, "italic": &st.Italic,
		"underline": &st.Underline, "reverse": &st.Reverse,
	} {
		if b, has, err := nodeBool(sm, key); err != nil {
			return Style{}, fmt.Errorf("bad widget: style %v", err)
		} else if has {
			*dst = b
		}
	}
	return st, nil
}

// ---- measurement ("fit") ----

// measureFit reports a widget's intrinsic (width, height) given the
// width available for text wrapping.
func measureFit(v any, availW int) (int, int, error) {
	kind, m, err := parseWidget(v)
	if err != nil {
		return 0, 0, err
	}
	switch kind {
	case "text":
		s, _, tErr := nodeStr(m, "text")
		if tErr != nil {
			return 0, 0, fmt.Errorf("bad widget: %v", tErr)
		}
		wrap, _, wErr := nodeStr(m, "wrap")
		if wErr != nil {
			return 0, 0, fmt.Errorf("bad widget: %v", wErr)
		}
		w := StringWidth(s)
		if wrap == "wrap" && availW > 0 && w > availW {
			return availW, (w + availW - 1) / availW, nil
		}
		return w, 1, nil
	case "rows", "cols":
		children, _, cErr := nodeList(m, "children")
		if cErr != nil {
			return 0, 0, fmt.Errorf("bad widget: %v", cErr)
		}
		gap, _, gErr := nodeInt(m, "gap")
		if gErr != nil {
			return 0, 0, fmt.Errorf("bad widget: %v", gErr)
		}
		maxW, maxH, sumW, sumH := 0, 0, 0, 0
		for _, c := range children {
			cw, ch, err := measureFit(c, availW)
			if err != nil {
				return 0, 0, err
			}
			sumW += cw
			sumH += ch
			if cw > maxW {
				maxW = cw
			}
			if ch > maxH {
				maxH = ch
			}
		}
		if n := len(children); n > 1 {
			sumW += gap * (n - 1)
			sumH += gap * (n - 1)
		}
		if kind == "rows" {
			return maxW, sumH, nil
		}
		return sumW, maxH, nil
	case "box":
		inset, child, bErr := boxInsets(m)
		if bErr != nil {
			return 0, 0, bErr
		}
		cw, ch := 0, 0
		if child != nil {
			var err error
			cw, ch, err = measureFit(child, availW-2*inset.x)
			if err != nil {
				return 0, 0, err
			}
		}
		return cw + 2*inset.x, ch + 2*inset.y, nil
	case "list-view":
		items, _, iErr := nodeList(m, "items")
		if iErr != nil {
			return 0, 0, fmt.Errorf("bad widget: %v", iErr)
		}
		maxW := 0
		for _, it := range items {
			label, lErr := itemLabel(it)
			if lErr != nil {
				return 0, 0, lErr
			}
			if w := StringWidth(label); w > maxW {
				maxW = w
			}
		}
		return maxW, len(items), nil
	case "table":
		widths, _, tErr := tableColumns(m)
		if tErr != nil {
			return 0, 0, tErr
		}
		dataRows, _, rErr := nodeList(m, "rows")
		if rErr != nil {
			return 0, 0, fmt.Errorf("bad widget: %v", rErr)
		}
		total := 0
		for _, w := range widths {
			total += w
		}
		if n := len(widths); n > 1 {
			total += n - 1 // one space between columns
		}
		return total, 1 + len(dataRows), nil
	case "input":
		value, _, vErr := nodeStr(m, "value")
		if vErr != nil {
			return 0, 0, fmt.Errorf("bad widget: %v", vErr)
		}
		mask, _, mErr := nodeBool(m, "mask")
		if mErr != nil {
			return 0, 0, fmt.Errorf("bad widget: %v", mErr)
		}
		if mask {
			// masked input paints one narrow bullet per rune
			return utf8.RuneCountInString(value) + 1, 1, nil
		}
		return StringWidth(value) + 1, 1, nil
	case "viewport":
		child, ok := m["child"]
		if !ok {
			return 0, 0, nil
		}
		return measureFit(child, availW)
	default: // spacer
		return 0, 0, nil
	}
}

// ---- painting ----

func (rd *renderer) paint(v any, rect Rect, inherited Style) error {
	kind, m, err := parseWidget(v)
	if err != nil {
		return err
	}
	st, err := styleOver(inherited, m)
	if err != nil {
		return err
	}
	if rect.W <= 0 || rect.H <= 0 {
		return nil // clipped away entirely; nothing to paint
	}
	switch kind {
	case "text":
		return rd.paintText(m, rect, st)
	case "rows":
		return rd.paintStack(m, rect, st, true)
	case "cols":
		return rd.paintStack(m, rect, st, false)
	case "box":
		return rd.paintBox(m, rect, st)
	case "list-view":
		return rd.paintList(m, rect, st)
	case "table":
		return rd.paintTable(m, rect, st)
	case "input":
		return rd.paintInput(m, rect, st)
	case "viewport":
		return rd.paintViewport(m, rect, st)
	default: // spacer
		return nil
	}
}

// drawText writes one run of styled text starting at (x, y), clipping
// at maxX: wide runes occupy two cells (dropped when split at the clip
// edge), combining marks join the previous cell's grapheme (unless they
// open the run — there is no previous cell of this run to join).
func (rd *renderer) drawText(x, y int, s string, st Style, maxX int) {
	start := x
	for _, r := range s {
		w := RuneWidth(r)
		if w == 0 {
			if x-1 >= start && x-1 < maxX {
				if c, ok := rd.frame.At(x-1, y); ok && c.Content != "" {
					c.Content += string(r)
					rd.frame.Set(x-1, y, c)
				}
			}
			continue
		}
		if x+w > maxX {
			break
		}
		rd.frame.Set(x, y, Cell{Content: string(r), Width: int8(w), Style: st})
		if w == 2 {
			rd.frame.Set(x+1, y, Cell{Content: "", Width: -1, Style: st})
		}
		x += w
	}
}

// fillStyle paints a rect's background style (used for cursor rows).
func (rd *renderer) fillStyle(rect Rect, st Style) {
	for y := rect.Y; y < rect.Y+rect.H; y++ {
		for x := rect.X; x < rect.X+rect.W; x++ {
			c, ok := rd.frame.At(x, y)
			if !ok {
				continue
			}
			c.Style = st
			rd.frame.Set(x, y, c)
		}
	}
}

func (rd *renderer) paintText(m map[string]any, rect Rect, st Style) error {
	s, _, err := nodeStr(m, "text")
	if err != nil {
		return fmt.Errorf("bad widget: %v", err)
	}
	wrap, _, err := nodeStr(m, "wrap")
	if err != nil {
		return fmt.Errorf("bad widget: %v", err)
	}
	if wrap != "wrap" {
		rd.drawText(rect.X, rect.Y, s, st, rect.X+rect.W)
		return nil
	}
	line, y := s, rect.Y
	for line != "" && y < rect.Y+rect.H {
		head, _ := truncateToWidth(line, rect.W)
		if head == "" {
			break // a wide rune that cannot fit at all
		}
		rd.drawText(rect.X, y, head, st, rect.X+rect.W)
		line = line[len(head):]
		y++
	}
	return nil
}

func (rd *renderer) paintStack(m map[string]any, rect Rect, st Style, vertical bool) error {
	children, _, err := nodeList(m, "children")
	if err != nil {
		return fmt.Errorf("bad widget: %v", err)
	}
	gap, _, err := nodeInt(m, "gap")
	if err != nil {
		return fmt.Errorf("bad widget: %v", err)
	}
	if gap < 0 {
		gap = 0
	}
	specs := make([]sizeSpec, len(children))
	fits := make([]int, len(children))
	for i, c := range children {
		cm, ok := asNode(c)
		if !ok {
			return fmt.Errorf("bad widget: a tree node must be a Map, got %T", c)
		}
		if specs[i], err = parseSizeSpec(cm); err != nil {
			return fmt.Errorf("bad widget: %v", err)
		}
		if specs[i].kind == sizeFit {
			fw, fh, mErr := measureFit(c, rect.W)
			if mErr != nil {
				return mErr
			}
			if vertical {
				fits[i] = fh
			} else {
				fits[i] = fw
			}
		}
	}
	total := rect.H
	if !vertical {
		total = rect.W
	}
	sizes := solveSizes(total, gap, specs, fits)
	pos := 0
	for i, c := range children {
		var childRect Rect
		if vertical {
			childRect = Rect{X: rect.X, Y: rect.Y + pos, W: rect.W, H: sizes[i]}
		} else {
			childRect = Rect{X: rect.X + pos, Y: rect.Y, W: sizes[i], H: rect.H}
		}
		if err := rd.paint(c, childRect, st); err != nil {
			return err
		}
		pos += sizes[i] + gap
	}
	return nil
}

type inset struct{ x, y int }

var borderSets = map[string][6]rune{
	// tl, tr, bl, br, horizontal, vertical
	"line":   {'┌', '┐', '└', '┘', '─', '│'},
	"round":  {'╭', '╮', '╰', '╯', '─', '│'},
	"double": {'╔', '╗', '╚', '╝', '═', '║'},
}

// boxInsets reads a box's border/pad config: the child inset per axis
// (border edge + padding) and the child node (nil when absent).
func boxInsets(m map[string]any) (inset, any, error) {
	border, has, err := nodeStr(m, "border")
	if err != nil {
		return inset{}, nil, fmt.Errorf("bad widget: %v", err)
	}
	if !has {
		border = "line"
	}
	if border != "none" {
		if _, ok := borderSets[border]; !ok {
			return inset{}, nil, fmt.Errorf("bad widget: border: unknown kind %q", border)
		}
	}
	in := inset{}
	if border != "none" {
		in = inset{x: 1, y: 1}
	}
	if pad, has, pErr := nodeList(m, "pad"); pErr != nil {
		return inset{}, nil, fmt.Errorf("bad widget: %v", pErr)
	} else if has {
		if len(pad) != 2 {
			return inset{}, nil, fmt.Errorf("bad widget: pad: must be [x y]")
		}
		px, okX := toInt(pad[0])
		py, okY := toInt(pad[1])
		if !okX || !okY || px < 0 || py < 0 {
			return inset{}, nil, fmt.Errorf("bad widget: pad: must be two non-negative Integers")
		}
		in.x += px
		in.y += py
	}
	return in, m["child"], nil
}

func (rd *renderer) paintBox(m map[string]any, rect Rect, st Style) error {
	border, has, err := nodeStr(m, "border")
	if err != nil {
		return fmt.Errorf("bad widget: %v", err)
	}
	if !has {
		border = "line"
	}
	in, child, err := boxInsets(m)
	if err != nil {
		return err
	}
	if border != "none" && rect.W >= 2 && rect.H >= 2 {
		set := borderSets[border]
		right, bottom := rect.X+rect.W-1, rect.Y+rect.H-1
		for x := rect.X + 1; x < right; x++ {
			rd.frame.Set(x, rect.Y, Cell{Content: string(set[4]), Width: 1, Style: st})
			rd.frame.Set(x, bottom, Cell{Content: string(set[4]), Width: 1, Style: st})
		}
		for y := rect.Y + 1; y < bottom; y++ {
			rd.frame.Set(rect.X, y, Cell{Content: string(set[5]), Width: 1, Style: st})
			rd.frame.Set(right, y, Cell{Content: string(set[5]), Width: 1, Style: st})
		}
		rd.frame.Set(rect.X, rect.Y, Cell{Content: string(set[0]), Width: 1, Style: st})
		rd.frame.Set(right, rect.Y, Cell{Content: string(set[1]), Width: 1, Style: st})
		rd.frame.Set(rect.X, bottom, Cell{Content: string(set[2]), Width: 1, Style: st})
		rd.frame.Set(right, bottom, Cell{Content: string(set[3]), Width: 1, Style: st})
		title, _, tErr := nodeStr(m, "title")
		if tErr != nil {
			return fmt.Errorf("bad widget: %v", tErr)
		}
		if title != "" && rect.W > 4 {
			label, _ := truncateToWidth(" "+title+" ", rect.W-2)
			rd.drawText(rect.X+1, rect.Y, label, st, right)
		}
	}
	if child == nil {
		return nil
	}
	childRect := Rect{
		X: rect.X + in.x, Y: rect.Y + in.y,
		W: rect.W - 2*in.x, H: rect.H - 2*in.y,
	}
	return rd.paint(child, childRect, st)
}

// itemLabel projects a list-view item: a string, or a {label: …} map.
func itemLabel(it any) (string, error) {
	if s, ok := it.(string); ok {
		return s, nil
	}
	if m, ok := asNode(it); ok {
		s, has, err := nodeStr(m, "label")
		if err == nil && has {
			return s, nil
		}
	}
	return "", fmt.Errorf("bad widget: list-view items must be Strings or {label: …} Maps")
}

// scrollOffset keeps the cursor row visible in a window of h rows. An
// out-of-range cursor (beyond the item count, or a count smaller than
// the window) clamps to a non-negative offset — bad app state paints
// from the top rather than indexing below zero.
func scrollOffset(cursor, count, h int) int {
	if h <= 0 || cursor < h {
		return 0
	}
	off := cursor - h + 1
	if max := count - h; off > max {
		off = max
	}
	if off < 0 {
		off = 0
	}
	return off
}

func (rd *renderer) paintList(m map[string]any, rect Rect, st Style) error {
	items, _, err := nodeList(m, "items")
	if err != nil {
		return fmt.Errorf("bad widget: %v", err)
	}
	cursor, _, err := nodeInt(m, "cursor")
	if err != nil {
		return fmt.Errorf("bad widget: %v", err)
	}
	off := scrollOffset(cursor, len(items), rect.H)
	for row := 0; row < rect.H && off+row < len(items); row++ {
		label, lErr := itemLabel(items[off+row])
		if lErr != nil {
			return lErr
		}
		lineStyle := st
		if off+row == cursor {
			lineStyle.Reverse = !lineStyle.Reverse
			rd.fillStyle(Rect{X: rect.X, Y: rect.Y + row, W: rect.W, H: 1}, lineStyle)
		}
		rd.drawText(rect.X, rect.Y+row, label, lineStyle, rect.X+rect.W)
	}
	return nil
}

// tableColumns reads a table's columns spec into per-column widths.
func tableColumns(m map[string]any) ([]int, []string, error) {
	cols, _, err := nodeList(m, "columns")
	if err != nil {
		return nil, nil, fmt.Errorf("bad widget: %v", err)
	}
	widths := make([]int, len(cols))
	titles := make([]string, len(cols))
	for i, c := range cols {
		cm, ok := asNode(c)
		if !ok {
			return nil, nil, fmt.Errorf("bad widget: table columns must be {title width} Maps")
		}
		title, _, tErr := nodeStr(cm, "title")
		if tErr != nil {
			return nil, nil, fmt.Errorf("bad widget: %v", tErr)
		}
		width, has, wErr := nodeInt(cm, "width")
		if wErr != nil {
			return nil, nil, fmt.Errorf("bad widget: %v", wErr)
		}
		if !has {
			width = StringWidth(title) + 2
		}
		if width < 1 {
			width = 1
		}
		widths[i] = width
		titles[i] = title
	}
	return widths, titles, nil
}

// cellText projects one table cell to text: strings pass through,
// numbers and booleans render canonically.
func cellText(v any) string {
	switch c := v.(type) {
	case string:
		return c
	case bool:
		return strconv.FormatBool(c)
	default:
		if n, ok := toInt(v); ok {
			return strconv.Itoa(n)
		}
		return fmt.Sprintf("%v", v)
	}
}

func (rd *renderer) paintTable(m map[string]any, rect Rect, st Style) error {
	widths, titles, err := tableColumns(m)
	if err != nil {
		return err
	}
	dataRows, _, err := nodeList(m, "rows")
	if err != nil {
		return fmt.Errorf("bad widget: %v", err)
	}
	cursor, _, err := nodeInt(m, "cursor")
	if err != nil {
		return fmt.Errorf("bad widget: %v", err)
	}
	drawRow := func(y int, cells []string, rowStyle Style) {
		x := rect.X
		for i, w := range widths {
			if i < len(cells) {
				label, _ := truncateToWidth(cells[i], w)
				rd.drawText(x, y, label, rowStyle, rect.X+rect.W)
			}
			x += w + 1
		}
	}
	headStyle := st
	headStyle.Bold = true
	drawRow(rect.Y, titles, headStyle)
	body := rect.H - 1
	off := scrollOffset(cursor, len(dataRows), body)
	for row := 0; row < body && off+row < len(dataRows); row++ {
		cellsAny, ok := dataRows[off+row].([]any)
		if !ok {
			return fmt.Errorf("bad widget: table rows must be Lists of cells")
		}
		cells := make([]string, len(cellsAny))
		for i, c := range cellsAny {
			cells[i] = cellText(c)
		}
		rowStyle := st
		y := rect.Y + 1 + row
		if off+row == cursor {
			rowStyle.Reverse = !rowStyle.Reverse
			rd.fillStyle(Rect{X: rect.X, Y: y, W: rect.W, H: 1}, rowStyle)
		}
		drawRow(y, cells, rowStyle)
	}
	return nil
}

func (rd *renderer) paintInput(m map[string]any, rect Rect, st Style) error {
	value, _, err := nodeStr(m, "value")
	if err != nil {
		return fmt.Errorf("bad widget: %v", err)
	}
	placeholder, _, err := nodeStr(m, "placeholder")
	if err != nil {
		return fmt.Errorf("bad widget: %v", err)
	}
	cursor, _, err := nodeInt(m, "cursor")
	if err != nil {
		return fmt.Errorf("bad widget: %v", err)
	}
	focus, _, err := nodeBool(m, "focus")
	if err != nil {
		return fmt.Errorf("bad widget: %v", err)
	}
	mask, _, err := nodeBool(m, "mask")
	if err != nil {
		return fmt.Errorf("bad widget: %v", err)
	}
	shown, shownStyle := value, st
	if mask {
		// one narrow bullet per rune — the value itself never paints
		shown = strings.Repeat("•", utf8.RuneCountInString(value))
	}
	if value == "" && placeholder != "" {
		// the placeholder is a hint, not a secret — never masked
		shown = placeholder
		shownStyle.FG = "grey"
	}
	rd.drawText(rect.X, rect.Y, shown, shownStyle, rect.X+rect.W)
	if focus {
		rd.focused++
		prefix := prefixByRunes(value, cursor)
		w := StringWidth(prefix)
		if mask {
			w = utf8.RuneCountInString(prefix)
		}
		x := rect.X + w
		if max := rect.X + rect.W - 1; x > max {
			x = max
		}
		rd.cursor = CursorPos{X: x, Y: rect.Y}
	}
	return nil
}

// prefixByRunes returns the first n runes of s (the input cursor is a
// rune index — Tui.edit's fold unit).
func prefixByRunes(s string, n int) string {
	if n <= 0 {
		return ""
	}
	seen := 0
	for i := range s {
		if seen == n {
			return s[:i]
		}
		seen++
	}
	return s
}

func (rd *renderer) paintViewport(m map[string]any, rect Rect, st Style) error {
	child, ok := m["child"]
	if !ok {
		return nil
	}
	ox, oy := 0, 0
	if off, has, err := nodeList(m, "offset"); err != nil {
		return fmt.Errorf("bad widget: %v", err)
	} else if has {
		if len(off) != 2 {
			return fmt.Errorf("bad widget: offset: must be [x y]")
		}
		x, okX := toInt(off[0])
		y, okY := toInt(off[1])
		if !okX || !okY || x < 0 || y < 0 {
			return fmt.Errorf("bad widget: offset: must be two non-negative Integers")
		}
		ox, oy = x, y
	}
	cw, ch, err := measureFit(child, rect.W)
	if err != nil {
		return err
	}
	if cw < rect.W {
		cw = rect.W
	}
	if ch < rect.H {
		ch = rect.H
	}
	if cw > viewportMaxCols {
		cw = viewportMaxCols
	}
	if ch > viewportMaxRows {
		ch = viewportMaxRows
	}
	sub := &renderer{frame: NewFrame(cw, ch)}
	if err := sub.paint(child, Rect{X: 0, Y: 0, W: cw, H: ch}, st); err != nil {
		return err
	}
	// blit the offset window; a focused input inside a viewport does not
	// place the hardware cursor (its frame coordinates are virtual)
	for y := 0; y < rect.H; y++ {
		for x := 0; x < rect.W; x++ {
			if c, ok := sub.frame.At(ox+x, oy+y); ok {
				rd.frame.Set(rect.X+x, rect.Y+y, c)
			}
		}
	}
	return nil
}
