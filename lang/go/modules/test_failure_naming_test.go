package modules

import (
	"bytes"
	"strings"
	"testing"
)

// TestTestCaseFailureIsNamedLoudly pins voxgig DX report B7: a failing
// `Test.test` case is caught (so later cases still run), but the failure
// must be surfaced loudly AND by name — otherwise the only signal is a
// count whose summary assertion (`0 Test.fail-count Assert.equal`) points
// at the summary line, naming nothing about which case failed.
func TestTestCaseFailureIsNamedLoudly(t *testing.T) {
	t.Run("failing case prints a named FAIL line", func(t *testing.T) {
		r := testRegistry(t)
		var buf bytes.Buffer
		r.ErrOutput = &buf
		// Test.test catches the assertion error, so the Run itself
		// succeeds — the name must come from the loud FAIL line.
		runTestAQL(t, r, `[ 1 2 Assert.equal ] "should-fail-named-foo" Test.test`)
		got := buf.String()
		if !strings.Contains(got, "FAIL") || !strings.Contains(got, "should-fail-named-foo") {
			t.Errorf("expected a named FAIL line for the failing case, got: %q", got)
		}
	})

	t.Run("describe path is included", func(t *testing.T) {
		r := testRegistry(t)
		var buf bytes.Buffer
		r.ErrOutput = &buf
		runTestAQL(t, r, `Test.describe "math" [ [ 2 3 Assert.equal ] "adds" Test.test ]`)
		got := buf.String()
		if !strings.Contains(got, "math") || !strings.Contains(got, "adds") {
			t.Errorf("expected FAIL line to include describe path + case name, got: %q", got)
		}
	})

	t.Run("passing case stays quiet", func(t *testing.T) {
		r := testRegistry(t)
		var buf bytes.Buffer
		r.ErrOutput = &buf
		runTestAQL(t, r, `[ 1 1 Assert.equal ] "should-pass" Test.test`)
		if strings.Contains(buf.String(), "FAIL") {
			t.Errorf("passing case must not print a FAIL line, got: %q", buf.String())
		}
	})
}
