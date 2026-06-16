package vault

// tui_styles.go — lipgloss styles and shared key bindings for the
// interactive vault TUI. Colors stay in the 8/16 ANSI range so the UI
// renders sensibly on any terminal theme.

import (
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/lipgloss"
)

// adapt picks a foreground that reads well on either terminal background:
// the light variant (normal ANSI 0-7) for light terminals, the dark variant
// (bright ANSI 8-15) for dark terminals. lipgloss resolves this at render
// time against the active background, which the theme control toggles, so
// changing the theme recolors everything without rebuilding any styles.
func adapt(light, dark string) lipgloss.AdaptiveColor {
	return lipgloss.AdaptiveColor{Light: light, Dark: dark}
}

// Color roles. Each is background-adaptive so contrast holds in dark, light,
// and (default) auto-detected modes.
var (
	tuiTitleStyle   = lipgloss.NewStyle().Bold(true).Foreground(adapt("4", "12"))
	tuiCrumbStyle   = lipgloss.NewStyle().Foreground(adapt("8", "7"))
	tuiVaultStyle   = lipgloss.NewStyle().Bold(true).Foreground(adapt("6", "14"))
	tuiLockedStyle  = lipgloss.NewStyle().Bold(true).Foreground(adapt("1", "9"))
	tuiOpenStyle    = lipgloss.NewStyle().Foreground(adapt("2", "10"))
	tuiDimStyle     = lipgloss.NewStyle().Foreground(adapt("8", "7"))  // readable secondary
	tuiTextStyle    = lipgloss.NewStyle().Foreground(adapt("0", "15")) // primary, high contrast
	tuiPathStyle    = lipgloss.NewStyle().Foreground(adapt("6", "14")) // folder paths
	tuiOKStyle      = lipgloss.NewStyle().Bold(true).Foreground(adapt("2", "10"))
	tuiErrStyle     = lipgloss.NewStyle().Bold(true).Foreground(adapt("1", "9"))
	tuiHeaderBar    = lipgloss.NewStyle().Bold(true).Foreground(adapt("4", "12"))
	tuiCursorStyle  = lipgloss.NewStyle().Foreground(adapt("4", "11"))
	tuiSectionStyle = lipgloss.NewStyle().Bold(true).Foreground(adapt("5", "13"))
	tuiKeyStyle     = lipgloss.NewStyle().Bold(true).Foreground(adapt("6", "14"))
)

// Global key bindings reserved across every non-input screen. They are
// always shown in the footer/help so a user need never guess them.
var (
	keyEnter   = key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "open"))
	keyBack    = key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back"))
	keyFilter  = key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "filter"))
	keyPalette = key.NewBinding(key.WithKeys(":"), key.WithHelp(":", "command"))
	keyHelpTog = key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "help"))
	keySwitch  = key.NewBinding(key.WithKeys("o", "ctrl+o"), key.WithHelp("o", "switch vault"))
	keyTheme   = key.NewBinding(key.WithKeys("T"), key.WithHelp("T", "theme (dark/light)"))
	keyQuit    = key.NewBinding(key.WithKeys("q"), key.WithHelp("q", "quit"))
)
