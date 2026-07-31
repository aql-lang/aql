package vault

// tui.go — `boru vault -i` / `boru vault --interactive`: the entry point that
// resolves which vault to open, guards against a non-terminal, and runs the
// bubbletea program.

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// cliVersion is the boru build version string, injected by the main package
// via SetVersion and shown in the interactive TUI header.
var cliVersion string

// SetVersion records the CLI version string for the interactive TUI header.
// The main package calls it during startup (mirroring run.SetVersion).
func SetVersion(v string) { cliVersion = v }

// runInteractive launches the interactive vault TUI. --boru runs the
// BORU implementation (the boru:vault-tui module over the boru:vault
// bridge — design/VAULT-TUI-PORT.0.md §1.3); the bubbletea TUI stays
// the default.
func runInteractive(args []string, homeDir string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("vault -i", flag.ContinueOnError)
	fs.SetOutput(stderr)
	useBORU := fs.Bool("boru", false, "run the BORU implementation of the interactive TUI (experimental)")
	if err := fs.Parse(args); err != nil {
		return 1
	}
	if *useBORU {
		return runInteractiveBORU(homeDir, stdin, stdout, stderr)
	}

	if !interactiveTTY(stdin, stdout) {
		fmt.Fprintln(stderr, "error: the interactive vault TUI requires a terminal (run `boru vault -i` in an interactive shell)")
		return 1
	}

	ctl := newTUIController(homeDir)
	folder, suffix, ok := resolveLaunchVault(homeDir)
	if ok {
		ctl.setActiveVault(folder, suffix)
		_ = recordVaultOpened(homeDir, folder, suffix)
	} else {
		// Default the active location to the home ~/.boru so the header and the
		// "new vault" defaults are sensible; the picker opens because no store
		// exists there yet.
		ctl.setActiveVault(homeBORUDir(homeDir), "")
	}

	// Theme: detect the terminal background once, then honor the saved
	// preference (auto uses the detected value).
	detectedDark := lipgloss.HasDarkBackground()
	mode := parseThemeMode(loadTUIPrefs(homeDir).Theme)
	applyTheme(mode, detectedDark)

	m := newRootModel(ctl)
	m.theme = mode
	m.detectedDark = detectedDark
	prog := tea.NewProgram(m, tea.WithInput(stdin), tea.WithOutput(stdout), tea.WithAltScreen())
	if _, err := t7runProgram(prog); err != nil {
		fmt.Fprintf(stderr, "tui: %s\n", err)
		return 1
	}
	return 0
}

// resolveLaunchVault decides which vault `boru vault -i` opens: an explicit
// --folder/--suffix (already promoted to the env by Run) opens that vault
// directly; otherwise the index default / most-recently-opened / sole vault
// is used. ok is false when there is no vault to open, so the TUI starts on
// the picker.
func resolveLaunchVault(homeDir string) (folder, suffix string, ok bool) {
	if os.Getenv(EnvFolder) != "" || os.Getenv(EnvSuffix) != "" {
		return filepath.Clean(vaultFolder(homeDir)), os.Getenv(EnvSuffix), true
	}
	return launchVault(homeDir)
}

// interactiveTTY reports whether both stdin and stdout are real terminals,
// which bubbletea requires.
func interactiveTTY(stdin io.Reader, stdout io.Writer) bool {
	in, ok := stdin.(*os.File)
	if !ok || !t7isTerminal(int(in.Fd())) {
		return false
	}
	out, ok := stdout.(*os.File)
	if !ok || !t7isTerminal(int(out.Fd())) {
		return false
	}
	return true
}
