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
	"encoding/json"
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
	// BackendOnePassword stores secrets in 1Password via the `op`
	// CLI. The user must have `op` installed and signed in to a
	// 1Password account; entries are written to the configured
	// vault as items of category "API Credential" with the alias
	// as the item title.
	BackendOnePassword = "1password"
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
	case BackendOnePassword:
		if _, err := exec.LookPath("op"); err != nil {
			return nil, fmt.Errorf("vault: 1password CLI `op` not found")
		}
		return &onePasswordKeyring{vault: os.Getenv("AQL_VAULT_1PASSWORD_VAULT")}, nil
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
	// -U updates if the entry already exists.
	//
	// NOTE(argv exposure): the secret is passed as the -w argument, so
	// it is briefly visible to other *same-user* processes via `ps`
	// while `security` runs. macOS does not expose another process's
	// argv across users, and `security` reads its interactive password
	// prompt from the controlling tty rather than stdin, so there is no
	// reliable stdin hand-off for this subcommand. A fully argv-free
	// path needs the native Keychain API (cgo) or a carefully escaped
	// `security -i` command stream; both want testing on real macOS and
	// are left as a follow-up. On a single-user machine the residual
	// risk is small.
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
	// NOTE(argv exposure): cmdkey takes the password as the /pass:
	// argument, briefly visible to same-user processes via Task
	// Manager / WMI. cmdkey has no stdin input mode and cannot read a
	// stored password back (see Get), so a robust fix means replacing
	// this backend with a PowerShell PasswordVault / CredWrite
	// implementation that also supports retrieval. That wants testing
	// on real Windows and is left as a follow-up; prefer the file or
	// 1password backend on Windows in the meantime.
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

const (
	// keyringMagic prefixes a self-describing keyring file. Older files
	// were written without it (a raw salt|nonce|ciphertext blob) and are
	// still readable; they are re-written in the current format on the
	// next save.
	keyringMagic = "AQLK"
	// keyringFormat is the keyring layout version this binary writes.
	// Bump it (and add a branch in decryptHeadered) when the KDF,
	// cipher, or byte layout changes.
	//
	//	1 — scrypt(N=2^15,r=8,p=1) + AES-256-GCM, header-bound AAD.
	keyringFormat   = 1
	keyringSaltLen  = 16
	keyringNonceLen = 12
)

// Headered layout: "AQLK" | format(1 byte) | salt(16) | nonce(12) | ciphertext|tag.
// The GCM additional data is the header bytes + salt, so the format
// byte is authenticated and cannot be downgraded by tampering.
func encryptBlob(plain []byte, passphrase string) ([]byte, error) {
	salt := make([]byte, keyringSaltLen)
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
	header := append([]byte(keyringMagic), byte(keyringFormat))
	ct := gcm.Seal(nil, nonce, plain, keyringAAD(header, salt))
	out := make([]byte, 0, len(header)+len(salt)+len(nonce)+len(ct))
	out = append(out, header...)
	out = append(out, salt...)
	out = append(out, nonce...)
	out = append(out, ct...)
	return out, nil
}

// keyringAAD binds the header and salt into the AEAD additional data.
func keyringAAD(header, salt []byte) []byte {
	aad := make([]byte, 0, len(header)+len(salt))
	aad = append(aad, header...)
	aad = append(aad, salt...)
	return aad
}

func decryptBlob(blob []byte, passphrase string) ([]byte, error) {
	if len(blob) >= len(keyringMagic)+1 && string(blob[:len(keyringMagic)]) == keyringMagic {
		return decryptHeadered(blob, passphrase)
	}
	return decryptLegacy(blob, passphrase)
}

// decryptHeadered reads the self-describing format, dispatching on the
// format byte. A newer format than this binary understands is reported
// clearly rather than surfaced as a generic "corrupt keyring".
func decryptHeadered(blob []byte, passphrase string) ([]byte, error) {
	const off = len(keyringMagic) + 1 // magic + format byte
	format := int(blob[len(keyringMagic)])
	if format > keyringFormat {
		return nil, fmt.Errorf("vault: keyring is format %d but this aql understands up to %d; upgrade aql", format, keyringFormat)
	}
	if format != 1 {
		return nil, fmt.Errorf("vault: unknown keyring format %d", format)
	}
	if len(blob) < off+keyringSaltLen+keyringNonceLen+16 {
		return nil, errors.New("vault: keyring file is truncated")
	}
	salt := blob[off : off+keyringSaltLen]
	nonce := blob[off+keyringSaltLen : off+keyringSaltLen+keyringNonceLen]
	ct := blob[off+keyringSaltLen+keyringNonceLen:]
	plain, err := aesGCMOpen(passphrase, salt, nonce, ct, keyringAAD(blob[:off], salt))
	if err != nil {
		return nil, err
	}
	return plain, nil
}

// decryptLegacy reads the original headerless salt|nonce|ciphertext
// layout (AAD = salt). Retained so vaults written before the format
// header remain readable; they are upgraded on the next save.
func decryptLegacy(blob []byte, passphrase string) ([]byte, error) {
	if len(blob) < keyringSaltLen+keyringNonceLen+16 {
		return nil, errors.New("vault: keyring file is truncated")
	}
	salt := blob[:keyringSaltLen]
	nonce := blob[keyringSaltLen : keyringSaltLen+keyringNonceLen]
	ct := blob[keyringSaltLen+keyringNonceLen:]
	return aesGCMOpen(passphrase, salt, nonce, ct, salt)
}

// aesGCMOpen derives the scrypt key and opens the GCM ciphertext,
// mapping any authentication failure to a single non-revealing error.
func aesGCMOpen(passphrase string, salt, nonce, ct, aad []byte) ([]byte, error) {
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
	plain, err := gcm.Open(nil, nonce, ct, aad)
	if err != nil {
		return nil, errors.New("vault: wrong passphrase or corrupt keyring")
	}
	return plain, nil
}

// --- 1Password via `op` CLI -------------------------------------------------

// onePasswordKeyring stores secrets in the 1Password vault named
// by AQL_VAULT_1PASSWORD_VAULT (default "Private"). Each alias
// becomes an "API Credential" item titled "aql:<alias>". The user
// must have an active `op signin` session for the `op` CLI calls
// to succeed; this is the responsibility of the host.
type onePasswordKeyring struct {
	vault string
}

func (k *onePasswordKeyring) Name() string { return BackendOnePassword }

func (k *onePasswordKeyring) itemTitle(alias string) string {
	return "aql:" + alias
}

func (k *onePasswordKeyring) opVault() string {
	if k.vault == "" {
		return "Private"
	}
	return k.vault
}

// opItemTemplate / opField are the minimal subset of the 1Password
// item-create JSON template needed to store a single concealed
// credential. The value rides in this JSON (delivered on stdin), never
// on argv.
type opItemTemplate struct {
	Title    string    `json:"title"`
	Category string    `json:"category"`
	Fields   []opField `json:"fields"`
}

type opField struct {
	ID    string `json:"id"`
	Type  string `json:"type"`
	Label string `json:"label"`
	Value string `json:"value"`
}

func (k *onePasswordKeyring) Set(alias, value string) error {
	// Best-effort delete of any prior item first. This uses the item
	// title, not the secret, so nothing sensitive reaches argv here.
	_ = exec.Command("op", "item", "delete", k.itemTitle(alias),
		"--vault", k.opVault()).Run()

	// Create from a JSON template piped on stdin (`op item create -`)
	// so the credential value never appears on this process's — or
	// op's — argv, where a same-user `ps` could read it. The label
	// "credential" matches what Get reads back.
	blob, err := json.Marshal(opItemTemplate{
		Title:    k.itemTitle(alias),
		Category: "API_CREDENTIAL",
		Fields:   []opField{{ID: "credential", Type: "CONCEALED", Label: "credential", Value: value}},
	})
	if err != nil {
		return err
	}
	cmd := exec.Command("op", "item", "create", "--vault", k.opVault(), "-")
	cmd.Stdin = bytes.NewReader(blob)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("op item create: %s: %s", err, strings.TrimSpace(out.String()))
	}
	return nil
}

func (k *onePasswordKeyring) Get(alias string) (string, error) {
	cmd := exec.Command("op", "item", "get", k.itemTitle(alias),
		"--vault", k.opVault(), "--fields", "label=credential", "--reveal")
	out, err := cmd.Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok && strings.Contains(string(ee.Stderr), "isn't an item") {
			return "", ErrNotFound
		}
		return "", fmt.Errorf("op item get: %w", err)
	}
	return strings.TrimRight(string(out), "\n"), nil
}

func (k *onePasswordKeyring) Delete(alias string) error {
	cmd := exec.Command("op", "item", "delete", k.itemTitle(alias),
		"--vault", k.opVault())
	if out, err := cmd.CombinedOutput(); err != nil {
		if strings.Contains(string(out), "isn't an item") {
			return nil
		}
		return fmt.Errorf("op item delete: %s: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}
