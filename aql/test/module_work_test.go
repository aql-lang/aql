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
		`("#FF0000" Color.hex2rgb) .r`,
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
		`"#FF8800" Color.hex2rgb`,
	})
	if err != nil {
		t.Fatal(err)
	}
	m := result[0].AsMap()
	rv, _ := m.Get("r")
	gv, _ := m.Get("g")
	bv, _ := m.Get("b")
	if rv.AsInteger() != 255 {
		t.Errorf("r = %d, want 255", rv.AsInteger())
	}
	if gv.AsInteger() != 136 {
		t.Errorf("g = %d, want 136", gv.AsInteger())
	}
	if bv.AsInteger() != 0 {
		t.Errorf("b = %d, want 0", bv.AsInteger())
	}
}

func TestColorRgb2hex(t *testing.T) {
	dir := moduleWorkDir(t)
	result, err := runRealFileSteps(t, dir, []string{
		`(import "./color")`,
		`do {r:[255] g:[136] b:[0]} Color.rgb2hex`,
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
		`"FF" Color.hex2int`,
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
		`255 Color.int2hex`,
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
		`("#FF8800" Color.make-color) .hex`,
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
		`"#A0B0C0" Color.hex2rgb Color.rgb2hex`,
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
		`300 Color.clamp`,
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
		`-10 Color.clamp`,
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
		s := result[0].AsString()
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
		r := result[0].AsInteger()
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
		`("#00FF00" Color.hex2rgb) .g`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "255")
}
