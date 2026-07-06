package ctl

// ctl_seam6e_test.go — covers the http.NewRequest URL-parse failure arms
// of getJSON and doAction (a control character in the --api URL).

import (
	"bytes"
	"strings"
	"testing"
)

const badURL = "http://bad\x7furl" // control byte -> url.Parse error

func TestStatusBadURL(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := New().Run([]string{"--api", badURL, "status"}, nil, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("exit = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "ctl:") {
		t.Errorf("stderr = %q, want ctl error", stderr.String())
	}
}

func TestActionBadURL(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := New().Run([]string{"--api", badURL, "stop", "svc"}, nil, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("exit = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "ctl:") {
		t.Errorf("stderr = %q, want ctl error", stderr.String())
	}
}
