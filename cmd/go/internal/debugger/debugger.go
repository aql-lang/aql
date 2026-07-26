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
	"strings"

	"github.com/aql-lang/aql/lang/go/capabilities"
	"github.com/aql-lang/aql/lang/go/native"
)

// stepMode is the session's stepping state.
type stepMode int

const (
	// modeStep pauses at the next new source line (the zero value: a
	// session starts paused at the program's first positioned token).
	modeStep stepMode = iota
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
}

// New builds a Session over the registry the program will run in.
func New(reg *native.Registry, cfg Config) *Session {
	return &Session{
		reg:   reg,
		in:    bufio.NewScanner(cfg.In),
		out:   cfg.Out,
		file:  cfg.File,
		lines: strings.Split(cfg.Source, "\n"),
		echo:  cfg.Echo,
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
	native.SetHostDebugOps(s.reg, s)
	s.reg.Source = source
	s.baseDefs = s.reg.Defs.Snapshot()
	engine := native.NewTop(s.reg)
	engine.SetSource(source)
	engine.SetTrace(s.onTrace)
	return engine.Run(tokens)
}

// onTrace is the per-step TraceCallback — the debug event loop. It
// coalesces raw engine steps to SOURCE LINES: one stop per contiguous
// run of a Row, skipping position-less synthetic tokens (§5.1) — so
// `step` means "advance one source line", not one tape token.
func (s *Session) onTrace(_ int, pointer int, stack []native.Value, _ string) {
	s.curStack, s.curPointer = stack, pointer
	if s.mode == modeDetached || s.inPrompt {
		return
	}
	row := 0
	if pointer >= 0 && pointer < len(stack) {
		row = stack[pointer].Pos().Row
	}
	prev := s.prevRow
	if row != 0 {
		s.prevRow = row
	}
	if s.mode != modeStep || row == 0 || row == prev {
		// Fast path: continuing, a synthetic token, or the same source line.
		return
	}
	s.curRow = row
	fmt.Fprintf(s.out, "at %s:%d  %s\n", s.file, row, s.sourceLine(row))
	s.showStack()
	s.prompt()
}

// OnStep implements capabilities.StepController. It receives the pauses
// that arrive OUTSIDE the session's own trace: the one-shot pause the
// Debug.break / break-when handler raises (Step == -1, AtBreak), and the
// per-step frames of any Debug.step the debugged program runs itself.
func (s *Session) OnStep(f capabilities.StepFrame) capabilities.StepAction {
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
			// EOF on the command input (Ctrl-D, or a drained --script
			// file): detach (§5.4) — the program runs to completion.
			fmt.Fprintln(s.out, "\n"+detachNotice)
			s.mode = modeDetached
			return s.mode
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
		case "continue", "c":
			s.mode = modeContinue
			return s.mode
		case "quit", "q":
			fmt.Fprintln(s.out, detachNotice)
			s.mode = modeDetached
			return s.mode
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

// splitCommand splits a prompt line into its command word and argument.
func splitCommand(line string) (cmd, arg string) {
	cmd, arg, _ = strings.Cut(line, " ")
	return cmd, strings.TrimSpace(arg)
}

// showStack renders the one-line data-stack summary shown at each pause.
func (s *Session) showStack() {
	if vals, ok := s.reg.CurrentStack(); ok {
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
	vals, ok := s.reg.CurrentStack()
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
	res, err := native.New(s.reg).Run(toks)
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
		parts[i] = v.String()
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
  step, s        run to the next source line
  continue, c    run to the next breakpoint (or completion)
  quit, q        detach — the program continues to completion
  stack          show every data-stack entry (top first)
  bt             backtrace of live inline fn frames
  defs, locals   the program's bindings in scope ('defs all' for every binding)
  print <expr>   evaluate an expression in the paused program's scope
  list, l        source around the current line
  help, h, ?     this help
`)
}
