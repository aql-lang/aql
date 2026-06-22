// Package model implements `aql model [flags] <model.jsonic>` — build a
// voxgig model: unify .jsonic source into a single JSON model (via aontu),
// write <base>/<name>.json, and optionally watch and rebuild on change.
//
// It is the command-line twin of the aql:model language module and mirrors
// the @voxgig/model CLI. Actions (Go generator callbacks) are not loadable
// from the command line, so this command performs the core job — resolving
// the model and writing the JSON — with optional watch.
package model

import (
	"encoding/json"
	"flag"
	stdfmt "fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"

	"github.com/aql-lang/aql/cmd/go/internal/command"
	"github.com/aql-lang/aql/cmd/go/internal/pathutil"
	model "github.com/voxgig/model/go"
)

type cmd struct{}

// New returns the model subcommand.
func New() command.Command { return &cmd{} }

func (*cmd) Name() string     { return "model" }
func (*cmd) Synopsis() string { return "build a voxgig model: unify .jsonic source into a JSON model" }

func (*cmd) Run(args []string, _ io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("model", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() {
		stdfmt.Fprintln(stderr, "usage: aql model [flags] <model.jsonic>")
		stdfmt.Fprintln(stderr, "\nUnify a .jsonic model and write <base>/<name>.json.\n\nFlags:")
		fs.PrintDefaults()
	}
	base := fs.String("base", "", "base directory for @\"...\" imports and JSON output (default: the model file's directory)")
	watch := fs.Bool("watch", false, "watch the source and rebuild on change (Ctrl-C to stop)")
	dryrun := fs.Bool("dryrun", false, "resolve the model but do not write the JSON output")
	printModel := fs.Bool("print", false, "print the unified model JSON to stdout instead of reporting the written file")
	config := fs.Bool("config", false, "resolve a .model-config build (off by default; needs a .model-config dir)")

	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	rest := fs.Args()
	if len(rest) != 1 {
		fs.Usage()
		return 2
	}

	path := pathutil.Expand(rest[0])
	// --print inspects the model without writing it, so it keeps the JSON
	// write off disk just like --dryrun.
	spec := model.ModelSpec{Path: path, Dryrun: *dryrun || *printModel}
	if *base != "" {
		spec.Base = pathutil.Expand(*base)
	}
	// Config is off by default: a .model-config build reads/creates files and
	// (as in voxgig/struct) often imports node-module paths that a pure Go
	// run cannot resolve. Opt in with --config.
	cfg := *config
	spec.Config = &cfg
	if *watch {
		// Surface rebuild activity on the watch goroutine.
		spec.Log = model.NewLog("info")
	}

	m := model.New(spec)

	report := func(br *model.BuildResult) int {
		if br == nil || !br.OK {
			if br != nil {
				for _, e := range br.Errs {
					stdfmt.Fprintf(stderr, "error: %s\n", e)
				}
			}
			return 1
		}
		if *printModel {
			data, err := json.MarshalIndent(m.Build().Model, "", "  ")
			if err != nil {
				stdfmt.Fprintf(stderr, "error: %s\n", err)
				return 1
			}
			stdfmt.Fprintln(stdout, string(data))
			return 0
		}
		out := outputPath(spec, path)
		if *dryrun {
			stdfmt.Fprintf(stdout, "ok (dryrun — %s not written)\n", out)
		} else {
			stdfmt.Fprintf(stdout, "wrote %s\n", out)
		}
		return 0
	}

	if *watch {
		if code := report(m.Start()); code != 0 {
			return code
		}
		stdfmt.Fprintln(stderr, "watching for changes (Ctrl-C to stop)…")
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, os.Interrupt)
		<-sig
		m.Stop()
		return 0
	}
	return report(m.Run())
}

// outputPath mirrors voxgig/model's ModelProducer: <base>/<name>.json, where
// base defaults to the model file's directory and name is the file's base
// name without extension.
func outputPath(spec model.ModelSpec, path string) string {
	base := spec.Base
	if base == "" {
		base = filepath.Dir(path)
	}
	name := filepath.Base(path)
	if ext := filepath.Ext(name); ext != "" {
		name = strings.TrimSuffix(name, ext)
	}
	return filepath.Join(base, name+".json")
}
