package vault

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// runExec implements `aql vault exec`. It resolves the listed
// aliases against the keyring, injects their values into the
// environment of a child process, then execs the child with the
// caller's stdio attached. Secrets only ever appear in the child's
// environment block — never on the command line, never in the
// audit log.
//
// Usage:
//
//	aql vault exec [flags] <alias[=ENV][,alias[=ENV]]...> -- <cmd> [args...]
//
// By default an alias maps to an env var of the same name. Use
// `alias=ENV_NAME` to remap, `--upper` to uppercase the derived
// names, or `--prefix=PFX` to prepend a fixed prefix.
func runExec(args []string, homeDir string, stdin io.Reader, stdout, stderr io.Writer) int {
	// Split the args at the first `--` separator: everything before
	// is parsed by our flag set; everything after is the child
	// command and its arguments. We do this before flag.Parse so
	// flags like `--upper` inside the child command are left alone.
	preArgs, cmdArgs, sawSep := splitAtDoubleDash(args)

	fs := flag.NewFlagSet("vault exec", flag.ContinueOnError)
	fs.SetOutput(stderr)
	upper := fs.Bool("upper", false, "uppercase env-var names derived from alias names")
	clearEnv := fs.Bool("clear-env", false, "do not inherit the parent environment (keeps PATH/HOME/USER/SHELL/TERM/LANG/LC_ALL/TMPDIR only)")
	prefix := fs.String("prefix", "", "prepend this prefix to env-var names derived from aliases")
	forRecipe := fs.String("for", "", "present a single alias as a tool's credential env (npm, yarn, pnpm, bun, pypi, uv, poetry, hatch, flit, cargo, gem, hex, swift, cocoapods, composer, github, gitlab, terraform)")
	registry := fs.String("registry", "", "registry/host for registry-scoped recipes (npm, pnpm, composer, terraform); default per recipe")
	dryRun := fs.Bool("dry-run", false, "inject a filler value instead of the real secret (no passphrase needed); for testing the plumbing, e.g. `npm publish --dry-run`")
	if err := fs.Parse(preArgs); err != nil {
		return 1
	}

	if fs.NArg() < 1 {
		fmt.Fprintln(stderr, "error: usage: aql vault exec [--upper] [--prefix=PFX] [--clear-env] [--dry-run] [--for=TOOL [--registry=HOST]] <alias[=ENV][,alias[=ENV]]...> -- <cmd> [args...]")
		return 1
	}
	if !sawSep || len(cmdArgs) == 0 {
		fmt.Fprintln(stderr, "error: missing command (separate aliases from the command with `--`)")
		return 1
	}

	s, err := requireStore(homeDir)
	if err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}
	// In --dry-run we never unlock the vault: no passphrase is read and no
	// real secret value is touched, so a locked vault is fine and so is a
	// vault this password could not actually read. Each requested alias is
	// still resolved against the store (a typo is still an error) but gets a
	// fixed, obviously-fake filler value in place of the secret. This lets
	// callers exercise the env-injection and child-command plumbing —
	// `npm publish --dry-run` and friends — without real credentials.
	var sess *Session
	if !*dryRun {
		if s.Locked {
			fmt.Fprintln(stderr, "error: vault is locked; run `aql vault unlock`")
			return 1
		}
		sess, err = authenticate(s, homeDir, stdin, stdout, "Vault passphrase: ")
		if err != nil {
			fmt.Fprintf(stderr, "error: %s\n", err)
			return 1
		}
		defer sess.Close()
		if err := requireScope(sess, OpRead); err != nil {
			fmt.Fprintf(stderr, "error: %s\n", err)
			return 1
		}
	}

	bin, err := exec.LookPath(cmdArgs[0])
	if err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}

	overrides := map[string]string{}
	if *forRecipe != "" {
		// Publisher recipe: a single alias presented in the credential
		// environment the named tool reads (the recipe sets the env-var
		// names, so --prefix/--upper and =ENV/comma mappings don't apply).
		rec, ok := lookupPublishRecipe(*forRecipe)
		if !ok {
			fmt.Fprintf(stderr, "error: unknown --for recipe %q (known: %s)\n", *forRecipe, strings.Join(publishRecipeNames(), ", "))
			return 1
		}
		spec := fs.Arg(0)
		if strings.ContainsAny(spec, ",=") {
			fmt.Fprintln(stderr, "error: --for takes a single alias (the recipe sets the env-var names)")
			return 1
		}
		if *prefix != "" || *upper {
			fmt.Fprintln(stderr, "error: --for sets the env-var names; drop --prefix/--upper")
			return 1
		}
		_, alias, err := findAliasRef(s, spec)
		if err != nil {
			fmt.Fprintf(stderr, "error: %s\n", err)
			return 1
		}
		ns, _ := splitAlias(alias)
		v, err := resolveSecret(sess, *dryRun, alias, ns)
		if err != nil {
			_ = appendAudit(homeDir, AuditEvent{Action: "vault.exec", Alias: alias, Outcome: "error", Reason: "keyring: " + err.Error()})
			fmt.Fprintf(stderr, "error: reading %s: %s\n", alias, err)
			return 1
		}
		reg := *registry
		if reg == "" {
			reg = rec.defaultRegistry
		}
		for k, val := range rec.env(v, reg) {
			overrides[k] = val
		}
		_ = appendAudit(homeDir, AuditEvent{Action: "vault.exec", Alias: alias, Outcome: "ok", Reason: auditReason(*dryRun, "for="+rec.name+" cmd="+filepath.Base(bin))})
	} else {
		mappings, err := parseExecAliases(fs.Arg(0), *prefix, *upper)
		if err != nil {
			fmt.Fprintf(stderr, "error: %s\n", err)
			return 1
		}
		// Resolve every alias reference (default-namespace sugar, ':name'
		// root escape) to its stored name before any keyring access.
		for i := range mappings {
			_, name, err := findAliasRef(s, mappings[i].alias)
			if err != nil {
				fmt.Fprintf(stderr, "error: %s\n", err)
				return 1
			}
			mappings[i].alias = name
		}
		for _, m := range mappings {
			if _, dup := overrides[m.envName]; dup {
				fmt.Fprintf(stderr, "error: duplicate env name %q in mapping\n", m.envName)
				return 1
			}
			ns, _ := splitAlias(m.alias)
			v, err := resolveSecret(sess, *dryRun, m.alias, ns)
			if err != nil {
				_ = appendAudit(homeDir, AuditEvent{
					Action: "vault.exec", Alias: m.alias,
					Outcome: "error", Reason: "keyring: " + err.Error(),
				})
				fmt.Fprintf(stderr, "error: reading %s: %s\n", m.alias, err)
				return 1
			}
			overrides[m.envName] = v
		}
		for _, m := range mappings {
			_ = appendAudit(homeDir, AuditEvent{
				Action: "vault.exec", Alias: m.alias,
				Outcome: "ok",
				Reason:  auditReason(*dryRun, "env="+m.envName+" cmd="+filepath.Base(bin)),
			})
		}
	}

	env := buildExecEnv(*clearEnv, overrides)

	child := exec.Command(bin, cmdArgs[1:]...)
	child.Env = env
	child.Stdin = stdin
	child.Stdout = stdout
	child.Stderr = stderr
	if err := child.Run(); err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return ee.ExitCode()
		}
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}
	return 0
}

// dryRunFiller is the placeholder injected for every alias in --dry-run
// mode. It is deliberately not a real secret and is shaped to be obviously
// fake if it ever surfaces in a publisher's output or a captured log.
const dryRunFiller = "AQL-DRY-RUN-FILLER-NOT-A-REAL-SECRET"

// resolveSecret returns the value to inject for an alias: the fixed filler
// in --dry-run mode (no session, no keyring access), otherwise the real
// decrypted secret from the authenticated session.
func resolveSecret(sess *Session, dryRun bool, alias, ns string) (string, error) {
	if dryRun {
		return dryRunFiller, nil
	}
	return sess.getValue(alias, ns)
}

// auditReason tags an audit reason with a dry-run marker so the audit log
// distinguishes a real injection from a filler one.
func auditReason(dryRun bool, reason string) string {
	if dryRun {
		return "dry-run " + reason
	}
	return reason
}

// execMapping pairs a vault alias with the env-var name the
// resolved value should be exposed under in the child process.
type execMapping struct {
	alias   string
	envName string
}

// parseExecAliases parses the comma-separated <alias[=ENV]...>
// specifier into a list of mappings, applying --prefix and --upper
// to any alias that did not carry an explicit env name.
func parseExecAliases(spec, prefix string, upper bool) ([]execMapping, error) {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return nil, fmt.Errorf("no aliases specified")
	}
	var out []execMapping
	seen := make(map[string]bool)
	for _, part := range strings.Split(spec, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		alias := part
		envName := ""
		if eq := strings.IndexByte(part, '='); eq >= 0 {
			alias = strings.TrimSpace(part[:eq])
			envName = strings.TrimSpace(part[eq+1:])
		}
		if !validAliasRef(alias) {
			return nil, fmt.Errorf("invalid alias %q", alias)
		}
		if envName == "" {
			// Derive from the base name: a namespace qualifier contains
			// ':' which no env var name can carry, and `proj:key` and
			// `key` should both surface as $key by default.
			envName = aliasBase(strings.TrimPrefix(alias, rootNamespaceRef))
			if upper {
				envName = strings.ToUpper(envName)
			}
			envName = prefix + envName
		}
		if !validEnvName(envName) {
			return nil, fmt.Errorf("invalid env name %q (must match [A-Za-z_][A-Za-z0-9_]*)", envName)
		}
		if seen[envName] {
			return nil, fmt.Errorf("duplicate env name %q in mapping", envName)
		}
		seen[envName] = true
		out = append(out, execMapping{alias: alias, envName: envName})
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no aliases specified")
	}
	return out, nil
}

// validEnvName accepts the POSIX-portable env-var name shape
// `[A-Za-z_][A-Za-z0-9_]*`. Stricter than the kernel allows, but
// matches what shells will round-trip cleanly.
func validEnvName(s string) bool {
	if s == "" {
		return false
	}
	for i, r := range s {
		switch {
		case r == '_':
		case r >= 'A' && r <= 'Z':
		case r >= 'a' && r <= 'z':
		case i > 0 && r >= '0' && r <= '9':
		default:
			return false
		}
	}
	return true
}

// buildExecEnv assembles the child's environment block: either the
// caller's environment (default) or a sanitized minimal one, then
// overlays the vault-sourced overrides on top, dropping any
// pre-existing entries that would collide.
func buildExecEnv(clearEnv bool, overrides map[string]string) []string {
	var base []string
	if clearEnv {
		// A near-empty env breaks most child commands: programs need
		// PATH to find their helpers, HOME for config, TERM for
		// curses, etc. We keep the unambiguously-safe non-secret
		// ambient vars and drop everything else.
		for _, k := range []string{"PATH", "HOME", "USER", "LOGNAME", "SHELL", "TERM", "LANG", "LC_ALL", "TMPDIR"} {
			if v, ok := os.LookupEnv(k); ok {
				base = append(base, k+"="+v)
			}
		}
	} else {
		base = os.Environ()
	}
	out := make([]string, 0, len(base)+len(overrides))
	for _, e := range base {
		eq := strings.IndexByte(e, '=')
		if eq >= 0 {
			if _, drop := overrides[e[:eq]]; drop {
				continue
			}
		}
		out = append(out, e)
	}
	for k, v := range overrides {
		out = append(out, k+"="+v)
	}
	return out
}

// splitAtDoubleDash returns args before the first standalone `--`
// and args after it. The boolean reports whether the separator was
// present at all, which lets the caller distinguish "no separator"
// from "separator with no following args".
func splitAtDoubleDash(args []string) (pre, post []string, sawSep bool) {
	for i, a := range args {
		if a == "--" {
			return args[:i], args[i+1:], true
		}
	}
	return args, nil, false
}
