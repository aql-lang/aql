package exec

// exec_seam6e_test.go — seam-arm tests (design/TEST-SEAMS.10.md) for run's
// constructor-error arm, the handler's init-error arm, and Start's
// listener-torn-down error return.

import (
	"bytes"
	"context"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/aql-lang/aql/cmd/go/internal/service"
	lang "github.com/aql-lang/aql/lang/go"
	"github.com/aql-lang/aql/lang/go/policy"
)

func TestRunNewServerError(t *testing.T) {
	orig := newServer
	newServer = func(string, string, policy.Policy) (*Server, error) {
		return nil, errors.New("exec: constructor boom")
	}
	t.Cleanup(func() { newServer = orig })
	var stdout, stderr bytes.Buffer
	if code := run(nil, &stdout, &stderr); code != 1 {
		t.Fatalf("exit = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "constructor boom") {
		t.Errorf("stderr = %q, want constructor boom", stderr.String())
	}
}

func TestHandlerInitError(t *testing.T) {
	orig := langNew
	langNew = func(...lang.Options) (*lang.AQL, error) { return nil, errors.New("init boom") }
	t.Cleanup(func() { langNew = orig })
	srv := httptest.NewServer(Handler("", nil))
	defer srv.Close()
	var got execResponse
	status := post(t, srv, "/v1/exec", execRequest{Code: "1 add 2"}, &got)
	if status != 500 {
		t.Fatalf("status = %d, want 500", status)
	}
	if !strings.Contains(got.Error, "init error") {
		t.Errorf("error = %q, want init error", got.Error)
	}
}

func TestStartListenerTornDown(t *testing.T) {
	s, err := NewServer("127.0.0.1:0", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	errCh := make(chan error, 1)
	go func() { errCh <- s.Start(context.Background()) }()

	deadline := time.Now().Add(5 * time.Second)
	for s.Status() != service.StateRunning {
		if time.Now().After(deadline) {
			t.Fatal("server never reached Running")
		}
		time.Sleep(5 * time.Millisecond)
	}
	// Tear the listener down underneath Serve: the non-ErrServerClosed
	// error path (`return err`) fires.
	s.mu.Lock()
	ln := s.ln
	s.mu.Unlock()
	if err := ln.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-errCh:
		if err == nil {
			t.Fatal("Start returned nil, want listener error")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Start did not return after listener close")
	}
}
