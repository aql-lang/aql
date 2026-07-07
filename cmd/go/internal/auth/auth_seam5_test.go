package auth

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSeam5SaveUsersMarshalError(t *testing.T) {
	boom := errors.New("marshal boom")
	orig := jsonMarshalIndent
	jsonMarshalIndent = func(any, string, string) ([]byte, error) { return nil, boom }
	t.Cleanup(func() { jsonMarshalIndent = orig })

	s := NewUserStore(t.TempDir())
	if err := s.saveUsers(map[string]*User{}); !errors.Is(err, boom) {
		t.Fatalf("saveUsers = %v, want %v", err, boom)
	}
}

func TestSeam5SaveTokensMarshalError(t *testing.T) {
	boom := errors.New("marshal boom")
	orig := jsonMarshalIndent
	jsonMarshalIndent = func(any, string, string) ([]byte, error) { return nil, boom }
	t.Cleanup(func() { jsonMarshalIndent = orig })

	s := NewUserStore(t.TempDir())
	if err := s.saveTokens(map[string]*TokenInfo{}); !errors.Is(err, boom) {
		t.Fatalf("saveTokens = %v, want %v", err, boom)
	}
}

func TestSeam5GenerateTokenRandError(t *testing.T) {
	boom := errors.New("entropy boom")
	orig := randRead
	randRead = func([]byte) (int, error) { return 0, boom }
	t.Cleanup(func() { randRead = orig })

	if _, err := GenerateToken(); !errors.Is(err, boom) {
		t.Fatalf("GenerateToken = %v, want %v", err, boom)
	}
}

func TestSeam5LoginGenerateTokenError(t *testing.T) {
	s := NewUserStore(t.TempDir())
	if err := s.Register("s5@example.com", "s5user", "pw"); err != nil {
		t.Fatal(err)
	}

	boom := errors.New("entropy boom")
	orig := randRead
	randRead = func([]byte) (int, error) { return 0, boom }
	t.Cleanup(func() { randRead = orig })

	if _, _, err := s.Login("s5user", "pw"); !errors.Is(err, boom) {
		t.Fatalf("Login = %v, want %v", err, boom)
	}
}

// TestSeam5LoginSaveTokensError drives Login's saveTokens error arm with
// a real filesystem input: tokens.json is a dangling symlink into a
// missing directory, so LoadTokens sees IsNotExist (empty map) but the
// subsequent WriteFile fails with ENOENT.
func TestSeam5LoginSaveTokensError(t *testing.T) {
	dir := t.TempDir()
	s := NewUserStore(dir)
	if err := s.Register("s5b@example.com", "s5buser", "pw"); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(dir, "missing-subdir", "tokens.json")
	if err := os.Symlink(target, filepath.Join(dir, "tokens.json")); err != nil {
		t.Fatal(err)
	}

	_, _, err := s.Login("s5buser", "pw")
	if err == nil {
		t.Fatal("Login must fail when tokens.json cannot be written")
	}
	if !os.IsNotExist(err) {
		t.Fatalf("Login error = %v, want an IsNotExist write failure", err)
	}
}

func TestSeam5ReadPasswordTerminalSuccess(t *testing.T) {
	f, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	origIsTerm := isTerminal
	origRead := readPassword
	isTerminal = func(int) bool { return true }
	readPassword = func(int) ([]byte, error) { return []byte("sekret"), nil }
	t.Cleanup(func() {
		isTerminal = origIsTerm
		readPassword = origRead
	})

	var out bytes.Buffer
	pw, err := NewInputReader(f).ReadPassword("Password: ", &out)
	if err != nil {
		t.Fatalf("ReadPassword error: %v", err)
	}
	if pw != "sekret" {
		t.Fatalf("ReadPassword = %q, want %q", pw, "sekret")
	}
	// The TTY arm prints the prompt and then a newline (echo was off).
	if got := out.String(); got != "Password: \n" {
		t.Fatalf("prompt output = %q, want %q", got, "Password: \n")
	}
}

func TestSeam5ReadPasswordTerminalError(t *testing.T) {
	f, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	boom := errors.New("tty boom")
	origIsTerm := isTerminal
	origRead := readPassword
	isTerminal = func(int) bool { return true }
	readPassword = func(int) ([]byte, error) { return nil, boom }
	t.Cleanup(func() {
		isTerminal = origIsTerm
		readPassword = origRead
	})

	var out bytes.Buffer
	pw, err := NewInputReader(f).ReadPassword("Password: ", &out)
	if !errors.Is(err, boom) {
		t.Fatalf("ReadPassword = %v, want %v", err, boom)
	}
	if pw != "" {
		t.Fatalf("ReadPassword returned %q on error, want empty", pw)
	}
	if !strings.HasSuffix(out.String(), "\n") {
		t.Fatalf("TTY arm must still print the closing newline, got %q", out.String())
	}
}
