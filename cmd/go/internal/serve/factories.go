// factories.go registers the Service factories that `aql serve`
// can compose. A factory parses one segment's flag tail and returns
// a ready-to-Start Service. Adding a new service means adding one
// entry here plus the package's own Service implementation.

package serve

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/aql-lang/aql/cmd/go/internal/api"
	"github.com/aql-lang/aql/cmd/go/internal/exec"
	"github.com/aql-lang/aql/cmd/go/internal/lsp"
	"github.com/aql-lang/aql/cmd/go/internal/permsflags"
	"github.com/aql-lang/aql/cmd/go/internal/registry"
	"github.com/aql-lang/aql/cmd/go/internal/repl"
	"github.com/aql-lang/aql/cmd/go/internal/service"
	"github.com/aql-lang/aql/cmd/go/internal/tui"
	"github.com/aql-lang/aql/cmd/go/internal/vault"
)

// Factory builds one Service from its flag tail. stdin/stdout/stderr
// are forwarded so stdio-bound services (repl, lsp without -p) wire
// up correctly when run as a single-service `aql serve` invocation.
type Factory func(args []string, stdin io.Reader, stdout, stderr io.Writer) (service.Service, error)

// factories is the static name → Factory map. Order matches the
// listing in the serve usage output.
var factories = map[string]Factory{
	"repl":        replFactory,
	"registry":    registryFactory,
	"lsp":         lspFactory,
	"api":         apiFactory,
	"exec":        execFactory,
	"tui":         tuiFactory,
	"vault-proxy": vaultProxyFactory,
}

// factoryOrder is the display order for help/usage output.
var factoryOrder = []string{"repl", "registry", "lsp", "api", "exec", "tui", "vault-proxy"}

func replFactory(args []string, stdin io.Reader, stdout, _ io.Writer) (service.Service, error) {
	fs := flag.NewFlagSet("repl", flag.ContinueOnError)
	registryPath := fs.String("r", "", "registry path passed to the UniversalManager")
	if err := fs.Parse(args); err != nil {
		return nil, err
	}
	return repl.NewServer(stdin, stdout, *registryPath), nil
}

func registryFactory(args []string, _ io.Reader, _, stderr io.Writer) (service.Service, error) {
	fs := flag.NewFlagSet("registry", flag.ContinueOnError)
	fs.SetOutput(stderr)
	dir := fs.String("r", "", "registry folder containing zip files")
	port := fs.Int("p", 8080, "port to listen on")
	if err := fs.Parse(args); err != nil {
		return nil, err
	}
	if *dir == "" {
		return nil, fmt.Errorf("-r <folder> is required")
	}
	srv, err := registry.NewServer(*dir, *port)
	if err != nil {
		// NewServer prefixes its errors with "registry: "; strip it
		// since buildServices already adds the segment name.
		return nil, fmt.Errorf("%s", strings.TrimPrefix(err.Error(), "registry: "))
	}
	return srv, nil
}

func lspFactory(args []string, stdin io.Reader, stdout, stderr io.Writer) (service.Service, error) {
	fs := flag.NewFlagSet("lsp", flag.ContinueOnError)
	fs.SetOutput(stderr)
	port := fs.Int("p", 0, "TCP port to listen on (0 = stdio mode)")
	if err := fs.Parse(args); err != nil {
		return nil, err
	}
	if *port == 0 {
		return lsp.NewStdioServer(stdin, stdout, stderr), nil
	}
	return lsp.NewTCPServer(*port, stderr), nil
}

func execFactory(args []string, _ io.Reader, _, stderr io.Writer) (service.Service, error) {
	fs := flag.NewFlagSet("exec", flag.ContinueOnError)
	fs.SetOutput(stderr)
	bind := fs.String("bind", "127.0.0.1:8091", "host:port to bind the exec HTTP server")
	port := fs.Int("p", 0, "port to listen on (overrides -bind host:port if >0)")
	registryPath := fs.String("r", "", "registry path passed to AQL instances")
	var pf permsflags.Flags
	permsflags.Register(fs, &pf)
	if err := fs.Parse(args); err != nil {
		return nil, err
	}
	pol, err := pf.Resolve()
	if err != nil {
		return nil, err
	}
	addr := *bind
	if *port > 0 {
		addr = fmt.Sprintf(":%d", *port)
	}
	srv, err := exec.NewServer(addr, *registryPath, pol)
	if err != nil {
		return nil, fmt.Errorf("%s", strings.TrimPrefix(err.Error(), "exec: "))
	}
	return srv, nil
}

func apiFactory(args []string, _ io.Reader, _, stderr io.Writer) (service.Service, error) {
	fs := flag.NewFlagSet("api", flag.ContinueOnError)
	fs.SetOutput(stderr)
	bind := fs.String("bind", "127.0.0.1:8090", "host:port to bind the api HTTP server")
	token := fs.String("token", "", "require a Bearer token of this value")
	if err := fs.Parse(args); err != nil {
		return nil, err
	}
	return api.NewServer(*bind, *token, stderr), nil
}

func vaultProxyFactory(args []string, _ io.Reader, _, stderr io.Writer) (service.Service, error) {
	fs := flag.NewFlagSet("vault-proxy", flag.ContinueOnError)
	fs.SetOutput(stderr)
	listen := fs.String("listen", "127.0.0.1:8787", "address to listen on (loopback recommended)")
	home := fs.String("home", "", "vault home directory (default: $AQL_HOME or $HOME)")
	if err := fs.Parse(args); err != nil {
		return nil, err
	}
	svc, err := vault.NewProxyService(*listen, *home, os.Getenv(vault.EnvPassphrase))
	if err != nil {
		// NewProxyService prefixes errors with "vault-proxy: " for
		// standalone use; strip it since buildServices re-adds the
		// segment name.
		return nil, fmt.Errorf("%s", strings.TrimPrefix(err.Error(), "vault-proxy: "))
	}
	return svc, nil
}

func tuiFactory(args []string, stdin io.Reader, stdout, stderr io.Writer) (service.Service, error) {
	fs := flag.NewFlagSet("tui", flag.ContinueOnError)
	fs.SetOutput(stderr)
	apiURL := fs.String("api", "", "api base URL (default: read from discovery file)")
	token := fs.String("token", "", "bearer token (default: read from discovery file)")
	if err := fs.Parse(args); err != nil {
		return nil, err
	}
	return tui.NewServer(*apiURL, *token, stdin, stdout, stderr), nil
}
