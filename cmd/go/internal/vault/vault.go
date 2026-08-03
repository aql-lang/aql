package vault

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/boru-lang/boru/cmd/go/internal/auth"
	"github.com/boru-lang/boru/cmd/go/internal/command"
	"github.com/boru-lang/boru/cmd/go/internal/pathutil"
)

// Env names recognised by every mode.
const (
	EnvPassphrase = "BORU_VAULT_PASSPHRASE"
	EnvHome       = "BORU_HOME" // used in tests; overrides os.UserHomeDir
)

type cmd struct{}

// New returns the vault subcommand.
func New() command.Command { return &cmd{} }

func (*cmd) Name() string     { return "vault" }
func (*cmd) Synopsis() string { return "manage a local key vault (init, add, get, list, grant, ...)" }
func (*cmd) Run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	return Run(args, stdin, stdout, stderr)
}

// Run handles `boru vault [--folder=F] [--suffix=S] <mode> [args...]`.
func Run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	// Pull the global --folder/--suffix location flags out first; what
	// remains is the mode and its own arguments.
	folder, folderSet, suffix, suffixSet, rest, err := splitLocationArgs(args)
	if err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}
	// A flag, when given, overrides the matching env var for this
	// process — the path resolvers in location.go read only the env, so
	// promoting the flag keeps a single source of truth.
	if folderSet {
		if err := os.Setenv(EnvFolder, folder); err != nil {
			fmt.Fprintf(stderr, "error: %s\n", err)
			return 1
		}
	}
	if suffixSet {
		if err := os.Setenv(EnvSuffix, suffix); err != nil {
			fmt.Fprintf(stderr, "error: %s\n", err)
			return 1
		}
	}
	if s := os.Getenv(EnvSuffix); s != "" && !validSuffix(s) {
		fmt.Fprintf(stderr, "error: invalid vault suffix %q (allowed: letters, digits, dot, dash, underscore; must start with a letter or digit)\n", s)
		return 1
	}
	if len(rest) == 0 {
		printUsage(stderr)
		return 1
	}
	mode, rest := rest[0], rest[1:]

	homeDir, err := homeDir()
	if err != nil {
		// A folder override makes the home location irrelevant — every
		// vault path resolves from BORU_VAULT_FOLDER instead — so don't
		// fail just because the OS can't report a home folder.
		if os.Getenv(EnvFolder) == "" {
			fmt.Fprintf(stderr, "error: %s\n", err)
			return 1
		}
		homeDir = ""
	}

	switch mode {
	case "help", "-h", "--help":
		printUsage(stdout)
		return 0
	case "-i", "--interactive":
		return runInteractive(rest, homeDir, stdin, stdout, stderr)
	case "init":
		return runInit(rest, homeDir, stdin, stdout, stderr)
	case "status":
		return runStatus(rest, homeDir, stdout, stderr)
	case "add":
		return runAdd(rest, homeDir, stdin, stdout, stderr)
	case "get":
		return runGet(rest, homeDir, stdout, stderr)
	case "list", "ls":
		return runList(rest, homeDir, stdout, stderr)
	case "expiry", "expires":
		return runExpiry(rest, homeDir, stdout, stderr)
	case "rm", "remove", "delete":
		return runRemove(rest, homeDir, stdout, stderr)
	case "mv", "rename":
		return runMv(rest, homeDir, stdin, stdout, stderr)
	case "verify", "fsck":
		return runVerify(rest, homeDir, stdin, stdout, stderr)
	case "import":
		return runImport(rest, homeDir, stdin, stdout, stderr)
	case "export":
		return runExport(rest, homeDir, stdin, stdout, stderr)
	case "grant":
		return runGrant(rest, homeDir, stdout, stderr)
	case "revoke":
		return runRevoke(rest, homeDir, stdout, stderr)
	case "lock":
		return runLock(homeDir, stdout, stderr)
	case "unlock":
		return runUnlock(homeDir, stdout, stderr)
	case "config":
		return runConfig(rest, homeDir, stdout, stderr)
	case "proxy":
		return runProxy(rest, homeDir, stdout, stderr)
	case "serve":
		return runServe(rest, homeDir, stdout, stderr)
	case "provider", "providers":
		return runProvider(rest, homeDir, stdout, stderr)
	case "folder", "folders", "vaults", "list-vaults":
		return runVaults(rest, homeDir, stdout, stderr)
	case "scan":
		return runScan(rest, homeDir, stdout, stderr)
	case "audit":
		return runAudit(rest, homeDir, stdout, stderr)
	case "rotate":
		return runRotate(rest, homeDir, stdin, stdout, stderr)
	case "policy":
		return runPolicy(rest, homeDir, stdin, stdout, stderr)
	case "password", "pw":
		return runPassword(rest, homeDir, stdin, stdout, stderr)
	case "history":
		return runHistory(rest, homeDir, stdout, stderr)
	case "restore":
		return runRestore(rest, homeDir, stdin, stdout, stderr)
	case "mcp":
		return runMCP(rest, homeDir, stdin, stdout, stderr)
	case "exec":
		return runExec(rest, homeDir, stdin, stdout, stderr)
	default:
		fmt.Fprintf(stderr, "error: unknown vault mode %q\n", mode)
		printUsage(stderr)
		return 1
	}
}

func printUsage(w io.Writer) {
	fmt.Fprintln(w, "Usage: boru vault [--folder=PATH] [--suffix=NAME] <mode> [args...]")
	fmt.Fprintln(w, "       boru vault -i   (interactive TUI — manage the vault with a menu)")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Modes:")
	for _, m := range modeDocs {
		fmt.Fprintf(w, "  %-10s %s\n", m.name, m.summary)
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Interactive: boru vault -i opens a menu-driven TUI (switch vaults, browse & edit")
	fmt.Fprintln(w, "  secrets, capabilities, passwords, maintenance); keys are shown on screen.")
	fmt.Fprintln(w, "Backends: auto (default), keychain, secret-service, wincred, file, 1password.")
	fmt.Fprintln(w, "Passphrases are prompted interactively (hidden); set BORU_VAULT_PASSPHRASE only for non-interactive use.")
	fmt.Fprintln(w, "Namespaces: qualify aliases as ns:name; bare names use the default namespace")
	fmt.Fprintln(w, "  (vault config --set namespace.default=NS, or BORU_VAULT_NAMESPACE; ':' = root, :name forces root).")
	fmt.Fprintln(w, "Location: the vault lives in ~/.boru with files vault.<part> by default. Override the")
	fmt.Fprintln(w, "  folder with --folder/BORU_VAULT_FOLDER and the file suffix (vault.<suffix>.jsonic)")
	fmt.Fprintln(w, "  with --suffix/BORU_VAULT_SUFFIX; pass the same values to every command on that vault.")
}

type modeDoc struct{ name, summary string }

var modeDocs = []modeDoc{
	{"init", "initialize the vault; choose a backend"},
	{"status", "print backend, secret count, lock state"},
	{"add", "store a secret under an alias"},
	{"get", "retrieve a secret (redacted unless --reveal)"},
	{"list", "list aliases and metadata (no values)"},
	{"expiry", "list pending key expiries; set/clear them (filter by namespace)"},
	{"rm", "remove a secret"},
	{"mv", "rename a key, move it between namespaces, or rename a namespace (ns:)"},
	{"verify", "reconcile the store and keyring; --prune repairs (also: fsck)"},
	{"import", "import secrets from a .env file or an encrypted export bundle"},
	{"export", "export secrets to a portable, passphrase-encrypted bundle"},
	{"grant", "issue a scoped capability token for an alias"},
	{"revoke", "revoke a capability token"},
	{"lock", "mark the vault locked (block get/grant)"},
	{"unlock", "mark the vault unlocked"},
	{"config", "view or set vault configuration"},
	{"proxy", "run a local credential broker for agents and tools"},
	{"serve", "serve secrets read-only over a HashiCorp-Vault-style wire protocol"},
	{"providers", "list provider presets; `provider add --url=U NAME` defines a custom one"},
	{"folder", "list vault folders; `folder add <dir>` registers an existing vault"},
	{"scan", "scan files for leaked secret-like strings (--home checks credential dotfiles)"},
	{"audit", "show the structured audit log"},
	{"rotate", "replace a stored secret value, optionally revoking caps"},
	{"policy", "declaratively apply / show vault aliases and capabilities"},
	{"password", "manage scoped vault passwords (add/assign/set/rm/list)"},
	{"history", "show the content-revision history (vault.jsonic.log)"},
	{"restore", "restore vault metadata to a past generation (admin)"},
	{"mcp", "run a stdio MCP server exposing aliases as tools"},
	{"exec", "run a command with secrets injected as env vars (--for=npm|cargo|gem|pypi|uv to publish)"},
}

// --- shared helpers --------------------------------------------------------

// homeDir resolves the directory holding ~/.boru, honoring BORU_HOME
// for tests.
func homeDir() (string, error) {
	if h := os.Getenv(EnvHome); h != "" {
		return h, nil
	}
	return os.UserHomeDir()
}

// requireStore loads the store and returns an error if the vault
// has not been initialized.
func requireStore(homeDir string) (*Store, error) {
	s, err := LoadStore(homeDir)
	if err != nil {
		return nil, err
	}
	if s == nil {
		return nil, errors.New("vault not initialized; run `boru vault init`")
	}
	return s, nil
}

// openKeyring resolves the backend recorded in s and prompts for
// the file passphrase when needed. The passphrase is sourced from
// BORU_VAULT_PASSPHRASE if set, then from stdin (echo suppressed)
// when stdin is a terminal.
func openKeyring(s *Store, homeDir string, stdin io.Reader, stdout io.Writer, prompt string) (keyring, error) {
	backend := s.Backend
	if backend == "" {
		backend = BackendAuto
	}
	resolved := backend
	if backend == BackendAuto {
		resolved = autoBackend()
	}
	if resolved != BackendFile {
		return selectKeyring(backend, vaultFolder(homeDir), "")
	}
	pass := os.Getenv(EnvPassphrase)
	if pass == "" && stdin != nil {
		ir := auth.NewInputReader(stdin)
		p, err := ir.ReadPassword(prompt, stdout)
		if err != nil {
			return nil, err
		}
		pass = p
	}
	if pass == "" {
		return nil, errors.New("file backend requires a passphrase; run interactively to be prompted, or set BORU_VAULT_PASSPHRASE for non-interactive use")
	}
	return selectKeyring(BackendFile, vaultFolder(homeDir), pass)
}

// --- init ------------------------------------------------------------------

func runInit(args []string, homeDir string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("vault init", flag.ContinueOnError)
	fs.SetOutput(stderr)
	backend := fs.String("backend", BackendAuto, "storage backend: auto, keychain, secret-service, wincred, file, 1password")
	force := fs.Bool("force", false, "reinitialize an existing vault")
	if err := fs.Parse(args); err != nil {
		return 1
	}

	if s, _ := LoadStore(homeDir); s != nil && !*force {
		fmt.Fprintf(stderr, "error: vault already initialized at %s (use --force to reinitialize)\n", StorePath(homeDir))
		return 1
	}

	chosen := *backend
	if chosen == BackendAuto {
		chosen = autoBackend()
	}
	// For the file backend, capture a passphrase up front so future
	// operations can require it. A non-empty passphrase is mandatory;
	// it is read from BORU_VAULT_PASSPHRASE if set, otherwise prompted
	// for twice with echo suppressed.
	if chosen == BackendFile {
		pass := os.Getenv(EnvPassphrase)
		if pass == "" {
			ir := auth.NewInputReader(stdin)
			p1, err := ir.ReadPassword("Set vault passphrase: ", stdout)
			if err != nil {
				fmt.Fprintf(stderr, "error: %s\n", err)
				return 1
			}
			p2, err := ir.ReadPassword("Confirm passphrase: ", stdout)
			if err != nil {
				fmt.Fprintf(stderr, "error: %s\n", err)
				return 1
			}
			if p1 != p2 {
				fmt.Fprintf(stderr, "error: passphrases did not match\n")
				return 1
			}
			pass = p1
		}
		if pass == "" {
			fmt.Fprintln(stderr, "error: empty passphrase — the file keyring would be effectively unencrypted (its salt is stored alongside the ciphertext, so anyone who can read ~/.boru/vault.keyring could recover every secret). Choose a non-empty passphrase, or set BORU_VAULT_PASSPHRASE.")
			return 1
		}
		// Initialize an empty keyring file so its presence and
		// passphrase are validated immediately.
		kr := &fileKeyring{folder: vaultFolder(homeDir), pass: pass}
		if err := kr.save(map[string]string{}); err != nil {
			fmt.Fprintf(stderr, "error: writing keyring: %s\n", err)
			return 1
		}
	} else {
		// Probe the host backend so we fail loudly here, not later
		// during the first `vault add`.
		if _, err := selectKeyring(chosen, vaultFolder(homeDir), ""); err != nil {
			fmt.Fprintf(stderr, "error: %s\n", err)
			return 1
		}
	}

	if err := withVaultLock(homeDir, func() error {
		// Re-check under the lock so two concurrent inits can't both
		// create a store (the early check above is best-effort).
		if existing, _ := c7loadStore(homeDir); existing != nil && !*force {
			return fmt.Errorf("vault already initialized at %s (use --force to reinitialize)", StorePath(homeDir))
		}
		return SaveStore(homeDir, &Store{
			Version: storeVersion,
			Backend: chosen,
			Config:  map[string]any{},
		})
	}); err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}
	_ = appendAudit(homeDir, AuditEvent{Action: "vault.init", Outcome: "ok", Reason: "backend=" + chosen})
	// Record the vault in the global index so it is auto-discoverable by the
	// interactive TUI and `vault vaults`, even when it lives in a custom
	// --folder location. Best-effort: a registry write never fails init.
	_ = recordVaultInit(homeDir, vaultFolder(homeDir), os.Getenv(EnvSuffix), chosen)
	fmt.Fprintf(stdout, "vault initialized: backend=%s store=%s\n", chosen, StorePath(homeDir))
	return 0
}

// --- status ----------------------------------------------------------------

func runStatus(args []string, homeDir string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("vault status", flag.ContinueOnError)
	fs.SetOutput(stderr)
	if err := fs.Parse(args); err != nil {
		return 1
	}
	s, err := LoadStore(homeDir)
	if err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}
	if s == nil {
		fmt.Fprintln(stdout, "vault: not initialized (run `boru vault init`)")
		return 0
	}
	active := s.ActiveCapabilities(time.Now())
	fmt.Fprintf(stdout, "backend:       %s\n", s.Backend)
	fmt.Fprintf(stdout, "store:         %s\n", StorePath(homeDir))
	fmt.Fprintf(stdout, "locked:        %t\n", s.Locked)
	fmt.Fprintf(stdout, "aliases:       %d\n", len(s.Aliases))
	if line := namespaceBreakdown(s); line != "" {
		fmt.Fprintf(stdout, "namespaces:    %s\n", line)
	}
	fmt.Fprintf(stdout, "capabilities:  %d active / %d total\n", len(active), len(s.Capabilities))
	fmt.Fprintf(stdout, "agents:        %d registered\n", len(s.Agents))
	if s.HasPasswordSlots() {
		fmt.Fprintf(stdout, "passwords:     %d slots (%d admin)\n", len(s.Passwords), adminSlotCount(s))
		fmt.Fprintf(stdout, "generation:    %d\n", s.Generation)
	}
	return 0
}

// adminSlotCount counts admin-scoped password slots.
func adminSlotCount(s *Store) int {
	n := 0
	for i := range s.Passwords {
		if s.Passwords[i].Scope == ScopeAdmin {
			n++
		}
	}
	return n
}

// namespaceBreakdown renders per-namespace alias counts, e.g.
// "(root)=2 proj=3". Empty when every alias is root-level, so the
// status output is unchanged for vaults that don't use namespaces.
func namespaceBreakdown(s *Store) string {
	counts := map[string]int{}
	for _, a := range s.Aliases {
		counts[aliasNamespace(a)]++
	}
	if len(counts) == 1 && counts[""] > 0 {
		return ""
	}
	names := make([]string, 0, len(counts))
	for ns := range counts {
		names = append(names, ns)
	}
	sort.Strings(names) // "" (root) sorts first
	parts := make([]string, 0, len(names))
	for _, ns := range names {
		label := ns
		if ns == "" {
			label = "(root)"
		}
		parts = append(parts, fmt.Sprintf("%s=%d", label, counts[ns]))
	}
	return strings.Join(parts, " ")
}

// --- add -------------------------------------------------------------------

func runAdd(args []string, homeDir string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("vault add", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fromEnv := fs.String("from-env", "", "read value from this environment variable instead of prompting")
	fromStdin := fs.Bool("from-stdin", false, "read value from a single line on stdin")
	fromClipboard := fs.Bool("from-clipboard", false, "read value from the OS clipboard, then wipe the clipboard")
	provider := fs.String("provider", "", "tag this secret with a provider (openai, anthropic, github, ...)")
	namespace := fs.String("namespace", "", "store under this namespace (same as the ns: prefix; ':' = root)")
	expiry := fs.String("expiry", "", "optional expiry reminder: YYYY-MM-DD, an RFC3339 timestamp, or a duration like 90d / 720h")
	ipWhitelist := fs.String("ip-whitelist", "", "comma-separated client IPs/CIDRs allowed to use this secret via the proxy (empty = no restriction)")
	yes := fs.Bool("yes", false, "confirm overwriting an existing secret non-interactively")
	fs.BoolVar(yes, "y", false, "confirm overwriting an existing secret non-interactively (shorthand)")
	if err := fs.Parse(args); err != nil {
		return 1
	}
	if fs.NArg() < 1 {
		fmt.Fprintf(stderr, "error: usage: boru vault add [--from-env=VAR | --from-stdin | --from-clipboard | --provider=...] [--namespace=NS] [--expiry=WHEN] [--ip-whitelist=IPs] <[ns:]alias>\n")
		return 1
	}
	expiresAt, err := parseExpiryFlag(*expiry)
	if err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}
	// --ip-whitelist is tri-state (as in rotate): absent = keep any existing
	// allowlist when overwriting; present-and-empty = clear it; present-and-set
	// = replace. fs.Visit distinguishes "absent" from an explicit empty value,
	// so `add --ip-whitelist=` actually removes a prior restriction instead of
	// silently preserving it (UpsertAlias treats nil as preserve).
	ipwlSet := false
	fs.Visit(func(f *flag.Flag) {
		if f.Name == "ip-whitelist" {
			ipwlSet = true
		}
	})
	var ipwl []string
	if ipwlSet {
		if ipwl, err = parseIPWhitelist(*ipWhitelist); err != nil {
			fmt.Fprintf(stderr, "error: %s\n", err)
			return 1
		}
		if ipwl == nil {
			ipwl = []string{} // explicit clear (non-nil empty = replace with none)
		}
	}
	aliasRef := fs.Arg(0)
	// --namespace is sugar for the ns: prefix; both at once is a
	// conflict rather than a precedence puzzle.
	if *namespace != "" {
		if strings.Contains(aliasRef, ":") {
			fmt.Fprintf(stderr, "error: alias %q already carries a namespace; drop --namespace or the prefix\n", aliasRef)
			return 1
		}
		if *namespace == rootNamespaceRef {
			aliasRef = rootNamespaceRef + aliasRef
		} else if !validNamespaceName(*namespace) {
			fmt.Fprintf(stderr, "error: invalid namespace %q\n", *namespace)
			return 1
		} else {
			aliasRef = *namespace + ":" + aliasRef
		}
	}
	s, err := requireStore(homeDir)
	if err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}
	if s.Locked {
		fmt.Fprintln(stderr, "error: vault is locked; run `boru vault unlock`")
		return 1
	}
	alias, err := resolveAliasRef(s, aliasRef)
	if err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}

	value, source, err := readSecretValue(*fromEnv, *fromStdin, *fromClipboard, stdin, stdout)
	if err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}
	if value == "" {
		fmt.Fprintln(stderr, "error: empty value; refusing to store")
		return 1
	}

	ns, _ := splitAlias(alias)
	// Overwriting an existing secret is destructive; a fresh alias is not.
	if existing, _ := s.FindAlias(alias); existing != nil {
		if err := confirmDestructive(confirmTier1, alias, fmt.Sprintf("overwrite existing secret %q", alias), *yes, false, stdin, stdout); err != nil {
			fmt.Fprintf(stderr, "error: %s\n", err)
			return 1
		}
	}
	sess, err := authenticate(s, homeDir, stdin, stdout, "Vault passphrase: ")
	if err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}
	defer sess.Close()
	if err := requireScopeNS(sess, OpWrite, ns); err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}
	if err := w8writeSecret(homeDir, sess, alias, ns, value); err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}
	if err := mutateStore(homeDir, func(s *Store) error {
		if s.Locked {
			return errLocked
		}
		s.UpsertAlias(Alias{
			Name:        alias,
			Provider:    *provider,
			Namespace:   ns, // derived from the name so the two can't disagree
			Source:      source,
			ExpiresAt:   expiresAt,
			IPWhitelist: ipwl,
		})
		return nil
	}); err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}
	sealIntegrity(homeDir, sess)
	_ = appendAudit(homeDir, AuditEvent{
		Action: "vault.add", Alias: alias, Provider: *provider, Outcome: "ok",
		Reason: "source=" + source,
	})
	fmt.Fprintf(stdout, "stored %s (backend=%s, %d bytes)\n", alias, sess.Keyring().Name(), len(value))
	if expiresAt != "" {
		fmt.Fprintf(stdout, "  expires %s\n", expiresAt)
	}
	if len(ipwl) > 0 {
		fmt.Fprintf(stdout, "  ip-whitelist %s\n", strings.Join(ipwl, ", "))
	}
	// Only wipe the clipboard once the secret is safely persisted, so a
	// storage failure leaves the value available for the user to retry.
	if *fromClipboard {
		clearClipboardAfterStore(stdout, stderr)
	}
	return 0
}

func readSecretValue(fromEnv string, fromStdin, fromClipboard bool, stdin io.Reader, stdout io.Writer) (string, string, error) {
	// At most one explicit source may be selected; otherwise we fall
	// back to an interactive prompt.
	n := 0
	if fromEnv != "" {
		n++
	}
	if fromStdin {
		n++
	}
	if fromClipboard {
		n++
	}
	if n > 1 {
		return "", "", fmt.Errorf("choose only one of --from-env, --from-stdin, --from-clipboard")
	}
	if fromEnv != "" {
		v := os.Getenv(fromEnv)
		if v == "" {
			return "", "", fmt.Errorf("environment variable %s is empty or unset", fromEnv)
		}
		return v, "env:" + fromEnv, nil
	}
	if fromClipboard {
		clip, err := detectClipboard(hostClipEnv())
		if err != nil {
			return "", "", fmt.Errorf("clipboard: %w", err)
		}
		v, err := clip.paste()
		if err != nil {
			return "", "", fmt.Errorf("reading clipboard: %w", err)
		}
		if v == "" {
			return "", "", fmt.Errorf("clipboard is empty")
		}
		return v, "clipboard", nil
	}
	if fromStdin {
		br := bufio.NewReader(stdin)
		line, err := br.ReadString('\n')
		if err != nil && line == "" {
			return "", "", fmt.Errorf("reading stdin: %w", err)
		}
		return strings.TrimRight(line, "\r\n"), "stdin", nil
	}
	ir := auth.NewInputReader(stdin)
	v, err := ir.ReadPassword("Secret value: ", stdout)
	if err != nil {
		return "", "", err
	}
	return v, "prompt", nil
}

// --- get -------------------------------------------------------------------

func runGet(args []string, homeDir string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("vault get", flag.ContinueOnError)
	fs.SetOutput(stderr)
	reveal := fs.Bool("reveal", false, "print the full secret to stdout (otherwise redacted)")
	if err := fs.Parse(args); err != nil {
		return 1
	}
	if fs.NArg() < 1 {
		fmt.Fprintf(stderr, "error: usage: boru vault get [--reveal] <alias>\n")
		return 1
	}
	s, err := requireStore(homeDir)
	if err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}
	if s.Locked {
		fmt.Fprintln(stderr, "error: vault is locked; run `boru vault unlock`")
		return 1
	}
	_, alias, err := findAliasRef(s, fs.Arg(0))
	if err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}
	ns, _ := splitAlias(alias)
	sess, err := authenticate(s, homeDir, os.Stdin, stdout, "Vault passphrase: ")
	if err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}
	defer sess.Close()
	if err := requireScopeNS(sess, OpRead, ns); err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}
	v, err := sess.getValue(alias, ns)
	if err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}
	if *reveal {
		fmt.Fprintln(stdout, v)
		return 0
	}
	fmt.Fprintln(stdout, redact(v))
	return 0
}

// redact returns a fixed-shape mask that reveals only the secret
// length category, never enough to guess any character. Length is
// useful for diagnosis ("is this the 51-char OpenAI key?") without
// being sensitive on its own.
func redact(v string) string {
	if v == "" {
		return "<empty>"
	}
	if len(v) <= 8 {
		return "********"
	}
	return strings.Repeat("*", 8) + fmt.Sprintf(" (%d bytes)", len(v))
}

// --- list ------------------------------------------------------------------

func runList(args []string, homeDir string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("vault list", flag.ContinueOnError)
	fs.SetOutput(stderr)
	namespace := fs.String("namespace", "", "filter by namespace (':' = root only)")
	if err := fs.Parse(args); err != nil {
		return 1
	}
	nsFilter, nsFiltered, err := normalizeNSFilter(*namespace)
	if err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}
	s, err := requireStore(homeDir)
	if err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}
	aliases := s.SortedAliases()
	if len(aliases) == 0 {
		fmt.Fprintln(stdout, "(no aliases)")
		return 0
	}
	fmt.Fprintf(stdout, "%-24s %-12s %-16s %-20s %-20s %-20s %s\n", "ALIAS", "PROVIDER", "NAMESPACE", "CREATED", "EXPIRES", "SOURCE", "IP-WHITELIST")
	for _, a := range aliases {
		if nsFiltered && aliasNamespace(a) != nsFilter {
			continue
		}
		fmt.Fprintf(stdout, "%-24s %-12s %-16s %-20s %-20s %-20s %s\n",
			a.Name, dash(a.Provider), dash(aliasNamespace(a)), a.CreatedAt, dash(a.ExpiresAt), dash(a.Source), dashJoin(a.IPWhitelist))
	}
	return 0
}

// dashJoin renders a string slice as a comma-separated list, or "-" when
// empty (the list-column counterpart of dash).
func dashJoin(xs []string) string {
	if len(xs) == 0 {
		return "-"
	}
	return strings.Join(xs, ",")
}

func dash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

// --- rm --------------------------------------------------------------------

func runRemove(args []string, homeDir string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("vault rm", flag.ContinueOnError)
	fs.SetOutput(stderr)
	yes := fs.Bool("yes", false, "confirm the deletion non-interactively")
	fs.BoolVar(yes, "y", false, "confirm the deletion non-interactively (shorthand)")
	if err := fs.Parse(args); err != nil {
		return 1
	}
	if fs.NArg() < 1 {
		fmt.Fprintf(stderr, "error: usage: boru vault rm [--yes] <alias>\n")
		return 1
	}
	s, err := requireStore(homeDir)
	if err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}
	_, alias, err := findAliasRef(s, fs.Arg(0))
	if err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}
	ns, _ := splitAlias(alias)
	sess, err := authenticate(s, homeDir, os.Stdin, stdout, "Vault passphrase: ")
	if err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}
	defer sess.Close()
	if err := requireScopeNS(sess, OpWrite, ns); err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}
	if err := confirmDestructive(confirmTier1, alias, fmt.Sprintf("delete secret %q", alias), *yes, false, os.Stdin, stdout); err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}
	// Drop the metadata reference first, then the keyring entry. A crash
	// between the two leaves an orphaned (unreferenced) keyring entry —
	// harmless and reclaimable — rather than a metadata alias whose
	// secret is already gone, which would fail every subsequent get.
	if err := mutateStore(homeDir, func(s *Store) error {
		s.RemoveAlias(alias)
		return nil
	}); err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}
	if err := sess.deleteValue(alias); err != nil {
		fmt.Fprintf(stderr, "warning: removed %s from the vault but could not delete its keyring entry: %s\n", alias, err)
	}
	sealIntegrity(homeDir, sess)
	_ = appendAudit(homeDir, AuditEvent{Action: "vault.rm", Alias: alias, Outcome: "ok"})
	fmt.Fprintf(stdout, "removed %s\n", alias)
	return 0
}

// --- import ----------------------------------------------------------------

// runImport ingests secrets from a positional file (or stdin when no
// path is given). The source is dispatched by content: an encrypted
// export bundle (recognized by its magic header) is handled by
// importBundle; anything else is parsed as a .env file. The two share
// the --namespace/--provider/--prefix flags; --overwrite and the
// BORU_VAULT_EXPORT_PASSPHRASE are bundle-only.
func runImport(args []string, homeDir string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("vault import", flag.ContinueOnError)
	fs.SetOutput(stderr)
	namespace := fs.String("namespace", "", "namespace for imported secrets (.env: qualifies bare keys; bundle: remaps; ':' = root)")
	provider := fs.String("provider", "", "tag imported secrets with this provider (.env only)")
	prefix := fs.String("prefix", "", "prepend this prefix to each alias")
	overwrite := fs.Bool("overwrite", false, "replace aliases that already exist (bundle import)")
	dryRun := fs.Bool("dry-run", false, "show what would be imported without changing the vault (.env)")
	if err := fs.Parse(args); err != nil {
		return 1
	}

	srcName := "stdin"
	fromStdin := true
	var data []byte
	if fs.NArg() >= 1 {
		// Expand a leading ~ the shell left verbatim (quoted path, etc.).
		srcName = pathutil.ExpandTilde(fs.Arg(0), homeDir)
		fromStdin = false
		b, err := os.ReadFile(srcName)
		if err != nil {
			fmt.Fprintf(stderr, "error: %s\n", err)
			return 1
		}
		data = b
	} else {
		b, err := io.ReadAll(stdin)
		if err != nil {
			fmt.Fprintf(stderr, "error: reading stdin: %s\n", err)
			return 1
		}
		data = b
	}

	if isExportBundle(data) {
		return importBundle(data, fromStdin, homeDir, stdin, stdout, stderr, *prefix, *namespace, *overwrite)
	}
	return importDotenv(data, srcName, fromStdin, homeDir, stdin, stdout, stderr, *namespace, *provider, *prefix, *dryRun)
}

// importDotenv loads KEY=VALUE pairs from .env-shaped content. This is
// the original `vault import` behavior, parameterized over an already
// read buffer so runImport can dispatch by content.
func importDotenv(data []byte, srcName string, fromStdin bool, homeDir string, stdin io.Reader, stdout, stderr io.Writer, namespace, provider, prefix string, dryRun bool) int {
	entries := parseDotenv(string(data))
	if len(entries) == 0 {
		fmt.Fprintln(stderr, "error: no key=value pairs in input")
		return 1
	}
	s, err := requireStore(homeDir)
	if err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}
	if s.Locked {
		fmt.Fprintln(stderr, "error: vault is locked; run `boru vault unlock`")
		return 1
	}
	// .env keys are bare names, so they follow the same resolution rule
	// as `vault add`: the --namespace flag (':' = root) wins, else the
	// active default namespace, else root.
	effNS, err := importNamespace(s, namespace)
	if err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}
	qualify := func(name string) string {
		if effNS == "" {
			return name
		}
		return effNS + ":" + name
	}
	if dryRun {
		for _, k := range sortedKeys(entries) {
			fmt.Fprintf(stdout, "would import %s (%d bytes)\n", qualify(prefix+k), len(entries[k]))
		}
		return 0
	}
	// When the content arrived on stdin, stdin is exhausted and cannot
	// also carry an interactive passphrase, so require it from the env.
	krStdin := stdin
	if fromStdin {
		krStdin = nil
	}
	sess, err := authenticate(s, homeDir, krStdin, stdout, "Vault passphrase: ")
	if err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}
	defer sess.Close()
	if err := requireScopeNS(sess, OpWrite, effNS); err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}
	var done []string
	for _, k := range sortedKeys(entries) {
		alias := qualify(prefix + k)
		if !validAlias(alias) {
			fmt.Fprintf(stderr, "warning: skipping invalid alias %q\n", alias)
			continue
		}
		if err := writeSecret(homeDir, sess, alias, effNS, entries[k]); err != nil {
			fmt.Fprintf(stderr, "error: storing %s: %s\n", alias, err)
			return 1
		}
		done = append(done, alias)
		_ = appendAudit(homeDir, AuditEvent{
			Action: "vault.import", Alias: alias, Provider: provider,
			Outcome: "ok", Reason: "source=" + srcName,
		})
		fmt.Fprintf(stdout, "imported %s\n", alias)
	}
	if err := mutateStore(homeDir, func(s *Store) error {
		for _, alias := range done {
			s.UpsertAlias(Alias{
				Name:      alias,
				Provider:  provider,
				Namespace: effNS,
				Source:    "import:" + srcName,
			})
		}
		return nil
	}); err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}
	sealIntegrity(homeDir, sess)
	return 0
}

func sortedKeys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// parseDotenv handles the common subset of .env syntax: blank
// lines, comments starting with '#', optional `export` prefix,
// single- or double-quoted values, and `KEY=VALUE` pairs. Anything
// it can't parse is silently skipped — the alias validator catches
// any junk that slips through.
func parseDotenv(s string) map[string]string {
	out := map[string]string{}
	for _, raw := range strings.Split(s, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		eq := strings.IndexByte(line, '=')
		if eq < 0 {
			continue
		}
		key := strings.TrimSpace(line[:eq])
		val := strings.TrimSpace(line[eq+1:])
		if i := strings.Index(val, " #"); i >= 0 {
			val = strings.TrimSpace(val[:i])
		}
		if len(val) >= 2 {
			if (val[0] == '"' && val[len(val)-1] == '"') || (val[0] == '\'' && val[len(val)-1] == '\'') {
				val = val[1 : len(val)-1]
			}
		}
		if key == "" {
			continue
		}
		out[key] = val
	}
	return out
}

// --- grant / revoke --------------------------------------------------------

func runGrant(args []string, homeDir string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("vault grant", flag.ContinueOnError)
	fs.SetOutput(stderr)
	agent := fs.String("agent", "", "name of the agent or tool receiving the capability")
	hosts := fs.String("hosts", "", "comma-separated host allowlist (e.g. api.openai.com)")
	methods := fs.String("methods", "", "comma-separated HTTP methods (default any)")
	ttl := fs.Duration("ttl", 2*time.Hour, "lifetime before the capability expires")
	maxCalls := fs.Int("max-calls", 0, "max total proxy calls (0 = unlimited)")
	maxCostCents := fs.Int("max-cost-cents", 0, "max total cost in cents from X-Boru-Vault-Cost-Cents (0 = unlimited)")
	approval := fs.Bool("require-approval", false, "advisory: proxy will deny until a human flips this off")
	if err := fs.Parse(args); err != nil {
		return 1
	}
	if fs.NArg() < 1 {
		fmt.Fprintf(stderr, "error: usage: boru vault grant [--agent=NAME] [--hosts=H,H] [--methods=GET,POST] [--ttl=2h] [--max-calls=N] [--max-cost-cents=N] [--require-approval] <alias | 'ns:*' | '*'>\n")
		return 1
	}
	s, err := requireStore(homeDir)
	if err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}
	if s.Locked {
		fmt.Fprintln(stderr, "error: vault is locked; run `boru vault unlock`")
		return 1
	}
	alias, wild, err := grantAliasRef(s, fs.Arg(0))
	if err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}
	var token string
	var tok Capability
	if err := mutateStore(homeDir, func(s *Store) error {
		if s.Locked {
			return errLocked
		}
		// A wildcard names a namespace, not one alias, so there is no
		// single record whose existence the grant depends on.
		if a, _ := s.FindAlias(alias); a == nil && !wild {
			return fmt.Errorf("no alias named %q", alias)
		}
		_, tk, err := c7newCapability(s, alias, *agent, splitCSV(*hosts), splitCSV(*methods), *ttl)
		if err != nil {
			return err
		}
		idx := len(s.Capabilities) - 1
		s.Capabilities[idx].MaxCalls = *maxCalls
		s.Capabilities[idx].MaxCostCents = *maxCostCents
		s.Capabilities[idx].RequireApproval = *approval
		// Bind the capability to the namespace its alias lives in, so a
		// broker can confirm a presented token is being used within the
		// namespace it was minted for (defence in depth on top of the
		// per-alias token check and the broker password's NDK scope).
		s.Capabilities[idx].Namespace = valueNamespace(alias)
		token, tok = tk, s.Capabilities[idx]
		return nil
	}); err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}
	_ = appendAudit(homeDir, AuditEvent{
		Action: "vault.grant", Alias: alias, Capability: tok.ID,
		Agent: tok.Agent, Outcome: "ok",
	})
	fmt.Fprintf(stdout, "capability: %s\n", tok.ID)
	fmt.Fprintf(stdout, "token:      %s\n", token)
	fmt.Fprintln(stdout, "  (use this token as the proxy Bearer credential — it is shown once; only its hash is stored)")
	fmt.Fprintf(stdout, "alias:      %s\n", tok.Alias)
	if wild {
		fmt.Fprintf(stdout, "  (namespace wildcard — grants read of every %q-namespace secret, wire protocol only: `boru vault serve`)\n", nsLabel(valueNamespace(alias)))
	}
	if tok.Agent != "" {
		fmt.Fprintf(stdout, "agent:      %s\n", tok.Agent)
	}
	if len(tok.Hosts) > 0 {
		fmt.Fprintf(stdout, "hosts:      %s\n", strings.Join(tok.Hosts, ","))
	}
	if len(tok.Methods) > 0 {
		fmt.Fprintf(stdout, "methods:    %s\n", strings.Join(tok.Methods, ","))
	}
	if tok.ExpiresAt != "" {
		fmt.Fprintf(stdout, "expires:    %s\n", tok.ExpiresAt)
	}
	if tok.MaxCalls > 0 {
		fmt.Fprintf(stdout, "max-calls:  %d\n", tok.MaxCalls)
	}
	if tok.MaxCostCents > 0 {
		fmt.Fprintf(stdout, "max-cost:   %dc\n", tok.MaxCostCents)
	}
	if tok.RequireApproval {
		fmt.Fprintln(stdout, "approval:   required (proxy will deny until cleared)")
	}
	return 0
}

func runRevoke(args []string, homeDir string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("vault revoke", flag.ContinueOnError)
	fs.SetOutput(stderr)
	if err := fs.Parse(args); err != nil {
		return 1
	}
	if fs.NArg() < 1 {
		fmt.Fprintf(stderr, "error: usage: boru vault revoke <capability-id>\n")
		return 1
	}
	id := fs.Arg(0)
	var revokedID, revokedAlias string
	if err := mutateStore(homeDir, func(s *Store) error {
		c, idx := s.FindCapability(id)
		if c == nil {
			return fmt.Errorf("no capability matching %q", id)
		}
		s.Capabilities[idx].Revoked = true
		revokedID = s.Capabilities[idx].ID
		revokedAlias = s.Capabilities[idx].Alias
		return nil
	}); err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}
	_ = appendAudit(homeDir, AuditEvent{
		Action: "vault.revoke", Capability: revokedID,
		Alias: revokedAlias, Outcome: "ok",
	})
	fmt.Fprintf(stdout, "revoked %s\n", revokedID)
	return 0
}

// --- lock / unlock ---------------------------------------------------------

func runLock(homeDir string, stdout, stderr io.Writer) int {
	if err := mutateStore(homeDir, func(s *Store) error {
		s.Locked = true
		return nil
	}); err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}
	_ = appendAudit(homeDir, AuditEvent{Action: "vault.lock", Outcome: "ok"})
	fmt.Fprintln(stdout, "vault locked")
	return 0
}

func runUnlock(homeDir string, stdout, stderr io.Writer) int {
	if err := mutateStore(homeDir, func(s *Store) error {
		s.Locked = false
		return nil
	}); err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}
	_ = appendAudit(homeDir, AuditEvent{Action: "vault.unlock", Outcome: "ok"})
	fmt.Fprintln(stdout, "vault unlocked")
	return 0
}

// --- config ---------------------------------------------------------------

func runConfig(args []string, homeDir string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("vault config", flag.ContinueOnError)
	fs.SetOutput(stderr)
	set := fs.String("set", "", "set a config key=value")
	unset := fs.String("unset", "", "remove a config key")
	if err := fs.Parse(args); err != nil {
		return 1
	}
	switch {
	case *set != "":
		eq := strings.IndexByte(*set, '=')
		if eq < 0 {
			fmt.Fprintln(stderr, "error: --set requires key=value")
			return 1
		}
		key, val := (*set)[:eq], (*set)[eq+1:]
		if err := mutateStore(homeDir, func(s *Store) error {
			if s.Config == nil {
				s.Config = map[string]any{}
			}
			s.Config[key] = val
			return nil
		}); err != nil {
			fmt.Fprintf(stderr, "error: %s\n", err)
			return 1
		}
		fmt.Fprintf(stdout, "set %s=%s\n", key, val)
		return 0
	case *unset != "":
		if err := mutateStore(homeDir, func(s *Store) error {
			delete(s.Config, *unset)
			return nil
		}); err != nil {
			fmt.Fprintf(stderr, "error: %s\n", err)
			return 1
		}
		fmt.Fprintf(stdout, "unset %s\n", *unset)
		return 0
	default:
		s, err := requireStore(homeDir)
		if err != nil {
			fmt.Fprintf(stderr, "error: %s\n", err)
			return 1
		}
		keys := make([]string, 0, len(s.Config))
		for k := range s.Config {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			fmt.Fprintf(stdout, "%s=%v\n", k, s.Config[k])
		}
		if len(keys) == 0 {
			fmt.Fprintln(stdout, "(no config)")
		}
		return 0
	}
}

// --- rotate ---------------------------------------------------------------

// runRotate replaces the secret behind an alias with a new value
// while preserving the alias metadata (provider, namespace, creation
// time). Existing capabilities continue to work — the rotation is
// transparent at the broker layer. Pass --revoke-caps to invalidate
// all live capabilities for the alias at the same time (the safer
// default for incident response).
func runRotate(args []string, homeDir string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("vault rotate", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fromEnv := fs.String("from-env", "", "read the new value from this environment variable")
	fromStdin := fs.Bool("from-stdin", false, "read the new value from one line on stdin")
	fromClipboard := fs.Bool("from-clipboard", false, "read the new value from the OS clipboard, then wipe the clipboard")
	revokeCaps := fs.Bool("revoke-caps", false, "revoke all capabilities scoped to this alias")
	expiry := fs.String("expiry", "", "update the expiry reminder (YYYY-MM-DD, RFC3339, or a duration like 90d); omitted = keep the current one")
	ipWhitelist := fs.String("ip-whitelist", "", "update the client IP/CIDR allowlist for proxy use; empty value clears it; omitted = keep the current one")
	yes := fs.Bool("yes", false, "confirm the overwrite non-interactively")
	fs.BoolVar(yes, "y", false, "confirm the overwrite non-interactively (shorthand)")
	if err := fs.Parse(args); err != nil {
		return 1
	}
	if fs.NArg() < 1 {
		fmt.Fprintf(stderr, "error: usage: boru vault rotate [--from-env=VAR | --from-stdin | --from-clipboard | --revoke-caps] [--expiry=WHEN] [--ip-whitelist=IPs] [--yes] <alias>\n")
		return 1
	}
	expiresAt, err := parseExpiryFlag(*expiry)
	if err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}
	// --ip-whitelist is tri-state: absent = keep; present-and-empty =
	// clear; present-and-set = replace. fs.Visit distinguishes "absent"
	// from an explicit empty value.
	ipwlSet := false
	fs.Visit(func(f *flag.Flag) {
		if f.Name == "ip-whitelist" {
			ipwlSet = true
		}
	})
	var ipwl []string
	if ipwlSet {
		if ipwl, err = parseIPWhitelist(*ipWhitelist); err != nil {
			fmt.Fprintf(stderr, "error: %s\n", err)
			return 1
		}
		if ipwl == nil {
			ipwl = []string{} // explicit clear (non-nil empty = replace with none)
		}
	}
	s, err := requireStore(homeDir)
	if err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}
	if s.Locked {
		fmt.Fprintln(stderr, "error: vault is locked; run `boru vault unlock`")
		return 1
	}
	_, alias, err := w8findAliasRef(s, fs.Arg(0))
	if err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}
	if err := confirmDestructive(confirmTier1, alias, fmt.Sprintf("overwrite secret %q", alias), *yes, false, stdin, stdout); err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}

	value, source, err := readSecretValue(*fromEnv, *fromStdin, *fromClipboard, stdin, stdout)
	if err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}
	if value == "" {
		fmt.Fprintln(stderr, "error: empty value; refusing to rotate")
		return 1
	}

	ns, _ := splitAlias(alias)
	sess, err := authenticate(s, homeDir, stdin, stdout, "Vault passphrase: ")
	if err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}
	defer sess.Close()
	if err := requireScopeNS(sess, OpWrite, ns); err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}

	// When revoking capabilities (the incident-response path), persist
	// the revocation BEFORE the new value goes live. A crash between the
	// two then leaves the old capabilities dead and the old value still
	// in place — fail-closed — never the new value reachable by the very
	// capabilities the operator was trying to kill.
	revoked := 0
	if *revokeCaps {
		if err := mutateStore(homeDir, func(s *Store) error {
			for i := range s.Capabilities {
				// Cover, not equal: a namespace wildcard capability can read
				// this alias too, and leaving it live would let the very
				// token being rotated away fetch the replacement value.
				if capabilityCoversAlias(&s.Capabilities[i], alias) && !s.Capabilities[i].Revoked {
					s.Capabilities[i].Revoked = true
					revoked++
				}
			}
			return nil
		}); err != nil {
			fmt.Fprintf(stderr, "error: %s\n", err)
			return 1
		}
	}

	if err := writeSecret(homeDir, sess, alias, ns, value); err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}
	if err := mutateStore(homeDir, func(s *Store) error {
		a, _ := s.FindAlias(alias)
		if a == nil {
			return fmt.Errorf("no alias named %q", alias)
		}
		a.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
		a.Source = source
		// Only touch the expiry when --expiry was given; a bare rotation
		// preserves the existing reminder.
		if expiresAt != "" {
			a.ExpiresAt = expiresAt
		}
		// Same for the IP whitelist: absent = keep; --ip-whitelist given
		// (set or empty) replaces (a non-nil ipwl, incl. an empty slice).
		if ipwlSet {
			a.IPWhitelist = ipwl
		}
		return nil
	}); err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}
	sealIntegrity(homeDir, sess)
	_ = appendAudit(homeDir, AuditEvent{
		Action: "vault.rotate", Alias: alias, Outcome: "ok",
		Reason: fmt.Sprintf("source=%s revoked-caps=%d", source, revoked),
	})
	fmt.Fprintf(stdout, "rotated %s (backend=%s, %d bytes)", alias, sess.Keyring().Name(), len(value))
	if revoked > 0 {
		fmt.Fprintf(stdout, "; revoked %d capability(s)", revoked)
	}
	fmt.Fprintln(stdout)
	if *fromClipboard {
		clearClipboardAfterStore(stdout, stderr)
	}
	return 0
}

// --- input helpers ---------------------------------------------------------

func splitCSV(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := parts[:0]
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// wildcardAlias reports whether a stored capability binding is a
// namespace wildcard ("*" or "ns:*") rather than a concrete alias name.
// Bookkeeping that walks capabilities by alias (verify's dangling check,
// rotate's revocation sweep) must treat these as namespace bindings, not
// as references to an alias record that could exist.
func wildcardAlias(alias string) bool {
	_, base := splitAlias(alias)
	return base == "*"
}

// grantAliasRef resolves the target of `vault grant`: a concrete alias
// reference, or a namespace wildcard authorizing reads of one whole
// namespace through the wire protocol (`boru vault serve`). Wildcards
// follow the same resolution rule as names — a bare "*" means the
// active default namespace (root when none is set), ":*" forces root,
// "ns:*" is explicit — so `grant` sugar cannot drift from `add`/`get`.
// The proxy and MCP brokers resolve capabilities by exact alias name
// and no stored alias can contain '*' (validAlias rejects it), so a
// wildcard capability fails closed everywhere except the serve
// endpoint.
func grantAliasRef(s *Store, ref string) (alias string, wild bool, err error) {
	if !strings.HasSuffix(ref, "*") {
		_, alias, err = w8findAliasRef(s, ref)
		return alias, false, err
	}
	switch {
	case ref == "*":
		ns, err := defaultNamespace(s)
		if err != nil {
			return "", false, err
		}
		if ns == "" {
			return "*", true, nil
		}
		return ns + ":*", true, nil
	case ref == rootNamespaceRef+"*":
		return "*", true, nil
	}
	if ns, base := splitAlias(ref); base == "*" && validNamespaceName(ns) {
		return ref, true, nil
	}
	return "", false, fmt.Errorf("invalid wildcard alias %q (use '*' or ':*' for the root namespace, or 'ns:*' for namespace ns)", ref)
}

// validAlias accepts a stored alias name: either one segment of the
// conservative ASCII subset [A-Za-z0-9._-], or two such segments
// joined by exactly one ':' (a namespace-qualified name, see
// namespace.go). Total length 1..128. A leading ':' is CLI sugar for
// the root namespace and is never part of a stored name, so it is
// rejected here. The charset disallows shell metacharacters and
// whitespace and matches typical .env key shapes.
func validAlias(s string) bool {
	if s == "" || len(s) > 128 {
		return false
	}
	if i := strings.IndexByte(s, ':'); i >= 0 {
		return validAliasSegment(s[:i]) && validAliasSegment(s[i+1:])
	}
	return validAliasSegment(s)
}

// validAliasSegment accepts one colon-free name segment.
func validAliasSegment(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= 'A' && r <= 'Z':
		case r >= '0' && r <= '9':
		case r == '.' || r == '_' || r == '-':
		default:
			return false
		}
	}
	return true
}
