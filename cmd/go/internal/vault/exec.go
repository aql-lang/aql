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

	"github.com/aql-lang/aql/cmd/go/internal/auth"
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
//
// `--for=RECIPE` instead presents an alias in a publishing tool's own
// credential env (npm, cargo, github, …). It is repeatable, and each
// entry may name its own secret as `RECIPE=alias`, so one child can
// carry several tools' credentials — e.g. publish to npm and push a
// git tag to GitHub from the same `make publish`:
//
//	aql vault exec --for=npm=npm --for=github=vxg:github -- make publish
//
// `--ask=ENV_NAME` prompts on the terminal (echo suppressed) and injects
// the typed value as ENV_NAME in the child — for a secret that is not in
// the vault. It needs no vault at all, and the value never touches disk,
// argv, or the audit log. `--ask-passphrase` prompts once for the vault
// passphrase, validates it against the store, and injects it as
// AQL_VAULT_PASSPHRASE so nested `aql vault` calls in the child run
// without re-prompting — one prompt for a whole multi-target deploy:
//
//	aql vault exec --ask GITHUB_TOKEN -- make tag-push-ts
//	aql vault exec --ask-passphrase -- make deploy-ts deploy-py deploy-go
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
	var forRecipes repeatedFlag
	fs.Var(&forRecipes, "for", "present an alias as a tool's credential env: `recipe` (alias from the positional arg) or `recipe=alias`; repeatable to target several tools at once (npm, yarn, pnpm, bun, pypi, uv, poetry, hatch, flit, cargo, gem, hex, swift, cocoapods, composer, github, gitlab, terraform)")
	registry := fs.String("registry", "", "registry/host for registry-scoped recipes (npm, pnpm, composer, terraform); default per recipe")
	dryRun := fs.Bool("dry-run", false, "inject a filler value instead of the real secret (no passphrase needed); for testing the plumbing, e.g. `npm publish --dry-run`")
	var askVars repeatedFlag
	fs.Var(&askVars, "ask", "prompt for a value on the terminal (echo suppressed) and inject it as this env var: `ENV_NAME`; repeatable. For secrets that are not in the vault — no vault needed")
	askPass := fs.Bool("ask-passphrase", false, "prompt once for the vault passphrase, validate it, and inject it as AQL_VAULT_PASSPHRASE so nested `aql vault` calls in the child run without re-prompting")
	if err := fs.Parse(preArgs); err != nil {
		return 1
	}

	// In --for mode the secrets can be named inside the flags
	// (`--for=recipe=alias`), so a positional alias spec is optional —
	// as it is when --ask/--ask-passphrase provide the credentials.
	forMode := len(forRecipes) > 0
	askMode := len(askVars) > 0 || *askPass
	if fs.NArg() < 1 && !forMode && !askMode {
		fmt.Fprintln(stderr, "error: usage: aql vault exec [--upper] [--prefix=PFX] [--clear-env] [--dry-run] [--for=RECIPE[=alias] ...] [--registry=HOST] [--ask=ENV_NAME ...] [--ask-passphrase] <alias[=ENV][,alias[=ENV]]...> -- <cmd> [args...]")
		return 1
	}
	if !sawSep || len(cmdArgs) == 0 {
		fmt.Fprintln(stderr, "error: missing command (separate aliases from the command with `--`)")
		return 1
	}

	// One reader backs every interactive prompt in this exec — the vault
	// passphrase (--ask-passphrase) and each --ask value. Sharing it is not
	// cosmetic: InputReader buffers, so a throwaway reader per prompt can
	// read past its own newline and strand the next prompt's input at EOF.
	// It also decides *where* to prompt (terminal vs the child's stdin);
	// see promptSource.
	prompts := newPromptSource(stdin)
	defer prompts.close()

	// The vault store is only opened when something actually needs it:
	// alias resolution, or --ask-passphrase validation. A pure `--ask`
	// invocation works with no vault at all.
	needAliases := forMode || fs.NArg() >= 1
	var s *Store
	if needAliases || (*askPass && !*dryRun) {
		st, err := requireStore(homeDir)
		if err != nil {
			fmt.Fprintf(stderr, "error: %s\n", err)
			return 1
		}
		s = st
	}
	// In --dry-run we never unlock the vault: no passphrase is read and no
	// real secret value is touched, so a locked vault is fine and so is a
	// vault this password could not actually read. Each requested alias is
	// still resolved against the store (a typo is still an error) but gets a
	// fixed, obviously-fake filler value in place of the secret. This lets
	// callers exercise the env-injection and child-command plumbing —
	// `npm publish --dry-run` and friends — without real credentials.
	var sess *Session
	var vaultPass string
	if !*dryRun && s != nil {
		if s.Locked {
			fmt.Fprintln(stderr, "error: vault is locked; run `aql vault unlock`")
			return 1
		}
		var err error
		if *askPass {
			// Source the passphrase (env or prompt) and validate it up
			// front — a typo fails here, not deep inside the child.
			// Prompt on stderr: stdout belongs to the child (it may be
			// piped), and interactive prompts conventionally go to stderr.
			vaultPass, err = readPassphraseVia(prompts, stderr, "Vault passphrase: ")
			if err != nil {
				fmt.Fprintf(stderr, "error: %s\n", err)
				return 1
			}
			sess, err = authenticateWith(s, homeDir, vaultPass)
			// An envelope vault validates the passphrase cryptographically
			// against its slot verifiers inside authenticateWith. A legacy
			// slotless file keyring does not — it only proves the
			// passphrase on the first decrypt — so probe one stored secret
			// now: a typo must fail at the prompt, not deep inside the
			// child's nested vault calls. (The probe must not run for
			// slotted vaults: a namespace-scoped password may legitimately
			// be unable to read the probe alias.) An empty vault has
			// nothing to check against (and nothing to leak).
			if err == nil && !s.HasPasswordSlots() && len(s.Aliases) > 0 {
				probe := s.Aliases[0].Name
				ns, _ := splitAlias(probe)
				if _, perr := sess.getValue(probe, ns); perr != nil {
					fmt.Fprintf(stderr, "error: vault passphrase validation failed: %s\n", perr)
					sess.Close()
					return 1
				}
			}
		} else {
			sess, err = authenticate(s, homeDir, stdin, stdout, "Vault passphrase: ")
		}
		if err != nil {
			fmt.Fprintf(stderr, "error: %s\n", err)
			return 1
		}
		defer sess.Close()
		if err := w8requireScope(sess, OpRead); err != nil {
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
	if forMode {
		// Publisher recipes: each binds an alias to the credential
		// environment a tool reads (the recipe sets the env-var names, so
		// --prefix/--upper and =ENV/comma mappings don't apply). Repeating
		// --for targets several tools in one process, each from its own
		// secret — e.g. publish to npm and push a git tag to GitHub from a
		// single `make publish`.
		if *prefix != "" || *upper {
			fmt.Fprintln(stderr, "error: --for sets the env-var names; drop --prefix/--upper")
			return 1
		}
		positional := ""
		if fs.NArg() >= 1 {
			positional = fs.Arg(0)
		}
		bindings, err := parseForRecipes(forRecipes, positional)
		if err != nil {
			fmt.Fprintf(stderr, "error: %s\n", err)
			return 1
		}
		setBy := map[string]string{} // env var -> recipe that set it (for clear collision errors)
		for _, b := range bindings {
			_, alias, err := findAliasRef(s, b.alias)
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
				reg = b.rec.defaultRegistry
			}
			for k, val := range b.rec.env(v, reg) {
				if prev, dup := overrides[k]; dup && prev != val {
					fmt.Fprintf(stderr, "error: --for=%s and --for=%s both set $%s to different values; run them as separate exec calls\n", setBy[k], b.rec.name, k)
					return 1
				}
				overrides[k] = val
				setBy[k] = b.rec.name
			}
			_ = appendAudit(homeDir, AuditEvent{Action: "vault.exec", Alias: alias, Outcome: "ok", Reason: auditReason(*dryRun, "for="+b.rec.name+" cmd="+filepath.Base(bin))})
		}
	} else if fs.NArg() >= 1 {
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
		// No duplicate-env-name re-check here: parseExecAliases already
		// rejects duplicates within the mapping, and overrides is empty in
		// this branch (forMode is false), so a collision is unconstructible
		// (see design/TEST-SEAMS.10.md on unreachable guards).
		for _, m := range mappings {
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

	// --ask: values typed by the operator at exec time. Prompted with echo
	// suppressed, injected only into the child's environment block, and
	// audited by env NAME only — the value never touches disk, argv, or
	// the audit log. --dry-run injects the filler without prompting.
	if len(askVars) > 0 {
		for _, name := range askVars {
			if !validEnvName(name) {
				fmt.Fprintf(stderr, "error: --ask %q: not a valid environment variable name\n", name)
				return 1
			}
			if _, dup := overrides[name]; dup {
				fmt.Fprintf(stderr, "error: --ask %s collides with another injected env var\n", name)
				return 1
			}
			v := dryRunFiller
			if !*dryRun {
				ir, err := prompts.reader()
				if err != nil {
					fmt.Fprintf(stderr, "error: %s\n", err)
					return 1
				}
				read, err := ir.ReadPassword(name+" (input not echoed): ", stderr)
				if err != nil {
					fmt.Fprintf(stderr, "error: reading %s: %s\n", name, err)
					return 1
				}
				if read == "" {
					fmt.Fprintf(stderr, "error: empty value for --ask %s\n", name)
					return 1
				}
				v = read
			}
			overrides[name] = v
			_ = appendAudit(homeDir, AuditEvent{Action: "vault.exec", Alias: "(ask)", Outcome: "ok", Reason: auditReason(*dryRun, "ask env="+name+" cmd="+filepath.Base(bin))})
		}
	}

	// --ask-passphrase: hand the passphrase (already validated above) to
	// the child so nested `aql vault` calls authenticate without prompting.
	if *askPass {
		if _, dup := overrides[EnvPassphrase]; dup {
			fmt.Fprintf(stderr, "error: --ask-passphrase collides with an injected $%s\n", EnvPassphrase)
			return 1
		}
		v := dryRunFiller
		if !*dryRun {
			v = vaultPass
		}
		overrides[EnvPassphrase] = v
		_ = appendAudit(homeDir, AuditEvent{Action: "vault.exec", Alias: "(passphrase)", Outcome: "ok", Reason: auditReason(*dryRun, "ask-passphrase cmd="+filepath.Base(bin))})
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

// promptSource lazily provides the single InputReader shared by every
// interactive prompt in one exec — the vault passphrase and each --ask
// value — so no prompt's buffered read-ahead can strand the next prompt's
// input at EOF.
//
// It also decides *where* to prompt. The child inherits our stdin
// (child.Stdin == stdin), so when that stdin is a redirected file or pipe,
// reading a prompt from it would steal a line meant for the child — e.g.
// `aql vault exec --ask TOKEN -- cat < payload` would swallow payload's
// first line. In that case we prompt on the controlling terminal instead,
// and fail if there is none. A terminal stdin, or any non-file reader
// (tests, programmatic callers), is prompted on directly.
type promptSource struct {
	stdin  io.Reader
	ir     *auth.InputReader
	closer func()
	err    error
	opened bool
}

func newPromptSource(stdin io.Reader) *promptSource { return &promptSource{stdin: stdin} }

// reader returns the shared InputReader, opening it (and, if needed, the
// controlling terminal) on first use so a prompt-free run touches no tty.
func (p *promptSource) reader() (*auth.InputReader, error) {
	if !p.opened {
		p.opened = true
		p.ir, p.closer, p.err = openPromptReader(p.stdin)
	}
	return p.ir, p.err
}

func (p *promptSource) close() {
	if p.closer != nil {
		p.closer()
	}
}

// openPromptReader picks the prompt input: the controlling terminal when
// stdin is a redirected file/pipe the child will consume, otherwise stdin
// itself. The returned closer releases the terminal handle if one was opened.
func openPromptReader(stdin io.Reader) (*auth.InputReader, func(), error) {
	if f, ok := stdin.(*os.File); ok && !isTerminal(int(f.Fd())) {
		// Redirected stdin belongs to the child; prompt on the tty so we
		// don't consume the child's input.
		tty, err := openTTY()
		if err != nil {
			return nil, nil, errors.New("cannot prompt: stdin is redirected to the child and no controlling terminal is available (pass the value via the environment, or use --dry-run)")
		}
		return auth.NewInputReader(tty), func() { _ = tty.Close() }, nil
	}
	return auth.NewInputReader(stdin), func() {}, nil
}

// readPassphraseVia sources the vault passphrase from AQL_VAULT_PASSPHRASE
// or, failing that, an interactive prompt on the shared reader. It mirrors
// readPassphrase but reuses the exec's prompt source so a following --ask
// prompt is not stranded by the passphrase read's buffering.
func readPassphraseVia(p *promptSource, w io.Writer, prompt string) (string, error) {
	if v := os.Getenv(EnvPassphrase); v != "" {
		return v, nil
	}
	ir, err := p.reader()
	if err != nil {
		return "", err
	}
	pw, err := ir.ReadPassword(prompt, w)
	if err != nil {
		return "", err
	}
	if pw == "" {
		return "", errors.New("empty passphrase")
	}
	return pw, nil
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

// repeatedFlag collects every occurrence of a flag, so `--for=a --for=b`
// yields {"a","b"} instead of the last value winning.
type repeatedFlag []string

func (r *repeatedFlag) String() string { return strings.Join(*r, " ") }
func (r *repeatedFlag) Set(v string) error {
	*r = append(*r, v)
	return nil
}

// recipeBinding pairs a publisher recipe with the alias whose secret feeds it.
type recipeBinding struct {
	rec   publishRecipe
	alias string
}

// parseForRecipes turns the repeated --for values into recipe→alias
// bindings. Each entry is `recipe` (the secret comes from the single
// positional alias, the legacy form) or `recipe=alias` (the secret is named
// in-flag, which is what lets several --for entries each target their own
// secret). A bare entry with no positional alias is an error.
func parseForRecipes(entries []string, positional string) ([]recipeBinding, error) {
	positional = strings.TrimSpace(positional)
	out := make([]recipeBinding, 0, len(entries))
	for _, e := range entries {
		e = strings.TrimSpace(e)
		name, alias := e, ""
		if eq := strings.IndexByte(e, '='); eq >= 0 {
			name = strings.TrimSpace(e[:eq])
			alias = strings.TrimSpace(e[eq+1:])
		}
		rec, ok := lookupPublishRecipe(name)
		if !ok {
			return nil, fmt.Errorf("unknown --for recipe %q (known: %s)", name, strings.Join(publishRecipeNames(), ", "))
		}
		if alias == "" {
			if positional == "" {
				return nil, fmt.Errorf("--for=%s needs an alias; write --for=%s=<alias>", name, name)
			}
			if strings.ContainsAny(positional, ",=") {
				return nil, fmt.Errorf("--for takes a single alias (the recipe sets the env-var names)")
			}
			alias = positional
		} else if strings.ContainsAny(alias, ",=") {
			return nil, fmt.Errorf("invalid alias %q for --for=%s", alias, name)
		}
		out = append(out, recipeBinding{rec: rec, alias: alias})
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no --for recipes specified")
	}
	return out, nil
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
