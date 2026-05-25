package vault

import (
	"crypto/rand"
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
	ID              string   `json:"id"`
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
func (s *Store) FindCapability(id string) (*Capability, int) {
	for i := range s.Capabilities {
		if s.Capabilities[i].ID == id || strings.HasPrefix(s.Capabilities[i].ID, id) {
			return &s.Capabilities[i], i
		}
	}
	return nil, -1
}

// NewCapability appends a fresh capability record. The caller is
// responsible for persisting via SaveStore.
func (s *Store) NewCapability(alias, agent string, hosts, methods []string, ttl time.Duration) (*Capability, error) {
	id, err := randomID()
	if err != nil {
		return nil, err
	}
	c := Capability{
		ID:        id,
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
	return &s.Capabilities[len(s.Capabilities)-1], nil
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
