# TABNAS-UPSTREAM-FIRST.0 — tabnas parser defects go upstream, not into a shim

**Status:** ACCEPTED as [ADR-014](../ADR.md#adr-014) · **Recorded:** 2026-08-10
(maintainer instruction: "no workarounds for issues in tabnas parsers —
these always require upstream fixes")

boru's two parsers are thin layers over a pair of dependency twins:
`github.com/tabnas/jsonic/go` over `github.com/tabnas/parser/go`, and
`@tabnas/jsonic` over `@tabnas/parser`. The dependency is where the lexer
lives. When the twins disagree with each other — or one of them is simply
wrong — the defect is upstream's, and this note records why it must be
fixed there rather than papered over in `parser/go` / `parser/ts`.

## The case that settled it

On 2026-08-09 a review found that boru's DATA-decode seam
(`SafeParseData`) had lost jsonic's lenient superset: `{v: 1.2.3}` raised,
and `1.x` silently decoded to the LIST `[1 '.x']`. The cause was boru's
own language-grammar decimal matcher installed into a plain data parser.

The first fix was a boru-side shim: a DATA mode that DECLINES a run it
cannot claim, handing it back to the stock scanner. That is textbook
workaround reasoning — "let the dependency deal with it" — and it was
**wrong in a way the author could not see from one port**. Declining is
only safe where the two stock scanners AGREE, and for dot-adjacent
base-prefixed runs they did not:

```
[0xFF.5]      Go: ["0xFF.5"]      TS: [[], "xFF.5"]   ← phantom element, '0' eaten
[-0xFF.5, 9]  Go: ["-0xFF.5", 9]  TS: [null, null, "xFF.5", 9]
{a:0xFF.5}    Go: {a:"0xFF.5"}    TS: throws
```

An adversarial verification pass caught it. The **second** workaround
layered on the first: claim the whole prose run as one `#TX` token in both
ports, so neither reaches the disagreeing stock path. It worked — and it
left two divergent matcher code paths, ten spec rows, and a comment
explaining a dependency bug, all inside boru.

Two workarounds deep, the actual defect was still in the dependency, still
unreported, and still shipping to every other tabnas consumer.

## What upstream-first bought instead

The defects were written up against the **bare** dependency — no boru
matchers, no boru seams, so each reproduction stood on its own — and fixed
upstream. Four issues, all resolved in `jsonic v0.6.0` / `parser v0.8.0`:

| # | Defect | Resolution |
|---|---|---|
| 1 | TS mangled dot-adjacent base-prefixed runs (fabricated elements, eaten characters) | TS declines the whole run; both ports agree |
| 2 | TS could not represent map insertion order for integer-like keys | opt-in `map:{ordered:true}` + `keyOrder()` side-channel |
| 3 | Go read `+_1` as the number 1, silently swallowing `+_` | both ports treat it as lenient text |
| 4 | `Make` bumped a package-global id counter unsynchronized (a data race) | `atomic.Int64`, documented concurrent-safe |

Every one of those had a boru-side cost that now goes away: a matcher arm,
a process-wide construction mutex, the `GuardMake` wrapper whose whole
signature existed to hold that mutex, and two documented seam asymmetries
that no shared spec row could express — the ordered-map fix turned the
second into ten `data.tsv` rows.

A FIFTH shim fell out of the same upgrade without being reported: the TS
rule engine used to bound its main loop and, on reaching the bound, simply
STOP — no throw, partial root returned — so `[Map<]` parsed to an empty
value stream where Go reported `unexpected \`]\``. boru carried
`watchRuleSteps`/`ruleStepCap` to detect that by counting rule iterations
and re-deriving the library's own cap formula. v0.8.0 throws instead, and
`errors.ts` translates it to the byte-identical diagnostic — notes and
position included, which is why 928 tests pass with the guard deleted. The
shim's own doc had predicted its death: *"If the library ever changes the
formula this guard stops firing rather than firing early — the fail-open
direction."* It stopped firing, silently, and only the 100%-coverage gate
noticed. A shim that reads the dependency's internals (`j.internal()`,
`config.rule.maxmul`) cannot help but rot like this.

## The rule, and its boundary

**Upstream.** A defect in dependency behaviour — lexing, token
boundaries, value construction, concurrency — is reproduced against the
bare dependency, reported, fixed, and consumed as a version bump.
`scripts/parity-probe.sh` is the instrument: it runs a source through both
ports and prints AGREE or DIFFER, which is what turns "TS looks wrong"
into a defect report with a table.

**Boru's own.** A divergence produced by boru's *grammar layer* — the
arrowfold, dotchains, its custom matchers, its recovery diagnostics — is
boru's to fix, recorded in `NUR.md` and pinned in
`parser/spec/divergent.tsv`. The 2026-08-09 sweep's 55 divergences across
nine classes are all of this kind; upgrading the dependency left them
byte-identical, which is the evidence that the two categories really are
distinct.

**The tell.** If you are about to write a comment that explains a
dependency's bug in order to justify boru code, stop: that comment belongs
in an upstream issue. A shim that survives the fix becomes dead weight
nobody dares delete, because its comment no longer describes reality.

## Cost, honestly

Upstream-first is slower when you need the fix now, and it does not work
at all if upstream is unresponsive. The escape hatch is explicit rather
than silent: a temporary shim is permitted only with the upstream issue
linked in its comment and a `NUR.md` record holding it open, so the shim's
removal is somebody's job rather than nobody's.
