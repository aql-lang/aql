package vault

import (
	"testing"

	"github.com/charmbracelet/lipgloss"
)

func TestApplyThemeSetsBackground(t *testing.T) {
	applyTheme(themeDark, false)
	if !lipgloss.HasDarkBackground() {
		t.Error("dark mode should force a dark background")
	}
	applyTheme(themeLight, true)
	if lipgloss.HasDarkBackground() {
		t.Error("light mode should force a light background")
	}
	applyTheme(themeAuto, true)
	if !lipgloss.HasDarkBackground() {
		t.Error("auto with detectedDark=true should be dark")
	}
	applyTheme(themeAuto, false)
	if lipgloss.HasDarkBackground() {
		t.Error("auto with detectedDark=false should be light")
	}
}

func TestThemeModeCycleAndParse(t *testing.T) {
	if themeAuto.next() != themeDark || themeDark.next() != themeLight || themeLight.next() != themeAuto {
		t.Errorf("theme cycle wrong: %v %v %v", themeAuto.next(), themeDark.next(), themeLight.next())
	}
	for _, s := range []string{"dark", "light", "auto", "bogus"} {
		if got := parseThemeMode(s).String(); s == "bogus" && got != "auto" {
			t.Errorf("parse(bogus) = %q, want auto", got)
		} else if s != "bogus" && got != s {
			t.Errorf("parse(%q).String() = %q", s, got)
		}
	}
}

func TestThemeKeyCyclesAndPersists(t *testing.T) {
	m := newTestModel(t)
	m.detectedDark = true
	m.theme = themeAuto

	m = feed(t, m, keyMsg("T")) // auto -> dark
	if m.theme != themeDark {
		t.Fatalf("after T = %v, want dark", m.theme)
	}
	if p := loadTUIPrefs(m.ctl.homeDir); p.Theme != "dark" {
		t.Errorf("prefs not persisted: %q", p.Theme)
	}

	m = feed(t, m, keyMsg("T")) // dark -> light
	m = feed(t, m, keyMsg("T")) // light -> auto
	if m.theme != themeAuto {
		t.Errorf("after 3 cycles = %v, want auto", m.theme)
	}
	if p := loadTUIPrefs(m.ctl.homeDir); p.Theme != "auto" {
		t.Errorf("prefs = %q, want auto", p.Theme)
	}
}

func TestAdaptiveColorRecolorsByTheme(t *testing.T) {
	// Force a color profile so AdaptiveColor resolves to escape codes in the
	// (non-TTY) test environment.
	lipgloss.SetColorProfile(lipgloss.ColorProfile()) // no-op guard if already set
	old := lipgloss.ColorProfile()
	defer lipgloss.SetColorProfile(old)
	lipgloss.SetColorProfile(2) // termenv.ANSI256-ish; any color-capable profile

	applyTheme(themeDark, true)
	dark := tuiTextStyle.Render("x")
	applyTheme(themeLight, true)
	light := tuiTextStyle.Render("x")
	applyTheme(themeAuto, true) // restore-ish

	if dark == light {
		t.Errorf("dark and light should render differently, both = %q", dark)
	}
}
