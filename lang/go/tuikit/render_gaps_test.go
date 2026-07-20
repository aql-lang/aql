package tuikit

import (
	"strings"
	"testing"
)

// Coverage-closing arms the primary suites route around: measure-time
// error propagation (reached via size:"fit"), style bg, clamps, and the
// color-table branches.

func TestRenderMeasureErrorArms(t *testing.T) {
	fitChild := func(child map[string]any) any {
		child["size"] = "fit"
		return w("rows", "children", []any{child})
	}
	cases := []struct {
		name string
		tree any
		want string
	}{
		{"text wrap in measure", fitChild(map[string]any{"w": "text", "text": "x", "wrap": 5}), "wrap: must be a String"},
		{"rows children in measure", fitChild(map[string]any{"w": "rows", "children": 5}), "children: must be a List"},
		{"rows gap in measure", fitChild(map[string]any{"w": "rows", "children": []any{}, "gap": "x"}), "gap: must be an Integer"},
		{"box insets in measure", fitChild(map[string]any{"w": "box", "pad": 5}), "pad: must be a List"},
		{"box border in measure", fitChild(map[string]any{"w": "box", "border": 5}), "border: must be a String"},
		{"box child in measure", fitChild(map[string]any{"w": "box", "child": map[string]any{"w": "text", "text": 5}}), "text: must be a String"},
		{"list items in measure", fitChild(map[string]any{"w": "list-view", "items": 5}), "items: must be a List"},
		{"list item shape in measure", fitChild(map[string]any{"w": "list-view", "items": []any{5}}), "items must be Strings"},
		{"table columns in measure", fitChild(map[string]any{"w": "table", "columns": 5}), "columns: must be a List"},
		{"table rows in measure", fitChild(map[string]any{"w": "table", "columns": []any{}, "rows": 5}), "rows: must be a List"},
		{"input value in measure", fitChild(map[string]any{"w": "input", "value": 5}), "value: must be a String"},
		{"style bg error", w("text", "text", "x", "style", map[string]any{"bg": 5}), "bg: must be a String"},
		{"viewport sub-paint error", w("viewport", "child",
			w("rows", "children", []any{map[string]any{"w": "text", "text": "x", "size": true}})),
			"size: must be"},
	}
	for _, c := range cases {
		if got := renderErr(t, c.tree); !strings.Contains(got, c.want) {
			t.Errorf("%s: error = %q, want substring %q", c.name, got, c.want)
		}
	}
}

func TestRenderClampArms(t *testing.T) {
	// bg inheritance sets the cell background
	res := mustRender(t, w("text", "text", "x", "style", map[string]any{"bg": "#202030"}), 1, 1)
	if c, _ := res.Frame.At(0, 0); c.Style.BG != "#202030" {
		t.Errorf("bg = %+v", c.Style)
	}
	// a negative gap clamps to zero
	res = mustRender(t, w("cols", "gap", -3, "children", []any{
		w("text", "text", "a", "size", 1), w("text", "text", "b", "size", 1),
	}), 2, 1)
	if got := screenOf(res)[0]; got != "ab" {
		t.Errorf("negative gap = %q", got)
	}
	// a zero column width clamps to one
	res = mustRender(t, w("table",
		"columns", []any{map[string]any{"title": "t", "width": 0}},
		"rows", []any{}), 3, 1)
	if got := screenOf(res)[0]; got != "t  " {
		t.Errorf("clamped column = %q", got)
	}
	// a viewport child smaller than the window expands to fill it
	res = mustRender(t, w("viewport", "child", w("text", "text", "x")), 4, 2)
	if got := screenOf(res)[0]; got != "x   " {
		t.Errorf("expanded viewport = %q", got)
	}
	// a pathologically tall wrap clamps at the canvas row cap
	res = mustRender(t, w("viewport",
		"child", w("text", "text", strings.Repeat("y", 5000), "wrap", "wrap")), 1, 2)
	if got := screenOf(res)[0]; got != "y" {
		t.Errorf("row-capped viewport = %q", got)
	}
	// fillStyle clips silently when a rect leaves the frame
	rd := &renderer{frame: NewFrame(1, 1)}
	rd.fillStyle(Rect{X: 0, Y: 0, W: 3, H: 3}, Style{Bold: true})
	if c, _ := rd.frame.At(0, 0); !c.Style.Bold {
		t.Error("fillStyle skipped the in-bounds cell")
	}
}

func TestColorBranchArms(t *testing.T) {
	// isGreyish min-update path (green below red) via a non-grey cube hit
	if got := (Color{Kind: ColorRGB, R: 200, G: 40, B: 120}).To256(); got < 16 || got > 231 {
		t.Errorf("cube pick = %d", got)
	}
	// cubeStep middle band (48..114)
	if got := cubeStep(100); got != 1 {
		t.Errorf("cubeStep(100) = %d", got)
	}
	// ansi16 green and blue bits
	if got := (Color{Kind: ColorRGB, R: 0, G: 200, B: 0}).To16(); got != 10 {
		t.Errorf("green To16 = %d", got)
	}
	if got := (Color{Kind: ColorRGB, R: 0, G: 0, B: 140}).To16(); got != 4 {
		t.Errorf("blue To16 = %d", got)
	}
}
