// Wave-4 coverage for the auth package error arms: unreadable and
// malformed store files, save failures, oversized bcrypt input, the
// prompt readers, and the client-user round trip edge cases.
package auth

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestW4LoadUsersMalformed(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "users.json"), []byte("{nope"), 0600); err != nil {
		t.Fatal(err)
	}
	s := NewUserStore(dir)
	if _, err := s.LoadUsers(); err == nil {
		t.Error("LoadUsers on malformed JSON succeeded, want error")
	}
}

func TestW4LoadUsersReadError(t *testing.T) {
	// users.json exists as a DIRECTORY: ReadFile fails with a
	// non-IsNotExist error.
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "users.json"), 0755); err != nil {
		t.Fatal(err)
	}
	s := NewUserStore(dir)
	if _, err := s.LoadUsers(); err == nil {
		t.Error("LoadUsers on a directory succeeded, want error")
	}
}

func TestW4LoadTokensMalformed(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "tokens.json"), []byte("[broken"), 0600); err != nil {
		t.Fatal(err)
	}
	s := NewUserStore(dir)
	if _, err := s.LoadTokens(); err == nil {
		t.Error("LoadTokens on malformed JSON succeeded, want error")
	}
}

func TestW4LoadTokensReadError(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "tokens.json"), 0755); err != nil {
		t.Fatal(err)
	}
	s := NewUserStore(dir)
	if _, err := s.LoadTokens(); err == nil {
		t.Error("LoadTokens on a directory succeeded, want error")
	}
}

func TestW4SaveUsersWriteError(t *testing.T) {
	// The store dir does not exist, so WriteFile fails.
	s := NewUserStore(filepath.Join(t.TempDir(), "zz-missing"))
	if err := s.saveUsers(map[string]*User{}); err == nil {
		t.Error("saveUsers into a missing dir succeeded, want error")
	}
}

func TestW4SaveTokensWriteError(t *testing.T) {
	s := NewUserStore(filepath.Join(t.TempDir(), "zz-missing"))
	if err := s.saveTokens(map[string]*TokenInfo{}); err == nil {
		t.Error("saveTokens into a missing dir succeeded, want error")
	}
}

func TestW4RegisterLoadUsersError(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "users.json"), []byte("{nope"), 0600); err != nil {
		t.Fatal(err)
	}
	s := NewUserStore(dir)
	if err := s.Register("a@b.c", "alice", "pw"); err == nil {
		t.Error("Register with a corrupt user store succeeded, want error")
	}
}

func TestW4RegisterHashError(t *testing.T) {
	// bcrypt rejects passwords longer than 72 bytes.
	dir := t.TempDir()
	s := NewUserStore(dir)
	long := strings.Repeat("p", 100)
	if err := s.Register("a@b.c", "alice", long); err == nil {
		t.Error("Register with a 100-byte password succeeded, want bcrypt error")
	}
}

func TestW4HashPasswordTooLong(t *testing.T) {
	if _, err := HashPassword(strings.Repeat("x", 80)); err == nil {
		t.Error("HashPassword(80 bytes) succeeded, want bcrypt error")
	}
}

func TestW4LoginLoadUsersError(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "users.json"), []byte("{nope"), 0600); err != nil {
		t.Fatal(err)
	}
	s := NewUserStore(dir)
	if _, _, err := s.Login("alice", "pw"); err == nil {
		t.Error("Login with a corrupt user store succeeded, want error")
	}
}

func TestW4LoginLoadTokensError(t *testing.T) {
	dir := t.TempDir()
	s := NewUserStore(dir)
	if err := s.Register("a@b.c", "alice", "pw"); err != nil {
		t.Fatal(err)
	}
	// Corrupt the token store after a valid registration.
	if err := os.WriteFile(filepath.Join(dir, "tokens.json"), []byte("{nope"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.Login("alice", "pw"); err == nil {
		t.Error("Login with a corrupt token store succeeded, want error")
	}
}

func TestW4ValidateTokenLoadError(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "tokens.json"), []byte("{nope"), 0600); err != nil {
		t.Fatal(err)
	}
	s := NewUserStore(dir)
	if _, err := s.ValidateToken("tok"); err == nil {
		t.Error("ValidateToken with a corrupt token store succeeded, want error")
	}
}

func TestW4ReadLine(t *testing.T) {
	var out strings.Builder
	ir := NewInputReader(strings.NewReader("  hello world  \n"))
	got, err := ir.ReadLine("Name: ", &out)
	if err != nil {
		t.Fatalf("ReadLine: %s", err)
	}
	if got != "hello world" {
		t.Errorf("ReadLine = %q, want hello world", got)
	}
	if !strings.Contains(out.String(), "Name: ") {
		t.Errorf("prompt not written: %q", out.String())
	}

	// Empty input errors.
	ir2 := NewInputReader(strings.NewReader(""))
	if _, err := ir2.ReadLine("x: ", &out); err == nil {
		t.Error("ReadLine on empty input succeeded, want error")
	}

	// A final line without newline is still returned.
	ir3 := NewInputReader(strings.NewReader("tail"))
	got, err = ir3.ReadLine("x: ", &out)
	if err != nil || got != "tail" {
		t.Errorf("ReadLine(no-newline) = (%q, %v), want (tail, nil)", got, err)
	}
}

func TestW4ReadPasswordNonTerminal(t *testing.T) {
	var out strings.Builder
	ir := NewInputReader(strings.NewReader("s3cret\n"))
	got, err := ir.ReadPassword("Password: ", &out)
	if err != nil {
		t.Fatalf("ReadPassword: %s", err)
	}
	if got != "s3cret" {
		t.Errorf("ReadPassword = %q, want s3cret", got)
	}

	ir2 := NewInputReader(strings.NewReader(""))
	if _, err := ir2.ReadPassword("Password: ", &out); err == nil {
		t.Error("ReadPassword on empty input succeeded, want error")
	}
}

func TestW4SaveClientUserVaultAlias(t *testing.T) {
	home := t.TempDir()
	cu := &ClientUser{
		Username:   "alice",
		Email:      "a@b.c",
		Token:      "",
		Registry:   "http://localhost:8080",
		TokenVault: "my-alias",
	}
	if err := SaveClientUser(home, cu); err != nil {
		t.Fatalf("SaveClientUser: %s", err)
	}
	data, err := os.ReadFile(filepath.Join(home, ".aql", "user.jsonic"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "token_vault: my-alias") {
		t.Errorf("user.jsonic missing token_vault line: %q", string(data))
	}

	// Round trip through LoadClientUser, including the token_vault key.
	got, err := LoadClientUser(home)
	if err != nil {
		t.Fatalf("LoadClientUser: %s", err)
	}
	if got.Username != "alice" || got.TokenVault != "my-alias" || got.Registry != "http://localhost:8080" {
		t.Errorf("LoadClientUser = %+v", got)
	}
}

func TestW4SaveClientUserMkdirError(t *testing.T) {
	// homeDir/.aql exists as a FILE, so MkdirAll fails.
	home := t.TempDir()
	if err := os.WriteFile(filepath.Join(home, ".aql"), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	cu := &ClientUser{Username: "alice"}
	if err := SaveClientUser(home, cu); err == nil {
		t.Error("SaveClientUser with .aql-as-file succeeded, want error")
	}
}

func TestW4LoadClientUserMissing(t *testing.T) {
	if _, err := LoadClientUser(t.TempDir()); err == nil {
		t.Error("LoadClientUser without a file succeeded, want error")
	}
}

func TestW4LoadClientUserSkipsJunkLines(t *testing.T) {
	home := t.TempDir()
	dir := filepath.Join(home, ".aql")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	content := "\nusername: bob\njunk-line-without-separator\n\nemail: b@c.d\ntoken: tk\nunknown: ignored\n"
	if err := os.WriteFile(filepath.Join(dir, "user.jsonic"), []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	cu, err := LoadClientUser(home)
	if err != nil {
		t.Fatalf("LoadClientUser: %s", err)
	}
	if cu.Username != "bob" || cu.Email != "b@c.d" || cu.Token != "tk" {
		t.Errorf("LoadClientUser = %+v", cu)
	}
	if cu.Registry != "" || cu.TokenVault != "" {
		t.Errorf("unexpected fields populated: %+v", cu)
	}
}
