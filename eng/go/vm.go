package eng

// The bytecode VM — the execution half of Stage 2 of
// design/aql-bytecode-plan.0.md, currently covering the Stage-1
// instruction set (straight-line PUSH_CONST / SWAP / CALL_NATIVE).
// Control-flow opcodes arrive with the rest of Stage 2.
//
// Termination note: all jumps are FORWARD (if-lowering produces no
// back-edges), and the VM enforces that, so a Program of N
// instructions still executes at most N steps — the interpreter's
// evaluation-limit parity obligation (plan R6 #27) remains
// structurally met. The step budget becomes load-bearing the moment
// the loop stage lands a back-edge and MUST ship in that change.

import "fmt"

// RunProgram executes a compiled Program against a registry and
// returns the residual value stack (bottom → top), matching what the
// interpreter's Run returns for the same source.
func RunProgram(p *Program, r *Registry) ([]Value, error) {
	if p == nil {
		return nil, fmt.Errorf("bytecode: nil program")
	}
	stack := make([]Value, 0, p.MaxStack)
	for pc := 0; pc < len(p.Code); pc++ {
		in := p.Code[pc]
		switch in.Op {
		case OpPushConst:
			stack = append(stack, p.Consts[in.Arg])
		case OpSwap:
			if len(stack) < 2 {
				return nil, vmInternalError(p, pc, "SWAP underflow")
			}
			stack[len(stack)-1], stack[len(stack)-2] = stack[len(stack)-2], stack[len(stack)-1]
		case OpCallNative:
			s := p.Sigs[in.Arg]
			n := len(s.Sig.Args)
			if len(stack) < n {
				return nil, vmInternalError(p, pc, "CALL_NATIVE underflow at "+s.Word)
			}
			// One argument convention: position 0 is the top of stack.
			args := make([]Value, n)
			for i := 0; i < n; i++ {
				args[i] = stack[len(stack)-1-i]
			}
			stack = stack[:len(stack)-n]
			results, err := s.Sig.Handler(args, r.Contexts.TopData(), nil, r)
			if err != nil {
				return nil, stampProgramPos(err, p, pc, r)
			}
			// Belt-and-braces: a handler that returns tape tokens (to
			// be re-stepped by the engine) must never have been
			// compiled — the emitter refuses fn-invoking and
			// code-splicing words. Fail loudly, never push tokens as
			// data.
			for _, rv := range results {
				if IsWord(rv) || IsMark(rv) || IsMove(rv) || IsForward(rv) ||
					IsOpenParen(rv) || IsSplice(rv) {
					return nil, vmInternalError(p, pc, "tape-coupled handler result at "+s.Word)
				}
			}
			stack = append(stack, results...)
		case OpJmp:
			if int(in.Arg) <= pc {
				return nil, vmInternalError(p, pc, "backward jump (loop stage not landed)")
			}
			pc = int(in.Arg) - 1
		case OpJmpIfFalse:
			if len(stack) < 1 {
				return nil, vmInternalError(p, pc, "JMP_IF_FALSE underflow")
			}
			cond := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if !CoerceBoolean(cond) {
				if int(in.Arg) <= pc {
					return nil, vmInternalError(p, pc, "backward jump (loop stage not landed)")
				}
				pc = int(in.Arg) - 1
			}
		default:
			return nil, vmInternalError(p, pc, "unknown opcode")
		}
	}
	return stack, nil
}

// stampProgramPos attaches the source position from the pc → SrcPos
// map to a handler error that lacks one, so compiled-mode errors
// render with the same source anchor the interpreter would give.
func stampProgramPos(err error, p *Program, pc int, r *Registry) error {
	ae, ok := err.(*AqlError)
	if !ok || pc >= len(p.Debug) {
		return err
	}
	if ae.Row == 0 {
		ae.Row = p.Debug[pc].Row
		ae.Col = p.Debug[pc].Col
	}
	if r != nil && ae.fullSource == "" {
		ae.fullSource = r.Source
	}
	return ae
}

// vmInternalError reports a stack-discipline violation — impossible
// for programs the emitter's Finalize verified; loud, never silent,
// if a future change breaks the invariant.
func vmInternalError(p *Program, pc int, msg string) error {
	pos := SrcPos{}
	if pc < len(p.Debug) {
		pos = p.Debug[pc]
	}
	return fmt.Errorf("bytecode: internal: %s (pc=%d, src %d:%d)", msg, pc, pos.Row, pos.Col)
}
