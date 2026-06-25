# `aql:log` — logging, OpenTelemetry abstraction, and provider hooks

> **Status: phases 1–2 implemented; phases 3–5 proposed.** Phase 1 (the
> traditional-logging layer — six severity levels, structured fields, a
> global threshold, the console/memory/null sinks, the `log` policy scope,
> and the `LogSinkRegistry` host seam) and phase 2 (contextual loggers —
> `Log.logger` / `Log.with` / `Logger.child`) have landed:
> `lang/go/native/log_module.go`, `lang/go/native/log_logger.go`,
> `lang/go/modules/log.go`, `lang/spec/module-log.tsv`,
> `lang/go/modules/log_test.go`. The remaining phases (provider hooks,
> traces, metrics) are still proposals. This note specifies a new
> capability module `aql:log` (namespace `Log`) that gives AQL programs
> three layers on one surface: (1) **traditional logging** backed by Go's
> standard `log` package, (2) a vendor-neutral **OpenTelemetry abstraction**
> (logs, spans, and — phase 2 — metrics) that never pulls the OTel SDK into
> the sealed runtime, and (3) **provider hooks** — a registry of named
> *sinks* that hosts (in Go) or AQL code (via a `register` word) attach at
> runtime, modelled on the existing parse/emit host-registration pattern.
> Read [NATIVE-MODULES.10.md](NATIVE-MODULES.10.md),
> [EXTENSION-MODULES.10.md](EXTENSION-MODULES.10.md) §2, and
> [GO-MODULES.10.md](GO-MODULES.10.md) first.

## 1. Motivation

AQL today has exactly one output path: `print` / `IO.printstr` write a
value's formatted form to `Registry.Output`, and `IO.stderr` to
`Registry.ErrOutput` (`native/native_print.go`, `native/io_module.go`).
That is fine for REPL results; it is not logging. There is no severity, no
structured fields, no logger names, no way to fan a record out to more than
one destination, and no way for an embedding host to route AQL diagnostics
into the observability stack it already runs (OpenTelemetry, Datadog,
stdout-JSON shipped to Loki, …).

Three needs, one module:

1. **Traditional logging** — `Log.info "msg"`, levels, a default
   human-readable console sink built on the standard library `log`
   package. Works with zero host wiring, like `print` does.
2. **An OpenTelemetry abstraction** — a vendor-neutral in-memory shape for
   the OTel **logs** and **traces** signals (and, phase 2, **metrics**), so
   AQL code emits *records* and *spans* without naming OTel, and a host
   translates them to the OTel SDK at the boundary. The OTel SDK must stay
   a **host dependency**, never a runtime one — the AQL runtime is sealed
   ([GO-MODULES.10.md](GO-MODULES.10.md)) and must not grow a transitive
   `go.opentelemetry.io/*` graph.
3. **Provider hooks** — the same *pluggable named-handler* shape AQL already
   uses for parse/emit: a registry of **sinks**, extensible at runtime from
   the host (`RegisterHostLogSink`) **and** from AQL (`Log.register`),
   gated by policy.

The design reuses every existing seam — the native-module sub-registry
pattern (§7), the host-capability key pattern (`CapFileOps`/`CapClock`/…,
`native/capabilities.go`), the parse/emit registration shape
([EXTENSION-MODULES.10.md](EXTENSION-MODULES.10.md) §2), the context stack
(`r.PushContext`/`ctx-get`), and `EffectiveClock` for timestamps — and adds
**one** new capability key, **one** new policy scope, and no kernel change.

## 2. Design principles

- **Neutral core, OTel at the edge.** The module produces a `LogRecord`
  Record and a `Span` object in plain AQL value space. Translation to the
  OTel data model happens *inside a host-installed sink*, so the module
  imports nothing from `go.opentelemetry.io`. The OTel data model (severity
  number + text, body, attributes, timestamp, trace/span id) is the *shape*
  we target, because it is the broadest common denominator — a sink that
  maps it to slog, zap, or Datadog is trivial.
- **Works with zero wiring.** Like `print`, an unconfigured `Log.info`
  writes through a built-in `console` sink using Go's `log` package to
  `Registry.ErrOutput`. No host code required.
- **Fan-out, not single-writer.** Emitting a record walks every attached
  sink whose level threshold admits it. Adding OTel is *attaching a sink*,
  not replacing the console.
- **Deterministic + testable.** Timestamps come from `EffectiveClock(r)`
  (freezable via `SetHostClock`, `native/clock_capability_test.go`); a
  built-in `memory` sink captures records for assertions. No `time.Now()`,
  no global logger state.
- **No panics, policy-gated.** Every handler guards with `AsConcrete*`
  and errors via `r.AqlError` (CLAUDE.md Panic Prevention). Sink
  attachment and emission gate on a new `log` policy scope, so a sandbox
  can disable telemetry egress without disabling computation.

## 3. Surface (namespace `Log`)

Import binds the `Log` namespace (capability module — plain name, not
`-util`, per the lang/go/CLAUDE.md naming rule):

```
import "aql:log"
```

### 3.1 Severity levels

Six levels, named to the OTel severity bands so the mapping is lossless:

| word | OTel severity number | text |
|---|---|---|
| `Log.trace` | 1 | TRACE |
| `Log.debug` | 5 | DEBUG |
| `Log.info`  | 9 | INFO |
| `Log.warn`  | 13 | WARN |
| `Log.error` | 17 | ERROR |
| `Log.fatal` | 21 | FATAL |

Each is overloaded: a bare message, or a message plus a structured-field
Map (top-first sig order — the message is written first):

| signature (top-first) | meaning |
|---|---|
| `[String msg] →` | Emit a record at this level with body `msg`. |
| `[String msg, Map fields] →` | …plus structured attributes `fields`. |

```
"server started" Log.info
"request handled" {method:"GET" path:"/x" ms:12} Log.info
"db timeout" {host:"db1" attempt:3} Log.error
```

`Log.log` is the generic form taking an explicit level so a computed
severity works: `level message fields Log.log` (`Log.Level`, String, Map).

`Log.Level` is an **Enum** type (exported) — `Log.Level` literal admits the
six level atoms and the integers `1/5/9/13/17/21`; `x is Log.Level` checks
membership. Minted per import into the module sub-registry (no global
FixedID, like `IO.StreamKind`).

### 3.2 Structured records

Every emission constructs a `LogRecord`, the neutral OTel-logs shape:

```
def LogRecord (refine Record [
  timestamp:    DateTime      # from EffectiveClock(r)
  severity:     Integer       # OTel severity number 1..24
  severity-text:String        # "INFO", …
  body:         Any           # the message (String) or any Value
  attributes:   Map           # structured fields
  logger:       String        # logger name ("" = root)
  trace-id:     String        # current span context, "" if none
  span-id:      String
])
```

Exported as `Log.LogRecord` for `x is Log.LogRecord` checks and for sinks
that consume records (§5). The level words are sugar that build a
`LogRecord` and hand it to the emit pipeline; `Log.emit record` takes a
pre-built `LogRecord` directly (the seam custom sinks and replay use).

### 3.3 Named / contextual loggers

A logger carries a name and a set of default attributes merged into every
record it emits — the structured/contextual pattern (slog/zap/logrus).
Built exactly like a `Rand.with-seed` instance (`modules/rand.go`): a
fresh OrderedMap of the level methods closing over the logger's state.

| word | signature | meaning |
|---|---|---|
| `Log.logger` | `[String name] → Logger` | A logger bound to `name`. |
| `Log.with`   | `[String name, Map fields] → Logger` | Logger with default attributes. |
| `Logger.child` | `[Map fields] → Logger` (on an instance) | Derive a child merging more defaults. |

```
def reqlog (Log.with "http" {service:"api" region:"eu"})
"request" {path:"/x"} reqlog.info        # attributes = {service,region,path}
def childlog (reqlog.child {request-id:"abc"})
"slow" childlog.warn                      # carries service,region,request-id
```

`reqlog.info` / `reqlog.warn` / … expose the same six levels as the
top-level namespace; the instance methods inject `name` and merge the
default attributes (child defaults override parent on key collision, the
record's own fields override both — last-writer-wins, documented).

### 3.4 Configuration

| word | signature | meaning |
|---|---|---|
| `Log.set-level` | `[Log.Level] →` | Global minimum level; records below are dropped before fan-out. |
| `Log.get-level` | `→ Log.Level` | Current global threshold. |
| `Log.enabled`   | `[Log.Level] → Boolean` | Would a record at this level be emitted? (cheap guard before building expensive fields). |
| `Log.set-format`| `[Atom] →` | Console sink rendering: `text` (default) or `json`. |

Level threshold and format live on the **sink registry capability** (§6),
not in a Go global — so two registries (or two test cases) never share
mutable logger state.

### 3.5 Spans (the OTel *traces* signal)

A span is the neutral trace shape. `Log.span` opens one and returns a
`Span` handle; `Log.with-span` brackets a body and auto-closes (recording
any error as a span status), which is the form to prefer:

| word | signature | meaning |
|---|---|---|
| `Log.span` | `[String name] → Span` / `[String name, Map attrs] → Span` | Start a span; push its id onto the context stack. |
| `Log.with-span` | `[String name, List body] → Any` | Run `body` inside a span (NoEvalArgs body); end on exit; record raised errors. |
| `Span.set-attr` | `[Atom key, Any val] →` (on instance) | Add/replace a span attribute. |
| `Span.add-event`| `[String name, Map attrs] →` | Record a timestamped event on the span. |
| `Span.record-error` | `[Any err] →` | Mark span failed, attach error. |
| `Log.end` | `[Span] →` | Close a span explicitly (paired with `Log.span`). |
| `Log.current-span` | `→ Span` / `None` | The active span from context, if any. |

```
import "aql:log"
"handle-request" {route:"/x"} [
  "validating" Log.debug
  "db.query" ["select" Log.debug] Log.with-span      # nested child span
  "done" Log.info
] Log.with-span
```

Span identity (`trace-id` / `span-id`, W3C-trace-context hex) is generated
from `EffectiveClock` + a per-registry counter (deterministic under a fixed
clock for tests; a host OTel sink replaces them with real OTel ids when it
owns sampling — see §4.3). Current span context propagates through the
**existing** context stack (`r.PushContext` / `ctx-set` / `ctx-get`,
`native/capabilities.go` + the ctx words), so records emitted inside a span
auto-populate `trace-id`/`span-id`. No new propagation machinery.

### 3.6 Metrics (phase 2)

Deferred to keep v1 focused. Sketch: `Log.counter name`, `Log.gauge name`,
`Log.histogram name` return instrument handles; `Instrument.add` /
`.record` forward measurements to sinks that implement the (optional)
metrics half of the sink interface (§5). The neutral shape is an OTel
`Measurement` `{instrument, kind, value, attributes, timestamp}`. Console
sink aggregates and prints on flush; an OTel host sink maps to a `Meter`.

## 4. The OpenTelemetry abstraction

### 4.1 Why neutral-core / edge-translation

OTel's value is its *data model*, not its Go SDK. By targeting the data
model in plain AQL values and translating only inside a host sink, we get
OTel interop **without** a runtime dependency:

- The sealed runtime's `go.mod` stays free of `go.opentelemetry.io/*`
  (consistent with [GO-MODULES.10.md](GO-MODULES.10.md): generation /
  integration is build- or host-time, off the hot path).
- A host that *doesn't* use OTel pays nothing and still gets levels,
  structured fields, and spans through the console sink.
- A host that *does* use OTel writes ~30 lines of adapter (§4.2) and every
  existing `Log.info` / `Log.with-span` flows into its collector.

### 4.2 The host OTel bridge (illustrative, lives in host code)

```go
// Host code — NOT in the aql runtime module. Imports the OTel SDK here.
import (
    "go.opentelemetry.io/otel"
    otellog "go.opentelemetry.io/otel/log"
    "go.opentelemetry.io/otel/trace"
)

modules.RegisterHostLogSink(reg, modules.LogSinkSpec{
    Name:     "otel",
    MinLevel: native.LevelTrace,
    // Logs: neutral LogRecord -> OTel LogRecord.
    OnRecord: func(rec modules.LogRecord) error {
        var r otellog.Record
        r.SetTimestamp(rec.Timestamp)
        r.SetSeverity(otellog.Severity(rec.Severity))
        r.SetBody(otellog.StringValue(rec.BodyString()))
        for k, v := range rec.Attributes { r.AddAttributes(kv(k, v)) }
        provider.Logger("aql").Emit(rec.Ctx, r)
        return nil
    },
    // Traces: neutral Span -> OTel span (sampling/ids owned by OTel).
    OnSpanStart: func(s *modules.Span) (modules.SpanToken, error) { … },
    OnSpanEnd:   func(tok modules.SpanToken, s *modules.Span) error { … },
})
```

The bridge is the *only* place OTel is named. Swapping OTel for Datadog or
slog is swapping this adapter — the AQL programs are untouched.

### 4.3 Signal coverage

| OTel signal | Neutral AQL shape | v1? |
|---|---|---|
| **Logs** | `Log.LogRecord` Record (severity #, body, attrs, timestamp, trace context) | ✅ |
| **Traces** | `Span` object + context-stack propagation; events, status, attributes | ✅ |
| **Metrics** | `Measurement` `{instrument,kind,value,attrs,ts}` | phase 2 (§3.6) |
| **Baggage** | key/values on the context stack (`ctx-set`) | reuse existing |

When a host OTel **trace** sink is attached it owns id generation and
sampling: `OnSpanStart` returns a `SpanToken` carrying the real OTel
`trace-id`/`span-id`, which the module then surfaces through
`Log.current-span` and stamps onto records — so the neutral ids are used
only when no OTel sink is present (local-dev / tests).

## 5. Provider hooks — the sink registry

A **sink** is a named consumer of records (and optionally spans/metrics).
This is the parse/emit *pluggable named handler* pattern
([EXTENSION-MODULES.10.md](EXTENSION-MODULES.10.md) §2) applied to a third
domain.

### 5.1 The `LogSinkSpec` (host registration envelope)

```go
// lang/go/modules/log.go
type LogSinkSpec struct {
    Name     string                 // lowercase kind atom: "console","otel","memory",…
    MinLevel native.LogLevel        // per-sink threshold (record < MinLevel skipped)
    Filter   func(LogRecord) bool   // optional extra predicate (nil = accept)
    OnRecord func(LogRecord) error   // logs handler (required)
    OnSpanStart func(*Span) (SpanToken, error) // optional traces
    OnSpanEnd   func(SpanToken, *Span) error
    OnMeasure   func(Measurement) error          // optional metrics (phase 2)
}

// Mirrors RegisterHostParser / RegisterHostEmitter exactly:
//  - validates (lowercase name, non-nil OnRecord, no duplicate)
//  - stores under CapLogSinks, working before AND after `import "aql:log"`
//  - gates on the "log" policy scope
func RegisterHostLogSink(reg *native.Registry, spec LogSinkSpec) error
```

State lives under a new capability key `CapLogSinks = "engine.logsinks"`
holding a `*LogSinkRegistry` (the attached sinks, the global level, and the
console format) — the same "pending pre-import + live" storage parselang
uses (`engine.parselang.host`), so a host can register a sink before the
AQL program runs `import "aql:log"`.

### 5.2 AQL-level registration (`Log.register`)

So AQL code — not just the host — can add a sink, exactly as
`ParseLang.register` installs an AQL fn as a parser:

| word | signature | meaning |
|---|---|---|
| `Log.register` | `[fn handler, Atom name, Log.Level min] →` | Install an AQL fn `record →` as a sink. |
| `Log.add-sink` | `[Atom name] →` | Attach a *registered* sink to the active pipeline. |
| `Log.remove-sink` | `[Atom name] →` | Detach it. |
| `Log.sinks` | `→ List[Atom]` | Names of attached sinks. |

```
import "aql:log"
def capture (fn [[rec:Log.LogRecord] [] [ rec.body my-collector.push ]])
capture errors/q Log.error/q Log.register     # AQL sink, ERROR+ only
errors/q Log.add-sink
```

Built-in sinks (always registered, attachable by name):

| sink | destination | notes |
|---|---|---|
| `console` | Go `log` package → `Registry.ErrOutput` | **default**, attached at import; `text` or `json` via `Log.set-format`. |
| `memory`  | in-process ring buffer | test/inspection; `Log.dump` returns captured `LogRecord`s. |
| `null`    | discards | benchmarking / silencing. |
| `otel`    | *only if a host registered it* | the §4.2 bridge. |

### 5.3 The default `console` sink uses Go's `log`

To satisfy "traditional logging via the go `log` package" literally: the
`console` sink holds a `*log.Logger` constructed with
`log.New(r.ErrOutput, "", 0)` (AQL owns timestamp/level formatting, so the
std flags are off) and calls `logger.Output(...)` per record. `text`
format renders `2026-06-23T12:00:00Z INFO  [http] request method=GET …`;
`json` renders the `LogRecord` as a one-line JSON object (reusing the
`emit json` path) for log shippers. This is the zero-config path — present
the moment `aql:log` is imported, no host code.

## 6. Capability, policy, and determinism

### 6.1 New capability key

One addition to `native/capabilities.go`, following the existing pattern
exactly:

```go
const CapLogSinks = "engine.logsinks" // *LogSinkRegistry

func HostLogSinks(r *Registry) *LogSinkRegistry { … eng.Cap[…] … }
func SetHostLogSinks(r *Registry, reg *LogSinkRegistry) { … policy gate … }
```

`SetHostLogSinks` mirrors `SetHostFileOps`: if the policy has
`log.install=false`, the slot is left empty and emission becomes a no-op
(records dropped silently — logging must never crash a program). When a
policy is present without `install=false`, each sink call is gated.

### 6.2 New policy scope `log`

Add `"log"` to `policy.KnownScopes` and `IsCapabilityScope`
(`policy/policy.go`). Ops:

- `log:emit` — emit a record / start a span (gate telemetry *egress*).
- `log:install` — register/attach a sink.

A sandbox profile can `log:{install:false}` to forbid attaching new
sinks (e.g. block an untrusted script from adding an exfiltrating sink)
while still allowing `log:emit` to the host-controlled console — the
console/OTel routing stays the host's decision, the script cannot redirect
it. Default (no policy) = allow-everything, the established opt-in posture.

### 6.3 Determinism

- **Timestamps** via `EffectiveClock(r).Now()` — freeze with
  `SetHostClock(r, capabilities.FixedClock{...})` for golden tests.
- **Span ids** from clock + per-registry counter — reproducible under a
  fixed clock; replaced by a host OTel sink when present (§4.3).
- **No global logger.** All state (attached sinks, level, format) lives on
  `CapLogSinks` per registry, so concurrent registries and table tests are
  isolated — the same discipline `aql:rand` follows for its PRNG.

## 7. Implementation shape (Go)

Standard native-module wiring (`NATIVE-MODULES.10.md` §"Adding a New
Native Module"), structured like `modules/rand.go` (stateful instances)
crossed with `modules/io.go` (host capability reach-through):

1. `lang/go/modules/log.go` — `BuildLogModule(parent) (ModuleDesc, error)`:
   builds a sub-registry, registers the level / span / config / sink
   natives, mints the module-scoped `Log.Level` enum and `Log.LogRecord` /
   `Span` / `Logger` types into it (per-import, no FixedID — the
   `IO.StreamKind` precedent), and exports FnDef wrappers
   (`makeModuleFnDef`, **`BarrierPos: -1`** on every inner sig per the
   Module-FnDef-wrapper rule in lang/go/CLAUDE.md).
2. `lang/go/native/log_module.go` — `LogModuleNativeFuncs(...)`: the Go
   handlers. Emission walks `HostLogSinks(r)`; the `console` sink uses the
   std `log` package against `r.ErrOutput`. All handlers guard with
   `AsConcrete*` and error via `r.AqlError("log_error", …, word)`.
3. `lang/go/modules/modules.go` — add `"log": BuildLogModule` to the
   `modules` map and an `InstallLogExports` test helper.
4. `lang/go/modules/docs_log.go` — `registerDocs("aql:log", {...})` for
   every export (asserted by `TestModuleExportDocs`).
5. `native/capabilities.go` — `CapLogSinks` + `HostLogSinks` /
   `SetHostLogSinks`; `policy/policy.go` — the `log` scope.
6. `RegisterHostLogSink` in `modules/log.go` (the §5.1 envelope), plus the
   built-in `console`/`memory`/`null` sink registrations.

The module imports **only** `native`, the std `log`, and `policy` — no
`go.opentelemetry.io`. OTel enters solely through a host-registered sink.

## 8. Errors

Kebab `r.AqlError` codes, no panics (CLAUDE.md):

- `log_error` — generic (bad level atom, malformed fields).
- `unknown-sink` — `Log.add-sink` / `register` names an unregistered kind.
- `sink-exists` — duplicate `Log.register` name.
- `span-mismatch` — `Log.end` on a span not on the context stack top.
- `capability_not_installed` — emission attempted with `log.install=false`
  is a **silent drop**, not this error (logging must not abort the
  program); this code is reserved for explicit sink *management* under a
  disabled scope.

## 9. Testing

Per the test discipline (always pair positive with negative):

- `lang/spec/module-log.tsv` — positive rows (each level builds the right
  `LogRecord` against a frozen clock + `memory` sink) and `ERROR:` rows
  (unknown sink, duplicate registration, bad level, `Log.end` mismatch).
- Go tests: frozen-clock golden records; fan-out to two sinks; per-sink
  level filtering; `Log.with-span` records a raised error and still closes;
  policy `log:{install:false}` blocks `add-sink` but lets `emit` reach the
  console; `recover()`-based no-panic test passing type literals to every
  level/span/sink word (`TestTypeLiteralNoPanic` discipline).
- A host-bridge test using a fake OTel sink (a `LogSinkSpec` that appends to
  a slice) proves the neutral→adapter handoff without importing OTel.

## 10. Open questions

- **Metrics in v1 or phase 2?** Leaning phase 2 (§3.6) — logs + traces
  cover the dominant need and keep the v1 surface small.
- **`fatal` semantics.** Emit-and-`os.Exit` (classic), or emit at FATAL
  severity and *return* (let the program decide)? Lean: **emit + raise an
  AQL error** (`log_fatal`), so control flow stays in AQL and the sealed
  runtime never calls `os.Exit`. Confirm.
- **One `log` scope, or split `log.emit` / `log.sinks`?** Proposed: one
  scope, two ops (§6.2). Sufficient for the sandbox-can't-redirect case.
- **Console default to stderr or a dedicated writer?** Proposed
  `r.ErrOutput` (diagnostics ≠ program results on `r.Output`). Confirm.
- **Should `Log.LogRecord` carry a `resource` map** (service.name, host) à
  la OTel Resource, or leave that to the host sink? Lean: a `Log.set-resource`
  global map merged by the sink, mirroring OTel's Resource — but defer to
  phase 2 unless a concrete need appears.

## 11. Implementation phases

1. **Core logs** — module skeleton, six levels, `LogRecord`, global level,
   `console` (Go `log`) + `memory` + `null` sinks, `CapLogSinks`, `log`
   policy scope, docs + spec rows. Zero-config `Log.info` works.
2. **Contextual loggers** — `Log.logger` / `Log.with` / `Logger.child`
   instances (the `Rand.with-seed` shape).
3. **Provider hooks** — `RegisterHostLogSink` + `Log.register` /
   `add-sink` / `remove-sink` / `sinks`; the host OTel-logs bridge example.
4. **Traces** — `Span`, `Log.span` / `with-span` / `end`, context-stack
   propagation, `OnSpanStart`/`OnSpanEnd` sink hooks, OTel-traces bridge.
5. **Metrics (phase 2)** — instruments, `Measurement`, the metrics half of
   the sink interface, OTel-metrics bridge.

Each phase ships positive + negative specs and, for the capability/policy
seam, `recover()`-based no-panic tests — the standard gate
(lang/go/CLAUDE.md "Test discipline").
