# Unison PoC: is `canon` a sound basis for content hashing?

> **Status:** Point-in-time findings, measured 2026-08-16 against a build of
> this tree. The follow-up verification pass on
> `unison-in-boru-report.0.md`, in the same relationship
> `verse-report-defects-investigation.0.md` has to `verse-in-boru-report.0.md`.
> **It corrects that report on a load-bearing point** — see §1.
>
> Reproduce: `scripts/hash-identity-probe.sh` (the in-language battery is
> `scripts/hash-identity-probe.boru`; the wrapper adds the two checks a
> single process cannot make for itself).

## 0. What was built and what it measured

`unison-in-boru-report.0.md` proposes deriving a content hash from the
ADR-015 canon contract and using the digest as a definition's identity. The
proof of concept is the smallest thing that tests that claim end to end, and
it needs **no engine change at all**:

```boru
BinUtil.fnv64 (canon v)
```

`canon` is the ADR-015 renderer; `BinUtil.fnv64` is the existing non-crypto
digest (`lang/go/modules/binary.go:501-521`). FNV-64 is not a candidate for
the real thing — it is a stand-in that makes the *structure* of the scheme
testable tonight. Every finding below is about what goes into the digest, so
none of them changes if the digest function does.

Ten properties, each one something content-addressed identity requires.
Verdict: **3 PASS, 8 FAIL, 1 hazard cleared.**

| | Property | Result |
|---|---|---|
| P1 | canonicity — `deq` values canon identically | **FAIL** |
| P2 | a fn's canon is a parseable source form | **FAIL** |
| P3 | name-independence — canon carries no binding name | **FAIL** |
| P4 | alpha-invariance — parameter names don't change identity | **FAIL** |
| P5 | closure sensitivity — identity tracks referents, not names | **FAIL** |
| P6 | macro expansion is visible to the hash | **FAIL** |
| P7 | canon terminates on a dependency cycle | PASS (see §3) |
| P8/P9 | a digest is stable across processes | **FAIL** for `Store` |
| P10 | Go and TS canon agree byte for byte | **PASS** (§5) |

## 1. The correction: round-tripping is not canonicity

The report says boru is "one already-accepted ADR away from being able to
hash a definition." **That is wrong, and P1 is the counterexample.**

```
PASS  m1 deq m2                       # {a:1 b:2} deq {b:2 a:1}  -> true
   canon m1 = {a:1 b:2}
   canon m2 = {b:2 a:1}
FAIL  canon m1 eq canon m2
   hash m1  = 6659990676878193357
   hash m2  = 1074991607099068245
FAIL  hash m1 eq hash m2
```

Two values that are `deq` render to different bytes and therefore hash
differently. Any identity scheme needs

> **`deq x y` ⟹ `hash x == hash y`**

and ADR-015 does not supply it. ADR-015 states the round-trip in **one
direction only** — `parse(canon v) deq v`. That is a statement about the
renderer being *faithful*; it says nothing about it being *canonical*, and
`CANON-ROUNDTRIP.0.md` §1 in fact records that the textual-fixpoint reading
was deliberately **rejected** as the contract. The rejection was right for
its own purpose and leaves hashing without the property it needs.

These are two independent contracts:

- **Faithfulness** (ADR-015, accepted): rendering loses nothing.
- **Canonicity** (unrecorded): equal values render identically.

Hashing needs both. So idea #1 in the report is gated on a *new* rule, not
just on landing an existing one. Note the tension to resolve before writing
that rule: map key order is preserved by `canon` and is *not* significant to
`deq`, so canonicity forces a decision — either sort keys in the canonical
form (and accept that canon no longer reproduces source order) or narrow
`deq` (and accept a wider notion of inequality). That is a language decision,
not an implementation detail, which is exactly why it deserves its own record.

## 2. The blocker the report understated: text hash ≠ meaning hash

P5 is the finding that most changes the recommendation.

```
   usedbl 5 before = 10   after = 500
   hash before     = 4947418992068046316   after = 4947418992068046316
FAIL  hash tracked the meaning change
```

`usedbl` calls `dbl`; rebinding `dbl` changes what `usedbl` *means* — the
documented call-time-binding semantics — while `usedbl`'s canon, and hence
its digest, is byte-identical. The cause is visible in the rendering:

```
fn usedbl[[n:Number][Number][word(dbl) word(n)]]
```

canon renders a callee as its **name**. Unison's normalisation replaces each
dependency with *its hash* before hashing, which is precisely the step that
makes the digest a statement about meaning rather than about text. boru
cannot do that substitution by simply walking the canon output, because
resolving `dbl` to a referent is the very thing boru defers to call time.

This matters because the report offered hash identity as the fix for the
`DepsFresh` hot-path cost and the F1 rebinding divergence
(`RELOAD-INVALIDATION.0.md` §2.2, §3). A digest with this property would not
fix them — **it would reproduce F1 exactly**, serving a compiled unit for
`usedbl` that was built against the old `dbl`. A text hash used as a compiled-
unit cache key is not a weaker version of the Unison scheme; for this purpose
it is unsound.

P6 is the same defect with a wider blast radius. Macros are not expanded in
canon:

```
fn usem[[n:Number][Any][word(unless) (n eq 0) [word(n)]]]
```

so a macro is a compile-time dependency wholly invisible to the digest. Edit
the macro, and every caller's meaning changes while every caller's hash
stands still.

## 3. The trap: what makes P7 pass is what makes P5 fail

Mutual recursion canons fine — no infinite descent:

```
fn isodd[[n:Number][Any][word(if) (n eq 0) [word(false)] [word(iseven) (sub n 1)]]]
PASS  canon terminates on a cycle
```

But it terminates *only because canon does not follow references*. The moment
you fix P5 by substituting referents, `isodd` reaches `iseven` reaches
`isodd` and the naive scheme does not terminate. Unison pays for this with
cycle components — a strongly-connected group hashed as a unit, its members
addressed `#x.n` by index. So P7 is not a free pass: it is the same property
as P5 wearing the opposite sign, and any fix has to buy the cycle machinery
at the same time.

## 4. The three that are already on the books, now measured

P2, P3 and P8 confirm defects the tree already records; the probe adds
measurements rather than news.

- **P2 — fn canon does not re-parse.** `fn sq[[x:Number][Number][word(mul)
  word(x) word(x)]]` fed back to the parser is a hard error
  (`cannot call 'fn' — no signature matches`). ADR-015 already calls a
  rendering no parser accepts a defect; NUR059 holds it open.
- **P3 — fn canon carries the binding name.** `sq` and `sq2` have
  byte-identical bodies and different canons. NUR031's verdict already
  requires a name-independent fn canon.
- **P8/P9 — `Store` canon carries live pointer addresses**, so its digest is
  not merely unstable across processes, it is unstable across *runs*:

  ```
  canon (context) = Store(&{Ideal/Store map[] map[] 0xc0007a00c0 <nil> })
  FAIL  (context)  6388564222072507957 vs 1335904146382219819
  PASS  {a:1 b:2}  6659990676878193357
  PASS  [1 2 3]    2354202127888773305
  PASS  'text'     2936008411154425334
  ```

  Data kinds are stable; the kinds ADR-015 §1 pointedly refused to exempt are
  not.

**P4 is new and is not covered by NUR031.** Two anonymous, *unbound*
functions differing only in a parameter name:

```
   canon x-form = fn [[x:Number][Any][word(mul) word(x) word(x)]]
   canon y-form = fn [[y:Number][Any][word(mul) word(y) word(y)]]
FAIL  alpha-equivalent fns canon identically
```

No binding name appears at all, so this is independent of P3. NUR031 removes
the *binding* name; nothing on the books removes *parameter* names. Unison
rewrites variables to positional references for exactly this reason. So the
normalisation idea #1 needs is strictly larger than NUR031: strip the binding
name **and** de-name the parameters.

## 5. One hazard investigated and cleared

The report's cross-port worry does not survive measurement. Go and TS canon
agree byte for byte on all twelve float cases tried, including max double,
the smallest subnormal, `-0.0`, and shortest-round-trip repr edges
(`0.1+0.2`, `123456789.123456789`), plus unicode, escapes and the empty
string:

```
PASS  all 12 float renderings agree across ports
```

An apparent `1000000` vs `1000000.0` divergence in an earlier draft of the
probe was the probe's own fault — a bare `1e6` parses as an **Integer**, so
the two sides were rendering different types. A second false alarm came from
coercing Go-side values with `add 0.0`, which turns `-0.0` into `0.0` before
canon ever sees it. Both are recorded in the script's comments, because they
are the natural way to get this wrong. **The parity discipline (ADR-014,
`scripts/parity-probe.sh`) is doing its job here**; cross-port canon
agreement is in better shape than the report assumed.

## 6. One practical gotcha for anyone building on this

The compiler **refuses** any program in which a function value reaches
`canon`:

```
warning: bytecode compilation refused, ran on the interpreter (slower):
function value reaches canon (Stage 3)
```

So a `hash` word built over `canon` is self-defeating in compiled mode: the
act of computing a definition's identity drops the program to the
interpreter. The probe passes `--no-compile` deliberately, so that it
measures the property rather than the fallback. Anything built on this needs
the Stage 3 refusal lifted first.

Two smaller edges found while writing the probe, both already recorded as
defects and both blocking a hashing pass over a real codebase:

- A 0-arg fn literal is invoked at `def` time — `def usem (fn [[] [Any]
  [...]])` binds `usem` to `Integer`, and `usem/r` then refuses with
  *"/r requires a function word"*. That is ADR-016's recorded exception, and
  it means 0-arg definitions cannot currently be reached to be hashed.
- `canon` is not reachable through a helper `fn v:Any` — the checker treats a
  possibly-`Function` parameter as a dispatch barrier, so `def hash fn v:Any
  Any [BinUtil.fnv64 (canon v)]` fails to check. The digest has to be spelled
  at each call site, which is why the probe has no `hash` helper.

## 7. What this does to the report's recommendations

| Report idea | Status after the PoC |
|---|---|
| #2 registry hash pinning | **Unaffected, and now clearly the first move.** It hashes *file bytes*, not values, so none of P1–P9 touches it. It is the only one of the four with no prerequisites. |
| #1 hash identity for compiled units | **Substantially harder than stated.** Needs, in order: canonicity as a new rule (§1), alpha-normalisation beyond NUR031 (P4), a referent-substitution step that collides with call-time binding (P5), macro expansion in the hashed form (P6), cycle components (§3), the fn-canon and Store renderers (P2/P8), and the Stage 3 refusal lifted (§6). It is a programme, not a patch. |
| #3 name→hash indirection | Unchanged in value, but strictly downstream of #1 as re-scoped. |
| #4 test caching | Unchanged: downstream of #1 plus effect rows. |

The `hash` word the report suggested as step 2 is still worth building — the
PoC *is* that word, minus the engine plumbing — but its honest billing changes.
It is a **data** digest, sound today for scalars, lists and maps (P9), and it
should be documented as not applying to functions or handles until P1–P6 are
closed. Shipping it as a general "identity of a definition" would be shipping
the unsoundness in §2.

## 8. Verdict

The proof of concept did its job by failing. The report's *direction* survives:
content addressing is still the right lens on the `DepsFresh` cost, the F1
divergence, and the AOT codec's refusal list, and registry hash pinning is
still cheap and still recommended. What does not survive is the report's
estimate of the distance. boru is not one ADR from hashing a definition; it is
one ADR from hashing **data**, and a real programme — canonicity, alpha
normalisation, referent substitution under call-time binding, macro expansion,
cycle components — from hashing **code**.

The most valuable single sentence to take from tonight: **a hash of boru's
canonical text is not a hash of a definition's meaning**, because canon names
its dependencies instead of addressing them, and closing that gap runs
straight into the language's call-time binding rule. That is the design
question idea #1 actually poses, and it was invisible until the thing was
built and run.
