// Package prep implements `aql prep [dir]` — parse aql.jsonic and
// write .aql/aql.json next to it.
package prep

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"

	jsonic "github.com/jsonicjs/jsonic/go"

	"github.com/aql-lang/aql/cmd/go/internal/command"
)

type cmd struct{}

// New returns the prep subcommand.
func New() command.Command { return &cmd{} }

func (*cmd) Name() string       { return "prep" }
func (*cmd) Synopsis() string   { return "parse aql.jsonic and write .aql/aql.json" }
func (*cmd) Mode() command.Mode { return command.ModeSinglePass }
func (*cmd) Run(args []string, _ io.Reader, stdout, stderr io.Writer) int {
	return Run(args, stdout, stderr)
}

// Run is exported so the pack and install commands can re-run prep
// as part of their own flow.
func Run(args []string, stdout, stderr io.Writer) int {
	dir := "."
	if len(args) > 0 {
		dir = args[0]
	}

	if _, err := Do(dir); err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}

	fmt.Fprintf(stdout, "%s\n", filepath.Join(dir, ".aql", "aql.json"))
	return 0
}

// Do parses aql.jsonic in dir and writes .aql/aql.json. It returns
// the parsed map for downstream use (pack reads the file list,
// install reads it for the version check).
func Do(dir string) (map[string]any, error) {
	src := filepath.Join(dir, "aql.jsonic")
	data, err := os.ReadFile(src)
	if err != nil {
		return nil, err
	}

	j := jsonic.Make()
	parsed, err := j.Parse(string(data))
	if err != nil {
		return nil, fmt.Errorf("invalid jsonic: %w", err)
	}

	m, ok := parsed.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("aql.jsonic must be a map")
	}

	out, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return nil, err
	}

	dst := filepath.Join(dir, ".aql", "aql.json")
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return nil, err
	}

	if err := os.WriteFile(dst, append(out, '\n'), 0644); err != nil {
		return nil, err
	}

	return m, nil
}
