package eng

import (
	"strings"
	"testing"
)

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

func TestCompiledUserPolyListParamQuoted(t *testing.T) {
	// A dynamic operand that is a LIST at run time: the poly re-match
	// binds the List overload, and the delivery loop quotes the param —
	// the compiled mirror of the interpreter's binding rule, so the
	// returned param CANONS as the quoted list on both paths (the
	// display render hides the flag; canon is the round-trip form).
	canonAll := func(vs []Value) string {
		parts := make([]string, len(vs))
		for i, v := range vs {
			parts[i] = CanonValue(v)
		}
		return strings.Join(parts, " | ")
	}
	tokens := func() []Value {
		return []Value{
			NewWord("upolyl"),
			NewOpenParen(), NewWord("clany"), NewCloseParen(),
		}
	}
	ri := covRegistry(t, registerUserPolyList)
	out, err := NewTop(ri).Run(tokens())
	if err != nil {
		t.Fatalf("interpreted: %v", err)
	}
	if got := canonAll(out); got != "(quote [1 2])" {
		t.Fatalf("interpreted = %q, want %q", got, "(quote [1 2])")
	}
	rc := covRegistry(t, registerUserPolyList)
	prog, reason := compileTokens(t, rc, tokens())
	if prog == nil {
		t.Logf("compile refused (%s)", reason)
		return
	}
	cOut, cErr := RunProgram(prog, rc)
	if cErr != nil {
		t.Fatalf("compiled: %v", cErr)
	}
	if got := canonAll(cOut); got != "(quote [1 2])" {
		t.Errorf("compiled = %q, want %q", got, "(quote [1 2])")
	}
}
