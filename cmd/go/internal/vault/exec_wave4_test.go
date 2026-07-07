package vault

// exec_wave4_test.go — error-arm coverage for exec.go: usage and parse
// failures, --ask/--ask-passphrase prompt plumbing (including the
// redirected-file-stdin + no-tty arm), --for collision and resolution
// failures, and child-launch failures.

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExecUsageAndParseErrors(t *testing.T) {
	testHome(t)
	mustInit(t)
	if code, _, _ := runVault(t, "", "exec", "-nope", "--", "true"); code == 0 {
		t.Error("exec with a bad flag should fail")
	}
	if code, _, e := runVault(t, "", "exec", "--", "true"); code == 0 || !strings.Contains(e, "usage") {
		t.Errorf("exec with no alias/ask spec = %d, %q", code, e)
	}
}

func TestExecStoreAndCommandErrors(t *testing.T) {
	t.Run("corrupt store", func(t *testing.T) {
		home := testHome(t)
		mustInit(t)
		w4CorruptStore(t, home)
		if code, _, _ := runVault(t, "", "exec", "k", "--", "true"); code == 0 {
			t.Error("exec on a corrupt store should fail")
		}
	})
	t.Run("command not found", func(t *testing.T) {
		testHome(t)
		if code, _, e := runVault(t, "", "exec", "--dry-run", "--ask", "X", "--", "w4-definitely-no-such-cmd"); code == 0 || e == "" {
			t.Errorf("exec of a missing command = %d, %q", code, e)
		}
	})
	t.Run("exec format error", func(t *testing.T) {
		testHome(t)
		dir := t.TempDir()
		// An executable file that is neither a script nor a binary: the
		// child fails to start (non-ExitError).
		if err := os.WriteFile(filepath.Join(dir, "w4broken"), []byte{0x00, 0x01, 0x02}, 0o755); err != nil {
			t.Fatal(err)
		}
		prependPath(t, dir)
		if code, _, e := runVault(t, "", "exec", "--dry-run", "--ask", "X", "--", "w4broken"); code == 0 || e == "" {
			t.Errorf("exec of an unloadable binary = %d, %q", code, e)
		}
	})
}

func TestExecAskPassphraseArms(t *testing.T) {
	t.Run("env passphrase is used and validated", func(t *testing.T) {
		w4EnvelopeVault(t) // EnvPassphrase = test-pass
		code, out, e := runVault(t, "", "exec", "--ask-passphrase", "--", "sh", "-c", "printf '%s' \"$AQL_VAULT_PASSPHRASE\"")
		if code != 0 {
			t.Fatalf("exec --ask-passphrase with env pass: %s", e)
		}
		if !strings.Contains(out, "test-pass") {
			t.Errorf("child did not receive the passphrase: %q", out)
		}
	})
	t.Run("prompt EOF", func(t *testing.T) {
		w4EnvelopeVault(t)
		t.Setenv(EnvPassphrase, "")
		if code, _, e := runVault(t, "", "exec", "--ask-passphrase", "--", "true"); code == 0 || e == "" {
			t.Errorf("EOF at the passphrase prompt = %d, %q", code, e)
		}
	})
	t.Run("empty passphrase typed", func(t *testing.T) {
		w4EnvelopeVault(t)
		t.Setenv(EnvPassphrase, "")
		if code, _, e := runVault(t, "\n", "exec", "--ask-passphrase", "--", "true"); code == 0 ||
			!strings.Contains(e, "empty passphrase") {
			t.Errorf("empty typed passphrase = %d, %q", code, e)
		}
	})
	t.Run("wrong envelope passphrase", func(t *testing.T) {
		w4EnvelopeVault(t)
		setPass(t, "wrong")
		if code, _, e := runVault(t, "", "exec", "--ask-passphrase", "--", "true"); code == 0 ||
			!strings.Contains(e, "wrong passphrase") {
			t.Errorf("wrong passphrase = %d, %q", code, e)
		}
	})
	t.Run("collides with an injected ask var", func(t *testing.T) {
		testHome(t)
		if code, _, e := runVault(t, "", "exec", "--dry-run", "--ask", EnvPassphrase, "--ask-passphrase", "--", "true"); code == 0 ||
			!strings.Contains(e, "collides") {
			t.Errorf("passphrase collision = %d, %q", code, e)
		}
	})
}

func TestExecAskPromptArms(t *testing.T) {
	t.Run("duplicate ask var", func(t *testing.T) {
		testHome(t)
		if code, _, e := runVault(t, "", "exec", "--dry-run", "--ask", "FOO", "--ask", "FOO", "--", "true"); code == 0 ||
			!strings.Contains(e, "collides") {
			t.Errorf("duplicate --ask = %d, %q", code, e)
		}
	})
	t.Run("prompt EOF", func(t *testing.T) {
		testHome(t)
		if code, _, e := runVault(t, "", "exec", "--ask", "FOO", "--", "true"); code == 0 || e == "" {
			t.Errorf("EOF at the --ask prompt = %d, %q", code, e)
		}
	})
	t.Run("redirected file stdin without a tty", func(t *testing.T) {
		if tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0); err == nil {
			tty.Close()
			t.Skip("a controlling terminal is available; the no-tty arm is not reachable here")
		}
		testHome(t)
		f, err := os.CreateTemp(t.TempDir(), "stdin")
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		var out, errb bytes.Buffer
		code := Run([]string{"exec", "--ask", "FOO", "--", "true"}, f, &out, &errb)
		if code == 0 || !strings.Contains(errb.String(), "cannot prompt") {
			t.Errorf("file stdin without tty = %d, %q", code, errb.String())
		}
		// The same arm through --ask-passphrase's prompt source.
		home := w4EnvelopeVault(t)
		_ = home
		t.Setenv(EnvPassphrase, "")
		errb.Reset()
		code = Run([]string{"exec", "--ask-passphrase", "--", "true"}, f, &out, &errb)
		if code == 0 || !strings.Contains(errb.String(), "cannot prompt") {
			t.Errorf("file stdin passphrase prompt = %d, %q", code, errb.String())
		}
	})
}

func TestExecForErrorArms(t *testing.T) {
	t.Run("unresolvable alias ref", func(t *testing.T) {
		testHome(t)
		mustInit(t)
		if code, _, e := runVault(t, "", "exec", "--for=npm=a:b:c", "--", "true"); code == 0 ||
			!strings.Contains(e, "invalid alias") {
			t.Errorf("--for with an invalid ref = %d, %q", code, e)
		}
	})
	t.Run("missing keyring value", func(t *testing.T) {
		home := testHome(t)
		mustInit(t)
		s, err := LoadStore(home)
		if err != nil {
			t.Fatal(err)
		}
		s.Aliases = append(s.Aliases, Alias{Name: "npmtok", CreatedAt: "2020-01-01T00:00:00Z"})
		if err := SaveStore(home, s); err != nil {
			t.Fatal(err)
		}
		if code, _, e := runVault(t, "", "exec", "--for=npm=npmtok", "--", "true"); code == 0 ||
			!strings.Contains(e, "reading npmtok") {
			t.Errorf("--for with a store-only alias = %d, %q", code, e)
		}
	})
	t.Run("conflicting recipes", func(t *testing.T) {
		testHome(t)
		mustInit(t)
		for _, kv := range [][2]string{{"tok-a", "secret-a"}, {"tok-b", "secret-b"}} {
			if code, _, e := runVault(t, kv[1]+"\n", "add", "--from-stdin", kv[0]); code != 0 {
				t.Fatalf("seed %s: %s", kv[0], e)
			}
		}
		if code, _, e := runVault(t, "", "exec", "--for=npm=tok-a", "--for=npm=tok-b", "--", "true"); code == 0 ||
			!strings.Contains(e, "different values") {
			t.Errorf("conflicting --for recipes = %d, %q", code, e)
		}
	})
	t.Run("mapping alias without value", func(t *testing.T) {
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
		if code, _, e := runVault(t, "", "exec", "ghost", "--", "true"); code == 0 ||
			!strings.Contains(e, "reading ghost") {
			t.Errorf("exec of a store-only alias = %d, %q", code, e)
		}
	})
}

func TestParseForRecipesEmpty(t *testing.T) {
	if _, err := parseForRecipes(nil, ""); err == nil ||
		!strings.Contains(err.Error(), "no --for recipes") {
		t.Errorf("parseForRecipes(nil) = %v", err)
	}
}

func TestParseExecAliasesEdges(t *testing.T) {
	// Empty parts between commas are skipped.
	got, err := parseExecAliases("a,,b", "", false)
	if err != nil || len(got) != 2 {
		t.Errorf("parseExecAliases(a,,b) = %v, %v", got, err)
	}
	// A spec that reduces to nothing is an error.
	if _, err := parseExecAliases(",", "", false); err == nil ||
		!strings.Contains(err.Error(), "no aliases") {
		t.Errorf("parseExecAliases(,) = %v", err)
	}
}
