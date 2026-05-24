package run

import (
	"bytes"
	"strings"
	"testing"
)

func TestEvalSuccess(t *testing.T) {
	var buf bytes.Buffer
	err := Eval(&buf, "1 add 2", "", 0)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "3") {
		t.Errorf("expected '3' in output, got %q", buf.String())
	}
}

func TestEvalParseError(t *testing.T) {
	var buf bytes.Buffer
	err := Eval(&buf, `"unterminated`, "", 0)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "parse error") {
		t.Errorf("expected 'parse error', got %q", err.Error())
	}
}

func TestEvalEngineError(t *testing.T) {
	var buf bytes.Buffer
	err := Eval(&buf, "10 div 0", "", 0)
	if err == nil {
		t.Fatal("expected error")
	}
}
