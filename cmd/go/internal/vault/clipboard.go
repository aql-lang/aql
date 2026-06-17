package vault

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

// clipboard support lets `vault add`/`vault rotate` ingest a secret a
// user has just copied from a SaaS console without it ever touching
// the shell history or this process's argv: the value is read from the
// OS clipboard and the clipboard is wiped immediately afterwards.
//
// Each platform exposes the clipboard through a small CLI: pbpaste /
// pbcopy on macOS, wl-clipboard or xclip / xsel on Linux, and
// PowerShell's Get-Clipboard / Set-Clipboard on Windows. The
// indirection here keeps the per-OS selection logic testable without a
// live desktop session (see detectClipboard).

// clipboardCmd describes how to read from and clear the OS clipboard
// using whatever tool is available on the host.
type clipboardCmd struct {
	label      string   // human-readable tool name, e.g. "pbpaste/pbcopy"
	pasteArgv  []string // command that writes clipboard contents to stdout
	clearArgv  []string // command that clears/overwrites the clipboard
	clearStdin bool     // if true, clearArgv is fed an empty stdin (tools that "clear" by setting empty content)
	copyArgv   []string // command that reads text from stdin and sets the clipboard
}

// clipEnv abstracts the host facts clipboard detection depends on, so
// the selection logic can be unit-tested for every platform from a
// single machine.
type clipEnv struct {
	goos     string
	getenv   func(string) string
	lookPath func(string) (string, error)
}

func hostClipEnv() clipEnv {
	return clipEnv{goos: runtime.GOOS, getenv: os.Getenv, lookPath: exec.LookPath}
}

// detectClipboard picks a clipboard toolchain for the host, or returns
// an error naming what to install. On Linux it prefers a Wayland tool
// when a Wayland session is detected, then falls back to X11 tools.
func detectClipboard(env clipEnv) (*clipboardCmd, error) {
	have := func(bin string) bool {
		_, err := env.lookPath(bin)
		return err == nil
	}
	switch env.goos {
	case "darwin":
		if have("pbpaste") && have("pbcopy") {
			return &clipboardCmd{
				label:      "pbpaste/pbcopy",
				pasteArgv:  []string{"pbpaste"},
				clearArgv:  []string{"pbcopy"}, // empty stdin -> empty clipboard
				clearStdin: true,
				copyArgv:   []string{"pbcopy"},
			}, nil
		}
		return nil, fmt.Errorf("pbpaste/pbcopy not found (they ship with macOS)")
	case "windows":
		if ps := powershellName(env); ps != "" {
			return &clipboardCmd{
				label:     ps + " Get-/Set-Clipboard",
				pasteArgv: []string{ps, "-NoProfile", "-Command", "Get-Clipboard -Raw"},
				// Overwrite with a single space: Set-Clipboard rejects an
				// empty string, and overwriting is what matters — the
				// secret no longer lives in the clipboard.
				clearArgv: []string{ps, "-NoProfile", "-Command", "Set-Clipboard -Value ' '"},
				copyArgv:  []string{ps, "-NoProfile", "-Command", "$input | Set-Clipboard"},
			}, nil
		}
		return nil, fmt.Errorf("powershell not found on PATH")
	default: // linux, *bsd, ...
		// Prefer Wayland tools when a Wayland session is present, or
		// when wl-clipboard is installed and there is no X11 display.
		if have("wl-paste") && have("wl-copy") &&
			(env.getenv("WAYLAND_DISPLAY") != "" || env.getenv("DISPLAY") == "") {
			return &clipboardCmd{
				label:     "wl-paste/wl-copy",
				pasteArgv: []string{"wl-paste"},
				clearArgv: []string{"wl-copy", "--clear"},
				copyArgv:  []string{"wl-copy"},
			}, nil
		}
		if have("xclip") {
			return &clipboardCmd{
				label:      "xclip",
				pasteArgv:  []string{"xclip", "-selection", "clipboard", "-o"},
				clearArgv:  []string{"xclip", "-selection", "clipboard"}, // empty stdin -> empty clipboard
				clearStdin: true,
				copyArgv:   []string{"xclip", "-selection", "clipboard"},
			}, nil
		}
		if have("xsel") {
			return &clipboardCmd{
				label:     "xsel",
				pasteArgv: []string{"xsel", "--clipboard", "--output"},
				clearArgv: []string{"xsel", "--clipboard", "--clear"},
				copyArgv:  []string{"xsel", "--clipboard", "--input"},
			}, nil
		}
		return nil, fmt.Errorf("no clipboard tool found; install wl-clipboard (Wayland) or xclip / xsel (X11)")
	}
}

// powershellName returns the first PowerShell executable on PATH, or
// "" if none is available.
func powershellName(env clipEnv) string {
	for _, name := range []string{"powershell.exe", "powershell", "pwsh.exe", "pwsh"} {
		if _, err := env.lookPath(name); err == nil {
			return name
		}
	}
	return ""
}

// paste runs the detected paste command and returns the clipboard
// contents with surrounding whitespace trimmed. Manually copied SaaS
// tokens never carry meaningful leading/trailing whitespace, and some
// clipboard tools append a trailing newline.
func (c *clipboardCmd) paste() (string, error) {
	cmd := exec.Command(c.pasteArgv[0], c.pasteArgv[1:]...)
	var out, errBuf bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errBuf
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("%s: %s: %s", c.label, err, strings.TrimSpace(errBuf.String()))
	}
	return strings.TrimSpace(out.String()), nil
}

// clear overwrites the clipboard so the just-ingested secret does not
// linger in it.
func (c *clipboardCmd) clear() error {
	cmd := exec.Command(c.clearArgv[0], c.clearArgv[1:]...)
	if c.clearStdin {
		cmd.Stdin = strings.NewReader("")
	}
	var errBuf bytes.Buffer
	cmd.Stderr = &errBuf
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s: %s: %s", c.label, err, strings.TrimSpace(errBuf.String()))
	}
	return nil
}

// copy sets the clipboard to text by feeding it on stdin to the platform's
// write tool. It is used to copy non-secret helper text (e.g. an exec command)
// to the clipboard from the TUI.
func (c *clipboardCmd) copy(text string) error {
	if len(c.copyArgv) == 0 {
		return fmt.Errorf("%s: copy not supported", c.label)
	}
	cmd := exec.Command(c.copyArgv[0], c.copyArgv[1:]...)
	cmd.Stdin = strings.NewReader(text)
	var errBuf bytes.Buffer
	cmd.Stderr = &errBuf
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s: %s: %s", c.label, err, strings.TrimSpace(errBuf.String()))
	}
	return nil
}

// clearClipboardAfterStore wipes the OS clipboard after a secret was
// ingested from it and reminds the operator that a clipboard manager
// may still retain a copy. Failures here are advisory: the secret is
// already stored, so we warn rather than fail the surrounding command.
func clearClipboardAfterStore(stdout, stderr io.Writer) {
	clip, err := detectClipboard(hostClipEnv())
	if err != nil {
		fmt.Fprintf(stderr, "warning: could not clear clipboard: %s\n", err)
		return
	}
	if err := clip.clear(); err != nil {
		fmt.Fprintf(stderr, "warning: could not clear clipboard: %s\n", err)
		return
	}
	fmt.Fprintf(stdout, "clipboard cleared (%s)\n", clip.label)
	fmt.Fprintln(stdout, "note: a clipboard manager, if you run one, may still hold a copy — clear its history too")
}
