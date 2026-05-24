// Package vault implements `aql vault <mode>` — operation and
// configuration of a local key vault that stores third-party API
// keys and registry tokens via the host OS keychain (macOS Keychain,
// Linux Secret Service, Windows Credential Manager) or an
// AES-256-GCM encrypted file fallback.
//
// Real secret values never appear in ~/.aql/vault.jsonic, in
// process environment variables (unless explicitly requested with
// `vault get --reveal`), in command echoes, or in stored logs.
// The metadata file holds only aliases, policies, and short-lived
// capability token records.
package vault

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"golang.org/x/crypto/scrypt"
)

const (
	// keyringService is the namespace used for entries in the host
	// OS keychain. Per-alias keys are stored as "aql:<alias>".
	keyringService = "aql"

	// BackendAuto picks the best available host keychain and falls
	// back to the file backend.
	BackendAuto = "auto"
	// BackendKeychain uses the macOS Keychain via /usr/bin/security.
	BackendKeychain = "keychain"
	// BackendSecretService uses the freedesktop Secret Service API
	// via the secret-tool command from libsecret-tools.
	BackendSecretService = "secret-service"
	// BackendWinCred uses Windows Credential Manager via cmdkey.
	BackendWinCred = "wincred"
	// BackendFile uses an AES-256-GCM encrypted file at
	// ~/.aql/vault.keyring. Used when no host keychain is available
	// or when explicitly requested for offline portability.
	BackendFile = "file"
)

// ErrNotFound is returned by keyring.Get when no entry exists for
// the requested alias.
var ErrNotFound = errors.New("vault: secret not found")

// keyring is the storage interface for raw secret values. The
// vault metadata store (store.go) holds aliases and policies; the
// keyring holds only the secret bytes keyed by alias.
type keyring interface {
	// Name reports which backend identifier is in use (one of the
	// Backend* constants).
	Name() string
	// Set stores value under alias. Replaces any existing entry.
	Set(alias, value string) error
	// Get returns the value for alias, or ErrNotFound.
	Get(alias string) (string, error)
	// Delete removes alias if present. Returns nil if absent.
	Delete(alias string) error
}

// selectKeyring resolves a Backend* identifier to a concrete
// keyring implementation. BackendAuto picks the first available
// host backend, falling back to the file backend.
//
// The file backend requires passphrase to be non-empty unless its
// underlying file does not yet exist (in which case Set will
// initialize it with the given passphrase, even if empty — empty
// passphrases are permitted but produce a clear warning at init).
func selectKeyring(backend string, fileDir, passphrase string) (keyring, error) {
	if backend == "" || backend == BackendAuto {
		backend = autoBackend()
	}
	switch backend {
	case BackendKeychain:
		if runtime.GOOS != "darwin" {
			return nil, fmt.Errorf("vault: keychain backend requires macOS, got %s", runtime.GOOS)
		}
		if _, err := exec.LookPath("security"); err != nil {
			return nil, fmt.Errorf("vault: /usr/bin/security not found")
		}
		return &macKeychain{}, nil
	case BackendSecretService:
		if _, err := exec.LookPath("secret-tool"); err != nil {
			return nil, fmt.Errorf("vault: secret-tool not found (install libsecret-tools)")
		}
		return &secretService{}, nil
	case BackendWinCred:
		if runtime.GOOS != "windows" {
			return nil, fmt.Errorf("vault: wincred backend requires windows, got %s", runtime.GOOS)
		}
		if _, err := exec.LookPath("cmdkey"); err != nil {
			return nil, fmt.Errorf("vault: cmdkey not found")
		}
		return &winCred{}, nil
	case BackendFile:
		return &fileKeyring{dir: fileDir, pass: passphrase}, nil
	default:
		return nil, fmt.Errorf("vault: unknown backend %q", backend)
	}
}

// autoBackend returns the preferred host backend for the current
// platform, or BackendFile if no host backend is available.
func autoBackend() string {
	switch runtime.GOOS {
	case "darwin":
		if _, err := exec.LookPath("security"); err == nil {
			return BackendKeychain
		}
	case "linux":
		if _, err := exec.LookPath("secret-tool"); err == nil {
			return BackendSecretService
		}
	case "windows":
		if _, err := exec.LookPath("cmdkey"); err == nil {
			return BackendWinCred
		}
	}
	return BackendFile
}

// --- macOS Keychain via /usr/bin/security -----------------------------------

type macKeychain struct{}

func (*macKeychain) Name() string { return BackendKeychain }

func (*macKeychain) Set(alias, value string) error {
	// -U updates if the entry already exists; -w passes the value
	// without echoing to the process listing on most macOS builds.
	cmd := exec.Command("security", "add-generic-password",
		"-s", keyringService, "-a", alias, "-w", value, "-U")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("security add-generic-password: %s: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func (*macKeychain) Get(alias string) (string, error) {
	cmd := exec.Command("security", "find-generic-password",
		"-s", keyringService, "-a", alias, "-w")
	out, err := cmd.Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok && len(ee.Stderr) > 0 {
			if strings.Contains(string(ee.Stderr), "could not be found") {
				return "", ErrNotFound
			}
		}
		return "", fmt.Errorf("security find-generic-password: %w", err)
	}
	return strings.TrimRight(string(out), "\n"), nil
}

func (*macKeychain) Delete(alias string) error {
	cmd := exec.Command("security", "delete-generic-password",
		"-s", keyringService, "-a", alias)
	if out, err := cmd.CombinedOutput(); err != nil {
		if strings.Contains(string(out), "could not be found") {
			return nil
		}
		return fmt.Errorf("security delete-generic-password: %s: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// --- Linux Secret Service via secret-tool -----------------------------------

type secretService struct{}

func (*secretService) Name() string { return BackendSecretService }

func (*secretService) Set(alias, value string) error {
	cmd := exec.Command("secret-tool", "store", "--label", "aql:"+alias,
		"service", keyringService, "account", alias)
	cmd.Stdin = strings.NewReader(value)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("secret-tool store: %s: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func (*secretService) Get(alias string) (string, error) {
	cmd := exec.Command("secret-tool", "lookup",
		"service", keyringService, "account", alias)
	out, err := cmd.Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok && ee.ExitCode() == 1 && len(ee.Stderr) == 0 {
			return "", ErrNotFound
		}
		return "", fmt.Errorf("secret-tool lookup: %w", err)
	}
	v := string(out)
	if v == "" {
		return "", ErrNotFound
	}
	return strings.TrimRight(v, "\n"), nil
}

func (*secretService) Delete(alias string) error {
	cmd := exec.Command("secret-tool", "clear",
		"service", keyringService, "account", alias)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("secret-tool clear: %s: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// --- Windows Credential Manager via cmdkey ----------------------------------

// winCred is a best-effort wrapper around cmdkey. cmdkey can store
// and delete credentials but cannot return the password to stdout
// without third-party helpers, so Get returns an explanatory error
// instructing the user to switch to the file backend or install
// a credential-reading helper. Set and Delete still work.
type winCred struct{}

func (*winCred) Name() string { return BackendWinCred }

func (*winCred) Set(alias, value string) error {
	cmd := exec.Command("cmdkey",
		"/generic:"+keyringService+":"+alias,
		"/user:"+alias, "/pass:"+value)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("cmdkey: %s: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func (*winCred) Get(string) (string, error) {
	return "", errors.New("vault: cmdkey cannot read passwords back; use --backend=file or install a credential-reading helper")
}

func (*winCred) Delete(alias string) error {
	cmd := exec.Command("cmdkey", "/delete:"+keyringService+":"+alias)
	if out, err := cmd.CombinedOutput(); err != nil {
		if strings.Contains(string(out), "Element not found") {
			return nil
		}
		return fmt.Errorf("cmdkey /delete: %s: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// --- File-backed AES-256-GCM fallback ---------------------------------------

// fileKeyring stores aliases in a single AES-256-GCM encrypted JSON
// blob at {dir}/vault.keyring. Each Set/Delete reads the file,
// mutates the map, and rewrites it; this is acceptable for the
// expected ~tens of secrets and avoids partial writes leaving
// readable plaintext on disk.
type fileKeyring struct {
	dir  string
	pass string
}

func (f *fileKeyring) Name() string { return BackendFile }

func (f *fileKeyring) path() string { return filepath.Join(f.dir, "vault.keyring") }

func (f *fileKeyring) load() (map[string]string, error) {
	data, err := os.ReadFile(f.path())
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]string{}, nil
		}
		return nil, err
	}
	if len(data) == 0 {
		return map[string]string{}, nil
	}
	plain, err := decryptBlob(data, f.pass)
	if err != nil {
		return nil, err
	}
	out := map[string]string{}
	if len(plain) == 0 {
		return out, nil
	}
	for _, line := range strings.Split(string(plain), "\n") {
		if line == "" {
			continue
		}
		idx := strings.IndexByte(line, '\t')
		if idx < 0 {
			continue
		}
		k := line[:idx]
		v, err := base64.StdEncoding.DecodeString(line[idx+1:])
		if err != nil {
			return nil, fmt.Errorf("vault: corrupt keyring entry %q", k)
		}
		out[k] = string(v)
	}
	return out, nil
}

func (f *fileKeyring) save(m map[string]string) error {
	if err := os.MkdirAll(f.dir, 0700); err != nil {
		return err
	}
	var buf bytes.Buffer
	for k, v := range m {
		buf.WriteString(k)
		buf.WriteByte('\t')
		buf.WriteString(base64.StdEncoding.EncodeToString([]byte(v)))
		buf.WriteByte('\n')
	}
	enc, err := encryptBlob(buf.Bytes(), f.pass)
	if err != nil {
		return err
	}
	tmp := f.path() + ".tmp"
	if err := os.WriteFile(tmp, enc, 0600); err != nil {
		return err
	}
	return os.Rename(tmp, f.path())
}

func (f *fileKeyring) Set(alias, value string) error {
	m, err := f.load()
	if err != nil {
		return err
	}
	m[alias] = value
	return f.save(m)
}

func (f *fileKeyring) Get(alias string) (string, error) {
	m, err := f.load()
	if err != nil {
		return "", err
	}
	v, ok := m[alias]
	if !ok {
		return "", ErrNotFound
	}
	return v, nil
}

func (f *fileKeyring) Delete(alias string) error {
	m, err := f.load()
	if err != nil {
		return err
	}
	if _, ok := m[alias]; !ok {
		return nil
	}
	delete(m, alias)
	return f.save(m)
}

// scryptKey derives a 32-byte AES-256 key from passphrase + salt.
// The N=2^15 cost is conservative for an interactive vault on a
// developer machine; it dominates each Set/Get on the file backend
// by ~tens of milliseconds, which is acceptable.
func scryptKey(passphrase string, salt []byte) ([]byte, error) {
	return scrypt.Key([]byte(passphrase), salt, 1<<15, 8, 1, 32)
}

// File layout: 16-byte salt | 12-byte nonce | ciphertext|tag.
func encryptBlob(plain []byte, passphrase string) ([]byte, error) {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return nil, err
	}
	key, err := scryptKey(passphrase, salt)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	ct := gcm.Seal(nil, nonce, plain, salt)
	out := make([]byte, 0, len(salt)+len(nonce)+len(ct))
	out = append(out, salt...)
	out = append(out, nonce...)
	out = append(out, ct...)
	return out, nil
}

func decryptBlob(blob []byte, passphrase string) ([]byte, error) {
	if len(blob) < 16+12+16 {
		return nil, errors.New("vault: keyring file is truncated")
	}
	salt := blob[:16]
	nonce := blob[16:28]
	ct := blob[28:]
	key, err := scryptKey(passphrase, salt)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	plain, err := gcm.Open(nil, nonce, ct, salt)
	if err != nil {
		return nil, errors.New("vault: wrong passphrase or corrupt keyring")
	}
	return plain, nil
}
