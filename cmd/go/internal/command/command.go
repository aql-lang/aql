// Package command defines the Command interface that every aql
// subcommand satisfies and the Registry used by the top-level
// dispatcher to look one up by name.
//
// Two operational modes are explicit on every Command:
//
//   - ModeSinglePass: parse args, do one thing, exit (run, check,
//     fmt, prep, pack, clean, help, do, install, register, login,
//     publish).
//   - ModeServer: stay alive accepting requests (repl, registry, lsp).
//
// The dispatcher uses Mode only for grouping in help/usage output;
// it has no runtime effect.
package command

import "io"

// Mode classifies a Command as either single-pass (one-shot CLI
// invocation) or server (long-running input loop).
type Mode int

const (
	// ModeSinglePass indicates the command parses args, runs once,
	// and exits.
	ModeSinglePass Mode = iota
	// ModeServer indicates the command stays alive accepting input
	// (REPL on stdio, HTTP server, LSP server, etc.).
	ModeServer
)

// String returns "single-pass" or "server".
func (m Mode) String() string {
	if m == ModeServer {
		return "server"
	}
	return "single-pass"
}

// Command is the contract every aql subcommand implements. The
// top-level dispatcher resolves args[0] to a Command and calls Run
// with the remaining args.
//
// Run returns the process exit code: 0 for success, non-zero for
// failure. Implementations write progress/results to stdout, errors
// and diagnostics to stderr, and read interactive input from stdin.
type Command interface {
	Name() string
	Synopsis() string
	Mode() Mode
	Run(args []string, stdin io.Reader, stdout, stderr io.Writer) int
}

// Registry holds the registered Commands in insertion order so
// help/usage output is stable and matches the order subcommands are
// registered at startup.
type Registry struct {
	cmds  map[string]Command
	order []string
}

// New creates an empty Registry.
func New() *Registry {
	return &Registry{cmds: make(map[string]Command)}
}

// Register adds c to the registry. Re-registering a name overwrites
// the previous entry (intended for tests that swap implementations);
// production code registers each name once at startup.
func (r *Registry) Register(c Command) {
	name := c.Name()
	if _, exists := r.cmds[name]; !exists {
		r.order = append(r.order, name)
	}
	r.cmds[name] = c
}

// Lookup returns the Command for name and whether it was found.
func (r *Registry) Lookup(name string) (Command, bool) {
	c, ok := r.cmds[name]
	return c, ok
}

// Commands returns the registered Commands in registration order.
func (r *Registry) Commands() []Command {
	out := make([]Command, 0, len(r.order))
	for _, name := range r.order {
		out = append(out, r.cmds[name])
	}
	return out
}
