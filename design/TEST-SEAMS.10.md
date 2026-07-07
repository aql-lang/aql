# Test seams — the mocking conventions behind 100% unit coverage

Maintainer direction (2026-07): the entire Go codebase must admit
mocking seams, the way file I/O already does through the virtual
FileOps capability, and unit test coverage must be 100% at all times
(see ADR-008). This note is the implementation convention: which seam
shape to use where, and the rules that keep seams from becoming API
surface.

## The precedent: virtual FileOps

`lang/go/capabilities` defines a narrow `FileOps` interface with an
OS-backed implementation and an in-memory one (`NewMem`). Word handlers
resolve it through the capability registry, so every filesystem touch
in the language layer is swappable in tests without patching globals.
Every other hard-to-test edge gets the same treatment, scaled to its
size.

## Seam shapes, in order of preference

1. **Existing capability / interface** — if the subsystem already has
   an interface (FileOps, keyring providers, Clock), route new code
   through it rather than adding a second seam.

2. **Package-level function variable** — for a single OS/stdlib call
   whose failure or platform arm is otherwise unreachable:

   ```go
   // osExit is a test seam (design/TEST-SEAMS.10.md); tests swap it
   // to observe exit codes without killing the process.
   var osExit = os.Exit
   ```

   The var defaults to the real implementation and is **unexported**.
   Canonical instances: `osExit`, `readPassword` (TTY prompts),
   `goosName` (a `= runtime.GOOS` var read at dispatch time, so
   darwin/windows arms are drivable on linux), `randRead`,
   `jsonMarshal`/encoder seams where a marshal error arm exists,
   `newDefaultRegistry`-style constructor vars for init-error arms.

3. **Narrow interface** — when several related calls form a unit
   (a network client, a process runner), define a small interface next
   to the consumer with the real implementation as the default field
   value; tests supply a fake. Keep it minimal — methods the consumer
   actually calls, nothing speculative.

## Rules

- A seam is **not API**: unexported, placed next to its single
  consumer (or the package's `seams.go` when several files share it).
- The default value is always the real implementation; production
  code paths never check "is the seam set".
- Tests that swap a seam MUST restore it (`t.Cleanup`), and must not
  run in `t.Parallel()` with other tests of the same package that read
  the same seam.
- `main()` bodies follow the extract-run pattern:
  `func main() { osExit(run(os.Args[1:], os.Stdout, os.Stderr)) }` —
  `run` carries the logic, and `main` itself is covered by swapping
  `osExit`.
- A defensive branch that is provably unreachable through every seam
  (the payload seal makes the input unconstructible, a stdlib contract
  guarantees success) is restructured or removed rather than left
  uncovered — an untestable guard is dead code under ADR-005's
  errors-not-panics rule, since nothing can ever observe it.

## Enforcement

`make cover-gate` (test/go/covergate) computes the deduplicated
cross-package statement coverage of every Go module — each module's
tests run with `-coverpkg` spanning the whole repo, profiles are
merged, and every statement must be hit by at least one suite. The
gate fails below 100%. Run it before commit alongside the standard
`make fmt && make vet && make lint && make test`.
