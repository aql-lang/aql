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

```
POSITIONAL_MATCH(values[], types[]):
    if len(values) < len(types):
        return false
    for i in 0..len(types)-1:
        if NOT values[i].type.MATCHES(types[i]):
            return false
        // Reject type literals (nil data) for concrete Map/List signatures
        if values[i].data == nil AND (types[i] == Map OR types[i] == List):
            return false
    return true
```

---

## 3. Signature Scoring

Signatures are ranked by a numeric score. Higher is better.

```
SIGNATURE_SCORE(sig):
    score = len(sig.args) * 100              // arity dominates
    for each type t in sig.args:
        score += t.SPECIFICITY()             // deeper types = more specific
    return score
```

**Example scores:**
- `(Integer)` → 1×100 + 3 = 103  (Integer = Scalar/Number/Integer, depth 3)
- `(String, String)` → 2×100 + 2+2 = 204
- `(Any)` → 1×100 + 1 = 101

---

## 4. Prefix (Stack) Matching — `MatchSignature`

This is the primary dispatch path. The engine looks at the top of the stack
and finds the best-matching signature.

```
MATCH_SIGNATURE(signatures[], stack[], modifiers):
    best       = nil
    bestScore  = 0

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
        if NOT POSITIONAL_MATCH(top, sig.args):
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

        // --- Compute score ---
        score = SIGNATURE_SCORE(sig)

        // Match quality bonus: reward specific (non-Any) type matches
        for j in 0..n-1:
            if sig.args[j] == Any:
                continue                     // no bonus for catch-all
            if top[j].type == sig.args[j]:
                score += 50                  // exact type match bonus
            else:
                score += 10                  // subtype (inexact) match bonus

        // --- Track best ---
        if best == nil OR score > bestScore:
            best      = sig
            bestScore = score
            bestArgs  = copy(top)

    return best, bestArgs                    // nil if no match
```

**Key properties:**
- Longer signatures (more args) always beat shorter ones (100 points per arg)
- Among equal-length signatures, more specific types win
- Exact type matches get +50 bonus, subtype matches get +10
- `Any` args contribute no match quality bonus (prevents inflation)

---

## 5. Reversed Stack Matching — `MatchSignatureReversed`

Used for forward-precedence words when all arguments end up on the stack but
in reversed order (top of stack = first argument).

```
MATCH_SIGNATURE_REVERSED(signatures[], stack[], modifiers):
    // Same as MATCH_SIGNATURE, but extract values in reverse:
    for each sig (same filtering as above):
        n = len(sig.args)
        reversed = []
        for j in 0..n-1:
            reversed[j] = stack[len(stack) - 1 - j]   // top → arg[0]

        if NOT POSITIONAL_MATCH(reversed, sig.args):
            continue

        // ... same pattern check and scoring as MATCH_SIGNATURE ...
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
            if COULD_PRODUCE_TYPE(peekVal, sig.args[firstUnmatched]):
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

## 7. Signature Sorting

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

## 8. End-to-End Example

Given a word `add` with signatures:
```
sig1: (Scalar, Scalar)         → score: 200 + 1+1 = 202
sig2: (Integer, Integer)       → score: 200 + 3+3 = 206
sig3: (String, String)         → score: 200 + 2+2 = 204
```

And stack: `[... 3, 5]` (both Integer = Scalar/Number/Integer)

1. **sig1 (Scalar, Scalar):** positional match succeeds (Integer matches Scalar).
   Score = 202 + 10 + 10 = 222 (subtype bonuses, not exact)
2. **sig2 (Integer, Integer):** positional match succeeds (exact match).
   Score = 206 + 50 + 50 = 306 (exact match bonuses)
3. **sig3 (String, String):** positional match fails (Integer doesn't match String).
   Skipped.

Result: **sig2 wins** with score 306. The correct integer addition handler is invoked.
