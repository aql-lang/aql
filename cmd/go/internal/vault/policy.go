package vault

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"time"
)

// Policy is the declarative shape of a vault policy file. A team
// commits one of these to a repo so vault state is auditable in
// version control and reproducible across machines.
//
// Aliases declared in a policy are upserted (created or refreshed
// with the new metadata; secret values are NOT carried in the
// policy and must already be present in the keyring, or supplied
// via the FromEnv hint). Capabilities declared in a policy are
// resolved by Agent identity — for each (Alias, Agent) pair we
// keep at most one active capability and refresh it from policy.
type Policy struct {
	Version      int                `json:"version"`
	Aliases      []PolicyAlias      `json:"aliases,omitempty"`
	Capabilities []PolicyCapability `json:"capabilities,omitempty"`
}

// PolicyAlias declares one alias and where its value should come
// from if it is not already in the keyring. FromEnv reads from
// an environment variable at apply time, leaving the value out of
// the file itself.
type PolicyAlias struct {
	Name      string `json:"name"`
	Provider  string `json:"provider,omitempty"`
	Namespace string `json:"namespace,omitempty"`
	FromEnv   string `json:"from_env,omitempty"`
}

// PolicyCapability declares one capability. The Agent field acts
// as the identity that policy apply will rewrite — replaying the
// same policy is idempotent for the (alias, agent) tuple.
type PolicyCapability struct {
	Alias           string   `json:"alias"`
	Agent           string   `json:"agent"`
	Hosts           []string `json:"hosts,omitempty"`
	Methods         []string `json:"methods,omitempty"`
	TTL             string   `json:"ttl,omitempty"`
	MaxCalls        int      `json:"max_calls,omitempty"`
	MaxCostCents    int      `json:"max_cost_cents,omitempty"`
	RequireApproval bool     `json:"require_approval,omitempty"`
}

// runPolicy implements `aql vault policy <apply|show> [args...]`.
// It is the only mode with a nested verb because the policy
// surface needs both a writer (apply) and a reader (show) and
// keeping them under one parent keeps the top-level mode list
// short.
func runPolicy(args []string, homeDir string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "error: usage: aql vault policy <apply|show> [args...]")
		return 1
	}
	sub, rest := args[0], args[1:]
	switch sub {
	case "apply":
		return runPolicyApply(rest, homeDir, stdin, stdout, stderr)
	case "show":
		return runPolicyShow(homeDir, stdout, stderr)
	default:
		fmt.Fprintf(stderr, "error: unknown policy verb %q (expected apply or show)\n", sub)
		return 1
	}
}

func runPolicyApply(args []string, homeDir string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("vault policy apply", flag.ContinueOnError)
	fs.SetOutput(stderr)
	dryRun := fs.Bool("dry-run", false, "report changes without writing them")
	if err := fs.Parse(args); err != nil {
		return 1
	}
	if fs.NArg() < 1 {
		fmt.Fprintln(stderr, "error: usage: aql vault policy apply [--dry-run] <policy.json>")
		return 1
	}
	path := fs.Arg(0)
	data, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}
	var pol Policy
	if err := json.Unmarshal(data, &pol); err != nil {
		fmt.Fprintf(stderr, "error: parsing %s: %s\n", path, err)
		return 1
	}

	s, err := requireStore(homeDir)
	if err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}
	if s.Locked {
		fmt.Fprintln(stderr, "error: vault is locked; run `aql vault unlock`")
		return 1
	}

	kr, err := openKeyring(s, homeDir, stdin, stdout, "Vault passphrase: ")
	if err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}

	var changes []string
	for _, a := range pol.Aliases {
		if !validAlias(a.Name) {
			fmt.Fprintf(stderr, "error: invalid alias %q\n", a.Name)
			return 1
		}
		existing, _ := s.FindAlias(a.Name)
		if existing == nil {
			changes = append(changes, "+alias "+a.Name)
		} else if existing.Provider != a.Provider || existing.Namespace != a.Namespace {
			changes = append(changes, "~alias "+a.Name)
		}
		// Provision value from env if requested and not already in
		// the keyring. The store metadata is upserted unconditionally
		// so apply is idempotent.
		if a.FromEnv != "" {
			if _, err := kr.Get(a.Name); err == ErrNotFound {
				val := os.Getenv(a.FromEnv)
				if val == "" {
					fmt.Fprintf(stderr, "error: alias %q FromEnv=%s is empty\n", a.Name, a.FromEnv)
					return 1
				}
				if !*dryRun {
					if err := kr.Set(a.Name, val); err != nil {
						fmt.Fprintf(stderr, "error: storing %s: %s\n", a.Name, err)
						return 1
					}
				}
				changes = append(changes, "+secret "+a.Name+" (from $"+a.FromEnv+")")
			}
		}
		if !*dryRun {
			s.UpsertAlias(Alias{
				Name:      a.Name,
				Provider:  a.Provider,
				Namespace: a.Namespace,
				Source:    "policy:" + path,
			})
		}
	}

	for _, c := range pol.Capabilities {
		if a, _ := s.FindAlias(c.Alias); a == nil {
			fmt.Fprintf(stderr, "error: policy capability references unknown alias %q\n", c.Alias)
			return 1
		}
		ttl, err := parsePolicyTTL(c.TTL)
		if err != nil {
			fmt.Fprintf(stderr, "error: capability for %s: %s\n", c.Alias, err)
			return 1
		}
		// Revoke any prior live capabilities for the same
		// (alias, agent) tuple so reapplying the policy does not
		// pile up stale capabilities.
		revokedAny := false
		for i := range s.Capabilities {
			cur := &s.Capabilities[i]
			if cur.Alias != c.Alias || cur.Agent != c.Agent || cur.Revoked {
				continue
			}
			cur.Revoked = true
			revokedAny = true
		}
		if revokedAny {
			changes = append(changes, "~capability "+c.Alias+"@"+c.Agent)
		} else {
			changes = append(changes, "+capability "+c.Alias+"@"+c.Agent)
		}
		if !*dryRun {
			tok, err := s.NewCapability(c.Alias, c.Agent, c.Hosts, c.Methods, ttl)
			if err != nil {
				fmt.Fprintf(stderr, "error: granting capability for %s: %s\n", c.Alias, err)
				return 1
			}
			idx := len(s.Capabilities) - 1
			s.Capabilities[idx].MaxCalls = c.MaxCalls
			s.Capabilities[idx].MaxCostCents = c.MaxCostCents
			s.Capabilities[idx].RequireApproval = c.RequireApproval
			_ = appendAudit(homeDir, AuditEvent{
				Action: "vault.policy.apply", Alias: c.Alias, Agent: c.Agent,
				Capability: tok.ID, Outcome: "ok",
			})
		}
	}

	if !*dryRun {
		if err := SaveStore(homeDir, s); err != nil {
			fmt.Fprintf(stderr, "error: %s\n", err)
			return 1
		}
	}

	if len(changes) == 0 {
		fmt.Fprintln(stdout, "policy: no changes")
		return 0
	}
	sort.Strings(changes)
	for _, c := range changes {
		fmt.Fprintln(stdout, c)
	}
	if *dryRun {
		fmt.Fprintln(stdout, "(dry-run; nothing written)")
	}
	return 0
}

func runPolicyShow(homeDir string, stdout, stderr io.Writer) int {
	s, err := requireStore(homeDir)
	if err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}
	pol := Policy{Version: 1}
	for _, a := range s.SortedAliases() {
		pol.Aliases = append(pol.Aliases, PolicyAlias{
			Name:      a.Name,
			Provider:  a.Provider,
			Namespace: a.Namespace,
		})
	}
	for _, c := range s.ActiveCapabilities(time.Now()) {
		ttl := ""
		if c.ExpiresAt != "" {
			if exp, err := time.Parse(time.RFC3339, c.ExpiresAt); err == nil {
				ttl = time.Until(exp).Round(time.Second).String()
			}
		}
		pol.Capabilities = append(pol.Capabilities, PolicyCapability{
			Alias:           c.Alias,
			Agent:           c.Agent,
			Hosts:           c.Hosts,
			Methods:         c.Methods,
			TTL:             ttl,
			MaxCalls:        c.MaxCalls,
			MaxCostCents:    c.MaxCostCents,
			RequireApproval: c.RequireApproval,
		})
	}
	b, _ := json.MarshalIndent(pol, "", "  ")
	fmt.Fprintln(stdout, string(b))
	return 0
}

// parsePolicyTTL accepts a Go duration string (e.g. "2h", "30m")
// and returns a positive duration. An empty TTL yields the default
// 2-hour grant, matching `vault grant` without --ttl.
func parsePolicyTTL(s string) (time.Duration, error) {
	if s == "" {
		return 2 * time.Hour, nil
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return 0, fmt.Errorf("invalid ttl %q: %w", s, err)
	}
	if d <= 0 {
		return 0, fmt.Errorf("ttl must be positive, got %q", s)
	}
	return d, nil
}
