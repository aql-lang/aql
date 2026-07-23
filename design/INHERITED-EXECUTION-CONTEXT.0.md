# Inherited Execution Context

**Status:** Discovery draft  
**Scope:** Core runtime design; not an ADR  
**Primary implementation areas:** evaluator, task creation, native module calls, checker, bytecode VM

## 1. Problem

AQL already has an implicit context store with inherited, prototype-like semantics. That store is useful for application configuration and dynamically scoped values, but platform work introduces a second, stricter requirement: operations such as HTTP requests, database calls, filesystem work, timers and asynchronous tasks need a common way to observe cancellation, deadlines, identity, policy and tracing information.

Go solves part of this problem with `context.Context`, passed explicitly through every participating call. Node.js solves a related problem with `AsyncLocalStorage`, which associates data with an asynchronous execution chain. AQL should adopt neither interface verbatim. It already has language-level inherited context, so the AQL-native design should use that strength while adding runtime guarantees that the general context store does not currently promise.

Without a composable execution context, each API must invent timeout options and explicitly forward them through every intermediate call. Scoped timeout helpers would merely recreate an implicit context under another name. Deep cancellation and request-scoped metadata therefore require a runtime-level facility.

## 2. Decision direction

Introduce an **inherited execution context** as a core runtime facility.

It is not an `aql:context` platform module. It is part of evaluation and task semantics, beneath modules such as `aql:time`, `aql:net`, `aql:os`, database modules and future server frameworks.

The existing user context store remains available for its current purposes. The execution context is an extension or protected lane, not a replacement.

## 3. Conceptual model

Each running AQL task has an execution-context frame. A child task inherits its parent frame and may derive a child frame with overrides. Frames should be immutable or copy-on-write from the point of view of ordinary AQL code.

The context contains separate typed lanes rather than an unstructured global bag:

- **Lifecycle:** cancellation state, deadline and cancellation reason.
- **Identity:** authenticated principal and identity claims.
- **Policy:** capabilities and operation-specific policy.
- **Observability:** trace identifier, span state, request identifier and structured diagnostic fields.
- **Host state:** narrowly scoped runtime handles required by platform modules.

The lane separation is conceptual even if an initial implementation uses one internal structure. A user identity claim must not be mistaken for a capability, and arbitrary user context must not be treated as trusted runtime policy.

## 4. Inheritance and concurrency

The execution context is task-local, not process-global. Concurrent requests must never share mutable context frames.

Required rules:

1. A child task inherits the parent frame as it existed when the child was created.
2. A child may derive a local frame without mutating its parent or siblings.
3. Cancellation propagates from parent to child.
4. Child cancellation does not cancel its parent unless an explicit joining construct defines that behaviour.
5. Deadlines compose by selecting the earliest effective deadline.
6. Context values survive ordinary calls and bytecode transitions.
7. Detached/background tasks must explicitly declare whether context is retained, reduced or replaced.
8. Interpreter fallback from compiled code must preserve the same frame.

These rules make the facility closer to async-local state plus structured cancellation than to a mutable registry.

## 5. Middleware and server use

A server request creates a root request frame. Middleware enriches that frame in stages:

1. The listener installs request and connection metadata.
2. A request-ID layer adds a correlation identifier.
3. Authentication validates a JWT, session or host identity and installs an identity value.
4. Authorization derives a capability set or policy decision.
5. Observability layers attach trace state.
6. Handlers and platform words consult the resulting inherited frame.

Identity data is informative. Capability data is authoritative. Sensitive words must check capabilities or host policy rather than trusting a user-controlled identity map.

## 6. Cancellation and deadlines

Cancellation is represented as runtime state, not as an arbitrary context key. It must be cheap to inspect and safe to observe concurrently.

A scope may be derived with:

- an explicit deadline,
- a relative timeout,
- manual cancellation,
- cancellation on a host event such as an OS termination signal,
- cancellation inherited from its parent.

A cancellation reason should be structured so callers can distinguish timeout, explicit cancellation, parent cancellation, signal termination and host shutdown.

Platform APIs should accept the implicit current scope by default. Explicit scope values may be considered later for advanced composition, but ordinary code should not be forced to thread a parameter through every call.

## 7. Reserved system namespace

A runtime-controlled namespace is desirable, potentially using AQL's `$` convention. The exact syntax is unresolved and is covered by the separate dollar-namespace discovery note.

Regardless of syntax, ordinary context mutation must not be able to forge lifecycle, identity-policy or capability state. Runtime mutation requires privileged internal APIs.

The preferred implementation is typed slots, not one `$state` map. This avoids collisions, supports static typing and prevents accidental capability leakage.

## 8. Public language surface

The execution context should primarily appear through scoped operations rather than a general context module. Candidate forms include concepts equivalent to:

- execute a block with a timeout,
- execute a block with a deadline,
- derive a cancellable scope,
- query cancellation,
- obtain a structured cancellation reason,
- attach trace fields,
- install identity or policy through trusted middleware APIs.

Names and exact signatures remain open until existing core and module exports have been checked for ADR-001 collisions.

The language should not expose arbitrary mutation of protected slots.

## 9. Type checking

The checker needs known carrier types for protected values such as `Deadline`, `Cancellation`, `Principal`, `CapabilitySet`, `TraceContext` and `ExecutionScope`.

Important rules:

- Access to a known protected slot has a known result type.
- A capability-gated operation may be statically rejected when the policy is compile-time known.
- Unknown runtime policy remains a runtime check; the checker must not pretend authorization is proven.
- Cancellation paths must be represented in effect or error analysis if AQL later formalizes effects.
- Protected values must not silently degrade to generic maps.
- Scope-producing words must have precise block result types.

User context remains dynamically extensible; protected execution state should be strongly typed.

## 10. Bytecode and VM

Bytecode execution must carry the current execution frame alongside the ordinary evaluation state.

Required VM behaviour:

- scope entry derives a frame;
- scope exit restores the prior frame;
- errors, traps and early returns restore correctly;
- tail calls preserve the current frame;
- asynchronous spawn captures the frame according to the task inheritance rule;
- cancellation can interrupt blocking native operations;
- interpreter islands use the same frame object;
- compiled artifacts do not embed host-specific deadlines or policy values.

The scope stack must unwind reliably on all control-flow exits. This is a correctness and security property.

## 11. ADR and NUR review

This design must preserve the following established constraints:

- New module words must not shadow core words.
- Public native exports require language-level spec coverage.
- New words are forward-collecting unless intrinsically stack-oriented.
- Errors cross the Go/AQL boundary as AQL errors rather than panics.
- No private parser or mini-language should be introduced for scope configuration.
- Native Go implementation requires corresponding Go tests.

A protected system lane may create a non-uniformity if ordinary context and protected context present superficially identical mutation APIs with different permissions. The implementation should avoid that by making them visibly different facilities. If a genuine special case remains, it must be recorded in the NUR process when implementation begins.

## 12. Security properties

- User data cannot forge capabilities.
- Request A cannot read or mutate request B's context.
- Child overrides cannot mutate parent or sibling frames.
- Cancellation state is monotonic.
- Deadlines cannot be extended beyond a parent deadline without an explicit privileged operation.
- Trusted middleware APIs distinguish verified values from untrusted claims.
- Diagnostic context must not automatically expose secrets.

## 13. Open questions

1. Exact relationship between the general context registry and protected runtime slots.
2. Whether `$` names identify protected slots or whether protection is represented by a distinct internal key type.
3. Syntax and names for scoped timeout/cancellation words.
4. Semantics for detached tasks.
5. Whether context values can be captured and re-entered explicitly.
6. How cancellation interacts with non-interruptible native work.
7. Whether the checker will model capabilities as types, effects or runtime-only policy.
