package vault

// cli_wave4_test.go — error-arm coverage for the vault.go command
// surface: location-flag failures, flag parse errors, corrupt-store
// negatives, wrong-passphrase and scope denials, clipboard-sourced
// values through PATH stubs, and status on an envelope vault.

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

type w4FailReader struct{}

func (w4FailReader) Read([]byte) (int, error) { return 0, errors.New("synthetic read failure") }

type w4FailWriter struct{}

func (w4FailWriter) Write([]byte) (int, error) { return 0, errors.New("synthetic write failure") }

func TestRunLocationFlagErrors(t *testing.T) {
	testHome(t)
	// A location flag with no value is rejected before any mode runs.
	if code, _, e := runVault(t, "", "--folder"); code == 0 || !strings.Contains(e, "needs a value") {
		t.Errorf("--folder without value = %d, %q", code, e)
	}
	// A NUL byte makes os.Setenv itself fail.
	if code, _, e := runVault(t, "", "--folder=a\x00b", "status"); code == 0 || !strings.Contains(e, "error") {
		t.Errorf("--folder with NUL = %d, %q", code, e)
	}
	if code, _, e := runVault(t, "", "--suffix=a\x00b", "status"); code == 0 || !strings.Contains(e, "error") {
		t.Errorf("--suffix with NUL = %d, %q", code, e)
	}
}

func TestOpenKeyringDirectArms(t *testing.T) {
	home := testHome(t)
	mustInit(t)
	// An empty backend resolves through auto (file on a bare linux box).
	s := &Store{Backend: ""}
	kr, err := openKeyring(s, home, nil, io.Discard, "p: ")
	if err != nil {
		t.Fatalf("openKeyring auto: %v", err)
	}
	if kr == nil {
		t.Fatal("nil keyring")
	}
	// File backend with no env passphrase prompts; EOF is an error.
	t.Setenv(EnvPassphrase, "")
	if _, err := openKeyring(&Store{Backend: BackendFile}, home, strings.NewReader(""), io.Discard, "p: "); err == nil {
		t.Error("EOF at the passphrase prompt should error")
	}
}

func TestInitErrorArms(t *testing.T) {
	t.Run("bad flag", func(t *testing.T) {
		testHome(t)
		if code, _, _ := runVault(t, "", "init", "-nope"); code == 0 {
			t.Error("init with a bad flag should fail")
		}
	})
	t.Run("prompt EOF first", func(t *testing.T) {
		testHome(t)
		t.Setenv(EnvPassphrase, "")
		if code, _, _ := runVault(t, "", "init", "--backend=file"); code == 0 {
			t.Error("EOF at the first passphrase prompt should fail init")
		}
	})
	t.Run("prompt EOF second", func(t *testing.T) {
		testHome(t)
		t.Setenv(EnvPassphrase, "")
		if code, _, _ := runVault(t, "pw\n", "init", "--backend=file"); code == 0 {
			t.Error("EOF at the confirmation prompt should fail init")
		}
	})
	t.Run("keyring not writable", func(t *testing.T) {
		home := testHome(t)
		// A regular file where the vault folder should go makes MkdirAll fail.
		if err := os.WriteFile(filepath.Join(home, ".aql"), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
		if code, _, e := runVault(t, "", "init", "--backend=file"); code == 0 ||
			!strings.Contains(e, "keyring") {
			t.Errorf("init into a file-blocked folder = %d, %q", code, e)
		}
	})
}

func TestStatusEnvelopeAndParse(t *testing.T) {
	testHome(t)
	if code, _, _ := runVault(t, "", "status", "-nope"); code == 0 {
		t.Error("status with a bad flag should fail")
	}
	w4EnvelopeVault(t)
	code, out, e := runVault(t, "", "status")
	if code != 0 {
		t.Fatalf("status: %s", e)
	}
	if !strings.Contains(out, "passwords:") || !strings.Contains(out, "generation:") {
		t.Errorf("envelope status missing slot lines:\n%s", out)
	}
}

func TestAddErrorArms(t *testing.T) {
	t.Run("bad flag", func(t *testing.T) {
		testHome(t)
		if code, _, _ := runVault(t, "", "add", "-nope", "k"); code == 0 {
			t.Error("add with a bad flag should fail")
		}
	})
	t.Run("corrupt store", func(t *testing.T) {
		home := testHome(t)
		mustInit(t)
		w4CorruptStore(t, home)
		if code, _, _ := runVault(t, "v\n", "add", "--from-stdin", "k"); code == 0 {
			t.Error("add on a corrupt store should fail")
		}
	})
	t.Run("wrong passphrase", func(t *testing.T) {
		w4EnvelopeVault(t)
		setPass(t, "wrong")
		if code, _, e := runVault(t, "v\n", "add", "--from-stdin", "proj:k2"); code == 0 ||
			!strings.Contains(e, "wrong passphrase") {
			t.Errorf("add with wrong pass = %d, %q", code, e)
		}
	})
	t.Run("stdin EOF", func(t *testing.T) {
		testHome(t)
		mustInit(t)
		if code, _, e := runVault(t, "", "add", "--from-stdin", "k"); code == 0 ||
			!strings.Contains(e, "reading stdin") {
			t.Errorf("add --from-stdin with empty stdin = %d, %q", code, e)
		}
	})
	t.Run("prompt EOF", func(t *testing.T) {
		testHome(t)
		mustInit(t)
		if code, _, _ := runVault(t, "", "add", "k"); code == 0 {
			t.Error("add with an EOF value prompt should fail")
		}
	})
	t.Run("write blocked by unknown ndk id", func(t *testing.T) {
		home := w4EnvelopeVault(t)
		s, err := LoadStore(home)
		if err != nil {
			t.Fatal(err)
		}
		s.CurrentNDK["proj"] = "00112233aabbccdd" // an id no session holds
		if err := SaveStore(home, s); err != nil {
			t.Fatal(err)
		}
		if code, _, e := runVault(t, "v\n", "add", "--from-stdin", "proj:k9"); code == 0 || e == "" {
			t.Errorf("add under an unknown NDK id = %d, %q", code, e)
		}
	})
}

// clipStubScripts installs a clipboard toolchain into dir that
// detectClipboard will pick up on the CURRENT platform: the paste side runs
// pasteScript (which must exit) and the clear/copy side runs clearScript. It
// returns the label detectClipboard reports for that toolchain, so callers can
// assert on the announced tool name portably. On platforms whose real tool
// cannot be stubbed as a shell script (Windows/PowerShell) the test is
// skipped rather than left to fail spuriously. detectClipboard branches on
// runtime.GOOS, so the stub has to match the real OS — a single xclip stub
// (the old approach) is invisible to the macOS pbpaste/pbcopy branch.
func clipStubScripts(t *testing.T, dir, pasteScript, clearScript string) (label string) {
	t.Helper()
	switch runtime.GOOS {
	case "darwin":
		// pbpaste reads the clipboard; pbcopy consumes stdin (copy/clear).
		stubBin(t, dir, "pbpaste", pasteScript)
		stubBin(t, dir, "pbcopy", clearScript)
		return "pbpaste/pbcopy"
	case "windows":
		t.Skip("clipboard integration cannot be stubbed as a shell script on windows")
		return ""
	default: // linux, *bsd — one xclip binary serves both selections.
		stubBin(t, dir, "xclip", "case \"$*\" in\n*-o*) "+pasteScript+" ;;\n*) "+clearScript+" ;;\nesac\n")
		t.Setenv("WAYLAND_DISPLAY", "")
		t.Setenv("DISPLAY", ":0")
		return "xclip"
	}
}

// w4ClipStub installs a clipboard PATH stub whose paste output is produced by
// pasteScript (which must exit); the clear/copy side always succeeds. It is
// platform-portable via clipStubScripts.
func w4ClipStub(t *testing.T, pasteScript string) {
	t.Helper()
	dir := t.TempDir()
	clipStubScripts(t, dir, pasteScript, "cat > /dev/null; exit 0")
	t.Setenv("PATH", dir)
}

func TestAddFromClipboardArms(t *testing.T) {
	t.Run("success wipes clipboard", func(t *testing.T) {
		testHome(t)
		mustInit(t)
		// paste prints the stored secret; clear consumes stdin.
		w4ClipStub(t, "printf 'clip-secret\\n'; exit 0")
		code, out, e := runVault(t, "", "add", "--from-clipboard", "clipk")
		if code != 0 {
			t.Fatalf("add --from-clipboard: %s", e)
		}
		if !strings.Contains(out, "clipboard cleared") {
			t.Errorf("clipboard not announced cleared: %q / %q", out, e)
		}
		if code, got, _ := runVault(t, "", "get", "--reveal", "clipk"); code != 0 || strings.TrimSpace(got) != "clip-secret" {
			t.Errorf("clipboard value = %d/%q", code, got)
		}
	})
	t.Run("paste fails", func(t *testing.T) {
		testHome(t)
		mustInit(t)
		w4ClipStub(t, "exit 1")
		if code, _, e := runVault(t, "", "add", "--from-clipboard", "k"); code == 0 ||
			!strings.Contains(e, "clipboard") {
			t.Errorf("failing paste = %d, %q", code, e)
		}
	})
	t.Run("clipboard empty", func(t *testing.T) {
		testHome(t)
		mustInit(t)
		w4ClipStub(t, "exit 0")
		if code, _, e := runVault(t, "", "add", "--from-clipboard", "k"); code == 0 ||
			!strings.Contains(e, "clipboard is empty") {
			t.Errorf("empty clipboard = %d, %q", code, e)
		}
	})
}

func TestGetErrorArms(t *testing.T) {
	t.Run("bad flag", func(t *testing.T) {
		testHome(t)
		if code, _, _ := runVault(t, "", "get", "-nope", "k"); code == 0 {
			t.Error("get with a bad flag should fail")
		}
	})
	t.Run("corrupt store", func(t *testing.T) {
		home := testHome(t)
		mustInit(t)
		w4CorruptStore(t, home)
		if code, _, _ := runVault(t, "", "get", "k"); code == 0 {
			t.Error("get on a corrupt store should fail")
		}
	})
	t.Run("invalid alias ref", func(t *testing.T) {
		testHome(t)
		mustInit(t)
		if code, _, e := runVault(t, "", "get", "a:b:c"); code == 0 || !strings.Contains(e, "invalid alias") {
			t.Errorf("get with double-qualified ref = %d, %q", code, e)
		}
	})
	t.Run("missing keyring entry", func(t *testing.T) {
		home := testHome(t)
		mustInit(t)
		s, err := LoadStore(home)
		if err != nil {
			t.Fatal(err)
		}
		s.Aliases = append(s.Aliases, Alias{Name: "ghost", CreatedAt: "2020-01-01T00:00:00Z"})
		if err := SaveStore(home, s); err != nil {
			t.Fatal(err)
		}
		if code, _, e := runVault(t, "", "get", "--reveal", "ghost"); code == 0 ||
			!strings.Contains(e, "not found") {
			t.Errorf("get with missing keyring entry = %d, %q", code, e)
		}
	})
}

func TestListRmErrorArms(t *testing.T) {
	t.Run("list bad flag", func(t *testing.T) {
		testHome(t)
		if code, _, _ := runVault(t, "", "list", "-nope"); code == 0 {
			t.Error("list with a bad flag should fail")
		}
	})
	t.Run("rm bad flag", func(t *testing.T) {
		testHome(t)
		if code, _, _ := runVault(t, "", "rm", "-nope", "k"); code == 0 {
			t.Error("rm with a bad flag should fail")
		}
	})
	t.Run("rm corrupt store", func(t *testing.T) {
		home := testHome(t)
		mustInit(t)
		w4CorruptStore(t, home)
		if code, _, _ := runVault(t, "", "rm", "--yes", "k"); code == 0 {
			t.Error("rm on a corrupt store should fail")
		}
	})
	t.Run("rm wrong passphrase", func(t *testing.T) {
		w4EnvelopeVault(t)
		setPass(t, "wrong")
		if code, _, e := runVault(t, "", "rm", "--yes", "proj:k"); code == 0 ||
			!strings.Contains(e, "wrong passphrase") {
			t.Errorf("rm wrong pass = %d, %q", code, e)
		}
	})
	t.Run("rm scope denied", func(t *testing.T) {
		w4EnvelopeVault(t)
		setPass(t, "reader-pass")
		if code, _, e := runVault(t, "", "rm", "--yes", "proj:k"); code == 0 ||
			!strings.Contains(e, "permission denied") {
			t.Errorf("rm by a read password = %d, %q", code, e)
		}
	})
	t.Run("rm warns when keyring delete fails", func(t *testing.T) {
		home := w4EnvelopeVault(t)
		if err := os.WriteFile(filepath.Join(vaultFolder(home), vaultFileName("keyring")),
			[]byte("garbage-no-magic"), 0o600); err != nil {
			t.Fatal(err)
		}
		code, _, e := runVault(t, "", "rm", "--yes", "proj:k")
		if code != 0 {
			t.Fatalf("rm should still succeed with a warning: %s", e)
		}
		if !strings.Contains(e, "warning") {
			t.Errorf("missing keyring-delete warning: %q", e)
		}
	})
}

func TestImportErrorArms(t *testing.T) {
	t.Run("bad flag", func(t *testing.T) {
		testHome(t)
		if code, _, _ := runVault(t, "", "import", "-nope"); code == 0 {
			t.Error("import with a bad flag should fail")
		}
	})
	t.Run("stdin read failure", func(t *testing.T) {
		testHome(t)
		mustInit(t)
		var out, errb bytes.Buffer
		if code := Run([]string{"import"}, w4FailReader{}, &out, &errb); code == 0 ||
			!strings.Contains(errb.String(), "reading stdin") {
			t.Errorf("import with failing stdin = %d, %q", code, errb.String())
		}
	})
	t.Run("corrupt store", func(t *testing.T) {
		home := testHome(t)
		mustInit(t)
		w4CorruptStore(t, home)
		if code, _, _ := runVault(t, "K=V\n", "import"); code == 0 {
			t.Error("import on a corrupt store should fail")
		}
	})
	t.Run("wrong passphrase", func(t *testing.T) {
		w4EnvelopeVault(t)
		setPass(t, "wrong")
		if code, _, e := runVault(t, "K=V\n", "import"); code == 0 ||
			!strings.Contains(e, "wrong passphrase") {
			t.Errorf("import wrong pass = %d, %q", code, e)
		}
	})
	t.Run("scope denied", func(t *testing.T) {
		w4EnvelopeVault(t)
		setPass(t, "reader-pass")
		if code, _, e := runVault(t, "K=V\n", "import", "--namespace=proj"); code == 0 ||
			!strings.Contains(e, "permission denied") {
			t.Errorf("import by a read password = %d, %q", code, e)
		}
	})
	t.Run("write blocked by unknown ndk id", func(t *testing.T) {
		home := w4EnvelopeVault(t)
		s, err := LoadStore(home)
		if err != nil {
			t.Fatal(err)
		}
		s.CurrentNDK["proj"] = "ffeeddccbbaa9988"
		if err := SaveStore(home, s); err != nil {
			t.Fatal(err)
		}
		if code, _, e := runVault(t, "K=V\n", "import", "--namespace=proj"); code == 0 ||
			!strings.Contains(e, "storing") {
			t.Errorf("import under an unknown NDK id = %d, %q", code, e)
		}
	})
	t.Run("root namespace flag", func(t *testing.T) {
		testHome(t)
		mustInit(t)
		if code, _, e := runVault(t, "ROOTK=v\n", "import", "--namespace=:"); code != 0 {
			t.Fatalf("import --namespace=: %s", e)
		}
		if code, out, _ := runVault(t, "", "get", "--reveal", "ROOTK"); code != 0 || strings.TrimSpace(out) != "v" {
			t.Errorf("root-imported key = %d/%q", code, out)
		}
	})
}

func TestGrantRevokeConfigErrorArms(t *testing.T) {
	t.Run("grant bad flag", func(t *testing.T) {
		testHome(t)
		if code, _, _ := runVault(t, "", "grant", "-nope", "k"); code == 0 {
			t.Error("grant with a bad flag should fail")
		}
	})
	t.Run("grant corrupt store", func(t *testing.T) {
		home := testHome(t)
		mustInit(t)
		w4CorruptStore(t, home)
		if code, _, _ := runVault(t, "", "grant", "k"); code == 0 {
			t.Error("grant on a corrupt store should fail")
		}
	})
	t.Run("revoke bad flag", func(t *testing.T) {
		testHome(t)
		if code, _, _ := runVault(t, "", "revoke", "-nope", "id"); code == 0 {
			t.Error("revoke with a bad flag should fail")
		}
	})
	t.Run("config bad flag", func(t *testing.T) {
		testHome(t)
		if code, _, _ := runVault(t, "", "config", "-nope"); code == 0 {
			t.Error("config with a bad flag should fail")
		}
	})
	t.Run("config mutations need a vault", func(t *testing.T) {
		testHome(t) // no init
		if code, _, e := runVault(t, "", "config", "--set", "a=b"); code == 0 ||
			!strings.Contains(e, "not initialized") {
			t.Errorf("config --set without vault = %d, %q", code, e)
		}
		if code, _, e := runVault(t, "", "config", "--unset", "a"); code == 0 ||
			!strings.Contains(e, "not initialized") {
			t.Errorf("config --unset without vault = %d, %q", code, e)
		}
	})
}

func TestRotateErrorArms(t *testing.T) {
	t.Run("bad flag", func(t *testing.T) {
		testHome(t)
		if code, _, _ := runVault(t, "", "rotate", "-nope", "k"); code == 0 {
			t.Error("rotate with a bad flag should fail")
		}
	})
	t.Run("corrupt store", func(t *testing.T) {
		home := testHome(t)
		mustInit(t)
		w4CorruptStore(t, home)
		if code, _, _ := runVault(t, "", "rotate", "--yes", "k"); code == 0 {
			t.Error("rotate on a corrupt store should fail")
		}
	})
	t.Run("empty from-env", func(t *testing.T) {
		testHome(t)
		mustInit(t)
		if code, _, e := runVault(t, "v\n", "add", "--from-stdin", "k"); code != 0 {
			t.Fatalf("seed: %s", e)
		}
		t.Setenv("W4_UNSET_SRC", "")
		if code, _, e := runVault(t, "", "rotate", "--yes", "--from-env=W4_UNSET_SRC", "k"); code == 0 ||
			!strings.Contains(e, "empty or unset") {
			t.Errorf("rotate from empty env = %d, %q", code, e)
		}
	})
	t.Run("wrong passphrase", func(t *testing.T) {
		w4EnvelopeVault(t)
		setPass(t, "wrong")
		if code, _, e := runVault(t, "v2\n", "rotate", "--yes", "--from-stdin", "proj:k"); code == 0 ||
			!strings.Contains(e, "wrong passphrase") {
			t.Errorf("rotate wrong pass = %d, %q", code, e)
		}
	})
	t.Run("scope denied", func(t *testing.T) {
		w4EnvelopeVault(t)
		setPass(t, "reader-pass")
		if code, _, e := runVault(t, "v2\n", "rotate", "--yes", "--from-stdin", "proj:k"); code == 0 ||
			!strings.Contains(e, "permission denied") {
			t.Errorf("rotate by a read password = %d, %q", code, e)
		}
	})
	t.Run("write blocked by unknown ndk id", func(t *testing.T) {
		home := w4EnvelopeVault(t)
		s, err := LoadStore(home)
		if err != nil {
			t.Fatal(err)
		}
		s.CurrentNDK["proj"] = "1122334455667788"
		if err := SaveStore(home, s); err != nil {
			t.Fatal(err)
		}
		if code, _, e := runVault(t, "v2\n", "rotate", "--yes", "--from-stdin", "proj:k"); code == 0 || e == "" {
			t.Errorf("rotate under an unknown NDK id = %d, %q", code, e)
		}
	})
	t.Run("from clipboard", func(t *testing.T) {
		testHome(t)
		mustInit(t)
		if code, _, e := runVault(t, "v\n", "add", "--from-stdin", "k"); code != 0 {
			t.Fatalf("seed: %s", e)
		}
		w4ClipStub(t, "printf 'rotated-clip\\n'; exit 0")
		code, out, e := runVault(t, "", "rotate", "--yes", "--from-clipboard", "k")
		if code != 0 {
			t.Fatalf("rotate --from-clipboard: %s", e)
		}
		if !strings.Contains(out, "clipboard cleared") {
			t.Errorf("rotate did not announce the clipboard wipe: %q", out)
		}
		if code, got, _ := runVault(t, "", "get", "--reveal", "k"); code != 0 || strings.TrimSpace(got) != "rotated-clip" {
			t.Errorf("rotated value = %d/%q", code, got)
		}
	})
}

func TestDefaultNamespaceConfigArms(t *testing.T) {
	testHome(t)
	mustInit(t)
	if code, _, e := runVault(t, "v\n", "add", "--from-stdin", "rootk"); code != 0 {
		t.Fatalf("seed: %s", e)
	}
	// A configured ":" default means root explicitly.
	if code, _, e := runVault(t, "", "config", "--set", "namespace.default=:"); code != 0 {
		t.Fatalf("config set: %s", e)
	}
	if code, out, _ := runVault(t, "", "get", "--reveal", "rootk"); code != 0 || strings.TrimSpace(out) != "v" {
		t.Errorf("get with ':' default ns = %d/%q", code, out)
	}
	// An invalid configured default is an error, not a silent root.
	if code, _, e := runVault(t, "", "config", "--set", "namespace.default=b!ad"); code != 0 {
		t.Fatalf("config set: %s", e)
	}
	if code, _, e := runVault(t, "", "get", "--reveal", "rootk"); code == 0 ||
		!strings.Contains(e, "invalid namespace") {
		t.Errorf("get with invalid default ns = %d, %q", code, e)
	}
}
