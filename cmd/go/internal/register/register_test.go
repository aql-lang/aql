package register

import (
	"bytes"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/aql-lang/aql/cmd/go/internal/registry"
)

func TestRunRegisterCLI(t *testing.T) {
	regDir := t.TempDir()
	srv := httptest.NewServer(registry.Handler(regDir))
	defer srv.Close()

	stdin := strings.NewReader("test@example.com\ntestuser\ntestpass\n")
	var stdout, stderr bytes.Buffer

	code := Run([]string{"-r", srv.URL}, stdin, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("Run failed: %s", stderr.String())
	}
	if !strings.Contains(stdout.String(), "registered testuser") {
		t.Errorf("unexpected output: %q", stdout.String())
	}
}
