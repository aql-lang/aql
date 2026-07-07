package policy

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	pol "github.com/aql-lang/aql/lang/go/policy"
)

// failWriter fails every Write, for driving encode-failure arms.
type failWriter struct{}

func (failWriter) Write([]byte) (int, error) { return 0, errors.New("write refused") }

// TestS7n_ShowProfileEncodeError drives showProfile's json Encode
// failure arm (policy.go) by giving it a stdout that always errors —
// no seam needed, since stdout is a plain io.Writer parameter
// reachable straight from the CLI's Run entry point.
func TestS7n_ShowProfileEncodeError(t *testing.T) {
	var stderr bytes.Buffer
	code := New().Run([]string{"show", "full"}, nil, failWriter{}, &stderr)
	if code != 1 {
		t.Fatalf("Run(show) = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "show: encode:") {
		t.Errorf("stderr = %q, want a show-encode error", stderr.String())
	}
}

// fakePolicy is a minimal pol.Policy whose Check always returns a
// plain error (not *pol.Denied), exercising testCheck's "reason"
// fallback branch that a real Compiled/composed policy can never
// reach.
type fakePolicy struct{ name string }

func (f fakePolicy) Check(scope, op string, args pol.Args) error { return errors.New("boom-check") }
func (f fakePolicy) CheckGlobal(name string) error               { return nil }
func (f fakePolicy) CheckWord(name string) error                 { return nil }
func (f fakePolicy) Installed(scope string) bool                 { return true }
func (f fakePolicy) Scope(name string) pol.Scope                 { return pol.Scope{} }
func (f fakePolicy) Limits() pol.Limits                          { return pol.Limits{} }
func (f fakePolicy) Name() string                                { return f.name }

// TestS7n_ExplainNonDeniedErrorReason drives testCheck's "reason"
// branch (the else arm when Check's error is not a *pol.Denied) by
// swapping the policyLoadAuto seam to return a fakePolicy.
func TestS7n_ExplainNonDeniedErrorReason(t *testing.T) {
	orig := policyLoadAuto
	t.Cleanup(func() { policyLoadAuto = orig })
	policyLoadAuto = func(src string) (pol.Policy, error) {
		return fakePolicy{name: "fake"}, nil
	}

	var stdout, stderr bytes.Buffer
	code := New().Run([]string{"explain", "fake", "fileops.read"}, nil, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("Run(explain) = %d, want 1; stderr: %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "decision: DENY") {
		t.Errorf("stdout = %q, want DENY decision", stdout.String())
	}
	if !strings.Contains(stdout.String(), "reason:   boom-check") {
		t.Errorf("stdout = %q, want the plain-error reason line", stdout.String())
	}
}
