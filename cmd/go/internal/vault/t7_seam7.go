package vault

// t7_seam7.go — package-level test seams for the seam7 coverage pass over the
// interactive TUI (design/TEST-SEAMS.10.md). Each variable defaults to the
// real implementation and is unexported; TestT7_* tests swap it (with a
// t.Cleanup restore) to drive otherwise-unreachable OS/stdlib failure arms and
// the bubbletea program entry point. Production code never checks "is the seam
// set", so behaviour with the defaults is identical to calling the underlying
// function directly.

import (
	"encoding/json"

	tea "github.com/charmbracelet/bubbletea"
	"golang.org/x/term"
)

var (
	// t7jsonMarshalIndent seams encoding/json.MarshalIndent in saveTUIPrefs
	// (tui_theme.go) and saveVaultIndex (tui_vaults.go). Their structs only
	// hold strings/ints/bools, so marshaling never fails in production; the
	// error arms are reachable only through this seam.
	t7jsonMarshalIndent = json.MarshalIndent

	// t7detectClipboard seams clipboard detection in copyToClipboard
	// (tui_controller.go) so the copy success / copy-error arms can be driven
	// without a real OS clipboard tool on the host.
	t7detectClipboard = detectClipboard

	// t7isTerminal seams golang.org/x/term.IsTerminal in interactiveTTY
	// (tui.go) so the terminal-gate arms are drivable without a real TTY.
	t7isTerminal = term.IsTerminal

	// t7runProgram seams (*tea.Program).Run in runInteractive (tui.go) so the
	// program-launch and its error arm are drivable without a real terminal
	// program loop.
	t7runProgram = func(p *tea.Program) (tea.Model, error) { return p.Run() }
)
