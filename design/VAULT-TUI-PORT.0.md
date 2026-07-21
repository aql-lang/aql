# VAULT-TUI-PORT

Design for porting the **interactive vault TUI** (`aql vault -i`) from its
bubbletea implementation (`cmd/go/internal/vault/tui_*.go`, ~3,900 lines of
Go) onto the **landed `aql:tui` surface** — the vault UI rewritten *in AQL*,
running under `Tui.run` on the same actor substrate as
`design/examples/apps/todo-tui.aql`.

The port is deliberately the first *real* application on `aql:tui`, and its
planning phase doubles as a **gap audit**: every capability the vault TUI
needs that AQL or its core modules cannot supply today is enumerated in §2
with a disposition — closed by this design, pushed to the app layer, or
explicitly deferred with a spec sketch (§7).

This is a **design RFC first, implementation tracked alongside** — the
implementation phases (§6.3) land on the same branch, and this file is
updated to as-built at the end.

> **Decisions proposed at design time** (the forks this RFC closes; a
> reviewer ratifies or reopens them):
> (1) the backend is a **bridge**: a native `aql:vault` module wraps the
> existing Go vault command layer; crypto, keyring, and storage stay in Go
> (§1.2). Migrating vault *logic* into AQL is a specced future phase (§7);
> (2) the AQL app ships **side-by-side opt-in** — `aql vault -i --aql`; the
> bubbletea TUI stays the default until parity is proven (§1.3);
> (3) the bridge host seam is **one generic `Do(op, params)` closure**
> injected from `cmd/go`, mirroring `RegisterHostTui` (§3.1);
> (4) the adapter — not AQL — owns the **session secrets**: active vault,
> cached passphrase, authed flag. AQL sees `authed: Bool` (§5.1);
> (5) screens are **data on a stack in app state**, folded by per-kind pure
> functions — not objects, not processes (§4.2);
> (6) vault operations run **inline in `update`** by default; only
> potentially blocking ops use the spawn-and-send idiom (§4.4);
> (7) forms are built **in AQL from `Tui.input` + `Tui.edit`** — no new
> widget kinds; the only tuikit change is a `mask` option on `input` (§2.2);
> (8) no env-var or exit-code words are added — the Go launcher and adapter
> own both ends (§2.3).

## 1. Goal and strategy

### 1.1 What is being ported

`aql vault -i` (see `CLI.md` "Interactive TUI") is a screen-stack app:
Home menu → Secrets / Access / Passwords / Maintenance / Settings, plus a
multi-vault picker, a passphrase gate, a `:` command palette, and a `?` help
overlay. Every mutation is driven through the vault CLI's `Run()` entry
point (via `tuiController`), guaranteeing the TUI and CLI can never drift.
The full behavioural inventory is the parity matrix in §6.1.

### 1.2 Bridge, not rewrite (decision 1)

The vault's security core — scrypt/AES-GCM file keyring, HKDF keyslots,
nacl/box namespace keys, OS keychain backends, atomic store writes,
journal/audit — is battle-tested Go with **no AQL surface**. Rewriting it in
AQL is neither possible today (no crypto words, §2.4) nor desirable in one
step (security-sensitive logic should not churn as a side effect of a UI
port).

So the port splits exactly where the Go TUI already splits:

| Layer | Go TUI today | AQL port |
| --- | --- | --- |
| Screens, navigation, keys, forms, chrome | bubbletea (`tui_model.go`, `tui_screens.go`, `tui_build.go`) | **AQL** (`lang/go/modules/vault_tui.aql`) on `aql:tui` |
| Command layer (all reads + mutations) | `tuiController` → `vault.Run()` | **unchanged Go**, exposed as the `aql:vault` bridge (§3) |
| Crypto / keyring / storage | Go (`keyring.go`, `keyslot.go`, `store.go`) | unchanged Go, behind the bridge |

The bridge preserves the TUI↔CLI parity property: the AQL app drives the
same command layer the CLI does.

### 1.3 Side-by-side launch (decision 2)

`aql vault -i --aql` runs the AQL app; plain `aql vault -i` remains the
bubbletea TUI, byte-identical. The flag flip (making AQL the default,
retiring the bubbletea screens) is out of scope for this RFC and gated on
the §6.1 parity matrix being demonstrably green.

## 2. Gap inventory

The audit question: *what does a full-screen vault app need that AQL and its
core modules cannot do today?* Each gap carries a disposition.

### 2.1 Gaps closed by new native surface (this design)

| # | Gap | Evidence | Disposition |
| --- | --- | --- | --- |
| G1 | **No vault surface in AQL.** The entire command layer is Go-only under `cmd/go/internal/vault/`. | no `vault` module in `lang/go/modules/modules.go` | New **`aql:vault` bridge module** (§3), host-injected — `lang/go` cannot import `cmd/go`, so the backend arrives via a `RegisterHostVault` seam exactly like `RegisterHostTui`. |
| G2 | **No clipboard word.** The Go TUI's `c` (copy exec recipe) and token copy use `clipboard.go` (pbcopy/xclip/…), Go-only. | no clipboard word in any module | `Vault.copy` rides the vault host spec; the adapter reuses `detectClipboard`. A general-purpose clipboard module is deferred (§7.4). |
| G3 | **No masked text input.** `tuikit`'s input widget paints `value` verbatim (`render.go` `paintInput`); a passphrase form would echo the passphrase. | `lang/go/tuikit/render.go` | Add a **`mask: true` option** honored by the renderer (§2.2). `Tui.input` already passes unknown option keys into the widget map, so only the renderer (and its measure path) changes. |

### 2.2 Gaps closed at the app layer (no new native surface)

| # | Gap | Disposition |
| --- | --- | --- |
| G4 | **No list/table filtering.** bubbles' `list` has built-in `/` filtering; `Tui.list-view`/`Tui.table` take items+cursor only. | App-level: `/` toggles a filter input in the screen's state; visible rows are recomputed by the screen builder. Keeps match semantics in AQL, no widget change. |
| G5 | **No form primitives.** The Go TUI leans on `huh` (fields, validation, password inputs, confirms). | A small **form model in AQL** (§4.5): fields as data, `Tui.edit` for editing, per-field validate fns, focus-as-state. This is the port's largest new AQL component and a deliberate dogfooding target. |
| G6 | **No focus manager; single `update` fn.** | Accepted framework property (TUI.0.md §3.4 "focus is app state"). Screens-as-data + `case` routing (§4.2). |
| G7 | **No terminal background detection.** The Go TUI calls `lipgloss.HasDarkBackground()` once at launch; tuikit has no equivalent. | The launcher passes `{dark: Bool}` (still computed Go-side) into `VaultTui.run`; the theme palette (§4.6) derives from `theme` + `dark`. Prefs persist via `Vault.prefs`/`Vault.set-prefs` (`~/.aql/tui.jsonic`), keeping Tier-1 untouched. |

### 2.3 Non-gaps on inspection

| # | Candidate gap | Why it is not one |
| --- | --- | --- |
| N1 | No env-var words (`AQL_VAULT_FOLDER/SUFFIX/PASSPHRASE`). | Launch-vault resolution and env promotion stay in the Go adapter (as `tuiController` does today). AQL never needs to read the environment. A general `Os.env` word set (gated by the existing-but-unused `env` policy scope) is deferred (§7.4). |
| N2 | No `exit` / exit-code word. | `Tui.run` blocks and returns the final state; the Go launcher maps an error to exit code 1. Nothing in the app needs to set a code. |
| N3 | No tick/timer primitive in `aql:tui`. | The Go TUI uses **no timers** (no `tea.Tick`, no subscriptions); the status line persists until replaced. Where the AQL app wants polish (status auto-expiry), `TimeUtil.timeout N [send {…} pid]` composes it — documented in §4.4, not required for parity. |
| N4 | No `while` loop. | The TEA fold needs none; iteration is `for`/`each`/`fold`/recursion throughout. |

Everything else the app needs is **already well-covered**: file I/O with
atomic writes and permissions (`aql:io`), JSON/jsonic
(`StructUtil.jsonify/parse`), error handling (`raise`, `do…error`), async
(`TimeUtil`, `spawn`/`send`/`receive`), sorting/filtering, typed `fn` +
closures, and testing (`aql:test`, TSV specs, the VirtualBackend e2e
harness).

### 2.4 Gaps found while building (the dogfooding yield)

Discovered by writing `vault_tui.aql` itself; each is worked around in
the app and marked for language-level follow-up:

| # | Gap | Evidence / workaround |
| --- | --- | --- |
| G8 | **A recovered `raise` inside a map-literal value tears down the enclosing fn's bindings.** `def f fn [[t:String] [Map] [ {title: t text: (g)} ]]` where `g` internally catches a raise (`do … error`) → `undefined word: t` at the *other* map keys. | Engine bug (fn-body cleanup runs on the unwind even though `do` recovers). Workaround: `def`-bind any raise-capable expression BEFORE the map literal (`vt-status-pager`). |
| G9 | **A multi-arg call in a `case` DEFAULT slot mis-collects.** The case machinery's own values sit on the stack, and an open call in the default position collects them as arguments (`expected Map, got fn __casematch(...)`), or silently folds the wrong value (the event became the app state). | Language sharp edge, at minimum a docs gap. Workaround: parenthesize call-form defaults — `(vt-screen-key state ev)`. |
| G10 | **`def why (dot message)` in an error handler never works.** The paren opens a fresh collection context, so `dot` sees no receiver — yet this exact idiom appears in `design/examples/apps/todo-tui-client.aql` (its error arms are untested). | Use unparenthesized `dot message` then a helper fn that takes the message from the stack (`vt-err-text` / `vt-vault-fail`). The client example needs the same fix. |
| G11 | **A list literal returned as a fn's result evaluates lazily — after body teardown.** `fn [[a:Map] [List] [ [ (f a.x) … ] ]]` raises `undefined word: a` at the *call site* that finally consumes the list, because auto-evaluation runs once the param binding is gone. | Language sharp edge (a docs gap at minimum; arguably the list should snapshot its lexical bindings). Workaround: `def`-bind the list inside the body — `def` evaluates eagerly — and return the name (`vt-alias-row`). |
| G12 | **An `/r`-parked fn does not match a `Function`-typed param.** `wa {…} f2/r` with `wa fn [[s:Map act:Function] …]` fails dispatch with `got (Map, __FN)` — the parked value carries the dispatch-marker type, and the call fails silently at runtime (the check names it, the interpreter just leaves the args stranded). Yet the same value stored in a map (`{run: f2/r}`) and invoked by dot-access (`m.run s`) dispatches perfectly. | First-class-function ergonomics gap: either `__FN` should satisfy `Function` sigs or the parking should convert. Workaround: continuations travel inside maps — every form `submit` and auth `then` is `{run: <fn/r>}` invoked as `scr.submit.run state`. |

### 2.5 Deferred gaps — the vault-logic-in-AQL phase (§7)

Blocking a *full* rewrite (not this port), confirmed absent from the
language layer:

- **Cryptography**: no AEAD (AES-GCM/XChaCha20-Poly1305), no KDFs
  (scrypt/argon2/HKDF), no SHA-256/512 or HMAC, no nacl/box, no
  constant-time compare. `aql:bin-util` offers only base64/hex and
  non-cryptographic fnv hashes.
- **Secure randomness**: `aql:rand` is `math/rand` — unusable for salts,
  nonces, keys, or tokens.
- **OS keyring**: macOS Keychain / Secret Service / wincred / 1Password
  integrations are Go-only (`keyring.go`).

§7 sketches the word surface for each so the migration path is on record.

## 3. The `aql:vault` bridge module

### 3.1 Host seam (decision 3)

`lang/go` stays free of vault (and OS) dependencies. The module ships the
words and their validation; the backend arrives from the host:

```go
// lang/go/modules/vault.go
type VaultSpec struct {
    Name string                                            // diagnostics ("cli", "fake")
    Do   func(op string, params map[string]any) (any, error)
}
func RegisterHostVault(reg *native.Registry, spec VaultSpec) error
```

Mechanics mirror `RegisterHostTui` (`lang/go/modules/tui.go`): a
per-registry capability slot (`capVaultHost`), appended to
`native.ModuleInheritedCaps` so file/inline module sub-registries inherit
it, one backend per registry, resolution at dispatch time. With no host
registered every word raises `no_backend` — the natural state of the spec
harness, wasm, and CI.

**One generic `Do`** instead of ~35 typed closures: the lang side owns
argument validation and value conversion (maps/lists/strings via the
existing any↔Value converters); the adapter returns plain
`map[string]any` / `[]any` / `string`. This minimizes the host API surface,
keeps the cover-gate cost of the bridge in table-driven code, and lets lang
tests inject a trivial scripted fake.

### 3.2 Word surface

Namespace **`Vault`**, import id **`aql:vault`**. All inner signatures use
`BarrierPos: -1` (the module-wrapper dispatch rule — see `lang/go/CLAUDE.md`
"Module FnDef Wrappers"). Option-map arguments are validated lang-side
*before* the backend lookup so usage errors are testable headlessly.

| Group | Words |
| --- | --- |
| Session | `status → Map` · `needs-passphrase → Boolean` · `authed → Boolean` · `authenticate <pass:String> → Boolean` · `status-text → String` · `providers-text → String` · `config-text → String` |
| Secrets | `secrets → List` · `secret <alias> → Map\|None` · `reveal <alias> → String` · `add {alias value provider namespace expiry}` · `rotate {alias value revoke-caps expiry}` · `remove <alias>` · `rename {from to revoke-caps}` · `set-expiry <alias> <when>` · `clear-expiry <alias>` · `recipes <alias> → List` |
| Access | `capabilities → Map` · `grant {alias agent hosts methods ttl …} → String` (one-time token output) · `revoke <id>` |
| Passwords | `passwords → List` · `password-add {name scope namespaces pass ttl}` · `password-remove <name>` · `password-revoke-temp` · `temp-password-count → Integer` |
| Maintenance | `verify {prune} → Map` · `scan <paths:List> → String` · `history → String` · `restore <generation:Integer>` · `audit ({filters}) → String` |
| Settings | `config-set <key> <value>` · `config-unset <key>` · `lock` · `unlock` · `locked → Boolean` · `prefs → Map` · `set-prefs {theme}` |
| Multi-vault | `vaults → List` · `switch {folder suffix}` · `create {folder suffix backend pass}` · `set-default` · `prune-index {folder suffix}` |
| Misc | `copy <text:String> → String` (clipboard; returns the tool label) |

`recipes` returns the provider-aware exec inject commands (the adapter
reuses `injectCommands`/`recipeExampleCmd`) so the detail screen is
parity-exact without duplicating provider knowledge in AQL.

**Error mapping**: bad/missing args → `vault_usage` (checked before backend
lookup); no host → `no_backend`; adapter op failure → code `vault` with the
adapter's first-line message. All raised via the standard error path so
`do […] error […]` folds them into the status line.

### 3.3 Policy

New scope **`vault`** in `policy.KnownScopes`, ops `read` (status/lists/
text panels), `reveal`, `mutate` (add/rotate/rename/expiry/remove/config/
lock/restore/grant/revoke/passwords), `admin` (create/switch/default/
prune-index), `clipboard`. `GlobalsFor`: read → `disk.read`; mutate/admin →
`disk.write`; clipboard → `process` (it execs pbcopy/xclip/…). Enforcement
mirrors the tui module's `terminal` scope check.

### 3.4 The cmd-side adapter

`cmd/go/internal/vault/aqlbridge.go` implements `VaultSpec.Do` as an
op-switch over the existing `tuiController` methods (and `Store` loads for
reads), plus clipboard, recipes, prefs, and launch-vault resolution.

**Every op runs under one `sync.Mutex`.** The controller promotes the
active vault and cached passphrase into the *process environment*
(`AQL_VAULT_FOLDER/SUFFIX/PASSPHRASE`) per op — safe in bubbletea because
Cmds run on one goroutine, and made safe here by serializing the adapter,
since the AQL app may call from spawned worker processes (§4.4).

## 4. Data flow and reactivity model

### 4.1 One TEA loop, one immutable state map

The app is fn-shaped — `Tui.run {init update view}` — not service-shaped:
the fold is explicit, state is inspectable, and the e2e harness asserts on
the returned final state (the todo-tui precedent). State is a single Map
threaded **functionally**: `state set k v` returns a new map; no `flex`
reference-maps anywhere in the app. The driver drains and coalesces up to
32 mailbox messages per repaint, so bursty updates cost one layout.

```
{ ui: Pid  cols: I  rows: I  dark: B  theme: "auto"|"dark"|"light"  pal: {…}
  vault: {…Vault.status…}                 # header cache
  screens: [ {kind:… …} … ]               # the nav stack; last = top
  status: {text: ""  err: false}
  palette: {open: false  input: (Tui.input …)}
  help: {open: false  scroll: 0}
  add-defaults: {provider: ""  namespace: ""  expiry: ""} }
```

### 4.2 Screens are data (decision 5)

The Go `screen` interface (Init/Update/View/Title/help/reload) becomes a
Map per stack entry — `{kind: "menu"|"table"|"pager"|"form"|"detail" …}` —
folded by three per-kind pure fn families dispatched with `case`:

- `screen-update kind state ev → state'`
- `screen-view kind state screen w h → tree`
- `screen-reload kind state screen → screen'`

Screens carry identity (`table: "secrets"`, `pager: "audit"`, …) so
**reload = re-run the screen's builder**, which fetches fresh rows via
`Vault.*` — the `reloadTopMsg` equivalent. Navigation is plain list ops on
`state.screens` (`push-screen` / `pop-screen` / `pop-to-root`, each followed
by a reload of the new top). The Go code routes navigation through
messages (`pushMsg`/`popMsg`) only because bubbletea commands are async; in
AQL the update fn simply returns the new state — no round-trip.

### 4.3 Event routing

One `update`, layered exactly like `rootModel.handleKey`:

1. **Tag dispatch** — `case ev.tag`: `init` stores `ui`/dims/`dark`, loads
   `Vault.status`, pushes Home (or the vault picker when no vault opens);
   `resize` stores dims; `key` continues below; app-defined tags (§4.4) go
   to their handlers; anything else is ignored (`state`).
2. **Key precedence** — help overlay open → scroll keys scroll, any other
   closes; palette open → `esc` closes / `enter` runs / else `Tui.edit`;
   top screen **captures input** (a form, or a table with `filtering:true`)
   → screen handler first (`esc` cancels); otherwise globals: `?` help,
   `:` palette, `o`/`ctrl+o` vault picker, `T` theme cycle (persisted via
   `Vault.set-prefs`), `esc` clear-applied-filter → pop → no-op at root,
   `q` quit at root else pop, `enter`/action keys → the top screen's
   per-kind handler. `ctrl+c`/`ctrl+\` quit via the driver (default mode).
3. **Quit** — `Tui.quit state`; the launcher drops the returned state
   (§5.2).

### 4.4 Effects: inline by default, spawn-and-send when blocking (decision 6)

Bridge ops are local file operations the Go TUI itself runs synchronously
on its UI goroutine — so the AQL app calls them **inline in `update`**,
wrapped `do [ … ] error [ …status… ]`. The screen cannot repaint during an
update (documented `aql:tui` contract), which for these ops is parity.

The **spawn-and-send idiom** (from `todo-tui-client.aql`) is reserved for
ops that may genuinely block — `verify`, `scan`, `create`, and
`unlock`/`reveal` on OS-keyring backends (helper processes can prompt):

```
spawn [ do [ def out (Vault.verify {})
             send {tag: "op-result"  text: out.text} ui ]
        error [ send {tag: "op-error"  why: (err.message)} ui ] ]
```

Tagged results fold like the Go TUI's messages: `op-result`/`op-error`
(= `opResultMsg`: set status, refresh `state.vault`, reload top),
`granted` (push the one-time-token pager), `revealed` (lands on the detail
screen). Any AQL process can drive the UI the same way —
`send msg (whereis "tui")`.

The status line persists until replaced (parity with the Go TUI). Optional
polish, documented not required: generation-guarded expiry via
`TimeUtil.timeout 5000 [send {tag:"status-clear" gen:G} ui]`.

### 4.5 Forms (decision 7) — the huh replacement

```
{ kind: "form"  title: S  fields: [ … ]  focus: I  error: ""
  submit: <Function>  confirm: {…} }
field := { name label desc  kind: "text"|"password"|"select"|"confirm"
           input: (Tui.input "" {mask:B placeholder:…})
           options: […]  sel: I  validate: <Function> }
```

Typed chars → `Tui.edit` on the focused field's input; `tab`/`down`/`enter`
advance (`enter` on the last field validates all fields, then runs the
`submit` continuation); `up` goes back; `left`/`right` cycle a select's
options; `esc` pops (cancel). Validate fns are pure `[String] → ""|error`;
the **passphrase form's validate is `Vault.authenticate`** run inline, so a
wrong passphrase fails in-place (mirroring huh's `Validate` →
`ctl.authenticate`). Typed-confirmation forms (delete / rotate / restore)
validate the typed value against the expected literal; bridge ops always
pass `--yes` because the form *is* the confirmation — the same contract the
Go TUI has.

**Passphrase gating**: `require-auth <then:Function>` — if
`Vault.needs-passphrase` and not `Vault.authed`, push the passphrase form
carrying `then` as an inert Function value (`…/r`) in the screen map; on
success pop and invoke it. Function-valued continuations in state are safe
here: this state never crosses a process or wire boundary.

### 4.6 Views

Pure `state → tree`. Chrome reproduces the Go TUI's fixed 5+4 layout:

```
Tui.rows [ title-line vault-line crumb-line        # 5-line top chrome
           (body … size {flex: 1})
           command-bar footer-or-palette status ]  # 4-line bottom chrome
```

`Tui.table` for secrets/access/passwords (builders slice rows to the
viewport window), `Tui.list-view` for menus and the picker, `Tui.viewport`
+ wrapped `Tui.text` for pagers, rows of labeled inputs for forms (the
focused field renders `focus: true` → hardware cursor). The command bar
shows the equivalent CLI command per screen (parity feature, carried as
screen data). `state.pal` is a small style-map palette (title/key/dim/err/
ok/crumb) computed from `theme` + `dark`; `T` cycles auto→dark→light and
persists via `Vault.set-prefs`.

## 5. Security model

### 5.1 The adapter owns session secrets (decision 4)

The cached passphrase and the active-vault env promotion live in the Go
adapter, exactly as in `tuiController`. AQL observes `authed: Bool` and
`needs-passphrase: Bool`; it never holds a long-lived passphrase.

### 5.2 Hygiene mandates in the AQL app

Enforced by review and pinned by e2e frame assertions:

- Passphrase and secret-value fields set `mask: true`.
- A passphrase string exists in AQL only as the in-flight form-field value
  and is cleared **in the same update** that calls `Vault.authenticate`
  (success and failure paths both).
- A revealed secret lives only in the detail screen's map and is deleted on
  hide, pop, and vault switch. The e2e suite asserts no frame after leaving
  the detail screen contains the value.
- The launcher drops `Tui.run`'s returned final state (`… drop`) so nothing
  reaches the residual stack.
- `Tui.serve` is never used by this app; no `Log.*`/`print` of field values.

## 6. Parity, testing, rollout

### 6.1 Parity matrix (Go TUI → AQL app)

Global keys: `enter` open · `esc` back · `/` filter · `:` palette · `?`
help · `o`/`ctrl+o` switch vault · `T` theme · `q` quit · `ctrl+c` quit.
Palette words (from `runPalette`): `secrets|secret|s`,
`access|caps|capabilities`, `passwords|password|pw`, `maintenance|maint`,
`settings|set`, `vaults|vault|folder|folders|switch`, `status`, `audit`,
`history`, `providers`, `config`, `add`, `grant`, `lock`, `unlock`, `home`,
`quit|exit|q`.

| Go screen (constructor) | Keys / features | AQL phase |
| --- | --- | --- |
| Home (`buildHome`) | 5-area menu | 2A |
| Secrets (`buildSecrets`) | table, `/` filter, `enter` detail, `a` add (defaults memory) | 2A |
| Secret detail (`secretDetailScreen`) | `r` reveal/hide · recipe list + `c` copy · `R` rotate · `m` rename · `e` expiry · `g` grant · `D` delete | 2A |
| Passphrase gate (`buildPassphraseForm`) | masked input, inline authenticate | 2A |
| Palette / help overlays | `:` command table above · `?` scrollable per-screen help | 2B |
| Theme (`tui_theme.go`) | `T` cycle auto/dark/light, persisted prefs | 2B |
| Access (`buildAccess`) | table · `g` grant → one-time token pager · `D` revoke | 2C |
| Passwords (`buildPasswords`) | table · `a` add · `D` remove · revoke-temp | 2C |
| Maintenance (`buildMaintenance`) | verify (+prune form) · scan form→pager · history (+typed-confirm restore) · audit pager | 2D |
| Settings (`buildSettings`) | config pager + `s`/`x` set/unset · lock/unlock · providers · CLI-help pager | 2D |
| Vault picker (`buildVaultPicker`) | `enter` switch · `n` new · `d` default · `D` prune; launch-into-picker when no vault | 2E |
| Chrome (`tui_model.go`) | title+version, vault line, breadcrumb, CLI command bar, footer keys, status | 2A |

### 6.2 Testing

1. **Lang unit tests** (ADR-008, positives paired with negatives):
   `modules/vault_test.go` drives every word against a scripted fake
   `VaultSpec` — validation, conversion, error mapping, `no_backend`,
   policy denial, swap-form wrapper dispatch. tuikit mask rendering tests.
2. **TSV specs**: `lang/spec/module-vault.tsv` (usage + `no_backend`
   negatives, headless-safe because validation precedes backend lookup);
   `module-tui.tsv` mask rows.
3. **Adapter tests**: `aqlbridge_test.go` on temp vaults (file backend,
   `t.TempDir()`), including a concurrent-ops env-promotion test.
4. **E2E**: `lang/go/test/app_vault_tui_test.go` — VirtualBackend + fake
   spec, scripted keys: unlock → browse → filter → detail → reveal (frame
   shows value) → hide (later frames don't) → add → rotate → grant → quit.
5. **AQL tests**: `vault_tui_test.aql` for the pure helpers (stack ops,
   filter, form validation, palette resolver, theme palette).
6. **Manual**: scratch vault (`AQL_VAULT_FOLDER=$(mktemp -d)`, file
   backend) → `aql vault -i --aql`; walk this matrix; confirm plain
   `aql vault -i` is unchanged.

### 6.3 Implementation phases

0. this RFC; 1a `aql:vault` bridge (green with fake host); 1b tuikit
`mask`; 1c cmd adapter + `--aql`; 2A secrets core (forms framework built on
the passphrase + add forms first); 2B palette/help/theme; 2C access +
passwords; 2D maintenance + settings; 2E multi-vault. Every phase lands
through `make fmt && make vet && make lint && make test && make
cover-gate`; structure/doc phases also rebuild the kg bundle.

## 7. Deferred: vault logic in AQL (spec sketch, not implemented)

The bridge is the *first* step of a migration, not its end state. The
follow-on phase — moving vault logic itself into AQL — needs:

### 7.1 `aql:crypto`

`Crypto.aead-seal <key> <nonce> <aad> <plain> → Bytes` /
`Crypto.aead-open …` (AES-256-GCM; XChaCha20-Poly1305 as an `{alg}`
option) · `Crypto.scrypt {n r p len} <salt> <pass> → Bytes` ·
`Crypto.argon2id {…}` · `Crypto.hkdf {info len} <salt> <ikm> → Bytes` ·
`Crypto.sha256/sha512 <bytes> → Bytes` · `Crypto.hmac <key> <bytes> →
Bytes` · `Crypto.eq <a> <b> → Boolean` (constant-time) ·
`Crypto.box-seal`/`box-open` (nacl). Policy scope `crypto`.

### 7.2 Secure randomness

`Crypto.rand-bytes <n> → Bytes` on `crypto/rand`. Deliberately **not** in
`aql:rand`, which stays `math/rand` and must never be reached for key
material.

### 7.3 OS keyring seam

`RegisterHostKeyring` mirroring the tui/vault seams — `get/set/delete`
over the platform backends — so keychain access stays host-injected.

### 7.4 `aql:os`

`Os.env <name> → String|None` (gated by the existing `env` policy scope),
`Os.clipboard-copy <text>`; the general-purpose homes for N1 and G2.

With those landed, `store.jsonic` read/write, keyslot envelopes, and
capability bookkeeping become expressible in AQL (`aql:io` already covers
atomic writes, locks, and permissions), and the bridge shrinks op by op.
