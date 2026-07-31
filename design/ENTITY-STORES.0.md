# ENTITY-STORES — Seneca-style data entities & store plugins on BORU

Design for a data-entity layer (the `seneca-entity` analog) and pluggable
**store** backends (the `seneca-*-store` analog, e.g. DynamoDB) on top of BORU's
`SERVICES.0.md` model, using **patrun** for routing. This is a **design RFC only —
no implementation code yet.**

## The question

Seneca ships an ActiveRecord-ish data layer: `seneca.make$('zone/base/name')`
gives an *entity* with `save$`/`load$`/`list$`/`remove$`, and store plugins
(`seneca-mem-store`, `seneca-dynamo-store`, …) implement persistence by handling
`role:'entity'` messages. **How does this map onto BORU using patrun — and do we
need to port Seneca's basic engine?**

## TL;DR — no engine port is needed

BORU's `SERVICES.0.md` is **already a re-derivation of Seneca's core**, built on the
*same* vendored patrun (`rjrodger/patrun`, `lang/go/native/internal/patrun/`). The
engine Seneca's entity layer depends on already exists in the BORU design:

| Seneca core (the "basic engine") | BORU equivalent | Status |
| --- | --- | --- |
| `seneca.add(pattern, action)` | `add {pattern} [handler] svc` (SERVICES §1) | designed |
| `seneca.act(msg, cb)` | `call {msg} svc` (sync) / `send` (async) | designed |
| patrun most-specific dispatch | patrun (`patrun.tsv`, rows 15–21) | **implemented** |
| `this.prior(msg, done)` | `prior` continuation (SERVICES §1) | designed |
| `seneca.use(plugin)` | module + `import` + constructor export (SERVICES §2) | partly impl. |
| `seneca.listen` / `seneca.client` | `listen` / `connect` (SERVICES §4) | designed |
| inward/outward interceptors | `wrap` middleware (SERVICES §1) | designed |

So the entity layer and store plugins are **ordinary BORU services and modules** —
nothing about Seneca's JS runtime needs porting. What Seneca's *entity layer* adds
*on top of* the engine is the only genuinely new surface BORU must design:

1. the **canon** — the `zone/base/name` naming of an entity kind;
2. the **entity handle** — `make$` + `save$`/`load$`/`list$`/`remove$`/`data$`;
3. the **store contract** + its conformance suite (`seneca-store-test`).

This is **greenfield** in BORU — there is no existing persistence/ORM/store design
in `design/` today. The rest of this document specifies those three pieces.

> Cross-references: `SERVICES.0.md` (§1 `add`/`call`/`prior`/`wrap`, §2 modules,
> §4 transport, §8 failure contract), `PROCESSES.0.md` (the actor substrate a
> served store runs on), `IDEAL.10.md` (how the `Entity` value type is added),
> `lang/spec/patrun.tsv` (routing semantics), `PERMISSIONS.10.md` (`network`).

## 1. The entity message contract

Seneca entities are sugar over messages; so are BORU entities. Every entity
operation is a `call` to an **entity service** with a fixed tagged-map vocabulary.
The **routing keys are scalar strings** (patrun requires scalar pattern values —
`patrun.tsv:50`); the row and the query travel as **payload Maps that are never
routed on**:

```boru
# routing tags (scalars)           payload (Maps, read off req — NOT routed)
{role: "entity"  cmd: "save"    base: "sys"  name: "user"   ent: <Map>}
{role: "entity"  cmd: "load"    base: "sys"  name: "user"   q:   <Map>}
{role: "entity"  cmd: "list"    base: "sys"  name: "user"   q:   <Map>  opts: <Map>}
{role: "entity"  cmd: "remove"  base: "sys"  name: "user"   q:   <Map>}
{role: "entity"  cmd: "native"  base: "sys"  name: "user"   native: <Map>}
```

- `role`/`cmd`/`zone`/`base`/`name` are top-level **scalar strings** → patrun
  routes on them (gap-B "scalars only" from the PROCESSES/SERVICES review).
- `ent` (the row being saved) and `q` (the query) are **Maps**. A handler reads
  them off `req` (`req.ent`, `req.q`) — exactly the SERVICES rule that an `add`
  pattern *routes but does not bind*. They cannot be patrun pattern values.

This is the crux of "using patrun": **Seneca's canon routing *is* patrun
most-specific-match over the scalar `role/cmd/zone/base/name` tags.**

## 2. Canon → store routing via patrun specificity

Seneca routes an entity to a store via a config `map` (`'-/sys/-': 'dynamo'`).
In BORU the **patrun specificity ladder is the map** — no separate mechanism:

```boru
# A default store catches every entity op (least specific):
add {role: "entity"  cmd: "save"} default-save  bus
# A base-specific store overrides for base "sys":
add {role: "entity"  cmd: "save"  base: "sys"} sys-save  bus
# A name-specific store is more specific still:
add {role: "entity"  cmd: "save"  base: "sys"  name: "audit"} audit-save  bus
```

patrun picks the **most-specific** matching rule (`patrun.tsv:15–17`), ignores
extra subject keys (subset match, `patrun.tsv:18`), and breaks ties on equal key
counts alphabetically (`patrun.tsv:30`). Seneca's wildcard dimensions
(`-/sys/-` = "any zone, base sys, any name") map to **omitting that tag** from the
BORU pattern. So the entire Seneca routing map collapses to "register each store's
handlers at the canon specificity it owns."

## 3. The `Entity` Ideal type (the handle)

The `make$`/`.save$` ergonomics are provided by a new **`Entity` Ideal type** — an
immutable value wrapping `{canon, data, bus}`:

- **canon** — the parsed `zone/base/name` (zone/base optional);
- **data** — the row's fields as a Map;
- **bus** — a reference to the entity `Service` the ops dispatch to.

It is registered like the existing `Patrun` Ideal type —
`eng.Builtin.RegisterExternalBuiltin("Ideal/Entity", <FixedID>, …)` — with the
next free FixedID in the 5000–9999 band (Module 5000, ModuleExport 5001, KeyVal
5002, MiniLang 5003, Patrun 5004; **coordinate** the allocation with the proposed
`Service`/`Pid` types — do not hardcode here). The type provides `Field` (so
`e.name` reads a data field), `Format`, and `Equal`.

```boru
import "boru:entity"

# `entity "zone/base/name" <bus>` builds an empty-data handle (the make$ analog).
def u ( entity "sys/user" bus )
def u ( set name "Alice" u )          # set a field → returns a new Entity (immutable)
def u ( save u )                      # persist; reply is the Entity with its id
load u u.id                           # load by id → Entity or None
list (entity "sys/user" bus) {active: true} {sort: {name: 1}  limit: 20}
remove u u.id
data u                                # the plain Map of fields
```

`save`/`load`/`list`/`remove`/`data` are words from `boru:entity` that **desugar to
the §1 messages** and `call` the entity bus. (Exact spellings — and whether they
are core or module-namespaced to avoid clashing with the core `list` — settle in
implementation; an object-method form `e.save` is also possible per
`OBJECT-METHODS.5.md`.) Crucially the handle is *data + a thin dispatch shim*,
never a hidden connection — identical to Seneca's "entities are just data."

## 4. The store-plugin contract

A store plugin is an **BORU module** that exports a constructor returning a
`Service` (or a set of handlers added onto the bus). It must implement the five
operations against the §1 contract:

| op | input (off `req`) | reply | notes |
| --- | --- | --- | --- |
| `save` | `ent` (+ canon) | `ent` with `id` | `id` present in `ent` ⇒ update, else insert |
| `load` | `q` (+ canon) | one `ent` or `None` | first match when `q` is not `{id:…}` |
| `list` | `q`, `opts` (+ canon) | `List` of `ent` | AND-logic `q`; `opts` = sort/limit/skip/fields |
| `remove` | `q` (+ canon) | removed `ent` or `None` | by id or query |
| `native` | `native` | store-specific | escape hatch to the raw backend |

This is the BORU form of the Seneca store API. **Conformance** is the
`seneca-store-test` analog: every store module must satisfy a shared checklist —
CRUD round-trip, id generation, insert-vs-update detection, AND-query semantics,
sort/limit/skip/field-projection, and the upsert race (concurrent inserts on a
unique field must not duplicate). Once the `boru:entity` words exist these become
positive **and** negative rows under `lang/spec/` (per the repo's test discipline).

## 5. Entity middleware via `prior` / `wrap`

Seneca layers behaviour over entity ops by `add`-ing another handler for the same
pattern and calling `this.prior()`. BORU's `prior`/`wrap` (SERVICES §1) is the same
mechanism — validation, auth, caching, audit, soft-delete, timestamps all layer
**without touching the store**:

```boru
# Stamp `updated` on every save, then delegate to the store.
# Use `wrap` (not a `prior`) so it applies to EVERY save regardless of which
# store matched — see the caveat below.
wrap [ [req state prior] => [
    if (eq req.cmd "save")
      [ prior (set ent (set updated (now) req.ent) req) ]
      [ prior req ]
] ] bus

# Per-action layering (this store's `save` only): `prior` is right here.
add {role: "entity"  cmd: "save"  name: "audit"} [ [req state prior] => [
    require-immutable req.ent                # audit rows can't be edited
    prior req
] ] bus

# Blanket audit around every entity op (any cmd):
wrap [ [req state prior] => [ def out (prior req)  audit req out  out ] ] bus
```

This is precisely Seneca's entity-action chaining, with no new concept.

> **`prior` vs `wrap` across canon specificities (the trap).** Stores register at
> *different* patrun specificities (a default `{role:"entity" cmd:"save"}`, a
> base-specific `{… base:"app"}`, …). A `prior` layer added at one signature wraps
> **only that signature's stack** — it does *not* wrap a more-specific store's
> handler, because patrun routes that request to a different entry. So a
> cross-cutting concern that must apply to **every** save *whichever store
> handles it* (timestamps, audit, validation) must be a **`wrap`** (which wraps
> whatever matched), not a `prior` at the least-specific pattern. Reserve `prior`
> for decorating *one* store's action. (General rule: `SERVICES.0.md` §1.)

## 6. Query model — dropping Seneca's `$`-suffix

Seneca overloads the query object with `sort$`/`limit$`/`skip$`/`fields$` and uses
the `$` suffix to keep meta-keys from clashing with data fields. BORU has no `$`
identifier convention; it can instead pass **query data and query meta as two
maps**, which is cleaner and removes the clash entirely:

```boru
list (entity "shop/product" bus)
     {color: "red"  in-stock: true}          # q — data filter (AND logic)
     {sort: {price: -1}  limit: 20  skip: 0  fields: [name price]}   # opts — meta
```

`q` stays pure data; `opts` carries `sort`/`limit`/`skip`/`fields`. Stores read
both off `req`. (Recorded as a deliberate DX improvement over Seneca.)

## 7. Store wiring: handlers-on-bus vs store-as-service

Two valid shapes, chosen by need — both reuse existing machinery:

- **(a) Handlers on one entity service.** Store modules `add` their cmd-handlers
  directly into the bus's patrun at their canon specificity. Simplest; best for
  in-process and the mem-store. This is the closest analog to Seneca's single
  shared instance.
- **(b) Store as its own `Service`.** A store is a standalone served service (its
  own process, mailbox, connection pool); the bus's handler **forwards/proxies**
  the message to it (`call`/`proxy`, SERVICES §4/§6). Better fault isolation and
  back-pressure for remote stores (dynamo). The uniform failure contract (§8) then
  applies across the hop for free.

The bus is a `service`; stores are modules. Multi-store apps register several at
different canon specificities (mem for `-/-/tmp`, dynamo for `-/app/-`, …).

## 8. Phasing & honest gaps

- **Phase 1 — mem-store.** A `Store`-backed store (the `seneca-mem-store` analog):
  pure in-process, needs nothing new beyond the SERVICES phase-1 surface. Validates
  the contract and the conformance suite.
- **Phase 2 — entity bus + `Entity` Ideal type + `prior`/`wrap` middleware**, and
  store-as-service wiring on the SERVICES/PROCESSES substrate.
- **Phase 3 — real stores (dynamo/postgres).** These need network I/O. BORU has
  `Net.fetch` (HTTP client, `lang/go/native/net_module.go`), and DynamoDB exposes a
  JSON/HTTP API, so a store *can* be written against `Net.fetch`. **Real gaps to
  flag:**
  - **Request signing.** DynamoDB requires AWS SigV4 (HMAC-SHA256 canonical
    request signing). BORU has no crypto/HMAC or signer today — a concrete
    prerequisite, not hand-wavable.
  - **Capability.** Store network access is gated by the **`network`** scope
    (`PERMISSIONS.10.md`); `sandbox`/`read-only` profiles hard-deny it.
  - **Transport.** A store that *serves* (vs. calls out) needs the TCP/socket
    server BORU still lacks (SERVICES §4, later phase).

## 9. Open questions

1. **Entity op spellings / namespacing** — core words vs `boru:entity`-namespaced
   (`list` clashes with the core list word); object-method form `e.save` vs word
   form `save e`. Leaning: module-namespaced words + optional method sugar.
2. **`id` generation** — store-assigned vs bus-assigned (uuid). Leaning:
   store-assigned, since stores like DynamoDB have native id/key semantics.
3. **Canon parsing** — accept Seneca's `"zone/base/name"` string *and* a
   `{zone base name}` map form? Leaning: both, normalised to a canon value.
4. **`native`** — keep the raw escape hatch in phase 1, or defer? Leaning: define
   the pattern now, implement per-store later.
5. **Transactions / batch** — out of scope for phase 1 (Seneca itself punts here);
   note as a future `cmd:"batch"` extension.
