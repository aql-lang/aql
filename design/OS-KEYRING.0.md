# `RegisterHostKeyring` — the OS keyring host seam (`boru:keyring` / `Keyring`)

> **Status: design proposal — not implemented.** This note specifies the
> host seam that lets boru vault logic read and write secrets in the
> platform keychain **without linking a credential daemon into the
> language layer**. It is the authoritative spec for
> [VAULT-TUI-PORT.0](VAULT-TUI-PORT.0.md) §7.3. The seam mirrors
> `RegisterHostTui` / `RegisterHostVault` exactly; the design is a
> deliberate copy of a pattern that already ships.

## 1. Purpose & status

The vault stores secret bytes in an **OS keyring backend** — macOS
Keychain, freedesktop Secret Service, Windows Credential Manager,
1Password, or an encrypted file — reached today only from Go
(`cmd/go/internal/vault/keyring.go`). To move vault logic into boru (§7 of
the port) without dragging `os/exec`, `/usr/bin/security`, the `op` CLI,
or a PowerShell helper into `lang/go`, the keyring must arrive the same
way the terminal and the vault backend already do: **injected by the host
through a capability seam**. `lang/go` stays free of OS-keychain
dependencies; the host supplies the backend; boru words resolve it at
dispatch.

This is the third member of a family — `RegisterHostTui`
(`lang/go/modules/tui.go`), `RegisterHostVault`
(`lang/go/modules/vault.go`), and now `RegisterHostKeyring`. Building it
means replicating the proven seam, not inventing one.

## 2. The host seam

```go
// lang/go/modules/keyring.go  (proposed)
func RegisterHostKeyring(reg *native.Registry, spec KeyringSpec) error
```

Semantics copied verbatim from `RegisterHostVault`
(`lang/go/modules/vault.go`):

- reject an empty `Name` and a nil handler;
- store the spec in a **per-registry capability slot** (`capKeyringHost =
  "engine.keyring.host"`), created on first registration;
- a **second registration on the same registry errors** ("already
  registered") — one backend per registry;
- **register before the import for module-body callers (CRITICAL).** On
  the *same* registry the words resolve the backend live at dispatch, so a
  top-level program may register before or after `import "boru:keyring"`.
  But the vault-in-boru use case runs inside an **imported module body**,
  and `RunModuleBody` snapshots the `ModuleInheritedCaps` slots from the
  parent **at import time**: if nothing has created the slot on the parent
  before the child imports, the child inherits no slot, and a later
  `RegisterHostKeyring` on the parent creates a backend the already-imported
  child never sees. So the host **must register the backend before the
  program imports the module** (which is what the launcher does, ordering
  `RegisterHost*` ahead of the run — matching `registerBoruTuiBackends`).
  This is the same child-registry snapshot semantics as
  [PERMISSIONS.10's Known gap](PERMISSIONS.10.md#known-gap-child-module-registries-do-not-inherit-the-policy);
  the seam's word docs must state the before-import requirement rather than
  the vault seam's looser "before or after" wording.
- with **no backend registered**, the words raise `[boru/no_backend]` —
  the natural state of the spec harness, wasm, and CI, where a **fake
  in-memory backend** is registered for tests.

## 3. `KeyringSpec` shape

The real thing being wrapped is a four-method Go interface
(`cmd/go/internal/vault/keyring.go`):

```go
type keyring interface {
    Name() string
    Set(alias, value string) error
    Get(alias string) (string, error)   // ErrNotFound on miss
    Delete(alias string) error          // idempotent
}
```

The seam should **mirror these typed methods** rather than the
`VaultSpec` single-`Do(op, params)` dispatcher, because the contract is
small, closed, and already proven in this exact shape:

```go
type KeyringSpec struct {
    Name   string                                  // "keychain", "1password", "fake", …
    Get    func(alias string) (string, bool, error) // (value, found, error)
    Set    func(alias, value string) error
    Delete func(alias string) error
}
```

The one adaptation: `Get` returns an explicit `found bool` rather than a
sentinel `ErrNotFound`, so the boru word maps a miss to `None` without the
host and the module agreeing on an error value across the package
boundary (§8).

## 4. Registry capability slot & module inheritance

Exactly as `capVaultHost` (`vault.go`): `capKeyringHost` is a
per-registry slot, and the seam's `init()` appends it to
`native.ModuleInheritedCaps` so an **imported module body inherits the
backend by pointer** at import time — the vault-in-boru logic runs inside a
module and must reach the same backend the host registered on the parent.
This is the mechanism that carries the terminal and vault backends into
module bodies today; the keyring rides the same rail.

## 5. The backend contract

Semantics are the vault's, so an in-boru vault behaves identically to the
Go one:

- **`Set(alias, value)`** replaces any existing value for `alias`; the
  per-alias OS key is namespaced (`"boru:<alias>"`, `keyringService="boru"`).
  The secret is delivered to the platform tool on **stdin, never argv**
  (the Go backends are careful about this — `security -i
  add-generic-password`, `op item create -`, the PowerShell helper's env
  var), and the host backend preserves that.
- **`Get(alias)`** returns `(value, true, nil)` on a hit and
  `("", false, nil)` on a miss — a miss is **not** an error.
- **`Delete(alias)`** is **idempotent**: deleting an absent alias is
  success.
- **Backend selection stays host-side.** `auto` / `keychain` /
  `secret-service` / `wincred` / `file` / `1password` and the
  `autoBackend` fallback-to-file resolution
  (`cmd/go/internal/vault/keyring.go`) are the host's concern; boru never
  names a backend constant. The seam is backend-agnostic.

## 6. Word surface — a minimal `boru:keyring` module

The seam alone is enough for an in-boru vault that the host wires up
directly, but §7.3 frames the deliverable as a seam *so keychain access
stays host-injected* — which still needs boru words to call. Ship a
**minimal `boru:keyring` (`Keyring`) module** of exactly three words over
the seam:

```boru
import "boru:keyring"       # binds the Keyring namespace

alias Keyring.get             # → String | None   (None on miss)
Keyring.set alias value       # → (no result)
Keyring.delete alias          # → (no result; idempotent)
```

Kept to three words: this is a thin, audited wrapper over an OS credential
store, not a general secrets API — the higher-level alias/capability/slot
logic lives in the vault layer above it.

## 7. Policy / capabilities (CRITICAL)

Keyring access is **secret storage reached through an OS credential
daemon** (often via a helper process). It is neither `disk.read`/`.write`
(it is not vault-file I/O) nor `env`, so gating it under those globals
would be dishonest — a sandbox that denies disk would still not want to
grant keychain access, and vice versa.

**Recommendation: a dedicated `keyring` scope with a dedicated `keyring`
global hard-cap.** This lets a profile deny secret storage independently
of everything else. Wiring (from [PERMISSIONS.10](PERMISSIONS.10.md)):

1. Add `"keyring"` to `policy.KnownScopes`.
2. Add a new global cap `"keyring"` to `policy.GlobalOps` (the fixed enum
   grows from 8 → 9) — this is a **hard-cap boundary change** and touches
   `subset.go` (attenuation coverage) and the exhaustive globals test, so
   it needs explicit sign-off.
3. Add a `policy.GlobalsFor` case: `get` → `keyring` (read), `set` /
   `delete` → `keyring` (write) — the read/write distinction carried in
   the op string, exactly like `fileops`.
4. Add `keyring` to every profile: `trusted` → allow; `sandbox` /
   `compute` / `gen` → `install:false` (a sandbox has no business
   reaching the OS keychain by default).
5. A `checkKeyringPolicy(r, op)` gate copying `checkVaultPolicy`.

**Cheaper alternative (documented, not recommended):** no new global —
`GlobalsFor` returns nil and `keyring` is gated only by its own `words`
block. This avoids the enum-widening churn but lets any sandbox that
allows the scope reach the credential daemon without a hard-cap backstop.

**The child-registry leak bites here (unlike crypto).** Because child
module registries do not inherit `CapPolicy`
([PERMISSIONS.10 → Known gap](PERMISSIONS.10.md#known-gap-child-module-registries-do-not-inherit-the-policy)),
a dispatch-time `checkKeyringPolicy` gate silently becomes allow-all when
`Keyring.*` runs inside an imported module body — which is *exactly* where
the in-boru vault will call it. Keyring is effectful and security-critical,
so this is not low-severity as it is for crypto (§8 there). This spec is
therefore **coupled to fixing that gap**: either land the `CapPolicy`
inheritance fix first, or (matching the more robust `permissionedFileOps`
pattern) install the keyring backend into module bodies as an
**already-attenuated wrapper** whose policy is baked into the object, so
enforcement survives even when `HostPolicy(child)` is nil. Prefer the
latter for defence in depth.

## 8. Errors

- **`no_backend`** — nothing registered on the resolving registry; every
  word raises it. Same posture as the tui/vault seams with no backend.
- **`keyring`** — a real backend failure (the platform tool errored, is
  absent, or the keychain is locked). A single error; it does not
  distinguish causes beyond what the host backend surfaces.
- **Not found is not an error** — `Keyring.get` returns `None` on a miss
  (value-or-None, matching `Os.getenv`), reserving `keyring` for genuine
  failures. `Keyring.delete` of an absent alias is success.

## 9. Overlap

- **`boru:vault` (`Vault`)** — the layer *above* this. The vault owns
  aliases, capabilities, password slots, and the envelope crypto; the
  keyring is just where the sealed secret bytes are parked. In the
  migration, the in-boru vault calls `Keyring.*` for storage and
  `Crypto.*` ([BORU-CRYPTO.0](BORU-CRYPTO.0.md)) for the envelopes.
- **`boru:crypto`** — supplies the AES-256-GCM envelope for the **file**
  keyring backend (the encrypted `vault.keyring` container), so even the
  fallback backend is expressible in boru.
- **`boru:io` (`IO`)** — the file keyring backend's persistence (atomic
  write, `0600` mode); host-side today, boru-expressible after the
  migration.

## 10. Open questions / out of scope

- **Dedicated global vs scope-only gating** (§7) — a real hard-cap
  boundary decision; recommended dedicated global, with the cheaper
  no-global alternative documented.
- **Typed methods vs `Do`-style handler** (§3) — recommended typed
  `Get`/`Set`/`Delete` to match the interface being wrapped; the seam
  family may prefer `Do(op, params)` for consistency with `VaultSpec`.
- **Backend enumeration.** Whether boru can *list* or *select* backends, or
  only use the host's choice — recommended host-only; the seam exposes no
  backend identity beyond `Name`.
- **1Password / helper latency.** Backends that shell out (`op`, PowerShell)
  are slow and can prompt interactively; whether the seam needs a timeout
  or a "non-interactive" contract is a backend concern the host owns.

## 11. Implementation sketch (wiring checklist — no code)

- **Seam.** `RegisterHostKeyring` + `KeyringSpec` + `capKeyringHost` +
  `hostKeyringSpec(r)` accessor + the `init()` append to
  `native.ModuleInheritedCaps`, all copied structurally from
  `lang/go/modules/vault.go`.
- **Module.** `BuildKeyringModule(parent)` — three trivial-delegation
  wrappers (`get`/`set`/`delete`) with inner-sig **`BarrierPos: -1`**,
  exported into the `Keyring` map; register in the resolver;
  `moduleDocs["boru:keyring"]`.
- **Policy.** §7 steps (add `keyring` to `KnownScopes`; the
  `GlobalOps`+`GlobalsFor`+`subset.go` global-widening if the dedicated
  global is chosen; profiles; `checkKeyringPolicy` — or the
  permissioned-wrapper form for leak-resistance).
- **Host wiring.** `cmd/go` provides the real backend by adapting
  `cmd/go/internal/vault`'s `keyring` interface to `KeyringSpec` and
  calling `RegisterHostKeyring` alongside the existing
  `RegisterHostVault` / `RegisterHostTui` in the launcher
  (`registerBoruTuiBackends`).
- **Governance gates.** `help.ModuleCatalog` (`catalog_sync`), ADR-003
  export-coverage TSV rows, `check-accuracy` ratchet.
- **Spec + tests.** A **fake in-memory backend** for CI (get/set/delete
  over a map); `lang/spec/keyring.tsv` rows positive **and** negative
  (no-backend → `no_backend`; miss → `None`; a policy that denies keyring
  → `[boru/…]`); `TestTypeLiteralNoPanic` entry.

## See also

- [VAULT-TUI-PORT.0](VAULT-TUI-PORT.0.md) §7.3 — the migration this serves.
- [BORU-CRYPTO.0](BORU-CRYPTO.0.md) — the sibling primitive module.
- [PERMISSIONS.10](PERMISSIONS.10.md) — the policy model and the
  child-registry gap §7 is coupled to.
- `cmd/go/internal/vault/keyring.go` — the backend interface and the
  platform backends the host adapts.
