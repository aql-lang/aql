package vault

// tui_seam8_test.go — wave-8 coverage for tui.go's runInteractive entry point
// and interactiveTTY, plus the t7runProgram default seam body. The bubbletea
// program launch is stubbed through the committed t7runProgram / t7isTerminal
// seams so no real terminal is required; the default seam is exercised once
// with a headless immediately-quitting program.

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// w8PipeFiles returns a read *os.File and a write *os.File (both real files, so
// they satisfy interactiveTTY's *os.File assertion), auto-closed on cleanup.
func w8PipeFiles(t *testing.T) (*os.File, *os.File) {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	t.Cleanup(func() { r.Close(); w.Close() })
	return r, w
}

// w8FakeTTY swaps t7isTerminal so every fd reports as a terminal.
func w8FakeTTY(t *testing.T) {
	t.Helper()
	orig := t7isTerminal
	t.Cleanup(func() { t7isTerminal = orig })
	t7isTerminal = func(int) bool { return true }
}

// w8StubProgram swaps t7runProgram to return the given error without launching
// a real program loop.
func w8StubProgram(t *testing.T, runErr error) {
	t.Helper()
	orig := t7runProgram
	t.Cleanup(func() { t7runProgram = orig })
	t7runProgram = func(*tea.Program) (tea.Model, error) { return nil, runErr }
}

// TestW8_RunInteractiveOpensExistingVault drives runInteractive's success path
// with an initialized vault (the resolveLaunchVault ok branch).
func TestW8_RunInteractiveOpensExistingVault(t *testing.T) {
	home := testHome(t)
	mustInit(t)
	w8FakeTTY(t)
	w8StubProgram(t, nil)
	stdin, stdout := w8PipeFiles(t)
	var stderr bytes.Buffer
	if code := runInteractive(nil, home, stdin, stdout, &stderr); code != 0 {
		t.Errorf("runInteractive on an existing vault = %d, want 0 (stderr=%q)", code, stderr.String())
	}
}

// TestW8_RunInteractiveNoVaultDefaultsHome drives runInteractive's else branch:
// with no vault, resolveLaunchVault reports !ok and the active location falls
// back to ~/.boru so the picker can open.
func TestW8_RunInteractiveNoVaultDefaultsHome(t *testing.T) {
	home := testHome(t) // no vault initialized
	w8FakeTTY(t)
	w8StubProgram(t, nil)
	stdin, stdout := w8PipeFiles(t)
	var stderr bytes.Buffer
	if code := runInteractive(nil, home, stdin, stdout, &stderr); code != 0 {
		t.Errorf("runInteractive with no vault = %d, want 0 (stderr=%q)", code, stderr.String())
	}
}

// TestW8_RunInteractiveProgramError drives runInteractive's program-launch error
// arm: t7runProgram returns an error, so the function prints it and returns 1.
func TestW8_RunInteractiveProgramError(t *testing.T) {
	home := testHome(t)
	mustInit(t)
	w8FakeTTY(t)
	w8StubProgram(t, io.ErrClosedPipe)
	stdin, stdout := w8PipeFiles(t)
	var stderr bytes.Buffer
	if code := runInteractive(nil, home, stdin, stdout, &stderr); code != 1 {
		t.Errorf("runInteractive with a program error = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "tui:") {
		t.Errorf("program error should be reported on stderr, got %q", stderr.String())
	}
}

// TestW8_InteractiveTTYStdoutArm drives interactiveTTY's stdout branch: a
// terminal stdin with a non-file stdout reports false; two terminal files
// report true.
func TestW8_InteractiveTTYStdoutArm(t *testing.T) {
	w8FakeTTY(t)

	// stdin is a real file (terminal), stdout is not an *os.File -> false.
	stdinFile, _ := w8PipeFiles(t)
	if interactiveTTY(stdinFile, &bytes.Buffer{}) {
		t.Error("a non-file stdout must not count as a terminal")
	}

	// Both stdin and stdout are terminal files -> true.
	r, w := w8PipeFiles(t)
	if !interactiveTTY(r, w) {
		t.Error("two terminal files should report interactive")
	}
}

// w8QuitModel is a bubbletea model that quits immediately, so p.Run() returns
// without needing a real terminal.
type w8QuitModel struct{}

func (w8QuitModel) Init() tea.Cmd                       { return tea.Quit }
func (w8QuitModel) Update(tea.Msg) (tea.Model, tea.Cmd) { return w8QuitModel{}, tea.Quit }
func (w8QuitModel) View() string                        { return "" }

// TestW8_RunProgramDefaultSeam exercises the default t7runProgram body
// (return p.Run()) with a headless, immediately-quitting program.
func TestW8_RunProgramDefaultSeam(t *testing.T) {
	prog := tea.NewProgram(
		w8QuitModel{},
		tea.WithInput(strings.NewReader("")),
		tea.WithOutput(io.Discard),
	)
	if _, err := t7runProgram(prog); err != nil {
		t.Errorf("headless quit program should run cleanly, got %v", err)
	}
}
