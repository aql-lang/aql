# Seneca-style entities & store plugins on boru — a DX probe

Illustrative, **non-executable** boru (the SERVICES words and the `Entity` Ideal
type don't exist yet), answering the question: *how would Seneca's `seneca-entity`
data layer and its store plugins (e.g. `seneca-dynamo-store`) be built on boru using
patrun — and do we need to port Seneca's basic engine?* The formal design is
`../../ENTITY-STORES.0.md`; this folder makes it concrete.

## The answer up front: no, we don't need Seneca's engine

boru's `SERVICES.0` model **is** Seneca's core, re-derived on the same vendored
patrun. The entity layer and store plugins are ordinary boru services and modules
on top:

| Seneca | boru | Where |
| --- | --- | --- |
| `seneca.add` / `seneca.act` | `add` / `call`+`send` | SERVICES §1 |
| patrun most-specific dispatch | patrun (same library) | `patrun.tsv` (impl.) |
| `this.prior()` | `prior` continuation | SERVICES §1 |
| `wrap`-style interceptors | `wrap` | SERVICES §1 |
| `seneca.use(plugin)` | module + `import` + ctor export | SERVICES §2 |
| `seneca.listen`/`client` | `listen`/`connect` | SERVICES §4 |
| **entity layer (make$/.save$)** | **`Entity` Ideal type + `boru:entity` ops** | this probe |
| **store plugin** | **a module exporting a store `Service`** | this probe |
| Seneca store `map` routing | **patrun specificity over canon tags** | `entity.boru` `mount` |

Only the three things in **bold** are new design — the canon, the entity handle,
and the store contract. Everything else is reused.

## Files

| File | Role |
| --- | --- |
| `entity.boru` | the entity **bus** (canon→store router) + the **handle ops** (`make`/`save`/`load`/`list`/`remove`) |
| `mem-store.boru` | the phase-1 in-memory store (`seneca-mem-store` analog) — pure `Store`, no I/O |
| `dynamo-store.boru` | a DynamoDB store **sketch** via `Net.fetch`, with explicit `# GAP:` prerequisites |
| `usage.boru` | the DX: two stores routed by canon, a timestamp + audit `wrap`, then `make`/`save`/`load`/`list`/`remove` |
| `store-test.boru` | the `seneca-store-test` conformance checklist every store must pass |

## How an entity op flows (a `save`)

```
usage.boru                         entity.boru                    a store module
---------                         ----------                    --------------
Entity.save u ──builds──▶ {role:"entity" cmd:"save"
                           base:"sys" name:"user" ent:{…}}
                                  │
                                  ▼  call bus
                          bus patrun routes by the SCALAR tags
                          {role,cmd,base,name} — most specific wins
                                  │
                       ┌──────────┴───────────┐
                  {…base:"app"}            {…}  (default)
                  → DynamoStore           → MemStore
                                              │
                                              ▼  handler reads req.ent / req.q
                                          insert/update, assign id, reply row
```

`ent` and `q` ride along as **payload Maps** — they are *never* patrun pattern
values (patrun matches scalars only, `patrun.tsv:50`). Routing is purely on the
scalar `role/cmd/zone/base/name` tags. That is the whole trick of "entities on
patrun": **Seneca's canon routing is patrun most-specific-match.**

## DX findings

- **The engine question is settled by reuse.** `mem-store.boru` is a plain
  `service` with five `add` handlers; the bus is a `service`; routing is patrun
  specificity; middleware is `wrap`. Nothing here reaches for a primitive that
  SERVICES/patrun doesn't already define. Porting Seneca's runtime would be
  redundant.

- **Canon routing falls out of patrun for free.** `Entity.mount {base:"app"} …`
  vs `Entity.mount {} …` *is* Seneca's `'-/app/-'` vs `'-/-/-'` map — expressed as
  pattern specificity, with wildcards = omitted tags. No separate routing table.

- **The `Entity` Ideal type earns the `make$/.save$` feel** — but note this probe
  models the handle as a Map `{canon data bus}` because boru can't mint a Go-level
  Ideal type from `.boru`. The real type's `Field` hook would make `u.name` read a
  data field directly; here we write `(Entity.data u) get name`. The Ideal type is
  the difference between "looks like ActiveRecord" and "looks like map-poking".

- **We dropped Seneca's `$`-suffix.** `list ent {team:"x"} {sort:{name:1} limit:2}`
  passes query *data* and query *meta* as two maps — cleaner than Seneca's
  `{team:"x", sort$:{…}, limit$:2}` overload, and there's no clash to namespace
  around. (See `store-test.boru` scenarios 4–5.)

- **`prior`/`wrap` give entity middleware with zero store changes.** `usage.boru`
  stamps `updated` on every save and audits every op from the outside — exactly
  Seneca's entity-action chaining. One subtlety the probe surfaced: a timestamp
  added as a `prior` at `{role:"entity" cmd:"save"}` only wraps the *same-specificity*
  (mem) handler, **not** the more-specific dynamo handler — so a *blanket* stamp
  must be a `wrap` (which wraps whichever handler matches), not a `prior`. Worth a
  note in the RFC's middleware section.

- **The real store gaps are honest and concrete.** `dynamo-store.boru` shows the
  store *shape* is trivial, but flags the true blockers: **AWS SigV4 signing**
  (boru has no HMAC/crypto/signer), the **`network`** capability scope, and the
  upsert race the real plugin documents. mem-store needs none of this — hence
  "mem-store first" (ENTITY-STORES §8).

- **One naming clash to resolve:** the op `list` collides with boru's core `list`;
  here the ops are `boru:entity`-namespaced (`Entity.list`). The RFC's Open Q #1
  tracks whether to keep them namespaced or add object-method sugar (`e.save`).

## Cross-references

- Full design + the Seneca→boru mapping: `../../ENTITY-STORES.0.md`
- Service model, `add`/`call`/`prior`/`wrap`, modules, transport: `../../SERVICES.0.md`
- patrun routing semantics (specificity, scalar-only, subset match): `../../../lang/spec/patrun.tsv`
- How the `Entity` Ideal type is registered: `../../IDEAL.10.md`
- `network` capability for real stores: `../../PERMISSIONS.10.md`
- The actor substrate a served store runs on: `../../PROCESSES.0.md`
