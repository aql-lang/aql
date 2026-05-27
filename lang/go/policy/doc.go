// Package policy implements AQL's capability-scoped permissions model.
//
// A Policy is a compiled, evaluable permission profile: a set of
// scopes (engine, modules, fileops, network, sqlite, formats, env,
// process, clock, and the special "global" hard-cap scope), each
// carrying a default decision and an ordered list of rules. The
// policy is consulted by:
//
//   - the engine dispatch loop, before invoking a kernel word;
//   - the module resolver, before loading a module;
//   - the module-export dispatch path, before invoking an export;
//   - each permissioned capability wrapper, before delegating to
//     the underlying I/O (FileOps, FormatRegistry, SQLiteStore, …).
//
// The package has no dependencies on eng or native — those packages
// depend on policy, never the reverse. A small WordChecker interface
// is exported for the engine's dispatch hook so that eng/ doesn't
// need to import the full Policy interface.
//
// Defaults are allow-everything: a *lang.AQL with no policy runs
// without any check, and any absent scope in a policy is treated as
// allow-all. Permissions are opt-in; existing callers are
// unaffected. See lang/doc/design/PERMISSIONS.0.md for the full
// design document.
package policy
