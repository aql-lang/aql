// Package login implements `boru login [-r <url>]` — prompt for
// username/password, POST /api/login on the registry server, save
// the returned token in ~/.boru/user.jsonic.
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

	"github.com/boru-lang/boru/cmd/go/internal/auth"
	"github.com/boru-lang/boru/cmd/go/internal/command"
	"github.com/boru-lang/boru/cmd/go/internal/vault"
)

type cmd struct{}

// New returns the login subcommand.
func New() command.Command { return &cmd{} }

func (*cmd) Name() string     { return "login" }
func (*cmd) Synopsis() string { return "log in to a boru registry" }
func (*cmd) Run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	return Run(args, stdin, stdout, stderr)
}

// Run handles `boru login [-r <url>]`.
func Run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("login", flag.ContinueOnError)
	fs.SetOutput(stderr)
	registryURL := fs.String("r", "http://localhost:8080", "registry server URL")
	useVault := fs.Bool("vault", false, "store the registry token in the boru vault instead of plaintext ~/.boru/user.jsonic")
	vaultAlias := fs.String("vault-alias", auth.DefaultRegistryTokenAlias, "vault alias for the token when --vault is set")

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
	if *useVault {
		// Store the token in the (encrypted) vault and keep only a
		// reference in user.jsonic. The vault passphrase comes from
		// BORU_VAULT_PASSPHRASE or an interactive prompt.
		if err := vault.WriteSecret(homeDir, *vaultAlias, cu.Token, "boru-login", stdin, stdout); err != nil {
			fmt.Fprintf(stderr, "error: storing token in vault: %s\n", err)
			return 1
		}
		cu.Token = ""
		cu.TokenVault = *vaultAlias
	}
	if err := auth.SaveClientUser(homeDir, cu); err != nil {
		fmt.Fprintf(stderr, "error: saving credentials: %s\n", err)
		return 1
	}

	if cu.TokenVault != "" {
		fmt.Fprintf(stdout, "logged in as %s (token stored in vault as %q)\n", cu.Username, cu.TokenVault)
	} else {
		fmt.Fprintf(stdout, "logged in as %s\n", cu.Username)
	}
	return 0
}
