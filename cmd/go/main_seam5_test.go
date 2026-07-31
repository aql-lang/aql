// Seam-based coverage for the top-level dispatcher (design/TEST-SEAMS.10.md):
// versionString's VCS-stamp arms via the readBuildInfo seam, runEmbedded's
// error and dispatch arms via the osExecutable/readEmbeddedPayload/buildrtMain
// seams, and Run() itself via the osExit seam.
//
// Every test here swaps package-level seams, so none of them may use
// t.Parallel().
package boru

import (
	"errors"
	"io"
	"os"
	"runtime/debug"
	"strings"
	"testing"

	"github.com/boru-lang/boru/cmd/go/internal/buildrt"
)

// seam5SetDevVersion pins Version to the dev default for the duration
// of a test and restores it afterwards.
func seam5SetDevVersion(t *testing.T) {
	t.Helper()
	orig := Version
	t.Cleanup(func() { Version = orig })
	Version = "0.1.0-dev"
}

// seam5SwapReadBuildInfo installs a fake readBuildInfo and restores the
// real one afterwards.
func seam5SwapReadBuildInfo(t *testing.T, fn func() (*debug.BuildInfo, bool)) {
	t.Helper()
	orig := readBuildInfo
	t.Cleanup(func() { readBuildInfo = orig })
	readBuildInfo = fn
}

// seam5BuildInfoWith returns a readBuildInfo fake reporting the given
// settings.
func seam5BuildInfoWith(settings ...debug.BuildSetting) func() (*debug.BuildInfo, bool) {
	return func() (*debug.BuildInfo, bool) {
		return &debug.BuildInfo{Settings: settings}, true
	}
}

func TestSeam5VersionStringNoBuildInfo(t *testing.T) {
	seam5SetDevVersion(t)
	seam5SwapReadBuildInfo(t, func() (*debug.BuildInfo, bool) { return nil, false })
	if got := versionString(); got != "0.1.0-dev" {
		t.Errorf("versionString() = %q, want plain dev version when build info is absent", got)
	}
}

func TestSeam5VersionStringDirtyLongRevision(t *testing.T) {
	seam5SetDevVersion(t)
	seam5SwapReadBuildInfo(t, seam5BuildInfoWith(
		debug.BuildSetting{Key: "vcs.revision", Value: "0123456789abcdef0123"},
		debug.BuildSetting{Key: "vcs.modified", Value: "true"},
	))
	if got, want := versionString(), "0.1.0-dev (git 0123456789ab, dirty)"; got != want {
		t.Errorf("versionString() = %q, want %q (long revision truncated, dirty suffix)", got, want)
	}
}

func TestSeam5VersionStringCleanShortRevision(t *testing.T) {
	seam5SetDevVersion(t)
	seam5SwapReadBuildInfo(t, seam5BuildInfoWith(
		debug.BuildSetting{Key: "vcs.revision", Value: "abc123"},
		debug.BuildSetting{Key: "vcs.modified", Value: "false"},
	))
	if got, want := versionString(), "0.1.0-dev (git abc123)"; got != want {
		t.Errorf("versionString() = %q, want %q (short revision kept, clean build)", got, want)
	}
}

func TestSeam5VersionStringEmptyRevision(t *testing.T) {
	seam5SetDevVersion(t)
	seam5SwapReadBuildInfo(t, seam5BuildInfoWith(
		debug.BuildSetting{Key: "vcs.modified", Value: "true"},
		debug.BuildSetting{Key: "goarch", Value: "amd64"},
	))
	if got := versionString(); got != "0.1.0-dev" {
		t.Errorf("versionString() = %q, want plain dev version when vcs.revision is absent", got)
	}
}

// --- runEmbedded seams ---

func TestSeam5RunEmbeddedExecutableError(t *testing.T) {
	orig := osExecutable
	t.Cleanup(func() { osExecutable = orig })
	osExecutable = func() (string, error) { return "", errors.New("no self path") }

	code, embedded := runEmbedded()
	if code != 0 || embedded {
		t.Errorf("runEmbedded() = (%d, %v), want (0, false) when the self path is unknown", code, embedded)
	}
}

// seam5CaptureStderr swaps os.Stderr for a pipe and returns a function
// that ends the capture and yields everything written.
func seam5CaptureStderr(t *testing.T) func() string {
	t.Helper()
	orig := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stderr = w
	t.Cleanup(func() { os.Stderr = orig })
	done := make(chan string, 1)
	go func() {
		b, _ := io.ReadAll(r)
		done <- string(b)
	}()
	return func() string {
		w.Close()
		os.Stderr = orig
		return <-done
	}
}

// seam5CaptureStdout swaps os.Stdout for a pipe and returns a function
// that ends the capture and yields everything written.
func seam5CaptureStdout(t *testing.T) func() string {
	t.Helper()
	orig := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	t.Cleanup(func() { os.Stdout = orig })
	done := make(chan string, 1)
	go func() {
		b, _ := io.ReadAll(r)
		done <- string(b)
	}()
	return func() string {
		w.Close()
		os.Stdout = orig
		return <-done
	}
}

func TestSeam5RunEmbeddedPayloadError(t *testing.T) {
	orig := readEmbeddedPayload
	t.Cleanup(func() { readEmbeddedPayload = orig })
	readEmbeddedPayload = func(string) (buildrt.Config, bool, error) {
		return buildrt.Config{}, false, errors.New("corrupt payload")
	}

	stderr := seam5CaptureStderr(t)
	code, embedded := runEmbedded()
	got := stderr()

	if code != 1 || !embedded {
		t.Errorf("runEmbedded() = (%d, %v), want (1, true) on a corrupt payload", code, embedded)
	}
	if want := "error: corrupt payload\n"; got != want {
		t.Errorf("stderr = %q, want %q", got, want)
	}
}

func TestSeam5RunEmbeddedDispatch(t *testing.T) {
	origRead := readEmbeddedPayload
	origMain := buildrtMain
	t.Cleanup(func() {
		readEmbeddedPayload = origRead
		buildrtMain = origMain
	})
	readEmbeddedPayload = func(string) (buildrt.Config, bool, error) {
		return buildrt.Config{Source: "1 add 2"}, true, nil
	}
	var gotCfg buildrt.Config
	buildrtMain = func(cfg buildrt.Config, _ []string, _ io.Reader, _, _ io.Writer) int {
		gotCfg = cfg
		return 42
	}

	code, embedded := runEmbedded()
	if code != 42 || !embedded {
		t.Errorf("runEmbedded() = (%d, %v), want (42, true) from the embedded program", code, embedded)
	}
	if gotCfg.Source != "1 add 2" {
		t.Errorf("buildrt.Main received Source %q, want the decoded payload's source", gotCfg.Source)
	}
}

// --- Run() via the osExit seam ---

// seam5ExitSentinel is the panic payload the swapped osExit throws so
// Run() never executes past an exit, mirroring real os.Exit semantics.
type seam5ExitSentinel struct{}

// seam5TrapExit swaps osExit for a recorder that panics with
// seam5ExitSentinel, and returns the slice of recorded codes.
func seam5TrapExit(t *testing.T) *[]int {
	t.Helper()
	orig := osExit
	t.Cleanup(func() { osExit = orig })
	codes := &[]int{}
	osExit = func(code int) {
		*codes = append(*codes, code)
		panic(seam5ExitSentinel{})
	}
	return codes
}

// seam5CallRecoveringExit invokes fn, recovering only the exit
// sentinel; any other panic propagates.
func seam5CallRecoveringExit(t *testing.T, fn func()) {
	t.Helper()
	defer func() {
		if r := recover(); r != nil {
			if _, ok := r.(seam5ExitSentinel); !ok {
				panic(r)
			}
		}
	}()
	fn()
}

func TestSeam5RunEmbeddedProgramExits(t *testing.T) {
	origRead := readEmbeddedPayload
	origMain := buildrtMain
	t.Cleanup(func() {
		readEmbeddedPayload = origRead
		buildrtMain = origMain
	})
	readEmbeddedPayload = func(string) (buildrt.Config, bool, error) {
		return buildrt.Config{}, true, nil
	}
	buildrtMain = func(buildrt.Config, []string, io.Reader, io.Writer, io.Writer) int { return 7 }
	codes := seam5TrapExit(t)

	seam5CallRecoveringExit(t, Run)

	if len(*codes) != 1 || (*codes)[0] != 7 {
		t.Errorf("exit codes = %v, want [7] from the embedded program", *codes)
	}
}

func TestSeam5RunNormalCLIPath(t *testing.T) {
	origRead := readEmbeddedPayload
	origArgs := os.Args
	t.Cleanup(func() {
		readEmbeddedPayload = origRead
		os.Args = origArgs
	})
	// No embedded payload: Run falls through to the normal CLI dispatch.
	readEmbeddedPayload = func(string) (buildrt.Config, bool, error) {
		return buildrt.Config{}, false, nil
	}
	// Benign argv so execute() returns quickly with code 0.
	os.Args = []string{"boru", "help"}
	codes := seam5TrapExit(t)

	stdout := seam5CaptureStdout(t)
	seam5CallRecoveringExit(t, Run)
	out := stdout()

	if len(*codes) != 1 || (*codes)[0] != 0 {
		t.Errorf("exit codes = %v, want [0] from `boru help`", *codes)
	}
	if !strings.Contains(out, "Commands:") {
		t.Errorf("expected the help overview on stdout, got %q", out)
	}
}
