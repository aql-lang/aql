// Wave-4 coverage for the register subcommand: prompt failures,
// required-field validation, transport and server-rejection arms, and
// the Command wrapper methods.
package register

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestW4CommandMethods(t *testing.T) {
	c := New()
	if c.Name() != "register" {
		t.Errorf("Name() = %q, want register", c.Name())
	}
	if c.Synopsis() == "" {
		t.Error("Synopsis() is empty")
	}
}

func TestW4RegisterBadFlag(t *testing.T) {
	var out, errOut bytes.Buffer
	code := New().Run([]string{"-zz-bad"}, strings.NewReader(""), &out, &errOut)
	if code != 1 {
		t.Errorf("Run(bad flag) = %d, want 1", code)
	}
}

func TestW4RegisterPromptEOF(t *testing.T) {
	// No input at all: the first ReadLine fails.
	var out, errOut bytes.Buffer
	code := Run(nil, strings.NewReader(""), &out, &errOut)
	if code != 1 {
		t.Fatalf("Run = %d, want 1", code)
	}
	if !strings.Contains(errOut.String(), "no input") {
		t.Errorf("stderr = %q, want no-input error", errOut.String())
	}
}

func TestW4RegisterUsernameEOF(t *testing.T) {
	// Email supplied, then EOF at the username prompt.
	var out, errOut bytes.Buffer
	code := Run(nil, strings.NewReader("a@b.c\n"), &out, &errOut)
	if code != 1 {
		t.Fatalf("Run = %d, want 1", code)
	}
}

func TestW4RegisterPasswordEOF(t *testing.T) {
	var out, errOut bytes.Buffer
	code := Run(nil, strings.NewReader("a@b.c\nalice\n"), &out, &errOut)
	if code != 1 {
		t.Fatalf("Run = %d, want 1", code)
	}
}

func TestW4RegisterEmptyFields(t *testing.T) {
	// All three prompts answered with blank lines.
	var out, errOut bytes.Buffer
	code := Run(nil, strings.NewReader("\n\n\n"), &out, &errOut)
	if code != 1 {
		t.Fatalf("Run = %d, want 1", code)
	}
	if !strings.Contains(errOut.String(), "required") {
		t.Errorf("stderr = %q, want required-fields error", errOut.String())
	}
}

func TestW4RegisterConnectionError(t *testing.T) {
	var out, errOut bytes.Buffer
	code := Run([]string{"-r", "http://127.0.0.1:1"}, strings.NewReader("a@b.c\nalice\npw\n"), &out, &errOut)
	if code != 1 {
		t.Fatalf("Run = %d, want 1", code)
	}
	if !strings.Contains(errOut.String(), "error:") {
		t.Errorf("stderr = %q, want transport error", errOut.String())
	}
}

func TestW4RegisterServerRejects(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "username already taken", http.StatusConflict)
	}))
	t.Cleanup(srv.Close)

	var out, errOut bytes.Buffer
	code := Run([]string{"-r", srv.URL}, strings.NewReader("a@b.c\nalice\npw\n"), &out, &errOut)
	if code != 1 {
		t.Fatalf("Run = %d, want 1", code)
	}
	if !strings.Contains(errOut.String(), "username already taken") {
		t.Errorf("stderr = %q, want server rejection", errOut.String())
	}
}

func TestW4RegisterSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/register" {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(http.StatusCreated)
	}))
	t.Cleanup(srv.Close)

	var out, errOut bytes.Buffer
	code := New().Run([]string{"-r", srv.URL}, strings.NewReader("a@b.c\nalice\npw\n"), &out, &errOut)
	if code != 0 {
		t.Fatalf("Run = %d, want 0; stderr: %s", code, errOut.String())
	}
	if !strings.Contains(out.String(), "registered alice") {
		t.Errorf("stdout = %q, want registered alice", out.String())
	}
}
