package vault

import (
	"io"
	"os/exec"
	"strings"
	"testing"
)

// unescapeBackslash reverses backslashEscapeAll: every "\x" becomes "x".
func unescapeBackslash(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+1 < len(s) {
			i++
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

func TestBackslashEscapeAllRoundTrips(t *testing.T) {
	for _, v := range []string{"abc", "a b c", `a\b`, "tab\tval", "quote'd\"x", "sk-XYZ_123-456"} {
		esc := backslashEscapeAll(v)
		// Every other byte is a backslash, so the raw value is never a
		// contiguous substring of the escaped form (for len>=2).
		if len(v) >= 2 && strings.Contains(esc, v) {
			t.Errorf("escaped %q still contains the raw value: %q", v, esc)
		}
		if got := unescapeBackslash(esc); got != v {
			t.Errorf("round-trip failed: %q -> %q -> %q", v, esc, got)
		}
	}
}

// argvJoined returns the command's argv as one string for substring checks.
func argvJoined(cmd *exec.Cmd) string { return strings.Join(cmd.Args, "\x00") }

func TestMacSetCmdKeepsSecretOffArgv(t *testing.T) {
	const secret = "s3cret-VALUE-not-in-argv"
	cmd := macSetCmd("my.alias", secret)

	// argv is just the interactive launcher; the secret is not there.
	if want := []string{"security", "-i"}; len(cmd.Args) != 2 || cmd.Args[0] != want[0] || cmd.Args[1] != want[1] {
		t.Fatalf("argv = %v, want %v", cmd.Args, want)
	}
	if strings.Contains(argvJoined(cmd), secret) {
		t.Errorf("secret leaked into argv: %v", cmd.Args)
	}

	// The secret is delivered on stdin, in escaped form, and decodes back.
	in, _ := io.ReadAll(cmd.Stdin)
	line := string(in)
	if strings.Contains(line, secret) {
		t.Errorf("raw secret appears verbatim in the stdin command line: %q", line)
	}
	if !strings.HasPrefix(line, "add-generic-password -U -s "+keyringService+" -a my.alias -w ") {
		t.Errorf("unexpected command line: %q", line)
	}
	if got := unescapeBackslash(strings.TrimSpace(line)); !strings.HasSuffix(got, "-w "+secret) {
		t.Errorf("decoded command line missing the secret: %q", got)
	}
}

func TestWinCredCmdKeepsSecretOffArgvAndEnv(t *testing.T) {
	const secret = "p@ss-not-in-argv-or-env"
	cmd := winCredCmd("set", "my.alias")
	cmd.Stdin = strings.NewReader(secret) // caller supplies the secret here

	if strings.Contains(argvJoined(cmd), secret) {
		t.Errorf("secret leaked into argv: %v", cmd.Args)
	}
	for _, e := range cmd.Env {
		if strings.Contains(e, secret) {
			t.Errorf("secret leaked into env: %q", e)
		}
	}
	// The operation and target travel in the environment, not argv.
	env := strings.Join(cmd.Env, "\n")
	for _, want := range []string{"BORU_KR_OP=set", "BORU_KR_TARGET=" + keyringService + ":my.alias", "BORU_KR_USER=my.alias"} {
		if !strings.Contains(env, want) {
			t.Errorf("env missing %q", want)
		}
	}
	// And the secret really is on stdin.
	in, _ := io.ReadAll(cmd.Stdin)
	if string(in) != secret {
		t.Errorf("stdin = %q, want the secret", in)
	}
}

func TestWinCredScriptShape(t *testing.T) {
	// Guard the helper against accidental truncation/edits: it must do
	// the three credential ops, read the secret from stdin, and key off
	// the environment variables winCredCmd sets.
	for _, want := range []string{
		"CredWriteW", "CredReadW", "CredDeleteW",
		"[Console]::In.ReadToEnd", "$env:BORU_KR_OP", "$env:BORU_KR_TARGET", "exit 2",
	} {
		if !strings.Contains(winCredScript, want) {
			t.Errorf("winCredScript missing %q", want)
		}
	}
}
