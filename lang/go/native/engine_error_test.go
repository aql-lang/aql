package native

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
)

// =============================================================================
// Error value and error word tests
// =============================================================================

func TestErrorValueType(t *testing.T) {
	err := fmt.Errorf("something went wrong")
	v := NewError(err)
	if !IsError(v) {
		t.Fatal("expected IsError() == true")
	}
	_as259, _ := AsError(v)
	if _as259.Message != "something went wrong" {
		_as260, _ := AsError(v)
		t.Errorf("message = %q, want %q", _as260.Message, "something went wrong")
	}
	if v.String() != "error(something went wrong)" {
		t.Errorf("String() = %q, want %q", v.String(), "error(something went wrong)")
	}
}

func TestTopLevelErrorHalts(t *testing.T) {
	// 1 div 0 mul 2 → halts with error, mul 2 never runs
	reg, _ := DefaultRegistry()
	e := NewTop(reg)
	_, err := e.Run([]Value{
		NewInteger(1), NewWord("div"), NewInteger(0),
		NewWord("mul"), NewInteger(2),
	})
	if err == nil {
		t.Fatal("expected error from div 0")
	}
	if !strings.Contains(err.Error(), "division by zero") {
		t.Errorf("expected 'division by zero', got %q", err.Error())
	}
}

func TestDoBlockCatchesError(t *testing.T) {
	// do [1 div 0] → error value on stack (not a Go error)
	reg, _ := DefaultRegistry()
	e := NewTop(reg)
	result, err := e.Run([]Value{
		NewWord("do"),
		NewList([]Value{NewInteger(1), NewWord("div"), NewInteger(0)}),
	})
	if err != nil {
		t.Fatalf("do block should catch error, got: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d: %v", len(result), result)
	}
	if !IsError(result[0]) {
		t.Fatalf("expected error value, got %s", result[0].String())
	}
	_as261, _ := AsError(result[0])
	if !strings.Contains(_as261.Message, "division by zero") {
		_as262, _ := AsError(result[0])
		t.Errorf("error message = %q", _as262.Message)
	}
}

func TestErrorWordSimple(t *testing.T) {
	// do [1 div 0] error [print] → prints "division by zero", continues
	reg, _ := DefaultRegistry()
	reg.Output = &bytes.Buffer{}
	e := NewTop(reg)
	result, err := e.Run([]Value{
		NewWord("do"),
		NewList([]Value{NewInteger(1), NewWord("div"), NewInteger(0)}),
		NewWord("error"),
		NewList([]Value{NewWord("print")}),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 0 {
		t.Errorf("expected empty stack after error, got %v", result)
	}
	out := reg.Output.(*bytes.Buffer).String()
	if !strings.Contains(out, "division by zero") {
		t.Errorf("expected 'division by zero' in output, got %q", out)
	}
}

func TestErrorWordWithList(t *testing.T) {
	// do [1 div 0] error [print] 3 mul 4 → 12
	// The error is on the stack inside the handler list; print consumes it.
	reg, _ := DefaultRegistry()
	reg.Output = &bytes.Buffer{}
	e := NewTop(reg)
	result, err := e.Run([]Value{
		NewWord("do"),
		NewList([]Value{NewInteger(1), NewWord("div"), NewInteger(0)}),
		NewWord("error"),
		NewList([]Value{NewWord("print")}),
		NewInteger(3), NewWord("mul"), NewInteger(4),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_as263, _ := AsInteger(result[0])
	if len(result) != 1 || _as263 != 12 {
		t.Errorf("expected [12], got %v", result)
	}
	out := reg.Output.(*bytes.Buffer).String()
	if !strings.Contains(out, "division by zero") {
		t.Errorf("expected error message in output, got %q", out)
	}
}

func TestErrorWordContinuesExecution(t *testing.T) {
	// do [1 div 0] error [drop] 3 mul 4 → 12
	reg, _ := DefaultRegistry()
	reg.Output = &bytes.Buffer{}
	e := NewTop(reg)
	result, err := e.Run([]Value{
		NewWord("do"),
		NewList([]Value{NewInteger(1), NewWord("div"), NewInteger(0)}),
		NewWord("error"),
		NewList([]Value{NewWord("drop")}),
		NewInteger(3), NewWord("mul"), NewInteger(4),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_as264, _ := AsInteger(result[0])
	if len(result) != 1 || _as264 != 12 {
		t.Errorf("expected [12], got %v", result)
	}
}

func TestDoBlockSuccessNoError(t *testing.T) {
	// do [1 add 2] → 3 (no error, normal result)
	reg, _ := DefaultRegistry()
	e := NewTop(reg)
	result, err := e.Run([]Value{
		NewWord("do"),
		NewList([]Value{NewInteger(1), NewWord("add"), NewInteger(2)}),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_as265, _ := AsInteger(result[0])
	if len(result) != 1 || _as265 != 3 {
		t.Errorf("expected [3], got %v", result)
	}
}

func TestUnhandledErrorOnStack(t *testing.T) {
	// do [1 div 0] 3 mul 4 → error value stays on stack alongside 12
	// The error is inert data — it doesn't block subsequent operations
	// that don't consume it.
	reg, _ := DefaultRegistry()
	reg.Output = &bytes.Buffer{}
	e := NewTop(reg)
	result, err := e.Run([]Value{
		NewWord("do"),
		NewList([]Value{NewInteger(1), NewWord("div"), NewInteger(0)}),
		NewInteger(3), NewWord("mul"), NewInteger(4),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("expected 2 results, got %d: %v", len(result), result)
	}
	if !IsError(result[0]) {
		t.Errorf("result[0] should be error, got %s", result[0].String())
	}
	_as266, _ := AsInteger(result[1])
	if _as266 != 12 {
		t.Errorf("result[1] = %v, want 12", result[1])
	}
}
