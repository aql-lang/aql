package eng

// W9 test seams (design/TEST-SEAMS.10.md). These are unexported package-level
// function variables that default to the real implementation; tests swap them
// (with t.Cleanup restore) to drive otherwise-unreachable init-error arms.
// They are NOT on any hot VM/dispatch path — builtinInitError is consulted
// once per NewRegistry, never per step.

// builtinInitError reports a malformed package-level builtin type table. In a
// valid build it is always nil; the seam lets a test exercise NewRegistry's
// ADR-005 init-error return without corrupting the shared builtin table.
var builtinInitError = BuiltinInitError
