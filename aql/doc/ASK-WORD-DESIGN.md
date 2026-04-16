# Design Document: `ask` Word — LLM Integration for AQL

## Context

AQL needs a native interface to large language models. The `ask` word provides
a concatenative, composable way to query LLMs that fits AQL's existing
patterns: options via maps, structured return types, configuration via the
context store, and tool integration via the module/function system.

The design prioritizes: minimal API surface, configuration through existing
AQL mechanisms (context store, options maps), and exposing AQL functions as
LLM tools without a separate tool-definition language.

---

## Core Word: `ask`

### Signatures

```
ask String               → String       basic prompt, default model
ask String Map            → String|Map   prompt with options
ask Map                   → String|Map   conversation (messages in map)
ask Map Map               → String|Map   conversation with options
```

All forms support forward precedence:
```aql
ask "What is 2+2?"                          # all forward
"What is 2+2?" ask                          # all prefix
"What is 2+2?" ask {model: "claude"}        # mixed
```

### Basic usage

```aql
# Simple question → string answer
ask "What is the capital of France?"
# => "Paris"

# With options
ask "Translate to French: hello" {model: "claude-sonnet-4-20250514", temp: 0.3}
# => "Bonjour"

# JSON response
ask "List 3 colors as JSON array" {format: "json"}
# => ["red", "green", "blue"]   (returned as AQL List, not a string)

# Conversation with message history
ask {
  messages: [
    {role: "system", content: "You are a helpful assistant."}
    {role: "user", content: "What is AQL?"}
  ]
}
```

---

## Options Map

```aql
{
  model:      "claude-sonnet-4-20250514"  # model identifier (default from config)
  temp:       0.7             # temperature 0.0-2.0
  max:        1024            # max output tokens
  format:     "text"          # "text" (default), "json", "list"
  timeout:    30000           # timeout in ms (default: 60000)
  system:     "You are..."    # system prompt (shorthand for messages)
  tools:      [...]           # tool definitions (see Tool Integration)
  stop:       ["\n\n"]        # stop sequences
  cache:      true            # enable prompt caching (default: true)
  retry:      3               # retry count on transient failure (default: 1)
  seed:       42              # deterministic sampling seed
}
```

Options follow the existing `parseStrOpts` pattern from string words
(`native_string_helpers.go:59-219`). Unknown keys are ignored with a
warning, not an error — forward-compatible with new model features.

### Format option behavior

| Format | LLM instruction | Return type | Parse behavior |
|---|---|---|---|
| `"text"` (default) | None | `String` | Raw response text |
| `"json"` | JSON mode / schema | `Map` or `List` | Parse JSON, error if invalid |
| `"list"` | None | `List` of `String` | Split response on newlines |
| `"jsonic"` | None | `Map` or `List` | Parse as Jsonic |

This mirrors `read`'s format handling in `fileio.go:230-250`.

---

## Configuration

### Context store approach

LLM configuration lives in the context store, following the existing
`__sys` store pattern from `registry.go:305-335`:

```aql
# Set default model for session
context set "ai" {
  model:    "claude-sonnet-4-20250514"
  api_key:  (env "ANTHROPIC_API_KEY")
  base_url: "https://api.anthropic.com"
  cache:    true
  timeout:  60000
}

# Now all ask calls use these defaults
ask "Hello"   # uses claude-sonnet-4-20250514
```

### Configuration precedence (lowest to highest)

1. **Built-in defaults** — hardcoded fallbacks (model: "claude-sonnet-4-20250514", timeout: 60000)
2. **Environment variables** — `ANTHROPIC_API_KEY`, `AQL_AI_MODEL`, `AQL_AI_BASE_URL`
3. **Context store** — `context set "ai" {...}` persists across REPL lines
4. **Per-call options** — `ask "prompt" {model: "gpt-4o"}` overrides everything

This is the same layering pattern as file I/O options: the `read` word
has defaults, per-call `{fmt: "json"}` overrides, and the context can
hold path configuration.

### Provider abstraction

The `base_url` + `api_key` pattern supports any OpenAI-compatible API:

```aql
# Anthropic (default)
context set "ai" {api_key: (env "ANTHROPIC_API_KEY")}

# OpenAI
context set "ai" {
  base_url: "https://api.openai.com/v1"
  api_key:  (env "OPENAI_API_KEY")
  model:    "gpt-4o"
}

# Local (Ollama)
context set "ai" {
  base_url: "http://localhost:11434/v1"
  api_key:  "ollama"
  model:    "llama3"
}

# Local (LM Studio / llama.cpp)
context set "ai" {
  base_url: "http://localhost:1234/v1"
  api_key:  "local"
  model:    "local-model"
}
```

All providers speak the OpenAI-compatible chat completions API. The
implementation sends to `{base_url}/chat/completions` with the
appropriate auth header. Provider-specific features (Anthropic's
prompt caching, OpenAI's response_format) are handled transparently
based on the base_url domain or an explicit `provider` option.

---

## Tool Integration

### Exposing AQL functions as tools

AQL functions defined with `fn` already have typed signatures. These
can be exposed as LLM tools directly:

```aql
# Define a function
def weather fn [[city:String] [Map] [
  # ... fetch weather data ...
  {temp: 22, condition: "sunny", city: city}
]]

# Expose as a tool to the LLM
ask "What's the weather in Paris?" {
  tools: [
    {
      name:   "weather"
      fn:     weather
      desc:   "Get current weather for a city"
    }
  ]
}
```

### How tool calling works

1. `ask` sends the prompt with tool definitions derived from AQL `fn` signatures
2. If the LLM requests a tool call, the `ask` handler:
   a. Looks up the function in the registry via `r.Lookup(name)`
   b. Converts the LLM's JSON arguments to AQL values
   c. Executes the function via a sub-engine (`New(r)` + `sub.Run(...)`)
   d. Converts the result back to JSON
   e. Sends the tool result back to the LLM
   f. Returns the LLM's final response
3. Multi-turn tool calling loops up to a configurable limit (default: 10)

### Tool definition from `fn` signatures

An AQL function like:

```aql
def lookup fn [[key:String] [Map] [context get key]]
```

Has signature `[String] → Map`. The tool definition sent to the LLM is:

```json
{
  "type": "function",
  "function": {
    "name": "lookup",
    "description": "Get current weather for a city",
    "parameters": {
      "type": "object",
      "properties": {
        "key": {"type": "string"}
      },
      "required": ["key"]
    }
  }
}
```

The type mapping:

| AQL type | JSON Schema type |
|---|---|
| String | `"string"` |
| Integer | `"integer"` |
| Decimal | `"number"` |
| Boolean | `"boolean"` |
| List | `"array"` |
| Map | `"object"` |
| Any | omit type constraint |

### Inline tool definitions (no pre-existing fn)

```aql
ask "Calculate 15% tip on $42.50" {
  tools: [
    {
      name:   "calculate"
      desc:   "Evaluate a math expression"
      params: {expression: String}
      body:   [expression do]
    }
  ]
}
```

When `body` is present instead of `fn`, the handler creates a temporary
function from the body list, following the same pattern as `each`/`fold`
which create sub-engines per invocation (`native_array_higher.go:29`).

---

## REPL Integration

### Interactive behavior

In the REPL, `ask` blocks until the response is complete (matching REPL's
synchronous model from `repl/repl.go`). The registry persists across lines,
so configuration set via `context set` carries forward:

```
aql> context set "ai" {model: "claude-sonnet-4-20250514"}
aql> ask "Hello"
Hello! How can I help you?
aql> ask "What did I just say?" {messages: (context get "ai_history")}
```

### Conversation history

`ask` can optionally store conversation history in the context:

```aql
context set "ai" {history: true}
ask "Hello"             # stores in context ai.messages
ask "Follow up"         # automatically includes prior messages
context get "ai" get "messages"  # access full history
```

When `history: true` is set, each `ask` call appends the user message and
assistant response to `ai.messages` in the context store. This uses the
existing COW prototype chain from `native_storage_set.go:99-148`.

### Streaming (future extension)

A future `ask-stream` word or `{stream: true}` option could return chunks
via the temporal/interval pattern. For now, `ask` always blocks and returns
the complete response. This matches how `read` blocks on file I/O.

---

## Module Structure

### `aql:ai` native module

```aql
"aql:ai" import

"What is AQL?" ai.ask
"Translate: hello" ai.ask {model: "gpt-4o", format: "json"}
```

The module lives at `aql/internal/nativemod/ai.go` following the existing
pattern from `nativemod/math.go`. Registration in `nativemod.go:22-28`:

```go
var modules = map[string]func(...) (engine.ModuleDesc, error){
    "math":   BuildMathModule,
    "time":   BuildTimeModule,
    "ai":     BuildAIModule,    // NEW
    // ...
}
```

### Exported words

| Word | Signature | Description |
|---|---|---|
| `ask` | `[String] → String` | Basic prompt |
| `ask` | `[String, Map] → String\|Map` | Prompt with options |
| `ask` | `[Map] → String\|Map` | Conversation messages |
| `ask` | `[Map, Map] → String\|Map` | Conversation with options |
| `embed` | `[String] → List` | Generate embedding vector |
| `embed` | `[List] → List` | Batch embeddings |
| `models` | `[] → List` | List available models |
| `tokens` | `[String] → Integer` | Estimate token count |

### Also register `ask` as a core word

Because `ask` is high-frequency, it should also be available without import
(like `add`, `if`, etc.). Register it in `registerCoreWords` in addition
to the module export. The module provides extended words (`embed`, `models`,
`tokens`) that are less commonly needed.

---

## Error Handling

Follows the existing pattern from `native_control_error.go`:

```aql
# Errors propagate normally
ask "prompt"
# If API fails: [aql/ai_error]: API request failed: 429 Too Many Requests

# Catch with error word
ask "prompt" error [
  # error value is on stack
  "LLM failed, using fallback" print
  "default answer"
]

# Timeout
ask "prompt" {timeout: 5000}
# If timeout: [aql/ai_error]: request timed out after 5000ms
```

Error codes:
- `ai_error` — general API failure
- `ai_timeout` — request exceeded timeout
- `ai_parse` — response format parsing failed (e.g., invalid JSON when format: "json")
- `ai_auth` — authentication failure (missing/invalid API key)
- `ai_tool` — tool execution failed during tool calling loop

---

## Implementation Architecture

### Go package structure

```
aql/internal/engine/
  native_ai_ask.go          # ask word registration and handler
  ai_client.go              # HTTP client for LLM APIs
  ai_tools.go               # Tool definition and execution logic
  ai_config.go              # Configuration resolution (env → context → options)

aql/internal/nativemod/
  ai.go                     # aql:ai module builder (embed, models, tokens)
```

### HTTP client design

The client uses Go's `net/http` standard library (no third-party deps):

```go
type aiClient struct {
    httpClient *http.Client
    baseURL    string
    apiKey     string
    provider   string  // "anthropic", "openai", "local"
}

func (c *aiClient) chatCompletion(req chatRequest) (*chatResponse, error) {
    // Marshal request
    // POST to {baseURL}/chat/completions (or /v1/messages for Anthropic)
    // Unmarshal response
    // Handle streaming vs non-streaming
}
```

This keeps the dependency count at zero — Go's `net/http`, `encoding/json`,
and `crypto/tls` handle everything needed for HTTPS API calls.

### Configuration resolution

```go
func resolveConfig(r *engine.Registry, opts *engine.OrderedMap) aiConfig {
    cfg := aiConfig{
        Model:   "claude-sonnet-4-20250514",
        Timeout: 60000,
        Cache:   true,
        Retry:   1,
    }

    // Layer 1: environment variables
    if v := os.Getenv("AQL_AI_MODEL"); v != "" { cfg.Model = v }
    if v := os.Getenv("ANTHROPIC_API_KEY"); v != "" { cfg.APIKey = v }

    // Layer 2: context store
    if store := r.ContextStore(); store != nil {
        if aiVal, ok := store.Get("ai"); ok {
            // merge aiVal map fields into cfg
        }
    }

    // Layer 3: per-call options
    if opts != nil {
        // merge opts fields into cfg (highest priority)
    }

    return cfg
}
```

---

## Security Considerations

- **API keys**: Never logged, never included in error messages, never
  returned as values. Stored in context store which is scoped and
  garbage-collected.
- **Tool execution**: Tools execute in a sub-engine with the same
  registry — they have the same permissions as the calling code.
  No sandboxing beyond AQL's existing execution model.
- **Network access**: `ask` is the first AQL word that makes outbound
  HTTP requests. This should be gated by a capability flag on the
  registry (similar to how file I/O can be disabled).
- **Prompt injection**: Tool results returned to the LLM are clearly
  delimited as tool responses. AQL does not parse or execute LLM
  output unless `format: "json"` explicitly requests parsing.
- **Local models**: Supporting `base_url` for local models means `ask`
  works fully offline with no external network access.

---

## Usage Patterns

### Data transformation pipeline

```aql
# Read CSV, summarize with LLM, write report
"data.csv" read
  ask "Summarize this data as key findings" {format: "json"}
  get "findings"
  each [get "description"]
  concat {sep: "\n- "}
  "- " add
  "report.md" write
```

### Structured extraction

```aql
# Extract entities from text
def extract fn [[text:String, schema:Map] [Map] [
  text ask {
    system: "Extract data matching this schema. Return JSON."
    format: "json"
  }
]]

"John Smith, age 42, lives in NYC"
  extract {name: String, age: Integer, city: String}
# => {name: "John Smith", age: 42, city: "NYC"}
```

### Multi-model comparison

```aql
# Ask same question to multiple models, compare
def models ["claude-sonnet-4-20250514" "gpt-4o" "llama3"]
models each [
  def m
  {model: m, answer: ("What is 1+1?" ask {model: m})}
]
# => [{model: "claude-sonnet-4-20250514", answer: "2"}, ...]
```

### Agent loop with tools

```aql
def search fn [[query:String] [List] [
  # ... search implementation ...
]]

def calculate fn [[expr:String] [String] [
  expr do convert String
]]

ask "What is the population of France divided by the area in km2?" {
  tools: [
    {name: "search", fn: search, desc: "Search for information"}
    {name: "calculate", fn: calculate, desc: "Calculate a math expression"}
  ]
  system: "Use tools to find facts and compute answers."
}
```

---

## Testing Strategy

### Unit tests

- `TestAskBasicPrompt` — mock HTTP server, verify request format
- `TestAskWithOptions` — verify all option fields propagated
- `TestAskJSONFormat` — verify JSON response parsing to Map
- `TestAskListFormat` — verify line-split response
- `TestAskConfigPrecedence` — env < context < options
- `TestAskTimeout` — verify timeout cancellation
- `TestAskRetry` — verify retry on 429/500
- `TestAskToolCalling` — verify tool call loop with mock
- `TestAskToolFromFn` — verify AQL fn → tool definition conversion
- `TestAskErrorHandling` — verify error codes and recovery
- `TestAskConversationHistory` — verify message accumulation

### Integration tests

- Use Go's `httptest.NewServer` for mock LLM API
- Test against Ollama locally when available (`AQL_TEST_OLLAMA=1`)
- REPL session test: multi-line conversation with context persistence

---

## Open Questions

1. **Should `ask` be a core word or module-only?** Recommendation: core word
   (registered in `registerCoreWords`), with extended words in `aql:ai` module.
   Rationale: LLM access is becoming as fundamental as file I/O.

2. **Streaming**: Defer to future `ask-stream` or `{stream: true}`. The
   temporal/interval pattern (`native_temporal_interval.go`) provides the
   substrate but adds complexity to the initial implementation.

3. **Conversation state**: Auto-history via `{history: true}` in context
   vs explicit message passing. Recommendation: support both — explicit
   is the default, auto-history is opt-in.

4. **Token counting**: Should `tokens` be a separate word or a return
   field? Recommendation: separate word in the module, plus `ask` can
   return usage metadata via `{meta: true}` option that wraps the
   response in `{content: "...", usage: {input: N, output: M}}`.
