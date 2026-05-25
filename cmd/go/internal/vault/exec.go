package vault

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// runExec implements `aql vault exec`. It resolves the listed
// aliases against the keyring, injects their values into the
// environment of a child process, then execs the child with the
// caller's stdio attached. Secrets only ever appear in the child's
// environment block — never on the command line, never in the
// audit log.
//
// Usage:
//
//	aql vault exec [flags] <alias[=ENV][,alias[=ENV]]...> -- <cmd> [args...]
//
// By default an alias maps to an env var of the same name. Use
// `alias=ENV_NAME` to remap, `--upper` to uppercase the derived
// names, or `--prefix=PFX` to prepend a fixed prefix.
func runExec(args []string, homeDir string, stdin io.Reader, stdout, stderr io.Writer) int {
	// Split the args at the first `--` separator: everything before
	// is parsed by our flag set; everything after is the child
	// command and its arguments. We do this before flag.Parse so
	// flags like `--upper` inside the child command are left alone.
	preArgs, cmdArgs, sawSep := splitAtDoubleDash(args)

	fs := flag.NewFlagSet("vault exec", flag.ContinueOnError)
	fs.SetOutput(stderr)
	upper := fs.Bool("upper", false, "uppercase env-var names derived from alias names")
	clearEnv := fs.Bool("clear-env", false, "do not inherit the parent environment (keeps PATH/HOME/USER/SHELL/TERM/LANG/LC_ALL/TMPDIR only)")
	prefix := fs.String("prefix", "", "prepend this prefix to env-var names derived from aliases")
	if err := fs.Parse(preArgs); err != nil {
		return 1
	}

	if fs.NArg() < 1 {
		fmt.Fprintln(stderr, "error: usage: aql vault exec [--upper] [--prefix=PFX] [--clear-env] <alias[=ENV][,alias[=ENV]]...> -- <cmd> [args...]")
		return 1
	}
	if !sawSep || len(cmdArgs) == 0 {
		fmt.Fprintln(stderr, "error: missing command (separate aliases from the command with `--`)")
		return 1
	}

	mappings, err := parseExecAliases(fs.Arg(0), *prefix, *upper)
	if err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}

	s, err := requireStore(homeDir)
	if err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}
	if s.Locked {
		fmt.Fprintln(stderr, "error: vault is locked; run `aql vault unlock`")
		return 1
	}
	for _, m := range mappings {
		if a, _ := s.FindAlias(m.alias); a == nil {
			fmt.Fprintf(stderr, "error: no alias named %q\n", m.alias)
			return 1
		}
	}

	kr, err := openKeyring(s, homeDir, stdin, stdout, "Vault passphrase: ")
	if err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}

	overrides := make(map[string]string, len(mappings))
	for _, m := range mappings {
		if _, dup := overrides[m.envName]; dup {
			fmt.Fprintf(stderr, "error: duplicate env name %q in mapping\n", m.envName)
			return 1
		}
		v, err := kr.Get(m.alias)
		if err != nil {
			_ = appendAudit(homeDir, AuditEvent{
				Action: "vault.exec", Alias: m.alias,
				Outcome: "error", Reason: "keyring: " + err.Error(),
			})
			fmt.Fprintf(stderr, "error: reading %s: %s\n", m.alias, err)
			return 1
		}
		overrides[m.envName] = v
	}

	bin, err := exec.LookPath(cmdArgs[0])
	if err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}

	for _, m := range mappings {
		_ = appendAudit(homeDir, AuditEvent{
			Action: "vault.exec", Alias: m.alias,
			Outcome: "ok",
			Reason:  "env=" + m.envName + " cmd=" + filepath.Base(bin),
		})
	}

	env := buildExecEnv(*clearEnv, overrides)

	child := exec.Command(bin, cmdArgs[1:]...)
	child.Env = env
	child.Stdin = stdin
	child.Stdout = stdout
	child.Stderr = stderr
	if err := child.Run(); err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return ee.ExitCode()
		}
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}
	return 0
}

// execMapping pairs a vault alias with the env-var name the
// resolved value should be exposed under in the child process.
type execMapping struct {
	alias   string
	envName string
}

// parseExecAliases parses the comma-separated <alias[=ENV]...>
// specifier into a list of mappings, applying --prefix and --upper
// to any alias that did not carry an explicit env name.
func parseExecAliases(spec, prefix string, upper bool) ([]execMapping, error) {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return nil, fmt.Errorf("no aliases specified")
	}
	var out []execMapping
	seen := make(map[string]bool)
	for _, part := range strings.Split(spec, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		alias := part
		envName := ""
		if eq := strings.IndexByte(part, '='); eq >= 0 {
			alias = strings.TrimSpace(part[:eq])
			envName = strings.TrimSpace(part[eq+1:])
		}
		if !validAlias(alias) {
			return nil, fmt.Errorf("invalid alias %q", alias)
		}
		if envName == "" {
			envName = alias
			if upper {
				envName = strings.ToUpper(envName)
			}
			envName = prefix + envName
		}
		if !validEnvName(envName) {
			return nil, fmt.Errorf("invalid env name %q (must match [A-Za-z_][A-Za-z0-9_]*)", envName)
		}
		if seen[envName] {
			return nil, fmt.Errorf("duplicate env name %q in mapping", envName)
		}
		seen[envName] = true
		out = append(out, execMapping{alias: alias, envName: envName})
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no aliases specified")
	}
	return out, nil
}

// validEnvName accepts the POSIX-portable env-var name shape
// `[A-Za-z_][A-Za-z0-9_]*`. Stricter than the kernel allows, but
// matches what shells will round-trip cleanly.
func validEnvName(s string) bool {
	if s == "" {
		return false
	}
	for i, r := range s {
		switch {
		case r == '_':
		case r >= 'A' && r <= 'Z':
		case r >= 'a' && r <= 'z':
		case i > 0 && r >= '0' && r <= '9':
		default:
			return false
		}
	}
	return true
}

// buildExecEnv assembles the child's environment block: either the
// caller's environment (default) or a sanitized minimal one, then
// overlays the vault-sourced overrides on top, dropping any
// pre-existing entries that would collide.
func buildExecEnv(clearEnv bool, overrides map[string]string) []string {
	var base []string
	if clearEnv {
		// A near-empty env breaks most child commands: programs need
		// PATH to find their helpers, HOME for config, TERM for
		// curses, etc. We keep the unambiguously-safe non-secret
		// ambient vars and drop everything else.
		for _, k := range []string{"PATH", "HOME", "USER", "LOGNAME", "SHELL", "TERM", "LANG", "LC_ALL", "TMPDIR"} {
			if v, ok := os.LookupEnv(k); ok {
				base = append(base, k+"="+v)
			}
		}
	} else {
		base = os.Environ()
	}
	out := make([]string, 0, len(base)+len(overrides))
	for _, e := range base {
		eq := strings.IndexByte(e, '=')
		if eq >= 0 {
			if _, drop := overrides[e[:eq]]; drop {
				continue
			}
		}
		out = append(out, e)
	}
	for k, v := range overrides {
		out = append(out, k+"="+v)
	}
	return out
}

// splitAtDoubleDash returns args before the first standalone `--`
// and args after it. The boolean reports whether the separator was
// present at all, which lets the caller distinguish "no separator"
// from "separator with no following args".
func splitAtDoubleDash(args []string) (pre, post []string, sawSep bool) {
	for i, a := range args {
		if a == "--" {
			return args[:i], args[i+1:], true
		}
	}
	return args, nil, false
}
