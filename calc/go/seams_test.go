package calc

import (
	"errors"
	"strings"
	"testing"

	eng "github.com/aql-lang/aql/eng/go"
)

// New's error arms are drivable only through the seams
// (design/TEST-SEAMS.10.md): registry construction and word
// registration cannot fail on a healthy build.
func TestNewErrorArms(t *testing.T) {
	prevReg, prevErr := newRegistry, registryErr
	t.Cleanup(func() { newRegistry, registryErr = prevReg, prevErr })

	newRegistry = func() (*eng.Registry, error) { return nil, errors.New("boom") }
	if _, err := New(nil); err == nil || !strings.Contains(err.Error(), "registry") {
		t.Errorf("New with failing registry: got %v, want registry error", err)
	}
	newRegistry = prevReg

	registryErr = func(*eng.Registry) error { return errors.New("dup word") }
	if _, err := New(nil); err == nil || !strings.Contains(err.Error(), "word registration") {
		t.Errorf("New with registration error: got %v, want registration error", err)
	}
}

// The unary preferInt arm (Integer in, integral result → Integer out)
// and sqrt's success arm.
func TestUnaryIntegerAndSqrtSuccess(t *testing.T) {
	c, _ := newCalc(t)
	stk, err := c.Eval("neg 5")
	if err != nil {
		t.Fatal(err)
	}
	if n := asInt(t, stk, "neg 5"); n != -5 {
		t.Errorf("neg 5 = %d, want -5", n)
	}
	c.Reset()
	stk, err = c.Eval("sqrt 9")
	if err != nil {
		t.Fatal(err)
	}
	if f := asDec(t, stk, "sqrt 9"); f != 3 {
		t.Errorf("sqrt 9 = %v, want 3", f)
	}
}
