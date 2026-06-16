package vault

// tui_theme.go — the dark / light / auto theme control for the vault TUI.
//
// Colors are defined as background-adaptive (see adapt in tui_styles.go), so
// switching the theme is just a matter of telling lipgloss which background
// to resolve against. "auto" uses the background detected at startup; "dark"
// and "light" force the choice. The selection is persisted in a small UI
// prefs file so it sticks across sessions.

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/charmbracelet/lipgloss"
)

type themeMode int

const (
	themeAuto themeMode = iota
	themeDark
	themeLight
)

func (t themeMode) String() string {
	switch t {
	case themeDark:
		return "dark"
	case themeLight:
		return "light"
	default:
		return "auto"
	}
}

func parseThemeMode(s string) themeMode {
	switch s {
	case "dark":
		return themeDark
	case "light":
		return themeLight
	default:
		return themeAuto
	}
}

// next cycles auto → dark → light → auto.
func (t themeMode) next() themeMode { return (t + 1) % 3 }

// applyTheme makes lipgloss resolve adaptive colors for the chosen mode.
// auto uses the background detected at startup.
func applyTheme(mode themeMode, detectedDark bool) {
	switch mode {
	case themeDark:
		lipgloss.SetHasDarkBackground(true)
	case themeLight:
		lipgloss.SetHasDarkBackground(false)
	default:
		lipgloss.SetHasDarkBackground(detectedDark)
	}
}

// --- prefs file (~/.aql/tui.jsonic) ---------------------------------------

const tuiPrefsFile = "tui.jsonic"

type tuiPrefs struct {
	Theme string `json:"theme,omitempty"`
}

func tuiPrefsPath(homeDir string) string {
	return filepath.Join(homeAQLDir(homeDir), tuiPrefsFile)
}

// loadTUIPrefs reads UI preferences; a missing or unreadable file yields
// defaults (theme auto).
func loadTUIPrefs(homeDir string) tuiPrefs {
	data, err := os.ReadFile(tuiPrefsPath(homeDir))
	if err != nil {
		return tuiPrefs{}
	}
	var p tuiPrefs
	if err := json.Unmarshal(data, &p); err != nil {
		return tuiPrefs{}
	}
	return p
}

// saveTUIPrefs persists UI preferences. Best-effort: never fails the UI.
func saveTUIPrefs(homeDir string, p tuiPrefs) error {
	if homeDir == "" {
		return nil
	}
	if err := os.MkdirAll(homeAQLDir(homeDir), 0700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(tuiPrefsPath(homeDir), data, 0600)
}
