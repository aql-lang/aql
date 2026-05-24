// Package register implements `aql register [-r <url>]` — prompt
// for email/username/password, POST /api/register on the registry
// server.
package register

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/aql-lang/aql/cmd/go/internal/auth"
	"github.com/aql-lang/aql/cmd/go/internal/command"
)

type cmd struct{}

// New returns the register subcommand.
func New() command.Command { return &cmd{} }

func (*cmd) Name() string       { return "register" }
func (*cmd) Synopsis() string   { return "create an account on an aql registry" }
func (*cmd) Mode() command.Mode { return command.ModeSinglePass }
func (*cmd) Run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	return Run(args, stdin, stdout, stderr)
}

// Run handles `aql register [-r <url>]`.
func Run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("register", flag.ContinueOnError)
	fs.SetOutput(stderr)
	registryURL := fs.String("r", "http://localhost:8080", "registry server URL")

	if err := fs.Parse(args); err != nil {
		return 1
	}

	ir := auth.NewInputReader(stdin)
	email, err := ir.ReadLine("Email: ", stdout)
	if err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}
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

	if email == "" || username == "" || password == "" {
		fmt.Fprintf(stderr, "error: email, username, and password are required\n")
		return 1
	}

	body, _ := json.Marshal(map[string]string{
		"email":    email,
		"username": username,
		"password": password,
	})

	resp, err := http.Post(
		strings.TrimRight(*registryURL, "/")+"/api/register",
		"application/json",
		bytes.NewReader(body),
	)
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

	fmt.Fprintf(stdout, "registered %s\n", username)
	return 0
}
