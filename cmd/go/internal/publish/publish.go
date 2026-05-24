// Package publish implements `aql publish [-r <url>] [dir]` — pack
// the current module into a zip, then upload it to the registry
// server using the locally-stored auth token from `aql login`.
package publish

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/aql-lang/aql/cmd/go/internal/auth"
	"github.com/aql-lang/aql/cmd/go/internal/command"
	"github.com/aql-lang/aql/cmd/go/internal/pack"
)

type cmd struct{}

// New returns the publish subcommand.
func New() command.Command { return &cmd{} }

func (*cmd) Name() string       { return "publish" }
func (*cmd) Synopsis() string   { return "upload the current module to an aql registry" }
func (*cmd) Mode() command.Mode { return command.ModeSinglePass }
func (*cmd) Run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	return Run(args, stdin, stdout, stderr)
}

// Run handles `aql publish [-r <url>] [dir]`.
func Run(args []string, _ io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("publish", flag.ContinueOnError)
	fs.SetOutput(stderr)
	registryURL := fs.String("r", "", "registry server URL")

	if err := fs.Parse(args); err != nil {
		return 1
	}

	dir := "."
	if fs.NArg() > 0 {
		dir = fs.Arg(0)
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}

	cu, err := auth.LoadClientUser(homeDir)
	if err != nil {
		fmt.Fprintf(stderr, "error: not logged in (run 'aql login' first)\n")
		return 1
	}
	if cu.Token == "" {
		fmt.Fprintf(stderr, "error: not logged in (run 'aql login' first)\n")
		return 1
	}

	regURL := *registryURL
	if regURL == "" {
		regURL = cu.Registry
	}
	if regURL == "" {
		regURL = "http://localhost:8080"
	}

	var packOut, packErr bytes.Buffer
	code := pack.Run([]string{dir}, &packOut, &packErr)
	if code != 0 {
		fmt.Fprintf(stderr, "error: pack failed: %s", packErr.String())
		return 1
	}

	zipPath := strings.TrimSpace(packOut.String())
	zipData, err := os.ReadFile(zipPath)
	if err != nil {
		fmt.Fprintf(stderr, "error: reading zip: %s\n", err)
		return 1
	}

	url := strings.TrimRight(regURL, "/") + "/api/publish"
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(zipData))
	if err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}
	req.Header.Set("Content-Type", "application/zip")
	req.Header.Set("Authorization", "Bearer "+cu.Token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusCreated {
		fmt.Fprintf(stderr, "error: %s\n", strings.TrimSpace(string(respBody)))
		return 1
	}

	var result map[string]string
	_ = json.Unmarshal(respBody, &result)
	fmt.Fprintf(stdout, "published %s@%s\n", result["module"], result["version"])
	return 0
}
