package flagutil

import (
	"bytes"
	"flag"
	"strings"
	"testing"
)

// newFS builds a FlagSet with one bool and one string flag, writing its
// own diagnostics into out instead of os.Stderr.
func newFS(out *bytes.Buffer) (*flag.FlagSet, *bool, *string) {
	fs := flag.NewFlagSet("probe", flag.ContinueOnError)
	fs.SetOutput(out)
	b := fs.Bool("json", false, "bool flag")
	s := fs.String("color", "auto", "string flag")
	return fs, b, s
}

func TestParseInterleavedFlagsBeforeOperands(t *testing.T) {
	var out bytes.Buffer
	fs, jsonOut, color := newFS(&out)
	pos, err := ParseInterleaved(fs, []string{"--json", "--color", "never", "a.boru", "b.boru"})
	if err != nil {
		t.Fatalf("ParseInterleaved: %v", err)
	}
	if !*jsonOut || *color != "never" {
		t.Errorf("flags = (%v, %q), want (true, never)", *jsonOut, *color)
	}
	if strings.Join(pos, " ") != "a.boru b.boru" {
		t.Errorf("positionals = %v, want [a.boru b.boru]", pos)
	}
}

// The whole reason this helper exists: a flag AFTER the operands is a
// flag, not a third file. A plain fs.Parse fails this test.
func TestParseInterleavedFlagsAfterOperands(t *testing.T) {
	var out bytes.Buffer
	fs, jsonOut, color := newFS(&out)
	pos, err := ParseInterleaved(fs, []string{"a.boru", "b.boru", "--json", "--color", "always"})
	if err != nil {
		t.Fatalf("ParseInterleaved: %v", err)
	}
	if !*jsonOut || *color != "always" {
		t.Errorf("flags = (%v, %q), want (true, always)", *jsonOut, *color)
	}
	if strings.Join(pos, " ") != "a.boru b.boru" {
		t.Errorf("positionals = %v, want [a.boru b.boru]", pos)
	}
}

func TestParseInterleavedFlagsBetweenOperands(t *testing.T) {
	var out bytes.Buffer
	fs, jsonOut, _ := newFS(&out)
	pos, err := ParseInterleaved(fs, []string{"a.boru", "--json", "b.boru"})
	if err != nil {
		t.Fatalf("ParseInterleaved: %v", err)
	}
	if !*jsonOut {
		t.Error("--json between operands was not parsed")
	}
	if strings.Join(pos, " ") != "a.boru b.boru" {
		t.Errorf("positionals = %v, want [a.boru b.boru]", pos)
	}
}

func TestParseInterleavedDashDashEscapesOperand(t *testing.T) {
	var out bytes.Buffer
	fs, _, _ := newFS(&out)
	pos, err := ParseInterleaved(fs, []string{"--", "-weird.boru"})
	if err != nil {
		t.Fatalf("ParseInterleaved: %v", err)
	}
	if strings.Join(pos, " ") != "-weird.boru" {
		t.Errorf("positionals = %v, want [-weird.boru]", pos)
	}
}

func TestParseInterleavedEmptyArgs(t *testing.T) {
	var out bytes.Buffer
	fs, _, _ := newFS(&out)
	pos, err := ParseInterleaved(fs, nil)
	if err != nil {
		t.Fatalf("ParseInterleaved: %v", err)
	}
	if len(pos) != 0 {
		t.Errorf("positionals = %v, want none", pos)
	}
}

// Negative: an unknown flag is REJECTED — it must not be silently
// accumulated as a positional operand.
func TestParseInterleavedUnknownFlagRejected(t *testing.T) {
	var out bytes.Buffer
	fs, _, _ := newFS(&out)
	pos, err := ParseInterleaved(fs, []string{"a.boru", "--nope"})
	if err == nil {
		t.Fatalf("expected an error for an unknown flag; positionals = %v", pos)
	}
	if pos != nil {
		t.Errorf("positionals = %v, want nil on error", pos)
	}
	if !strings.Contains(out.String(), "flag provided but not defined") {
		t.Errorf("FlagSet output = %q, want the undefined-flag message", out.String())
	}
}

// Negative: -h is reported as flag.ErrHelp, not swallowed.
func TestParseInterleavedHelpIsErrHelp(t *testing.T) {
	var out bytes.Buffer
	fs, _, _ := newFS(&out)
	if _, err := ParseInterleaved(fs, []string{"-h"}); err != flag.ErrHelp {
		t.Fatalf("err = %v, want flag.ErrHelp", err)
	}
	if !strings.Contains(out.String(), "-json") {
		t.Errorf("FlagSet output = %q, want the usage listing", out.String())
	}
}
