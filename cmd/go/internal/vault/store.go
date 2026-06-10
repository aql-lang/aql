package vault

import (
	"crypto/rand"
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
}

const storeVersion = 1

// StorePath returns the on-disk metadata path for the vault rooted
// at homeDir/.aql.
func StorePath(homeDir string) string {
	return filepath.Join(homeDir, ".aql", "vault.jsonic")
}

// LoadStore reads and parses the vault metadata file. Returns
// (nil, nil) — not an error — if the file does not exist; callers
// distinguish "not initialized" from "broken file" by inspecting
// the returned pointer.
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
	return s, nil
}

// SaveStore writes the metadata file with mode 0600. The parent
// directory ~/.aql is created with mode 0700 if absent.
func SaveStore(homeDir string, s *Store) error {
	if s.Version == 0 {
		s.Version = storeVersion
	}
	dir := filepath.Join(homeDir, ".aql")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	tmp := StorePath(homeDir) + ".tmp"
	if err := os.WriteFile(tmp, data, 0600); err != nil {
		return err
	}
	return os.Rename(tmp, StorePath(homeDir))
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
// provenance of an existing one.
func (s *Store) UpsertAlias(a Alias) {
	now := time.Now().UTC().Format(time.RFC3339)
	if existing, idx := s.FindAlias(a.Name); existing != nil {
		s.Aliases[idx].Provider = a.Provider
		s.Aliases[idx].Namespace = a.Namespace
		s.Aliases[idx].Source = a.Source
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

// SortedAliases returns aliases ordered by name for stable display.
func (s *Store) SortedAliases() []Alias {
	out := append([]Alias(nil), s.Aliases...)
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
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
// enforce quotas the same way.
func recordCapabilityUse(homeDir, capID string, costCents int) error {
	s, err := LoadStore(homeDir)
	if err != nil || s == nil {
		return err
	}
	_, idx := s.FindCapabilityExact(capID)
	if idx < 0 {
		return nil
	}
	s.Capabilities[idx].UsedCalls++
	s.Capabilities[idx].UsedCostCents += costCents
	return SaveStore(homeDir, s)
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
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// randomToken returns a 256-bit random bearer token as hex.
func randomToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
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
