// provider_cli.go implements the `vault provider` mode family: listing
// the provider presets (built-in and operator-defined) and minting or
// removing custom presets, so the credential broker can front any
// upstream API — not just the compiled-in three. `providers` (plural)
// stays the listing alias it has always been.

package vault

import (
	"flag"
	"fmt"
	"io"
	"strings"
)

// runProvider dispatches the provider sub-modes, mirroring the `folder`
// mode family: bare (or `list`) lists, `add` mints a custom preset, `rm`
// removes one.
func runProvider(args []string, homeDir string, stdout, stderr io.Writer) int {
	if len(args) > 0 {
		switch args[0] {
		case "add", "set":
			return runProviderAdd(args[1:], homeDir, stdout, stderr)
		case "rm", "remove", "delete":
			return runProviderRemove(args[1:], homeDir, stdout, stderr)
		case "list", "ls":
			return runProviderList(args[1:], homeDir, stdout, stderr)
		}
	}
	return runProviderList(args, homeDir, stdout, stderr)
}

// runProviderList prints every preset the broker would consult: the
// built-ins plus the vault's custom ones, with a SOURCE column so an
// operator can tell which are removable. An uninitialized vault lists
// just the built-ins — the listing must work before `vault init`, as the
// bare `providers` mode always has.
func runProviderList(args []string, homeDir string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("vault provider", flag.ContinueOnError)
	fs.SetOutput(stderr)
	if err := fs.Parse(args); err != nil {
		return 1
	}
	if fs.NArg() > 0 {
		fmt.Fprintf(stderr, "error: unknown provider subcommand %q (want add, rm, or list)\n", fs.Arg(0))
		return 1
	}
	s, err := LoadStore(homeDir)
	if err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "%-12s %-40s %-16s %s\n", "NAME", "BASE-URL", "AUTH-STYLE", "SOURCE")
	for _, p := range ListProvidersIn(s) {
		base := p.BaseURL
		if base == "" {
			base = "-"
		}
		source := "builtin"
		if !builtinProvider(p.Name) {
			source = "custom"
		}
		fmt.Fprintf(stdout, "%-12s %-40s %-16s %s\n", p.Name, base, p.AuthStyle, source)
	}
	return 0
}

// runProviderAdd implements `vault provider add <name> --url=... [--auth-style=...]`.
// Everything is validated at mint time — name shape, built-in collision,
// URL shape, auth style — so a broken preset can never reach the broker
// and fail one request at a time.
func runProviderAdd(args []string, homeDir string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("vault provider add", flag.ContinueOnError)
	fs.SetOutput(stderr)
	rawURL := fs.String("url", "", "upstream base URL (required; e.g. https://api.example.com or https://api.example.com/v2)")
	style := fs.String("auth-style", "bearer", "credential injection style: bearer, x-api-key, header:<name>, query:<name>, none")
	if err := fs.Parse(args); err != nil {
		return 1
	}
	if fs.NArg() != 1 || *rawURL == "" {
		fmt.Fprintf(stderr, "error: usage: aql vault provider add --url=BASE [--auth-style=STYLE] <name>\n")
		return 1
	}
	name := fs.Arg(0)
	if !validAliasSegment(name) {
		fmt.Fprintf(stderr, "error: invalid provider name %q (allowed: letters, digits, dot, dash, underscore)\n", name)
		return 1
	}
	if builtinProvider(name) {
		fmt.Fprintf(stderr, "error: %q is a built-in preset and cannot be redefined; pick another name\n", name)
		return 1
	}
	baseURL, err := validateProviderBaseURL(*rawURL)
	if err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}
	if err := validateAuthStyle(*style); err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}
	// No pre-flight store checks: mutateStore reloads under the vault
	// lock and reports "not initialized" itself, and the closure's lock
	// re-check is the single authoritative one — a duplicate check here
	// would be an unreachable statement.
	updated := false
	if err := mutateStore(homeDir, func(s *Store) error {
		if s.Locked {
			return errLocked
		}
		_, idx := s.FindCustomProvider(name)
		updated = idx >= 0
		s.UpsertCustomProvider(Provider{Name: name, BaseURL: baseURL, AuthStyle: *style})
		return nil
	}); err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}
	_ = appendAudit(homeDir, AuditEvent{
		Action: "vault.provider.add", Provider: name, Host: mustHost(baseURL), Outcome: "ok",
	})
	verb := "added"
	if updated {
		verb = "updated"
	}
	fmt.Fprintf(stdout, "%s provider %q -> %s (auth-style %s)\n", verb, name, baseURL, *style)
	if strings.HasPrefix(baseURL, "http://") {
		fmt.Fprintf(stderr, "warning: %s is plain http; the secret will travel unencrypted to this upstream\n", baseURL)
	}
	fmt.Fprintf(stdout, "tag secrets with it: aql vault add --provider=%s <alias>\n", name)
	return 0
}

// runProviderRemove implements `vault provider rm <name>`. Built-ins are
// untouchable, and a preset still referenced by aliases is refused with
// the blockers named — removing it would quietly break their brokering
// (the lookup would fall back to the URL-less generic preset).
func runProviderRemove(args []string, homeDir string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("vault provider rm", flag.ContinueOnError)
	fs.SetOutput(stderr)
	if err := fs.Parse(args); err != nil {
		return 1
	}
	if fs.NArg() != 1 {
		fmt.Fprintf(stderr, "error: usage: aql vault provider rm <name>\n")
		return 1
	}
	name := fs.Arg(0)
	if builtinProvider(name) {
		fmt.Fprintf(stderr, "error: %q is a built-in preset and cannot be removed\n", name)
		return 1
	}
	if err := mutateStore(homeDir, func(s *Store) error {
		if s.Locked {
			return errLocked
		}
		if users := s.ProvidersInUse(name); len(users) > 0 {
			return fmt.Errorf("provider %q is still used by %d alias(es): %s; retag or remove them first",
				name, len(users), strings.Join(users, ", "))
		}
		if !s.RemoveCustomProvider(name) {
			return fmt.Errorf("no custom provider named %q", name)
		}
		return nil
	}); err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}
	_ = appendAudit(homeDir, AuditEvent{
		Action: "vault.provider.rm", Provider: name, Outcome: "ok",
	})
	fmt.Fprintf(stdout, "removed provider %q\n", name)
	return 0
}
