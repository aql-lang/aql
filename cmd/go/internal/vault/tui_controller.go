package vault

// tui_controller.go — the bridge between the bubbletea UI and the vault
// command layer.
//
// To guarantee the interactive TUI and the CLI can never drift, every
// MUTATING operation is driven through the very same Run() entry point the
// CLI uses: the collected passphrase is supplied via AQL_VAULT_PASSPHRASE,
// any typed value via stdin (--from-stdin), and --yes is set because the
// TUI performs its own typed confirmation before calling in. Pure read
// views either load the store directly (for the browsable, actionable
// lists) or capture a run* command's already-formatted output (for the
// informational panels) — both reuse the canonical implementation.
//
// The active vault is selected by promoting its folder/suffix into the
// AQL_VAULT_FOLDER / AQL_VAULT_SUFFIX environment, which is the single
// source of truth every path resolver reads (the same mechanism Run() uses
// to promote the --folder/--suffix flags). The UI drives one vault at a
// time on the bubbletea goroutine, so there is no concurrent env race.

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// tuiController holds the active vault location and the session passphrase.
type tuiController struct {
	homeDir string
	folder  string // active vault folder
	suffix  string // active vault suffix ("" == default)
	pass    string // session passphrase (empty until collected / not needed)
	authed  bool   // a passphrase has been validated this session
}

func newTUIController(homeDir string) *tuiController {
	return &tuiController{homeDir: homeDir}
}

// setActiveVault points the process path-resolvers at (folder, suffix) by
// promoting them into the env. Clears any cached auth, since a different
// vault needs its own passphrase.
func (c *tuiController) setActiveVault(folder, suffix string) {
	c.folder = filepath.Clean(folder)
	c.suffix = suffix
	_ = os.Setenv(EnvFolder, c.folder)
	_ = os.Setenv(EnvSuffix, suffix)
	c.pass = ""
	c.authed = false
}

// activeMetaPath is the metadata file path of the active vault, for display.
func (c *tuiController) activeMetaPath() string {
	return filepath.Join(c.folder, metaFileName(c.suffix))
}

// run drives a vault subcommand through Run() with the active vault's env,
// the session passphrase, and the given stdin. Returns stdout, stderr, code.
func (c *tuiController) run(stdin string, args ...string) (string, string, int) {
	if c.pass != "" {
		_ = os.Setenv(EnvPassphrase, c.pass)
		defer func() { _ = os.Unsetenv(EnvPassphrase) }()
	}
	var out, errb bytes.Buffer
	code := Run(args, strings.NewReader(stdin), &out, &errb)
	return out.String(), errb.String(), code
}

// cmdErr turns a non-zero run into an error, preferring stderr and falling
// back to stdout, trimmed of the "error: " prefix the CLI prints.
func cmdErr(stderr, stdout string) error {
	msg := strings.TrimSpace(stderr)
	if msg == "" {
		msg = strings.TrimSpace(stdout)
	}
	msg = strings.TrimPrefix(msg, "error: ")
	if msg == "" {
		msg = "command failed"
	}
	// Collapse to the first line so the UI status stays single-line.
	if i := strings.IndexByte(msg, '\n'); i >= 0 {
		msg = msg[:i]
	}
	return fmt.Errorf("%s", msg)
}

// --- vault lifecycle -------------------------------------------------------

// listVaults enumerates every known vault (scan ∪ index).
func (c *tuiController) listVaults() ([]discoveredVault, error) {
	return enumerateVaults(c.homeDir)
}

// switchVault makes (folder, suffix) the active vault, validating that it
// exists, and stamps its last-opened time.
func (c *tuiController) switchVault(folder, suffix string) error {
	c.setActiveVault(folder, suffix)
	s, err := LoadStore(c.homeDir)
	if err != nil {
		return err
	}
	if s == nil {
		return fmt.Errorf("no vault at %s", c.activeMetaPath())
	}
	_ = recordVaultOpened(c.homeDir, c.folder, suffix)
	return nil
}

// createVault initializes a new vault at (folder, suffix) with the given
// backend, then makes it active. It drives the same init path as the CLI,
// which records the vault in the index. folder defaults to the home ~/.aql.
func (c *tuiController) createVault(folder, suffix, backend, pass string) error {
	if folder == "" {
		folder = homeAQLDir(c.homeDir)
	}
	if pass != "" {
		_ = os.Setenv(EnvPassphrase, pass)
		defer func() { _ = os.Unsetenv(EnvPassphrase) }()
	}
	args := []string{"--folder=" + folder}
	if suffix != "" {
		args = append(args, "--suffix="+suffix)
	}
	args = append(args, "init", "--backend="+backend)
	var out, errb bytes.Buffer
	if code := Run(args, strings.NewReader(""), &out, &errb); code != 0 {
		return cmdErr(errb.String(), out.String())
	}
	c.setActiveVault(folder, suffix)
	c.pass = pass
	c.authed = pass != ""
	return nil
}

// setDefault marks the active vault as the launch default.
func (c *tuiController) setDefault() error {
	return setDefaultVault(c.homeDir, c.folder, c.suffix)
}

// pruneVault drops a stale (folder, suffix) from the index.
func (c *tuiController) pruneVault(folder, suffix string) error {
	return pruneVaultEntry(c.homeDir, folder, suffix)
}

// --- authentication --------------------------------------------------------

// needsPassphrase reports whether the active vault requires a passphrase to
// reach secrets/admin operations.
func (c *tuiController) needsPassphrase() (bool, error) {
	s, err := LoadStore(c.homeDir)
	if err != nil {
		return false, err
	}
	if s == nil {
		return false, nil
	}
	return vaultNeedsPassphrase(s), nil
}

// authenticate verifies pass actually opens the active vault, then caches
// it for subsequent operations. Verification is load-bearing: the TUI holds
// one passphrase for the whole session and never re-prompts once authed, so
// accepting a mistyped one here would wedge every later operation (and run a
// mutation like rename under the wrong key). verifyPassphrase checks an
// envelope vault against a keyslot AND a legacy file backend by decrypting
// its keyring — closing the gap where the file backend used to accept any
// passphrase and defer the failure to the first value read.
func (c *tuiController) authenticate(pass string) error {
	s, err := requireStore(c.homeDir)
	if err != nil {
		return err
	}
	if err := verifyPassphrase(s, c.homeDir, pass); err != nil {
		return err
	}
	c.pass = pass
	c.authed = true
	return nil
}

// --- status / browsable reads ---------------------------------------------

// vaultStatus is the header/status summary, read without authentication.
type vaultStatus struct {
	Backend    string
	Path       string
	Locked     bool
	Aliases    int
	Caps       int
	ActiveCaps int
	Agents     int
	HasSlots   bool
	Slots      int
	AdminSlots int
	Generation int64
}

// status loads the active vault's summary. ok is false when the vault is
// not initialized.
func (c *tuiController) status() (vaultStatus, bool, error) {
	s, err := LoadStore(c.homeDir)
	if err != nil {
		return vaultStatus{}, false, err
	}
	if s == nil {
		return vaultStatus{}, false, nil
	}
	return vaultStatus{
		Backend:    s.Backend,
		Path:       StorePath(c.homeDir),
		Locked:     s.Locked,
		Aliases:    len(s.Aliases),
		Caps:       len(s.Capabilities),
		ActiveCaps: len(s.ActiveCapabilities(time.Now())),
		Agents:     len(s.Agents),
		HasSlots:   s.HasPasswordSlots(),
		Slots:      len(s.Passwords),
		AdminSlots: adminSlotCount(s),
		Generation: s.Generation,
	}, true, nil
}

// listAliases returns the active vault's aliases, sorted (metadata only).
func (c *tuiController) listAliases() ([]Alias, error) {
	s, err := requireStore(c.homeDir)
	if err != nil {
		return nil, err
	}
	return s.SortedAliases(), nil
}

// alias returns one alias's metadata, or (Alias{}, false) if absent.
func (c *tuiController) alias(name string) (Alias, bool) {
	s, err := requireStore(c.homeDir)
	if err != nil {
		return Alias{}, false
	}
	if a, _ := s.FindAlias(name); a != nil {
		return *a, true
	}
	return Alias{}, false
}

// copyToClipboard sets the OS clipboard to text (non-secret helper text),
// returning the clipboard tool's label.
func (c *tuiController) copyToClipboard(text string) (string, error) {
	clip, err := t7detectClipboard(hostClipEnv())
	if err != nil {
		return "", err
	}
	if err := clip.copy(text); err != nil {
		return "", err
	}
	return clip.label, nil
}

// listCapabilities returns all capabilities and the set of currently-active
// IDs (neither revoked nor expired).
func (c *tuiController) listCapabilities() ([]Capability, map[string]bool, error) {
	s, err := requireStore(c.homeDir)
	if err != nil {
		return nil, nil, err
	}
	active := map[string]bool{}
	for _, c := range s.ActiveCapabilities(time.Now()) {
		active[c.ID] = true
	}
	caps := append([]Capability(nil), s.Capabilities...)
	return caps, active, nil
}

// listPasswordSlots returns the scoped password slots, sorted (no secrets).
func (c *tuiController) listPasswordSlots() ([]PasswordSlot, error) {
	s, err := requireStore(c.homeDir)
	if err != nil {
		return nil, err
	}
	return s.SortedPasswordSlots(), nil
}

// locked reports whether the active vault is locked.
func (c *tuiController) locked() (bool, error) {
	s, err := LoadStore(c.homeDir)
	if err != nil || s == nil {
		return false, err
	}
	return s.Locked, nil
}

// --- informational text panels (capture canonical run* output) ------------

func (c *tuiController) statusText() string { out, _, _ := c.run("", "status"); return out }
func (c *tuiController) configText() string { out, _, _ := c.run("", "config"); return out }
func (c *tuiController) providersText() string {
	out, _, _ := c.run("", "providers")
	return out
}
func (c *tuiController) historyText() string {
	out, errb, code := c.run("", "history")
	if code != 0 {
		return out + errb
	}
	return out
}

// auditText captures the audit log, optionally filtered.
func (c *tuiController) auditText(filterFlags ...string) string {
	args := append([]string{"audit"}, filterFlags...)
	out, errb, _ := c.run("", args...)
	if strings.TrimSpace(out) == "" {
		return strings.TrimSpace(errb)
	}
	return out
}

// verifyText runs verify/fsck (optionally repairing) and returns its report.
func (c *tuiController) verifyText(prune bool) (string, bool) {
	args := []string{"verify"}
	if prune {
		args = append(args, "--prune")
	}
	out, errb, code := c.run("", args...)
	return strings.TrimRight(out+errb, "\n"), code == 0
}

// scanText scans the given paths (defaults to ".") for leaked secrets.
func (c *tuiController) scanText(paths ...string) string {
	args := append([]string{"scan"}, paths...)
	out, errb, _ := c.run("", args...)
	return strings.TrimRight(out+errb, "\n")
}

// --- secret mutations (driven through Run for CLI parity) -----------------

func (c *tuiController) addSecret(alias, value, provider, namespace, expiry string) error {
	args := []string{"add", "--from-stdin", "--yes"}
	if provider != "" {
		args = append(args, "--provider="+provider)
	}
	if namespace != "" {
		args = append(args, "--namespace="+namespace)
	}
	if expiry != "" {
		args = append(args, "--expiry="+expiry)
	}
	args = append(args, alias)
	out, errb, code := c.run(value+"\n", args...)
	if code != 0 {
		return cmdErr(errb, out)
	}
	return nil
}

// revealSecret returns the cleartext secret for alias (the explicit reveal
// action; values are masked everywhere else).
func (c *tuiController) revealSecret(alias string) (string, error) {
	out, errb, code := c.run("", "get", "--reveal", alias)
	if code != 0 {
		return "", cmdErr(errb, out)
	}
	return strings.TrimRight(out, "\n"), nil
}

func (c *tuiController) removeSecret(alias string) error {
	out, errb, code := c.run("", "rm", "--yes", alias)
	if code != 0 {
		return cmdErr(errb, out)
	}
	return nil
}

func (c *tuiController) rotateSecret(alias, value string, revokeCaps bool, expiry string) error {
	args := []string{"rotate", "--from-stdin", "--yes"}
	if revokeCaps {
		args = append(args, "--revoke-caps")
	}
	if expiry != "" {
		args = append(args, "--expiry="+expiry)
	}
	args = append(args, alias)
	out, errb, code := c.run(value+"\n", args...)
	if code != 0 {
		return cmdErr(errb, out)
	}
	return nil
}

func (c *tuiController) renameSecret(from, to string, revokeCaps bool) error {
	args := []string{"mv"}
	if revokeCaps {
		args = append(args, "--revoke-caps")
	}
	args = append(args, from, to)
	out, errb, code := c.run("", args...)
	if code != 0 {
		return cmdErr(errb, out)
	}
	return nil
}

func (c *tuiController) setExpiry(alias, when string) error {
	out, errb, code := c.run("", "expiry", "set", alias, when)
	if code != 0 {
		return cmdErr(errb, out)
	}
	return nil
}

func (c *tuiController) clearExpiry(alias string) error {
	out, errb, code := c.run("", "expiry", "clear", alias)
	if code != 0 {
		return cmdErr(errb, out)
	}
	return nil
}

// --- capability mutations --------------------------------------------------

// grant mints a capability and returns the full grant output (which
// includes the one-time bearer token, shown once).
func (c *tuiController) grant(alias, agent, hosts, methods, ttl string, maxCalls, maxCostCents int, requireApproval bool) (string, error) {
	args := []string{"grant"}
	if agent != "" {
		args = append(args, "--agent="+agent)
	}
	if hosts != "" {
		args = append(args, "--hosts="+hosts)
	}
	if methods != "" {
		args = append(args, "--methods="+methods)
	}
	if ttl != "" {
		args = append(args, "--ttl="+ttl)
	}
	if maxCalls > 0 {
		args = append(args, fmt.Sprintf("--max-calls=%d", maxCalls))
	}
	if maxCostCents > 0 {
		args = append(args, fmt.Sprintf("--max-cost-cents=%d", maxCostCents))
	}
	if requireApproval {
		args = append(args, "--require-approval")
	}
	args = append(args, alias)
	out, errb, code := c.run("", args...)
	if code != 0 {
		return "", cmdErr(errb, out)
	}
	return out, nil
}

func (c *tuiController) revoke(id string) error {
	out, errb, code := c.run("", "revoke", id)
	if code != 0 {
		return cmdErr(errb, out)
	}
	return nil
}

// --- password slot mutations ----------------------------------------------

func (c *tuiController) passwordAdd(name, scope, namespaces, newPass, ttl string) error {
	_ = os.Setenv(EnvNewPassphrase, newPass)
	defer func() { _ = os.Unsetenv(EnvNewPassphrase) }()
	args := []string{"password", "add"}
	if scope != "" {
		args = append(args, "--scope="+scope)
	}
	if namespaces != "" {
		args = append(args, "--namespaces="+namespaces)
	}
	if ttl != "" {
		args = append(args, "--ttl="+ttl)
	}
	args = append(args, name)
	out, errb, code := c.run("", args...)
	if code != 0 {
		return cmdErr(errb, out)
	}
	return nil
}

func (c *tuiController) passwordRemove(name string) error {
	// --yes: the TUI form already ran its own typed confirmation
	// (buildPasswordRemoveForm), same as the rm/rotate/restore
	// mutations — without it the CLI's non-interactive confirm gate
	// refuses every removal.
	out, errb, code := c.run("", "password", "rm", "--yes", name)
	if code != 0 {
		return cmdErr(errb, out)
	}
	return nil
}

// passwordRevokeTemp revokes every temporary (expiring) password at once,
// mirroring `password rm --temp`. --yes: the TUI form already confirmed.
func (c *tuiController) passwordRevokeTemp() error {
	out, errb, code := c.run("", "password", "rm", "--temp", "--yes")
	if code != 0 {
		return cmdErr(errb, out)
	}
	return nil
}

// temporaryPasswordCount reports how many scoped passwords carry an
// expiry, so the TUI can label and gate the bulk-revoke action.
func (c *tuiController) temporaryPasswordCount() int {
	slots, err := c.listPasswordSlots()
	if err != nil {
		return 0
	}
	n := 0
	for _, p := range slots {
		if p.ExpiresAt != "" {
			n++
		}
	}
	return n
}

// --- maintenance / settings mutations -------------------------------------

func (c *tuiController) lock() error {
	out, errb, code := c.run("", "lock")
	if code != 0 {
		return cmdErr(errb, out)
	}
	return nil
}

func (c *tuiController) unlock() error {
	out, errb, code := c.run("", "unlock")
	if code != 0 {
		return cmdErr(errb, out)
	}
	return nil
}

func (c *tuiController) configSet(key, value string) error {
	out, errb, code := c.run("", "config", "--set", key+"="+value)
	if code != 0 {
		return cmdErr(errb, out)
	}
	return nil
}

func (c *tuiController) configUnset(key string) error {
	out, errb, code := c.run("", "config", "--unset", key)
	if code != 0 {
		return cmdErr(errb, out)
	}
	return nil
}

// restore rolls the vault metadata back to a past generation. The TUI does
// its own typed (generation-number) confirmation, so --yes is passed.
func (c *tuiController) restore(generation int64) error {
	out, errb, code := c.run("", "restore", fmt.Sprintf("--generation=%d", generation), "--yes")
	if code != 0 {
		return cmdErr(errb, out)
	}
	return nil
}
