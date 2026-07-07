package vault

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Alias is one named secret entry. The secret value itself lives
// in the keyring (OS or file); this struct holds only the metadata
// needed to enumerate, scope, and audit usage.
type Alias struct {
	Name      string `json:"name"`
	Provider  string `json:"provider,omitempty"`
	Namespace string `json:"namespace,omitempty"`
	Source    string `json:"source,omitempty"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at,omitempty"`
	// ExpiresAt is an optional RFC3339 timestamp marking when the
	// underlying key expires upstream. It is informational — a reminder
	// surfaced by `vault list` and `vault expiry`, used to flag keys due
	// for rotation. It is never enforced: an expired alias still
	// resolves, since the secret may well outlive the recorded estimate.
	ExpiresAt string `json:"expires_at,omitempty"`
}

// Capability is a short-lived, scoped permission to use one alias.
// It maps to the "capability token" pattern from the recommendation
// and is enforced by the broker proxy at request time.
//
// Beyond the time- and host/method-scoping primitives, capabilities
// can carry quantitative constraints (MaxCalls, MaxCostCents) and a
// human-approval flag for risky operations. Used* fields are
// incremented on each successful proxy request and persisted so the
// quotas survive proxy restarts.
type Capability struct {
	ID string `json:"id"`
	// TokenHash is the hex SHA-256 of the bearer token a proxy client
	// presents. The token itself is shown once at grant time and never
	// stored, so a reader of vault.jsonic cannot use a capability — only
	// reference it by ID (to revoke or audit it).
	TokenHash       string   `json:"token_hash,omitempty"`
	Alias           string   `json:"alias"`
	Agent           string   `json:"agent,omitempty"`
	Hosts           []string `json:"hosts,omitempty"`
	Methods         []string `json:"methods,omitempty"`
	CreatedAt       string   `json:"created_at"`
	ExpiresAt       string   `json:"expires_at,omitempty"`
	Revoked         bool     `json:"revoked,omitempty"`
	MaxCalls        int      `json:"max_calls,omitempty"`
	UsedCalls       int      `json:"used_calls,omitempty"`
	MaxCostCents    int      `json:"max_cost_cents,omitempty"`
	UsedCostCents   int      `json:"used_cost_cents,omitempty"`
	RequireApproval bool     `json:"require_approval,omitempty"`
	// Namespace binds this capability to the namespace whose data key it
	// may reach, and SlotName records the password slot that granted it.
	// A broker authenticates a presented token into a session scoped to
	// Namespace (intersected with the broker password's grants), so a
	// token never reaches a namespace outside the scope it was minted for
	// even when the broker runs under a broader password.
	Namespace string `json:"namespace,omitempty"`
	SlotName  string `json:"slot_name,omitempty"`
}

// PasswordSlot is one named, scoped vault password (a "keyslot" in the
// LUKS sense). Each slot owns an X25519 keypair: EncPrivKey is the
// private key sealed under a per-slot KEK derived from the slot's
// passphrase, PublicKey is its public half, and WrappedKeys holds each
// granted namespace data key (NDK) sealed to that public key. Admin
// slots hold every NDK and can re-seal an NDK to any slot's public key.
//
// Scope is plaintext but cryptographically bound — it is mixed into the
// Verifier and the EncPrivKey AAD (see keyslot.go) — so editing it in
// vault.jsonic makes the slot fail authentication rather than escalate.
// Namespaces are NOT bound there: they are gated by NDK possession
// (editing the field grants nothing without the wrapped data key), which
// also lets an admin reassign namespaces without the holder's
// passphrase. PubMAC (keyed by the integrity key) authenticates
// PublicKey + Scope + Namespaces before any admin re-seal targets it.
//
// None of Salt/Verifier/PubMAC/EncPrivKey/WrappedKeys is a plaintext
// secret — they are derivation outputs and ciphertext, useless without
// the slot passphrase — but `password list` must never print them.
type PasswordSlot struct {
	Name        string            `json:"name"`                   // unique; validAliasSegment (no colon)
	Scope       string            `json:"scope"`                  // admin|read|write|move — bound into Verifier + EncPrivKey AAD
	Namespaces  []string          `json:"namespaces,omitempty"`   // stored-form ns; ""=root; nil/["*"]=all
	Salt        string            `json:"salt"`                   // base64, 16B — per-slot HKDF info (not a scrypt salt)
	Verifier    string            `json:"verifier"`               // hex — HMAC over label,Name,Scope,sorted(NS)
	PublicKey   string            `json:"public_key"`             // base64, X25519 public key
	PubMAC      string            `json:"pub_mac,omitempty"`      // hex — HMAC(integrityKey, label,Name,Pub,Scope,NS)
	EncPrivKey  string            `json:"enc_priv_key"`           // base64, AES-GCM(KEK) of X25519 private key
	WrappedKeys map[string]string `json:"wrapped_keys,omitempty"` // "ns#ndkID" -> base64 sealed NDK
	CreatedAt   string            `json:"created_at"`
	UpdatedAt   string            `json:"updated_at,omitempty"`
}

// Store is the on-disk vault metadata file. It contains aliases,
// capabilities, allowed agents, and configuration — never the
// raw secret values themselves.
type Store struct {
	Version      int            `json:"version"`
	Backend      string         `json:"backend"`
	Locked       bool           `json:"locked,omitempty"`
	Aliases      []Alias        `json:"aliases,omitempty"`
	Capabilities []Capability   `json:"capabilities,omitempty"`
	Agents       []string       `json:"agents,omitempty"`
	Config       map[string]any `json:"config,omitempty"`
	// VaultSalt is the single per-vault scrypt salt (base64, 16B). One
	// scrypt over VaultSalt yields the master KEK; each slot's KEK is an
	// HKDF expansion of it, so authenticate runs scrypt once regardless
	// of slot count (see keyslot.go).
	VaultSalt string `json:"vault_salt,omitempty"`
	// Passwords holds the scoped password slots. Empty == legacy
	// single-passphrase vault (implicit admin).
	Passwords []PasswordSlot `json:"passwords,omitempty"`
	// Generation is a monotonic content revision, bumped on every commit
	// (distinct from the schema Version). It anchors integrity detection,
	// the event-sourced journal, and restore.
	Generation int64 `json:"generation,omitempty"`
	// CurrentNDK maps each namespace (stored form; "" is root, plus the
	// reserved integrity namespace) to the hex id of the namespace data
	// key new writes seal under. A rekey transiently leaves an old and a
	// new id both decryptable, but only the current id seals new values.
	CurrentNDK map[string]string `json:"current_ndk,omitempty"`
}

// maxPasswordSlots bounds the slot table so authenticate's per-slot work
// and the integrity checks stay bounded even if the file is tampered to
// declare a huge number of slots. LoadStore refuses a store above it.
const maxPasswordSlots = 64

// storeVersion is the on-disk schema version this binary writes. Bump
// it in the same commit as any change to the Store/Capability/Alias
// shape that an older binary could mishandle, and add a migration to
// storeMigrations plus a golden-file test (see store_version_test.go).
//
//	v1 — initial schema.
//	v2 — capability bearer tokens are hashed (TokenHash). Capabilities
//	     minted under v1 carry no TokenHash and can no longer
//	     authenticate, so the v1->v2 migration revokes them explicitly
//	     instead of leaving them as silent 401s.
//	v3 — aliases gain the optional ExpiresAt field. The bump exists only
//	     so an older binary refuses a store that may carry alias expiries
//	     rather than silently dropping them on its next save.
//	v4 — scoped password slots, the envelope (VaultSalt/Passwords), a
//	     content Generation counter, and capability namespace-binding.
//	     The bump makes an older binary refuse a v4 store rather than
//	     silently dropping the password slots / key material on its next
//	     save (which would orphan every envelope-sealed secret).
const storeVersion = 4

// storeMigrations[i] upgrades a store from schema version i+1 to i+2.
// Each is a pure, in-place transform; the slice length must be
// storeVersion-1.
var storeMigrations = []func(*Store) error{
	migrateStoreV1ToV2, // index 0: v1 -> v2
	migrateStoreV2ToV3, // index 1: v2 -> v3
	migrateStoreV3ToV4, // index 2: v3 -> v4
}

// migrateStoreV1ToV2 revokes capabilities that predate token hashing.
// They have an empty TokenHash and therefore cannot be presented as a
// bearer token any more; revoking makes the security transition
// explicit (and visible in `vault status`) rather than a mysterious
// authentication failure.
func migrateStoreV1ToV2(s *Store) error {
	for i := range s.Capabilities {
		if s.Capabilities[i].TokenHash == "" && !s.Capabilities[i].Revoked {
			s.Capabilities[i].Revoked = true
		}
	}
	return nil
}

// migrateStoreV2ToV3 is a no-op. v3 only adds the optional
// Alias.ExpiresAt field, which is absent on every v2 alias and
// correctly reads as "no expiry"; there is nothing to transform. The
// version bump alone is the migration — it makes an older binary fail
// loud on a v3 store instead of stripping expiries it cannot model.
func migrateStoreV2ToV3(s *Store) error {
	return nil
}

// migrateStoreV3ToV4 is a no-op transform. A v3 store has no password
// slots, no VaultSalt, and no Generation, which correctly read as
// "legacy single-passphrase vault, implicit admin, untracked content".
// The version bump alone is the migration — it makes an older binary
// fail loud on a v4 store instead of silently stripping the password
// slots and key material it cannot model (which would orphan every
// envelope-sealed secret).
func migrateStoreV3ToV4(s *Store) error {
	return nil
}

// migrateStore brings s up to storeVersion in place, or returns an
// error if s was written by a newer binary than this one. A zero
// version is treated as v1 (the pre-versioning baseline). Migrations
// are applied in memory; they become durable on the next SaveStore.
func migrateStore(s *Store) error {
	v := s.Version
	if v == 0 {
		v = 1
	}
	if v > storeVersion {
		return fmt.Errorf("vault: store is version %d but this aql understands up to version %d; upgrade aql", v, storeVersion)
	}
	for v < storeVersion {
		if err := storeMigrations[v-1](s); err != nil {
			return fmt.Errorf("vault: migrating store v%d->v%d: %w", v, v+1, err)
		}
		v++
	}
	s.Version = v
	return nil
}

// StorePath returns the on-disk metadata path for the vault: the
// vault folder (homeDir/.aql by default, or AQL_VAULT_FOLDER) joined
// with the metadata file name (vault.jsonic, or vault.<suffix>.jsonic
// when AQL_VAULT_SUFFIX is set).
func StorePath(homeDir string) string {
	return filepath.Join(vaultFolder(homeDir), vaultFileName("jsonic"))
}

// LoadStore reads and parses the vault metadata file. Returns
// (nil, nil) — not an error — if the file does not exist; callers
// distinguish "not initialized" from "broken file" by inspecting
// the returned pointer.
//
// A store written by a newer aql is rejected rather than parsed
// leniently: Go's json drops unknown fields, so loading-then-saving a
// future-version file with this binary would silently strip data it
// does not understand. Older stores are migrated forward in memory.
func LoadStore(homeDir string) (*Store, error) {
	path := StorePath(homeDir)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	s := &Store{}
	if err := json.Unmarshal(data, s); err != nil {
		return nil, fmt.Errorf("vault: parsing %s: %w", path, err)
	}
	if err := migrateStore(s); err != nil {
		return nil, err
	}
	if len(s.Passwords) > maxPasswordSlots {
		return nil, fmt.Errorf("vault: store declares %d password slots, exceeding the maximum of %d", len(s.Passwords), maxPasswordSlots)
	}
	return s, nil
}

// SaveStore writes the metadata file atomically and durably with mode
// 0600. The vault folder is created with mode 0700 if absent.
//
// For a vault with scoped password slots it also maintains the
// content-versioning + integrity side artifacts: the Generation counter
// is bumped when (and only when) the content actually changed (so a
// no-op re-save stays byte-stable), a keyless signature sidecar
// (vault.jsonic.sig) is written, and on a content change one record is
// appended to the event-sourced journal (vault.jsonic.log). These are
// best-effort — failing to write them never fails the store save.
//
// A LEGACY single-passphrase vault (no slots) is written exactly as the
// original implementation did — no generation counter, sidecar, or
// journal — so existing vaults stay byte-identical and the broker's
// per-request quota-counter saves stay cheap. Feature B activates once
// the vault has password slots (the first `password add` migration).
func SaveStore(homeDir string, s *Store) error {
	if s.Version == 0 {
		s.Version = storeVersion
	}
	if err := os.MkdirAll(vaultFolder(homeDir), 0700); err != nil {
		return err
	}
	if !s.HasPasswordSlots() {
		data, err := json.MarshalIndent(s, "", "  ")
		if err != nil {
			return err
		}
		return writeFileAtomic(StorePath(homeDir), data, 0600)
	}
	contentH := storeContentHash(s)
	changed := true
	if prev, _ := readSidecar(homeDir); prev != nil && prev.ContentHash != "" && prev.ContentHash == contentH {
		changed = false
	}
	if changed {
		s.Generation++
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	if err := writeFileAtomic(StorePath(homeDir), data, 0600); err != nil {
		return err
	}
	_ = writeSidecar(homeDir, s.Generation, data, nil)
	if changed {
		_ = appendJournal(homeDir, nil, s.Generation, defaultActor(), "vault.save", storeSHA256(data), redactStore(s))
		_ = pruneJournal(homeDir, journalKeepFromConfig(s))
	}
	return nil
}

// FindAlias returns a pointer to the named alias and its index, or
// (nil, -1) if absent.
func (s *Store) FindAlias(name string) (*Alias, int) {
	for i := range s.Aliases {
		if s.Aliases[i].Name == name {
			return &s.Aliases[i], i
		}
	}
	return nil, -1
}

// UpsertAlias inserts a new alias or updates the timestamps and
// provenance of an existing one. A non-empty ExpiresAt updates the
// recorded expiry; an empty one preserves whatever expiry the existing
// alias already had, so a bare re-add or a bulk import never silently
// wipes a previously set expiry (use `vault expiry clear` to remove
// one).
func (s *Store) UpsertAlias(a Alias) {
	now := time.Now().UTC().Format(time.RFC3339)
	if existing, idx := s.FindAlias(a.Name); existing != nil {
		s.Aliases[idx].Provider = a.Provider
		s.Aliases[idx].Namespace = a.Namespace
		s.Aliases[idx].Source = a.Source
		if a.ExpiresAt != "" {
			s.Aliases[idx].ExpiresAt = a.ExpiresAt
		}
		s.Aliases[idx].UpdatedAt = now
		return
	}
	a.CreatedAt = now
	s.Aliases = append(s.Aliases, a)
}

// RemoveAlias drops the named alias and any capabilities scoped to
// it. Returns true if the alias existed.
func (s *Store) RemoveAlias(name string) bool {
	_, idx := s.FindAlias(name)
	if idx < 0 {
		return false
	}
	s.Aliases = append(s.Aliases[:idx], s.Aliases[idx+1:]...)
	kept := s.Capabilities[:0]
	for _, c := range s.Capabilities {
		if c.Alias != name {
			kept = append(kept, c)
		}
	}
	s.Capabilities = kept
	return true
}

// RenameAlias renames alias from to to in place, preserving
// provenance (Provider, Source, CreatedAt) and re-deriving the
// namespace metadata from the new name. Capabilities bound to from
// follow the rename, or are revoked instead when revokeCaps is set.
// Returns the number of capabilities touched and whether from
// existed. The caller pre-flights destination collisions; from == to
// is a pure metadata refresh (used when a namespace move only
// re-tags a legacy alias).
func (s *Store) RenameAlias(from, to string, revokeCaps bool) (int, bool) {
	_, idx := s.FindAlias(from)
	if idx < 0 {
		return 0, false
	}
	ns, _ := splitAlias(to)
	s.Aliases[idx].Name = to
	s.Aliases[idx].Namespace = ns
	s.Aliases[idx].UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	touched := 0
	for i := range s.Capabilities {
		if s.Capabilities[i].Alias != from {
			continue
		}
		if revokeCaps {
			if !s.Capabilities[i].Revoked {
				s.Capabilities[i].Revoked = true
				touched++
			}
			continue
		}
		s.Capabilities[i].Alias = to
		touched++
	}
	return touched, true
}

// SortedAliases returns aliases ordered by name for stable display.
func (s *Store) SortedAliases() []Alias {
	out := append([]Alias(nil), s.Aliases...)
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// HasPasswordSlots reports whether the vault has any scoped password
// slots. No slots == a legacy single-passphrase vault (implicit admin).
func (s *Store) HasPasswordSlots() bool { return len(s.Passwords) > 0 }

// FindPasswordSlot returns a pointer to the named slot and its index, or
// (nil, -1) if absent.
func (s *Store) FindPasswordSlot(name string) (*PasswordSlot, int) {
	for i := range s.Passwords {
		if s.Passwords[i].Name == name {
			return &s.Passwords[i], i
		}
	}
	return nil, -1
}

// UpsertPasswordSlot inserts a new slot or replaces an existing one by
// name, preserving CreatedAt and stamping UpdatedAt on replace.
func (s *Store) UpsertPasswordSlot(slot PasswordSlot) {
	now := time.Now().UTC().Format(time.RFC3339)
	if existing, idx := s.FindPasswordSlot(slot.Name); existing != nil {
		slot.CreatedAt = existing.CreatedAt
		slot.UpdatedAt = now
		s.Passwords[idx] = slot
		return
	}
	if slot.CreatedAt == "" {
		slot.CreatedAt = now
	}
	s.Passwords = append(s.Passwords, slot)
}

// RemovePasswordSlot drops the named slot. Returns true if it existed.
func (s *Store) RemovePasswordSlot(name string) bool {
	_, idx := s.FindPasswordSlot(name)
	if idx < 0 {
		return false
	}
	s.Passwords = append(s.Passwords[:idx], s.Passwords[idx+1:]...)
	return true
}

// SortedPasswordSlots returns slots ordered by name for stable display.
func (s *Store) SortedPasswordSlots() []PasswordSlot {
	out := append([]PasswordSlot(nil), s.Passwords...)
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// OtherFunctionalAdminExists reports whether an admin-scoped slot other
// than excludingName exists. Identity-aware (not a bare count) so the
// last-admin guard can't be tricked by an injected confusable admin slot
// into allowing the real admin to be removed.
func (s *Store) OtherFunctionalAdminExists(excludingName string) bool {
	for i := range s.Passwords {
		if s.Passwords[i].Name == excludingName {
			continue
		}
		if s.Passwords[i].Scope == ScopeAdmin {
			return true
		}
	}
	return false
}

// FindCapability returns the capability matching id (or its short
// form, the first 8 hex chars) and its index, or (nil, -1).
//
// This prefix match is a CLI convenience for commands like `vault
// revoke <short-id>`. It must NOT be used to authenticate a bearer
// token — see FindCapabilityExact and the warning there.
func (s *Store) FindCapability(id string) (*Capability, int) {
	for i := range s.Capabilities {
		if s.Capabilities[i].ID == id || strings.HasPrefix(s.Capabilities[i].ID, id) {
			return &s.Capabilities[i], i
		}
	}
	return nil, -1
}

// FindCapabilityExact returns the capability whose public ID exactly
// equals id, compared in constant time, or (nil, -1). This is internal
// bookkeeping (e.g. debiting quota counters by the ID the proxy already
// authenticated). It does NOT authenticate a bearer token — that is
// FindCapabilityByToken, which matches the token hash.
func (s *Store) FindCapabilityExact(id string) (*Capability, int) {
	idb := []byte(id)
	for i := range s.Capabilities {
		if subtle.ConstantTimeCompare([]byte(s.Capabilities[i].ID), idb) == 1 {
			return &s.Capabilities[i], i
		}
	}
	return nil, -1
}

// FindCapabilityByToken authenticates a presented bearer token: it
// hashes the token and constant-time compares against each stored
// TokenHash, returning the matching capability or (nil, -1).
//
// Matching the hash (not the token, which is never stored) means a
// reader of vault.jsonic cannot use a capability, and the constant-time
// compare avoids leaking via timing how much of a guessed token was
// correct. There is no prefix match: only the full token authenticates.
func (s *Store) FindCapabilityByToken(token string) (*Capability, int) {
	if token == "" {
		return nil, -1
	}
	want := []byte(hashToken(token))
	for i := range s.Capabilities {
		if s.Capabilities[i].TokenHash == "" {
			continue
		}
		if subtle.ConstantTimeCompare([]byte(s.Capabilities[i].TokenHash), want) == 1 {
			return &s.Capabilities[i], i
		}
	}
	return nil, -1
}

// FindActiveCapability returns the first capability granted to agent for
// alias that is neither revoked nor expired at now, or (nil, -1). The
// MCP server resolves capabilities this way (by identity) since, unlike
// the proxy, its caller presents no bearer token.
func (s *Store) FindActiveCapability(alias, agent string, now time.Time) (*Capability, int) {
	for i := range s.Capabilities {
		c := &s.Capabilities[i]
		if c.Alias == alias && c.Agent == agent && !c.Revoked && capabilityActive(c, now) {
			return c, i
		}
	}
	return nil, -1
}

// NewCapability appends a fresh capability record and returns it along
// with its one-time bearer token. Only the token's hash is persisted;
// the plaintext token is returned to be shown to the operator once and
// never again. The caller is responsible for persisting via SaveStore.
func (s *Store) NewCapability(alias, agent string, hosts, methods []string, ttl time.Duration) (*Capability, string, error) {
	id, err := randomID()
	if err != nil {
		return nil, "", err
	}
	token, err := randomToken()
	if err != nil {
		return nil, "", err
	}
	c := Capability{
		ID:        id,
		TokenHash: hashToken(token),
		Alias:     alias,
		Agent:     agent,
		Hosts:     hosts,
		Methods:   methods,
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	}
	if ttl > 0 {
		c.ExpiresAt = time.Now().UTC().Add(ttl).Format(time.RFC3339)
	}
	s.Capabilities = append(s.Capabilities, c)
	return &s.Capabilities[len(s.Capabilities)-1], token, nil
}

// recordCapabilityUse debits the call counter and cost meter on the
// capability with the given public ID and persists the store. A missing
// capability is a no-op. Shared by the proxy and the MCP server so both
// enforce quotas the same way. The mutateStore lock serializes it
// against every other store writer — its own request goroutines, and
// CLI commands in other processes — so no increment is lost.
func recordCapabilityUse(homeDir, capID string, costCents int) error {
	return mutateStore(homeDir, func(s *Store) error {
		_, idx := s.FindCapabilityExact(capID)
		if idx < 0 {
			return nil
		}
		s.Capabilities[idx].UsedCalls++
		s.Capabilities[idx].UsedCostCents += costCents
		return nil
	})
}

// ActiveCapabilities returns capabilities that are neither revoked
// nor past their ExpiresAt timestamp (computed against now).
func (s *Store) ActiveCapabilities(now time.Time) []Capability {
	var out []Capability
	for _, c := range s.Capabilities {
		if c.Revoked {
			continue
		}
		if c.ExpiresAt != "" {
			t, err := time.Parse(time.RFC3339, c.ExpiresAt)
			if err == nil && now.After(t) {
				continue
			}
		}
		out = append(out, c)
	}
	return out
}

func randomID() (string, error) {
	b := make([]byte, 16)
	if _, err := s7randRead(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// randomToken returns a 256-bit random bearer token as hex.
func randomToken() (string, error) {
	b := make([]byte, 32)
	if _, err := s7randRead(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// hashToken returns the hex SHA-256 of a bearer token, the form stored
// in and compared against TokenHash.
func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
