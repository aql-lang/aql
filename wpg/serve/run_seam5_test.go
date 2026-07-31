package main

import (
	"errors"
	"net/http"
	"os"
	"testing"

	lang "github.com/boru-lang/boru/lang/go"
)

// run and main are covered through the seams (design/TEST-SEAMS.10.md):
// listenServe is stubbed so no socket is opened, newInstance to drive
// the constructor arm, osExit to observe main's exit code.
func TestRunSeams(t *testing.T) {
	prevListen, prevNew := listenServe, newInstance
	t.Cleanup(func() { listenServe, newInstance = prevListen, prevNew })

	// Happy path: the server "runs" and returns cleanly.
	var gotAddr string
	listenServe = func(addr string, h http.Handler) error {
		gotAddr = addr
		if h == nil {
			t.Error("listenServe got a nil handler")
		}
		return nil
	}
	if code := run([]string{"-port", "8123"}); code != 0 {
		t.Fatalf("run = %d, want 0", code)
	}
	if gotAddr != ":8123" {
		t.Errorf("addr = %q, want :8123", gotAddr)
	}

	// Serve failure surfaces as exit 1.
	listenServe = func(string, http.Handler) error { return errors.New("bind failed") }
	if code := run(nil); code != 1 {
		t.Errorf("run with failing serve = %d, want 1", code)
	}

	// Constructor failure surfaces as exit 1.
	newInstance = func(...lang.Options) (*lang.Boru, error) { return nil, errors.New("boom") }
	if code := run(nil); code != 1 {
		t.Errorf("run with failing constructor = %d, want 1", code)
	}
	newInstance = prevNew

	// Bad flag: usage error.
	if code := run([]string{"-definitely-not-a-flag"}); code != 2 {
		t.Errorf("run with bad flag = %d, want 2", code)
	}
}

func TestMainThroughSeams(t *testing.T) {
	prevExit, prevListen, prevArgs := osExit, listenServe, os.Args
	t.Cleanup(func() { osExit, listenServe, os.Args = prevExit, prevListen, prevArgs })

	os.Args = []string{"serve"}
	listenServe = func(string, http.Handler) error { return nil }
	var code = -1
	osExit = func(c int) { code = c }
	main()
	if code != 0 {
		t.Errorf("main exited %d, want 0", code)
	}
}
