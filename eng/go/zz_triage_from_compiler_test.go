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

	core "github.com/boru-lang/boru/core/go"
)

// Test blocks re-homed by compiler-driven triage at the carve.

func TestCompiledBranchBothComputedArms(t *testing.T) {
	// `cif (cond) (a) (b)` — both arms eagerly computed events.
	got := runDifferential(t, registerBranchWords, func() []core.Value {
		return []core.Value{
			core.NewWord("cif"),
			core.NewOpenParen(), core.NewWord("cgt"), core.NewInteger(5), core.NewInteger(3), core.NewCloseParen(),
			core.NewOpenParen(), core.NewWord("cadd"), core.NewInteger(1), core.NewInteger(2), core.NewCloseParen(),
			core.NewOpenParen(), core.NewWord("cadd"), core.NewInteger(30), core.NewInteger(40), core.NewCloseParen(),
		}
	})
	if got != "3" {
		t.Errorf("both-computed then = %q", got)
	}
	got = runDifferential(t, registerBranchWords, func() []core.Value {
		return []core.Value{
			core.NewWord("cif"),
			core.NewOpenParen(), core.NewWord("cgt"), core.NewInteger(3), core.NewInteger(5), core.NewCloseParen(),
			core.NewOpenParen(), core.NewWord("cadd"), core.NewInteger(1), core.NewInteger(2), core.NewCloseParen(),
			core.NewOpenParen(), core.NewWord("cadd"), core.NewInteger(30), core.NewInteger(40), core.NewCloseParen(),
		}
	})
	if got != "70" {
		t.Errorf("both-computed else = %q", got)
	}
}

func TestCompiledBranchComputedElseArm(t *testing.T) {
	// `cif (cond) [body] (expr)` — the else value is an eager event.
	got := runDifferential(t, registerBranchWords, func() []core.Value {
		return []core.Value{
			core.NewWord("cif"),
			core.NewOpenParen(), core.NewWord("cgt"), core.NewInteger(5), core.NewInteger(3), core.NewCloseParen(),
			codeBody(core.NewWord("cadd"), core.NewInteger(1), core.NewInteger(2)),
			core.NewOpenParen(), core.NewWord("cadd"), core.NewInteger(30), core.NewInteger(40), core.NewCloseParen(),
		}
	})
	if got != "3" {
		t.Errorf("computed-else taken-then = %q", got)
	}
	got = runDifferential(t, registerBranchWords, func() []core.Value {
		return []core.Value{
			core.NewWord("cif"),
			core.NewOpenParen(), core.NewWord("cgt"), core.NewInteger(3), core.NewInteger(5), core.NewCloseParen(),
			codeBody(core.NewWord("cadd"), core.NewInteger(1), core.NewInteger(2)),
			core.NewOpenParen(), core.NewWord("cadd"), core.NewInteger(30), core.NewInteger(40), core.NewCloseParen(),
		}
	})
	if got != "70" {
		t.Errorf("computed-else taken-else = %q", got)
	}
}

func TestCompiledBranchComputedThenArm(t *testing.T) {
	// `cif (cond) (expr) [body]` — the then value is an eager event.
	got := runDifferential(t, registerBranchWords, func() []core.Value {
		return []core.Value{
			core.NewWord("cif"),
			core.NewOpenParen(), core.NewWord("cgt"), core.NewInteger(5), core.NewInteger(3), core.NewCloseParen(),
			core.NewOpenParen(), core.NewWord("cmul"), core.NewInteger(6), core.NewInteger(7), core.NewCloseParen(),
			codeBody(core.NewInteger(0)),
		}
	})
	if got != "42" {
		t.Errorf("computed-then taken-then = %q", got)
	}
	got = runDifferential(t, registerBranchWords, func() []core.Value {
		return []core.Value{
			core.NewWord("cif"),
			core.NewOpenParen(), core.NewWord("cgt"), core.NewInteger(3), core.NewInteger(5), core.NewCloseParen(),
			core.NewOpenParen(), core.NewWord("cmul"), core.NewInteger(6), core.NewInteger(7), core.NewCloseParen(),
			codeBody(core.NewInteger(0)),
		}
	})
	if got != "0" {
		t.Errorf("computed-then taken-else = %q", got)
	}
}
