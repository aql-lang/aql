# Signature Matching Algorithm — Pseudocode

This document describes the AQL signature matching algorithm used to dispatch
function calls to the correct overload based on the types of values on the
stack.

## Overview

AQL is a concatenative (stack-based) language. When a word (function) is
invoked, the engine must choose which of its registered signatures best
matches the current stack state. The algorithm has two modes:

1. **Prefix (stack) matching** — arguments are already on the stack
2. **Forward matching** — some arguments come from future tokens not yet evaluated

---

## 1. Type System

Types are hierarchical slash-separated paths (e.g. `Scalar/Number/Integer`).

```
TYPE.MATCHES(pattern):
    if pattern == "Any":
        return true                          // Any matches everything
    if depth(self) < depth(pattern):
        return false                         // child can't be shallower than parent
    for i in 0..depth(pattern)-1:
        if self.parts[i] != pattern.parts[i]:
            return false                     // path prefix must match exactly
    return true                              // child matches parent

TYPE.SPECIFICITY():
    return depth(self)                        // number of path segments

TYPE.IS_SUBTYPE_OF(parent):
    return depth(self) > depth(parent) AND self.MATCHES(parent)

TYPE.EQUAL(other):
    return self.parts == other.parts          // exact identity
```

**Examples:**
- `Scalar/Number/Integer` matches pattern `Scalar/Number` (child matches parent)
- `Scalar/Number` does NOT match pattern `Scalar/Number/Integer` (parent does not match child)
- `Scalar/String` does NOT match pattern `Scalar/Number` (different branch)
- Anything matches pattern `Any`

---

## 2. Positional Match

Values are matched against signature types strictly in order — no permutation.
The `/q` modifier (implicit quote) on a signature position allows Word values
to match as Atoms without evaluation.

```
POSITIONAL_MATCH(values[], sig):
    if len(values) < len(sig.args):
        return false
    for i in 0..len(sig.args)-1:
        v = values[i]
        t = sig.args[i]

        // /q modifier: treat Word as Atom for matching
        if sig.quoteArgs[i] AND v.type == Word:
            if NOT Atom.MATCHES(t):
                return false
            continue

        if NOT v.type.MATCHES(t):
            return false
        // Reject type literals (nil data) for concrete Map/List signatures
        if v.data == nil AND (t == Map OR t == List):
            return false
    return true
```

---

## 3. Signature Scoring

Signatures are ranked by a numeric score. Higher is better. The score uses
three magnitude tiers: arity (1e6), specificity (1e4), and type inherent score.

```
SIGNATURE_SCORE(sig):
    score = len(sig.args) * 1_000_000       // arity dominates
    for each type t in sig.args:
        score += t.SPECIFICITY() * 10_000   // specificity tier
        score += TYPE_INHERENT_SCORE(t)     // unique per type (~100 increments)
    return score
```

Each type has a unique inherent score based roughly on value cardinality
(e.g. Boolean=1200, Integer=1100, String=2100, Any=200). This ensures
deterministic ordering even between types at the same depth.

**Example scores:**
- `(Integer)` → 1×1e6 + 3×1e4 + 1100 = 1_031_100
- `(String, String)` → 2×1e6 + (2×1e4+2100)×2 = 2_044_200
- `(Any)` → 1×1e6 + 1×1e4 + 200 = 1_010_200

---

## 4. Prefix (Stack) Matching — `MatchSignature`

This is the primary dispatch path. The engine looks at the top of the stack
and finds the first matching signature. Signatures are pre-sorted by
`SortSignatures` (longest and most specific first, fallbacks last), so the
first match is the best match.

```
MATCH_SIGNATURE(signatures[], stack[], modifiers):
    // signatures must be pre-sorted by SORT_SIGNATURES

    for each sig in signatures:

        // --- Filter by modifier constraints ---
        if modifiers.argCount >= 0 AND len(sig.args) != modifiers.argCount:
            continue                         // explicit arity constraint

        n = len(sig.args)
        if len(stack) < n:
            continue                         // not enough values on stack

        // --- Extract top N values ---
        top = stack[len(stack)-n .. len(stack)-1]

        // --- Positional type check ---
        if NOT POSITIONAL_MATCH(top, sig):
            continue

        // --- Structural pattern check (e.g. map literal patterns) ---
        if sig.patterns is not empty:
            for each (argIndex, pattern) in sig.patterns:
                if both are maps with data (not type literals):
                    if NOT OPEN_UNIFY_MAP(pattern, top[argIndex]):
                        continue outer       // map keys must be a subset match
                else:
                    if NOT UNIFY(top[argIndex], pattern):
                        continue outer

        // --- First match wins ---
        return sig, copy(top)

    return nil                               // no match
```

**Key properties:**
- Signatures are pre-sorted: longer first, then more specific types, fallbacks last
- First match wins — no scoring needed at match time
- Sorting ensures the most specific applicable signature is tried first

---

## 5. Reversed Stack Matching — `MatchSignatureReversed`

Used for forward-precedence words when all arguments end up on the stack but
in reversed order (top of stack = first argument).

```
MATCH_SIGNATURE_REVERSED(signatures[], stack[], modifiers):
    // signatures must be pre-sorted by SORT_SIGNATURES
    // Same as MATCH_SIGNATURE, but extract values in reverse:
    for each sig (same filtering as above):
        n = len(sig.args)
        reversed = []
        for j in 0..n-1:
            reversed[j] = stack[len(stack) - 1 - j]   // top → arg[0]

        if NOT POSITIONAL_MATCH(reversed, sig):
            continue

        // ... same pattern check as MATCH_SIGNATURE ...

        return sig, copy(reversed)           // first match wins

    return nil
```

---

## 6. Forward Matching — `plannerBestSigForForward`

When a word has forward precedence, it can collect arguments from tokens that
appear after it in the source. The planner decides which signature to use and
how many args come from the stack vs. from forward tokens.

```
PLANNER_BEST_SIG_FOR_FORWARD(function, modifiers, resolvedStack[]):
    best           = nil
    bestScore      = 0
    bestStackCount = 0

    peekVal = PEEK_NEXT_FORWARD_VALUE()      // look at next non-structural token

    for each sig in function.signatures:
        if len(sig.args) == 0:
            continue                         // need at least 1 arg
        if modifiers.argCount >= 0 AND len(sig.args) != modifiers.argCount:
            continue

        // How many stack values can fill the LAST N sig positions?
        stackCount, usedArgs = FORWARD_STACK_COVERAGE(sig.args, resolvedStack)

        score = SIGNATURE_SCORE(sig)
        score += stackCount * 25             // bonus for consuming stack values

        // Lookahead bonus: can the next forward token satisfy the first
        // unmatched arg position?
        if peekVal != nil AND stackCount < len(sig.args):
            firstUnmatched = first index i where usedArgs[i] == false
            canMatch = COULD_PRODUCE_TYPE(peekVal, sig.args[firstUnmatched])
            // /q modifier: Word peek value matches Atom-typed /q slot
            if NOT canMatch AND sig.quoteArgs[firstUnmatched]
               AND peekVal.type == Word AND Atom.MATCHES(sig.args[firstUnmatched]):
                canMatch = true
            if canMatch:
                if sig.args[firstUnmatched] == Any:
                    score += 25              // weaker bonus for catch-all
                else:
                    score += 50              // stronger bonus for specific type

        if best == nil OR score > bestScore:
            best           = sig
            bestScore      = score
            bestStackCount = stackCount

    return best, bestStackCount
```

### Forward Stack Coverage

Determines how many stack values can fill signature positions from the end,
since forward tokens fill from the beginning.

```
FORWARD_STACK_COVERAGE(sigArgs[], resolved[]):
    usedArgs = [false] * len(sigArgs)
    maxTry   = min(len(sigArgs), len(resolved))

    // Try largest coverage first, shrink until it fits
    for tryN = maxTry downto 1:
        sigStart = len(sigArgs) - tryN       // fill last tryN positions

        ok = true
        for j in 0..tryN-1:
            stackVal = resolved[len(resolved) - 1 - j]   // top of stack first
            if NOT stackVal.type.MATCHES(sigArgs[sigStart + j]):
                ok = false
                break

        if ok:
            mark usedArgs[sigStart..end] = true
            return tryN, usedArgs

    return 0, usedArgs
```

**Example:** `sigArgs = [Integer, String, Boolean]`, stack = `[true, "hello"]`
- Try 2: sigStart=1, stack top="hello" matches String, next=true matches Boolean → match!
- Returns stackCount=2: forward will collect 1 arg (Integer), stack provides 2

---

## 7. The /q Modifier (Implicit Quote)

The `/q` modifier on a signature argument position indicates that a Word value
at that position should be treated as an Atom for matching purposes and captured
without evaluation during forward collection.

### Signature Definition

```
Signature {
    args:      Type[]
    quoteArgs: map[int]bool   // positions with /q modifier
    handler:   function
    ...
}
```

### Where /q applies

The /q modifier affects matching at every point in the engine where type
compatibility is checked:

1. **POSITIONAL_MATCH** — Word at /q position matches if Atom.MATCHES(sigType)
2. **Forward collection** — Word value at /q position is accepted by the forward
3. **Forward word capture** — `hasPendingForwardExpectingWord` returns true for /q positions, preventing word evaluation during collection
4. **Planner peek bonus** — Word peek at /q position gets the specific-type bonus
5. **Early resolution** — `shouldResolveForwardEarly` recognizes /q when checking if a shorter sig matches collected types
6. **hasForwardValues** — /q positions are recognized as accepting words
7. **Overload switching** — /q positions match Word values during forward sig switching

### Example: `def` signatures

```
sig1: [String, Any]                          — def "name" body
sig2: [Atom/q, Any]   quoteArgs={0: true}   — def name body
```

In `def foo 42`:
- Planner selects sig2 (Atom/q matches word peek)
- `foo` arrives as Word; hasPendingForwardExpectingWord → true (quoteArgs[0])
- Word captured without evaluation
- positionalMatch: Word at /q slot → Atom.MATCHES(Atom) → true
- Handler receives Word(foo), extracts name "foo"

---

## 8. Signature Sorting

Signatures are pre-sorted so higher-priority ones are checked first.

```
SORT_SIGNATURES(sigs[]):
    // Stable insertion sort
    for i = 1 to len(sigs)-1:
        for j = i downto 1:
            // Fallback signatures always sink to the end
            if sigs[j-1].fallback AND NOT sigs[j].fallback:
                swap(sigs[j], sigs[j-1])
                continue
            if sigs[j].fallback:
                break
            if SIGNATURE_SCORE(sigs[j]) > SIGNATURE_SCORE(sigs[j-1]):
                swap(sigs[j], sigs[j-1])
            else:
                break
```

**Priority order:**
1. More arguments (arity) — always wins
2. More specific types (deeper hierarchy) — breaks ties
3. Registration order — preserved for equal scores (stable sort)
4. Fallback signatures — always last

---

## 9. End-to-End Example

Given a word `add` with signatures (registered in any order):
```
sig1: (Scalar, Scalar)         → score: 2e6 + (1e4+2500)*2 = 2_025_000
sig2: (Integer, Integer)       → score: 2e6 + (3e4+1100)*2 = 2_062_200
sig3: (String, String)         → score: 2e6 + (2e4+2100)*2 = 2_044_200
```

After `SortSignatures`, the order becomes (highest score first):
```
sig2: (Integer, Integer)       → score 2_062_200
sig3: (String, String)         → score 2_044_200
sig1: (Scalar, Scalar)         → score 2_025_000
```

And stack: `[... 3, 5]` (both Integer = Scalar/Number/Integer)

1. **sig2 (Integer, Integer):** positional match succeeds (exact match). **First match — return immediately.**

Result: **sig2 wins**. The correct integer addition handler is invoked.
The less specific signatures are never even tried.

---

## 10. Sequential Forward Planner (Experimental)

An alternative to the scoring-based `plannerBestSigForForward` (Section 6).
Instead of computing scores for all signatures and picking the highest, this
planner walks through pre-sorted signatures sequentially and tries to match
each one against the actual forward token stream. The first fully-matched
signature wins. Controlled by `Registry.SequentialPlanner` feature flag.

```
PLANNER_SEQUENTIAL_FORWARD(function, modifiers, resolvedStack[]):
    // signatures are pre-sorted by SIGNATURE_SCORE (highest first)
    for each sig in function.signatures:
        if len(sig.args) == 0:
            continue
        if modifiers.argCount >= 0 AND len(sig.args) != modifiers.argCount:
            continue
        if sig.fallback:
            continue

        forwardMatched, stackCount = TRY_SEQUENTIAL_MATCH(sig, resolvedStack)
        if forwardMatched + stackCount == len(sig.args) AND forwardMatched > 0:
            return sig, stackCount

    return nil, 0
```

### Sequential Token Matching

Walks the forward token stream starting after the current pointer. For each
token, decides whether it satisfies the next expected signature argument.

```
TRY_SEQUENTIAL_MATCH(sig, resolvedStack[]):
    nArgs          = len(sig.args)
    forwardMatched = 0
    scanIdx        = pointer + 1

    while forwardMatched < nArgs AND scanIdx < len(stack):
        tok          = stack[scanIdx]
        expectedType = sig.args[forwardMatched]

        // --- Structural markers: stop scanning ---
        if tok is forward marker:
            break

        if tok is word:
            name = tok.name

            // End / close-paren: stop
            if name == "end" OR name == ")":
                break

            // Open paren: sub-expression will produce a value
            if name == "(":
                forwardMatched++; scanIdx++; continue

            // /q position: word treated as Atom
            if sig.quoteArgs[forwardMatched]:
                if Atom.MATCHES(expectedType):
                    forwardMatched++; scanIdx++; continue
                else:
                    break

            // Known function with forward precedence: will produce a value
            if fn = LOOKUP(name); fn != nil:
                if fn.forwardPrecedence:
                    forwardMatched++; scanIdx++; continue
                else:
                    break   // stack-only function, stop

            // Defined word: resolve to its def type
            if def = DEF_STACK(name); def exists:
                if def.type.MATCHES(expectedType) OR expectedType == Any:
                    forwardMatched++; scanIdx++; continue
                else:
                    break

            // Unknown word: becomes Atom (or Boolean for true/false)
            resolvedType = Atom
            if name == "true" OR name == "false":
                resolvedType = Boolean
            if name is a type name:
                break   // type literals are not collectible values
            if resolvedType.MATCHES(expectedType) OR expectedType == Any:
                forwardMatched++; scanIdx++; continue
            break

        // Literal value: direct type check
        if tok.type.MATCHES(expectedType) OR expectedType == Any:
            if tok.data == nil AND (expectedType == Map OR expectedType == List):
                break   // reject type literals
            forwardMatched++; scanIdx++; continue

        break   // type mismatch

    // All matched from forward?
    if forwardMatched == nArgs:
        return forwardMatched, 0

    // Try filling remaining args from the stack (suffix)
    remaining  = nArgs - forwardMatched
    stackCount = SEQUENTIAL_STACK_MATCH(sig.args, forwardMatched, resolvedStack)
    if stackCount == remaining:
        return forwardMatched, stackCount

    return forwardMatched, 0   // partial — not enough args
```

### Sequential Stack Match

Fills remaining signature positions from the resolved stack.

```
SEQUENTIAL_STACK_MATCH(sigArgs[], startIdx, resolved[]):
    remaining = len(sigArgs) - startIdx
    if remaining <= 0 OR len(resolved) < remaining:
        return 0
    for j in 0..remaining-1:
        stackVal = resolved[len(resolved) - 1 - j]
        if NOT stackVal.type.MATCHES(sigArgs[startIdx + j]):
            return 0
    return remaining
```

### Known Limitations

The sequential planner has a known ordering issue: `LOOKUP(name)` runs before
`DEF_STACK(name)`. User-defined words (created by `def`) are registered as
forward-precedence functions, so `LOOKUP` finds them and optimistically accepts
them at any type position — bypassing the actual type check that `DEF_STACK`
would perform. This causes incorrect signature matching when a def'd word's
type doesn't match the expected signature position (e.g. `undef a` where `a`
is defined as Integer, but the sig expects `TFnUndef` at position 1).

### Test Results (Feature-Flagged)

**Targeted tests** (planner_sequential_test.go): 11/13 pass

| Test | Result | Notes |
|------|--------|-------|
| BasicInfix (2 add 3) | PASS | |
| DefForward (def foo 42 foo) | PASS | |
| DefGreeting (def greeting "hello") | PASS | |
| UndefSimple (def foo 1 undef foo foo) | **FAIL** | "no matching signature for undef" — Lookup/DefStack ordering bug |
| SetGet (set k 99 get k) | PASS | |
| ContextSetGet | PASS | |
| AddPrefix/Forward/Infix | PASS | All 3 positions work |
| DefFnSquare (fn [...] 5 sq) | **FAIL** | "no matching signature for sq" — fn-defined functions not handled |
| Quote | PASS | |
| Dup | PASS | |
| TypeDef | PASS | |
| PlannerUnit | PASS | |
| PlannerStackMatch | PASS | |

**Integration tests** (planner_sequential_full_test.go): 19/22 pass

| Test | Result | Notes |
|------|--------|-------|
| add_forward, add_infix, add_prefix | PASS | |
| mul_forward, sub_infix | PASS | |
| def_word_int, def_string_body, def_list_body | PASS | |
| undef_basic | **FAIL** | Lookup/DefStack ordering bug |
| undef_stacked | **FAIL** | Same root cause |
| set_get_word, set_get_string | PASS | |
| quote_word, quote_int | PASS | |
| dup, swap, drop | PASS | |
| lt_true, gt_false, eq_true | PASS | |
| concat_forward | **FAIL** | concat expects TList; 3 string args not auto-collected |
| nested_forward | PASS | |

**Root causes of failures:**

1. **Lookup before DefStack** (undef, DefFnSquare): Def'd words are registered
   as forward-precedence functions via `installDef`. `LOOKUP(name)` finds them
   and optimistically accepts at any type position, bypassing the DefStack type
   check. Fixing: check DefStack before Lookup, or add type awareness to the
   Lookup path.

2. **concat expects TList** (concat_forward): concat's signature is
   `[TList]` — it expects a single list argument. The test passes 3 separate
   words. The scoring-based planner wouldn't match this either; the test case
   itself is likely incorrect for any forward-matching planner.
