// Package pack implements `boru pack [dir]` — run prep, then zip up
// the files listed in boru.jsonic (plus boru.jsonic itself) into
// .boru/_pack/<name>-<version>.zip.
package pack

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/boru-lang/boru/cmd/go/internal/command"
	"github.com/boru-lang/boru/cmd/go/internal/pathutil"
	"github.com/boru-lang/boru/cmd/go/internal/prep"
)

type cmd struct{}

// New returns the pack subcommand.
func New() command.Command { return &cmd{} }

func (*cmd) Name() string     { return "pack" }
func (*cmd) Synopsis() string { return "build a publishable module zip" }
func (*cmd) Run(args []string, _ io.Reader, stdout, stderr io.Writer) int {
	return Run(args, stdout, stderr)
}

// Run is exported so publish can build the zip without re-implementing it.
func Run(args []string, stdout, stderr io.Writer) int {
	dir := "."
	if len(args) > 0 {
		// Expand a leading ~ the shell left verbatim (e.g. a quoted dir).
		dir = pathutil.Expand(args[0])
	}

	m, err := prep.Do(dir)
	if err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}

	name, _ := m["name"].(string)
	if name == "" {
		fmt.Fprintf(stderr, "error: boru.jsonic missing name\n")
		return 1
	}

	major, _ := m["major"].(float64)
	minor, _ := m["minor"].(float64)
	patch, _ := m["patch"].(float64)
	version := fmt.Sprintf("%d.%d.%d", int(major), int(minor), int(patch))

	rawFiles, ok := m["files"].([]any)
	if !ok {
		fmt.Fprintf(stderr, "error: boru.jsonic missing files list\n")
		return 1
	}

	files := []string{"boru.jsonic"}
	for _, f := range rawFiles {
		if s, ok := f.(string); ok {
			files = append(files, s)
		}
	}

	zipName := fmt.Sprintf("%s-%s.zip", name, version)
	packDir := filepath.Join(dir, ".boru", "_pack")
	if err := os.MkdirAll(packDir, 0755); err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}
	zipPath := filepath.Join(packDir, zipName)

	zf, err := os.Create(zipPath)
	if err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}
	defer zf.Close()

	zw := zip.NewWriter(zf)
	defer zw.Close()

	for _, f := range files {
		data, err := os.ReadFile(filepath.Join(dir, f))
		if err != nil {
			fmt.Fprintf(stderr, "error: %s\n", err)
			return 1
		}
		w, err := zipCreate(zw, f)
		if err != nil {
			fmt.Fprintf(stderr, "error: %s\n", err)
			return 1
		}
		if _, err := zipWriteEntry(w, data); err != nil {
			fmt.Fprintf(stderr, "error: %s\n", err)
			return 1
		}
	}

	fmt.Fprintf(stdout, "%s\n", zipPath)
	return 0
}
