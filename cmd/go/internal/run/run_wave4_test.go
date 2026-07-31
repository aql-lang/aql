// Wave-4 coverage for the run subcommand: the Command wrapper and
// SetVersion, the -version flag, script-file execution and its read
// error, the permission-flag and --options error arms, and the no-
// source REPL drop-in.
package run

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestW4CommandMethodsAndSetVersion(t *testing.T) {
	orig := Version
	defer SetVersion(orig)
	SetVersion("7.7.7-w4")

	c := New()
	if c.Name() != "run" {
		t.Errorf("Name() = %q, want run", c.Name())
	}
	if c.Synopsis() == "" {
		t.Error("Synopsis() is empty")
	}

	var stdout, stderr bytes.Buffer
	code := c.Run([]string{"-version"}, strings.NewReader(""), &stdout, &stderr)
	if code != 0 {
		t.Fatalf("Run(-version) = %d, want 0", code)
	}
	if !strings.Contains(stdout.String(), "boru 7.7.7-w4") {
		t.Errorf("stdout = %q, want the injected version", stdout.String())
	}
}

func TestW4ExecuteUsageOnHelp(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Execute([]string{"-h"}, strings.NewReader(""), &stdout, &stderr)
	if code != 1 {
		t.Fatalf("Execute(-h) = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "Usage: boru [options]") {
		t.Errorf("stderr = %q, want the usage text", stderr.String())
	}
}

func TestW4ExecuteScriptFile(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "w4.boru")
	if err := os.WriteFile(script, []byte("4 mul 10"), 0644); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	code := Execute([]string{script}, strings.NewReader(""), &stdout, &stderr)
	if code != 0 {
		t.Fatalf("Execute(script) = %d, want 0; stderr: %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "40") {
		t.Errorf("stdout = %q, want 40", stdout.String())
	}
}

func TestW4ExecuteScriptFileMissing(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Execute([]string{filepath.Join(t.TempDir(), "zz-missing.boru")}, strings.NewReader(""), &stdout, &stderr)
	if code != 1 {
		t.Fatalf("Execute(missing script) = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "error:") {
		t.Errorf("stderr = %q, want a read error", stderr.String())
	}
}

func TestW4ExecutePermsConflict(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Execute([]string{"--perms", "none", "--perms-inline", "{}", "-e", "1 add 2"}, strings.NewReader(""), &stdout, &stderr)
	if code != 1 {
		t.Fatalf("Execute(perms conflict) = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "mutually exclusive") {
		t.Errorf("stderr = %q, want mutual-exclusion error", stderr.String())
	}
}

func TestW4ExecuteOptionsParseError(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Execute([]string{"--options", "{{", "-e", "1"}, strings.NewReader(""), &stdout, &stderr)
	if code != 1 {
		t.Fatalf("Execute(bad --options) = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "error:") {
		t.Errorf("stderr = %q, want a parse error", stderr.String())
	}
}

func TestW4ExecuteOptionsApplyError(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Execute([]string{"--options", "zz-unknown:1", "-e", "1"}, strings.NewReader(""), &stdout, &stderr)
	if code != 1 {
		t.Fatalf("Execute(unknown option) = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "unknown option") {
		t.Errorf("stderr = %q, want unknown-option error", stderr.String())
	}
}

func TestW4ExecuteOptionsValid(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Execute([]string{"--options", "tape:initial:1024", "-e", "1 add 2"}, strings.NewReader(""), &stdout, &stderr)
	if code != 0 {
		t.Fatalf("Execute(valid --options) = %d, want 0; stderr: %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "3") {
		t.Errorf("stdout = %q, want 3", stdout.String())
	}
}

func TestW4ExecuteREPLDropIn(t *testing.T) {
	// No source at all: prints the banner and starts the REPL, which
	// exits immediately on stdin EOF.
	var stdout, stderr bytes.Buffer
	code := Execute(nil, strings.NewReader(""), &stdout, &stderr)
	if code != 0 {
		t.Fatalf("Execute(no source) = %d, want 0", code)
	}
	if !strings.Contains(stdout.String(), "boru ") {
		t.Errorf("stdout = %q, want the version banner", stdout.String())
	}
}
