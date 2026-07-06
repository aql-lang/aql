// Package build implements `aql build <prog.aql>` — compile an AQL program
// into a standalone native executable.
//
// Two mechanisms produce the binary:
//
//   - Self-embedding launcher (default). Copy the running `aql` binary and
//     append the program (plus any bundled file imports and the baked engine
//     settings) as a payload. At startup the copy detects the payload and runs
//     it through the full interpreter (see cmd/go/main.go). No Go toolchain or
//     module resolution is needed, and it runs any program — at the cost of a
//     binary the size of `aql`, for the host OS/arch only.
//
//   - Native (`--native`). Generate a tiny main.go that embeds the program and
//     calls buildrt.Main, then invoke `go build`. Smaller, cross-compilable,
//     but needs the Go toolchain and the aql module graph (see native.go).
//
// Both paths bundle file imports (`import "./lib.aql"`) so the produced binary
// is self-contained; built-in `aql:` modules are already in the runtime and
// need no bundling.
package build

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/aql-lang/aql/cmd/go/internal/buildrt"
	"github.com/aql-lang/aql/cmd/go/internal/command"
	"github.com/aql-lang/aql/cmd/go/internal/pathutil"
	lang "github.com/aql-lang/aql/lang/go"
)

type cmd struct{}

// New returns the build subcommand.
func New() command.Command { return &cmd{} }

func (*cmd) Name() string     { return "build" }
func (*cmd) Synopsis() string { return "compile a program into a standalone executable" }

func (*cmd) Run(args []string, _ io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("build", flag.ContinueOnError)
	fs.SetOutput(stderr)
	out := fs.String("o", "", "output binary path (default: source basename without .aql)")
	native := fs.Bool("native", false, "use the Go toolchain (go build) instead of the self-embedding launcher")
	keep := fs.Bool("keep", false, "(native) retain the generated temp dir and print its path")
	registry := fs.String("r", "", "registry path baked into the binary")
	var seed int64
	fs.Int64Var(&seed, "s", 0, "random seed baked into the binary")
	compileFlag := fs.Bool("compile", false, "EXPERIMENTAL: bake best-effort bytecode compilation into the binary")
	forceCompileFlag := fs.Bool("force-compile", false, "EXPERIMENTAL: bake REQUIRED bytecode compilation into the binary")
	optionsStr := fs.String("options", "", "engine options as jsonic, baked in (e.g. tape:initial:65536)")

	// flag.Parse stops at the first non-flag token, so flags placed after the
	// script (the natural `aql build prog.aql -o prog` form) would be missed.
	// Interleave: re-parse after each positional so flags work in any position.
	var positionals []string
	rest := args
	for len(rest) > 0 {
		if err := fs.Parse(rest); err != nil {
			return 1
		}
		if fs.NArg() == 0 {
			break
		}
		positionals = append(positionals, fs.Arg(0))
		rest = fs.Args()[1:]
	}

	if len(positionals) != 1 {
		fmt.Fprintf(stderr, "error: aql build requires exactly one <prog.aql>\n")
		return 1
	}
	srcPath := pathutil.Expand(positionals[0])

	// Validate --options eagerly so a typo fails at build time, not when the
	// produced binary runs.
	if *optionsStr != "" {
		var probe lang.Options
		m, err := lang.ParseOptions(*optionsStr)
		if err != nil {
			fmt.Fprintf(stderr, "error: %s\n", err)
			return 1
		}
		if err := lang.ApplyOptions(&probe, m); err != nil {
			fmt.Fprintf(stderr, "error: %s\n", err)
			return 1
		}
	}

	cfg, err := buildConfig(srcPath, *registry, seed, resolveCompile(*compileFlag, *forceCompileFlag), *optionsStr)
	if err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}

	outPath := *out
	if outPath == "" {
		outPath = defaultOutput(srcPath)
	}

	if *native {
		if err := buildNative(cfg, outPath, *keep, stdout, stderr); err != nil {
			fmt.Fprintf(stderr, "error: %s\n", err)
			return 1
		}
		fmt.Fprintf(stdout, "wrote %s\n", outPath)
		return 0
	}

	if err := buildSelfEmbed(cfg, outPath); err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "wrote %s\n", outPath)
	return 0
}

// resolveCompile maps the two flags to a CompileMode. force wins over try
// (mirrors run.ResolveCompileMode without the env-var rollout knobs, which do
// not apply to a frozen binary).
func resolveCompile(compile, force bool) buildrt.CompileMode {
	switch {
	case force:
		return buildrt.CompileForce
	case compile:
		return buildrt.CompileTry
	default:
		return buildrt.CompileOff
	}
}

// defaultOutput is the source basename without its extension, in the cwd:
// prog.aql -> prog, a/b/c.aql -> c.
func defaultOutput(srcPath string) string {
	name := filepath.Base(srcPath)
	if ext := filepath.Ext(name); ext != "" {
		name = strings.TrimSuffix(name, ext)
	}
	return name
}

// buildConfig reads the entry program, walks its file-import graph, and
// assembles the buildrt.Config baked into the binary. Built-in aql: imports
// are skipped (they are in the runtime); every reachable .aql file is embedded
// under its absolute path so the in-memory file system resolves it at run time.
func buildConfig(srcPath, registry string, seed int64, mode buildrt.CompileMode, optionsBlob string) (buildrt.Config, error) {
	entryAbs, err := filepath.Abs(srcPath)
	if err != nil {
		return buildrt.Config{}, err
	}
	src, err := os.ReadFile(entryAbs)
	if err != nil {
		return buildrt.Config{}, err
	}

	files := map[string][]byte{entryAbs: src}
	if err := collectImports(entryAbs, src, files); err != nil {
		return buildrt.Config{}, err
	}

	cfg := buildrt.Config{
		Source:      string(src),
		EntryDir:    filepath.Dir(entryAbs),
		Registry:    registry,
		Seed:        seed,
		Compile:     mode,
		OptionsBlob: optionsBlob,
	}
	// Only attach Files when there is more than the entry itself — a
	// single-file program needs no in-memory file system.
	if len(files) > 1 {
		cfg.Files = files
	}
	return cfg, nil
}

// osExecutable is a test seam (design/TEST-SEAMS.10.md); tests swap it to
// drive buildSelfEmbed's locate/read/nested-payload arms — os.Executable
// does not fail on a healthy host and always names a stock test binary.
var osExecutable = os.Executable

// encodePayload is a test seam (design/TEST-SEAMS.10.md); tests swap it to
// drive buildSelfEmbed's encode-failure arm, which is unreachable with
// the plain buildrt.Config shape.
var encodePayload = buildrt.EncodePayload

// buildSelfEmbed produces the standalone binary by copying the running aql
// executable and appending the encoded payload.
func buildSelfEmbed(cfg buildrt.Config, outPath string) error {
	self, err := osExecutable()
	if err != nil {
		return fmt.Errorf("locate self: %w", err)
	}
	image, err := os.ReadFile(self)
	if err != nil {
		return fmt.Errorf("read self: %w", err)
	}
	// Guard against building from an already-built binary, which would nest
	// payloads and run the wrong program.
	if _, ok, _ := buildrt.DecodePayload(image); ok {
		return fmt.Errorf("the running aql binary is itself a built executable; run `aql build` with a stock aql binary")
	}
	payload, err := encodePayload(cfg)
	if err != nil {
		return err
	}
	combined := append(image, payload...)
	if err := os.WriteFile(outPath, combined, 0o755); err != nil {
		return fmt.Errorf("write %s: %w", outPath, err)
	}
	return nil
}
