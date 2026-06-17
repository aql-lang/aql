<!--
Provenance: downloaded from voxgig-aql/bloom-filter @ 4b208cb
  proposals/lazy-arg-resolution.md (2026-06-06), surfaced as gotcha N1 in
  that project's dx-report.md (see design/VOXGIG-AQL-REPORTS.5.md, Part 1,
  "N1. import now requires a terminator").
This is the verbatim downstream proposal. An assessment grounded in the
current engine (eng/go/engine.go) was provided separately to the team.
-->

# AQL Proposal: Structure-First, Lazy Argument Resolution for Dispatch

**Status:** Draft / RFC
**Target:** `aql-lang/aql` interpreter (dispatch / overload resolution)
**Provenance:** surfaced while upgrading the `bloom-filter` module to
`aql @ db828ec`; recorded as gotcha **N1** in that project's
`dx-report.md`.
**Build referenced:** `aql @ db828ec` (`db828ecb6ee1d161ff177134478f42c56484f051`).
Every "current behaviour" transcript below was run against that build;
"proposed behaviour" blocks are design intent and are labelled as such.

---

## 1. Summary

AQL resolves an overloaded word by **evaluating following tokens to
discover the argument list, then matching that list against the word's
signatures**. Because the arguments are computed *before* a signature is
chosen, resolution can execute code that the chosen signature never
needed — most visibly, an unterminated `import "mod"` evaluates the
*next* expression while deciding whether a longer overload applies, and
that expression often refers to names `import` is about to install.

This proposal replaces the *eager-probe* model with a **structure-first,
lazy** one:

1. **Resolve the signature from structure** — choose the overload using
   only what is knowable *without evaluation*: token shape (string / list
   / atom / number literal, parenthesised group, barrier) and
   literal types.
2. **Evaluate claimed arguments lazily** — once a signature is committed,
   evaluate exactly the forward groups it consumes, left to right, once
   each.

This removes speculative evaluation entirely. It fixes the `import`
terminator hazard, eliminates a class of premature/duplicated
side-effects, and — unlike the "swallow errors during probing"
alternative (§7) — keeps every genuine error loud and precisely located.

---

## 2. Background: forward dispatch and overloaded words

AQL is concatenative. A word takes its arguments from the tokens that
follow it (forward) and/or from the stack. These three forms are
equivalent (verified):

```aql
(add 2 3) print end     # => 5
(2 add 3) print end     # => 5
(2 3 add) print end     # => 5
```

A native word may have several **signatures** (overloads). Each
signature lists parameter types in *sig order* — `sig[0]` binds to the
top of the stack / the first forward argument, `sig[1]` to the next, and
so on. The dispatcher chooses the signature whose arity and types match
the supplied arguments.

`import` is a richly overloaded word. Its signatures, in the order the
dispatcher tries them (from `describe import`):

```
[ [List Atom List] ]      # inline module + rename
[ [Atom Atom List] ]      # inline module + single rename
[ [List Module]    ]      # rename selected exports from an instance
[ [List String]    ]      # rename selected exports from a path
[ [Atom Module]    ]      # single rename from an instance
[ [Atom List]      ]      # inline module
[ [Module]         ]      # import an already-resolved instance
[ [String]         ]      # import a file / native module  ← the common one
```

Note the everyday form, `[String]`, is matched **last**, and no overload
has a `String` in `sig[0]` *except* `[String]`.

---

## 3. The problem: speculative argument evaluation

### 3.1 Reproduction (current behaviour, `aql @ db828ec`)

```aql
# bug.aql
import "aql:string-util"
(StringUtil.indexof " ABC" "B") print end
```

```
$ aql bug.aql
error: [aql/undefined_word]: undefined word: StringUtil
  --> 2:2
  1 | import "aql:string-util"
  2 | (StringUtil.indexof " ABC" "B") print end
       ^^^^^^^^^^ undefined word: StringUtil
```

The error is on **line 2**, inside the parenthesised expression — and it
fires *before* line 1's `import` has installed the `StringUtil`
namespace. That ordering is the whole bug: `import`'s resolution reached
forward into line 2 and evaluated `(StringUtil.indexof …)` to decide
which overload applied, dereferencing a name that `import` itself was
about to bind.

### 3.2 It is not a "wrong overload" — `[String]` is correct

Add a terminator and it works; the `[String]` overload was right all
along:

```aql
import "aql:string-util" end
(StringUtil.indexof " ABC" "B") print end     # => 2
```

And a *harmless* follower shows `import` only ever wanted one argument —
the trailing value is left untouched, not consumed as a second arg:

```aql
import "aql:string-util" 42
"after" print end
# => after
# => 42        (42 was never an argument to import; it stays on the stack)
```

So the failure in §3.1 is not a mis-selected signature. `[String]` is the
intended match; the statement simply dies during the *probe* that
precedes selection.

### 3.3 Why a bare value is fine but a parenthesised one explodes

A bare literal following `import` is left alone (§3.2). A parenthesised
group, however, is a value-producing expression the probe evaluates to
type-check it as a candidate argument — and evaluating
`(StringUtil.indexof …)` is what raises. The current escape hatch is the
`end` barrier, which tells the dispatcher "no further arguments," so the
probe never reaches line 2.

---

## 4. Root cause

The dispatcher uses an **eager-probe** model:

```
1. collect-and-EVALUATE forward tokens to discover the argument list
2. match the evaluated values against the signatures (try arity high → low)
3. run the selected handler
```

Step 1 evaluates arguments **before** step 2 commits to a signature. So:

- arguments for overloads that are ultimately rejected still run;
- for `import`, a rejected higher-arity probe evaluates the following
  expression *before* the word's own effect (installing the namespace)
  has happened.

The defect is **ordering and necessity**: code runs that the chosen
signature did not require, and it runs at the wrong time.

---

## 5. The proposal

Split resolution so that *no evaluation happens while choosing a
signature*.

### Phase 1 — structural arity/signature resolution (no evaluation)

Select the overload using only information available without running any
code:

- **Token shape:** is the next forward token a string literal `"…"`, a
  number literal, a list literal `[…]`, a quoted atom, a parenthesised
  group `(…)`, or a barrier (`end`, another word)?
- **Literal types:** the type you can read directly off a literal
  (`"x"` is `String`, `[…]` is `List`, `42` is `Integer`, …).
- **Barriers:** `end` / parens cap the available argument count.

From the candidate signatures plus this structural view, commit to one
signature (or at least a definite arity). A parenthesised group whose
*value* is unknown without evaluation contributes only "there is one
argument-shaped thing here," never its value.

### Phase 2 — lazy argument evaluation (after commitment)

With a signature committed, evaluate exactly the forward groups it
claims, left to right, **once each**. Anything not claimed is left as the
next expression. Errors in a *claimed* argument propagate normally, with
their precise source span.

### Precedent: this generalises machinery AQL already has

`import`'s rename overloads are registered with `NoEvalArgs` / `QuoteArgs`:

```go
{   // import [Orig Renamed] "path"
    Args:       []*Type{TList, TString},
    NoEvalArgs: map[int]bool{0: true},   // the [names] list is taken RAW
    …
},
{   // inline module form
    Args:      []*Type{TAtom, TList},
    QuoteArgs: map[int]bool{0: true},    // the module atom is captured, not run
    NoEvalArgs:map[int]bool{1: true},
},
```

These already say "consume this argument **by structure**, do not
evaluate it." The engine can demonstrably take an argument as a raw
token. This proposal extends that same stance from *handling a claimed
argument* to *choosing the signature* in the first place.

---

## 6. Worked example: `import` under the proposal

Input (the failing program from §3.1, **no terminator**):

```aql
import "aql:string-util"
(StringUtil.indexof " ABC" "B") print end
```

**Phase 1 (structure):** the first forward token is the string literal
`"aql:string-util"`. Its type, `String`, is known from the literal. Among
`import`'s overloads, the only one whose `sig[0]` is `String` is
`[String]` — and it has arity 1. *Commit to `[String]`.* The
parenthesised group on line 2 is never inspected, because `[String]`
needs no second argument.

**Phase 2 (lazy eval):** `[String]` claims one argument, the string
literal (already a value — nothing to run). `import` executes and
installs the `StringUtil` namespace.

**Line 2** then evaluates as an ordinary statement, now with `StringUtil`
bound:

```
# proposed behaviour
=> 2
```

No terminator required, no speculative evaluation, nothing swallowed.

---

## 7. Why this beats the "swallow errors while probing" alternative

A tempting one-line fix is: *during probing, if evaluating a candidate
argument errors, discard that signature and move on.* It also makes
§3.1 pass — but at a steep, global price, because it keeps the
speculative evaluation and merely hides its failures.

| Concern | Swallow-errors rule | This proposal |
|---|---|---|
| Fixes `import` §3.1 | yes | yes |
| Real errors in arguments | **silently hidden** → vague `no matching signature` or a different overload runs | propagate with exact span |
| "arg has a bug" vs "signature doesn't apply" | conflated | never conflated (no eval during selection) |
| Side-effecting argument | evaluated, discarded, then **re-evaluated** → double / partial effects | evaluated **once**, only if claimed |
| Overload chosen depends on… | whether an arg happens to throw at runtime | structure + types (deterministic) |
| Static analysis / `RunInCheckMode` | needs to run args to know viability | decidable without running args |
| Cost shape | try-eval across candidates | evaluate only claimed args |

The swallow rule treats the *symptom* (an error escapes the probe) by
suppressing errors language-wide. This proposal removes the *cause* (the
probe) and so needs no suppression.

---

## 8. Scope and limits

This is clean when overloads are **distinguishable by shape + literal
type** — which `import`'s are: they fork on `String` vs `[List]` vs
`Atom` vs `Module`, all readable without evaluation.

It does **not**, by itself, resolve overloads that genuinely differ by an
argument's *runtime* value or runtime type hidden behind a `(…)` group;
there you must evaluate that argument to disambiguate.

Even then, **lazy left-to-right** evaluation is strictly better than
eager-probe:

- you evaluate the first genuinely-ambiguous argument once, and match
  progressively as values arrive;
- you **never** evaluate *trailing* groups merely to test a longer
  overload — which is exactly the `import` pathology.

So the worst remaining case (value-dependent dispatch) evaluates only the
single argument it disambiguates on, once; the "evaluate a following
expression that isn't even mine, before I've run" behaviour is gone in
every case.

---

## 9. Compatibility and migration

- **Programs that already terminate calls** (`import "x" end`, parens):
  unaffected — `end` still caps arity; Phase 1 simply also succeeds
  without it where structure is unambiguous.
- **Programs that relied on the missing terminator failing:** none should
  exist; the failure is the bug being removed.
- **Newly-valid programs:** `import "x"` directly followed by an
  expression starts working. This is a strict superset of today's
  accepted programs for shape-unambiguous words; nothing previously
  accepted changes meaning.
- **The `bloom-filter` module** keeps `import "x" end` regardless — it is
  correct under both models and remains the recommended, portable style.

---

## 10. Implementation sketch

1. **Argument descriptor without evaluation.** Give the parser/dispatcher
   a way to read, for each forward token, a *structural descriptor*
   (`StringLit`, `NumberLit`, `ListLit`, `QuotedAtom`, `Group`, `Barrier`,
   `BoundName→type if statically known`) without executing it.
2. **Match on descriptors.** Replace "evaluate then match" with "match
   descriptors against signature `Args` (and `NoEvalArgs`/`QuoteArgs`)
   to commit to one signature/arity." Prefer the existing high→low arity
   order, but reject a signature as soon as a *structural* mismatch is
   known.
3. **Lazy claim.** After commitment, pull and evaluate exactly the
   claimed forward groups in order; feed raw tokens for `NoEval`/`Quote`
   positions as today.
4. **Fallback for value-dependent overloads.** When descriptors cannot
   disambiguate (two viable signatures differ only by a `Group`'s runtime
   type), evaluate that single argument lazily and re-test — still never
   touching later groups.
5. **Barriers unchanged.** `end` / parens continue to cap arity; they
   become an optimisation/encouraged-style rather than a correctness
   crutch.

---

## 11. Acceptance criteria

Behavioural tests that should pass under the proposal:

```aql
# 1. unterminated import followed by a reference to the imported namespace
import "aql:string-util"
(StringUtil.indexof " ABC" "B") print end          # => 2   (today: error)

# 2. terminated form keeps working
import "aql:string-util" end
(StringUtil.upper "hi") print end                  # => HI

# 3. no signature's arguments are evaluated speculatively:
#    a side-effecting follower must NOT run during import resolution
import "aql:string-util"
("only once" print) end                            # prints "only once" exactly once

# 4. genuine errors stay loud and located
import "aql:string-util" end
(NoSuchThing.nope 1) print end                     # => undefined word: NoSuchThing at the call site

# 5. existing arity behaviour for shape-unambiguous overloads is unchanged
import "aql:string-util" 42
"after" print end                                  # => after ; 42 left on the stack
```

Plus: `aql check` can determine `import`'s selected signature for cases
1, 2, 5 **without** evaluating any argument body.

---

## Appendix A — verified current transcripts (`aql @ db828ec`)

```text
$ printf 'import "aql:string-util"\n(StringUtil.indexof " ABC" "B") print end\n' | aql /dev/stdin
error: [aql/undefined_word]: undefined word: StringUtil  --> 2:2

$ printf 'import "aql:string-util" end\n(StringUtil.indexof " ABC" "B") print end\n' | aql /dev/stdin
2

$ printf 'import "aql:string-util" 42\n"after" print end\n' | aql /dev/stdin
after
42

$ printf '(add 2 3) print end\n(2 add 3) print end\n(2 3 add) print end\n' | aql /dev/stdin
5
5
5
```

## Appendix B — `describe import` (signatures in match order)

```
Signatures: (in match order)
  [ [List Atom List]   ]
  [ [Atom Atom List]   ]
  [ [List Module]      ]
  [ [List String]      ]
  [ [Atom Module]      ]
  [ [Atom List]        ]
  [ [Module]           ]
  [ [String]           ]
```
