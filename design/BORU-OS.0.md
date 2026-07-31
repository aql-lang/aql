# `boru:os` Platform Module

**Status:** Discovery draft  
**Module:** `boru:os`  
**Purpose:** Portable host-operating-system facts and lifecycle integration

## 1. Positioning

Go's `os` package spans files, processes, environment and host interaction. Node's `os` module is narrower and primarily exposes host facts. boru should follow the narrower model.

`boru:os` is not a dumping ground for every host API. It exposes portable operating-system facts, conventional directories, process metadata, feature probing and safe process-lifecycle integration.

File operations remain in boru's I/O facilities. Path manipulation remains in the path module. Networking remains in networking modules. Process execution belongs in a separate, capability-gated execution facility.

## 2. Design principles

- Portable names and values where a portable abstraction exists.
- Explicit `none`/unsupported results where it does not.
- Read-only facts are easy to access.
- Mutating or sensitive operations are capability-gated.
- Host-specific values are not constant-folded into portable bytecode.
- Signal delivery never executes arbitrary boru code inside a native signal handler.
- API names must be checked against core words before finalization.

## 3. Proposed surface areas

### 3.1 Platform identity

Provide normalized host facts:

- operating-system family/platform,
- architecture,
- machine architecture where distinct,
- endianness,
- kernel or OS release,
- human-readable OS version,
- hostname,
- line-ending convention,
- null-device path.

Raw host strings may be available alongside normalized enum-like values when useful.

### 3.2 Resource facts

Provide:

- logical parallelism,
- total and available memory where supported,
- uptime,
- load averages where meaningful,
- page size or other low-level facts only if a clear portable use case exists.

Unsupported resource facts return a documented optional result rather than invented values.

### 3.3 Conventional directories

Provide normalized paths for:

- home directory,
- temporary directory,
- user configuration,
- user cache,
- user data,
- system configuration where meaningful.

Directory lookup should follow platform conventions rather than forcing Unix paths onto Windows. Merely returning a directory does not create it.

### 3.4 Process facts

Read-only process facts may include:

- process identifier,
- parent process identifier,
- argument vector,
- executable path,
- current working directory.

Changing the working directory affects process-global state and is unsafe under concurrency. If exposed, it must be capability-gated and documented as process-wide. Prefer explicit base paths in APIs over routine `chdir`.

### 3.5 Environment

Environment access is host interaction and can reveal secrets.

Provide:

- lookup of a named variable,
- optional enumeration of the environment,
- a snapshot rather than a live mutable map.

Environment mutation is process-global and concurrency-sensitive. Defer it or gate it strongly. Platform applications should prefer explicit configuration and inherited execution context.

### 3.6 User facts

Basic current-user metadata may be exposed when the host can provide it, but authentication and biometric identity do not belong here. Host keychains, user verification, passkeys and hardware-bound keys are deferred to a future `boru:trust` design.

## 4. Signals

Signals require both a safe portable layer and an advanced host layer.

### 4.1 Portable lifecycle layer

The default API maps process termination events to structured boru lifecycle behaviour:

- interrupt and terminate cancel the root or selected execution scope;
- cancellation includes a structured signal reason;
- service code performs orderly shutdown through ordinary boru control flow;
- hangup may be exposed as a reload event on systems where it exists.

This is sufficient for most CLI tools and daemons.

### 4.2 Event delivery

Where applications need explicit signal handling, the runtime may expose a queued signal stream or subscription:

- native handlers perform only async-signal-safe bookkeeping;
- the runtime queues a signal event;
- boru code observes the event later on the normal scheduler;
- subscriptions have explicit lifetime and cleanup;
- duplicate/coalesced delivery semantics are documented.

boru callbacks are never invoked directly from a POSIX signal handler.

### 4.3 Raw and unsafe operations

Advanced operations such as ignoring a signal, restoring default disposition or sending a signal are capability-gated host operations.

Uncatchable signals such as `SIGKILL` and `SIGSTOP` are represented accurately. The API must not imply they can be intercepted.

Signal names should use portable symbolic identities where possible, with host support probing. Numeric values are host-specific implementation facts and should not be the primary interface.

### 4.4 Daemon lifecycle

A conformant service typically needs to:

1. establish the root execution scope;
2. register termination/reload events;
3. start workers;
4. cancel on termination;
5. stop accepting new work;
6. await or bound worker shutdown;
7. flush logs/state;
8. exit with a meaningful status.

`boru:os` provides lifecycle events; structured task and execution-context facilities provide the shutdown mechanics. Traditional Unix daemonization such as fork/session/double-fork is not a default portable abstraction and may be left to a host-specific service layer.

## 5. Feature probing

Provide a uniform way to ask whether an optional host feature is available. This is preferable to platform-name branching.

Examples include load average support, particular signals, user lookup, memory availability and secure host services.

Feature probes must have stable symbolic names and language-level spec coverage.

## 6. Capability model

Read-only low-sensitivity facts may be available by default. Potentially sensitive or mutating operations require host policy capabilities, including:

- full environment enumeration,
- environment mutation,
- process-wide working-directory mutation,
- raw signal disposition,
- sending signals to other processes,
- privileged host details.

Capabilities are part of the inherited execution context or host policy, not arbitrary arguments that untrusted code can forge.

## 7. Type checking

Use precise types for platform, architecture, signal, path, duration, byte count and process identifier values. Do not return a generic map when a stable record type is possible.

Capability-gated operations remain type-correct even when policy is dynamic. If compile-time policy is known, the checker may reject an impossible call early; otherwise runtime enforcement remains authoritative.

Optional host facts must have explicit optional/result types.

## 8. Bytecode

Most OS facts are runtime operations. Do not fold platform, architecture, directories, environment or process identifiers into bytecode unless compiling for a declared fixed target and the value is explicitly target-level rather than machine-level.

Pure normalization helpers can compile normally. Signal subscription and lifecycle scopes need reliable cleanup during errors, early returns and cancellation.

## 9. Testing and specifications

Every public export requires a formal spec row. Hermetic specs should cover normalization, validation and deterministic failure paths. Host-dependent success cases belong in Go tests with platform guards.

Signal tests must not make the test runner flaky. Use controllable internal event injection for most semantics and a small set of real-signal integration tests.

## 10. Non-goals

- Filesystem read/write APIs.
- Path parsing and joining.
- Networking.
- Shell or process execution.
- Direct biometric APIs.
- Vault or keychain business logic.
- A full POSIX compatibility layer.

## 11. Open questions

1. Final module export names.
2. Record versus individual words for host facts.
3. Whether environment enumeration is enabled by default.
4. Whether `chdir` is exposed at all.
5. Exact portable signal identity type.
6. Whether raw signal controls live under `boru:os` with capabilities or a separate host module.
