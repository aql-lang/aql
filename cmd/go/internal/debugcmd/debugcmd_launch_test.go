package debugcmd

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	lang "github.com/aql-lang/aql/lang/go"
)

// The interactive-launch end-to-end tests: each writes a program file,
// drives the debugger with a scripted command transcript on stdin, and
// asserts the resulting transcript (design/AQL-DEBUGGER.0.md §11 — the
// scripted front end IS the CI story, so these are the debugger's
// executable spec).

func writeProgram(t *testing.T, src string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "prog.aql")
	if err := os.WriteFile(path, []byte(src), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func launch(t *testing.T, args []string, stdin string) (int, string, string) {
	t.Helper()
	var out, errOut bytes.Buffer
	code := New().Run(args, strings.NewReader(stdin), &out, &errOut)
	return code, out.String(), errOut.String()
}

// requireOrder asserts each marker appears in out, each after the previous.
func requireOrder(t *testing.T, out string, markers ...string) {
	t.Helper()
	at := 0
	for _, m := range markers {
		i := strings.Index(out[at:], m)
		if i < 0 {
			t.Fatalf("marker %q missing (after byte %d) in transcript:\n%s", m, at, out)
		}
		at += i + len(m)
	}
}

func TestLaunchStepsSourceLines(t *testing.T) {
	path := writeProgram(t, "def x 1\ndef y 2\nx add y\n")
	code, out, errOut := launch(t, []string{path}, "step\nstep\nstep\n")
	if code != 0 {
		t.Fatalf("exit = %d, stderr = %q", code, errOut)
	}
	// One pause per source line, in order, then the result and exit notice.
	requireOrder(t, out,
		"at "+path+":1", "def x 1",
		"at "+path+":2", "def y 2",
		"at "+path+":3", "x add y",
		"3", "(program exited)")
}

func TestLaunchCoalescesOneLineToOneStop(t *testing.T) {
	// Negative (§5.1): a single source line expands to MANY engine steps —
	// the debugger must stop exactly once for it, not per token.
	path := writeProgram(t, "1 add 2 add 3\n")
	code, out, _ := launch(t, []string{path}, "step\nstep\nstep\n")
	if code != 0 {
		t.Fatalf("exit = %d", code)
	}
	if got := strings.Count(out, "at "+path+":1"); got != 1 {
		t.Errorf("one source line must pause once, got %d stops:\n%s", got, out)
	}
	if !strings.Contains(out, "6") {
		t.Errorf("result missing:\n%s", out)
	}
}

func TestLaunchContinueRunsToCompletion(t *testing.T) {
	path := writeProgram(t, "def x 1\ndef y 2\nx add y\n")
	code, out, _ := launch(t, []string{path}, "continue\n")
	if code != 0 {
		t.Fatalf("exit = %d", code)
	}
	if got := strings.Count(out, "at "+path+":"); got != 1 {
		t.Errorf("continue must not pause again without breakpoints; got %d stops", got)
	}
	requireOrder(t, out, "3", "(program exited)")
}

func TestLaunchQuitDetachesAndProgramDrains(t *testing.T) {
	// §5.4 / §11: quit DETACHES — the program still runs to completion,
	// so every side effect after the quit must still happen.
	path := writeProgram(t, "print \"before\"\nprint \"after\"\n")
	code, out, _ := launch(t, []string{path}, "quit\n")
	if code != 0 {
		t.Fatalf("exit = %d", code)
	}
	requireOrder(t, out, "(detached", "before", "after", "(program exited)")
}

func TestLaunchEOFDetaches(t *testing.T) {
	path := writeProgram(t, "print \"ran\"\n")
	code, out, _ := launch(t, []string{path}, "")
	if code != 0 {
		t.Fatalf("exit = %d", code)
	}
	requireOrder(t, out, "(detached", "ran", "(program exited)")
}

func TestLaunchBreakMarkerPausesUnderContinue(t *testing.T) {
	path := writeProgram(t, `import "aql:debug"
def a 1
Debug.break
def b 2
a add b
`)
	code, out, _ := launch(t, []string{path}, "continue\ncontinue\n")
	if code != 0 {
		t.Fatalf("exit = %d", code)
	}
	requireOrder(t, out, "break hit (Debug.break)", "3", "(program exited)")
}

func TestLaunchBreakWhenFalseNeverPauses(t *testing.T) {
	// Negative: a false conditional breakpoint must not stop the program.
	path := writeProgram(t, `import "aql:debug"
Debug.break-when false
1 add 2
`)
	code, out, _ := launch(t, []string{path}, "continue\n")
	if code != 0 {
		t.Fatalf("exit = %d", code)
	}
	if strings.Contains(out, "break hit") {
		t.Errorf("break-when false must not pause:\n%s", out)
	}
}

func TestLaunchQuitAtBreakDetaches(t *testing.T) {
	// Quitting AT a break marker detaches; later markers stay silent
	// (OnStep's detached arm) and the program still finishes.
	path := writeProgram(t, `import "aql:debug"
print "one"
Debug.break
print "two"
Debug.break
print "three"
`)
	code, out, _ := launch(t, []string{path}, "continue\nquit\n")
	if code != 0 {
		t.Fatalf("exit = %d", code)
	}
	if got := strings.Count(out, "break hit"); got != 1 {
		t.Errorf("after detach no further break renders; got %d", got)
	}
	requireOrder(t, out, "one", "break hit", "(detached", "two", "three", "(program exited)")
}

func TestLaunchInspectionCommands(t *testing.T) {
	path := writeProgram(t, "def greeting \"hi\"\ndef n 41\nn\nadd 1\n")
	stdin := strings.Join([]string{
		"stack",                // row 1: nothing on the stack yet
		"list",                 // row 1: low-edge window
		"step", "step", "step", // → row 4, with 41 on the stack
		"defs",
		"defs all",
		"stack",
		"bt",                 // top level — no fn frames
		"list",               // row 4: high-edge window
		"p n add 9",          // eval against live defs
		"print (",            // parse-error arm
		"print boguswordxyz", // runtime-error arm
		"print",              // usage arm
		"help",
		"whatnot", // unknown command
		"",        // empty line: ignored
		"c",
	}, "\n") + "\n"
	code, out, errOut := launch(t, []string{path}, stdin)
	if code != 0 {
		t.Fatalf("exit = %d, stderr = %q", code, errOut)
	}
	// Negative: bare `defs` hides the registry's native mirror — the exact
	// count "defs (2):" pins that only the program's two bindings show
	// (the unfiltered table holds 100+ natives, as `defs all` then proves).
	requireOrder(t, out,
		"(stack empty)",
		"->    1  def greeting",
		"at "+path+":4",
		"defs (2):",
		"greeting = hi",
		"n = 41",
		"quote = fn", // `defs all` lifts the program-only filter
		"#0  41",
		"(top level — no fn frames)",
		"->    4  add 1",
		"50",
		"parse error:",
		"error:",
		"usage: print <expr>",
		"commands:",
		`unknown command "whatnot"`,
		"42", "(program exited)")
}

func TestLaunchBacktraceInsideFn(t *testing.T) {
	// A Debug.break inside a fn body pauses with the inline frame live:
	// bt names the fn and its bindings, defs shows the bound param (§5.3).
	path := writeProgram(t, `import "aql:debug"
def f fn [[x:Integer] [Integer] [
Debug.break
x add 1
]]
f 41
`)
	code, out, _ := launch(t, []string{path}, "continue\nbt\ndefs\ncontinue\n")
	if code != 0 {
		t.Fatalf("exit = %d", code)
	}
	requireOrder(t, out, "break hit", "#0  f", "x = 41", "42", "(program exited)")
	if !strings.Contains(out, "(x)") {
		t.Errorf("bt must list the frame's bindings:\n%s", out)
	}
}

func TestLaunchListAtBreakIsUnknown(t *testing.T) {
	// Negative: a one-shot break pause carries no source position (§7),
	// so `list` there reports the line unknown rather than guessing.
	path := writeProgram(t, `import "aql:debug"
Debug.break
1 add 2
`)
	code, out, _ := launch(t, []string{path}, "continue\nlist\ncontinue\n")
	if code != 0 {
		t.Fatalf("exit = %d", code)
	}
	requireOrder(t, out, "break hit", "(source line unknown)")
}

func TestLaunchInnerDebugStepReachesSession(t *testing.T) {
	// A Debug.step the PROGRAM runs consults the same session (the
	// installed DebugOps), with source-located frames via the StepFrame
	// widening — and `step`/`continue` drive it.
	path := writeProgram(t, `import "aql:debug"
[1 add 2] Debug.step
`)
	code, out, _ := launch(t, []string{path}, "continue\nstep\ncontinue\n")
	if code != 0 {
		t.Fatalf("exit = %d", code)
	}
	requireOrder(t, out, "step 0 at "+path+":2", "step 1 at ", "3", "(program exited)")
}

func TestLaunchEvalBreakDoesNotNest(t *testing.T) {
	// Negative (§5 re-entrancy): a `print` expression that itself hits
	// Debug.break must not open a nested prompt.
	path := writeProgram(t, `import "aql:debug"
def n 1
n add 1
`)
	code, out, _ := launch(t, []string{path}, "step\nprint (Debug.break 5)\ncontinue\n")
	if code != 0 {
		t.Fatalf("exit = %d", code)
	}
	if strings.Contains(out, "break hit") {
		t.Errorf("an eval-raised break must not nest a pause:\n%s", out)
	}
	requireOrder(t, out, "5", "2", "(program exited)")
}

func TestLaunchScriptMode(t *testing.T) {
	path := writeProgram(t, "1 add 2\n")
	script := filepath.Join(t.TempDir(), "cmds.dbg")
	if err := os.WriteFile(script, []byte("continue\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	code, out, _ := launch(t, []string{"--script", script, path}, "IGNORED\n")
	if code != 0 {
		t.Fatalf("exit = %d", code)
	}
	// Script mode echoes each command after the prompt, so the transcript
	// is self-contained and reproducible (§6.3).
	requireOrder(t, out, "(adbg) continue", "3", "(program exited)")
}

func TestLaunchScriptMissing(t *testing.T) {
	path := writeProgram(t, "1 add 2\n")
	code, _, errOut := launch(t, []string{"--script", filepath.Join(t.TempDir(), "nope"), path}, "")
	if code != 1 || !strings.Contains(errOut, "script") {
		t.Errorf("exit = %d, stderr = %q", code, errOut)
	}
}

func TestLaunchScriptArgsReachProgram(t *testing.T) {
	path := writeProgram(t, `import "aql:io"
IO.args
`)
	code, out, _ := launch(t, []string{"--no-check", path, "alpha", "beta"}, "continue\n")
	if code != 0 {
		t.Fatalf("exit = %d", code)
	}
	requireOrder(t, out, "alpha", "beta", "(program exited)")
}

func TestLaunchUsageAndFlagErrors(t *testing.T) {
	// No file positional.
	if code, _, errOut := launch(t, []string{"--no-check"}, ""); code != 1 ||
		!strings.Contains(errOut, "usage: aql debug") {
		t.Errorf("no-file: exit %d, stderr %q", code, errOut)
	}
	// Unknown flag.
	if code, _, _ := launch(t, []string{"--bogus", "x.aql"}, ""); code != 1 {
		t.Errorf("bad flag must exit 1, got %d", code)
	}
}

func TestLaunchPreflightGate(t *testing.T) {
	// Check-by-default: a program the checker rejects never starts.
	path := writeProgram(t, "boguswordxyz\n")
	code, out, _ := launch(t, []string{path}, "")
	if code != 1 {
		t.Errorf("preflight failure must exit 1, got %d", code)
	}
	if strings.Contains(out, "(program exited)") {
		t.Errorf("a rejected program must not run:\n%s", out)
	}
}

func TestLaunchParseErrorNoCheck(t *testing.T) {
	path := writeProgram(t, "\"unterminated\n")
	code, _, errOut := launch(t, []string{"--no-check", path}, "")
	if code != 1 || !strings.Contains(errOut, "parse") {
		t.Errorf("exit = %d, stderr = %q", code, errOut)
	}
}

func TestLaunchRuntimeErrorPlainAndColor(t *testing.T) {
	path := writeProgram(t, "def x 1\nboguswordxyz\n")
	// Plain rendering (a buffer is not a terminal under --color=auto).
	code, _, errOut := launch(t, []string{"--no-check", path}, "continue\n")
	if code != 1 || !strings.Contains(errOut, "boguswordxyz") {
		t.Errorf("plain: exit = %d, stderr = %q", code, errOut)
	}
	// Forced color takes the structured-render arm.
	code, _, errOut = launch(t, []string{"--no-check", "--color", "always", path}, "continue\n")
	if code != 1 || !strings.Contains(errOut, "boguswordxyz") {
		t.Errorf("color: exit = %d, stderr = %q", code, errOut)
	}
}

func TestLaunchInitError(t *testing.T) {
	// The langNew seam (design/TEST-SEAMS.10.md): a registry-construction
	// failure surfaces as an init error.
	orig := langNew
	langNew = func(...lang.Options) (*lang.AQL, error) { return nil, errors.New("boom-init") }
	defer func() { langNew = orig }()
	path := writeProgram(t, "1 add 2\n")
	code, _, errOut := launch(t, []string{"--no-check", path}, "")
	if code != 1 || !strings.Contains(errOut, "boom-init") {
		t.Errorf("exit = %d, stderr = %q", code, errOut)
	}
}
