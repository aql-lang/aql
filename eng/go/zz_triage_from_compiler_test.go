package eng

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

import (
	"testing"
)

// Test blocks re-homed by compiler-driven triage at the carve.

func TestCompiledBranchBothComputedArms(t *testing.T) {
	// `cif (cond) (a) (b)` — both arms eagerly computed events.
	got := runDifferential(t, registerBranchWords, func() []Value {
		return []Value{
			NewWord("cif"),
			NewOpenParen(), NewWord("cgt"), NewInteger(5), NewInteger(3), NewCloseParen(),
			NewOpenParen(), NewWord("cadd"), NewInteger(1), NewInteger(2), NewCloseParen(),
			NewOpenParen(), NewWord("cadd"), NewInteger(30), NewInteger(40), NewCloseParen(),
		}
	})
	if got != "3" {
		t.Errorf("both-computed then = %q", got)
	}
	got = runDifferential(t, registerBranchWords, func() []Value {
		return []Value{
			NewWord("cif"),
			NewOpenParen(), NewWord("cgt"), NewInteger(3), NewInteger(5), NewCloseParen(),
			NewOpenParen(), NewWord("cadd"), NewInteger(1), NewInteger(2), NewCloseParen(),
			NewOpenParen(), NewWord("cadd"), NewInteger(30), NewInteger(40), NewCloseParen(),
		}
	})
	if got != "70" {
		t.Errorf("both-computed else = %q", got)
	}
}

func TestCompiledBranchComputedElseArm(t *testing.T) {
	// `cif (cond) [body] (expr)` — the else value is an eager event.
	got := runDifferential(t, registerBranchWords, func() []Value {
		return []Value{
			NewWord("cif"),
			NewOpenParen(), NewWord("cgt"), NewInteger(5), NewInteger(3), NewCloseParen(),
			codeBody(NewWord("cadd"), NewInteger(1), NewInteger(2)),
			NewOpenParen(), NewWord("cadd"), NewInteger(30), NewInteger(40), NewCloseParen(),
		}
	})
	if got != "3" {
		t.Errorf("computed-else taken-then = %q", got)
	}
	got = runDifferential(t, registerBranchWords, func() []Value {
		return []Value{
			NewWord("cif"),
			NewOpenParen(), NewWord("cgt"), NewInteger(3), NewInteger(5), NewCloseParen(),
			codeBody(NewWord("cadd"), NewInteger(1), NewInteger(2)),
			NewOpenParen(), NewWord("cadd"), NewInteger(30), NewInteger(40), NewCloseParen(),
		}
	})
	if got != "70" {
		t.Errorf("computed-else taken-else = %q", got)
	}
}

func TestCompiledBranchComputedThenArm(t *testing.T) {
	// `cif (cond) (expr) [body]` — the then value is an eager event.
	got := runDifferential(t, registerBranchWords, func() []Value {
		return []Value{
			NewWord("cif"),
			NewOpenParen(), NewWord("cgt"), NewInteger(5), NewInteger(3), NewCloseParen(),
			NewOpenParen(), NewWord("cmul"), NewInteger(6), NewInteger(7), NewCloseParen(),
			codeBody(NewInteger(0)),
		}
	})
	if got != "42" {
		t.Errorf("computed-then taken-then = %q", got)
	}
	got = runDifferential(t, registerBranchWords, func() []Value {
		return []Value{
			NewWord("cif"),
			NewOpenParen(), NewWord("cgt"), NewInteger(3), NewInteger(5), NewCloseParen(),
			NewOpenParen(), NewWord("cmul"), NewInteger(6), NewInteger(7), NewCloseParen(),
			codeBody(NewInteger(0)),
		}
	})
	if got != "0" {
		t.Errorf("computed-then taken-else = %q", got)
	}
}
