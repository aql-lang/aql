package build

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/boru-lang/boru/cmd/go/internal/buildrt"
)

// --- New / Name / Synopsis ---

func TestCommandMetadata(t *testing.T) {
	c := New()
	if got := c.Name(); got != "build" {
		t.Errorf("Name() = %q, want build", got)
	}
	if c.Synopsis() == "" {
		t.Error("Synopsis() is empty")
	}
}

// runBuild drives the subcommand and returns exit code plus outputs.
func runBuild(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := New().Run(args, nil, &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}

// --- Run: flag/argument validation failures ---

func TestRunRequiresExactlyOneProgram(t *testing.T) {
	code, _, stderr := runBuild(t)
	if code != 1 {
		t.Fatalf("exit = %d, want 1", code)
	}
	if !strings.Contains(stderr, "exactly one") {
		t.Errorf("stderr = %q, want 'exactly one'", stderr)
	}

	code, _, stderr = runBuild(t, "a.boru", "b.boru")
	if code != 1 {
		t.Fatalf("two positionals: exit = %d, want 1", code)
	}
	if !strings.Contains(stderr, "exactly one") {
		t.Errorf("two positionals: stderr = %q, want 'exactly one'", stderr)
	}
}

func TestRunRejectsUnknownFlag(t *testing.T) {
	code, _, stderr := runBuild(t, "-nope", "p.boru")
	if code != 1 {
		t.Fatalf("exit = %d, want 1", code)
	}
	if !strings.Contains(stderr, "nope") {
		t.Errorf("stderr = %q, want flag error", stderr)
	}
}

func TestRunRejectsBadOptions(t *testing.T) {
	dir := t.TempDir()
	src := write(t, dir, "p.boru", "add 1 2")

	// Unknown option key fails ApplyOptions validation at build time.
	code, _, stderr := runBuild(t, "-options", "nosuch:1", src)
	if code != 1 {
		t.Fatalf("unknown option: exit = %d, want 1", code)
	}
	if !strings.Contains(stderr, "unknown option") {
		t.Errorf("unknown option: stderr = %q", stderr)
	}

	// A tape option with the wrong shape fails too.
	code, _, stderr = runBuild(t, "-options", "tape:1", src)
	if code != 1 {
		t.Fatalf("bad tape shape: exit = %d, want 1", code)
	}
	if !strings.Contains(stderr, "error:") {
		t.Errorf("bad tape shape: stderr = %q", stderr)
	}
}

func TestRunMissingSourceFile(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "nope.boru")
	code, _, stderr := runBuild(t, missing)
	if code != 1 {
		t.Fatalf("exit = %d, want 1", code)
	}
	if !strings.Contains(stderr, "error:") {
		t.Errorf("stderr = %q, want read error", stderr)
	}
}

// --- Run: self-embed happy path (copies the test binary; no toolchain) ---

func TestRunSelfEmbedSuccess(t *testing.T) {
	dir := t.TempDir()
	src := write(t, dir, "p.boru", "add 1 2")
	out := filepath.Join(dir, "p-built")

	// Flags after the positional exercise the interleaved re-parse loop.
	code, stdout, stderr := runBuild(t, src, "-o", out, "-s", "42", "-options", "tape:initial:1024")
	if code != 0 {
		t.Fatalf("exit = %d, want 0; stderr: %s", code, stderr)
	}
	if !strings.Contains(stdout, "wrote "+out) {
		t.Errorf("stdout = %q, want wrote line", stdout)
	}

	cfg, ok, err := buildrt.ReadEmbeddedPayload(out)
	if err != nil || !ok {
		t.Fatalf("payload not detectable (ok=%v err=%v)", ok, err)
	}
	if cfg.Source != "add 1 2" {
		t.Errorf("Source = %q, want add 1 2", cfg.Source)
	}
	if cfg.Seed != 42 {
		t.Errorf("Seed = %d, want 42", cfg.Seed)
	}
	if cfg.OptionsBlob != "tape:initial:1024" {
		t.Errorf("OptionsBlob = %q", cfg.OptionsBlob)
	}
}

func TestRunSelfEmbedDefaultOutput(t *testing.T) {
	dir := t.TempDir()
	src := write(t, dir, "prog.boru", "add 1 2")

	// Run from the temp dir so the defaulted output lands there.
	prevWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(prevWD) })

	code, stdout, stderr := runBuild(t, src)
	if code != 0 {
		t.Fatalf("exit = %d, want 0; stderr: %s", code, stderr)
	}
	if !strings.Contains(stdout, "wrote prog") {
		t.Errorf("stdout = %q, want default output name", stdout)
	}
	if _, err := os.Stat(filepath.Join(dir, "prog")); err != nil {
		t.Errorf("expected default output file: %v", err)
	}
}

func TestRunSelfEmbedWriteFailure(t *testing.T) {
	dir := t.TempDir()
	src := write(t, dir, "p.boru", "add 1 2")
	out := filepath.Join(dir, "no-such-dir", "p")

	code, _, stderr := runBuild(t, src, "-o", out)
	if code != 1 {
		t.Fatalf("exit = %d, want 1", code)
	}
	if !strings.Contains(stderr, "write") {
		t.Errorf("stderr = %q, want write error", stderr)
	}
}

// --- buildSelfEmbed: write-error branch, direct ---

func TestBuildSelfEmbedWriteError(t *testing.T) {
	cfg := buildrt.Config{Source: "add 1 2"}
	err := buildSelfEmbed(cfg, filepath.Join(t.TempDir(), "missing", "out"))
	if err == nil || !strings.Contains(err.Error(), "write") {
		t.Fatalf("err = %v, want write error", err)
	}
}
