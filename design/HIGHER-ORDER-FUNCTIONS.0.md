# Higher-Order Functions in boru — capability audit

**Status: point-in-time report (2026-08-19).** A dated empirical snapshot,
not a living reference — see `design/README.md`. Its claims describe the
engine as built from this tree on that date; several of them are pinned to
NUR records whose fixes will change the answers.

**Method:** empirical, against `cmd/go/bin/boru` built from this tree,
cross-read with `core/`, `lang/`, `NUR.md`, `ADR.md` and
`lang/spec/*.tsv`. Every claim below is either a source citation or a
command that was run and whose output is quoted verbatim. Examples are in
the canonical **forward call form** (`AGENTS.md` §"Working in the code")
and every one of them was re-run in that spelling — see §4.1 for why that
re-run is not a formality.

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
| Can all combinators be implemented? | **Almost.** S, K, I, B, C, W, U, Church numerals and pairs, Church `true`/`false`/`if`/`not`, CPS, and a working parser-combinator library were all built and run. Church `and`/`or` — which must return one *named* boolean unchanged — resisted every spelling tried (§1.2). |
| Is it *pleasant*? | **No.** The polymorphic ones need two escape hatches (`(args).N` and boxing) that no document names, and the naive spelling fails. |
| Is it *safe*? | **Not yet.** Three silent-wrong-answer classes and one interpreter-vs-compiler divergence, listed in §5. |

**One-line summary:** boru's function substrate is close to complete —
every combinator in the audit except Church `and`/`or` was built and run —
but the surface has a *call-vs-value* ambiguity that is resolved
differently at four different sites, and the correctness of a higher-order
program can depend on which engine runs it.

---

## 1. What works — the evidence

Every program below was written and run against this tree, under both
`-no-compile` (interpreter) and the default (bytecode with interpreter
fallback), and is quoted here in full — definitions **and** the calls that
produced the quoted output — so it can be re-run from the note itself.

### 1.1 The classical combinator bases

`I = S K K`, derived rather than assumed:

```boru
def kk ([x:Any] => [([y:Any] => [x])])
def ss ([f:Function] => [([g:Function] => [([x:Any] => [(f x) (g x)])])])
def ii ((ss kk/r) kk/r)
print (ii 42)        ;# 42
print (ii "hello")   ;# hello
```

`check: 0 error(s)`, correct on both engines. **BCKW** likewise:

```boru
def bb ([f:Function] => [([g:Function] => [([x:Any] => [f (g x)])])])
def cc ([f:Function] => [([x:Any] => [([y:Any] => [(f y) x])])])
def kk ([x:Any] => [([y:Any] => [x])])
def ww ([f:Function] => [([x:Any] => [(f x) x])])
def add2 ([a:Integer] => [([b:Integer] => [add a b])])
print (((bb ([n:Integer] => [mul n 2])) ([n:Integer] => [add n 3])) 4)  ;# 14
print (((cc add2/r) 1) 10)                                             ;# 11
print ((kk 7) 99)                                                      ;# 7
print ((ww add2/r) 4)                                                  ;# 8
```

### 1.2 Church encodings

Numerals and pairs work unmodified:

```boru
def czero ([f:Function] => [([x:Any] => [x])])
def csucc ([n:Function] => [([f:Function] => [([x:Any] => [f ((n f/r) x)])])])
def cplus ([m:Function] => [([n:Function] => [([f:Function] => [([x:Any] =>
             [((m f/r) ((n f/r) x))])])])])
def cmult ([m:Function] => [([n:Function] => [([f:Function] => [(m (n f/r))])])])
def toint ([n:Function] => [((n ([k:Integer] => [add k 1])) 0)])
def c1 (csucc czero/r)
def c2 (csucc c1/r)
def c3 (csucc c2/r)
print (toint c3/r)                    ;# 3
print (toint ((cplus c2/r) c3/r))     ;# 5
print (toint ((cmult c2/r) c3/r))     ;# 6
```

```boru
def cpair ([a:Any] => [([b:Any] => [([s:Function] => [((s a) b)])])])
def cfst  ([p:Function] => [(p ([a:Any] => [([b:Any] => [a])]))])
def csnd  ([p:Function] => [(p ([a:Any] => [([b:Any] => [b])]))])
def p ((cpair 1) 2)
print (cfst p/r)   ;# 1
print (csnd p/r)   ;# 2
```

Church **booleans** do *not* work in their naive spelling — `λt.λf.t`
returns a parameter, and returning a `Function`-valued parameter is the
central hazard of this report (§5.2). They need **two** escape hatches.
`(args).N` reads the innermost fn's own argument list, so the *outer*
argument must first be boxed in a list and read back positionally:

```boru
def ctrue  fn [[t:Any][Function][ def bx [(args).0] ( fn [[f:Any][Any][ bx.0 ]] ) ]]
def cfalse fn [[t:Any][Function][ ( fn [[f:Any][Any][ (args).0 ]] ) ]]
def cif    fn [[p:Function t:Any e:Any][Any][ def r (p t) (r e) ]]
def cnot   fn [[p:Function][Function][ def bp [(args).0]
  ( fn [[t:Any][Function][ def bt [(args).0]
      ( fn [[f:Any][Any][ def qq ((bp.0) f) (qq (bt.0)) ]] ) ]] ) ]]
print (cif ctrue/r  "T" "F")          ;# T
print (cif cfalse/r "T" "F")          ;# F
print (cif (cnot ctrue/r)  "T" "F")   ;# F
print (cif (cnot cfalse/r) "T" "F")   ;# T
```

`cand` / `cor` were **not** completed. They must return one Church
boolean *as itself*, and every spelling tried hit one of two walls: with
named booleans, `lang/spec/fn-value.tsv` §5's loud-dispatch rule fires
(`[boru/uncalled_function]: call to 'cfalse' matched no signature`,
pointing back at the `cfalse/r` argument); holding them anonymously in a
list to dodge that rule then ran into forward-collection barriers
(`cand (bools.0) (bools.1)` → *"takes 2 arguments, but 1 was supplied"*).
This is the one construction in the audit that was not finished, and it
is a direct consequence of §5.2 rather than a separate defect.

### 1.3 Continuation-passing style

CPS works cleanly, including the closure-per-frame that CPS implies:

```boru
def factk fn [[n:Integer k:Function][Any][
  if (lte 1 n) [ (k 1) ]
               [ def kk ( fn [[r:Integer][Any][ def m (mul n r) (k m) ]] )
                 (factk (sub 1 n) kk/r) ] ]]
print (factk 5  ([v:Integer] => [v]))          ;# 120
print (factk 10 ([v:Integer] => [add 0 v]))    ;# 3628800
```

### 1.4 Anonymous recursion

The U-combinator form runs on **both** engines with a clean check:

```boru
def fgen fn [[s:Function][Function][
  ( fn [[n:Integer][Integer][ if (lte 1 n) [1] [ mul n ((s s) (sub 1 n)) ] ]] ) ]]
def fact (fgen fgen/r)
print (fact 5)       ;# 120
```

The full **Z combinator** (`λf.(λx.f(λv.(x x)v))²`) **diverges** — it
hangs until the step limit. The cause is not call-by-value eagerness:
lambda bodies were confirmed lazy (a `raise` inside an unapplied lambda
body never fires). The cause is §5.4 — `((x x) v)` does not parse as an
application inside a body, so the `λv` thunk never actually guards the
recursion.

### 1.5 A real parser-combinator library

The standard stress test for higher-order support — `item`, `sat`, `alt`,
`seq`, `many`, all returning closures over their arguments. A parser is a
`Function: String -> {ok val rest}`:

```boru
def pitem ([s:String] => [
  if (eq 0 (size s)) [ {ok:false rest:s val:None} ]
                     [ {ok:true val:(slice 0 1 s) rest:(slice 1 (size s) s)} ] ])

def psat fn [[p:Function][Function][
  ( fn [[s:String][Map][
      def r (pitem s)
      if (r.ok) [ if (p (r.val)) [ r ] [ {ok:false rest:s val:None} ] ]
                [ r ] ]] ) ]]

def palt fn [[a:Function b:Function][Function][
  ( fn [[s:String][Map][ def r (a s)  if (r.ok) [ r ] [ (b s) ] ]] ) ]]

def pseq fn [[a:Function b:Function][Function][
  ( fn [[s:String][Map][
      def r1 (a s)
      if (r1.ok)
        [ def r2 (b (r1.rest))
          if (r2.ok) [ {ok:true val:[(r1.val) (r2.val)] rest:(r2.rest)} ]
                     [ {ok:false rest:s val:None} ] ]
        [ {ok:false rest:s val:None} ] ]] ) ]]

def manyloop fn [[a:Function s:String acc:List][Map][
  def r (a s)
  if (r.ok) [ (manyloop a/r (r.rest) (push (r.val) acc)) ]
            [ {ok:true val:acc rest:s} ] ]]
def pmany fn [[a:Function][Function][
  ( fn [[s:String][Map][ def z [] (manyloop a/r s z) ]] ) ]]

def isdigit ([c:String] => [ and (gte "0" c) (lte "9" c) ])
def digit  (psat isdigit/r)
def digits (pmany digit/r)
def ab     (palt (psat ([c:String] => [eq "a" c])) (psat ([c:String] => [eq "b" c])))
def two    (pseq digit/r digit/r)

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
| Closures capturing fn-locals, escaping their scope | ✅ `def mk fn [[k:Integer][Function][ def loc (mul 10 k) (fn [[x:Integer][Integer][add loc x]]) ]]`; `(mk 3)` then called with `1` → `31` |
| Two closures from one factory keep distinct captures | ✅ `(mk 1)` → `1`, `(mk 100)` → `100` |
| Functions in a list, retrieved and called | ✅ `((fs get 1) 5)` → `10` |
| Functions in a map; dynamic key dispatch | ✅ `((tbl get k) 5)` → `10` |
| Pipeline over a list of functions | ✅ `fold [apply] fs 5` → `12` |
| Named `compose` returning a `Function` | ✅ `(compose ([a:Integer] => [add 1 a]) ([a:Integer] => [mul 2 a]))` applied to `5` → `11` |
| `memoize` over a captured `flex` cache | ✅ MISS/hit/MISS |
| Mutable capture (`flex` mutated through a closure) | ✅ `[1 2 3]` |
| `usurp` (`/u`) wraps a fn and the wrapper is storable | ✅ `(sub2 10 3)` → `7`; `(sub2/u 10 3)` → `-7`; `def g (f2/u)` then `(g 10 3)` → `-7` |
| Tail recursion, 1 000 000 deep | ✅ constant space; bounded by the 10M **step** limit, not the tape |
| Non-tail recursion, 100 000 deep | ✅ `sum 100000` → `5000050000` |
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
| Anonymous lambda syntax | `\x ->` | `lambda` | `=>` | `[ ]` | **`([x:T] => [body])`** |
| Returning a function | ✓ | ✓ | ✓ | ✓ | **✓** |
| Auto-currying | ✓ | ✗ | ✗ | ✗ | **✗** (manual nesting) |
| Partial application | ✓ | SRFI-26 | `.bind` | `curry` | **mechanism ✓, named combinator ✗** — see below |
| Named `compose` / `pipe` | `.` `>>>` | `compose` | — | `compose` | **✗ — user-written** |
| `flip` | ✓ | ✓ | — | `swap` | **≈ `usurp` / `/u`** (2-arg) |
| Function type in the type system | `a -> b` | — | — | stack effects | **✗ — `Function` is opaque** |
| Polymorphic identity `λx.x` | ✓ | ✓ | ✓ | ✓ | **⚠ only via `(args).0`, not through the parameter's name (§5.2)** |
| Dataflow shufflers (`dip`/`keep`/`bi`) | n/a | n/a | n/a | ✓ | **✗** |
| Laziness / infinite structures | ✓ | promises / SRFI-41 | generators | library | **✗ — strict, materialised** |
| Callback uniformity across containers | ✓ | ✓ | ✓ | ✓ | **✗ (§5.3)** |
| Guaranteed TCO | ✓ | ✓ (required) | ✗ | ✓ | **effectively ✓** (1M deep, constant space) |

Three entries deserve emphasis.

**Partial application exists; the *word* does not.** `curryOrStack`
(`core/go/engine.go:7965`) packages an under-supplied forward call — word
plus collected args — into a value that completes when it is later
expanded, and `lang/go/test/basic.tsv` exercises it across the arithmetic
words (`def add5 word [add 5]` then `10 add5` → `15`). So the row is not
"boru cannot partially apply"; it is that there is no first-class
`partial`/`curry` word taking a `Function` and some arguments and
returning a `Function`.

**`Function` is opaque.** There is no way to write "a function from
`Integer` to `String`". `tpartial` is unrelated (it makes record fields
optional). The cost is real: the checker cannot verify a call through a
`Function` parameter, which is exactly why self-application draws
`no_signature: cannot call x … got (Function)`. Compare Haskell's
`(a -> b) -> [a] -> [b]`, which makes `map` both checkable and
self-documenting. boru's `map`-equivalent can only say `Function`.

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
calls; a value at the pointer dispatches; `/r` takes the reference."*

That rule is coherent, and it is the source of every §5 hazard. In an
applicative language `f` denotes the function and `f(x)` calls it. In
boru `f` *calls*, and `f/r` denotes. Combinator code is precisely the
code that mostly wants to **denote**.

---

## 4. The idiom sheet

What a higher-order programmer needs, in one place. None of this is
currently in `REFERENCE.md` or `HOWTO.md`.

| Intent | Write | Not |
|---|---|---|
| Pass a function as an argument | `hof f/r xs` | `hof f xs` (works today, but NUR078 will remove it) |
| Return a `Function`-typed parameter | `[p/r]` | `[p]` — this **calls** `p` |
| Return a parameter of *unknown* kind | `[(args).0]` | `[p]` or `[p/r]` — each fails for one kind |
| Read an *enclosing* fn's parameter of unknown kind | box it: `def bx [(args).0]` outside, `bx.0` inside | `(args).N` inside — that is the *inner* fn's list |
| Apply a computed function inside a body | `def h (g 1)` then `v h/r apply` | `((g 1) v)` — yields two values |
| Callback for `each`/`fold`/`scan` over a **list** | `each [f] xs` (quotation) | `each f/r xs` — no such signature |
| Callback for `filter` over either shape | `filter f/r xs` — the callback gets a `{key value}` pair over a list, a `KeyVal` over a map, never the bare element | — (`filter` is the only one of the five with a list `Function` form) |
| Let a *user-defined* word take a quotation | `hof (codequote [body]) xs` | `hof [body] xs` — evaluated at the call site |
| Computed key for a **quoting** accessor (`has`, `set`) | `m has (k)` | `m has k` — silently uses the literal `"k"` |
| Computed key for `get` | `m get k` — `get` **evaluates** the key (`lang/spec/accessor.tsv`) | — this one needs no parens |

The `(args).N` escape deserves its own line, because it is the only
parameter read that is total over both function and non-function
bindings, and it appears in no user-facing document:

```boru
def ident fn [[t:Any][Any][(args).0]]
def konst fn [[t:Any u:Any][Any][(args).0]]
print (ident 5)                                   ;# 5
print (ident "s")                                 ;# s
def g (ident ([z:Integer] => [add 1 z]))
print (g 5)                                       ;# 6
```

### 4.1 Forward form reverses the operands

`AGENTS.md` makes forward call form canonical for new code, and
`design/README.md` notes that `a f b` is "the lone non-equivalent two-arg
(swap) form". For a non-commutative binary word the two spellings compute
**different values**, and nothing in the expression says which you got:

```
$ boru do 'sub 10 3'    →   -7
$ boru do '10 sub 3'    →    7
$ boru do 'lte 5 1'     →   true
$ boru do '5 lte 1'     →   false
```

`lte x y` is `y ≤ x`; `sub a b` is `b − a`. So "n ≤ 1" is `lte 1 n` and
"n − 1" is `sub 1 n`. Transcribing a guard from infix habit into forward
form silently inverts it — a factorial written with `lte n 1` returns `1`
for every input instead of recursing, exit 0. That is not hypothetical:
it happened while converting this note's examples, which is why every one
of them was re-run after conversion.

---

## 5. Gotchas, ranked by how quietly they fail

### 5.1 A capitalised name bound to a function never calls — silently

```
$ boru do 'def I ([x:Integer] => [add 1 x]) end I 5'
fn (Integer) 5
$ echo $?
0
```

`def <Capitalised>` installs a **type** binding, not a value binding
(`eng/go/CLAUDE.md:611`, `eng/go/core_type.go::InstallType`,
`lang/go/CLAUDE.md:890`). Given a function body this mints a **predicate
type** — a real and useful feature:

```
$ boru do 'def Even fn [[n:Integer][Boolean][eq 0 (mod 2 n)]] end 4 is Even'
true
```

The collision is with convention: the combinator literature is `S`, `K`,
`I`, `B`, `C`, `W`, `Y` — all capitals. A reader transcribing them gets
no error, exit 0, and a wrong stack.

**Workaround:** lowercase names; or `5 I/r apply` → `5`.
**Severity:** silent wrong answer. **Not a bug** — but it costs a
diagnostic. `I 5` leaving a `Function` and an `Integer` stranded is
exactly the shape a hint could catch.

### 5.2 Naming a `Function`-valued parameter calls it; `/r` cannot be used defensively

The identity function is not writable through its parameter's name:

```
$ boru do 'def idf ([t:Any] => [t]) end def g (idf ([z:Integer] => [add 1 z])) end (g 5)'
error: [boru/signature_error]: cannot call `t` — no signature matches the arguments
```

`/r` fixes that case and breaks the other:

```
$ boru do 'def idf ([t:Any] => [t/r]) end (idf 5)'
error: [boru/illegal_ref]: /r requires a function word: t is bound to Integer
```

So `[t]` works iff `t` is *not* a function and `[t/r]` works iff it *is*
— **no spelling through the bound name is total over both.** Nor can the
program branch on it, because the test itself names the parameter:

```
$ boru do 'def idf ([t:Any] => [if (t is Function) [t/r] [t]]) end ...'
error: [boru/signature_error]: t is still waiting for 1 argument(s) when `is`
begins its own dispatch — a function word is a barrier …
```

This is what breaks naive Church booleans (`cand`/`cor` pass one Church
boolean as another's argument, so `λt.λf.t` must return a `Function`).

**Workaround:** `(args).N` (§4), which bypasses name dispatch entirely,
plus boxing to reach an enclosing fn's parameter.
**Severity:** loud error, but with no discoverable remedy — this is the
single biggest ergonomic gap for combinator work. Recorded as **NUR085**.

### 5.3 `each`/`for-each`/`fold`/`scan` take a `Function` callback over a Map but not over a List

```
$ boru do 'def dbl ([x:Integer] => [mul 2 x]) end each dbl/r [1 2 3]'
error: [boru/signature_error]: cannot call `each` — no signature matches the arguments
  = note: candidate `each (Function, Map)` takes 2 arguments, but none were supplied
  = note: candidate `each (Reach, List)`  takes 2 arguments, but none were supplied
```

At source: `each` registers `{TFunction, TMap}` and no `{TFunction,
TList}` (`lang/go/native/native_array.go:326`); same for `for-each`
(`:348`), `fold` (`:393-394`) and `scan` (`:420`). `filter` — alone —
registers `{TFunction, TAny}` and so has a list `Function` form
(`lang/go/native/natives.go:260`).

Neither `Function` form is a plain element callback, though: `each`'s map
form hands the lambda a `KeyVal`, and `filter`'s list form hands it a
`{key, value}` pair (`lang/go/native/filter.go`). So an
`Integer -> Integer` function value cannot be handed directly to *any* of
the five — only a quotation naming it can.

**Workaround:** `each [f] xs`, or `each ([kv:Any] => [f (kv.v)]) m`.
**Severity:** confusing error; a learnability tax on the most-reached-for
words. Recorded as **NUR086**.

### 5.4 Applying a computed function value depends on the enclosing context

The same expression means two things:

```
$ boru do 'def mk fn [[a:Integer][Function][(fn [[b:Integer][Integer][add a b]])]] end ((mk 1) 2)'
3
$ boru do 'def mk fn [[a:Integer][Function][(fn [[b:Integer][Integer][add a b]])]] end print ((mk 1) 2)'
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
error: [boru/type_error]: ap: expected 1 return value(s), got 2 — [fn (Integer) 2]
```

This is why the Z combinator diverges: `((x x) v)` never applies, so the
`λv` guard is inert.

**Workaround:** `def h (g 1)` then `v h/r apply` → `3`.
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
  ( fn [[s:Integer][Map][
      def r1 (a s)
      if (r1.ok)
        [ def r2 (b (r1.rest))
          if (r2.ok) [ {ok:true val:[(r1.val) (r2.val)] rest:(r2.rest)} ]
                     [ {ok:false rest:s val:None} ] ]
        [ {ok:false rest:s val:None} ] ]] ) ]]
def h (mk ([z:Integer] => [ {ok:true val:1 rest:8} ])
          ([z:Integer] => [ {ok:true val:2 rest:9} ]))
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
Recorded as **NUR087**.

### 5.6 Non-local names in a closure resolve late, through the def stack

```
$ boru do 'def n 1 end def c ([x:Integer] => [add n x]) end def n 100 end (c 5)'
105
$ boru do 'def n 1 end def c ([x:Integer] => [add n x]) end def n 100 end undef n end (c 5)'
6
```

fn-**locals** are captured properly and outlive their scope (§1.6). Names
from the enclosing module scope are not captured — they are resolved at
call time against the shadowing stack. This matches JavaScript's
behaviour for a reassigned outer binding, but `def n 100` is a *shadow
push*, not an assignment, so an ML/Haskell reader expecting `6` gets
`105`.

**Severity:** confusing; matters whenever a combinator is defined before
a name it mentions is re-`def`ed.

### 5.7 The engines disagree — NUR073, live today

```
$ cat n73.boru
def z fn [[][Integer][42]]  def h fn [[f:Function][Any][f/r]]  ((h z/r))

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
  fn-locals: `def mkq fn [[n:Integer][List][(codequote [add n])]]` then
  `each q [1 2 3]` → `cannot call add`. Only `Function` values close.
- **User-defined words cannot take an unevaluated quotation.**
  `hof [mul 2] xs` evaluates the bracket at the call site;
  `NoEvalArgs` is a native-word privilege. Use `codequote`.
- **A computed key for a *quoting* accessor must be parenthesised.**
  `cache has k` silently looks up the literal `"k"` and returns `false`
  — NUR040's class, and what made a first `memoize` attempt miss on
  every call while the cache visibly filled. `get` is not affected: it
  evaluates a bare bound key (`lang/spec/accessor.tsv`).
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
- **No `while`.** `for` is a numeric range with `break`; loops over a
  condition are recursion.

---

## 6. Recommendations, cheapest first

1. **Document the `(args).N` idiom** in `REFERENCE.md` and `HOWTO.md`. It
   is the only parameter read that is total over both function and
   non-function bindings, and it is currently folklore. One paragraph
   closes §5.2's discoverability gap.
2. **Add a diagnostic for §5.1.** A statement ending with an
   uncalled `Function` beside stranded operands, where the function came
   from a capitalised binding, is a near-certain typo. `undefined_word`
   already offers "did you mean"; this deserves the same.
3. **Give `each`/`for-each`/`fold`/`scan` a `{TFunction, TList}`
   signature** (NUR086). Note this buys *signature availability*, not
   callback uniformity: `filter`'s list `Function` form passes a
   `{key,value}` pair, so a matching `filter` change is needed before an
   element-shaped callback works across the whole family.
4. **Ship the missing vocabulary as a module** — `compose`, `pipe`,
   `curry`, `partial`, `const`, `identity`, `flip`, `on`, `memoize`. Every
   one was writable here in a handful of lines; `boru:fn-util` next to
   `boru:type-util` would remove most of the friction this audit found
   without touching the kernel.
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

The grade this audit would give: **substrate A−, surface C+**. Nothing is
missing from the machine. What is missing is a spelling for "the value,
whatever kind it is", a uniform callback convention, and the vocabulary
that would make the first two rarely needed.
