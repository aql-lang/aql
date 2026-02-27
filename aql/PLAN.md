# AQL Core Execution Loop - Implementation Plan

## Analysis

AQL is a **concatenative stack machine**, not a traditional expression-based
language. The current scaffolding follows a lexer → parser → AST → evaluator
pipeline modeled on languages like Monkey/Lox. This is the wrong architecture
for AQL. In a concatenative language:

- There is no AST tree — the program is a flat sequence of tokens.
- "Parsing" is trivial — just tokenization.
- The real work is the **stack machine engine** that processes tokens one at a
  time, matching typed function signatures against the stack.

The existing packages will be substantially reworked:

| Package     | Current                          | Becomes                                  |
|-------------|----------------------------------|------------------------------------------|
| `token/`    | C-like tokens (let, fn, if, ...) | AQL tokens (WORD, INTEGER, STRING, ...)  |
| `lexer/`    | Stub returning EOF               | AQL tokenizer (whitespace-delimited)     |
| `ast/`      | Tree-structured AST nodes        | **Removed** — not needed for concatenative |
| `parser/`   | Recursive-descent stub           | **Removed** — lexer feeds engine directly |
| `evaluator/`| AST walker stub                  | **Replaced** by `engine/` stack machine  |
| `object/`   | Simple value wrappers            | Typed stack values with hierarchical types |
| (new)       | —                                | `engine/` — the core execution loop      |

---

## Steps

### Step 1: Redefine Token Types (`internal/token/`)

Replace the current C-like token set with tokens appropriate for AQL.

**Tokens needed:**

| Token     | Examples               | Notes                                   |
|-----------|------------------------|-----------------------------------------|
| `WORD`    | `upper`, `lower`, `a`  | Unquoted identifier or function name    |
| `INTEGER` | `1`, `99`              | Numeric literal                         |
| `STRING`  | `"hello"`, `'hello'`   | Quoted string literal                   |
| `SLASH`   | `/`                    | Arg count modifier (e.g. `lower/1`)     |
| `EQUALS`  | `=`                    | Prefix/suffix forcing modifier          |
| `COLON`   | `:`                    | Jsonic key:value separator              |
| `COMMA`   | `,`                    | Jsonic list separator                   |
| `LBRACKET`| `[`                    | Jsonic array open                       |
| `RBRACKET`| `]`                    | Jsonic array close                      |
| `LBRACE`  | `{`                    | Jsonic map open                         |
| `RBRACE`  | `}`                    | Jsonic map close                        |
| `GT`      | `>`                    | For set predicates (e.g. `>2`)          |
| `EOF`     |                        | End of input                            |
| `ILLEGAL` |                        | Unrecognized character                  |

Remove all current keywords (`LET`, `FUNCTION`, `IF`, `ELSE`, `RETURN`, etc.)
and operators (`==`, `!=`, `+`, `-`, `*`, `!`) that belong to expression-based
languages.

Update `LookupIdent` to recognize AQL built-in words (`upper`, `lower`, `dup`,
`swap`, `drop`, `set`, `get`, `list`, `uniq`), returning `WORD` for all of
them — the function registry handles dispatch, not the token system.

**Files:** `internal/token/token.go`, `internal/token/token_test.go`

---

### Step 2: Implement the Lexer (`internal/lexer/`)

The AQL lexer tokenizes whitespace-delimited input:

- **Whitespace** separates tokens (spaces, tabs, newlines).
- **Quoted strings:** `"hello"` and `'hello'` — read until matching close quote.
- **Integers:** sequences of digits.
- **Words:** sequences of non-whitespace, non-delimiter characters. A word may
  contain embedded `/` and `=` for modifiers (e.g. `lower/1`, `lower=`,
  `=lower`). The lexer emits these as a single `WORD` token; modifier parsing
  happens in the engine.
- **Single-char delimiters:** `:`, `,`, `[`, `]`, `{`, `}`, `>` emitted
  individually.

The lexer should handle the full modifier syntax within words:
- `lower/1` → WORD "lower/1"
- `lower=` → WORD "lower="
- `=lower` → WORD "=lower"
- `lower/1=` → WORD "lower/1="
- `F =lower/1` → WORD "F", then WORD "=lower/1"

**Files:** `internal/lexer/lexer.go`, `internal/lexer/lexer_test.go`

**Tests:** tokenize the ENGINE.md examples:
- `a upper` → [WORD "a", WORD "upper", EOF]
- `lower B` → [WORD "lower", WORD "B", EOF]
- `99 lower` → [INTEGER "99", WORD "lower", EOF]
- `lower/1 D` → [WORD "lower/1", WORD "D", EOF]

---

### Step 3: Define the Type System (`internal/types/` — new package)

AQL types are **hierarchical paths**: `string`, `string/proper`, `string/empty`,
`number/integer`, `data/map`.

```go
type Type struct {
    Parts []string // e.g. ["string", "proper"]
}
```

**Key operations:**
- `Matches(pattern Type) bool` — does this type satisfy a signature type?
  A value of type `string/proper` matches pattern `string` (child matches
  parent). A value of type `string` does NOT match pattern `string/proper`.
- `Specificity() int` — number of parts. More parts = more specific.
- `String() string` — `"string/proper"`.

**Special types:**
- `any` — matches everything (used in `dup`, `swap`, `drop`).
- `forward` — the forward primitive marker type.

**Files:** `internal/types/types.go`, `internal/types/types_test.go`

---

### Step 4: Rework the Object System (`internal/object/`)

Stack values need to carry hierarchical type information. Rework the object
system:

```go
type Value struct {
    VType  types.Type
    Data   interface{} // Go value: int64, string, map, etc.
}
```

**Value constructors:**
- `NewInteger(n int64) Value` — type `number/integer`
- `NewString(s string) Value` — type `string/proper` (or `string/empty` if `""`)
- `NewMap(m map[string]interface{}) Value` — type `data/map`
- `NewForward(fn string, args int, arg int) Value` — type `forward`, data
  holds forward accounting struct

**Forward accounting struct:**
```go
type ForwardInfo struct {
    FuncName   string // the deferred function name
    ExpectedArgs int  // how many suffix args needed
    CollectedArg int  // how many have been collected so far
}
```

The `Forward` value gets placed on the stack when a suffix signature wins.
It tracks how many arguments still need to be resolved before the deferred
function can execute.

**Files:** `internal/object/object.go`, `internal/object/object_test.go`

---

### Step 5: Function Signatures and Registry (`internal/engine/registry.go` — new)

A function in AQL has a name and one or more **signatures**. Each signature
specifies the types it expects and whether they come from prefix (stack) or
suffix (future) positions.

```go
type Signature struct {
    Prefix  []types.Type // types expected on the stack (rightmost = top)
    Suffix  []types.Type // types expected from future tokens
    Handler func(args []Value) ([]Value, error)
}

type Function struct {
    Name       string
    Signatures []Signature
}

type Registry struct {
    funcs map[string]*Function
}
```

**Signature matching algorithm:**
1. For a given function name and current stack state, iterate all signatures.
2. For each prefix-only signature: check if the top N stack values match the
   required types (rightmost = top of stack).
3. For each suffix signature: these are candidate forward matches.
4. Among all matching signatures, pick the **most specific**: longest total
   arg count, then narrowest types (highest total specificity).
5. Ties are broken by preferring prefix over suffix.

**Built-in function registrations** (initial set):
- `upper`: prefix `[string] → [string]`
- `lower`: prefix `[string] → [string]`, suffix `[|string] → [string]`
- `dup`: prefix `[any] → [any, any]`
- `swap`: prefix `[any, any] → [any, any]`
- `drop`: prefix `[any] → []`

**Files:** `internal/engine/registry.go`, `internal/engine/registry_test.go`

---

### Step 6: Implement the Core Execution Loop (`internal/engine/engine.go` — new)

This is the heart of AQL. The engine maintains a **unified stack** where past
(resolved) values and future (unresolved) tokens coexist, separated by a
stack pointer.

```
[resolved values ... | ^pointer ... unresolved tokens]
```

**Data structures:**
```go
type Engine struct {
    stack    []StackEntry  // unified stack
    pointer  int           // index separating past from future
    registry *Registry
}

type StackEntry struct {
    Kind     EntryKind     // Resolved or Unresolved
    Value    *Value        // set when resolved
    Token    *token.Token  // set when unresolved
}
```

**Core loop (pseudocode):**
```
func (e *Engine) Run(tokens []token.Token) (Value, error):
    // Initialize: all tokens on the stack as unresolved
    for each token in tokens:
        stack.push(StackEntry{Kind: Unresolved, Token: &token})
    pointer = 0

    loop:
        if pointer >= len(stack):
            break  // no more future tokens

        entry = stack[pointer]

        if entry is unresolved:
            resolve entry:
                if token is INTEGER literal:
                    replace with resolved Value(number/integer)
                    pointer++
                    continue
                if token is STRING literal:
                    replace with resolved Value(string/proper or string/empty)
                    pointer++
                    continue
                if token is WORD:
                    // First: check if this is a plain value or a function
                    parse word for modifiers (name, argCount, forcePrefix, forceSuffix)
                    lookup function in registry

                    if not a registered function:
                        // treat as a bare identifier / string value
                        replace with resolved Value(string/proper)
                        pointer++
                        continue

                    // Try signature matching
                    match = registry.Match(name, stack[:pointer], stack[pointer+1:],
                                           forcePrefix, forceSuffix, argCount)

                    if match is prefix:
                        // Extract prefix args from stack, execute handler
                        args = pop match.Prefix count from stack before pointer
                        results = match.Handler(args)
                        // Replace function entry with results
                        splice results into stack at pointer position
                        adjust pointer
                        continue

                    if match is suffix:
                        // Insert forward primitive
                        insert ForwardInfo{name, len(match.Suffix), 0} before pointer
                        pointer++ // skip past the forward
                        continue

                    if no match:
                        return SignatureError

        if entry is resolved AND entry.Value is Forward:
            // The implicit forward-check: a resolved value appeared after
            // a forward primitive. Rearrange the stack.
            forward = find the nearest Forward entry before pointer
            move this resolved value to before the forward's function
            forward.CollectedArg++
            if forward.CollectedArg == forward.ExpectedArgs:
                // All suffix args collected; now retry the function
                // with a prefix match
                remove the forward entry
                set pointer back to the function position
            continue

        // Regular resolved value — just advance
        pointer++

    // Output: top of resolved stack
    return stack[len(stack)-1].Value
```

**Files:** `internal/engine/engine.go`, `internal/engine/engine_test.go`

---

### Step 7: Implement the Forward Mechanism (within engine)

The forward mechanism is the trickiest part. It's integrated into Step 6 but
deserves explicit attention.

**The forward protocol:**
1. When `lower B` is processed, `lower` has no prefix match (stack is empty).
   The suffix signature `[|string] → [string]` matches.
2. The engine inserts: `[lower, forward{args:1, arg:0} | ^'B']`
3. When `B` is resolved to a string value, the engine detects a forward entry
   behind the pointer.
4. The forward mechanism moves `'B'` to before `lower`:
   `['B', lower | ^]`
5. The pointer resets to `lower`. Now `lower` has a prefix match: `[string]`.
6. `lower` executes normally, producing `['b' | ^]`.

**The implicit forward signature:**
All functions must have an implicit, very-low-precedence signature that checks
for a Forward value on the stack. This is handled by the engine loop itself
(Step 6), not by individual function registrations.

**Files:** Part of `internal/engine/engine.go`

---

### Step 8: Remove Unused Packages and Wire Up

- **Remove** `internal/ast/` — concatenative language has no tree structure.
- **Remove** `internal/parser/` — lexer feeds the engine directly.
- **Remove** `internal/evaluator/` — replaced by `internal/engine/`.
- **Update** `cmd/aql/main.go`:
  - `run()` becomes: lexer.New(source) → lexer.Tokenize() → engine.New(registry) → engine.Run(tokens)
  - `-ast` flag removed (or repurposed to show stack trace).
  - `-tokens` flag still works (lexer output).
- **Update** `internal/repl/repl.go`:
  - REPL loop: read line → tokenize → engine.Run → print result.
  - The engine instance persists across REPL lines (maintaining store state).

**Files:** `cmd/aql/main.go`, `internal/repl/repl.go`, delete `internal/ast/`,
`internal/parser/`, `internal/evaluator/`

---

### Step 9: Implement Initial Built-in Primitives

Register these in the default registry to validate the architecture:

| Function | Signatures                          | Behavior                        |
|----------|-------------------------------------|---------------------------------|
| `upper`  | `[string] → [string]`               | Uppercase the string            |
| `lower`  | `[string] → [string]`               | Lowercase the string            |
|          | `[\|string] → [string]`             | (suffix variant)                |
| `dup`    | `[any] → [any, any]`                | Duplicate top of stack          |
| `swap`   | `[any, any] → [any, any]`           | Swap top two values             |
| `drop`   | `[any] → []`                        | Remove top of stack             |
| `set`    | `[\|word, any] → []`                | Store key=value                 |
| `get`    | `[\|word] → [any]`                  | Retrieve stored value           |

**Files:** `internal/engine/builtins.go`

---

### Step 10: End-to-End Tests

Validate the complete ENGINE.md examples:

```go
// Prefix: simple case
{"a upper", "A"}

// Suffix: forward mechanism
{"lower B", "b"}

// Prefix: already on stack
{"C lower", "c"}

// Signature error
{"99 lower", ERROR}

// Arg count disambiguation
{"lower/1 D", "d"}

// Force suffix
{"lower= E", "e"}

// Force prefix
{"F =lower", "f"}

// Literals self-insert
{"42", "42"}
{"hello", "hello"}
```

**Files:** `internal/engine/engine_test.go` (integration tests)

---

## Execution Order

```
Step 1: Token types          ─┐
Step 2: Lexer                 ├─ Foundation (no dependencies between 1-3)
Step 3: Type system          ─┘
Step 4: Object system        ← depends on Step 3
Step 5: Function registry    ← depends on Steps 3, 4
Step 6: Core execution loop  ← depends on Steps 1, 2, 4, 5
Step 7: Forward mechanism    ← part of Step 6, can be incremental
Step 8: Wire up + cleanup    ← depends on Steps 1-7
Step 9: Built-in primitives  ← depends on Steps 5, 6
Step 10: End-to-end tests    ← depends on everything
```

Steps 1, 2, and 3 can be implemented in parallel. Steps 4 and 5 follow.
Step 6 is the critical path. Steps 8-10 finalize.

---

## Risks and Open Questions

1. **Modifier parsing complexity**: Should `lower/1=` be a single WORD token
   or should the lexer split it? Plan says single token, engine parses
   modifiers. This keeps the lexer simple.

2. **Jsonic literals**: The SAMPLES.md shows `a:1,b:c,d:[{e:"f"}]` as a single
   expression producing `data/map`. This requires the lexer/engine to recognize
   jsonic syntax. This can be deferred to a follow-up — not needed for the core
   loop.

3. **Set predicates**: `>2` producing `uniq/predicate/number/integer` is a
   higher-level feature. Deferred.

4. **Store persistence**: `set`/`get` need a key-value store in the engine.
   Simple `map[string]Value` suffices initially.

5. **Multiple return values**: Functions like `dup` return multiple values.
   The handler signature `func(args []Value) ([]Value, error)` handles this —
   results are spliced onto the stack.
