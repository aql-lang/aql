// Package prep implements `boru prep [dir]` — parse boru.jsonic and
// write .boru/boru.json next to it.
package prep

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/boru-lang/boru/cmd/go/internal/command"
	"github.com/boru-lang/boru/cmd/go/internal/pathutil"
	"github.com/boru-lang/boru/eng/go/parser"
)

type cmd struct{}

// New returns the prep subcommand.
func New() command.Command { return &cmd{} }

func (*cmd) Name() string     { return "prep" }
func (*cmd) Synopsis() string { return "parse boru.jsonic and write .boru/boru.json" }
func (*cmd) Run(args []string, _ io.Reader, stdout, stderr io.Writer) int {
	return Run(args, stdout, stderr)
}

// Run is exported so the pack and install commands can re-run prep
// as part of their own flow.
func Run(args []string, stdout, stderr io.Writer) int {
	dir := "."
	if len(args) > 0 {
		// Expand a leading ~ the shell left verbatim (e.g. a quoted dir).
		dir = pathutil.Expand(args[0])
	}

	if _, err := Do(dir); err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}

	fmt.Fprintf(stdout, "%s\n", filepath.Join(dir, ".boru", "boru.json"))
	return 0
}

// Do parses boru.jsonic in dir and writes .boru/boru.json. It returns
// the parsed map for downstream use (pack reads the file list,
// install reads it for the version check).
func Do(dir string) (map[string]any, error) {
	src := filepath.Join(dir, "boru.jsonic")
	data, err := os.ReadFile(src)
	if err != nil {
		return nil, err
	}

	parsed, err := parser.SafeParse(string(data))
	if err != nil {
		return nil, fmt.Errorf("invalid jsonic: %w", err)
	}

	// boru.jsonic is order-agnostic project config; flatten the parser's
	// ordered object node to a plain map.
	m, ok := parser.Plainify(parsed).(map[string]any)
	if !ok {
		return nil, fmt.Errorf("boru.jsonic must be a map")
	}

	out, err := jsonMarshalIndent(m, "", "  ")
	if err != nil {
		return nil, err
	}

	dst := filepath.Join(dir, ".boru", "boru.json")
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return nil, err
	}

	if err := os.WriteFile(dst, append(out, '\n'), 0644); err != nil {
		return nil, err
	}

	return m, nil
}
