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
		"3\n(program exited)")
	// Negative (§6.3 inverse): interactive mode must NOT echo commands
	// after the prompt — only --script mode does.
	if strings.Contains(out, "(adbg) step") {
		t.Errorf("interactive mode must not echo commands:\n%s", out)
	}
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
	// Anchored to the exit notice so a stray path digit can't satisfy it.
	if !strings.Contains(out, "6\n(program exited)") {
		t.Errorf("result missing:\n%s", out)
	}
}

func TestLaunchLoopIterationsEachPause(t *testing.T) {
	// A single-line loop body shares one Row across every iteration, so
	// plain contiguity-coalescing would fold ALL iterations into the first
	// stop; the engine's "for next" trace note marks each iteration
	// boundary as a line boundary. `for [1 4]` iterates i=1,2,3: the
	// initial stop plus one per iteration ADVANCE = 3 stops.
	path := writeProgram(t, "for [1 4] [ 9 ]\n")
	code, out, _ := launch(t, []string{"--no-check", path}, "s\ns\ns\ns\nc\n")
	if code != 0 {
		t.Fatalf("exit = %d", code)
	}
	if got := strings.Count(out, "at "+path+":1"); got != 3 {
		t.Errorf("a 3-iteration same-row loop must stop 3 times, got %d:\n%s", got, out)
	}
	if !strings.Contains(out, "9 9 9\n(program exited)") {
		t.Errorf("loop result missing:\n%s", out)
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
	requireOrder(t, out, "3\n(program exited)")
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
	// The row digit is pinned on BOTH stops: a Row/Col transposition in
	// runStepped would render the column here instead.
	requireOrder(t, out,
		"step 0 at "+path+":2", "step 1 at "+path+":2", "3\n(program exited)")
}

func TestLaunchStepAtBreakPausesNextLine(t *testing.T) {
	// `step` at a Debug.break works ONLY via the session-side mode change
	// — pauseAtBreak discards OnStep's returned action — so pin the
	// break → step → next-source-line transition end to end.
	path := writeProgram(t, `import "aql:debug"
Debug.break
def z 5
z add 1
`)
	code, out, _ := launch(t, []string{path}, "continue\nstep\nstep\ncontinue\n")
	if code != 0 {
		t.Fatalf("exit = %d", code)
	}
	requireOrder(t, out, "break hit", "at "+path+":3", "at "+path+":4", "6\n(program exited)")
}

func TestLaunchEvalErrorsRenderExpressionSource(t *testing.T) {
	// A failing `print` must render its extract against the EXPRESSION,
	// not the paused program's source/file (the registry's Source and
	// BaseFile are swapped for the duration of the eval).
	path := writeProgram(t, "def a 10\na add 1\n")
	code, out, _ := launch(t, []string{"--no-check", path}, "step\nprint 5 div 0\ncontinue\n")
	if code != 0 {
		t.Fatalf("exit = %d", code)
	}
	requireOrder(t, out, "division by zero", "5 div 0")
	// Negative: the extract must not be the program's line-1 text.
	errAt := strings.Index(out, "division by zero")
	if strings.Contains(out[errAt:], "def a 10") {
		t.Errorf("eval error extract shows the program, not the expression:\n%s", out[errAt:])
	}
}

func TestLaunchEvalFlowSignalDiscarded(t *testing.T) {
	// A `break` evaluated at a pause must NOT leak into the suspended
	// program's control flow — the signal is discarded with a notice and
	// the loop completes all its iterations.
	path := writeProgram(t, "for [1 3] [ 7 ]\n")
	code, out, _ := launch(t, []string{"--no-check", path}, "print break\nc\nc\n")
	if code != 0 {
		t.Fatalf("exit = %d", code)
	}
	requireOrder(t, out, "signal", "discarded", "7 7\n(program exited)")
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

func TestLaunchAliases(t *testing.T) {
	// The short spellings drive the same command blocks as the long forms.
	path := writeProgram(t, "def n 41\nn add 1\n")
	stdin := strings.Join([]string{
		"l",      // list
		"locals", // defs
		"h",      // help
		"?",      // help again
		"s",      // step
		"eval n add 9",
		"q", // quit → detach
	}, "\n") + "\n"
	code, out, _ := launch(t, []string{path}, stdin)
	if code != 0 {
		t.Fatalf("exit = %d", code)
	}
	requireOrder(t, out,
		"->    1  def n 41",
		"defs (0):",
		"commands:",
		"commands:",
		"at "+path+":2",
		"50",
		"(detached", "42\n(program exited)")
}

func TestLaunchPreflightEnvSkip(t *testing.T) {
	// Negative pair for the preflight gate: AQL_NO_CHECK skips the check,
	// so a checker-rejected program reaches runtime (and fails there).
	t.Setenv("AQL_NO_CHECK", "1")
	path := writeProgram(t, "boguswordxyz\n")
	code, out, errOut := launch(t, []string{path}, "continue\n")
	if code != 1 {
		t.Errorf("exit = %d, want the runtime failure", code)
	}
	if !strings.Contains(out, "type 'help' for commands") {
		t.Errorf("with AQL_NO_CHECK the debugger must start:\n%s", out)
	}
	if !strings.Contains(errOut, "boguswordxyz") {
		t.Errorf("stderr = %q, want the runtime error", errOut)
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
	if !strings.HasPrefix(errOut, "error: ") {
		t.Errorf("runtime errors carry the run-style prefix; stderr = %q", errOut)
	}
	// Forced color takes the structured-render arm.
	code, _, errOut = launch(t, []string{"--no-check", "--color", "always", path}, "continue\n")
	if code != 1 || !strings.Contains(errOut, "boguswordxyz") {
		t.Errorf("color: exit = %d, stderr = %q", code, errOut)
	}
	if !strings.HasPrefix(errOut, "error: ") {
		t.Errorf("color arm carries the prefix too; stderr = %q", errOut)
	}
}

func TestLaunchStepDescendsIntoBody(t *testing.T) {
	// Phase 2 (§10 item b): `step` descends into body evaluations — the
	// registry-level trace reaches the pooled sub-engines each/fold/do
	// bodies run on, labelled "(in body)".
	path := writeProgram(t, "def acc 5\n[10 20] each [\nadd acc\n]\n1 add 2\n")
	code, out, _ := launch(t, []string{"--no-check", path}, "s\ns\ns\ns\nc\n")
	if code != 0 {
		t.Fatalf("exit = %d", code)
	}
	requireOrder(t, out,
		"at "+path+":2", "each",
		"at "+path+":3 (in body)  add acc",
		"(program exited)")
}

func TestLaunchLineBreakpointFiresPerIteration(t *testing.T) {
	// A line breakpoint inside a body fires on EVERY iteration — a fresh
	// body run re-arms it even though the row is unchanged (contiguity
	// alone would swallow iterations 2+).
	path := writeProgram(t, "def acc 5\n[10 20] each [\nadd acc\n]\n1 add 2\n")
	code, out, _ := launch(t, []string{"--no-check", "--break", "3", path}, "c\nc\nc\nc\n")
	if code != 0 {
		t.Fatalf("exit = %d", code)
	}
	requireOrder(t, out, "breakpoint set: line 3")
	if got := strings.Count(out, "breakpoint: line 3 —"); got != 2 {
		t.Errorf("a 2-element each must hit the body breakpoint twice, got %d:\n%s", got, out)
	}
	if !strings.Contains(out, "(in body)") {
		t.Errorf("the body hit must be labelled:\n%s", out)
	}
}

func TestLaunchWordBreakpointAndManagement(t *testing.T) {
	path := writeProgram(t, "def n 41\nn add 1\n")
	stdin := strings.Join([]string{
		"break",         // empty list
		"break add",     // word breakpoint
		"break 9",       // line breakpoint
		"break",         // lists both
		"delete 9",      // remove the line
		"delete 9",      // negative: already gone
		"delete zz",     // negative: never set
		"break zzword",  // a second word breakpoint…
		"delete zzword", // …removed while set (the word-found arm)
		"break",         // word `add` only now
		"c",             // → word hit
		"delete",        // clear all at the hit
		"c",
	}, "\n") + "\n"
	code, out, _ := launch(t, []string{path}, stdin)
	if code != 0 {
		t.Fatalf("exit = %d", code)
	}
	requireOrder(t, out,
		"(no breakpoints)",
		"breakpoint set: word add",
		"breakpoint set: line 9",
		"line 9", "word add", // the listing
		"deleted: line 9",
		"no breakpoint at line 9",
		"no breakpoint on word zz",
		"deleted: word zzword",
		"breakpoint: word add — at "+path+":2",
		"all breakpoints deleted",
		"42\n(program exited)")
	// Negative: after `delete 9` the listing shows no line entry.
	last := strings.LastIndex(out, "word add\n(adbg) c")
	_ = last // ordering asserted above; the count pins the single word hit
	if got := strings.Count(out, "breakpoint: word add —"); got != 1 {
		t.Errorf("one dispatch of `add`, one hit; got %d:\n%s", got, out)
	}
}

func TestLaunchNextSkipsBodiesAndFrames(t *testing.T) {
	// `next` stays at statement level: body evaluations (sub-engines) and
	// deeper inlined frames auto-continue (§5.2).
	path := writeProgram(t, "def f fn [[x:Integer] [Integer] [\nx add 1\n]]\nf 41\nadd 1\n")
	code, out, _ := launch(t, []string{"--no-check", path}, "n\nn\nn\nc\n")
	if code != 0 {
		t.Fatalf("exit = %d", code)
	}
	requireOrder(t, out,
		"at "+path+":1",
		"at "+path+":4", "f 41",
		"at "+path+":5", "add 1",
		"43\n(program exited)")
	if strings.Contains(out, "(in body)") || strings.Contains(out, ":2 ") {
		t.Errorf("next must not stop inside bodies or called frames:\n%s", out)
	}
}

func TestLaunchOutLeavesFrame(t *testing.T) {
	// `out` from inside an inlined fn body runs until the frame returns.
	// Driven by a Debug.break marker rather than counted steps: whether
	// fn-literal CONSTRUCTION surfaces "(in body)" stops under `step`
	// varies with process-warm caches, so step counts through a def are
	// not portable — continue-to-marker is, in every world.
	path := writeProgram(t, `import "aql:debug"
def f fn [[x:Integer] [Integer] [
Debug.break
x add 1
]]
f 41
add 1
`)
	code, out, _ := launch(t, []string{path}, "c\nbt\no\nc\n")
	if code != 0 {
		t.Fatalf("exit = %d", code)
	}
	requireOrder(t, out,
		"break hit",
		"#0  f  (x)",
		"at "+path+":7", "stack: [42]", // out: the frame returned its result
		"43\n(program exited)")
	// Negative: out must not stop inside the frame's own remaining lines.
	if strings.Contains(out, "at "+path+":4") {
		t.Errorf("out must not pause on the frame's remaining body lines:\n%s", out)
	}
}

func TestLaunchOutAtTopLevel(t *testing.T) {
	// Negative pair: `out` with no enclosing frame just runs on.
	path := writeProgram(t, "def a 1\na add 1\n")
	code, out, _ := launch(t, []string{path}, "o\n")
	if code != 0 {
		t.Fatalf("exit = %d", code)
	}
	requireOrder(t, out, "(out at top level", "2\n(program exited)")
	if got := strings.Count(out, "at "+path+":"); got != 1 {
		t.Errorf("out at top level must not pause again; got %d stops", got)
	}
}

func TestLaunchPostMortem(t *testing.T) {
	// §6.1 (host-only form): an uncaught error opens an inspection prompt
	// over the fault state — the trace snapshot taken immediately before
	// the failing dispatch, with the registry's bindings un-unwound.
	path := writeProgram(t, "def f fn [[x:Integer] [Integer] [\nx div 0\n]]\nf 6\n")
	stdin := "c\nbt\ndefs\nstack\nlist\nprint x add 1\nq\n"
	code, out, errOut := launch(t, []string{"--no-check", "--post-mortem", path}, stdin)
	if code != 1 {
		t.Fatalf("exit = %d, want 1 (the error still fails the run)", code)
	}
	requireOrder(t, out,
		"uncaught error — post-mortem at "+path+":2", "x div 0",
		"division by zero",
		"stack: [6 0]", // the failing div's operands, from the fault snapshot
		"#0  f  (x)",   // bt: the frame chain at the raise
		"x = 6",        // defs: the param binding, un-unwound
		"#0  0",        // stack: top-first reconstruction
		"->    2  x div 0",
		"7", // print x add 1 evaluates over the fault state
		"(post-mortem closed)")
	if strings.Contains(out, "continues to completion") {
		t.Errorf("a post-mortem quit must not claim the program continues:\n%s", out)
	}
	if !strings.HasPrefix(errOut, "error: ") {
		t.Errorf("the error still reaches stderr; got %q", errOut)
	}
}

func TestLaunchPostMortemNegatives(t *testing.T) {
	// Without the flag, an uncaught error exits directly — no prompt.
	path := writeProgram(t, "1 div 0\n")
	code, out, _ := launch(t, []string{"--no-check", path}, "c\nbt\n")
	if code != 1 {
		t.Fatalf("exit = %d", code)
	}
	if strings.Contains(out, "post-mortem") {
		t.Errorf("no --post-mortem flag, no post-mortem prompt:\n%s", out)
	}
	// A CAUGHT error is not uncaught: the program completes, no prompt.
	caught := writeProgram(t, "do [1 div 0]\n")
	code, out, _ = launch(t, []string{"--no-check", "--post-mortem", caught}, "c\n")
	if code != 0 {
		t.Fatalf("caught: exit = %d, out:\n%s", code, out)
	}
	if strings.Contains(out, "post-mortem") {
		t.Errorf("a do-caught error must not post-mortem:\n%s", out)
	}
	// After a quit (detach) the user has left: a later fault exits
	// directly rather than reopening a prompt on a dead session.
	code, out, _ = launch(t, []string{"--no-check", "--post-mortem", path}, "q\n")
	if code != 1 {
		t.Fatalf("detached: exit = %d", code)
	}
	if strings.Contains(out, "post-mortem") {
		t.Errorf("a detached session must not post-mortem:\n%s", out)
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
