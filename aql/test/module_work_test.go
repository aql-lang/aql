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

// --- Colour module: hex to RGB conversion ---

func TestColourModuleRedRGB(t *testing.T) {
	dir := moduleWorkDir(t)
	result, err := runRealFileSteps(t, dir, []string{
		`import "colour"`,
		`Colour red dot r dot`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "255")
}

func TestColourModuleRedHex(t *testing.T) {
	dir := moduleWorkDir(t)
	result, err := runRealFileSteps(t, dir, []string{
		`import "colour"`,
		`Colour red dot hex dot`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "'#FF0000'")
}

func TestColourModuleOrangeGreen(t *testing.T) {
	dir := moduleWorkDir(t)
	result, err := runRealFileSteps(t, dir, []string{
		`import "colour"`,
		`Colour orange dot g dot`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "128")
}

func TestColourModuleBlueComponents(t *testing.T) {
	dir := moduleWorkDir(t)
	result, err := runRealFileSteps(t, dir, []string{
		`import "colour"`,
		`Colour blue dot r dot`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "0")
}

func TestColourModuleBlueBlueChannel(t *testing.T) {
	dir := moduleWorkDir(t)
	result, err := runRealFileSteps(t, dir, []string{
		`import "colour"`,
		`Colour blue dot b dot`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "255")
}

func TestColourModuleBlackAllZero(t *testing.T) {
	dir := moduleWorkDir(t)
	result, err := runRealFileSteps(t, dir, []string{
		`import "colour"`,
		`Colour black dot r dot`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "0")
}

func TestColourModuleWhiteAllMax(t *testing.T) {
	dir := moduleWorkDir(t)
	result, err := runRealFileSteps(t, dir, []string{
		`import "colour"`,
		`Colour white dot r dot`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "255")
}

func TestColourModuleGreyMidpoint(t *testing.T) {
	dir := moduleWorkDir(t)
	result, err := runRealFileSteps(t, dir, []string{
		`import "colour"`,
		`Colour grey dot r dot`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "128")
}

func TestColourModulePurpleHex(t *testing.T) {
	dir := moduleWorkDir(t)
	result, err := runRealFileSteps(t, dir, []string{
		`import "colour"`,
		`Colour purple dot hex dot`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "'#800080'")
}

// --- Colour-scheme module: generates schemes with hex and RGB ---

func TestColourSchemeSunsetName(t *testing.T) {
	dir := moduleWorkDir(t)
	result, err := runRealFileSteps(t, dir, []string{
		`import "colour-scheme"`,
		`Schemes sunset dot name dot`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "'sunset'")
}

func TestColourSchemeSunsetPrimaryHex(t *testing.T) {
	dir := moduleWorkDir(t)
	result, err := runRealFileSteps(t, dir, []string{
		`import "colour-scheme"`,
		`Schemes sunset dot primary dot hex dot`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "'#E63946'")
}

func TestColourSchemeSunsetPrimaryRed(t *testing.T) {
	dir := moduleWorkDir(t)
	result, err := runRealFileSteps(t, dir, []string{
		`import "colour-scheme"`,
		`Schemes sunset dot primary dot r dot`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "230")
}

func TestColourSchemeOceanPrimaryHex(t *testing.T) {
	dir := moduleWorkDir(t)
	result, err := runRealFileSteps(t, dir, []string{
		`import "colour-scheme"`,
		`Schemes ocean dot primary dot hex dot`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "'#264653'")
}

func TestColourSchemeOceanAccentGreen(t *testing.T) {
	dir := moduleWorkDir(t)
	result, err := runRealFileSteps(t, dir, []string{
		`import "colour-scheme"`,
		`Schemes ocean dot accent dot g dot`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "196")
}

func TestColourSchemeNeonPrimaryHex(t *testing.T) {
	dir := moduleWorkDir(t)
	result, err := runRealFileSteps(t, dir, []string{
		`import "colour-scheme"`,
		`Schemes neon dot primary dot hex dot`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "'#FF006E'")
}

func TestColourSchemeNeonDarkBlue(t *testing.T) {
	dir := moduleWorkDir(t)
	result, err := runRealFileSteps(t, dir, []string{
		`import "colour-scheme"`,
		`Schemes neon dot dark dot b dot`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "236")
}

func TestColourSchemeHasBothHexAndRGB(t *testing.T) {
	// Verify that each colour in a scheme has both hex and RGB values.
	dir := moduleWorkDir(t)
	for _, field := range []string{"primary", "secondary", "accent", "dark", "background"} {
		// Check hex field exists (is a string starting with #)
		result, err := runRealFileSteps(t, dir, []string{
			`import "colour-scheme"`,
			`Schemes sunset dot ` + field + ` dot hex dot`,
		})
		if err != nil {
			t.Fatalf("sunset.%s.hex: %v", field, err)
		}
		s := result[0].AsString()
		if len(s) != 7 || s[0] != '#' {
			t.Errorf("sunset.%s.hex = %q, want #XXXXXX format", field, s)
		}

		// Check r field exists (is an integer 0-255)
		result, err = runRealFileSteps(t, dir, []string{
			`import "colour-scheme"`,
			`Schemes sunset dot ` + field + ` dot r dot`,
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
