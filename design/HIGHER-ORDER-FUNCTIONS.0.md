# Higher-Order Functions in boru — capability audit

**Status: point-in-time report (2026-08-19; re-assessed 2026-08-21).**
A dated empirical snapshot, not a living reference — see
`design/README.md`. Its claims describe the engine as built from this
tree on that date; several of them are pinned to NUR records whose fixes
will change the answers.

**Method:** empirical, against `cmd/go/bin/boru` built from this tree,
cross-read with `core/`, `lang/`, `NUR.md`, `ADR.md` and
`lang/spec/*.tsv`. Every claim below is either a source citation or a
command that was run and whose output is quoted verbatim. Examples are in
the canonical **forward call form** (`AGENTS.md` §"Working in the code")
and use the **fewest square brackets** each signature admits
([`STYLE-GUIDE.md`](../STYLE-GUIDE.md) §S1 / §S2); every one was re-run in
its final spelling — see §4.1 and §4.2 for why those re-runs are not a
formality. Where a `fn` here keeps the bracketed spec-list form, its
signature cannot be reduced: two or more parameters, zero parameters, or
overloads (§4.2).

Companion to [`LISP-ANALYSIS.5.md`](LISP-ANALYSIS.5.md), which graded
higher-order functions **A−** and combinators **B+** from a design
reading. This note tests those grades by *writing the programs*. The
substrate grade holds. What the design reading missed is a cluster of
**call-vs-value** hazards that only surface once a function value flows
through a parameter, and one live **engine disagreement**.

---

## 0. Verdict

| Question | Answer |
|---|---|
| Are functions first-class? | **Yes.** Storable in lists and maps, passable, returnable, closing over locals, recursive, introspectable. |
| Can all combinators be implemented? | **Yes**, since the `/v` change (2026-08-19). S, K, I, B, C, W, U, the full Church basis — booleans *including* `and`/`or`, numerals, pairs — CPS, and a working parser-combinator library were all built and run. The one construction the first pass could not finish now works in its naive spelling (§1.2, §5.2). |
| Is it *pleasant*? | **Much more so.** `/v` is total over binding kinds, so the polymorphic combinators need no escape hatch — the two undocumented idioms (`(args).N` and boxing) are no longer required. |
| Is it *safe*? | **Not yet.** Three silent-wrong-answer classes and one interpreter-vs-compiler divergence, listed in §5. |

**One-line summary:** boru's function substrate is complete — every
combinator in the audit was built and run — and since `/v` became total
over binding kinds the surface has one spelling for "the value, whatever
kind it is". What remains is narrower: a *call-vs-value* ambiguity at the
paren-application and capitalised-name sites, and the fact that the
correctness of a higher-order program can still depend on which engine
runs it (§5.7).

> **Re-assessed 2026-08-19** after `/r`→`/v` / `ref`→`valof` landed with the
> function-only gate removed. §5.2 is closed and NUR085 retired; §5.1,
> §5.3–§5.9 were each re-run against the new binary and are unchanged.

> **Re-assessed 2026-08-21** after the type-node fusion (PR #394) and
> the valof-flip / NUR095 work (PR #396). Every §1 program, §4 spelling
> and §5 hazard was re-run against a binary built from that tree. The
> substrate verdict stands — every §1 program still produces its quoted
> output on both engines — and §5.2 stays closed (no valof-flip
> regression on non-type bindings). Four claims moved, each corrected in
> place: §5.1's `I/v apply` escape hatch is **gone** (`/v` on a type
> binding now denotes the minted node — `lang/spec/valof.tsv` §11 — so
> lowercase names are the only remedy) and its stranded pair prints
> differently; §2's "`Function` is opaque" now holds for *bare*
> `Function` only (`fnsig` shipped, and fn-shape class members apply on
> both engines — NUR095 retired, NUR096 records the checker lag);
> §1.6's non-tail 100 000-deep row holds only on the compiled lane (the
> interpreter now trips the tape ceiling near depth 40 000); and §5.4's
> fn-body return-arity error lost its `ap:` prefix (it now names the
> enclosing fn). NUR073 (§5.7) was re-run and is live, unchanged, the
> default still siding with the compiler; NUR086–NUR089 all still
> reproduce byte-for-byte. New records NUR091 and NUR096 join §5.1's
> and §5.5's defect classes respectively. Stale citations fixed in
> place.

---

## 1. What works — the evidence

Every program below was written and run against this tree, under both
`-no-compile` (interpreter) and the default (bytecode with interpreter
fallback), and is quoted here in full — definitions **and** the calls that
produced the quoted output — so it can be re-run from the note itself.

> **Pinned 2026-08-21.** These transcripts are now also a standing gate:
> `lang/spec/frontier/frontier-hof-audit.tsv` re-checks every §1.1–§1.5
> value (plus §1.6's combinator rows) on the interpreter oracle on every
> `make test`, with the compile ledger in
> `test/go/langspec/frontier_spec_test.go` pinning which rows the
> compiler still refuses and why (§5.8's provenance class, §4.3's capture
> family, NUR087's check false positive). The §1.4 divergences — Z, and
> the naive Y that no strict language can run — are deliberately NOT
> pinned: no budget bounds them (see the §1.4 correction), so a test
> would hang the suite. The deterministic §5.4 rule that causes Z's
> divergence is pinned instead (the corpus file's §7 rows).

### 1.1 The classical combinator bases

`I = S K K`, derived rather than assumed:

```boru
def kk x:Any => [y:Any => [x]]
def ss f:Function => [g:Function => [x:Any => [(f x) (g x)]]]
def ii ((ss kk/v) kk/v)
print (ii 42)        ;# 42
print (ii "hello")   ;# hello
```

`check: 0 error(s)`, correct on both engines. **BCKW** runs correctly on
both engines too, but — unlike SKI — it does **not** check clean:

```boru
def bb f:Function => [g:Function => [x:Any => [f (g x)]]]
def cc f:Function => [x:Any => [y:Any => [(f y) x]]]
def kk x:Any => [y:Any => [x]]
def ww f:Function => [x:Any => [(f x) x]]
def add2 a:Integer => [b:Integer => [add a b]]
print (((bb (n:Integer => [mul n 2])) (n:Integer => [add n 3])) 4)  ;# 14
print (((cc add2/v) 1) 10)                                             ;# 11
print ((kk 7) 99)                                                      ;# 7
print ((ww add2/v) 4)                                                  ;# 8
```

`14 11 7 8` on both engines, but:

```
check: 1:51: [error] no_signature: cannot call `g` — no signature matches
  the arguments; got (Integer); assuming best-fit candidate for analysis
  1 | def bb f:Function => [g:Function => [x:Any => [f (g x)]]]
                                                        ^
check failed: 1 error(s)
```

**The combinator is not what decides this — the CALL SITE is.** The same
`bb`, called with named fn references instead of inline lambdas, checks
clean and returns the same `14`:

```
;# args are inline lambdas       → check failed: 1 error(s), runs to 14
print (((bb (n:Integer => [mul n 2])) (n:Integer => [add n 3])) 4)

;# args are named fn references  → check: 0 error(s),        runs to 14
def d fn n:Integer Integer [mul n 2]
def e fn n:Integer Integer [add n 3]
print (((bb d/v) e/v) 4)
```

And `ss` — which §1.1 shows checking clean — fails identically when
*it* is called with inline lambdas. So this is not S-versus-B and not a
property of the body: **an inline lambda argument loses information a
named `/v` reference keeps**, and only the lambda spelling makes the
analysis mistake `g` for an `Integer`. The SKI block above escapes it
solely because it applies `kk/v`, a reference.

That is a checker defect, not the opacity cost of §2 — an inline lambda
and a named reference to the same function should be equally
checkable. Recorded as **NUR089**.

**A declared function type does not fix it, and it is worth being exact
about that.** Replacing `f:Function` with a declared `f:IntToInt`
(`design/FUNCTION-TYPES.0.md`) leaves the lambda call site still failing
the check — the shape it rejects is real (a `=>` lambda declares no
return, so it is `Integer → Any`, not `Integer → Integer`), but widening
the declared type to `Integer → Any` makes the program *run* while the
check still errors. So NUR089 is orthogonal to opacity and survives the
fn-type work. What function types do buy is stated in §2 and measured in
`FUNCTION-TYPES.0.md`: **sound rejection** of a wrong-shaped argument
that bare `Function` accepts.

### 1.2 Church encodings

Numerals and pairs work unmodified:

```boru
def czero f:Function => [x:Any => [x]]
def csucc n:Function => [f:Function => [x:Any => [f ((n f/v) x)]]]
def cplus m:Function => [n:Function => [f:Function => [x:Any =>
             [((m f/v) ((n f/v) x))]]]]
def cmult m:Function => [n:Function => [f:Function => [(m (n f/v))]]]
def toint n:Function => [((n (k:Integer => [add k 1])) 0)]
def c1 (csucc czero/v)
def c2 (csucc c1/v)
def c3 (csucc c2/v)
print (toint c3/v)                    ;# 3
print (toint ((cplus c2/v) c3/v))     ;# 5
print (toint ((cmult c2/v) c3/v))     ;# 6
```

```boru
def cpair a:Any => [b:Any => [s:Function => [((s a) b)]]]
def cfst  p:Function => [(p (a:Any => [b:Any => [a]]))]
def csnd  p:Function => [(p (a:Any => [b:Any => [b]]))]
def p ((cpair 1) 2)
print (cfst p/v)   ;# 1
print (csnd p/v)   ;# 2
```

Church **booleans** work in their naive spelling. `λt.λf.t` returns a
parameter, and since `/v` is total over binding kinds that is just `t/v`
— no boxing, no `(args).N`:

```boru
def ctrue  t:Any => [f:Any => [t/v]]
def cfalse t:Any => [f:Any => [f/v]]
def cif  p:Function => [t:Any => [e:Any => [((p t) e)]]]
def cnot p:Function => [t:Any => [f:Any => [((p f) t)]]]
def cand p:Function => [q:Function => [((p q/v) cfalse/v)]]
def cor  p:Function => [q:Function => [((p ctrue/v) q/v)]]
print ((((cif ctrue/v)  "T") "F"))
print ((((cif cfalse/v) "T") "F"))
print ((((cif (cnot ctrue/v))  "T") "F"))
print ((((cif (cnot cfalse/v)) "T") "F"))
print ((((cif ((cand ctrue/v)  cfalse/v)) "T") "F"))
print ((((cif ((cand ctrue/v)  ctrue/v))  "T") "F"))
print ((((cif ((cor  ctrue/v)  cfalse/v)) "T") "F"))
print ((((cif ((cor  cfalse/v) cfalse/v)) "T") "F"))
```

```
T  F  F  T  F  T  T  F      (one per line)
```

`check: 0 error(s)`. That is `true`, `false`, `¬true`, `¬false`, `T∧F`,
`T∧T`, `T∨F`, `F∨F` — the full basis.

> **Before `/v` (kept as the record of what changed).** Under the old
> `/r`, this did not work. `[t]` called a function-bound parameter and
> `[t/r]` refused a non-function one, so `ctrue`/`cfalse` needed
> `(args).N` plus a boxing step to reach the enclosing fn's argument —
> and `cand`/`cor`, which must return one Church boolean *as itself*,
> resisted every spelling tried: named booleans tripped
> `lang/spec/fn-value.tsv` §5's loud-dispatch rule ("a failed dispatch
> is LOUD, wherever the value would have gone" — the original text
> mis-cited `valof.tsv` §5 for this; fixed 2026-08-21), and holding them
> anonymously in a list then ran into forward-collection barriers. It
> was the one construction the audit could not finish. Removing the
> function-only gate closed it (§5.2, NUR085 retired).

### 1.3 Continuation-passing style

CPS works cleanly, including the closure-per-frame that CPS implies:

```boru
def factk fn [[n:Integer k:Function][Any][
  if (lte 1 n) [ (k 1) ]
               [ def kk ( fn r:Integer Any [ def m (mul n r) (k m) ] )
                 (factk (sub 1 n) kk/v) ] ]]
print (factk 5  (v:Integer => [v]))          ;# 120
print (factk 10 (v:Integer => [add 0 v]))    ;# 3628800
```

### 1.4 Anonymous recursion

The U-combinator form runs on **both** engines with a clean check:

```boru
def fgen fn s:Function Function [
  ( fn n:Integer Integer [ if (lte 1 n) [1] [ mul n ((s s) (sub 1 n)) ] ] ) ]
def fact (fgen fgen/v)
print (fact 5)       ;# 120
```

The full **Z combinator** (`λf.(λx.f(λv.(x x)v))²`) **diverges**. The
cause is not call-by-value eagerness: lambda bodies were confirmed lazy
(a `raise` inside an unapplied lambda body never fires). The cause is
§5.4 — `((x x) v)` does not parse as an application inside a body, so
the `λv` thunk never actually guards the recursion.

> **Corrected 2026-08-21.** This originally said "it hangs until the
> step limit". Measured: the step limit never trips — a run under
> `--options steps:200000` was still going at 30 s, and an in-process
> run grows until the OS kills it — so the divergence is bounded by
> nothing the engine counts, only by an outside timeout. That is also
> why Z is not pinned as a test (§1's pinning note): there is no budget
> under which "diverges" terminates into an assertable error.

### 1.5 A real parser-combinator library

The standard stress test for higher-order support — `item`, `sat`, `alt`,
`seq`, `many`, all returning closures over their arguments. A parser is a
`Function: String -> {ok val rest}`:

```boru
def pitem s:String => [
  if (eq 0 (size s)) [ {ok:false rest:s val:None} ]
                     [ {ok:true val:(slice 0 1 s) rest:(slice 1 (size s) s)} ] ]

def psat fn p:Function Function [
  ( fn s:String Map [
      def r (pitem s)
      if (r.ok) [ if (p (r.val)) [ r ] [ {ok:false rest:s val:None} ] ]
                [ r ] ] ) ]

def palt fn [[a:Function b:Function][Function][
  ( fn s:String Map [ def r (a s)  if (r.ok) [ r ] [ (b s) ] ] ) ]]

def pseq fn [[a:Function b:Function][Function][
  ( fn s:String Map [
      def r1 (a s)
      if (r1.ok)
        [ def r2 (b (r1.rest))
          if (r2.ok) [ {ok:true val:[(r1.val) (r2.val)] rest:(r2.rest)} ]
                     [ {ok:false rest:s val:None} ] ]
        [ {ok:false rest:s val:None} ] ] ) ]]

def manyloop fn [[a:Function s:String acc:List][Map][
  def r (a s)
  if (r.ok) [ (manyloop a/v (r.rest) (push (r.val) acc)) ]
            [ {ok:true val:acc rest:s} ] ]]
def pmany fn a:Function Function [
  ( fn s:String Map [ def z [] (manyloop a/v s z) ] ) ]

def isdigit c:String => [ and (gte "0" c) (lte "9" c) ]
def digit  (psat isdigit/v)
def digits (pmany digit/v)
def ab     (palt (psat (c:String => [eq "a" c])) (psat (c:String => [eq "b" c])))
def two    (pseq digit/v digit/v)

print (digits "123ab")
print (ab "bzz")
print (ab "zzz")
print (two "42x")
```

```
{"ok": true, "val": ["1", "2", "3"], "rest": "ab"}
{"ok": true, "val": "b", "rest": "zz"}
{"ok": false, "rest": "zzz", "val": null}
{"ok": true, "val": ["4", "2"], "rest": "x"}
```

Identical on both engines. It does **not** pass `boru check` — see §5.5.

### 1.6 Everything else that held up

| Capability | Verdict |
|---|---|
| Closures capturing fn-locals, escaping their scope | ✅ `def mk fn k:Integer Function [ def loc (mul 10 k) (fn x:Integer Integer [add loc x]) ]`; `(mk 3)` then called with `1` → `31` |
| Two closures from one factory keep distinct captures | ✅ `(mk 1)` → `1`, `(mk 100)` → `100` |
| Functions in a list, retrieved and called | ✅ `((fs get 1) 5)` → `10` |
| Functions in a map; dynamic key dispatch | ✅ `((tbl get k) 5)` → `10` |
| Pipeline over a list of functions | ✅ `fold [apply] fs 5` → `12` |
| Named `compose` returning a `Function` | ✅ `(compose (a:Integer => [add 1 a]) (a:Integer => [mul 2 a]))` applied to `5` → `11` |
| `memoize` over a captured `flex` cache | ✅ MISS/hit/MISS |
| Mutable capture (`flex` mutated through a closure) | ✅ `[1 2 3]` |
| `usurp` (`/u`) wraps a fn and the wrapper is storable | ✅ `(sub2 10 3)` → `7`; `(sub2/u 10 3)` → `-7`; `def g (f2/u)` then `(g 10 3)` → `-7` |
| Tail recursion, 1 000 000 deep | ✅ constant space; bounded by the 10M **step** limit, not the tape |
| Non-tail recursion, 100 000 deep | ✅ compiled lane: `sum 100000` → `5000050000`. **Re-assessed 2026-08-21:** the interpreter (`-no-compile`) now trips the 396 718-entry tape ceiling near depth 40 000, so this depth passes only under the default lane |
| Non-tail recursion, 1 000 000 deep | ✅ *clean* refusal naming the tape ceiling and the remedy |
| `gensym` | ✅ exists (`LISP-ANALYSIS.5.md` grading it **D** is stale) |

---

## 2. Where boru sits among functional languages

Reading across Haskell, OCaml/SML, Scheme, Clojure, JavaScript, and the
concatenative peers (Factor, Joy):

| Capability | Haskell | Scheme | JS | Factor | **boru** |
|---|---|---|---|---|---|
| First-class functions | ✓ | ✓ | ✓ | ✓ (quotations) | **✓** |
| Lexical closures | ✓ | ✓ | ✓ | ✓ | **✓ for fn-locals; non-locals resolve late (§5.6)** |
| Anonymous lambda syntax | `\x ->` | `lambda` | `=>` | `[ ]` | **`x:T => [body]`** |
| Returning a function | ✓ | ✓ | ✓ | ✓ | **✓** |
| Auto-currying | ✓ | ✗ | ✗ | ✗ | **✗** (manual nesting) |
| Partial application | ✓ | SRFI-26 | `.bind` | `curry` | **mechanism ✓, named combinator ✗** — see below |
| Named `compose` / `pipe` | `.` `>>>` | `compose` | — | `compose` | **✗ — user-written** |
| `flip` | ✓ | ✓ | — | `swap` | **≈ `usurp` / `/u`** (2-arg) |
| Function type in the type system | `a -> b` | — | — | stack effects | **✓ where annotated — `fnsig` (see below); bare `Function` opaque** |
| Polymorphic identity `λx.x` | ✓ | ✓ | ✓ | ✓ | **✓ since `/v` — `t:Any => [t/v]` (§5.2)** |
| Dataflow shufflers (`dip`/`keep`/`bi`) | n/a | n/a | n/a | ✓ | **✗** |
| Laziness / infinite structures | ✓ | promises / SRFI-41 | generators | library | **✗ — strict, materialised** |
| Callback uniformity across containers | ✓ | ✓ | ✓ | ✓ | **✗ (§5.3)** |
| Guaranteed TCO | ✓ | ✓ (required) | ✗ | ✓ | **effectively ✓** (1M deep, constant space) |

Three entries deserve emphasis.

**Partial application exists; the *word* does not.** `curryOrStack`
(`core/go/engine.go:7923`) packages an under-supplied forward call — word
plus collected args — into a value that completes when it is later
expanded, and `lang/go/test/basic.tsv` exercises it across the arithmetic
words (`def add5 word [add 5]` then `10 add5` → `15`). So the row is not
"boru cannot partially apply"; it is that there is no first-class
`partial`/`curry` word taking a `Function` and some arguments and
returning a `Function`.

**`Function` is opaque — where left bare.** At the audit's writing this
read "there is no way to write a function from `Integer` to `String`";
`fnsig` has since closed that where you annotate (see the re-assessment
below). `tpartial` is unrelated (it makes record fields optional). The
cost of a *bare* `Function` is unchanged: a call through it carries no
static information at all — the checker passes it, and the mismatch
surfaces only at run time:

```
$ cat sa.boru
def sa fn x:Function Any [(x x)]  def i t:Any => [t/v]  print (sa i/v)
$ boru check sa.boru
check: 0 error(s), 0 warning(s), 0 info
$ boru run sa.boru
error: [boru/signature_error]: x is still waiting for 1 argument(s) when
  `x` begins its own dispatch — a function word is a barrier and never
  feeds forward collection (strict rule); group the call in parens so its
  RESULT becomes the argument: x (x …)
```

A clean check on a program that cannot run is the shape of the cost.
Compare Haskell's `(a -> b) -> [a] -> [b]`, which makes `map` both
checkable and self-documenting. boru's `map`-equivalent can only say
`Function` — or, now, a named `fnsig` type.

> **Re-assessed 2026-08-21.** `fnsig` merged with the same branch that
> merged this audit (2026-08-19) — the prototype
> `design/FUNCTION-TYPES.0.md` described as unmerged shipped with it,
> so this row was stale on arrival. `def IntToStr fnsig Integer String`
> mints an enforceable fn-shape type: a wrong-shaped argument is
> refused by the checker AND by dispatch (verified under `-no-check`,
> contravariant parameters / covariant return), and the `sa` program
> above, rewritten with `x:AA` for `def AA fnsig Any Any`, is caught at
> CHECK time — the exact clean-check-cannot-run failure this paragraph
> records moves to check time where you annotate. Since NUR095's fix
> (2026-08-20) a fn stored through a named class's fnsig-typed field
> applies on both engines (`lang/spec/class.tsv` §fn-members), with the
> checker still modelling that member apply as inert — NUR096. The
> transcript above still reproduces verbatim: a bare `Function`
> parameter stays opaque, but that is now a choice, not a limit.

**boru's real combinator strength is elsewhere.** The
`usurp`/`stack-args`/`forward-args`/`force-arity` family adapts a
function's *calling convention* and composes — closer to Factor's
`dip`/`keep` algebra than to anything in Scheme, and it has no
counterpart in the ML family. That is a genuine differentiator, and it is
under-advertised relative to the missing `compose`/`curry` staples.

---

## 3. The one rule that explains most of the surprises

ADR-011: *"A function value is always the inert, referenceable thing —
**calling is an act of the use site**. … A bare name bound to a function
calls; a value at the pointer dispatches; `/v` takes the reference."*

That rule is coherent, and it is the source of every §5 hazard. In an
applicative language `f` denotes the function and `f(x)` calls it. In
boru `f` *calls*, and `f/v` denotes. Combinator code is precisely the
code that mostly wants to **denote**.

---

## 4. The idiom sheet

What a higher-order programmer needs, in one place. None of this is
currently in `REFERENCE.md` or `HOWTO.md`.

| Intent | Write | Not |
|---|---|---|
| Pass a function as an argument | `hof f/v xs` | `hof f xs` (works today, but NUR078 will remove it) |
| Return a parameter — **any** kind | `[p/v]` | `[p]` — this **calls** `p` when `p` holds a function |
| Read an *enclosing* fn's parameter | `[p/v]` — `/v` closes over like any name | `(args).N` — the *inner* fn's list, and no longer needed here |
| Apply a computed function inside a body | `def h (g 1)` then `v h/v apply` | `((g 1) v)` — yields two values |
| Callback for `each`/`fold`/`scan` over a **list** | `each [f] xs` (quotation) | `each f/v xs` — no such signature |
| Callback for `filter` over either shape | `filter f/v xs` — the callback gets a `{key value}` pair over a list, a `KeyVal` over a map, never the bare element | — (`filter` is the only one of the five with a list `Function` form) |
| Let a *user-defined* word take a quotation | `hof (codequote [body]) xs` | `hof [body] xs` — evaluated at the call site |
| Computed key for the **quoting** accessor `set` | `set (k) v m` | `set k v m` — silently uses the literal `"k"` |
| Computed key for `get` — and, since 2026-08-21, `has` | `m get k` / `m has k` — both **evaluate** the key (`lang/spec/accessor.tsv`) | — no parens needed; a literal name is `'k'` or `k/q` |

`/v` deserves its own line, because one spelling now covers every
binding kind — a fn (call suppressed), a non-fn (identity), and a
parameter whose kind is not known statically:

```boru
def ident t:Any => [t/v]
print (ident 5)                                   ;# 5
print (ident "s")                                 ;# s
def g (ident (z:Integer => [add 1 z]))
print (g 5)                                       ;# 6
```

It closes over an enclosing fn's parameter like any other name, so the
boxing step the first pass needed is gone:

```boru
def mk fn t:Any Function [ ( fn f:Any Any [ t/v ] ) ]
def c (mk 7)
def d (mk (z:Integer => [add 1 z]))
print (c 0)                                       ;# 7
def g (d 0)
print (g 5)                                       ;# 6
```

`(args).N` still reads a parameter positionally and is still the way to
reach an argument that has no name, but it is no longer the *only* total
read, and the two idioms this audit had to invent — `(args).N` for a
polymorphic slot and boxing for an enclosing one — are both retired.

### 4.1 Forward form reverses the operands

Forward call form is canonical for new code except for the two-argument
words convention reads as infix operators, which house style writes infix
(`AGENTS.md`, `STYLE-GUIDE.md` §S2). The two spellings are not
interchangeable: moving an operand across the word moves it to a
different signature position, so for a non-commutative binary word they
compute **different values**, and nothing in the expression says which
you got:

```
$ boru do 'sub 10 3'    →   -7
$ boru do '10 sub 3'    →    7
$ boru do 'lte 5 1'     →   true
$ boru do '5 lte 1'     →   false
```

`lte x y` is `y ≤ x`; `sub a b` is `b − a`. So "n ≤ 1" is `n lte 1`
infix, or `lte 1 n` written all-forward; "n − 1" is `n sub 1` infix, or
`sub 1 n` all-forward. Transcribing a guard from infix habit into
forward form silently inverts it — a factorial written with `lte n 1`
returns `1` for every input instead of recursing, exit 0. That is not hypothetical:
it happened while converting this note's examples, which is why every one
of them was re-run after conversion.

### 4.2 Signature spellings — six ways to write one signature

Every `fn` in this note uses the fewest square brackets that express its
signature ([`STYLE-GUIDE.md`](../STYLE-GUIDE.md) §S1). For the common
one-parameter, one-return case there are **six** valid spellings, and
they are the same *value*, not merely the same answer:

```boru
def s1 fn x:Integer Integer [mul 2 x]        ;# 1 bracket pair  <- least
def s2 fn x:Integer [Integer] [mul 2 x]      ;# 2
def s3 fn [x:Integer Integer [mul 2 x]]      ;# 2
def s4 fn [[x:Integer] Integer [mul 2 x]]    ;# 3
def s5 fn [x:Integer [Integer] [mul 2 x]]    ;# 3
def s6 fn [[x:Integer] [Integer] [mul 2 x]]  ;# 4  <- fully canonical

print (s1 21) print (s2 21) print (s3 21)
print (s4 21) print (s5 21) print (s6 21)
print (deq (canon s1/v) (canon s6/v))
print (deq (canon s2/v) (canon s6/v))
print (deq (canon s3/v) (canon s6/v))
print (deq (canon s4/v) (canon s6/v))
print (deq (canon s5/v) (canon s6/v))
```

```
42 42 42 42 42 42        (one per line)
true true true true true (one per line)
```

`check: 0 error(s)`. The bodies' brackets are not optional — a body is
always a list — so one pair is the floor.

These eleven rows are no longer only a transcript — both halves are
pinned, so the equivalence is re-checked on every `make test` rather
than only on the day this note was written: the six spellings computing
`42` are spec rows at `lang/spec/fn-triple.tsv` §2b, and the five
`canon` equalities are `TestFnSignatureSpellingsAreOneValue` in
`lang/go/test/fn_triple_compiled_test.go`. They are split because
`canon` of a function value refuses to compile (Stage 3, soundness) and
the spec corpus holds `refusalCeiling = 0`.

**A bracketed input is not a longer spelling; it is a different form.**

```
$ boru do 'def f fn [x:Integer] [Integer] [mul 2 x] end f 21'
error: [boru/fn_error]: fn: list length must be a non-zero multiple of 3
  (input output body triples); use `fnsig` for the type-only form, or the
  3-arg form `fn input output body` for a single triple with a non-list input
```

`fn` has two signatures — `[(tnot List) Any List]` and `[List]`. A list
in the input position selects the second, so `[x:Integer]` is read as a
spec list of length 1 and the following `[Integer] [mul 2 x]` strand as
separate arguments. `boru describe fn` states it: *"a list input always
selects the spec-list form."*

**What cannot be reduced**, and so is not a style violation:

| shape | why the spec list is required |
|---|---|
| `fn [[a:Integer b:Integer] [Integer] [add a b]]` | 2+ parameters — the input is a list |
| `fn [[] [Integer] [42]]` | 0 parameters — `[]` is a list |
| `fn [[a:Integer] [Integer] […] [s:String] [String] […]]` | overloads need one list of triples |

Overloads whose inputs are each a single parameter also take the flat
triple spelling, which is shorter: `fn [a:Integer Integer [add 1 a]
s:String String [s]]` builds the same two-overload function.

**`boru fmt` implements this rule only from the fully canonical
spelling** — `fn [[x:Integer] [Integer] [mul 2 x]]` becomes
`fn x:Integer Integer [mul 2 x]`, while `s2`–`s5` above pass through it
untouched. So a file can be `fmt`-clean and still carry four spellings of
one signature. Recorded as **NUR088**.

### 4.3 Curried-lambda spellings — three ways to nest an arrow

Every curried combinator in §1 nests one `=>` inside another, and there
are three ways to write the nesting. They are **not** three spellings of
one value:

```boru
def kk x:Any => [y:Any => [x]]    ;# A — inner lambda in a code list
def kk x:Any => (y:Any => [x])    ;# B — inner lambda in a paren group
def kk x:Any => y:Any => [x]      ;# C — chained arrows, no wrapper
```

All three answer `7` for `((kk 7) 99)`, on the interpreter and the
compiler alike, and check identically. The difference is structural, and
`canon` is what sees it:

```
A  fn [[x:Any][Any][({y:word(Any)} sugar(lambda) [word(x)])]]
B  fn [[x:Any][Any][(({y:word(Any)} sugar(lambda) [word(x)]))]]
C  fn [[x:Any][Any][({y:word(Any)} sugar(lambda) [word(x)])]]
```

**At two levels A and C are the same value** — `deq (canon a/v) (canon
c/v)` is `true` — and **B is not**: its explicit parens are one group
more than the arrow already supplies, so its body canons doubled,
`[((…))]` against `[(…)]`.

At three levels even A and C diverge:

```
A  …[({g:…} sugar(lambda) [({x:…} sugar(lambda) [word(f) (g x)])])]…
C  …[({g:…} sugar(lambda)  ({x:…} sugar(lambda) [word(f) (g x)]) )]…
```

**A wraps each nested lambda in its own code list; C makes the inner
lambda the enclosing body directly.** The two-level case coincides only
because the OUTERMOST body is `fn`'s body slot, which auto-wraps a
non-list body into a one-element list (`ParseFnDef`'s abbreviation rule,
`lang/spec/fn-triple.tsv`). Inner lambda bodies get no such wrap — they
keep whatever the source wrote. So the deeper the currying, the more the
three spellings diverge as values while agreeing as programs.

**Which to write.** C is the fewest brackets (§S1b) and, at two levels,
literally the same value as A. But the equality does not survive a third
level, so "A and C are interchangeable" is true only for the shallowest
case — do not generalise it. This note keeps **A** throughout §1,
because a nested combinator basis is exactly where the equality stops
holding and the bracketed form is the one whose structure matches its
indentation.

**Where the difference bites.** Nowhere in evaluation — but `canon` is
the input to content-addressed identity
(`design/CONTENT-ADDRESSING.0.md`), so A, B and C hash as three
different functions. That is the same class as **NUR074** (`canon`
renders parameter names, so alpha-equivalent functions digest
differently): a purely syntactic choice moving a content hash.

---

## 5. Gotchas, ranked by how quietly they fail

### 5.1 A capitalised name bound to a function never calls — silently

```
$ boru do 'def I x:Integer => [add 1 x] end I 5'
I 5
$ echo $?
0
```

> **Re-assessed 2026-08-21.** This stranded pair printed
> `fn (Integer) 5` when the audit was written. Since the type-node
> fusion the bare capitalised name evaluates to the minted lattice
> node, which renders as its own name — so the wrong stack now looks
> like an unevaluated echo of the input, which is quieter still.

`def <Capitalised>` installs a **type** binding, not a value binding
(`eng/go/CLAUDE.md:655`, `core/go/core_type.go::InstallType`,
`lang/go/CLAUDE.md:904`). Given a function body this mints a **predicate
type** — a real and useful feature:

```
$ boru do 'def Even fn n:Integer Boolean [eq 0 (mod 2 n)] end 4 is Even'
true
```

The collision is with convention: the combinator literature is `S`, `K`,
`I`, `B`, `C`, `W`, `Y` — all capitals. A reader transcribing them gets
no error, exit 0, and a wrong stack.

**Workaround:** lowercase names — now the only one. The escape hatch
this audit originally recorded, `5 I/v apply` → `6`, no longer exists:
the valof flip pinned `/v` on a type binding to the minted node for
every declaration kind (`lang/spec/valof.tsv` §11), identical to bare
evaluation, so the same spelling now refuses with
`[boru/signature_error]` — "cannot call `apply` … expected Reach, got
I (an I)" — exit 1, on both engines. The recorded fn body stays reachable
only as the node's *content* (`TypeContentOf`), which has no surface
spelling.
**Severity:** silent wrong answer. **Not a bug** — but it costs a
diagnostic, and the case for one is stronger now that no `/v` spelling
recovers the value. `I 5` leaving a type node and an `Integer` stranded
is exactly the shape a hint could catch. The same silent-stranding
class has since been caught at declaration time too: a malformed `fn`
whose output slot is bare (`def f fn List Any [1]`) strands its
operands and binds nothing, exit 0, where its bracketed-output twin
raises — recorded as **NUR091**.

### 5.2 ~~Naming a `Function`-valued parameter calls it~~ — RESOLVED

**This finding is closed.** It was the audit's headline gap: a bare name
CALLED a function-bound parameter, `/r` REFUSED a non-function one, and
the `x is Function` guard could not be written either because naming `x`
started the call — so no spelling through a parameter's bound name was
total over both kinds, and the identity function had none.

The fix (2026-08-19, this branch): `/r` became **`/v`** and `ref` became
**`valof`**, and the function-only gate was removed. `/v` now denotes the
binding's VALUE whatever kind it is — for a fn binding it suppresses the
call, for anything else it is the identity:

```
$ boru do 'def idf t:Any => [t/v] end (idf 5)'
5
$ boru do 'def idf t:Any => [t/v] end (idf "s")'
s
$ boru do 'def idf t:Any => [t/v] end def g (idf (z:Integer => [add 1 z])) end (g 5)'
6
```

One body, three kinds. The guard is writable now too, because `t/v`
takes the value instead of starting a call:

```
$ boru do 'def idf t:Any => [if (t/v is Function) ["fn"] ["not"]] end (idf 5)'
not
$ boru do 'def idf t:Any => [if (t/v is Function) ["fn"] ["not"]] end (idf (z:Integer => [z]))'
fn
```

And the construction §1.2 could not finish — Church `and`/`or` — now
works in the **naive** spelling, with no boxing and no `(args).N`
(§1.2). NUR085 is retired; `(args).N` still works and is no longer
needed for this.

The remaining edge is unchanged and is not this rule's: a bare `[t]`
still calls a function-bound parameter. That is ADR-011 working as
designed ("calling is an act of the use site"); `/v` is how you say you
meant the value.

### 5.3 `each`/`for-each`/`fold`/`scan` take a `Function` callback over a Map but not over a List

```
$ boru do 'def dbl x:Integer => [mul 2 x] end each dbl/v [1 2 3]'
error: [boru/signature_error]: cannot call `each` — no signature matches the arguments
  = note: candidate `each (Function, Map)` takes 2 arguments, but none were supplied
  = note: candidate `each (Reach, List)`  takes 2 arguments, but none were supplied
```

At source: `each` registers `{TFunction, TMap}` and no `{TFunction,
TList}` (`lang/go/native/native_array.go:326`); same for `for-each`
(`:348`), `fold` (`:393-394`) and `scan` (`:420`). `filter` — alone —
registers `{TFunction, TAny}` and so has a list `Function` form
(`lang/go/native/natives.go:261`).

Neither `Function` form is a plain element callback, though: `each`'s map
form hands the lambda a `KeyVal`, and `filter`'s list form hands it a
`{key, value}` pair (`lang/go/native/filter.go`). So an
`Integer -> Integer` function value cannot be handed directly to *any* of
the five — only a quotation naming it can.

**Workaround:** `each [f] xs`, or `each (kv:Any => [f (kv.v)]) m`.
**Severity:** confusing error; a learnability tax on the most-reached-for
words. Recorded as **NUR086**.

### 5.4 Applying a computed function value depends on the enclosing context

The same expression means two things:

```
$ boru do 'def mk fn a:Integer Function [(fn b:Integer Integer [add a b])] end ((mk 1) 2)'
3
$ boru do 'def mk fn a:Integer Function [(fn b:Integer Integer [add a b])] end print ((mk 1) 2)'
fn (Integer)
2
$ echo $?
0
```

`(mk 1)` *resolves* a function; the **call belongs to whatever encloses
the group** (`lang/spec/fn-value.tsv` §4, NUR035). At statement level the
statement applies it; under `print` the argument window merely collects
both. Inside a fn body the same shape yields a return-arity error:

```
error: [boru/type_error]: g: expected 1 return value(s), got 2 — [fn (Integer) 2]
```

(The prefix names the enclosing fn — `g` here, empty for an anonymous
one — and the message now carries a source span pointing at the group
and the declaration, `core/go/return_check_msg.go`. It read `ap:` when
the audit was written; same class, same payload.)

This is why the Z combinator diverges: `((x x) v)` never applies, so the
`λv` guard is inert.

**Workaround:** `def h (mk 1)` then `2 h/v apply` → `3`.
**Forward hazard:** NUR073's accepted **BROAD** verdict removes inline
application entirely — *"`(fn Integer [Integer] [10 add]) 7` becomes two
values and the inline-application idiom is removed"*. That idiom
currently answers `17`. Combinator code written against today's top-level
behaviour will change meaning when the fix lands.
**Severity:** silent wrong answer under `print`.

### 5.5 The checker rejects correct higher-order programs, and `boru run` obeys it

Minimal repro — a `def` inside an `if` branch becomes invisible once an
earlier `def` in the same body bound the result of a call **through a
`Function` parameter**:

```boru
def mk fn [[a:Function b:Function][Function][
  ( fn s:Integer Map [
      def r1 (a s)
      if (r1.ok)
        [ def r2 (b (r1.rest))
          if (r2.ok) [ {ok:true val:[(r1.val) (r2.val)] rest:(r2.rest)} ]
                     [ {ok:false rest:s val:None} ] ]
        [ {ok:false rest:s val:None} ] ] ) ]]
def h (mk (z:Integer => [ {ok:true val:1 rest:8} ])
          (z:Integer => [ {ok:true val:2 rest:9} ]))
print (h 1)
```

```
check: 6:15: [error] undefined_word: undefined word: r2
  = help: did you mean `r1`?
```

It runs correctly (`{"ok": true, "val": [1, 2], "rest": 9}`). Remove
`def r1 (a s)` — replace it with a literal — and the check passes. The
parser-combinator library of §1.5 hits this: plain `boru run` on it
**refuses to run it**, `check failed: 6 error(s)`, exit 1. `-no-check`
runs it correctly.

**Workaround:** `-no-check`, or hoist the binding.
**Severity:** a correct program is refused by the default invocation.
Recorded as **NUR087**. Post-audit, the checker-vs-runtime family
gained a member one layer up: NUR095's fix (2026-08-20) made a fn
stored through a fn-shape-typed class member apply in compiled
execution, but the check pass still models that member apply as the
inert pre-fix stack — recorded as **NUR096**.

### 5.6 Non-local names in a closure resolve late, through the def stack

```
$ boru do 'def n 1 end def c x:Integer => [add n x] end def n 100 end (c 5)'
105
$ boru do 'def n 1 end def c x:Integer => [add n x] end def n 100 end undef n end (c 5)'
6
```

fn-**locals** are captured properly and outlive their scope (§1.6). Names
from the enclosing module scope are not captured — they are resolved at
call time against the shadowing stack. This matches JavaScript's
behaviour for a reassigned outer binding, but `def n 100` is a *shadow
push*, not an assignment, so an ML/Haskell reader expecting `6` gets
`105`.

**Severity:** confusing; matters whenever a combinator is defined before
a name it mentions is re-`def`ed. Recorded as **NUR097** (2026-08-21),
with the proposed verdict *Allowed plus an in-file diagnostic* — the
late half is the top-level liveness contract, not a defect. The
freezing idiom is parameter capture:
`def mkc fn nn:Integer Function [(fn x:Integer Integer [add nn x])]`
then `def c (mkc n)` → `6` however `n` is later re-`def`ed. All three
behaviours (105, the post-`undef` 6, and the frozen 6) are pinned as
`lang/spec/frontier/frontier-hof-audit.tsv` §8, and the contract is
documented in `REFERENCE.md` §"Definition and scoping".

### 5.7 The engines disagree — NUR073, live today

```
$ cat n73.boru
def z fn [[][Integer][42]]  def h fn f:Function Any [f/v]  ((h z/v))

$ boru run -no-check -no-compile n73.boru     # interpreter
42
$ boru run -no-check -force-compile n73.boru  # compiler
fn z
$ boru run n73.boru                           # the plain default — exit 0
fn z
```

`NUR.md` records the divergence as *"the only such row in the corpus"*;
what its text does not say is that the **default** invocation now sides
with the compiler, so `-no-compile` and the default disagree, silently,
exit 0 both ways. For higher-order code the choice of engine is a choice
of semantics.

**Severity:** highest. Two supported invocations of the same program
print different answers with no diagnostic.

### 5.8 Compilation refuses nearly all curried higher-order code

Every curried combinator in §1.1–1.2 draws the same refusal:

```
warning: bytecode compilation refused, ran on the interpreter (slower):
  fn ss: body result of unknown provenance
```

Other observed reasons: `fn-value application bounded by a paren (dynamic
value precedes args)`, and NUR037's fn-local-fn-as-body-word refusal.

This is the **right** behaviour — `design/COMPILABLE-SUBSET.md`'s "slow,
not wrong" — and unlike §5.7 it is *announced* on stderr. But it means
higher-order style opts out of the bytecode VM as a rule, not an
exception. Worth stating plainly in the docs so the performance
trade-off is a choice rather than a surprise.

### 5.9 Smaller edges met while writing the programs

- **Quotations are not closures.** A `codequote`d body does not capture
  fn-locals: `def mkq fn n:Integer List [(codequote [add n])]` then
  `each q [1 2 3]` → `cannot call add`. Only `Function` values close.
- **User-defined words cannot take an unevaluated quotation.**
  `hof [mul 2] xs` evaluates the bracket at the call site;
  `NoEvalArgs` is a native-word privilege. Use `codequote`.
- **A computed key for a *quoting* accessor must be parenthesised.**
  At the audit's writing `cache has k` silently looked up the literal
  `"k"` and returned `false` — NUR040's class, and what made a first
  `memoize` attempt miss on every call while the cache visibly filled.
  **Corrected 2026-08-21:** `has` now evaluates its key exactly as
  `get` does, so that failure mode is gone for `has` — a bound bare
  key computes, and an unbound one raises `undefined_word` loudly.
  `set` remains the quoting member of the family (NUR040, Allowed):
  its computed key still needs `(k)`.
- **`flex` map keys must be Strings.** An `Integer` key is refused, which
  bites every memo table.
- **No comparator-based sort.** `sort` takes no callback;
  `ArrayUtil.sortby` takes a *parallel key list*, not a comparator, so
  ordering by a user function needs a Schwartzian transform.
- **User fn names collide with core words** with a hard-to-read error:
  `def outer fn [[a:Function]…]` → `[boru/extend_owner]: a core word can
  be extended only with at least one NOMINAL argument type the extending
  scope owns`.
- **No laziness.** `size (take 3 (iota 10000000))` answers `3` in 5.3s —
  the ten million elements are built first — and `iota` caps at
  16 777 216 (`[boru/iota_error]: iota: count … exceeds the cap`). No
  infinite sequences, no generators; a `take`-from-an-unbounded-stream
  program has no expression.
- ~~**No `while`.**~~ **Closed 2026-08-21:** `while [cond] [body]` is a
  word — condition re-evaluated per iteration, body values accumulate,
  `break`/`continue` as in `for`, step-budget-bounded (see
  `REFERENCE.md` §"`while` — the condition loop";
  `lang/spec/frontier/frontier-while.tsv`). At the audit's writing,
  `for` was a numeric range with `break` and loops over a condition
  were recursion.

---

## 6. Recommendations, cheapest first

1. ~~**Document the `(args).N` idiom.**~~ **Done differently, and
   better:** `/r`→`/v` with the function-only gate removed makes `/v`
   itself the total read, so there is no folklore idiom left to document
   (§5.2). `boru describe valof` and `lang/spec/valof.tsv` §9 carry the
   rule; `REFERENCE.md`/`HOWTO.md` were swept to the new spelling.
2. **Add a diagnostic for §5.1.** A statement ending with a stranded
   minted type node beside unconsumed operands — §5.1's shape since the
   type-node fusion — is a near-certain typo, and since the valof flip
   removed the `I/v apply` escape hatch a hint is the only help left.
   `undefined_word` already offers "did you mean"; this deserves the
   same.
3. **Give `each`/`for-each`/`fold`/`scan` a `{TFunction, TList}`
   signature** (NUR086). Note this buys *signature availability*, not
   callback uniformity: `filter`'s list `Function` form passes a
   `{key,value}` pair, so a matching `filter` change is needed before an
   element-shaped callback works across the whole family.
4. ~~**Ship the missing vocabulary as a module**~~ — **Done
   (2026-08-21):** `boru:fn-util` ships `compose`, `pipe`, `curry`,
   `partial`, `const`, `identity`, `flip`, `on`, `memoize` as native
   words next to `boru:type-util` (`lang/go/modules/fn.go`;
   `REFERENCE.md` §"The `boru:fn-util` module"; behaviour rows in
   `lang/spec/frontier/frontier-fn-util.tsv`, ledgered under the
   def-bound-computed-fn refusal until that family graduates).
5. **Fix NUR087** — the checker refusing a correct closure is the only
   finding that stops a working program from running at all.
6. **Land NUR073.** Until it lands, `-no-compile` and the default are two
   languages for higher-order code, and the difference is silent.

---

## 7. Where the LISP analysis needs updating

`LISP-ANALYSIS.5.md` §3 says *"inner fns snapshot enclosing-fn locals"* —
confirmed for locals, but §5.6 shows non-locals are resolved late, which
the grade should reflect. Its §0 grades `gensym` **D**; `gensym` exists
today. Its "Gaps" list (no `compose`/`curry`/`partial`/`const`/
`identity`/`flip`-by-name) is right as a *vocabulary* claim, though
partial application itself does exist as a mechanism (§2). §4's call for
Factor's `dip`/`keep`/`bi` still stands.

The grade this audit would give: **substrate A−, surface B−** (was C+
before `/v`). Nothing is missing from the machine, and the biggest
surface gap — no spelling for "the value, whatever kind it is" — is now
closed: `/v` is it. What is still missing is a uniform callback
convention (§5.3) and the point-free vocabulary that would make
hand-rolling `compose`/`curry`/`partial` unnecessary.
