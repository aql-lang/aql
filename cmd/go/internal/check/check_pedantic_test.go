package check

import (
	"bytes"
	"strings"
	"testing"

	"github.com/boru-lang/boru/lang/go"
)

// warnSrc emits exactly one warning (unused_def) and no errors.
const warnSrc = "def y 1  2"

// infoSrc emits exactly one info (forward_strands_operand) and no errors.
const infoSrc = "1 2 add 3 mul"

// errSrc emits an Error-severity diagnostic.
const errSrc = "1 add true"

// TestGatesMatrix pins Opts.gates over every tier combination. Soft must
// short-circuit each one; errors gate regardless of Pedantic; the
// advisory tiers gate only under Pedantic.
func TestGatesMatrix(t *testing.T) {
	cases := []struct {
		name     string
		opts     Opts
		sum      lang.CheckSummary
		wantGate bool
		wantMsg  string
	}{
		{"clean default", Opts{}, lang.CheckSummary{}, false, ""},
		{"clean pedantic", Opts{Pedantic: true}, lang.CheckSummary{}, false, ""},
		{"errors default", Opts{}, lang.CheckSummary{Errors: 2}, true, "2 error(s)"},
		{"errors pedantic", Opts{Pedantic: true}, lang.CheckSummary{Errors: 1}, true, "1 error(s)"},
		{"errors soft", Opts{Soft: true}, lang.CheckSummary{Errors: 3}, false, ""},
		{"warnings default", Opts{}, lang.CheckSummary{Warnings: 1}, false, ""},
		{"warnings pedantic", Opts{Pedantic: true}, lang.CheckSummary{Warnings: 1}, true, "1 warning(s)"},
		{"infos default", Opts{}, lang.CheckSummary{Infos: 4}, false, ""},
		{"infos pedantic", Opts{Pedantic: true}, lang.CheckSummary{Infos: 4}, true, "4 info"},
		{"soft beats pedantic", Opts{Soft: true, Pedantic: true}, lang.CheckSummary{Warnings: 9}, false, ""},
		// An error under Pedantic must report as an ERROR, not be
		// swallowed by the advisory arm.
		{"errors outrank advisories", Opts{Pedantic: true}, lang.CheckSummary{Errors: 1, Warnings: 5}, true, "1 error(s)"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.opts.gates(tc.sum)
			if tc.wantGate {
				if err == nil {
					t.Fatalf("gates() = nil, want a gating error")
				}
				if !strings.Contains(err.Error(), tc.wantMsg) {
					t.Fatalf("gates() = %q, want it to mention %q", err, tc.wantMsg)
				}
				return
			}
			if err != nil {
				t.Fatalf("gates() = %v, want nil", err)
			}
		})
	}
}

// TestRunWithPedanticText covers the text path: a warning gates only
// with Pedantic set, and the message names the promoting flag.
func TestRunWithPedanticText(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if err := RunWith(&stdout, &stderr, warnSrc, Opts{}); err != nil {
		t.Fatalf("without --pedantic: err = %v, want nil", err)
	}
	stdout.Reset()
	stderr.Reset()
	err := RunWith(&stdout, &stderr, warnSrc, Opts{Pedantic: true})
	if err == nil || !strings.Contains(err.Error(), "--pedantic") {
		t.Fatalf("with --pedantic: err = %v, want a --pedantic failure", err)
	}
	if !strings.Contains(stderr.String(), "unused_def") {
		t.Fatalf("stderr = %q, want the unused_def diagnostic", stderr.String())
	}
}

// TestRunWithPedanticJSON covers the JSON path separately: the two exit
// sites are distinct, so `--json --pedantic` must not disagree with
// `--pedantic`.
func TestRunWithPedanticJSON(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if err := RunWith(&stdout, &stderr, infoSrc, Opts{JSON: true}); err != nil {
		t.Fatalf("json without --pedantic: err = %v, want nil", err)
	}
	if !strings.Contains(stdout.String(), "\"summary\"") {
		t.Fatalf("stdout = %q, want a JSON result", stdout.String())
	}
	stdout.Reset()
	if err := RunWith(&stdout, &stderr, infoSrc, Opts{JSON: true, Pedantic: true}); err == nil {
		t.Fatalf("json with --pedantic: err = nil, want a gating error")
	}
}

// TestRunWithPedanticSoft pins the composition rule: --soft still means
// "never gate", so --soft --pedantic exits 0 on both paths.
func TestRunWithPedanticSoft(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if err := RunWith(&stdout, &stderr, warnSrc, Opts{Soft: true, Pedantic: true}); err != nil {
		t.Fatalf("text: err = %v, want nil under --soft", err)
	}
	if err := RunWith(&stdout, &stderr, errSrc, Opts{JSON: true, Soft: true, Pedantic: true}); err != nil {
		t.Fatalf("json: err = %v, want nil under --soft even with errors", err)
	}
}

// TestRunCLIPedanticFlag covers both spellings of the flag through the
// argv parser, and asserts the un-flagged run stays exit 0.
func TestRunCLIPedanticFlag(t *testing.T) {
	for _, flag := range []string{"--pedantic", "-pedantic"} {
		var stdout, stderr bytes.Buffer
		if code := RunCLI([]string{flag, "-e", warnSrc}, &stdout, &stderr); code == 0 {
			t.Fatalf("%s: exit = 0, want non-zero on a warning", flag)
		}
	}
	var stdout, stderr bytes.Buffer
	if code := RunCLI([]string{"-e", warnSrc}, &stdout, &stderr); code != 0 {
		t.Fatalf("without the flag: exit = %d, want 0", code)
	}
}

// TestRunColorDelegates pins that the fixed-shape wrapper still routes
// through RunWith with Pedantic unset — a caller of the old API must not
// start gating on advisories.
func TestRunColorDelegates(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if err := RunColor(&stdout, &stderr, warnSrc, "", 0, false, false, false, false); err != nil {
		t.Fatalf("RunColor = %v, want nil (advisories must not gate)", err)
	}
}
