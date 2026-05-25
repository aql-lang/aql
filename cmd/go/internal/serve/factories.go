// factories.go registers the Service factories that `aql serve`
// can compose. A factory parses one segment's flag tail and returns
// a ready-to-Start Service. Adding a new service means adding one
// entry here plus the package's own Service implementation.

package serve

import (
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/aql-lang/aql/cmd/go/internal/lsp"
	"github.com/aql-lang/aql/cmd/go/internal/registry"
	"github.com/aql-lang/aql/cmd/go/internal/repl"
	"github.com/aql-lang/aql/cmd/go/internal/service"
)

// Factory builds one Service from its flag tail. stdin/stdout/stderr
// are forwarded so stdio-bound services (repl, lsp without -p) wire
// up correctly when run as a single-service `aql serve` invocation.
type Factory func(args []string, stdin io.Reader, stdout, stderr io.Writer) (service.Service, error)

// factories is the static name → Factory map. Order matches the
// listing in the serve usage output.
var factories = map[string]Factory{
	"repl":     replFactory,
	"registry": registryFactory,
	"lsp":      lspFactory,
}

// factoryOrder is the display order for help/usage output.
var factoryOrder = []string{"repl", "registry", "lsp"}

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
