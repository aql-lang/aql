package vault

// scan_seam7_test.go — seam-7 coverage for scan.go and scan_credentials.go:
// runScan flag/warn arms, scanPath/scanFile stat/open/walk failures (the
// last via the c7walkDir seam and an unopenable unix socket), and the
// buildVaultSecretIndex error ladder.

import (
	"bytes"
	"errors"
	iofs "io/fs"
	"net"
	"os"
	"path/filepath"
	"testing"
)

func TestC7RunScanFlagParseError(t *testing.T) {
	home := testHome(t)
	var out, errb bytes.Buffer
	if code := runScan([]string{"-not-a-flag"}, home, &out, &errb); code != 1 {
		t.Fatalf("bad flag code=%d", code)
	}
}

func TestC7RunScanDefaultPath(t *testing.T) {
	home := testHome(t)
	// Scan the working directory by default; chdir into an empty dir so the
	// default "." walk is cheap and deterministic.
	t.Chdir(t.TempDir())
	var out, errb bytes.Buffer
	if code := runScan(nil, home, &out, &errb); code != 0 {
		t.Fatalf("default-path scan code=%d out=%q", code, out.String())
	}
}

func TestC7RunScanMatchVaultLoadWarning(t *testing.T) {
	home := testHome(t)
	mustInit(t)
	w4CorruptStore(t, home)
	clean := filepath.Join(t.TempDir(), "clean.txt")
	if err := os.WriteFile(clean, []byte("nothing to see\n"), 0600); err != nil {
		t.Fatal(err)
	}
	var out, errb bytes.Buffer
	if code := runScan([]string{"--match-vault", clean}, home, &out, &errb); code != 0 {
		t.Fatalf("match-vault code=%d", code)
	}
	if !bytes.Contains(errb.Bytes(), []byte("vault lookup unavailable")) {
		t.Errorf("expected lookup-unavailable warning, got %q", errb.String())
	}
}

func TestC7RunScanHomeEmpty(t *testing.T) {
	var out, errb bytes.Buffer
	if code := runScan([]string{"--home"}, "", &out, &errb); code != 0 {
		t.Fatalf("--home empty code=%d", code)
	}
	if !bytes.Contains(errb.Bytes(), []byte("cannot locate your home directory")) {
		t.Errorf("stderr=%q", errb.String())
	}
}

func TestC7RunScanPathError(t *testing.T) {
	home := testHome(t)
	var out, errb bytes.Buffer
	missing := filepath.Join(t.TempDir(), "does-not-exist")
	if code := runScan([]string{missing}, home, &out, &errb); code != 0 {
		t.Fatalf("missing path code=%d", code)
	}
	if !bytes.Contains(errb.Bytes(), []byte("scanning")) {
		t.Errorf("expected scanning warning, got %q", errb.String())
	}
}

func TestC7RunScanSingleFileWithFinding(t *testing.T) {
	home := testHome(t)
	f := filepath.Join(t.TempDir(), "leak.txt")
	// A GitHub PAT shape triggers a finding, so scanFile runs on a plain file.
	if err := os.WriteFile(f, []byte("token=ghp_"+repeatByte('a', 40)+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	var out, errb bytes.Buffer
	if code := runScan([]string{f}, home, &out, &errb); code != 2 {
		t.Fatalf("finding scan code=%d out=%q", code, out.String())
	}
}

func TestC7ScanPathWalkNonRegular(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "real.txt"), []byte("hi\n"), 0600); err != nil {
		t.Fatal(err)
	}
	// A symlink is a non-regular entry the walk must skip.
	if err := os.Symlink(filepath.Join(dir, "real.txt"), filepath.Join(dir, "link")); err != nil {
		t.Fatal(err)
	}
	if _, err := scanPath(dir, 1<<20, nil); err != nil {
		t.Fatalf("scanPath dir = %v", err)
	}
}

func TestC7ScanPathWalkError(t *testing.T) {
	old := c7walkDir
	t.Cleanup(func() { c7walkDir = old })
	// Drive the callback's walkErr != nil arm.
	c7walkDir = func(root string, fn iofs.WalkDirFunc) error {
		return fn(root, nil, errors.New("boom"))
	}
	dir := t.TempDir()
	if _, err := scanPath(dir, 1<<20, nil); err != nil {
		t.Fatalf("scanPath with walk error = %v", err)
	}
}

func TestC7ScanFileStatAndOpenErrors(t *testing.T) {
	// os.Stat failure.
	if _, err := scanFile(filepath.Join(t.TempDir(), "nope"), 1<<20, nil); err == nil {
		t.Fatal("scanFile on missing path should error")
	}
	// os.Open failure: a unix socket stats fine but cannot be opened (ENXIO).
	sockDir := t.TempDir()
	sock := filepath.Join(sockDir, "s.sock")
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	if _, err := scanFile(sock, 1<<20, nil); err == nil {
		t.Fatal("scanFile on a socket should fail to open")
	}
}

func TestC7BuildVaultSecretIndexArms(t *testing.T) {
	testHome(t)

	// Corrupt store → LoadStore error.
	corrupt := t.TempDir()
	if err := os.MkdirAll(vaultFolder(corrupt), 0700); err != nil {
		t.Fatal(err)
	}
	w4CorruptStore(t, corrupt)
	if _, err := buildVaultSecretIndex(corrupt); err == nil {
		t.Error("corrupt store should error")
	}

	// Absent store → "not initialized".
	empty := t.TempDir()
	if _, err := buildVaultSecretIndex(empty); err == nil {
		t.Error("uninitialized vault should error")
	}

	// Locked store.
	locked := testHome(t)
	mustInit(t)
	c7lockVault(t, locked)
	if _, err := buildVaultSecretIndex(locked); err == nil {
		t.Error("locked vault should error")
	}

	// Authenticate failure (wrong passphrase, envelope vault).
	env := w4EnvelopeVault(t)
	setPass(t, "wrong-pass")
	if _, err := buildVaultSecretIndex(env); err == nil {
		t.Error("wrong passphrase should error")
	}
}

func TestC7BuildVaultSecretIndexSkipsUnreadable(t *testing.T) {
	home := testHome(t)
	mustInit(t)
	if code, _, e := runVault(t, "real-value\n", "add", "--from-stdin", "good"); code != 0 {
		t.Fatalf("add good: %s", e)
	}
	// Add an alias to the store with no keyring value so getValue fails and
	// the loop skips it (continue).
	s, err := LoadStore(home)
	if err != nil {
		t.Fatal(err)
	}
	s.UpsertAlias(Alias{Name: "ghost"})
	if err := SaveStore(home, s); err != nil {
		t.Fatal(err)
	}
	idx, err := buildVaultSecretIndex(home)
	if err != nil {
		t.Fatalf("buildVaultSecretIndex = %v", err)
	}
	if idx["real-value"] != "good" {
		t.Errorf("readable secret missing from index: %+v", idx)
	}
}

func TestC7ScanCredentialFileOpenError(t *testing.T) {
	if res := scanCredentialFile(filepath.Join(t.TempDir(), "nope"), "npm",
		credentialFiles[0].Patterns, nil); res != nil {
		t.Errorf("open failure should yield nil, got %+v", res)
	}
}

func TestC7IsPlaceholderValueMatch(t *testing.T) {
	if !isPlaceholderValue("my-example-token") {
		t.Error("value containing 'example' should be a placeholder")
	}
	if isPlaceholderValue("realtokenvalue12345") {
		t.Error("a real value should not be a placeholder")
	}
}

// repeatByte returns a string of n copies of b.
func repeatByte(b byte, n int) string {
	out := make([]byte, n)
	for i := range out {
		out[i] = b
	}
	return string(out)
}
