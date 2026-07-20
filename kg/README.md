# kg/ — the project knowledge graph

This directory holds two things:

1. **A reusable, evidence-backed knowledge-graph pipeline written in
   AQL** — ingest candidate facts, normalize entities, resolve identity,
   validate every claim, preserve provenance, and query the result with
   bounded traversals.
2. **The knowledge graph of this repository itself** — built by that
   pipeline from [`project/aql-project.jsonic`](project/aql-project.jsonic)
   and committed at [`out/graph.json`](out/graph.json). Agents and humans
   can read it to see, in one structured place, what the repo's modules,
   documents, tools, and concepts are and how they relate — with every
   assertion backed by a quoted passage from the repo's own docs.

**Keep it fresh: when a PR changes the repository's structure, tooling,
or documentation set, update `project/aql-project.jsonic` accordingly and
rebuild the committed bundle (`make graph` here, or
`cd kg && ../cmd/go/bin/aql main.aql`).** The build is deterministic, so
an unchanged input produces a byte-identical bundle and a clean diff.

## Quick start

Everything runs **from this directory** (AQL imports resolve against the
working directory):

```bash
make -C ../cmd/go build     # once: build the aql binary
cd kg
make check                  # aql check every module and test
make test                   # run the whole test suite
make graph                  # rebuild out/graph.json + out/graph.sql
```

Query the committed graph from AQL:

```aql
import "aql:io"
import "./queries.aql"
def g (IO.read (make Pathon "out/graph.json"))
KgQuery.entities-by-type g "Document"
KgQuery.neighbors g "<entity id>"
KgQuery.two-hop-paths g "<id a>" "<id b>"
```

## Architecture

```
candidate bundles (JSON / JSONic / CSV / TSV rows / text-derived facts)
        │  ingest.aql      candidates -> typed records, deterministic ids
        ▼
entities + assertions + sources        (schema.aql: typed Records; every
        │                               object built via checked mk-*)
        │  resolve.aql     evidence-based identity decisions + safe merges
        │  assertions.aql  conflict detection -> disputed, never dropped
        ▼
draft bundle
        │  validate.aql    bundle-level rules -> KgIssue records
        │  report.aql      competency report, review queue, summary
        ▼
aql-kg/1 bundle  ──►  storage.aql  ──►  out/graph.json  (+ out/graph.sql)
                                        queries.aql — bounded graph queries
```

| File | Responsibility |
|------|----------------|
| `schema.aql` | Closed vocabularies, typed `Record` shapes, checked constructors |
| `identifiers.aql` | Deterministic FNV-1a ids, canonicalization, collision detection |
| `normalize.aql` | Label normalization (NFC, trim, whitespace collapse, casefold) |
| `ingest.aql` | Candidate bundles → graph objects; CSV/TSV and bundle re-ingest adapters |
| `entities.aql` | Entity indexing, reversible merges, merge-chain resolution |
| `assertions.aql` | Assertion construction (evidence required), conflict marking |
| `resolve.aql` | Identity-resolution policy and automatic-merge rules |
| `validate.aql` | Every bundle-level validation rule, reported as issues |
| `queries.aql` | The query API — lookups, evidence, bounded 1/2-hop traversal, review views |
| `storage.aql` | JSON bundle write/read, round-trip check, normalized SQL emission |
| `report.aql` | Pipeline orchestration (`build-graph`), competency report, summary |
| `main.aql` | Builds the project graph from `project/aql-project.jsonic` |
| `util.aql` | Shared helpers (`get-or`, `list-at`, `starts-with`, `as-map`) |

## The bundle (output contract)

`out/graph.json` follows `schema_version: "aql-kg/1"`:

```
{ schema_version, generated_at,
  sources[], entities[], assertions[], identity_decisions[],
  validation_issues[], schema_proposals[], competency_results[],
  human_review_queue[], summary{} }
```

- **Source** `{id kind locator title retrieved_at content_hash authority metadata}`
- **Entity** `{id type label normalized_label aliases[] external_ids{} attributes{} status}`
  — status ∈ accepted | candidate | merged | rejected
- **Assertion** `{id subject_id predicate object evidence[] confidence
  status valid_from valid_to recorded_at rule}` — the object is exactly
  one of `{kind:"entity" entity_id}` or `{kind:"literal" value datatype
  unit language}`
- **Evidence** `{source_id locator quote extraction_method extractor}`
- **Identity decision** `{id left_entity_id right_entity_id decision
  supporting_evidence[] conflicting_evidence[] confidence review_required}`
- **Validation issue** `{id severity rule target_kind target_id message
  suggested_correction automatic_correction_safe}`
- **Schema proposal** — new vocabulary terms are **never added
  silently**; they ship as proposals with `status: "requires_approval"`.

Approved entity types: Person, Organization, Place, Event, Document,
Product, Concept, Role, Identifier, Other. Approved predicates: type,
same_as, part_of, located_in, member_of, works_for, owns, created_by,
participated_in, has_role, occurred_at, mentions, supports, contradicts,
supersedes, related_to, has_attribute.

## Identifiers

All local ids are **deterministic** (64-bit FNV-1a over a canonical
string; no random ids anywhere):

```
src:<fp(locator|content_hash)>
ent:<Type>:<fp(Type|normalized_label|key)>
ast:<fp(subject|predicate|canonical object|valid_from|valid_to|sorted source ids)>
identity:<fp(left|right)>          issue:<fp(rule|target|message)>
```

The candidate **key** joins the entity fingerprint so two same-named
records stay *distinct* entities at construction — identity is decided
by resolution evidence, never by an id coincidence. A hash match alone
is never trusted: `KgId.collisions` reports any id claimed by two
different canonical inputs as a `kg_id_collision` error. Labels
normalize as NFC → trim → collapse whitespace → lowercase (comparison
form); the display label is preserved separately. Authoritative external
identifiers live in `external_ids` and drive identity resolution.

## Identity resolution

Names alone **never** merge entities:

| Evidence | Decision | Auto-merge? |
|----------|----------|-------------|
| Matching external id (same scheme+value, no conflicting scheme) | same_entity 0.98 | yes |
| Same type+label and ≥2 corroborating attributes, no conflicts | same_entity 0.95 | yes |
| Explicit `same_as` assertion, no corroboration | same_entity (assertion confidence) | **no — review** |
| Same type+label only | possible_match 0.60 | **no — review** |
| Shared scheme, conflicting values | different_entity (or insufficient_evidence if others match) | no |

Merges are **non-destructive and reversible**: the loser keeps its
record with `status: "merged"` and `attributes.merged_into` pointing at
the canonical entity; its label and aliases fold into the winner's
aliases and its external ids union in. Queries chase merge chains
through a **bounded** resolver (8 hops max).

## Claims, provenance, conflicts

- Every assertion **must** carry ≥1 evidence record; the constructor
  refuses evidence-free claims and ingest reports them as
  `kg_no_provenance` errors.
- Model/agent-proposed candidates pass the same checks; **model output
  is never evidence** — evidence must point at the original source, and
  model-extracted evidence without a quote is flagged for review.
- Conflicting claims on a single-valued predicate (type, located_in,
  works_for, occurred_at) are **preserved** and marked `disputed` —
  nothing is overwritten or discarded; the pair lands in the review
  queue. Claims whose objects canonicalize identically share one id
  (deduplication, not conflict).
- `inferred` assertions must record their rule id.

## Validation

`validate.aql` reports (never hides, never auto-repairs) issues for:
duplicate/missing ids everywhere; unknown source kinds/authorities;
unknown entity types/statuses; empty labels; merged-entity pointer
integrity (existing target, no self-merge); unknown predicates;
malformed objects (must be exactly entity-ref XOR literal); dangling
subjects/objects/sources; unknown datatypes; missing provenance;
confidence outside [0,1]; inverted validity intervals; rule-less
inferred assertions; lone disputed assertions; illegal or unsupported
merges (threshold 0.95 + evidence, possible_match never merged);
non-`requires_approval` proposals; fingerprint collisions; and missing
quotes on text/model evidence (warnings).

## Queries

All pure functions over the bundle in `queries.aql`: `entity-by-id`,
`entities-by-type`, `entities-by-label` (alias-aware, normalized),
`assertions-for-subject`, `assertions-for-object`,
`assertions-by-predicate`, `assertions-by-source`,
`assertions-in-range`, `evidence-for-assertion`, `edge-list`,
`neighbors`, `one-hop-paths`, `two-hop-paths`,
`unresolved-identities`, `conflicting-assertions`, `validation-errors`,
`human-review-items`. Traversal depth is **bounded by construction** —
one and two hops only; there is deliberately no recursive walker.
(`aql:query`'s SQL pipeline resolves FROM-tables from the context
store — a Go-registration fit — so the query layer uses plain
`filter`/`fold`, which also reads clearer here.)

## Testing

```bash
cd kg && make test
```

`tests/` covers: constructor accept/reject pairs, vocabulary membership,
deterministic ids (including evidence-order independence), collision
handling, normalization (with property tests: idempotence,
id-as-function-of-input), every resolution decision (safe merge,
rejected name-only merge, corroborated merge, conflicting ids, same_as
without corroboration, bounded chain chasing), every planted defect in
`fixtures/invalid.jsonic`, the full query API over the fixture graph,
JSON round-trips, file write/read-back, build determinism, re-ingest id
stability, and SQL emission (determinism + escaping). Fixtures:
`valid.jsonic`, `conflicts.jsonic`, `ambiguous-identities.jsonic`,
`invalid.jsonic`.

## SQLite

REFERENCE.md documents `sqlite-open`/`sqlite-exec`/`sqlite-query` behind
a `sqlite` capability, but the current engine build does not register
those words (`aql describe sqlite-open` → no description). Until it
does, `storage.aql` emits the graph as **normalized SQL**
(`out/graph.sql` — sources, entities, entity_aliases,
entity_external_ids, entity_attributes, assertions, assertion_evidence,
identity_decisions, validation_issues, schema_proposals; one
transaction, never an opaque JSON blob):

```bash
sqlite3 graph.db ".read out/graph.sql"
```

## Permissions

The pipeline needs only local file reads/writes inside `kg/` (the CLI's
default `fileio` capability). It performs no network access, no process
execution, and stores no credentials anywhere. Model-assisted extraction,
if you use it, happens *outside* this pipeline and enters as candidate
bundles subject to the full evidence checks.

## Known limitations

- **`aql fmt` cannot be run on these sources.** The current formatter
  corrupts template-string interpolations — `` `hi ${name}` `` becomes
  `` `hi $ {name:name} ` `` (the `${expr}` hole is re-parsed as a map
  literal), which changes program semantics. The kg sources use
  templates heavily and are therefore hand-formatted; `make fmt` here
  is a guarded no-op until the formatter is template-safe. Every file
  passes `aql check` (`make check`).
- Native SQLite words are unavailable in the current build (see above) —
  `out/graph.sql` is the relational path; it loads cleanly into stock
  `sqlite3` with zero foreign-key violations.
- Pair generation in resolution is a bounded O(n²) sweep — fine at this
  repo's scale; add blocking (e.g. by normalized-label prefix) before
  pointing it at very large entity sets.
- `text`-kind sources are ingested as *candidate assertion bundles*
  (extraction happens outside AQL — by an agent, a rule, or a model);
  AQL validates evidence and quotes but does not itself do NLP.
- `generated_at`/`recorded_at` are pinned in `main.aql` (`run-stamp`)
  so rebuilds are byte-identical; bump the stamp when regenerating
  after a content change.
