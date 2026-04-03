package test

import (
	"path/filepath"
	"testing"
)

func moduleWorkDir(t *testing.T) string {
	t.Helper()
	abs, err := filepath.Abs("module-work")
	if err != nil {
		t.Fatal(err)
	}
	return abs
}

// --- Color module: exported conversion functions via map dot-access ---

func TestColorHex2rgbRed(t *testing.T) {
	dir := moduleWorkDir(t)
	result, err := runRealFileSteps(t, dir, []string{
		`(import "./color")`,
		`"#FF0000" color.hex2rgb .r`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "255")
}

func TestColorHex2rgbComponents(t *testing.T) {
	dir := moduleWorkDir(t)
	result, err := runRealFileSteps(t, dir, []string{
		`(import "./color")`,
		`"#FF8800" color.hex2rgb`,
	})
	if err != nil {
		t.Fatal(err)
	}
	m := result[0].AsMap()
	rv, _ := m.Get("r")
	gv, _ := m.Get("g")
	bv, _ := m.Get("b")
	rvi, _ := rv.AsInteger()
	gvi, _ := gv.AsInteger()
	bvi, _ := bv.AsInteger()
	if rvi != 255 {
		t.Errorf("r = %d, want 255", rvi)
	}
	if gvi != 136 {
		t.Errorf("g = %d, want 136", gvi)
	}
	if bvi != 0 {
		t.Errorf("b = %d, want 0", bvi)
	}
}

func TestColorRgb2hex(t *testing.T) {
	dir := moduleWorkDir(t)
	result, err := runRealFileSteps(t, dir, []string{
		`(import "./color")`,
		`{r:255 g:136 b:0} color.rgb2hex`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "'#FF8800'")
}

func TestColorHex2int(t *testing.T) {
	dir := moduleWorkDir(t)
	result, err := runRealFileSteps(t, dir, []string{
		`(import "./color")`,
		`"FF" color.hex2int`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "255")
}

func TestColorInt2hex(t *testing.T) {
	dir := moduleWorkDir(t)
	result, err := runRealFileSteps(t, dir, []string{
		`(import "./color")`,
		`255 color.int2hex`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "'FF'")
}

func TestColorMakeColor(t *testing.T) {
	dir := moduleWorkDir(t)
	result, err := runRealFileSteps(t, dir, []string{
		`(import "./color")`,
		`("#FF8800" color.make-color) .hex`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "'#FF8800'")
}

func TestColorRoundTrip(t *testing.T) {
	dir := moduleWorkDir(t)
	result, err := runRealFileSteps(t, dir, []string{
		`(import "./color")`,
		`"#A0B0C0" color.hex2rgb color.rgb2hex`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "'#A0B0C0'")
}

func TestColorClamp(t *testing.T) {
	dir := moduleWorkDir(t)
	result, err := runRealFileSteps(t, dir, []string{
		`(import "./color")`,
		`300 color.clamp`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "255")
}

func TestColorClampNegative(t *testing.T) {
	dir := moduleWorkDir(t)
	result, err := runRealFileSteps(t, dir, []string{
		`(import "./color")`,
		`-10 color.clamp`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "0")
}

// --- Color-scheme module: uses color.aql's exported functions ---

func TestColorSchemeSunsetName(t *testing.T) {
	dir := moduleWorkDir(t)
	result, err := runRealFileSteps(t, dir, []string{
		`(import "./color-scheme")`,
		`Schemes.sunset.name`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "'sunset'")
}

func TestColorSchemeSunsetPrimaryHex(t *testing.T) {
	dir := moduleWorkDir(t)
	result, err := runRealFileSteps(t, dir, []string{
		`(import "./color-scheme")`,
		`Schemes.sunset.primary.hex`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "'#E63946'")
}

func TestColorSchemeSunsetPrimaryRed(t *testing.T) {
	dir := moduleWorkDir(t)
	result, err := runRealFileSteps(t, dir, []string{
		`(import "./color-scheme")`,
		`Schemes.sunset.primary.r`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "230")
}

func TestColorSchemeOceanPrimaryHex(t *testing.T) {
	dir := moduleWorkDir(t)
	result, err := runRealFileSteps(t, dir, []string{
		`(import "./color-scheme")`,
		`Schemes.ocean.primary.hex`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "'#264653'")
}

func TestColorSchemeOceanAccentGreen(t *testing.T) {
	dir := moduleWorkDir(t)
	result, err := runRealFileSteps(t, dir, []string{
		`(import "./color-scheme")`,
		`Schemes.ocean.accent.g`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "196")
}

func TestColorSchemeNeonPrimaryHex(t *testing.T) {
	dir := moduleWorkDir(t)
	result, err := runRealFileSteps(t, dir, []string{
		`(import "./color-scheme")`,
		`Schemes.neon.primary.hex`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "'#FF006E'")
}

func TestColorSchemeNeonDarkBlue(t *testing.T) {
	dir := moduleWorkDir(t)
	result, err := runRealFileSteps(t, dir, []string{
		`(import "./color-scheme")`,
		`Schemes.neon.dark.b`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "236")
}

func TestColorSchemeHasBothHexAndRGB(t *testing.T) {
	dir := moduleWorkDir(t)
	for _, field := range []string{"primary", "secondary", "accent", "dark", "background"} {
		result, err := runRealFileSteps(t, dir, []string{
			`(import "./color-scheme")`,
			`Schemes.sunset.` + field + `.hex`,
		})
		if err != nil {
			t.Fatalf("sunset.%s.hex: %v", field, err)
		}
		s, _ := result[0].AsString()
		if len(s) != 7 || s[0] != '#' {
			t.Errorf("sunset.%s.hex = %q, want #XXXXXX format", field, s)
		}

		result, err = runRealFileSteps(t, dir, []string{
			`(import "./color-scheme")`,
			`Schemes.sunset.` + field + `.r`,
		})
		if err != nil {
			t.Fatalf("sunset.%s.r: %v", field, err)
		}
		r, _ := result[0].AsInteger()
		if r < 0 || r > 255 {
			t.Errorf("sunset.%s.r = %d, want 0-255", field, r)
		}
	}
}

// --- Project module: bare import of installed module ---

func TestProjectImportInstalledColor(t *testing.T) {
	dir := moduleWorkDir(t)
	projDir := filepath.Join(dir, "project")
	result, err := runRealFileSteps(t, projDir, []string{
		`(import "color")`,
		`"#00FF00" color.hex2rgb .g`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "255")
}

// --- Map quotation: word values resolve without list wrappers ---

func TestDoMapWordValuesResolve(t *testing.T) {
	dir := moduleWorkDir(t)
	result, err := runRealFileSteps(t, dir, []string{
		`(import "./color")`,
		`def r 100 def g 200 def b 50`,
		`do {r:r g:g b:b}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	m := result[0].AsMap()
	rv, _ := m.Get("r")
	gv, _ := m.Get("g")
	bv, _ := m.Get("b")
	rvi2, _ := rv.AsInteger()
	gvi2, _ := gv.AsInteger()
	bvi2, _ := bv.AsInteger()
	if rvi2 != 100 {
		t.Errorf("r = %d, want 100", rvi2)
	}
	if gvi2 != 200 {
		t.Errorf("g = %d, want 200", gvi2)
	}
	if bvi2 != 50 {
		t.Errorf("b = %d, want 50", bvi2)
	}
}

func TestDoMapLiteralValues(t *testing.T) {
	dir := moduleWorkDir(t)
	result, err := runRealFileSteps(t, dir, []string{
		`do {x:1 y:"hello" z:true}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	m := result[0].AsMap()
	xv, _ := m.Get("x")
	yv, _ := m.Get("y")
	zv, _ := m.Get("z")
	xvi, _ := xv.AsInteger()
	yvs, _ := yv.AsString()
	zvb, _ := zv.AsBoolean()
	if xvi != 1 {
		t.Errorf("x = %v, want 1", xv)
	}
	if yvs != "hello" {
		t.Errorf("y = %v, want hello", yv)
	}
	if !zvb {
		t.Errorf("z = %v, want true", zv)
	}
}

func TestDoMapListValuesStillWork(t *testing.T) {
	dir := moduleWorkDir(t)
	result, err := runRealFileSteps(t, dir, []string{
		`do {x:[3 add 4] y:[upper "a"]}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	m := result[0].AsMap()
	xv, _ := m.Get("x")
	yv, _ := m.Get("y")
	xvi2, _ := xv.AsInteger()
	if xvi2 != 7 {
		t.Errorf("x = %v, want 7", xv)
	}
	yvs2, _ := yv.AsString()
	if yvs2 != "A" {
		t.Errorf("y = %v, want A", yv)
	}
}

// --- Module export: word values in export maps resolve to defs ---

func TestExportMapWordValues(t *testing.T) {
	dir := moduleWorkDir(t)
	result, err := runRealFileSteps(t, dir, []string{
		`(import "./color")`,
		`"AA" color.hex2int`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "170")
}

// --- Color module: function chaining across multiple calls ---

func TestColorChainHex2rgbThenRgb2hex(t *testing.T) {
	dir := moduleWorkDir(t)
	result, err := runRealFileSteps(t, dir, []string{
		`(import "./color")`,
		`"#123456" color.hex2rgb color.rgb2hex`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "'#123456'")
}

func TestColorChainMakeColorThenAccessRGB(t *testing.T) {
	dir := moduleWorkDir(t)
	result, err := runRealFileSteps(t, dir, []string{
		`(import "./color")`,
		`("#AABB00" color.make-color) .g`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "187")
}

// --- Color module: hex edge cases ---

func TestColorHex2rgbBlack(t *testing.T) {
	dir := moduleWorkDir(t)
	result, err := runRealFileSteps(t, dir, []string{
		`(import "./color")`,
		`"#000000" color.hex2rgb`,
	})
	if err != nil {
		t.Fatal(err)
	}
	m := result[0].AsMap()
	rv, _ := m.Get("r")
	gv, _ := m.Get("g")
	bv, _ := m.Get("b")
	rvb, _ := rv.AsInteger()
	gvb, _ := gv.AsInteger()
	bvb, _ := bv.AsInteger()
	if rvb != 0 || gvb != 0 || bvb != 0 {
		t.Errorf("want {r:0 g:0 b:0}, got {r:%d g:%d b:%d}", rvb, gvb, bvb)
	}
}

func TestColorHex2rgbWhite(t *testing.T) {
	dir := moduleWorkDir(t)
	result, err := runRealFileSteps(t, dir, []string{
		`(import "./color")`,
		`"#FFFFFF" color.hex2rgb`,
	})
	if err != nil {
		t.Fatal(err)
	}
	m := result[0].AsMap()
	rv, _ := m.Get("r")
	gv, _ := m.Get("g")
	bv, _ := m.Get("b")
	rvw, _ := rv.AsInteger()
	gvw, _ := gv.AsInteger()
	bvw, _ := bv.AsInteger()
	if rvw != 255 || gvw != 255 || bvw != 255 {
		t.Errorf("want {r:255 g:255 b:255}, got {r:%d g:%d b:%d}", rvw, gvw, bvw)
	}
}

// --- Color module: clamp boundary values ---

func TestColorClampZero(t *testing.T) {
	dir := moduleWorkDir(t)
	result, err := runRealFileSteps(t, dir, []string{
		`(import "./color")`,
		`0 color.clamp`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "0")
}

func TestColorClampMax(t *testing.T) {
	dir := moduleWorkDir(t)
	result, err := runRealFileSteps(t, dir, []string{
		`(import "./color")`,
		`255 color.clamp`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "255")
}

func TestColorClampMiddle(t *testing.T) {
	dir := moduleWorkDir(t)
	result, err := runRealFileSteps(t, dir, []string{
		`(import "./color")`,
		`128 color.clamp`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "128")
}

// --- Color-scheme module: all three schemes have consistent structure ---

func TestColorSchemeAllSchemesHaveExpectedFields(t *testing.T) {
	dir := moduleWorkDir(t)
	schemes := []string{"sunset", "ocean", "neon"}
	fields := []string{"name", "primary", "secondary", "accent", "dark", "background"}
	for _, scheme := range schemes {
		for _, field := range fields {
			_, err := runRealFileSteps(t, dir, []string{
				`(import "./color-scheme")`,
				`Schemes.` + scheme + `.` + field,
			})
			if err != nil {
				t.Errorf("Schemes.%s.%s: %v", scheme, field, err)
			}
		}
	}
}

// --- Cross-module: color-scheme uses color module functions ---

func TestColorSchemeOceanSecondaryRoundTrip(t *testing.T) {
	dir := moduleWorkDir(t)
	result, err := runRealFileSteps(t, dir, []string{
		`(import "./color-scheme")`,
		`(import "./color")`,
		`Schemes.ocean.secondary.hex`,
	})
	if err != nil {
		t.Fatal(err)
	}
	hex, _ := result[0].AsString()

	result, err = runRealFileSteps(t, dir, []string{
		`(import "./color")`,
		`"` + hex + `" color.hex2rgb color.rgb2hex`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "'"+hex+"'")
}

// --- Map quotation with do: mixed word and list values ---

func TestDoMapMixedWordAndListValues(t *testing.T) {
	dir := moduleWorkDir(t)
	result, err := runRealFileSteps(t, dir, []string{
		`def n 10`,
		`do {a:n b:[n add 5] c:"literal"}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	m := result[0].AsMap()
	av, _ := m.Get("a")
	bv, _ := m.Get("b")
	cv, _ := m.Get("c")
	avi, _ := av.AsInteger()
	bvi3, _ := bv.AsInteger()
	cvs, _ := cv.AsString()
	if avi != 10 {
		t.Errorf("a = %d, want 10", avi)
	}
	if bvi3 != 15 {
		t.Errorf("b = %d, want 15", bvi3)
	}
	if cvs != "literal" {
		t.Errorf("c = %v, want literal", cv)
	}
}
