package vault

// aqlbridge.go — the modules.VaultSpec adapter that lets the AQL vault
// TUI (the aql:vault-tui module) drive this package's command layer
// through the aql:vault bridge words (design/VAULT-TUI-PORT.0.md §3.4),
// plus the `aql vault -i --aql` launcher.
//
// Every op runs under one mutex: the controller promotes the active
// vault and the cached passphrase into the PROCESS environment per op —
// safe on the bubbletea goroutine, and made safe here because the AQL
// app may call from spawned worker processes.

import (
	"fmt"
	"io"
	"sort"
	"strings"
	"sync"

	"github.com/aql-lang/aql/cmd/go/internal/termback"
	lang "github.com/aql-lang/aql/lang/go"
	"github.com/aql-lang/aql/lang/go/modules"
	"github.com/aql-lang/aql/lang/go/native"
	"github.com/charmbracelet/lipgloss"
)

type aqlBridge struct {
	mu  sync.Mutex
	ctl *tuiController
}

func newAqlBridge(ctl *tuiController) *aqlBridge { return &aqlBridge{ctl: ctl} }

// spec is the host seam handed to modules.RegisterHostVault.
func (b *aqlBridge) spec() modules.VaultSpec {
	return modules.VaultSpec{Name: "cli", Do: b.do}
}

// --- param readers (the lang side validated shapes; these just read) ---

func bpStr(params map[string]any, k string) string {
	s, _ := params[k].(string)
	return s
}

func bpBool(params map[string]any, k string) bool {
	v, _ := params[k].(bool)
	return v
}

func bpInt(params map[string]any, k string) int {
	switch v := params[k].(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	}
	return 0
}

func bpStrList(params map[string]any, k string) []string {
	items, _ := params[k].([]any)
	out := make([]string, 0, len(items))
	for _, it := range items {
		if s, ok := it.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

// --- result shapers ---

func aliasMap(a Alias) map[string]any {
	return map[string]any{
		"name": a.Name, "provider": a.Provider, "namespace": a.Namespace,
		"source": a.Source, "created-at": a.CreatedAt, "updated-at": a.UpdatedAt,
		"expires-at": a.ExpiresAt,
	}
}

func capabilityMap(c Capability) map[string]any {
	// TokenHash stays host-side: the AQL app has no use for it and the
	// hygiene rule is to ship nothing secret-adjacent that isn't shown.
	return map[string]any{
		"id": c.ID, "alias": c.Alias, "agent": c.Agent,
		"hosts": strings.Join(c.Hosts, ","), "methods": strings.Join(c.Methods, ","),
		"created-at": c.CreatedAt, "expires-at": c.ExpiresAt,
		"revoked": c.Revoked, "max-calls": c.MaxCalls, "used-calls": c.UsedCalls,
		"max-cost-cents": c.MaxCostCents, "used-cost-cents": c.UsedCostCents,
		"require-approval": c.RequireApproval,
	}
}

func passwordSlotMap(p PasswordSlot) map[string]any {
	// Metadata only — never the salts, verifiers, or wrapped keys.
	return map[string]any{
		"name": p.Name, "scope": p.Scope, "namespaces": strings.Join(p.Namespaces, ","),
		"created-at": p.CreatedAt, "updated-at": p.UpdatedAt, "expires-at": p.ExpiresAt,
	}
}

// execPrefix mirrors rootModel.execPrefix for the recipes op: "aql
// vault" plus the active vault's location flags.
func (b *aqlBridge) execPrefix() string {
	var fl []string
	if b.ctl.folder != homeAQLDir(b.ctl.homeDir) {
		fl = append(fl, "--folder="+shortenHome(b.ctl.folder, b.ctl.homeDir))
	}
	if b.ctl.suffix != "" {
		fl = append(fl, "--suffix="+b.ctl.suffix)
	}
	if len(fl) == 0 {
		return "aql vault"
	}
	return "aql vault " + strings.Join(fl, " ")
}

// do executes one bridge op against the controller. The op names are
// the aql:vault word names; params carry the lang-side-validated
// arguments, map-shaped args flattened by key.
func (b *aqlBridge) do(op string, params map[string]any) (any, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	c := b.ctl
	switch op {
	// --- session ---
	case "status":
		st, ok, err := c.status()
		if err != nil {
			return nil, err
		}
		m := map[string]any{
			"ok": ok, "folder": c.folder, "suffix": c.suffix,
			"exec-prefix": b.execPrefix(),
		}
		if ok {
			m["backend"] = st.Backend
			m["path"] = st.Path
			m["locked"] = st.Locked
			m["aliases"] = st.Aliases
			m["caps"] = st.Caps
			m["active-caps"] = st.ActiveCaps
			m["agents"] = st.Agents
			m["has-slots"] = st.HasSlots
			m["slots"] = st.Slots
			m["admin-slots"] = st.AdminSlots
			m["generation"] = st.Generation
		}
		return m, nil
	case "needs-passphrase":
		need, err := c.needsPassphrase()
		if err != nil {
			return nil, err
		}
		return need, nil
	case "authed":
		return c.authed, nil
	case "authenticate":
		if err := c.authenticate(bpStr(params, "pass")); err != nil {
			return nil, err
		}
		return true, nil
	case "status-text":
		return c.statusText(), nil
	case "providers-text":
		return c.providersText(), nil
	case "config-text":
		return c.configText(), nil

	// --- secrets ---
	case "secrets":
		aliases, err := c.listAliases()
		if err != nil {
			return nil, err
		}
		rows := make([]any, 0, len(aliases))
		for _, a := range aliases {
			rows = append(rows, aliasMap(a))
		}
		return rows, nil
	case "secret":
		if a, ok := c.alias(bpStr(params, "alias")); ok {
			return aliasMap(a), nil
		}
		return nil, nil
	case "reveal":
		return c.revealSecret(bpStr(params, "alias"))
	case "add":
		return nil, c.addSecret(bpStr(params, "alias"), bpStr(params, "value"),
			bpStr(params, "provider"), bpStr(params, "namespace"), bpStr(params, "expiry"))
	case "rotate":
		return nil, c.rotateSecret(bpStr(params, "alias"), bpStr(params, "value"),
			bpBool(params, "revoke-caps"), bpStr(params, "expiry"))
	case "remove":
		return nil, c.removeSecret(bpStr(params, "alias"))
	case "rename":
		return nil, c.renameSecret(bpStr(params, "from"), bpStr(params, "to"),
			bpBool(params, "revoke-caps"))
	case "set-expiry":
		return nil, c.setExpiry(bpStr(params, "alias"), bpStr(params, "when"))
	case "clear-expiry":
		return nil, c.clearExpiry(bpStr(params, "alias"))
	case "recipes":
		alias := bpStr(params, "alias")
		provider := ""
		if a, ok := c.alias(alias); ok {
			provider = a.Provider
		}
		cmds := injectCommands(b.execPrefix(), alias, provider)
		rows := make([]any, 0, len(cmds))
		for _, ic := range cmds {
			rows = append(rows, map[string]any{"cmd": ic.cmd, "desc": ic.desc})
		}
		return rows, nil

	// --- access ---
	case "capabilities":
		caps, active, err := c.listCapabilities()
		if err != nil {
			return nil, err
		}
		rows := make([]any, 0, len(caps))
		for _, cp := range caps {
			m := capabilityMap(cp)
			m["active"] = active[cp.ID]
			rows = append(rows, m)
		}
		activeIDs := make([]any, 0, len(active))
		for id := range active {
			activeIDs = append(activeIDs, id)
		}
		sort.Slice(activeIDs, func(i, j int) bool { return activeIDs[i].(string) < activeIDs[j].(string) })
		return map[string]any{"caps": rows, "active": activeIDs}, nil
	case "grant":
		return c.grant(bpStr(params, "alias"), bpStr(params, "agent"),
			bpStr(params, "hosts"), bpStr(params, "methods"), bpStr(params, "ttl"),
			bpInt(params, "max-calls"), bpInt(params, "max-cost-cents"),
			bpBool(params, "require-approval"))
	case "revoke":
		return nil, c.revoke(bpStr(params, "id"))

	// --- passwords ---
	case "passwords":
		slots, err := c.listPasswordSlots()
		if err != nil {
			return nil, err
		}
		rows := make([]any, 0, len(slots))
		for _, p := range slots {
			rows = append(rows, passwordSlotMap(p))
		}
		return rows, nil
	case "password-add":
		return nil, c.passwordAdd(bpStr(params, "name"), bpStr(params, "scope"),
			bpStr(params, "namespaces"), bpStr(params, "pass"), bpStr(params, "ttl"))
	case "password-remove":
		return nil, c.passwordRemove(bpStr(params, "name"))
	case "password-revoke-temp":
		return nil, c.passwordRevokeTemp()
	case "temp-password-count":
		return c.temporaryPasswordCount(), nil

	// --- maintenance ---
	case "verify":
		text, ok := c.verifyText(bpBool(params, "prune"))
		return map[string]any{"text": text, "ok": ok}, nil
	case "scan":
		paths := bpStrList(params, "paths")
		if len(paths) == 0 {
			paths = []string{"."}
		}
		return c.scanText(paths...), nil
	case "history":
		return c.historyText(), nil
	case "restore":
		return nil, c.restore(int64(bpInt(params, "generation")))
	case "audit":
		// filter keys become audit CLI flags, sorted for determinism
		keys := make([]string, 0, len(params))
		for k := range params {
			if _, ok := params[k].(string); ok {
				keys = append(keys, k)
			}
		}
		sort.Strings(keys)
		flags := make([]string, 0, len(keys))
		for _, k := range keys {
			flags = append(flags, "--"+k+"="+bpStr(params, k))
		}
		return c.auditText(flags...), nil

	// --- settings ---
	case "config-set":
		return nil, c.configSet(bpStr(params, "key"), bpStr(params, "value"))
	case "config-unset":
		return nil, c.configUnset(bpStr(params, "key"))
	case "lock":
		return nil, c.lock()
	case "unlock":
		return nil, c.unlock()
	case "locked":
		locked, err := c.locked()
		if err != nil {
			return nil, err
		}
		return locked, nil
	case "prefs":
		return map[string]any{"theme": loadTUIPrefs(c.homeDir).Theme}, nil
	case "set-prefs":
		return nil, saveTUIPrefs(c.homeDir, tuiPrefs{Theme: bpStr(params, "theme")})

	// --- multi-vault ---
	case "vaults":
		// enumerateVaults never errors (a corrupt index degrades to empty)
		vaults, _ := c.listVaults()
		rows := make([]any, 0, len(vaults))
		for _, v := range vaults {
			rows = append(rows, map[string]any{
				"folder": v.Folder, "suffix": v.Suffix, "label": v.Label,
				"backend": v.Backend, "default": v.Default,
				"exists": v.Exists, "locked": v.Locked,
				"last-opened-at": v.LastOpenedAt,
				"active":         v.Folder == c.folder && v.Suffix == c.suffix,
			})
		}
		return rows, nil
	case "switch":
		return nil, c.switchVault(bpStr(params, "folder"), bpStr(params, "suffix"))
	case "create":
		return nil, c.createVault(bpStr(params, "folder"), bpStr(params, "suffix"),
			bpStr(params, "backend"), bpStr(params, "pass"))
	case "set-default":
		return nil, c.setDefault()
	case "prune-index":
		return nil, c.pruneVault(bpStr(params, "folder"), bpStr(params, "suffix"))

	// --- misc ---
	case "copy":
		return c.copyToClipboard(bpStr(params, "text"))
	}
	return nil, fmt.Errorf("unknown vault op %q", op)
}

// --- the `aql vault -i --aql` launcher ---

// aqlTuiSource is the two-line program the launcher runs: import the
// embedded app module and run it, DROPPING the returned final state so
// nothing (least of all a revealed value that leaked into state) ever
// prints to the residual stack (VAULT-TUI-PORT.0.md §5.2).
const aqlTuiSource = `import "aql:vault-tui"  (VaultTui.run {dark: %v}) drop`

// b8langNew / b8runSource / b8termbackSpec seam lang.New, the program
// run, and the real-TTY backend for the launcher's arms
// (design/TEST-SEAMS.10.md — a test cannot open a real terminal).
var (
	b8langNew      = lang.New
	b8runSource    = func(a *lang.AQL, src string) error { _, err := a.RunInterp(src); return err }
	b8termbackSpec = termback.Spec
)

// runInteractiveAQL launches the AQL implementation of the vault TUI
// (design/VAULT-TUI-PORT.0.md §1.3): the real-TTY terminal backend and
// this package's vault bridge are registered as host seams, then the
// embedded aql:vault-tui app runs under Tui.run until quit.
func runInteractiveAQL(homeDir string, stdin io.Reader, stdout, stderr io.Writer) int {
	if !interactiveTTY(stdin, stdout) {
		fmt.Fprintln(stderr, "error: the interactive vault TUI requires a terminal (run `aql vault -i` in an interactive shell)")
		return 1
	}
	ctl := newTUIController(homeDir)
	folder, suffix, ok := resolveLaunchVault(homeDir)
	if ok {
		ctl.setActiveVault(folder, suffix)
		_ = recordVaultOpened(homeDir, folder, suffix)
	} else {
		ctl.setActiveVault(homeAQLDir(homeDir), "")
	}

	a, err := b8langNew(lang.Options{})
	if err != nil {
		fmt.Fprintf(stderr, "aql-tui: %s\n", err)
		return 1
	}
	reg := a.NativeRegistry()
	registerAqlTuiBackends(reg, ctl)
	if err := b8runSource(a, fmt.Sprintf(aqlTuiSource, lipgloss.HasDarkBackground())); err != nil {
		fmt.Fprintf(stderr, "aql-tui: %s\n", err)
		return 1
	}
	return 0
}

// registerAqlTuiBackends wires the two host seams the app dispatches
// against. Registration MUST precede the program's import — the module
// sub-registry snapshots the capability slots at import time.
func registerAqlTuiBackends(reg *native.Registry, ctl *tuiController) {
	_ = modules.RegisterHostTui(reg, b8termbackSpec())
	_ = modules.RegisterHostVault(reg, newAqlBridge(ctl).spec())
}
