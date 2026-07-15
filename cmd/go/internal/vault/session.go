package vault

// session.go — authenticate a vault command into a Session.
//
// A Session is the authenticated context for one command: the opened
// keyring plus, in envelope mode, the slot's scope, the namespace data
// keys (NDKs) it may use, and the integrity key. For a legacy vault with
// no password slots it is behaviour-identical to the old openKeyring:
// implicit admin, raw (un-enveloped) values. All envelope machinery is
// dormant until a vault has slots, so legacy vaults are unaffected.

import (
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/aql-lang/aql/cmd/go/internal/auth"
)

// errSlotExpired is returned when a supplied passphrase matches a
// temporary password slot whose ExpiresAt has passed. It is distinct from
// errSlotAuth so the holder (who presented the right password) gets a
// clear "expired" message rather than a misleading "wrong passphrase".
var errSlotExpired = errors.New("vault: this temporary password has expired")

// slotExpired reports whether slot's optional ExpiresAt has passed as of
// now. An empty ExpiresAt means the slot never expires. An unparseable
// timestamp fails closed (treated as expired) so a corrupt or tampered
// expiry can never extend a temporary password past its intended life.
func slotExpired(slot *PasswordSlot, now time.Time) bool {
	if slot.ExpiresAt == "" {
		return false
	}
	exp, err := time.Parse(time.RFC3339, slot.ExpiresAt)
	if err != nil {
		return true
	}
	return !now.Before(exp)
}

// Session carries one command's authenticated state.
type Session struct {
	kr      keyring
	Slot    *PasswordSlot     // nil == legacy implicit-admin
	Scope   string            // ScopeAdmin for legacy
	ndks    map[string][]byte // "ns#idhex" -> data key
	byID    map[string][]byte // idhex -> data key (for opening values)
	current map[string]string // ns -> current idhex (for new writes)
	privKey []byte            // slot X25519 private key
	ikey    []byte            // integrity key (nil if none / legacy)
}

// Keyring exposes the opaque storage backend.
func (s *Session) Keyring() keyring { return s.kr }

// IntegrityKey returns the store-integrity HMAC key, or nil when the
// session cannot reach it (legacy vault, or a slot lacking the integrity
// NDK).
func (s *Session) IntegrityKey() []byte { return s.ikey }

// legacy reports the no-slots implicit-admin mode.
func (s *Session) legacy() bool { return s == nil || s.Slot == nil }

// Close zeroizes the private key, NDKs, and integrity key.
func (s *Session) Close() {
	if s == nil {
		return
	}
	zeroize(s.privKey)
	for _, k := range s.ndks {
		zeroize(k)
	}
}

// resolveBackend mirrors openKeyring's backend resolution.
func resolveBackend(s *Store) string {
	b := s.Backend
	if b == "" || b == BackendAuto {
		return autoBackend()
	}
	return b
}

// openEnvelopeKeyring returns the opaque-ciphertext keyring for an
// envelope-mode vault: the plaintext envelope file backend, or the host
// keychain backend (which stores opaque strings unchanged).
func openEnvelopeKeyring(s *Store, homeDir string) (keyring, error) {
	if resolveBackend(s) == BackendFile {
		return &envelopeFileKeyring{folder: vaultFolder(homeDir)}, nil
	}
	return selectKeyring(s.Backend, vaultFolder(homeDir), "")
}

// authenticate resolves the active session. With no password slots it is
// the legacy implicit-admin path (identical to openKeyring). With slots,
// it matches the supplied passphrase against each slot and unseals that
// slot's keys.
func authenticate(s *Store, homeDir string, stdin io.Reader, stdout io.Writer, prompt string) (*Session, error) {
	if !s.HasPasswordSlots() {
		kr, err := openKeyring(s, homeDir, stdin, stdout, prompt)
		if err != nil {
			return nil, err
		}
		return &Session{kr: kr, Scope: ScopeAdmin}, nil
	}
	pass, err := readPassphrase(stdin, stdout, prompt)
	if err != nil {
		return nil, err
	}
	return openSession(s, homeDir, pass)
}

// readPassphrase sources the passphrase from AQL_VAULT_PASSPHRASE or an
// interactive prompt.
func readPassphrase(stdin io.Reader, stdout io.Writer, prompt string) (string, error) {
	if p := os.Getenv(EnvPassphrase); p != "" {
		return p, nil
	}
	if stdin == nil {
		return "", errors.New("vault requires a passphrase; set AQL_VAULT_PASSPHRASE for non-interactive use")
	}
	ir := auth.NewInputReader(stdin)
	p, err := ir.ReadPassword(prompt, stdout)
	if err != nil {
		return "", err
	}
	if p == "" {
		return "", errors.New("empty passphrase")
	}
	return p, nil
}

// authenticateWith resolves a session from a passphrase already in hand
// (e.g. one the TUI collected via a masked input field) instead of reading
// stdin. It mirrors authenticate's resolution exactly: a legacy vault opens
// the keyring directly — needing the passphrase only for the file backend —
// and an envelope vault matches the passphrase against its slots via
// openSession.
func authenticateWith(s *Store, homeDir, pass string) (*Session, error) {
	if !s.HasPasswordSlots() {
		backend := s.Backend
		if backend == "" {
			backend = BackendAuto
		}
		resolved := backend
		if backend == BackendAuto {
			resolved = autoBackend()
		}
		if resolved != BackendFile {
			kr, err := selectKeyring(backend, vaultFolder(homeDir), "")
			if err != nil {
				return nil, err
			}
			return &Session{kr: kr, Scope: ScopeAdmin}, nil
		}
		if pass == "" {
			return nil, errors.New("file backend requires a passphrase")
		}
		// selectKeyring(BackendFile, …) only ever constructs a *fileKeyring
		// and cannot fail, so build it directly rather than leave the
		// unreachable error arm uncovered (design/TEST-SEAMS.10.md, ADR-005).
		kr := &fileKeyring{folder: vaultFolder(homeDir), pass: pass}
		return &Session{kr: kr, Scope: ScopeAdmin}, nil
	}
	return openSession(s, homeDir, pass)
}

// vaultNeedsPassphrase reports whether opening a secret/admin session for s
// will require a passphrase: envelope vaults always do; a legacy vault does
// only for the file backend (host keychains need none).
func vaultNeedsPassphrase(s *Store) bool {
	return s.HasPasswordSlots() || resolveBackend(s) == BackendFile
}

// verifyPassphrase confirms pass can actually open s's secrets, so a caller
// that caches the passphrase (the interactive TUI) never accepts a mistyped
// one and then runs a mutation under it.
//
//   - Envelope vault: openSession authenticates pass against a keyslot's
//     verifier; a wrong passphrase returns errSlotAuth.
//   - Legacy FILE backend: authenticateWith builds the file keyring WITHOUT
//     verifying (it defers to the first value read), so verify here by
//     decrypting the AES-GCM keyring blob — a wrong passphrase fails the GCM
//     tag. A vault with no encrypted keyring yet (no secrets stored) has no
//     ciphertext to check against, so any passphrase is provisionally
//     accepted (it becomes the key that encrypts the first secret) — the same
//     first-use semantics the file backend has always had.
//   - Host keychain / 1Password: the OS guards access and there is no
//     passphrase to verify.
func verifyPassphrase(s *Store, homeDir, pass string) error {
	if s.HasPasswordSlots() {
		sess, err := openSession(s, homeDir, pass)
		if err != nil {
			return err
		}
		sess.Close()
		return nil
	}
	if resolveBackend(s) != BackendFile {
		return nil
	}
	if pass == "" {
		return errors.New("file backend requires a passphrase")
	}
	kr := &fileKeyring{folder: vaultFolder(homeDir), pass: pass}
	if _, err := kr.load(); err != nil {
		return fmt.Errorf("passphrase does not open this vault: %w", err)
	}
	return nil
}

// openSession runs the single scrypt over the shared VaultSalt, then
// tries each slot's HKDF-derived KEK against its verifier. The matching
// slot's private key and granted NDKs are unsealed. No match -> the
// non-revealing errSlotAuth.
func openSession(s *Store, homeDir, pass string) (*Session, error) {
	vsalt, err := base64.StdEncoding.DecodeString(s.VaultSalt)
	if err != nil || len(vsalt) == 0 {
		return nil, errors.New("vault: missing vault salt (corrupt store)")
	}
	master, err := s7vaultMasterKEK(pass, vsalt)
	if err != nil {
		return nil, err
	}
	defer zeroize(master)

	sawExpired := false
	for i := range s.Passwords {
		slot := &s.Passwords[i]
		salt, err := base64.StdEncoding.DecodeString(slot.Salt)
		if err != nil {
			continue
		}
		kek, err := s7slotKEK(master, salt)
		if err != nil {
			continue
		}
		if !verifyMatches(slot.Verifier, deriveVerifier(kek, slot.Name, slot.Scope)) {
			zeroize(kek)
			continue
		}
		if slotExpired(slot, time.Now()) {
			// The passphrase matched a temporary slot whose max duration has
			// elapsed. Do NOT return yet: the same passphrase may also be a
			// permanent (or still-valid) slot's — password reuse is not
			// forbidden — and an expired temporary slot must not shadow it.
			// Keep scanning; only report expiry if nothing unexpired matches.
			zeroize(kek)
			sawExpired = true
			continue
		}
		priv, err := openPrivKey(kek, slot.EncPrivKey, privAAD(slot.Name, slot.Scope, slot.ExpiresAt))
		zeroize(kek)
		if err != nil {
			return nil, err // tampered scope/namespace binding, or corrupt slot
		}
		return buildSession(s, homeDir, slot, priv)
	}
	if sawExpired {
		return nil, errSlotExpired
	}
	return nil, errSlotAuth
}

// buildSession opens the keyring and unseals the matched slot's NDKs and
// integrity key.
func buildSession(s *Store, homeDir string, slot *PasswordSlot, priv *[32]byte) (*Session, error) {
	pubBytes, err := base64.StdEncoding.DecodeString(slot.PublicKey)
	if err != nil || len(pubBytes) != 32 {
		return nil, errors.New("vault: invalid slot public key")
	}
	var pub [32]byte
	copy(pub[:], pubBytes)

	kr, err := openEnvelopeKeyring(s, homeDir)
	if err != nil {
		return nil, err
	}
	sess := &Session{
		kr:      kr,
		Slot:    slot,
		Scope:   slot.Scope,
		ndks:    map[string][]byte{},
		byID:    map[string][]byte{},
		current: map[string]string{},
		privKey: priv[:],
	}
	for k, v := range s.CurrentNDK {
		sess.current[k] = v
	}
	for wk, sealed := range slot.WrappedKeys {
		ndk, err := openNDK(sealed, &pub, priv)
		if err != nil {
			continue
		}
		sess.ndks[wk] = ndk
		if i := strings.LastIndexByte(wk, '#'); i >= 0 {
			sess.byID[wk[i+1:]] = ndk
		}
	}
	if id := sess.current[integrityNamespace]; id != "" {
		sess.ikey = sess.ndks[integrityNamespace+"#"+id]
	}
	return sess, nil
}

// ndkByID resolves a value's NDK id to its key (for opening sealed
// values).
func (s *Session) ndkByID(id []byte) ([]byte, bool) {
	k, ok := s.byID[hex.EncodeToString(id)]
	return k, ok
}

// getValue retrieves and, in envelope mode, decrypts the value for alias
// in namespace ns.
func (s *Session) getValue(alias, ns string) (string, error) {
	raw, err := s.kr.Get(alias)
	if err != nil {
		return "", err
	}
	if s.legacy() {
		return raw, nil
	}
	return openValue(raw, ns, alias, s.ndkByID)
}

// setValue stores value for alias, sealing it under the namespace's
// current NDK in envelope mode.
func (s *Session) setValue(alias, ns, value string) error {
	if s.legacy() {
		return s.kr.Set(alias, value)
	}
	id := s.current[ns]
	if id == "" {
		return fmt.Errorf("vault: no data key for namespace %q (not granted to this password)", nsLabel(ns))
	}
	ndk, ok := s.ndks[ns+"#"+id]
	if !ok {
		return fmt.Errorf("vault: this password cannot write namespace %q", nsLabel(ns))
	}
	idBytes, err := hex.DecodeString(id)
	if err != nil {
		return err
	}
	sealed, err := sealValue(ndk, idBytes, ns, alias, value)
	if err != nil {
		return err
	}
	return s.kr.Set(alias, sealed)
}

// deleteValue removes a stored value.
func (s *Session) deleteValue(alias string) error { return s.kr.Delete(alias) }

// ensureSessionNamespace makes the session able to write namespace ns.
// If the session already holds ns's current data key, it is a no-op. A
// legacy (no-slots) session is always a no-op. Otherwise — for an
// envelope vault — a brand-new namespace must be created by an admin:
// the admin generates the NDK, seals it to every admin slot, persists,
// and the new key is reflected into the live session so the subsequent
// write can use it. A non-admin session that lacks the namespace's key
// gets a clear error rather than the bare "not granted".
func ensureSessionNamespace(homeDir string, sess *Session, ns string) error {
	if sess.legacy() {
		return nil
	}
	if id := sess.current[ns]; id != "" {
		if _, ok := sess.ndks[ns+"#"+id]; ok {
			return nil
		}
	}
	if sess.Scope != ScopeAdmin {
		return fmt.Errorf("namespace %q is not writable by this password (no data key); an admin must create it first", nsLabel(ns))
	}
	var newNDKBytes []byte
	var newIDHex string
	if err := withVaultLock(homeDir, func() error {
		st, err := LoadStore(homeDir)
		if err != nil {
			return err
		}
		if id := st.CurrentNDK[ns]; id != "" {
			return fmt.Errorf("namespace %q was created concurrently; re-run the command", nsLabel(ns))
		}
		ndk, err := s7newNDK()
		if err != nil {
			return err
		}
		idb, err := s7newNDKID()
		if err != nil {
			return err
		}
		newIDHex = hex.EncodeToString(idb)
		if st.CurrentNDK == nil {
			st.CurrentNDK = map[string]string{}
		}
		st.CurrentNDK[ns] = newIDHex
		if err := sealNDKToAdmins(st, ns, newIDHex, ndk, sess.IntegrityKey()); err != nil {
			return err
		}
		newNDKBytes = ndk
		return SaveStore(homeDir, st)
	}); err != nil {
		return err
	}
	sess.current[ns] = newIDHex
	sess.ndks[ns+"#"+newIDHex] = newNDKBytes
	sess.byID[newIDHex] = newNDKBytes
	return nil
}

// writeSecret seals value for alias in namespace ns, first ensuring the
// session can write that namespace (auto-creating a brand-new one as
// admin). This is the single write entry point for the secret commands.
func writeSecret(homeDir string, sess *Session, alias, ns, value string) error {
	if err := ensureSessionNamespace(homeDir, sess, ns); err != nil {
		return err
	}
	return sess.setValue(alias, ns, value)
}

// valueNamespace returns the namespace a value is sealed under: always
// the name-derived namespace (root, "", for an unqualified name). This
// is the single invariant used by every seal/open site. The legacy
// Alias.Namespace metadata tag is cosmetic and must NOT be used for
// sealing — doing so makes a tag-only legacy alias seal under a
// different namespace than get/rotate read it under (AAD mismatch).
func valueNamespace(aliasName string) string {
	ns, _ := splitAlias(aliasName)
	return ns
}
