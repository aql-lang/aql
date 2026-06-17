package vault

// tui_build.go — constructors that assemble concrete vault TUI screens from
// the generic screen types, wiring the controller's operations behind each
// action key. Also the ':' palette resolver and the passphrase-gating helper
// that prompts (only when the CLI would) before a secret/admin operation.

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
)

// --- per-screen action key bindings ---------------------------------------

var (
	kReveal   = key.NewBinding(key.WithKeys("g", "enter"), key.WithHelp("g", "reveal"))
	kAdd      = key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "add"))
	kRotate   = key.NewBinding(key.WithKeys("R"), key.WithHelp("R", "rotate"))
	kRemove   = key.NewBinding(key.WithKeys("D"), key.WithHelp("D", "delete"))
	kRename   = key.NewBinding(key.WithKeys("m"), key.WithHelp("m", "rename"))
	kExpiry   = key.NewBinding(key.WithKeys("e"), key.WithHelp("e", "expiry"))
	kGrant    = key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "grant"))
	kRevoke   = key.NewBinding(key.WithKeys("D"), key.WithHelp("D", "revoke"))
	kPwAdd    = key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "add"))
	kPwRemove = key.NewBinding(key.WithKeys("D"), key.WithHelp("D", "remove"))
	kNewVault = key.NewBinding(key.WithKeys("n"), key.WithHelp("n", "new"))
	kSetDef   = key.NewBinding(key.WithKeys("d"), key.WithHelp("d", "default"))
	kPrune    = key.NewBinding(key.WithKeys("D"), key.WithHelp("D", "prune"))
	kPrune2   = key.NewBinding(key.WithKeys("p"), key.WithHelp("p", "prune"))
	kRestore  = key.NewBinding(key.WithKeys("R"), key.WithHelp("R", "restore"))
	kSet      = key.NewBinding(key.WithKeys("s"), key.WithHelp("s", "set"))
	kUnset    = key.NewBinding(key.WithKeys("x"), key.WithHelp("x", "unset"))
)

// requireAuth runs do, first prompting for the vault passphrase when the
// active vault needs one and it has not yet been collected this session.
func (m *rootModel) requireAuth(do func() tea.Cmd) tea.Cmd {
	need, err := m.ctl.needsPassphrase()
	if err != nil {
		return statusErr(err.Error())
	}
	if !need || m.ctl.authed {
		return do()
	}
	return pushScreen(m.buildPassphraseForm(do))
}

func (m *rootModel) buildPassphraseForm(do func() tea.Cmd) screen {
	var pass string
	form := huh.NewForm(huh.NewGroup(
		huh.NewInput().
			Title("Vault passphrase").
			Description("unlock " + m.ctl.activeMetaPath()).
			EchoMode(huh.EchoModePassword).
			Value(&pass).
			Validate(func(s string) error {
				if s == "" {
					return fmt.Errorf("passphrase required")
				}
				return m.ctl.authenticate(s) // wrong passphrase -> error shown inline
			}),
	))
	return newFormScreen("unlock", form, func() tea.Cmd {
		// Validation already authenticated; close the prompt and run the
		// gated action.
		return tea.Sequence(popScreen(), do())
	})
}

// --- Home ------------------------------------------------------------------

func (m *rootModel) buildHome() screen {
	items := []list.Item{
		listItem{name: "Secrets", desc: "browse & manage stored secrets (add, reveal, rotate, remove, rename, expiry)", act: func() tea.Cmd { return pushScreen(m.buildSecrets()) }},
		listItem{name: "Access", desc: "capabilities: grant scoped tokens, revoke them", act: func() tea.Cmd { return pushScreen(m.buildAccess()) }},
		listItem{name: "Passwords", desc: "scoped vault passwords (add, remove, list)", act: func() tea.Cmd { return pushScreen(m.buildPasswords()) }},
		listItem{name: "Maintenance", desc: "verify, scan, history & restore, audit log", act: func() tea.Cmd { return pushScreen(m.buildMaintenance()) }},
		listItem{name: "Settings", desc: "config, lock/unlock, providers", act: func() tea.Cmd { return pushScreen(m.buildSettings()) }},
	}
	return newListScreen("", items, nil, nil).
		withHelp(`Pick an area and press enter to open it. Press ? on any screen for detailed
help about that screen's actions.

  Secrets       store, reveal, rotate, rename, remove, set expiry reminders
  Access        grant and revoke capability tokens for the proxy
  Passwords     scoped vault passwords (keyslots)
  Maintenance   verify, scan, history & restore, audit log
  Settings      config, lock/unlock, theme, providers`)
}

// --- Vault picker / switcher ----------------------------------------------

func (m *rootModel) buildVaultPicker() screen {
	build := func() []list.Item { return m.vaultItems() }
	acts := []listKeyAction{
		{binding: kNewVault, run: func(_ listItem) tea.Cmd { return pushScreen(m.buildCreateVaultForm()) }},
		{binding: kSetDef, run: func(sel listItem) tea.Cmd {
			folder, suffix := splitVaultKey(sel.filter)
			return submitOpNoPop("set "+vaultLabel(sel)+" as default", func() error {
				cur, curS := m.ctl.folder, m.ctl.suffix
				m.ctl.setActiveVault(folder, suffix)
				err := m.ctl.setDefault()
				m.ctl.setActiveVault(cur, curS)
				return err
			})
		}},
		{binding: kPrune, run: func(sel listItem) tea.Cmd {
			folder, suffix := splitVaultKey(sel.filter)
			return submitOpNoPop("pruned "+vaultLabel(sel), func() error {
				return m.ctl.pruneVault(folder, suffix)
			})
		}},
	}
	return newListScreen("vaults", build(), acts, build).
		withHelp(`The vaults aql knows about — found on disk and recorded when created.

  enter   Switch to the selected vault.
  n       Create a new vault.
  d       Make the selected vault the default that aql vault -i opens.
  D       Forget a stale entry from the list (its files are left untouched).`)
}

func (m *rootModel) vaultItems() []list.Item {
	vs, _ := m.ctl.listVaults()
	items := make([]list.Item, 0, len(vs))
	for _, dv := range vs {
		marker := ""
		switch {
		case dv.Folder == m.ctl.folder && dv.Suffix == m.ctl.suffix:
			marker = "● "
		case dv.Default:
			marker = "★ "
		}
		state := "ok"
		switch {
		case !dv.Exists:
			state = "MISSING"
		case dv.Locked:
			state = "locked"
		}
		// Capture per-item location + clean name for the enter action.
		folder, suffix, name := dv.Folder, dv.Suffix, dv.displayName()
		items = append(items, listItem{
			name:   marker + name,
			desc:   fmt.Sprintf("%s · %s · %s", dash(dv.Backend), state, dv.Folder),
			filter: vaultKey(folder, suffix),
			act: func() tea.Cmd {
				return func() tea.Msg {
					if err := m.ctl.switchVault(folder, suffix); err != nil {
						return opResultMsg{err: err}
					}
					return switchedToVaultMsg{name: name}
				}
			},
		})
	}
	return items
}

// vaultLabel returns a picker item's display name without its status marker.
func vaultLabel(it listItem) string { return strings.TrimLeft(it.name, "●★ ") }

func vaultKey(folder, suffix string) string { return folder + "\x1f" + suffix }
func splitVaultKey(k string) (folder, suffix string) {
	if i := strings.IndexByte(k, '\x1f'); i >= 0 {
		return k[:i], k[i+1:]
	}
	return k, ""
}

func (m *rootModel) buildCreateVaultForm() screen {
	var suffix, folder, backend, pass string
	backend = BackendAuto
	form := huh.NewForm(huh.NewGroup(
		huh.NewInput().Title("Suffix").Description("blank = the default vault; else vault.<suffix>.jsonic").Value(&suffix),
		huh.NewInput().Title("Folder").Description("blank = ~/.aql").Value(&folder),
		huh.NewSelect[string]().Title("Backend").
			Options(
				huh.NewOption("auto", BackendAuto),
				huh.NewOption("file (passphrase)", BackendFile),
				huh.NewOption("keychain", "keychain"),
				huh.NewOption("secret-service", "secret-service"),
				huh.NewOption("wincred", "wincred"),
				huh.NewOption("1password", "1password"),
			).Value(&backend),
		huh.NewInput().Title("Passphrase").Description("required for the file backend").
			EchoMode(huh.EchoModePassword).Value(&pass),
	))
	return newFormScreen("new vault", form, func() tea.Cmd {
		name := suffix
		if name == "" {
			name = "default"
		}
		return tea.Sequence(popScreen(), func() tea.Msg {
			if err := m.ctl.createVault(folder, suffix, backend, pass); err != nil {
				return opResultMsg{err: err}
			}
			return switchedToVaultMsg{name: name}
		})
	})
}

// --- Secrets ---------------------------------------------------------------

func (m *rootModel) secretsTable() ([]table.Column, []table.Row) {
	cols := []table.Column{
		{Title: "ALIAS", Width: 26},
		{Title: "PROVIDER", Width: 12},
		{Title: "NAMESPACE", Width: 14},
		{Title: "EXPIRES", Width: 20},
	}
	aliases, _ := m.ctl.listAliases()
	rows := make([]table.Row, 0, len(aliases))
	for _, a := range aliases {
		rows = append(rows, table.Row{a.Name, dash(a.Provider), dash(aliasNamespace(a)), dash(a.ExpiresAt)})
	}
	return cols, rows
}

func (m *rootModel) buildSecrets() screen {
	aliasAt := func(idx int) string {
		aliases, _ := m.ctl.listAliases()
		if idx >= 0 && idx < len(aliases) {
			return aliases[idx].Name
		}
		return ""
	}
	acts := []tableAction{
		{binding: kReveal, run: func(idx int) tea.Cmd {
			alias := aliasAt(idx)
			if alias == "" {
				return nil
			}
			return m.requireAuth(func() tea.Cmd {
				v, err := m.ctl.revealSecret(alias)
				if err != nil {
					return statusErr(err.Error())
				}
				return pushScreen(m.buildRevealPager(alias, v))
			})
		}},
		{binding: kAdd, run: func(_ int) tea.Cmd { return pushScreen(m.buildAddForm()) }},
		{binding: kRotate, run: func(idx int) tea.Cmd {
			if alias := aliasAt(idx); alias != "" {
				return pushScreen(m.buildRotateForm(alias))
			}
			return nil
		}},
		{binding: kRemove, run: func(idx int) tea.Cmd {
			if alias := aliasAt(idx); alias != "" {
				return pushScreen(m.buildRemoveForm(alias))
			}
			return nil
		}},
		{binding: kRename, run: func(idx int) tea.Cmd {
			if alias := aliasAt(idx); alias != "" {
				return pushScreen(m.buildRenameForm(alias))
			}
			return nil
		}},
		{binding: kExpiry, run: func(idx int) tea.Cmd {
			if alias := aliasAt(idx); alias != "" {
				return pushScreen(m.buildExpiryForm(alias))
			}
			return nil
		}},
	}
	cols, rows := m.secretsTable()
	return newTableScreen("secrets", cols, rows, acts, "no secrets yet — press a to add one", m.secretsTable).
		withHelp(`Your stored API keys and tokens. Values stay hidden until you reveal them.

  g / enter   Reveal the value on a temporary screen (esc clears it from view).
  a           Add a new secret — you type the value (entry is hidden).
  R           Rotate: replace the value with a new one, keeping the same alias
              and its capabilities. Tokens issued for it keep working unless you
              also choose to revoke them.
  D           Delete the secret and every capability scoped to it.
  m           Rename the secret, or move it to another namespace.
  e           Set or clear its expiry reminder (a reminder only — never enforced).`)
}

func (m *rootModel) buildRevealPager(alias, value string) screen {
	content := tuiErrStyle.Render("⚠ secret revealed — press esc to clear from the screen") + "\n\n" +
		tuiTitleStyle.Render(alias) + "\n\n" + value
	return newPagerScreen("reveal", content, nil, nil)
}

func (m *rootModel) buildAddForm() screen {
	var alias, value, provider, namespace, expiry string
	form := huh.NewForm(huh.NewGroup(
		huh.NewInput().Title("Alias").Description("name, or ns:name to qualify a namespace").Value(&alias).
			Validate(nonEmpty("alias")),
		huh.NewInput().Title("Secret value").EchoMode(huh.EchoModePassword).Value(&value).
			Validate(nonEmpty("value")),
		huh.NewInput().Title("Provider").Description("optional: openai, anthropic, github, …").Value(&provider),
		huh.NewInput().Title("Namespace").Description("optional; overrides any ns: prefix").Value(&namespace),
		huh.NewInput().Title("Expiry").Description("optional: YYYY-MM-DD, RFC3339, or a duration like 90d").Value(&expiry),
	))
	return newFormScreen("add", form, func() tea.Cmd {
		return submitOp("stored "+alias, func() error {
			return m.ctl.addSecret(alias, value, provider, namespace, expiry)
		})
	})
}

func (m *rootModel) buildRotateForm(alias string) screen {
	var value, expiry string
	var revoke bool
	form := huh.NewForm(huh.NewGroup(
		huh.NewNote().Title("Rotate "+alias),
		huh.NewInput().Title("New value").EchoMode(huh.EchoModePassword).Value(&value).Validate(nonEmpty("value")),
		huh.NewInput().Title("Expiry").Description("optional; blank keeps the current reminder").Value(&expiry),
		huh.NewConfirm().Title("Revoke existing capabilities for this alias?").Value(&revoke),
	))
	return newFormScreen("rotate", form, func() tea.Cmd {
		return submitOp("rotated "+alias, func() error {
			return m.ctl.rotateSecret(alias, value, revoke, expiry)
		})
	})
}

func (m *rootModel) buildRemoveForm(alias string) screen {
	var ok bool
	form := huh.NewForm(huh.NewGroup(
		huh.NewConfirm().
			Title("Delete secret " + alias + "?").
			Description("this permanently removes the secret and its capabilities").
			Affirmative("Delete").Negative("Cancel").Value(&ok),
	))
	return newFormScreen("delete", form, func() tea.Cmd {
		if !ok {
			return tea.Batch(popScreen(), status("cancelled"))
		}
		return submitOp("deleted "+alias, func() error { return m.ctl.removeSecret(alias) })
	})
}

func (m *rootModel) buildRenameForm(alias string) screen {
	to := alias
	var revoke bool
	form := huh.NewForm(huh.NewGroup(
		huh.NewNote().Title("Rename / move "+alias),
		huh.NewInput().Title("New name").Description("name, ns:name, or a trailing ns: to move").Value(&to).Validate(nonEmpty("name")),
		huh.NewConfirm().Title("Revoke capabilities instead of re-binding them?").Value(&revoke),
	))
	return newFormScreen("rename", form, func() tea.Cmd {
		return submitOp("renamed "+alias+" → "+to, func() error {
			return m.ctl.renameSecret(alias, to, revoke)
		})
	})
}

func (m *rootModel) buildExpiryForm(alias string) screen {
	var when string
	form := huh.NewForm(huh.NewGroup(
		huh.NewNote().Title("Expiry reminder for "+alias),
		huh.NewInput().Title("Expires").Description("YYYY-MM-DD, RFC3339, duration like 90d — blank clears it").Value(&when),
	))
	return newFormScreen("expiry", form, func() tea.Cmd {
		if strings.TrimSpace(when) == "" {
			return submitOp("cleared expiry for "+alias, func() error { return m.ctl.clearExpiry(alias) })
		}
		return submitOp("set expiry for "+alias, func() error { return m.ctl.setExpiry(alias, when) })
	})
}

// --- Access (capabilities) -------------------------------------------------

func (m *rootModel) accessTable() ([]table.Column, []table.Row) {
	cols := []table.Column{
		{Title: "ID", Width: 12},
		{Title: "ALIAS", Width: 20},
		{Title: "AGENT", Width: 12},
		{Title: "STATUS", Width: 9},
		{Title: "EXPIRES", Width: 20},
	}
	caps, active, _ := m.ctl.listCapabilities()
	rows := make([]table.Row, 0, len(caps))
	for _, c := range caps {
		st := "active"
		switch {
		case c.Revoked:
			st = "revoked"
		case !active[c.ID]:
			st = "expired"
		}
		id := c.ID
		if len(id) > 10 {
			id = id[:10]
		}
		rows = append(rows, table.Row{id, c.Alias, dash(c.Agent), st, dash(c.ExpiresAt)})
	}
	return cols, rows
}

func (m *rootModel) buildAccess() screen {
	idAt := func(idx int) string {
		caps, _, _ := m.ctl.listCapabilities()
		if idx >= 0 && idx < len(caps) {
			return caps[idx].ID
		}
		return ""
	}
	acts := []tableAction{
		{binding: kGrant, run: func(_ int) tea.Cmd { return pushScreen(m.buildGrantForm()) }},
		{binding: kRevoke, run: func(idx int) tea.Cmd {
			if id := idAt(idx); id != "" {
				return pushScreen(m.buildRevokeForm(id))
			}
			return nil
		}},
	}
	cols, rows := m.accessTable()
	return newTableScreen("access", cols, rows, acts, "no capabilities — press a to grant one", m.accessTable).
		withHelp(`Capability tokens the credential proxy will accept for a secret.

  a   Grant: mint a scoped token for an alias. The bearer token is shown ONCE —
      copy it then. You can scope it by agent, hosts, methods, a TTL, and
      call/cost limits.
  D   Revoke: permanently disable the selected token; it stops working at once.`)
}

func (m *rootModel) buildGrantForm() screen {
	var alias, agent, hosts, methods, ttl, maxCalls, maxCost string
	var approval bool
	ttl = "2h"
	form := huh.NewForm(huh.NewGroup(
		huh.NewInput().Title("Alias").Description("the secret this capability may use").Value(&alias).Validate(nonEmpty("alias")),
		huh.NewInput().Title("Agent").Description("optional: name of the tool/agent").Value(&agent),
		huh.NewInput().Title("Hosts").Description("optional: comma-separated allowlist").Value(&hosts),
		huh.NewInput().Title("Methods").Description("optional: comma-separated HTTP methods").Value(&methods),
		huh.NewInput().Title("TTL").Description("lifetime, e.g. 2h, 30m").Value(&ttl),
		huh.NewInput().Title("Max calls").Description("optional integer; 0 = unlimited").Value(&maxCalls).Validate(optionalInt),
		huh.NewInput().Title("Max cost (cents)").Description("optional integer; 0 = unlimited").Value(&maxCost).Validate(optionalInt),
		huh.NewConfirm().Title("Require human approval?").Value(&approval),
	))
	return newFormScreen("grant", form, func() tea.Cmd {
		return tea.Sequence(popScreen(), func() tea.Msg {
			out, err := m.ctl.grant(alias, agent, hosts, methods, ttl, atoiOr0(maxCalls), atoiOr0(maxCost), approval)
			if err != nil {
				return opResultMsg{err: err}
			}
			// Show the one-time token on a pager, then refresh.
			return grantedMsg{output: out}
		})
	})
}

func (m *rootModel) buildRevokeForm(id string) screen {
	var ok bool
	short := id
	if len(short) > 10 {
		short = short[:10]
	}
	form := huh.NewForm(huh.NewGroup(
		huh.NewConfirm().Title("Revoke capability " + short + "?").Affirmative("Revoke").Negative("Cancel").Value(&ok),
	))
	return newFormScreen("revoke", form, func() tea.Cmd {
		if !ok {
			return tea.Batch(popScreen(), status("cancelled"))
		}
		return submitOp("revoked "+short, func() error { return m.ctl.revoke(id) })
	})
}

// --- Passwords -------------------------------------------------------------

func (m *rootModel) passwordsTable() ([]table.Column, []table.Row) {
	cols := []table.Column{
		{Title: "NAME", Width: 18},
		{Title: "SCOPE", Width: 8},
		{Title: "NAMESPACES", Width: 22},
		{Title: "CREATED", Width: 20},
	}
	slots, _ := m.ctl.listPasswordSlots()
	rows := make([]table.Row, 0, len(slots))
	for _, p := range slots {
		rows = append(rows, table.Row{p.Name, p.Scope, namespacesLabel(p.Namespaces), p.CreatedAt})
	}
	return cols, rows
}

func (m *rootModel) buildPasswords() screen {
	nameAt := func(idx int) string {
		slots, _ := m.ctl.listPasswordSlots()
		if idx >= 0 && idx < len(slots) {
			return slots[idx].Name
		}
		return ""
	}
	acts := []tableAction{
		{binding: kPwAdd, run: func(_ int) tea.Cmd { return pushScreen(m.buildPasswordAddForm()) }},
		{binding: kPwRemove, run: func(idx int) tea.Cmd {
			if name := nameAt(idx); name != "" {
				return pushScreen(m.buildPasswordRemoveForm(name))
			}
			return nil
		}},
	}
	cols, rows := m.passwordsTable()
	return newTableScreen("passwords", cols, rows, acts, "no scoped passwords (legacy single-passphrase vault) — press a to add one", m.passwordsTable).
		withHelp(`Scoped vault passwords (keyslots). Each unlocks a chosen scope and set of
namespaces, so different holders can be given different access.

  a   Add: create a named password with a scope (read / write / move / admin)
      and namespaces. You first authenticate with an existing admin password.
  D   Remove: delete a password slot; the other slots keep working.`)
}

func (m *rootModel) buildPasswordAddForm() screen {
	var name, scope, namespaces, newPass string
	scope = ScopeRead
	namespaces = "*"
	form := huh.NewForm(huh.NewGroup(
		huh.NewInput().Title("Name").Value(&name).Validate(nonEmpty("name")),
		huh.NewSelect[string]().Title("Scope").Options(
			huh.NewOption("read", ScopeRead),
			huh.NewOption("write", ScopeWrite),
			huh.NewOption("move", ScopeMove),
			huh.NewOption("admin", ScopeAdmin),
		).Value(&scope),
		huh.NewInput().Title("Namespaces").Description("comma-separated; * = all, : = root").Value(&namespaces),
		huh.NewInput().Title("New password").EchoMode(huh.EchoModePassword).Value(&newPass).Validate(nonEmpty("password")),
	))
	return newFormScreen("password add", form, func() tea.Cmd {
		return m.requireAuth(func() tea.Cmd {
			return submitOp("added password "+name, func() error {
				return m.ctl.passwordAdd(name, scope, namespaces, newPass)
			})
		})
	})
}

func (m *rootModel) buildPasswordRemoveForm(name string) screen {
	var ok bool
	form := huh.NewForm(huh.NewGroup(
		huh.NewConfirm().Title("Remove password " + name + "?").Affirmative("Remove").Negative("Cancel").Value(&ok),
	))
	return newFormScreen("password rm", form, func() tea.Cmd {
		if !ok {
			return tea.Batch(popScreen(), status("cancelled"))
		}
		return m.requireAuth(func() tea.Cmd {
			return submitOp("removed password "+name, func() error { return m.ctl.passwordRemove(name) })
		})
	})
}

// --- Maintenance -----------------------------------------------------------

func (m *rootModel) buildMaintenance() screen {
	items := []list.Item{
		listItem{name: "Verify", desc: "reconcile store & keyring; repair with prune", act: func() tea.Cmd {
			return m.requireAuth(func() tea.Cmd { return pushScreen(m.buildVerifyPager()) })
		}},
		listItem{name: "Scan", desc: "scan files for leaked secret-like strings", act: func() tea.Cmd {
			return pushScreen(m.buildScanForm())
		}},
		listItem{name: "History", desc: "content-revision history; restore a generation", act: func() tea.Cmd {
			return pushScreen(m.buildHistoryPager())
		}},
		listItem{name: "Audit", desc: "structured audit log", act: func() tea.Cmd {
			return pushScreen(newPagerScreen("audit", m.ctl.auditText(), nil, func() string { return m.ctl.auditText() }).
				withHelp("The structured log of vault operations (newest last). Scroll with ↑/↓; secrets are never recorded here."))
		}},
	}
	return newListScreen("maintenance", items, nil, nil).
		withHelp(`Vault upkeep and history. Open an item with enter.

  Verify    Reconcile the metadata with the keyring and report problems;
            optionally repair (prune orphans and dangling records).
  Scan      Search files for leaked secret-like strings.
  History   Browse content revisions and restore to an earlier generation
            (you confirm by typing its number).
  Audit     View the structured log of vault operations.`)
}

func (m *rootModel) buildVerifyPager() screen {
	content, _ := m.ctl.verifyText(false)
	acts := []pagerAction{
		{binding: kPrune2, run: func() tea.Cmd { return pushScreen(m.buildVerifyPruneForm()) }},
	}
	return newPagerScreen("verify", content, acts, func() string { c, _ := m.ctl.verifyText(false); return c }).
		withHelp(`Reconciles the metadata with the keyring and reports any problems above.

  p   Repair: prune orphaned keyring entries, drop dangling records, and remove
      stale temp files. You confirm before anything is changed.`)
}

func (m *rootModel) buildVerifyPruneForm() screen {
	var ok bool
	form := huh.NewForm(huh.NewGroup(
		huh.NewConfirm().Title("Repair the vault (prune orphans, drop dangling records)?").Affirmative("Repair").Negative("Cancel").Value(&ok),
	))
	return newFormScreen("prune", form, func() tea.Cmd {
		if !ok {
			return tea.Batch(popScreen(), status("cancelled"))
		}
		return m.requireAuth(func() tea.Cmd {
			return submitOp("verify --prune complete", func() error {
				if _, okv := m.ctl.verifyText(true); !okv {
					// verify exits non-zero when it had to repair; that is not a
					// failure of the operation itself.
					return nil
				}
				return nil
			})
		})
	})
}

func (m *rootModel) buildScanForm() screen {
	path := "."
	form := huh.NewForm(huh.NewGroup(
		huh.NewInput().Title("Path to scan").Description("directory or file; default current directory").Value(&path),
	))
	return newFormScreen("scan", form, func() tea.Cmd {
		p := strings.TrimSpace(path)
		return tea.Sequence(popScreen(), func() tea.Msg {
			out := m.ctl.scanText(p)
			return pushMsg{m.textPager("scan", out)}
		})
	})
}

func (m *rootModel) buildHistoryPager() screen {
	acts := []pagerAction{
		{binding: kRestore, run: func() tea.Cmd { return pushScreen(m.buildRestoreForm()) }},
	}
	return newPagerScreen("history", m.ctl.historyText(), acts, func() string { return m.ctl.historyText() }).
		withHelp(`Each row above is a saved content revision (generation), newest last.

  R   Restore the vault metadata to a generation. You confirm by typing its
      number. Password slots, namespace keys, and config are preserved.`)
}

func (m *rootModel) buildRestoreForm() screen {
	var genStr, confirm string
	form := huh.NewForm(huh.NewGroup(
		huh.NewNote().Title("Restore vault metadata").Description("see the generation numbers in History"),
		huh.NewInput().Title("Generation").Value(&genStr).Validate(func(s string) error {
			if _, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64); err != nil {
				return fmt.Errorf("enter a generation number")
			}
			return nil
		}),
		huh.NewInput().Title("Type the generation number again to confirm").Value(&confirm).Validate(func(s string) error {
			if strings.TrimSpace(s) != strings.TrimSpace(genStr) {
				return fmt.Errorf("does not match")
			}
			return nil
		}),
	))
	return newFormScreen("restore", form, func() tea.Cmd {
		gen, _ := strconv.ParseInt(strings.TrimSpace(genStr), 10, 64)
		return m.requireAuth(func() tea.Cmd {
			return submitOp(fmt.Sprintf("restored to generation %d", gen), func() error {
				return m.ctl.restore(gen)
			})
		})
	})
}

// --- Settings --------------------------------------------------------------

func (m *rootModel) buildSettings() screen {
	items := []list.Item{
		listItem{name: "Config", desc: "view & edit vault configuration", act: func() tea.Cmd { return pushScreen(m.buildConfigPager()) }},
		listItem{name: "Lock / Unlock", desc: "toggle the vault lock", act: func() tea.Cmd { return m.toggleLock() }},
		listItem{name: "Theme", desc: "switch dark / light / auto (also: T anywhere)", act: func() tea.Cmd { return m.cycleTheme() }},
		listItem{name: "Providers", desc: "built-in provider presets", act: func() tea.Cmd {
			return pushScreen(m.textPager("providers", m.ctl.providersText()))
		}},
		listItem{name: "CLI-only commands", desc: "proxy / mcp / exec run from your shell — press enter for how", act: func() tea.Cmd {
			return pushScreen(m.textPager("cli-only", cliOnlyHelp))
		}},
	}
	return newListScreen("settings", items, nil, nil).
		withHelp(`Vault configuration and preferences. Open an item with enter.

  Config         View and edit config keys (e.g. the default namespace).
  Lock / Unlock  Toggle the lock; a locked vault blocks reads and grants.
  Theme          Switch dark / light / auto (also: T anywhere).
  Providers      Built-in provider presets (base URL, auth style).`)
}

func (m *rootModel) toggleLock() tea.Cmd {
	locked, err := m.ctl.locked()
	if err != nil {
		return statusErr(err.Error())
	}
	if locked {
		return submitOpNoPop("vault unlocked", m.ctl.unlock)
	}
	return submitOpNoPop("vault locked", m.ctl.lock)
}

func (m *rootModel) buildConfigPager() screen {
	acts := []pagerAction{
		{binding: kSet, run: func() tea.Cmd { return pushScreen(m.buildConfigSetForm()) }},
		{binding: kUnset, run: func() tea.Cmd { return pushScreen(m.buildConfigUnsetForm()) }},
	}
	return newPagerScreen("config", m.ctl.configText(), acts, func() string { return m.ctl.configText() }).
		withHelp(`Vault configuration keys and their values are listed above.

  s   Set a key to a value (e.g. namespace.default).
  x   Unset (remove) a key.`)
}

func (m *rootModel) buildConfigSetForm() screen {
	var k, v string
	form := huh.NewForm(huh.NewGroup(
		huh.NewInput().Title("Key").Value(&k).Validate(nonEmpty("key")),
		huh.NewInput().Title("Value").Value(&v),
	))
	return newFormScreen("config set", form, func() tea.Cmd {
		return submitOp("set "+k, func() error { return m.ctl.configSet(k, v) })
	})
}

func (m *rootModel) buildConfigUnsetForm() screen {
	var k string
	form := huh.NewForm(huh.NewGroup(
		huh.NewInput().Title("Key to unset").Value(&k).Validate(nonEmpty("key")),
	))
	return newFormScreen("config unset", form, func() tea.Cmd {
		return submitOp("unset "+k, func() error { return m.ctl.configUnset(k) })
	})
}

// textPager builds a simple scrollable pager over static text.
func (m *rootModel) textPager(title, content string) screen {
	if strings.TrimSpace(content) == "" {
		content = tuiDimStyle.Render("(empty)")
	}
	return newPagerScreen(title, content, nil, nil)
}

const cliOnlyHelp = `These commands run as long-lived processes or hand the terminal to a
child process, so they are not managed from this TUI. Run them from your
shell:

  aql vault proxy   — local credential broker (HTTP server)
  aql vault mcp     — stdio MCP server exposing aliases as tools
  aql vault exec    — run a command with secrets injected as env vars

Capabilities for the proxy are granted/revoked under Access in this TUI.`

// --- palette ---------------------------------------------------------------

func (m *rootModel) runPalette(cmd string) tea.Cmd {
	switch strings.ToLower(cmd) {
	case "":
		return nil
	case "secrets", "secret", "s":
		return pushScreen(m.buildSecrets())
	case "access", "caps", "capabilities":
		return pushScreen(m.buildAccess())
	case "passwords", "password", "pw":
		return pushScreen(m.buildPasswords())
	case "maintenance", "maint":
		return pushScreen(m.buildMaintenance())
	case "settings", "set":
		return pushScreen(m.buildSettings())
	case "vaults", "vault", "folder", "folders", "switch":
		return pushScreen(m.buildVaultPicker())
	case "status":
		return pushScreen(m.textPager("status", m.ctl.statusText()))
	case "audit":
		return pushScreen(newPagerScreen("audit", m.ctl.auditText(), nil, func() string { return m.ctl.auditText() }))
	case "history":
		return pushScreen(m.buildHistoryPager())
	case "providers":
		return pushScreen(m.textPager("providers", m.ctl.providersText()))
	case "config":
		return pushScreen(m.buildConfigPager())
	case "add":
		return pushScreen(m.buildAddForm())
	case "grant":
		return pushScreen(m.buildGrantForm())
	case "lock":
		return submitOpNoPop("vault locked", m.ctl.lock)
	case "unlock":
		return submitOpNoPop("vault unlocked", m.ctl.unlock)
	case "home":
		return func() tea.Msg { return popToRootMsg{} }
	case "quit", "exit", "q":
		return tea.Quit
	default:
		return statusErr("unknown command: " + cmd)
	}
}

// --- small form/value helpers ----------------------------------------------

func nonEmpty(field string) func(string) error {
	return func(s string) error {
		if strings.TrimSpace(s) == "" {
			return fmt.Errorf("%s is required", field)
		}
		return nil
	}
}

func optionalInt(s string) error {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	if _, err := strconv.Atoi(s); err != nil {
		return fmt.Errorf("must be a number")
	}
	return nil
}

func atoiOr0(s string) int {
	n, _ := strconv.Atoi(strings.TrimSpace(s))
	return n
}

// submitOp variants that don't pop a form (used from menus/toggles).
func submitOpNoPop(okText string, do func() error) tea.Cmd {
	return func() tea.Msg {
		if err := do(); err != nil {
			return opResultMsg{err: err}
		}
		return opResultMsg{okText: okText}
	}
}
