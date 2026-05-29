package test

import (
	"path/filepath"
	"testing"

	"github.com/aql-lang/aql/lang/go/native"
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
		`( "#FF0000" color.hex2rgb ) .r`,
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
	m, _ := native.AsMap(result[0])
	rv, _ := m.Get("r")
	gv, _ := m.Get("g")
	bv, _ := m.Get("b")
	rvi, _ := native.AsInteger(rv)
	gvi, _ := native.AsInteger(gv)
	bvi, _ := native.AsInteger(bv)
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
		s, _ := native.AsString(result[0])
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
		r, _ := native.AsInteger(result[0])
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
		`( "#00FF00" color.hex2rgb ) .g`,
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
	m, _ := native.AsMap(result[0])
	rv, _ := m.Get("r")
	gv, _ := m.Get("g")
	bv, _ := m.Get("b")
	rvi2, _ := native.AsInteger(rv)
	gvi2, _ := native.AsInteger(gv)
	bvi2, _ := native.AsInteger(bv)
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
	m, _ := native.AsMap(result[0])
	xv, _ := m.Get("x")
	yv, _ := m.Get("y")
	zv, _ := m.Get("z")
	xvi, _ := native.AsInteger(xv)
	yvs, _ := native.AsString(yv)
	zvb, _ := native.AsBoolean(zv)
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
	m, _ := native.AsMap(result[0])
	xv, _ := m.Get("x")
	yv, _ := m.Get("y")
	xvi2, _ := native.AsInteger(xv)
	if xvi2 != 7 {
		t.Errorf("x = %v, want 7", xv)
	}
	yvs2, _ := native.AsString(yv)
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
	m, _ := native.AsMap(result[0])
	rv, _ := m.Get("r")
	gv, _ := m.Get("g")
	bv, _ := m.Get("b")
	rvb, _ := native.AsInteger(rv)
	gvb, _ := native.AsInteger(gv)
	bvb, _ := native.AsInteger(bv)
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
	m, _ := native.AsMap(result[0])
	rv, _ := m.Get("r")
	gv, _ := m.Get("g")
	bv, _ := m.Get("b")
	rvw, _ := native.AsInteger(rv)
	gvw, _ := native.AsInteger(gv)
	bvw, _ := native.AsInteger(bv)
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
	hex, _ := native.AsString(result[0])

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
	m, _ := native.AsMap(result[0])
	av, _ := m.Get("a")
	bv, _ := m.Get("b")
	cv, _ := m.Get("c")
	avi, _ := native.AsInteger(av)
	bvi3, _ := native.AsInteger(bv)
	cvs, _ := native.AsString(cv)
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

// --- Color type: make Color produces validated record maps ---

func TestColorMakeColorTypeFields(t *testing.T) {
	// make Color validates field types (Integer)
	dir := moduleWorkDir(t)
	result, err := runRealFileSteps(t, dir, []string{
		`(import "./color")`,
		`"#1A2B3C" color.hex2rgb`,
	})
	if err != nil {
		t.Fatal(err)
	}
	m, _ := native.AsMap(result[0])
	for _, key := range []string{"r", "g", "b"} {
		v, ok := m.Get(key)
		if !ok {
			t.Errorf("missing field %s in Color record", key)
			continue
		}
		_, err := native.AsInteger(v)
		if err != nil {
			t.Errorf("Color field %s should be Integer, got %s", key, v.String())
		}
	}
}

func TestColorHex2rgbUsesColorRecord(t *testing.T) {
	// hex2rgb uses make Color internally; result is accepted by rgb2hex (which expects Color param)
	dir := moduleWorkDir(t)
	result, err := runRealFileSteps(t, dir, []string{
		`(import "./color")`,
		`"#AABBCC" color.hex2rgb color.rgb2hex`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "'#AABBCC'")
}

func TestColorMakeColorTypeHasHex(t *testing.T) {
	// make-color returns ColorHex record with all 4 fields
	dir := moduleWorkDir(t)
	result, err := runRealFileSteps(t, dir, []string{
		`(import "./color")`,
		`"#DEADBE" color.make-color`,
	})
	if err != nil {
		t.Fatal(err)
	}
	m, _ := native.AsMap(result[0])
	for _, key := range []string{"hex", "r", "g", "b"} {
		if _, ok := m.Get(key); !ok {
			t.Errorf("missing field %s in ColorHex record", key)
		}
	}
	hexv, _ := m.Get("hex")
	s, _ := native.AsString(hexv)
	if s != "#DEADBE" {
		t.Errorf("hex = %q, want #DEADBE", s)
	}
}

func TestColorClampRgbEvaluatedMapValues(t *testing.T) {
	// clamp-rgb uses make Color with paren expressions as map values
	dir := moduleWorkDir(t)
	result, err := runRealFileSteps(t, dir, []string{
		`(import "./color")`,
		`{r:300 g:-10 b:128} color.clamp-rgb`,
	})
	if err != nil {
		t.Fatal(err)
	}
	m, _ := native.AsMap(result[0])
	rv, _ := m.Get("r")
	gv, _ := m.Get("g")
	bv, _ := m.Get("b")
	rvi, _ := native.AsInteger(rv)
	gvi, _ := native.AsInteger(gv)
	bvi, _ := native.AsInteger(bv)
	if rvi != 255 {
		t.Errorf("r = %d, want 255", rvi)
	}
	if gvi != 0 {
		t.Errorf("g = %d, want 0", gvi)
	}
	if bvi != 128 {
		t.Errorf("b = %d, want 128", bvi)
	}
}

func TestColorClampRgbRoundTrip(t *testing.T) {
	// clamp-rgb output feeds into rgb2hex (Color type accepted)
	dir := moduleWorkDir(t)
	result, err := runRealFileSteps(t, dir, []string{
		`(import "./color")`,
		`def clamped ({r:300 g:0 b:255} color.clamp-rgb)`,
		`clamped color.rgb2hex`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "'#FF00FF'")
}

// --- Color-scheme: evaluated map values in scheme definitions ---

func TestColorSchemeMapValuesEvaluated(t *testing.T) {
	// Scheme maps use evaluated word values (primary:accent1 etc.)
	// Verify that nested access resolves color values through def'd words.
	dir := moduleWorkDir(t)

	// Access the primary color's red component
	result, err := runRealFileSteps(t, dir, []string{
		`(import "./color-scheme")`,
		`Schemes.sunset.primary.r`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "230") // #E6 = 230

	// Access accent hex field (make-color returns ColorHex with hex)
	result, err = runRealFileSteps(t, dir, []string{
		`(import "./color-scheme")`,
		`Schemes.ocean.accent.hex`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "'#E9C46A'")
}

func TestColorSchemeEachSchemeHasColorFields(t *testing.T) {
	// All color fields in every scheme should be integers (from make Color)
	dir := moduleWorkDir(t)
	schemes := []string{"sunset", "ocean", "neon"}
	colorFields := []string{"primary", "secondary", "accent", "dark", "background"}
	rgbComponents := []string{"r", "g", "b"}

	for _, scheme := range schemes {
		for _, field := range colorFields {
			for _, comp := range rgbComponents {
				result, err := runRealFileSteps(t, dir, []string{
					`(import "./color-scheme")`,
					`Schemes.` + scheme + `.` + field + `.` + comp,
				})
				if err != nil {
					t.Errorf("Schemes.%s.%s.%s: %v", scheme, field, comp, err)
					continue
				}
				v, ierr := native.AsInteger(result[0])
				if ierr != nil {
					t.Errorf("Schemes.%s.%s.%s: expected Integer, got %s", scheme, field, comp, result[0].String())
				}
				if v < 0 || v > 255 {
					t.Errorf("Schemes.%s.%s.%s: %d not in 0-255", scheme, field, comp, v)
				}
			}
		}
	}
}

func TestColorSchemeHexFieldPresent(t *testing.T) {
	// make-color returns ColorHex, so schemes have hex field on each color
	dir := moduleWorkDir(t)
	result, err := runRealFileSteps(t, dir, []string{
		`(import "./color-scheme")`,
		`Schemes.neon.accent.hex`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "'#FFBE0B'")
}

// --- Color type with def and evaluated map values ---

func TestColorDefAndMapExpr(t *testing.T) {
	// Use def'd Color values inside evaluated map expressions
	dir := moduleWorkDir(t)
	result, err := runRealFileSteps(t, dir, []string{
		`(import "./color")`,
		`def red (color.hex2rgb "#FF0000")`,
		`def blue (color.hex2rgb "#0000FF")`,
		`{r1:(red .r) b2:(blue .b)}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	m, _ := native.AsMap(result[0])
	r1, _ := m.Get("r1")
	b2, _ := m.Get("b2")
	r1i, _ := native.AsInteger(r1)
	b2i, _ := native.AsInteger(b2)
	if r1i != 255 {
		t.Errorf("r1 = %d, want 255", r1i)
	}
	if b2i != 255 {
		t.Errorf("b2 = %d, want 255", b2i)
	}
}

func TestColorExportedTypeUsableDirectly(t *testing.T) {
	// The exported Color type can be accessed and used
	dir := moduleWorkDir(t)
	result, err := runRealFileSteps(t, dir, []string{
		`(import "./color")`,
		`color.Color`,
	})
	if err != nil {
		t.Fatal(err)
	}
	got := formatStack(result)
	if got != "record{r:Integer,g:Integer,b:Integer}" {
		t.Errorf("Color type = %s", got)
	}
}

func TestColorExportedColorHexType(t *testing.T) {
	// ColorHex type is exported with hex, r, g, b fields
	dir := moduleWorkDir(t)
	result, err := runRealFileSteps(t, dir, []string{
		`(import "./color")`,
		`color.ColorHex`,
	})
	if err != nil {
		t.Fatal(err)
	}
	got := formatStack(result)
	if got != "record{hex:String,r:Integer,g:Integer,b:Integer}" {
		t.Errorf("ColorHex type = %s", got)
	}
}
