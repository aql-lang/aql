// Package login implements `aql login [-r <url>]` — prompt for
// username/password, POST /api/login on the registry server, save
// the returned token in ~/.aql/user.jsonic.
package login

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
)

type cmd struct{}

// New returns the login subcommand.
func New() command.Command { return &cmd{} }

func (*cmd) Name() string       { return "login" }
func (*cmd) Synopsis() string   { return "log in to an aql registry" }
func (*cmd) Mode() command.Mode { return command.ModeSinglePass }
func (*cmd) Run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	return Run(args, stdin, stdout, stderr)
}

// Run handles `aql login [-r <url>]`.
func Run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("login", flag.ContinueOnError)
	fs.SetOutput(stderr)
	registryURL := fs.String("r", "http://localhost:8080", "registry server URL")

	if err := fs.Parse(args); err != nil {
		return 1
	}

	ir := auth.NewInputReader(stdin)
	username, err := ir.ReadLine("Username: ", stdout)
	if err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}
	password, err := ir.ReadPassword("Password: ", stdout)
	if err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}

	if username == "" || password == "" {
		fmt.Fprintf(stderr, "error: username and password are required\n")
		return 1
	}

	body, _ := json.Marshal(map[string]string{
		"username": username,
		"password": password,
	})

	resp, err := http.Post(
		strings.TrimRight(*registryURL, "/")+"/api/login",
		"application/json",
		bytes.NewReader(body),
	)
	if err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		fmt.Fprintf(stderr, "error: %s\n", strings.TrimSpace(string(respBody)))
		return 1
	}

	var result map[string]string
	if err := json.Unmarshal(respBody, &result); err != nil {
		fmt.Fprintf(stderr, "error: invalid response\n")
		return 1
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}

	cu := &auth.ClientUser{
		Username: result["username"],
		Email:    result["email"],
		Token:    result["token"],
		Registry: strings.TrimRight(*registryURL, "/"),
	}
	if err := auth.SaveClientUser(homeDir, cu); err != nil {
		fmt.Fprintf(stderr, "error: saving credentials: %s\n", err)
		return 1
	}

	fmt.Fprintf(stdout, "logged in as %s\n", cu.Username)
	return 0
}
