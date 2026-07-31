// Package boru is the top-level dispatcher for the boru command-line
// tool. It owns the Version constant (rewritten by `make publish`)
// and one short execute() function that routes args[0] to the
// matching subcommand package under internal/. Everything else
// lives in its own package.
package boru

import (
	"fmt"
	"io"
	"os"
	"runtime/debug"

	"github.com/boru-lang/boru/cmd/go/internal/api"
	"github.com/boru-lang/boru/cmd/go/internal/attach"
	"github.com/boru-lang/boru/cmd/go/internal/build"
	"github.com/boru-lang/boru/cmd/go/internal/buildrt"
	"github.com/boru-lang/boru/cmd/go/internal/check"
	"github.com/boru-lang/boru/cmd/go/internal/clean"
	"github.com/boru-lang/boru/cmd/go/internal/command"
	"github.com/boru-lang/boru/cmd/go/internal/ctl"
	"github.com/boru-lang/boru/cmd/go/internal/debugcmd"
	"github.com/boru-lang/boru/cmd/go/internal/describe"
	"github.com/boru-lang/boru/cmd/go/internal/do"
	"github.com/boru-lang/boru/cmd/go/internal/exec"
	borufmt "github.com/boru-lang/boru/cmd/go/internal/fmt"
	"github.com/boru-lang/boru/cmd/go/internal/help"
	"github.com/boru-lang/boru/cmd/go/internal/install"
	"github.com/boru-lang/boru/cmd/go/internal/login"
	"github.com/boru-lang/boru/cmd/go/internal/lsp"
	"github.com/boru-lang/boru/cmd/go/internal/model"
	"github.com/boru-lang/boru/cmd/go/internal/pack"
	"github.com/boru-lang/boru/cmd/go/internal/policy"
	"github.com/boru-lang/boru/cmd/go/internal/prep"
	"github.com/boru-lang/boru/cmd/go/internal/publish"
	"github.com/boru-lang/boru/cmd/go/internal/register"
	"github.com/boru-lang/boru/cmd/go/internal/registry"
	"github.com/boru-lang/boru/cmd/go/internal/repl"
	"github.com/boru-lang/boru/cmd/go/internal/run"
	"github.com/boru-lang/boru/cmd/go/internal/serve"
	testcmd "github.com/boru-lang/boru/cmd/go/internal/test"
	"github.com/boru-lang/boru/cmd/go/internal/tui"
	"github.com/boru-lang/boru/cmd/go/internal/vault"
)

// Version is the boru CLI version. It is rewritten by the publish
// target before tagging, and may also be overridden at build time
// with `-ldflags "-X github.com/boru-lang/boru/cmd/go.Version=x.y.z"`.
var Version = "0.1.0-dev"

// osExit is a test seam (design/TEST-SEAMS.10.md); tests swap it to
// observe exit codes without killing the process.
var osExit = os.Exit

// readBuildInfo is a test seam (design/TEST-SEAMS.10.md); tests swap it
// to drive versionString's VCS-stamp arms, which the test binary's own
// build info cannot reach deterministically.
var readBuildInfo = debug.ReadBuildInfo

// osExecutable is a test seam (design/TEST-SEAMS.10.md); tests swap it
// to drive runEmbedded's self-path error arm.
var osExecutable = os.Executable

// readEmbeddedPayload is a test seam (design/TEST-SEAMS.10.md); tests
// swap it to drive runEmbedded's corrupt-payload and embedded-program
// arms without building a payload-carrying executable.
var readEmbeddedPayload = buildrt.ReadEmbeddedPayload

// buildrtMain is a test seam (design/TEST-SEAMS.10.md); tests swap it
// to observe the embedded-program dispatch without executing a program.
var buildrtMain = buildrt.Main

// versionString returns the CLI version, augmenting the dev default
// with the VCS stamp the Go toolchain already embeds (decision DX
// report finding 8): a from-source build reports
// `0.1.0-dev (git 958c379b1229, dirty)` instead of an
// unidentifiable `0.1.0-dev`, so consumers pinning a commit can tell
// which build is on PATH. Published builds (ldflags-stamped Version)
// pass through untouched.
func versionString() string {
	if Version != "0.1.0-dev" {
		return Version
	}
	bi, ok := readBuildInfo()
	if !ok {
		return Version
	}
	var rev, modified string
	for _, s := range bi.Settings {
		switch s.Key {
		case "vcs.revision":
			rev = s.Value
		case "vcs.modified":
			modified = s.Value
		}
	}
	if rev == "" {
		return Version
	}
	if len(rev) > 12 {
		rev = rev[:12]
	}
	if modified == "true" {
		return Version + " (git " + rev + ", dirty)"
	}
	return Version + " (git " + rev + ")"
}

// Run is the binary entrypoint. The thin main package at
// cmd/go/boru calls this so the installed binary is named `boru`
// rather than `go`.
func Run() {
	// A binary produced by `boru build` (self-embedding launcher) carries an
	// appended payload describing the program to run. When present, run it and
	// exit before any CLI dispatch — this executable IS the program, not the
	// boru CLI.
	if code, embedded := runEmbedded(); embedded {
		osExit(code)
	}

	// Publish the version (with VCS stamp for dev builds) to the api
	// package so the /v1/server endpoint can report it without an
	// import cycle.
	api.SetSupervisorVersion(versionString())
	osExit(execute(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

// runEmbedded checks whether this executable has a program payload appended by
// `boru build`. If so it runs that program and returns its exit code with
// embedded=true; otherwise it returns embedded=false and the normal CLI runs.
// A self-read or decode failure is reported to stderr and treated as embedded
// (exit non-zero) so a corrupt built binary fails loudly rather than silently
// behaving as the CLI.
func runEmbedded() (code int, embedded bool) {
	exe, err := osExecutable()
	if err != nil {
		return 0, false
	}
	cfg, ok, err := readEmbeddedPayload(exe)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %s\n", err)
		return 1, true
	}
	if !ok {
		return 0, false
	}
	return buildrtMain(cfg, os.Args[1:], os.Stdin, os.Stdout, os.Stderr), true
}

// execute resolves args[0] to a Command and runs it. If args[0]
// is not a registered subcommand (or args is empty), the call
// falls through to the run subcommand, which owns the legacy
// `boru [-e expr] [script.boru]` shape and the no-args REPL drop-in.
func execute(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	run.SetVersion(versionString())
	vault.SetVersion(versionString())
	reg := buildRegistry()

	if len(args) > 0 {
		if c, ok := reg.Lookup(args[0]); ok {
			return c.Run(args[1:], stdin, stdout, stderr)
		}
	}

	// Legacy fallthrough: boru, boru -e EXPR, boru script.boru, boru -version.
	fallback, _ := reg.Lookup("run")
	return fallback.Run(args, stdin, stdout, stderr)
}

// buildRegistry registers every subcommand. The order here drives
// the Usage listing — one-shot Commands first (grouped by purpose),
// then long-running Services at the end.
func buildRegistry() *command.Registry {
	r := command.New()
	// provide hands the help command the live registry + service
	// classification at Run time. r is captured by reference and fully
	// populated before the closure is ever called, so the cycle that
	// would arise from importing the top-level package is avoided.
	provide := func() (*command.Registry, map[string]bool) { return r, serviceNames }
	// Commands: language execution.
	r.Register(run.New())
	r.Register(do.New())
	r.Register(check.New())
	r.Register(testcmd.New())
	r.Register(help.New(provide))
	r.Register(describe.New())
	r.Register(borufmt.New())
	r.Register(build.New())
	// Commands: model generation.
	r.Register(model.New())
	// Commands: project lifecycle.
	r.Register(prep.New())
	r.Register(pack.New())
	r.Register(clean.New())
	// Commands: registry client.
	r.Register(install.New())
	r.Register(register.New())
	r.Register(login.New())
	r.Register(publish.New())
	// Commands: local secret management.
	r.Register(vault.New())
	// Commands: permission profiles.
	r.Register(policy.New())
	// Commands: cross-process debugging (serve introspection / attach).
	r.Register(debugcmd.New())
	// Commands: supervisor control plane client.
	r.Register(attach.New())
	r.Register(ctl.New())
	// Services: long-running input loops.
	r.Register(repl.New())
	r.Register(registry.New())
	r.Register(lsp.New())
	r.Register(exec.New())
	r.Register(serve.New())
	r.Register(tui.New())
	return r
}

// serviceNames is the set of Commands that are also long-running
// services (composable under `boru serve`). Used by the help command to
// group them separately from one-shot commands.
var serviceNames = map[string]bool{
	"repl":     true,
	"registry": true,
	"lsp":      true,
	"exec":     true,
	"serve":    true,
	"tui":      true,
}
