package vault

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/aql-lang/aql/cmd/go/internal/auth"
	"github.com/aql-lang/aql/cmd/go/internal/command"
)

// Env names recognised by every mode.
const (
	EnvPassphrase = "AQL_VAULT_PASSPHRASE"
	EnvHome       = "AQL_HOME" // used in tests; overrides os.UserHomeDir
)

type cmd struct{}

// New returns the vault subcommand.
func New() command.Command { return &cmd{} }

func (*cmd) Name() string     { return "vault" }
func (*cmd) Synopsis() string { return "manage a local key vault (init, add, get, list, grant, ...)" }
func (*cmd) Run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	return Run(args, stdin, stdout, stderr)
}

// Run handles `aql vault <mode> [args...]`.
func Run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		printUsage(stderr)
		return 1
	}
	mode, rest := args[0], args[1:]
	homeDir, err := homeDir()
	if err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}

	switch mode {
	case "help", "-h", "--help":
		printUsage(stdout)
		return 0
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
	case "rm", "remove", "delete":
		return runRemove(rest, homeDir, stdout, stderr)
	case "mv", "rename":
		return runMv(rest, homeDir, stdin, stdout, stderr)
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
	case "providers":
		return runProviders(stdout)
	case "scan":
		return runScan(rest, homeDir, stdout, stderr)
	case "audit":
		return runAudit(rest, homeDir, stdout, stderr)
	case "rotate":
		return runRotate(rest, homeDir, stdin, stdout, stderr)
	case "policy":
		return runPolicy(rest, homeDir, stdin, stdout, stderr)
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
	fmt.Fprintln(w, "Usage: aql vault <mode> [args...]")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Modes:")
	for _, m := range modeDocs {
		fmt.Fprintf(w, "  %-10s %s\n", m.name, m.summary)
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Backends: auto (default), keychain, secret-service, wincred, file, 1password.")
	fmt.Fprintln(w, "Passphrases are prompted interactively (hidden); set AQL_VAULT_PASSPHRASE only for non-interactive use.")
	fmt.Fprintln(w, "Namespaces: qualify aliases as ns:name; bare names use the default namespace")
	fmt.Fprintln(w, "  (vault config --set namespace.default=NS, or AQL_VAULT_NAMESPACE; ':' = root, :name forces root).")
}

type modeDoc struct{ name, summary string }

var modeDocs = []modeDoc{
	{"init", "initialize the vault; choose a backend"},
	{"status", "print backend, secret count, lock state"},
	{"add", "store a secret under an alias"},
	{"get", "retrieve a secret (redacted unless --reveal)"},
	{"list", "list aliases and metadata (no values)"},
	{"rm", "remove a secret"},
	{"mv", "rename a key, move it between namespaces, or rename a namespace (ns:)"},
	{"import", "import secrets from a .env file or an encrypted export bundle"},
	{"export", "export secrets to a portable, passphrase-encrypted bundle"},
	{"grant", "issue a scoped capability token for an alias"},
	{"revoke", "revoke a capability token"},
	{"lock", "mark the vault locked (block get/grant)"},
	{"unlock", "mark the vault unlocked"},
	{"config", "view or set vault configuration"},
	{"proxy", "run a local credential broker for agents and tools"},
	{"providers", "list built-in provider presets"},
	{"scan", "scan files for leaked secret-like strings"},
	{"audit", "show the structured audit log"},
	{"rotate", "replace a stored secret value, optionally revoking caps"},
	{"policy", "declaratively apply / show vault aliases and capabilities"},
	{"mcp", "run a stdio MCP server exposing aliases as tools"},
	{"exec", "run a command with secrets injected as env vars"},
}

// --- shared helpers --------------------------------------------------------

// homeDir resolves the directory holding ~/.aql, honoring AQL_HOME
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
		return nil, errors.New("vault not initialized; run `aql vault init`")
	}
	return s, nil
}

// openKeyring resolves the backend recorded in s and prompts for
// the file passphrase when needed. The passphrase is sourced from
// AQL_VAULT_PASSPHRASE if set, then from stdin (echo suppressed)
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
		return selectKeyring(backend, fileDir(homeDir), "")
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
		return nil, errors.New("file backend requires a passphrase; run interactively to be prompted, or set AQL_VAULT_PASSPHRASE for non-interactive use")
	}
	return selectKeyring(BackendFile, fileDir(homeDir), pass)
}

func fileDir(homeDir string) string {
	return filepath.Join(homeDir, ".aql")
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
	// it is read from AQL_VAULT_PASSPHRASE if set, otherwise prompted
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
			fmt.Fprintln(stderr, "error: empty passphrase — the file keyring would be effectively unencrypted (its salt is stored alongside the ciphertext, so anyone who can read ~/.aql/vault.keyring could recover every secret). Choose a non-empty passphrase, or set AQL_VAULT_PASSPHRASE.")
			return 1
		}
		// Initialize an empty keyring file so its presence and
		// passphrase are validated immediately.
		kr := &fileKeyring{dir: fileDir(homeDir), pass: pass}
		if err := kr.save(map[string]string{}); err != nil {
			fmt.Fprintf(stderr, "error: writing keyring: %s\n", err)
			return 1
		}
	} else {
		// Probe the host backend so we fail loudly here, not later
		// during the first `vault add`.
		if _, err := selectKeyring(chosen, fileDir(homeDir), ""); err != nil {
			fmt.Fprintf(stderr, "error: %s\n", err)
			return 1
		}
	}

	s := &Store{
		Version: storeVersion,
		Backend: chosen,
		Config:  map[string]any{},
	}
	if err := SaveStore(homeDir, s); err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}
	_ = appendAudit(homeDir, AuditEvent{Action: "vault.init", Outcome: "ok", Reason: "backend=" + chosen})
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
		fmt.Fprintln(stdout, "vault: not initialized (run `aql vault init`)")
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
	return 0
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
	if err := fs.Parse(args); err != nil {
		return 1
	}
	if fs.NArg() < 1 {
		fmt.Fprintf(stderr, "error: usage: aql vault add [--from-env=VAR | --from-stdin | --from-clipboard | --provider=...] [--namespace=NS] <[ns:]alias>\n")
		return 1
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
		fmt.Fprintln(stderr, "error: vault is locked; run `aql vault unlock`")
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

	kr, err := openKeyring(s, homeDir, stdin, stdout, "Vault passphrase: ")
	if err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}
	if err := kr.Set(alias, value); err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}
	ns, _ := splitAlias(alias)
	s.UpsertAlias(Alias{
		Name:      alias,
		Provider:  *provider,
		Namespace: ns, // derived from the name so the two can't disagree
		Source:    source,
	})
	if err := SaveStore(homeDir, s); err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}
	_ = appendAudit(homeDir, AuditEvent{
		Action: "vault.add", Alias: alias, Provider: *provider, Outcome: "ok",
		Reason: "source=" + source,
	})
	fmt.Fprintf(stdout, "stored %s (backend=%s, %d bytes)\n", alias, kr.Name(), len(value))
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
		fmt.Fprintf(stderr, "error: usage: aql vault get [--reveal] <alias>\n")
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
	_, alias, err := findAliasRef(s, fs.Arg(0))
	if err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}
	kr, err := openKeyring(s, homeDir, os.Stdin, stdout, "Vault passphrase: ")
	if err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}
	v, err := kr.Get(alias)
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
	fmt.Fprintf(stdout, "%-24s %-12s %-16s %-20s %s\n", "ALIAS", "PROVIDER", "NAMESPACE", "CREATED", "SOURCE")
	for _, a := range aliases {
		if nsFiltered && aliasNamespace(a) != nsFilter {
			continue
		}
		fmt.Fprintf(stdout, "%-24s %-12s %-16s %-20s %s\n",
			a.Name, dash(a.Provider), dash(aliasNamespace(a)), a.CreatedAt, dash(a.Source))
	}
	return 0
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
	if err := fs.Parse(args); err != nil {
		return 1
	}
	if fs.NArg() < 1 {
		fmt.Fprintf(stderr, "error: usage: aql vault rm <alias>\n")
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
	kr, err := openKeyring(s, homeDir, os.Stdin, stdout, "Vault passphrase: ")
	if err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}
	if err := kr.Delete(alias); err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}
	s.RemoveAlias(alias)
	if err := SaveStore(homeDir, s); err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}
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
// AQL_VAULT_EXPORT_PASSPHRASE are bundle-only.
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
		srcName = fs.Arg(0)
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
		fmt.Fprintln(stderr, "error: vault is locked; run `aql vault unlock`")
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
	kr, err := openKeyring(s, homeDir, krStdin, stdout, "Vault passphrase: ")
	if err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}
	for _, k := range sortedKeys(entries) {
		alias := qualify(prefix + k)
		if !validAlias(alias) {
			fmt.Fprintf(stderr, "warning: skipping invalid alias %q\n", alias)
			continue
		}
		if err := kr.Set(alias, entries[k]); err != nil {
			fmt.Fprintf(stderr, "error: storing %s: %s\n", alias, err)
			return 1
		}
		s.UpsertAlias(Alias{
			Name:      alias,
			Provider:  provider,
			Namespace: effNS,
			Source:    "import:" + srcName,
		})
		_ = appendAudit(homeDir, AuditEvent{
			Action: "vault.import", Alias: alias, Provider: provider,
			Outcome: "ok", Reason: "source=" + srcName,
		})
		fmt.Fprintf(stdout, "imported %s\n", alias)
	}
	if err := SaveStore(homeDir, s); err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}
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
	maxCostCents := fs.Int("max-cost-cents", 0, "max total cost in cents from X-AQL-Vault-Cost-Cents (0 = unlimited)")
	approval := fs.Bool("require-approval", false, "advisory: proxy will deny until a human flips this off")
	if err := fs.Parse(args); err != nil {
		return 1
	}
	if fs.NArg() < 1 {
		fmt.Fprintf(stderr, "error: usage: aql vault grant [--agent=NAME] [--hosts=H,H] [--methods=GET,POST] [--ttl=2h] [--max-calls=N] [--max-cost-cents=N] [--require-approval] <alias>\n")
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
	_, alias, err := findAliasRef(s, fs.Arg(0))
	if err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}
	_, token, err := s.NewCapability(alias, *agent, splitCSV(*hosts), splitCSV(*methods), *ttl)
	if err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}
	idx := len(s.Capabilities) - 1
	s.Capabilities[idx].MaxCalls = *maxCalls
	s.Capabilities[idx].MaxCostCents = *maxCostCents
	s.Capabilities[idx].RequireApproval = *approval
	tok := &s.Capabilities[idx]
	if err := SaveStore(homeDir, s); err != nil {
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
		fmt.Fprintf(stderr, "error: usage: aql vault revoke <capability-id>\n")
		return 1
	}
	id := fs.Arg(0)
	s, err := requireStore(homeDir)
	if err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}
	c, idx := s.FindCapability(id)
	if c == nil {
		fmt.Fprintf(stderr, "error: no capability matching %q\n", id)
		return 1
	}
	s.Capabilities[idx].Revoked = true
	if err := SaveStore(homeDir, s); err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}
	_ = appendAudit(homeDir, AuditEvent{
		Action: "vault.revoke", Capability: s.Capabilities[idx].ID,
		Alias: s.Capabilities[idx].Alias, Outcome: "ok",
	})
	fmt.Fprintf(stdout, "revoked %s\n", s.Capabilities[idx].ID)
	return 0
}

// --- lock / unlock ---------------------------------------------------------

func runLock(homeDir string, stdout, stderr io.Writer) int {
	s, err := requireStore(homeDir)
	if err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}
	s.Locked = true
	if err := SaveStore(homeDir, s); err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}
	_ = appendAudit(homeDir, AuditEvent{Action: "vault.lock", Outcome: "ok"})
	fmt.Fprintln(stdout, "vault locked")
	return 0
}

func runUnlock(homeDir string, stdout, stderr io.Writer) int {
	s, err := requireStore(homeDir)
	if err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}
	s.Locked = false
	if err := SaveStore(homeDir, s); err != nil {
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
	s, err := requireStore(homeDir)
	if err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}
	if s.Config == nil {
		s.Config = map[string]any{}
	}
	switch {
	case *set != "":
		eq := strings.IndexByte(*set, '=')
		if eq < 0 {
			fmt.Fprintln(stderr, "error: --set requires key=value")
			return 1
		}
		s.Config[(*set)[:eq]] = (*set)[eq+1:]
		if err := SaveStore(homeDir, s); err != nil {
			fmt.Fprintf(stderr, "error: %s\n", err)
			return 1
		}
		fmt.Fprintf(stdout, "set %s=%s\n", (*set)[:eq], (*set)[eq+1:])
		return 0
	case *unset != "":
		delete(s.Config, *unset)
		if err := SaveStore(homeDir, s); err != nil {
			fmt.Fprintf(stderr, "error: %s\n", err)
			return 1
		}
		fmt.Fprintf(stdout, "unset %s\n", *unset)
		return 0
	default:
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
	if err := fs.Parse(args); err != nil {
		return 1
	}
	if fs.NArg() < 1 {
		fmt.Fprintf(stderr, "error: usage: aql vault rotate [--from-env=VAR | --from-stdin | --from-clipboard | --revoke-caps] <alias>\n")
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
	a, alias, err := findAliasRef(s, fs.Arg(0))
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
		fmt.Fprintln(stderr, "error: empty value; refusing to rotate")
		return 1
	}

	kr, err := openKeyring(s, homeDir, stdin, stdout, "Vault passphrase: ")
	if err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}
	if err := kr.Set(alias, value); err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}
	a.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	a.Source = source

	revoked := 0
	if *revokeCaps {
		for i := range s.Capabilities {
			if s.Capabilities[i].Alias == alias && !s.Capabilities[i].Revoked {
				s.Capabilities[i].Revoked = true
				revoked++
			}
		}
	}
	if err := SaveStore(homeDir, s); err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}
	_ = appendAudit(homeDir, AuditEvent{
		Action: "vault.rotate", Alias: alias, Outcome: "ok",
		Reason: fmt.Sprintf("source=%s revoked-caps=%d", source, revoked),
	})
	fmt.Fprintf(stdout, "rotated %s (backend=%s, %d bytes)", alias, kr.Name(), len(value))
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

// runProviders prints the built-in provider presets. Aliases tag
// themselves with one of these via `vault add --provider=...` so
// the proxy knows how to attach credentials to outbound requests.
func runProviders(stdout io.Writer) int {
	fmt.Fprintf(stdout, "%-12s %-32s %s\n", "NAME", "BASE-URL", "AUTH-STYLE")
	for _, p := range ListProviders() {
		base := p.BaseURL
		if base == "" {
			base = "-"
		}
		fmt.Fprintf(stdout, "%-12s %-32s %s\n", p.Name, base, p.AuthStyle)
	}
	return 0
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
