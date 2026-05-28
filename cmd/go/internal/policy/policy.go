// Package policy implements the `aql policy` subcommand: a thin
// CLI surface for listing built-in profiles, validating user
// profiles, and explaining permission decisions.
//
// Subcommands:
//
//	aql policy list
//	aql policy show <name|path>
//	aql policy validate <path>
//	aql policy test <name|path> <scope>.<op> [arg=value...]
//	aql policy explain <name|path> <scope>.<op> [arg=value...]
package policy

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/aql-lang/aql/cmd/go/internal/command"
	pol "github.com/aql-lang/aql/lang/go/policy"
)

type cmd struct{}

// New returns the policy subcommand.
func New() command.Command { return &cmd{} }

func (*cmd) Name() string     { return "policy" }
func (*cmd) Synopsis() string { return "manage and inspect permission profiles" }

func (*cmd) Run(args []string, _ io.Reader, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		printUsage(stderr)
		return 1
	}
	sub, rest := args[0], args[1:]
	switch sub {
	case "list":
		return listProfiles(stdout)
	case "show":
		return showProfile(stdout, stderr, rest)
	case "validate":
		return validateProfile(stdout, stderr, rest)
	case "test":
		return testCheck(stdout, stderr, rest, false)
	case "explain":
		return testCheck(stdout, stderr, rest, true)
	case "help", "-h", "--help":
		printUsage(stdout)
		return 0
	}
	fmt.Fprintf(stderr, "unknown subcommand: %s\n", sub)
	printUsage(stderr)
	return 1
}

func printUsage(w io.Writer) {
	fmt.Fprintln(w, "Usage: aql policy <subcommand> [args...]")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Subcommands:")
	fmt.Fprintln(w, "  list                          list built-in profiles")
	fmt.Fprintln(w, "  show <name|path>              pretty-print resolved profile")
	fmt.Fprintln(w, "  validate <path>               check profile syntax + semantics")
	fmt.Fprintln(w, "  test <name|path> <scope.op>   exit 0 if allowed, 1 if denied")
	fmt.Fprintln(w, "  explain <name|path> <scope.op>  print blame chain for the decision")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Args are key=value pairs after the scope.op token, e.g.")
	fmt.Fprintln(w, "  aql policy explain sandbox fileops.write path=/etc/passwd")
}

// listProfiles writes the names of every built-in profile, in
// trust-tier order (most permissive first).
func listProfiles(w io.Writer) int {
	for _, name := range pol.BuiltinNames() {
		fmt.Fprintln(w, name)
	}
	return 0
}

// showProfile loads name and pretty-prints the resolved Compiled
// shape as JSON.
func showProfile(stdout, stderr io.Writer, args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "show: profile name or path required")
		return 1
	}
	p, err := pol.LoadAuto(args[0])
	if err != nil {
		fmt.Fprintf(stderr, "show: %s\n", err)
		return 1
	}
	// Compiled doesn't expose its internals directly, but Scope(name)
	// gives us each scope. Build a JSON-printable view.
	view := map[string]any{
		"name":   p.Name(),
		"limits": p.Limits(),
		"scopes": map[string]any{},
	}
	scopes := view["scopes"].(map[string]any)
	for _, name := range pol.KnownScopes {
		s := p.Scope(name)
		scopes[name] = s
	}
	enc := json.NewEncoder(stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(view); err != nil {
		fmt.Fprintf(stderr, "show: encode: %s\n", err)
		return 1
	}
	return 0
}

// validateProfile loads a profile from disk (path required, not a
// name) and reports the first error or success.
func validateProfile(stdout, stderr io.Writer, args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "validate: file path required")
		return 1
	}
	_, err := pol.LoadFile(args[0])
	if err != nil {
		fmt.Fprintf(stderr, "%s: %s\n", args[0], err)
		return 1
	}
	fmt.Fprintf(stdout, "%s: ok\n", args[0])
	return 0
}

// testCheck loads name and runs a single Check against scope.op
// with any supplied args. When verbose is true, prints the blame
// chain; otherwise just sets the exit code.
func testCheck(stdout, stderr io.Writer, args []string, verbose bool) int {
	if len(args) < 2 {
		fmt.Fprintln(stderr, "test/explain: usage: <name|path> <scope.op> [k=v...]")
		return 1
	}
	p, err := pol.LoadAuto(args[0])
	if err != nil {
		fmt.Fprintf(stderr, "%s\n", err)
		return 1
	}
	scope, op, err := splitScopeOp(args[1])
	if err != nil {
		fmt.Fprintf(stderr, "%s\n", err)
		return 1
	}
	checkArgs := pol.Args{}
	for _, kv := range args[2:] {
		parts := strings.SplitN(kv, "=", 2)
		if len(parts) != 2 {
			fmt.Fprintf(stderr, "bad arg %q (expected k=v)\n", kv)
			return 1
		}
		checkArgs[parts[0]] = parts[1]
	}
	err = p.Check(scope, op, checkArgs)
	if verbose {
		fmt.Fprintf(stdout, "profile:  %s\n", p.Name())
		fmt.Fprintf(stdout, "scope:    %s\n", scope)
		fmt.Fprintf(stdout, "op:       %s\n", op)
		fmt.Fprintf(stdout, "args:     %v\n", checkArgs)
		if err == nil {
			fmt.Fprintln(stdout, "decision: ALLOW")
			return 0
		}
		fmt.Fprintln(stdout, "decision: DENY")
		if d, ok := err.(*pol.Denied); ok {
			fmt.Fprintf(stdout, "code:     %s\n", d.Code)
			fmt.Fprintf(stdout, "blame:    %s\n", d.Blame)
		} else {
			fmt.Fprintf(stdout, "reason:   %s\n", err)
		}
		return 1
	}
	if err != nil {
		return 1
	}
	return 0
}

func splitScopeOp(raw string) (scope, op string, err error) {
	parts := strings.SplitN(raw, ".", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("expected scope.op form, got %q", raw)
	}
	return parts[0], parts[1], nil
}
