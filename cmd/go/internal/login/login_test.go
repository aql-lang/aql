package login

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/aql-lang/aql/cmd/go/internal/auth"
	"github.com/aql-lang/aql/cmd/go/internal/registry"
)

func TestRunLoginCLI(t *testing.T) {
	regDir := t.TempDir()
	srv := httptest.NewServer(registry.Handler(regDir))
	defer srv.Close()

	regBody := `{"email":"cli@example.com","username":"cliuser","password":"clipass"}`
	resp, _ := http.Post(srv.URL+"/api/register", "application/json", strings.NewReader(regBody))
	resp.Body.Close()

	homeDir := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", homeDir)
	defer os.Setenv("HOME", origHome)

	stdin := strings.NewReader("cliuser\nclipass\n")
	var stdout, stderr bytes.Buffer

	code := Run([]string{"-r", srv.URL}, stdin, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("Run failed: %s", stderr.String())
	}
	if !strings.Contains(stdout.String(), "logged in as cliuser") {
		t.Errorf("unexpected output: %q", stdout.String())
	}

	cu, err := auth.LoadClientUser(homeDir)
	if err != nil {
		t.Fatal(err)
	}
	if cu.Username != "cliuser" {
		t.Errorf("username = %q, want cliuser", cu.Username)
	}
	if cu.Token == "" {
		t.Error("token is empty")
	}
}
