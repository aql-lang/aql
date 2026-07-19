package tuikit

import (
	"encoding/json"
	"testing"
)

// --- width ---

func TestRuneAndStringWidth(t *testing.T) {
	cases := []struct {
		r    rune
		want int
	}{
		{0, 0}, {'́', 0}, {'​', 0}, {'️', 0},
		{'a', 1}, {'-', 1}, {'é', 1},
		{'漢', 2}, {'あ', 2}, {'한', 2}, {'Ａ', 2}, {'🙂', 2}, {'🚀', 2}, {'🧠', 2},
	}
	for _, c := range cases {
		if got := RuneWidth(c.r); got != c.want {
			t.Errorf("RuneWidth(%q) = %d, want %d", c.r, got, c.want)
		}
	}
	if got := StringWidth("a漢b🙂"); got != 6 {
		t.Errorf("StringWidth = %d, want 6", got)
	}
	if got := StringWidth("é"); got != 1 {
		t.Errorf("combining width = %d, want 1", got)
	}
}

func TestTruncateToWidth(t *testing.T) {
	s, w := truncateToWidth("a漢b", 3)
	if s != "a漢" || w != 3 {
		t.Errorf("truncate = %q,%d", s, w)
	}
	s, w = truncateToWidth("a漢b", 2)
	if s != "a" || w != 1 {
		t.Errorf("truncate wide split = %q,%d", s, w)
	}
	s, w = truncateToWidth("ab", 5)
	if s != "ab" || w != 2 {
		t.Errorf("truncate full = %q,%d", s, w)
	}
}

// --- color ---

func TestParseColor(t *testing.T) {
	if c, ok := ParseColor(""); !ok || c.Kind != ColorNone {
		t.Errorf("empty = %+v,%v", c, ok)
	}
	if c, ok := ParseColor("Red"); !ok || c.Kind != ColorNamed || c.Index != 1 {
		t.Errorf("named = %+v,%v", c, ok)
	}
	if c, ok := ParseColor("grey"); !ok || c.Index != 8 {
		t.Errorf("grey = %+v,%v", c, ok)
	}
	if c, ok := ParseColor("#20A0f0"); !ok || c.Kind != ColorRGB || c.R != 0x20 || c.G != 0xA0 || c.B != 0xF0 {
		t.Errorf("hex = %+v,%v", c, ok)
	}
	if _, ok := ParseColor("#123"); ok {
		t.Error("short hex accepted")
	}
	if _, ok := ParseColor("#zzzzzz"); ok {
		t.Error("bad hex digits accepted")
	}
	if c, ok := ParseColor("203"); !ok || c.Kind != ColorIndex || c.Index != 203 {
		t.Errorf("index = %+v,%v", c, ok)
	}
	if _, ok := ParseColor("999"); ok {
		t.Error("out-of-range index accepted")
	}
	if _, ok := ParseColor("blurple"); ok {
		t.Error("unknown name accepted")
	}
}

func TestColorDegradation(t *testing.T) {
	if got := (Color{Kind: ColorNamed, Index: 3}).To256(); got != 3 {
		t.Errorf("named To256 = %d", got)
	}
	if got := (Color{Kind: ColorIndex, Index: 200}).To256(); got != 200 {
		t.Errorf("index To256 = %d", got)
	}
	if got := (Color{Kind: ColorNone}).To256(); got != -1 {
		t.Errorf("none To256 = %d", got)
	}
	// grey ramp: dark clamp, light clamp, middle
	if got := (Color{Kind: ColorRGB, R: 2, G: 2, B: 2}).To256(); got != 16 {
		t.Errorf("near-black To256 = %d", got)
	}
	if got := (Color{Kind: ColorRGB, R: 250, G: 250, B: 250}).To256(); got != 231 {
		t.Errorf("near-white To256 = %d", got)
	}
	mid := (Color{Kind: ColorRGB, R: 128, G: 128, B: 128}).To256()
	if mid < 232 || mid > 255 {
		t.Errorf("mid-grey To256 = %d", mid)
	}
	// cube
	cube := (Color{Kind: ColorRGB, R: 0x20, G: 0xA0, B: 0xF0}).To256()
	if cube < 16 || cube > 231 {
		t.Errorf("cube To256 = %d", cube)
	}

	if got := (Color{Kind: ColorNamed, Index: 5}).To16(); got != 5 {
		t.Errorf("named To16 = %d", got)
	}
	if got := (Color{Kind: ColorIndex, Index: 9}).To16(); got != 9 {
		t.Errorf("low index To16 = %d", got)
	}
	if got := (Color{Kind: ColorIndex, Index: 240}).To16(); got < 0 || got > 15 {
		t.Errorf("grey-ramp index To16 = %d", got)
	}
	if got := (Color{Kind: ColorIndex, Index: 196}).To16(); got != 9 { // pure bright red cube entry
		t.Errorf("cube index To16 = %d", got)
	}
	if got := (Color{Kind: ColorRGB, R: 255, G: 0, B: 0}).To16(); got != 9 {
		t.Errorf("rgb To16 = %d", got)
	}
	if got := (Color{Kind: ColorRGB, R: 100, G: 0, B: 0}).To16(); got != 0 {
		t.Errorf("dim rgb To16 = %d", got)
	}
	if got := (Color{Kind: ColorNone}).To16(); got != -1 {
		t.Errorf("none To16 = %d", got)
	}
	// rgbFrom256 dim-base picks
	if rgb := rgbFrom256(3); rgb != [3]int{128, 128, 0} {
		t.Errorf("rgbFrom256(3) = %v", rgb)
	}
	if rgb := rgbFrom256(12); rgb != [3]int{85, 85, 255} {
		t.Errorf("rgbFrom256(12) = %v", rgb)
	}
}

// --- toInt / node readers / size specs ---

func TestToIntShapes(t *testing.T) {
	cases := []struct {
		v    any
		want int
		ok   bool
	}{
		{3, 3, true}, {int64(4), 4, true}, {5.0, 5, true}, {5.5, 0, false},
		{json.Number("6"), 6, true}, {json.Number("6.5"), 0, false},
		{"x", 0, false},
	}
	for _, c := range cases {
		got, ok := toInt(c.v)
		if got != c.want || ok != c.ok {
			t.Errorf("toInt(%v) = %d,%v", c.v, got, ok)
		}
	}
}

func TestNodeReaders(t *testing.T) {
	m := map[string]any{"s": "x", "n": 3, "b": true, "l": []any{1}, "bad": struct{}{}}
	if s, has, err := nodeStr(m, "s"); s != "x" || !has || err != nil {
		t.Error("nodeStr present")
	}
	if _, has, err := nodeStr(m, "missing"); has || err != nil {
		t.Error("nodeStr absent")
	}
	if _, _, err := nodeStr(m, "n"); err == nil {
		t.Error("nodeStr wrong type")
	}
	if n, has, err := nodeInt(m, "n"); n != 3 || !has || err != nil {
		t.Error("nodeInt present")
	}
	if _, has, err := nodeInt(m, "missing"); has || err != nil {
		t.Error("nodeInt absent")
	}
	if _, _, err := nodeInt(m, "s"); err == nil {
		t.Error("nodeInt wrong type")
	}
	if b, has, err := nodeBool(m, "b"); !b || !has || err != nil {
		t.Error("nodeBool present")
	}
	if _, has, err := nodeBool(m, "missing"); has || err != nil {
		t.Error("nodeBool absent")
	}
	if _, _, err := nodeBool(m, "s"); err == nil {
		t.Error("nodeBool wrong type")
	}
	if l, has, err := nodeList(m, "l"); len(l) != 1 || !has || err != nil {
		t.Error("nodeList present")
	}
	if _, has, err := nodeList(m, "missing"); has || err != nil {
		t.Error("nodeList absent")
	}
	if _, _, err := nodeList(m, "s"); err == nil {
		t.Error("nodeList wrong type")
	}
}

func TestParseSizeSpec(t *testing.T) {
	spec := func(v any) map[string]any { return map[string]any{"size": v} }
	if s, err := parseSizeSpec(map[string]any{}); err != nil || s.kind != sizeFlex || s.n != 1 {
		t.Errorf("default = %+v,%v", s, err)
	}
	if s, err := parseSizeSpec(spec(7)); err != nil || s.kind != sizeFixed || s.n != 7 {
		t.Errorf("fixed = %+v,%v", s, err)
	}
	if s, err := parseSizeSpec(spec(-3)); err != nil || s.n != 0 {
		t.Errorf("negative fixed = %+v,%v", s, err)
	}
	if s, err := parseSizeSpec(spec("fit")); err != nil || s.kind != sizeFit {
		t.Errorf("fit = %+v,%v", s, err)
	}
	if _, err := parseSizeSpec(spec("huge")); err == nil {
		t.Error("bad keyword accepted")
	}
	if s, err := parseSizeSpec(spec(map[string]any{"pct": 30})); err != nil || s.kind != sizePct || s.n != 30 {
		t.Errorf("pct = %+v,%v", s, err)
	}
	if _, err := parseSizeSpec(spec(map[string]any{"pct": "x"})); err == nil {
		t.Error("bad pct accepted")
	}
	if s, err := parseSizeSpec(spec(map[string]any{"flex": 2})); err != nil || s.kind != sizeFlex || s.n != 2 {
		t.Errorf("flex = %+v,%v", s, err)
	}
	if s, err := parseSizeSpec(spec(map[string]any{"flex": 0})); err != nil || s.n != 1 {
		t.Errorf("flex clamp = %+v,%v", s, err)
	}
	if _, err := parseSizeSpec(spec(map[string]any{"flex": "x"})); err == nil {
		t.Error("bad flex accepted")
	}
	if _, err := parseSizeSpec(spec(map[string]any{"nope": 1})); err == nil {
		t.Error("empty size map accepted")
	}
	if _, err := parseSizeSpec(spec(true)); err == nil {
		t.Error("bool size accepted")
	}
}

func TestSolveSizes(t *testing.T) {
	eq := func(got, want []int) bool {
		if len(got) != len(want) {
			return false
		}
		for i := range got {
			if got[i] != want[i] {
				return false
			}
		}
		return true
	}
	if got := solveSizes(10, 0, nil, nil); len(got) != 0 {
		t.Errorf("empty = %v", got)
	}
	// fixed + pct + fit + flex resolution order
	specs := []sizeSpec{
		{kind: sizeFixed, n: 3},
		{kind: sizePct, n: 50},
		{kind: sizeFit},
		{kind: sizeFlex, n: 1},
	}
	got := solveSizes(20, 0, specs, []int{0, 0, 4, 0})
	if !eq(got, []int{3, 10, 4, 3}) {
		t.Errorf("mixed = %v", got)
	}
	// flex largest-remainder: 10 cells across weights 1,1,1 → 4,3,3
	got = solveSizes(10, 0, []sizeSpec{{kind: sizeFlex, n: 1}, {kind: sizeFlex, n: 1}, {kind: sizeFlex, n: 1}}, make([]int, 3))
	if !eq(got, []int{4, 3, 3}) {
		t.Errorf("flex remainder = %v", got)
	}
	sum := 0
	for _, n := range got {
		sum += n
	}
	if sum != 10 {
		t.Errorf("flex does not sum exactly: %v", got)
	}
	// gaps subtract from the pool; fixed overflow clamps to remaining
	got = solveSizes(10, 2, []sizeSpec{{kind: sizeFixed, n: 9}, {kind: sizeFixed, n: 9}}, make([]int, 2))
	if !eq(got, []int{8, 0}) {
		t.Errorf("gap+overflow = %v", got)
	}
	// negative pct claims nothing
	got = solveSizes(10, 0, []sizeSpec{{kind: sizePct, n: -50}, {kind: sizeFlex, n: 1}}, make([]int, 2))
	if !eq(got, []int{0, 10}) {
		t.Errorf("negative pct = %v", got)
	}
	// gaps wider than the total clamp the pool to zero
	got = solveSizes(1, 5, []sizeSpec{{kind: sizeFlex, n: 1}, {kind: sizeFlex, n: 1}}, make([]int, 2))
	if !eq(got, []int{0, 0}) {
		t.Errorf("gap overflow = %v", got)
	}
}

// --- diff ---

func TestDiffFramesTotalDamage(t *testing.T) {
	next := NewFrame(2, 2)
	next.Set(0, 0, Cell{Content: "a"})
	if spans := DiffFrames(nil, next); len(spans) != 2 || spans[0].Y != 0 || len(spans[0].Cells) != 2 {
		t.Errorf("nil prev spans = %+v", spans)
	}
	prev := NewFrame(3, 2)
	if spans := DiffFrames(prev, next); len(spans) != 2 {
		t.Errorf("resize spans = %+v", spans)
	}
}

func TestDiffFramesRuns(t *testing.T) {
	prev := NewFrame(5, 2)
	next := NewFrame(5, 2)
	if spans := DiffFrames(prev, next); spans != nil {
		t.Errorf("identical frames diffed: %+v", spans)
	}
	next.Set(1, 0, Cell{Content: "a"})
	next.Set(2, 0, Cell{Content: "b"})
	next.Set(4, 0, Cell{Content: "c"})
	next.Set(0, 1, Cell{Content: "d"})
	spans := DiffFrames(prev, next)
	if len(spans) != 3 {
		t.Fatalf("spans = %+v", spans)
	}
	if spans[0].X != 1 || spans[0].Y != 0 || len(spans[0].Cells) != 2 {
		t.Errorf("span0 = %+v", spans[0])
	}
	if spans[1].X != 4 || len(spans[1].Cells) != 1 {
		t.Errorf("span1 = %+v", spans[1])
	}
	if spans[2].Y != 1 || spans[2].X != 0 {
		t.Errorf("span2 = %+v", spans[2])
	}
}
