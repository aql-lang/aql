// Package debugger implements the interactive session behind
// `aql debug <file.aql>` (design/AQL-DEBUGGER.0.md §5): a host-side
// realisation of the StepController contract that pauses the engine's
// per-step trace between SOURCE LINES, renders the paused state, and
// drives a gdb/pdb-style command prompt.
//
// Phase 1 scope (§10): single-step (source-line coalesced), continue,
// quit (detach — the engine has no mid-run interrupt, ADR-005), inline
// Debug.break / break-when stops, and stack / bt / defs / print / list
// inspection at a pause. One Session object owns all step and pause
// semantics so every front end — the TTY prompt now, a DAP adapter later
// — shares them (§3 principle 2).
package debugger

import (
	"bufio"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/aql-lang/aql/lang/go/capabilities"
	"github.com/aql-lang/aql/lang/go/native"
)

// stepMode is the session's stepping state.
type stepMode int

const (
	// modeStep pauses at the next new source line (the zero value: a
	// session starts paused at the program's first positioned token),
	// descending into body evaluations (§5.1).
	modeStep stepMode = iota
	// modeNext pauses at the next new source line at or above the
	// recorded frame depth — deeper (called) frames and body
	// evaluations auto-continue (§5.2 "over").
	modeNext
	// modeOut runs until the recorded frame is left — the first
	// positioned token strictly above it (§5.2).
	modeOut
	// modeContinue runs to the next breakpoint (or completion).
	modeContinue
	// modeDetached never pauses again; the program drains to completion —
	// `quit` detaches rather than killing (§5.4, ADR-005: no control-flow
	// panics, no mid-run interruption seam).
	modeDetached
)

// maxStackShown caps the one-line stack summary rendered at each pause;
// the `stack` command shows every entry.
const maxStackShown = 8

// detachNotice is printed whenever the session detaches (quit or EOF) —
// it must say the program CONTINUES, because that is what detach means.
const detachNotice = "(detached — program continues to completion)"

// Config wires a Session's I/O and source identity.
type Config struct {
	In     io.Reader // debugger command input (a TTY, a pipe, or a --script file)
	Out    io.Writer // debugger UI output
	File   string    // program path, shown in pause locations
	Source string    // program source text, for `list` and location lines
	Echo   bool      // echo each command after the prompt (script/batch mode)
}

// Session is the interactive debug session. It implements
// capabilities.DebugOps and StepController, so the same object that
// drives the per-step trace also receives the one-shot pauses the
// Debug.break handler raises and the per-step frames of any Debug.step
// the debugged program runs itself.
type Session struct {
	reg   *native.Registry
	in    *bufio.Scanner
	out   io.Writer
	file  string
	lines []string
	echo  bool

	mode    stepMode
	prevRow int // row of the last positioned token stepped (line coalescing)
	curRow  int // row of the current pause (0 = unknown)

	// baseDepth is the live-frame depth recorded when `next`/`out` was
	// issued; the pause rule compares against it (§5.2).
	baseDepth int

	// lineBPs / wordBPs are the prompt-settable breakpoints (§7): pause
	// on entering a source line, or when a named word is about to
	// dispatch. They fire in every mode except detached — including
	// inside body evaluations (the sub-engine descent).
	lineBPs map[int]bool
	wordBPs map[string]bool

	// Pause context: the whole-tape snapshot from the most recent trace
	// fire, for `bt`. Storing the reference is free — the engine already
	// allocated the snapshot for the trace call.
	curStack   []native.Value
	curPointer int

	// inPrompt guards re-entrant pauses: a `print` expression that itself
	// hits Debug.break (or runs Debug.step) must never nest the prompt.
	inPrompt bool

	// baseDefs is the def-table depth snapshot taken at launch, so `defs`
	// can show the PROGRAM's bindings (new or deepened since) rather than
	// the registry's full native/def table; `defs all` shows everything.
	baseDefs map[string]int

	// dead marks a post-mortem session: the program has already
	// terminated, so detach must not claim it "continues".
	dead bool

	// mu serializes the pause machinery. The per-step trace runs on the
	// program's goroutine, but the installed DebugOps is shared registry
	// state, so a concurrent fork (a timer body hitting Debug.break) can
	// call OnStep from another goroutine. onTrace holds mu across a
	// pause; OnStep only TryLocks — a caller that cannot acquire the
	// session (another pause is at the prompt, or the pause was raised by
	// the prompt's own `print` evaluation) continues immediately, so
	// exactly one goroutine ever reads the command input.
	mu sync.Mutex

	// clearTrace disarms the engine's per-step trace; set by RunProgram.
	// Called on detach so the draining program stops paying the per-step
	// snapshot the trace forces (detach means "run at full speed").
	clearTrace func()
}

// New builds a Session over the registry the program will run in.
func New(reg *native.Registry, cfg Config) *Session {
	in := bufio.NewScanner(cfg.In)
	// Lift the Scanner's default 64KiB token cap so a long `print`
	// expression is a working command, not a silent detach.
	in.Buffer(make([]byte, 0, 64*1024), 1<<20)
	return &Session{
		reg:     reg,
		in:      in,
		out:     cfg.Out,
		file:    cfg.File,
		lines:   strings.Split(cfg.Source, "\n"),
		echo:    cfg.Echo,
		lineBPs: make(map[int]bool),
		wordBPs: make(map[string]bool),
	}
}

// Controller implements capabilities.DebugOps: the session is its own
// step controller.
func (s *Session) Controller() capabilities.StepController { return s }

// RunProgram executes the parsed program under the session: installs the
// session as the host DebugOps (filling the socket Debug.break pauses
// reach), arms the per-step trace, and runs the tokens on a fresh
// top-level interpreter engine — the debugger forces the interpreter
// path, exactly as Debug.step does (§5.5: the compiled VM does not fire
// the per-token trace).
func (s *Session) RunProgram(tokens []native.Value, source string) ([]native.Value, error) {
	// The session borrows the registry for the run: restore the prior
	// DebugOps (usually none) and Source on the way out, so a host that
	// reuses the registry afterwards doesn't keep prompting a dead session.
	prevOps, hadOps := native.EffectiveDebugOps(s.reg)
	prevSrc := s.reg.Source
	native.SetHostDebugOps(s.reg, s)
	s.reg.Source = source
	s.baseDefs = s.reg.Defs.Snapshot()
	// The registry-level hook is what descends into body evaluations
	// (each/fold/do bodies, paren groups, same-registry fn bodies): every
	// engine the registry spawns starts with it (eng SetDebugTrace). The
	// top-level engine gets the MAIN callback afterwards, so the session
	// can tell its own tape's fires from a body's.
	s.reg.SetDebugTrace(s.onSubTrace)
	engine := native.NewTop(s.reg)
	engine.SetSource(source)
	engine.SetTrace(s.onTrace)
	s.clearTrace = func() {
		engine.SetTrace(nil)
		s.reg.SetDebugTrace(nil)
	}
	defer func() {
		s.reg.SetDebugTrace(nil)
		s.clearTrace = nil
		s.reg.Source = prevSrc
		if hadOps {
			native.SetHostDebugOps(s.reg, prevOps)
		} else {
			native.RemoveHostDebugOps(s.reg)
		}
	}()
	return engine.Run(tokens)
}

// onTrace receives the MAIN engine's per-step fires; onSubTrace — the
// registry-level hook — receives every OTHER engine's (body
// evaluations, paren groups, same-registry fn bodies). The split is
// what gives the modes their semantics: `step` descends into bodies,
// `next`/`out` treat them as part of the line, and breakpoints fire in
// both.
func (s *Session) onTrace(_ int, pointer int, stack []native.Value, note string) {
	s.trace(pointer, stack, note, false, false)
}

func (s *Session) onSubTrace(step int, pointer int, stack []native.Value, note string) {
	// step 0 marks a FRESH body evaluation (each pooled run restarts its
	// counter): line breakpoints re-arm there, so a loop body that stays
	// on one source row still breaks on every iteration.
	s.trace(pointer, stack, note, true, step == 0)
}

// trace is the debug event loop's pause decision. It coalesces raw
// engine steps to SOURCE LINES — one stop per contiguous run of a Row,
// skipping position-less synthetic tokens (§5.1) — so `step` means
// "advance one source line", not one tape token.
func (s *Session) trace(pointer int, stack []native.Value, note string, sub, freshRun bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.curStack, s.curPointer = stack, pointer
	if s.mode == modeDetached || s.inPrompt {
		return
	}
	if strings.HasPrefix(note, "for next ") {
		// A `for` iteration boundary is a line boundary: without this, a
		// single-line loop body coalesces ALL its iterations into the
		// first stop (every token shares one Row, contiguously).
		s.prevRow = 0
	}
	row := 0
	if pointer >= 0 && pointer < len(stack) {
		row = stack[pointer].Pos().Row
	}
	prev := s.prevRow
	if row != 0 {
		s.prevRow = row
	}
	newLine := row != 0 && row != prev

	// Breakpoints pause in every mode: a line breakpoint once per line
	// entry, a word breakpoint whenever the word is about to dispatch —
	// inside body evaluations too (the sub-engine descent).
	hit := ""
	switch {
	case row != 0 && s.lineBPs[row] && (newLine || freshRun):
		hit = fmt.Sprintf("breakpoint: line %d", row)
	case len(s.wordBPs) > 0:
		if w, ok := wordNameAt(stack, pointer); ok && s.wordBPs[w] {
			hit = "breakpoint: word " + w
		}
	}
	if hit == "" {
		switch s.mode {
		case modeStep:
			if !newLine {
				return
			}
		case modeNext:
			// A body evaluation is part of its line; a deeper inlined
			// frame is a call in progress — both auto-continue.
			if sub || !newLine || liveFrameDepth(stack, pointer) > s.baseDepth {
				return
			}
		case modeOut:
			if sub || row == 0 || liveFrameDepth(stack, pointer) >= s.baseDepth {
				return
			}
		default: // modeContinue: only breakpoints pause
			return
		}
	}
	s.curRow = row
	where := ""
	if sub {
		where = " (in body)"
	}
	if hit != "" {
		fmt.Fprintf(s.out, "%s — at %s:%s%s  %s\n", hit, s.file, rowLabel(row), where, s.sourceLine(row))
	} else {
		fmt.Fprintf(s.out, "at %s:%s%s  %s\n", s.file, rowLabel(row), where, s.sourceLine(row))
	}
	s.showStack()
	s.prompt()
}

// rowLabel renders a 1-based source row, "?" when unknown.
func rowLabel(row int) string {
	if row == 0 {
		return "?"
	}
	return strconv.Itoa(row)
}

// wordNameAt returns the name of the Word about to execute, if any.
func wordNameAt(stack []native.Value, pointer int) (string, bool) {
	if pointer < 0 || pointer >= len(stack) || !native.IsWord(stack[pointer]) {
		return "", false
	}
	// IsWord guarantees the payload; AsWord's error arm is unreachable.
	w, _ := native.AsWord(stack[pointer])
	return w.Name, true
}

// liveFrameDepth counts the fn frames open at the pointer — one O(n)
// pass tracking paren nesting, counting only frame-marked opens
// (IsFrameOpen) still unmatched when the pointer is reached.
func liveFrameDepth(stack []native.Value, pointer int) int {
	if pointer > len(stack) {
		pointer = len(stack)
	}
	depth := 0
	var frames []bool
	for i := 0; i < pointer; i++ {
		switch {
		case native.IsOpenParen(stack[i]):
			isFrame := native.IsFrameOpen(stack[i])
			frames = append(frames, isFrame)
			if isFrame {
				depth++
			}
		case native.IsCloseParen(stack[i]):
			if n := len(frames); n > 0 {
				if frames[n-1] {
					depth--
				}
				frames = frames[:n-1]
			}
		}
	}
	return depth
}

// OnStep implements capabilities.StepController. It receives the pauses
// that arrive OUTSIDE the session's own trace: the one-shot pause the
// Debug.break / break-when handler raises (Step == -1, AtBreak), and the
// per-step frames of any Debug.step the debugged program runs itself.
// These can arrive from another goroutine (a fork's body hitting a
// break), so the session is only TryLock-acquired: a pause that loses
// the race — or one raised by the prompt's own `print` evaluation —
// continues immediately rather than interleaving two prompts on one
// command input.
func (s *Session) OnStep(f capabilities.StepFrame) capabilities.StepAction {
	if !s.mu.TryLock() {
		return capabilities.StepContinue
	}
	defer s.mu.Unlock()
	if s.mode == modeDetached {
		// Detached: tell any inner stepper to stand down too.
		return capabilities.StepQuit
	}
	if s.inPrompt {
		// A pause raised while evaluating a `print` expression: never nest
		// the prompt — let the evaluation run through.
		return capabilities.StepContinue
	}
	s.curRow = f.Row
	if f.AtBreak {
		fmt.Fprintf(s.out, "break hit (Debug.break)%s\n", frameLoc(f))
	} else {
		fmt.Fprintf(s.out, "step %d%s\n", f.Step, frameLoc(f))
	}
	s.showStack()
	switch s.prompt() {
	case modeContinue:
		return capabilities.StepContinue
	case modeDetached:
		return capabilities.StepQuit
	}
	return capabilities.StepInto
}

// frameLoc renders the optional source location of a StepFrame.
func frameLoc(f capabilities.StepFrame) string {
	if f.Row == 0 {
		return ""
	}
	name := f.File
	if name == "" {
		name = "?"
	}
	return fmt.Sprintf(" at %s:%d", name, f.Row)
}

// prompt reads and dispatches commands until an action command (step /
// continue / quit) or end-of-input decides how execution proceeds,
// returning the session's resulting mode. Inspection commands loop.
func (s *Session) prompt() stepMode {
	s.inPrompt = true
	defer func() { s.inPrompt = false }()
	for {
		fmt.Fprint(s.out, "(adbg) ")
		if !s.in.Scan() {
			// End of the command input: detach (§5.4) — the program runs
			// to completion. Say WHY: a read failure (an oversized line,
			// an I/O error) must not masquerade as a clean Ctrl-D or a
			// drained --script file.
			if err := s.in.Err(); err != nil {
				fmt.Fprintf(s.out, "\ncommand input error: %s\n", err)
			}
			fmt.Fprintln(s.out, "\n"+s.detachMsg())
			return s.detach()
		}
		line := strings.TrimSpace(s.in.Text())
		if s.echo {
			fmt.Fprintln(s.out, line)
		}
		cmd, arg := splitCommand(line)
		switch cmd {
		case "":
			continue
		case "step", "s":
			s.mode = modeStep
			return s.mode
		case "next", "n":
			s.mode = modeNext
			s.baseDepth = liveFrameDepth(s.curStack, s.curPointer)
			return s.mode
		case "out", "o":
			s.mode = modeOut
			s.baseDepth = liveFrameDepth(s.curStack, s.curPointer)
			if s.baseDepth == 0 {
				fmt.Fprintln(s.out, "(out at top level runs to the next breakpoint or completion)")
			}
			return s.mode
		case "continue", "c":
			s.mode = modeContinue
			return s.mode
		case "break", "b":
			if arg == "" {
				s.listBreaks()
			} else {
				fmt.Fprintln(s.out, "  "+s.SetBreak(arg))
			}
		case "delete":
			fmt.Fprintln(s.out, "  "+s.deleteBreak(arg))
		case "quit", "q":
			fmt.Fprintln(s.out, s.detachMsg())
			return s.detach()
		case "stack":
			s.renderFullStack()
		case "bt":
			s.renderBacktrace()
		case "defs", "locals":
			s.renderDefs(arg == "all")
		case "print", "p", "eval":
			s.evalExpr(arg)
		case "list", "l":
			s.renderList()
		case "help", "h", "?":
			s.renderHelp()
		default:
			fmt.Fprintf(s.out, "unknown command %q — try 'help'\n", cmd)
		}
	}
}

// PostMortem opens an inspection prompt over the FAULT state after
// RunProgram returned an uncaught error (§6.1's post-mortem entry,
// host-only): the per-step trace fired immediately BEFORE the failing
// dispatch, so the retained snapshot (curStack/curPointer) is exactly
// the program state at the raise, and an error return performs no
// frame unwind, so the registry still holds the fault's bindings.
// Every inspection command works; the program has already terminated,
// so any action command (step/continue/quit) simply ends the session.
// A detached session gets no post-mortem — the user already left.
func (s *Session) PostMortem(runErr error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.mode == modeDetached {
		return
	}
	s.dead = true
	loc := ""
	if s.curPointer >= 0 && s.curPointer < len(s.curStack) {
		if row := s.curStack[s.curPointer].Pos().Row; row != 0 {
			s.curRow = row
			loc = fmt.Sprintf(" at %s:%d  %s", s.file, row, s.sourceLine(row))
		}
	}
	fmt.Fprintf(s.out, "uncaught error — post-mortem%s\n", loc)
	fmt.Fprintf(s.out, "  %s\n", runErr)
	fmt.Fprintln(s.out, "  (the program has terminated; inspect its state — step/continue/quit exit)")
	s.showStack()
	s.prompt()
}

// dataStack resolves the data stack an inspection command should show:
// the live engine's (CurrentStack) while the program runs, or — in a
// post-mortem, when no engine is running — a reconstruction from the
// retained fault snapshot's resolved region. ok is false only when
// there is neither (a session that never ran).
func (s *Session) dataStack() ([]native.Value, bool) {
	if vals, ok := s.reg.CurrentStack(); ok {
		return vals, true
	}
	if s.curStack == nil {
		return nil, false
	}
	n := s.curPointer
	if n > len(s.curStack) {
		n = len(s.curStack)
	}
	vals := make([]native.Value, 0, n)
	for _, v := range s.curStack[:n] {
		if isEngineMarker(v) {
			continue
		}
		vals = append(vals, v)
	}
	return vals, true
}

// isEngineMarker reports whether a resolved-region tape value is engine
// bookkeeping rather than program data — the same classes the live
// CurrentStack view filters.
func isEngineMarker(v native.Value) bool {
	return native.IsWord(v) || native.IsForward(v) || native.IsMark(v) ||
		native.IsMove(v) || native.IsOpenParen(v) || native.IsCloseParen(v) ||
		native.IsEnd(v) || native.IsDefCleanup(v) || native.IsReturnCheck(v)
}

// detach puts the session in its terminal mode and disarms the per-step
// trace, so the draining program stops paying the whole-tape snapshot
// the armed trace forces every step — detach means "run at full speed".
// Clearing the trace from inside the callback is safe: the step loop
// re-checks it each iteration. Nil-guarded for a session that never ran
// a program (a bare prompt unit test).
func (s *Session) detach() stepMode {
	s.mode = modeDetached
	if s.clearTrace != nil {
		s.clearTrace()
	}
	return s.mode
}

// detachMsg is the leave-the-session notice: a running program drains
// to completion; a post-mortem one is already gone.
func (s *Session) detachMsg() string {
	if s.dead {
		return "(post-mortem closed)"
	}
	return detachNotice
}

// SetBreak parses and installs one breakpoint spec — "N" or "file:N"
// (a source line) or a word name — returning a confirmation line. The
// prompt's `break <spec>` and the launch's `--break <spec>` share it.
func (s *Session) SetBreak(spec string) string {
	if n, ok := parseLineSpec(spec); ok {
		s.lineBPs[n] = true
		return fmt.Sprintf("breakpoint set: line %d", n)
	}
	s.wordBPs[spec] = true
	return "breakpoint set: word " + spec
}

// deleteBreak removes one breakpoint (by the same spec grammar), or —
// with an empty spec — every breakpoint.
func (s *Session) deleteBreak(spec string) string {
	if spec == "" {
		s.lineBPs = make(map[int]bool)
		s.wordBPs = make(map[string]bool)
		return "all breakpoints deleted"
	}
	if n, ok := parseLineSpec(spec); ok {
		if !s.lineBPs[n] {
			return fmt.Sprintf("no breakpoint at line %d", n)
		}
		delete(s.lineBPs, n)
		return fmt.Sprintf("deleted: line %d", n)
	}
	if !s.wordBPs[spec] {
		return "no breakpoint on word " + spec
	}
	delete(s.wordBPs, spec)
	return "deleted: word " + spec
}

// parseLineSpec reads "N" or "anything:N" as a positive source line.
func parseLineSpec(spec string) (int, bool) {
	numPart := spec
	if i := strings.LastIndex(spec, ":"); i >= 0 {
		numPart = spec[i+1:]
	}
	n, err := strconv.Atoi(numPart)
	return n, err == nil && n > 0
}

// listBreaks renders the installed breakpoints.
func (s *Session) listBreaks() {
	lines := make([]int, 0, len(s.lineBPs))
	for n := range s.lineBPs {
		lines = append(lines, n)
	}
	sort.Ints(lines)
	words := make([]string, 0, len(s.wordBPs))
	for w := range s.wordBPs {
		words = append(words, w)
	}
	sort.Strings(words)
	if len(lines) == 0 && len(words) == 0 {
		fmt.Fprintln(s.out, "  (no breakpoints)")
		return
	}
	for _, n := range lines {
		fmt.Fprintf(s.out, "  line %d\n", n)
	}
	for _, w := range words {
		fmt.Fprintf(s.out, "  word %s\n", w)
	}
}

// splitCommand splits a prompt line into its command word and argument.
func splitCommand(line string) (cmd, arg string) {
	cmd, arg, _ = strings.Cut(line, " ")
	return cmd, strings.TrimSpace(arg)
}

// showStack renders the one-line data-stack summary shown at each pause.
func (s *Session) showStack() {
	if vals, ok := s.dataStack(); ok {
		fmt.Fprintf(s.out, "  stack: %s\n", summarizeStack(vals))
	}
}

// summarizeStack renders a data stack on one line, truncating to the
// top maxStackShown entries.
func summarizeStack(vals []native.Value) string {
	parts := make([]string, 0, len(vals))
	for _, v := range vals {
		parts = append(parts, native.FormatForPrint(v))
	}
	if len(parts) > maxStackShown {
		parts = append(
			[]string{fmt.Sprintf("… %d more", len(parts)-maxStackShown)},
			parts[len(parts)-maxStackShown:]...)
	}
	return "[" + strings.Join(parts, " ") + "]"
}

// renderFullStack renders every data-stack entry, top (#0) first.
func (s *Session) renderFullStack() {
	vals, ok := s.dataStack()
	if !ok || len(vals) == 0 {
		fmt.Fprintln(s.out, "  (stack empty)")
		return
	}
	for i := len(vals) - 1; i >= 0; i-- {
		fmt.Fprintf(s.out, "  #%d  %s\n", len(vals)-1-i, native.FormatForPrint(vals[i]))
	}
}

// frameInfo is one live inline fn frame reconstructed from the tape.
type frameInfo struct {
	name     string
	installs []string
}

// renderBacktrace reconstructs the live inline fn-frame chain from the
// tape snapshot (§5.3). Sub-engine frames (module fns, higher-order
// bodies) are invisible to the trace in Phase 1 and so absent here — the
// documented blindness (§3 principle 3).
func (s *Session) renderBacktrace() {
	frames := liveFrames(s.curStack, s.curPointer)
	if len(frames) == 0 {
		fmt.Fprintln(s.out, "  (top level — no fn frames)")
		return
	}
	for i := len(frames) - 1; i >= 0; i-- { // innermost first, gdb-style
		f := frames[i]
		line := fmt.Sprintf("  #%d  %s", len(frames)-1-i, f.name)
		if len(f.installs) > 0 {
			line += "  (" + strings.Join(f.installs, " ") + ")"
		}
		fmt.Fprintln(s.out, line)
	}
}

// liveFrames scans the tape snapshot for fn frames opened below the
// pointer whose matching close paren is at/after it — calls still in
// flight, mirroring the liveness rule of the engine's own
// unwindLiveFrames (eng/go/fn_frame.go).
func liveFrames(stack []native.Value, pointer int) []frameInfo {
	if pointer > len(stack) {
		pointer = len(stack)
	}
	var out []frameInfo
	for i := 0; i < pointer; i++ {
		if !native.IsFrameOpen(stack[i]) {
			continue
		}
		depth, closed := 0, -1
		for j := i; j < len(stack); j++ {
			switch {
			case native.IsOpenParen(stack[j]):
				depth++
			case native.IsCloseParen(stack[j]):
				depth--
			}
			if depth == 0 {
				closed = j
				break
			}
		}
		if closed >= 0 && closed < pointer {
			continue // the frame already closed — not live
		}
		if info, err := native.AsFrameOpen(stack[i]); err == nil && info.Meta != nil {
			out = append(out, frameInfo{name: info.Meta.Name, installs: info.Meta.InstallNames})
		}
	}
	return out
}

// renderDefs lists current value/type bindings, sorted. By default only
// the PROGRAM's bindings show — names new or deepened since the launch
// snapshot (params, locals, its defs and imports) — because the full def
// table also mirrors every native word; `defs all` lifts the filter.
func (s *Session) renderDefs(all bool) {
	names := s.reg.Defs.Names()
	sort.Strings(names)
	shown := make([]string, 0, len(names))
	for _, n := range names {
		if all || s.reg.Defs.Depth(n) > s.baseDefs[n] {
			shown = append(shown, n)
		}
	}
	fmt.Fprintf(s.out, "  defs (%d):\n", len(shown))
	for _, n := range shown {
		if v, ok := s.reg.Defs.Top(n); ok {
			fmt.Fprintf(s.out, "  %s = %s\n", n, native.FormatForPrint(v))
		}
	}
}

// evalExpr parses and runs an expression against the paused program's
// registry in a child interpreter engine — the debugserve eval pattern:
// the expression sees the program's defs and stores, its own defs
// persist (like any eval), and it can never exceed the program's grants
// (§8.1 / §9).
func (s *Session) evalExpr(src string) {
	if src == "" {
		fmt.Fprintln(s.out, "usage: print <expr>")
		return
	}
	if s.reg.ParseFunc == nil {
		fmt.Fprintln(s.out, "  (no parser configured)")
		return
	}
	toks, err := s.reg.ParseFunc(src)
	if err != nil {
		fmt.Fprintf(s.out, "  parse error: %s\n", err)
		return
	}
	// Error positions must render against the EXPRESSION, not the paused
	// program: the registry's Source/BaseFile are the program's (AqlError
	// bakes r.Source in at construction), so swap them to the expression
	// for the duration of the child run — the module loader's established
	// swap/restore shape. FlowCtrl is snapshotted too: a bare `break` /
	// `continue` in the expression would otherwise leak the signal into
	// the suspended program's loop when it resumes.
	savedSrc, savedFile, savedFlow := s.reg.Source, s.reg.BaseFile, s.reg.FlowCtrl
	s.reg.Source, s.reg.BaseFile = src, ""
	// Suppress the registry-level debug hook for the eval: its child
	// engines (paren groups, bodies) must neither pause nor clobber the
	// retained pause context. Re-armed only if a program run armed it.
	if s.clearTrace != nil {
		s.reg.SetDebugTrace(nil)
		defer s.reg.SetDebugTrace(s.onSubTrace)
	}
	engine := native.New(s.reg)
	engine.SetTrace(nil)
	engine.SetSource(src)
	res, err := engine.Run(toks)
	s.reg.Source, s.reg.BaseFile = savedSrc, savedFile
	if s.reg.FlowCtrl != savedFlow {
		leaked := s.reg.FlowCtrl
		s.reg.FlowCtrl = savedFlow
		fmt.Fprintf(s.out, "  (expression raised a %v signal with no enclosing loop — discarded)\n", leaked)
		return
	}
	if err != nil {
		fmt.Fprintf(s.out, "  error: %s\n", err)
		return
	}
	if len(res) == 0 {
		fmt.Fprintln(s.out, "  (no result)")
		return
	}
	parts := make([]string, len(res))
	for i, v := range res {
		parts[i] = native.FormatForPrint(v)
	}
	fmt.Fprintln(s.out, "  "+strings.Join(parts, " "))
}

// renderList shows the source around the current pause line.
func (s *Session) renderList() {
	if s.curRow <= 0 || s.curRow > len(s.lines) {
		fmt.Fprintln(s.out, "  (source line unknown)")
		return
	}
	lo, hi := s.curRow-2, s.curRow+2
	if lo < 1 {
		lo = 1
	}
	if hi > len(s.lines) {
		hi = len(s.lines)
	}
	for n := lo; n <= hi; n++ {
		marker := "   "
		if n == s.curRow {
			marker = "-> "
		}
		fmt.Fprintf(s.out, "  %s%4d  %s\n", marker, n, s.lines[n-1])
	}
}

// sourceLine returns the trimmed source text of a 1-based line.
func (s *Session) sourceLine(row int) string {
	if row >= 1 && row <= len(s.lines) {
		return strings.TrimSpace(s.lines[row-1])
	}
	return ""
}

// renderHelp lists the Phase-1 command set.
func (s *Session) renderHelp() {
	fmt.Fprint(s.out, `commands:
  step, s        run to the next source line (descends into bodies)
  next, n        like step, but run through deeper frames and bodies
  out, o         run until the current fn frame returns
  continue, c    run to the next breakpoint (or completion)
  break <spec>   set a breakpoint: a line ('break 12'), or a word
                 ('break add'); bare 'break' lists them
  delete [spec]  delete one breakpoint (bare 'delete' clears all)
  quit, q        detach — the program continues to completion
  stack          show every data-stack entry (top first)
  bt             backtrace of live inline fn frames
  defs, locals   the program's bindings in scope ('defs all' for every binding)
  print <expr>   evaluate an expression in the paused program's scope
  list, l        source around the current line
  help, h, ?     this help
`)
}
