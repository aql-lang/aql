package eng

// The bytecode VM — the execution half of Stage 2 of
// design/aql-bytecode-plan.0.md, currently covering the Stage-1
// instruction set (straight-line PUSH_CONST / SWAP / CALL_NATIVE).
// Control-flow opcodes arrive with the rest of Stage 2.
//
// Termination and resource parity (plan R6 #27): the only back-edge
// the emitter produces is a counted loop's trailing JMP to its
// FOR_NEXT, and the VM enforces exactly that shape — so every loop
// is bounded by its popped count, and an emitted body nets one value
// per iteration. Unbounded value accumulation (`for huge [i]`) hits
// the same growth ceiling the tape enforces: the VM stack's ceiling
// is computed from the registry's TapeConfig and overflowing it
// raises the interpreter's tape_exhausted taxonomy.

import "fmt"

// RunProgram executes a compiled Program against a registry and
// returns the residual value stack (bottom → top), matching what the
// interpreter's Run returns for the same source.
func RunProgram(p *Program, r *Registry) ([]Value, error) {
	if p == nil {
		return nil, fmt.Errorf("bytecode: nil program")
	}
	ceiling := vmStackCeiling(r)
	stack := make([]Value, 0, p.MaxStack)
	locals := make([]Value, p.NumLocals)
	type vmLoop struct {
		limit, i int64
		slot     int
	}
	var loops []vmLoop
	for pc := 0; pc < len(p.Code); pc++ {
		if len(stack) > ceiling {
			return nil, vmExhausted(p, pc, r, ceiling)
		}
		in := p.Code[pc]
		switch in.Op {
		case OpPushConst:
			stack = append(stack, p.Consts[in.Arg])
		case OpPushLocal:
			stack = append(stack, locals[in.Arg])
		case OpForSetup:
			if len(stack) < 1 {
				return nil, vmInternalError(p, pc, "FOR_SETUP underflow")
			}
			cnt := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			n, err := cnt.AsConcreteInteger()
			if err != nil {
				return nil, stampProgramPos(r.AqlError("for_error", "for: count must be a concrete Integer", "for"), p, pc, r)
			}
			loops = append(loops, vmLoop{limit: n, slot: int(in.Arg)})
		case OpForNext:
			if len(loops) == 0 {
				return nil, vmInternalError(p, pc, "FOR_NEXT without a loop")
			}
			lp := &loops[len(loops)-1]
			if lp.i >= lp.limit {
				loops = loops[:len(loops)-1]
				pc = int(in.Arg) - 1
				continue
			}
			locals[lp.slot] = NewInteger(lp.i)
			lp.i++
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
			t := int(in.Arg)
			// The only legal back-edge is a counted loop's trailing
			// jump to its FOR_NEXT — termination then rides the loop
			// counter.
			if t <= pc && (t < 0 || t >= len(p.Code) || p.Code[t].Op != OpForNext) {
				return nil, vmInternalError(p, pc, "backward jump not to a FOR_NEXT")
			}
			pc = t - 1
		case OpJmpIfFalse:
			if len(stack) < 1 {
				return nil, vmInternalError(p, pc, "JMP_IF_FALSE underflow")
			}
			cond := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if !CoerceBoolean(cond) {
				if int(in.Arg) <= pc {
					return nil, vmInternalError(p, pc, "backward conditional jump")
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

// vmStackCeiling mirrors the tape's bounded-growth ceiling for the
// VM value stack: initial · factorᴺ entries from the registry's
// TapeConfig, exactly NewTapeWith's arithmetic, so a program that
// accumulates without bound fails with the same resource taxonomy in
// both engines.
func vmStackCeiling(r *Registry) int {
	var cfg TapeConfig
	if r != nil {
		cfg = r.TapeConfig
	}
	initial, maxGrows, factor := cfg.resolve(0)
	ceil := float64(initial)
	for i := 0; i < maxGrows; i++ {
		ceil *= factor
	}
	if ceil >= float64(maxIntCap) {
		return maxIntCap
	}
	if int(ceil) < initial {
		return initial
	}
	return int(ceil)
}

// vmExhausted is the VM's tape_exhausted analogue — same code, same
// remedy hint, so tooling and users see one taxonomy.
func vmExhausted(p *Program, pc int, r *Registry, ceiling int) error {
	err := r.AqlErrorHint("tape_exhausted",
		fmt.Sprintf("evaluation stack exhausted its growth ceiling of %d entries — the program consumed unbounded space (an unbounded loop accumulating results?)", ceiling),
		"",
		"raise the tape size via options (initial size / grow count / growth factor) for a legitimately large program; otherwise check the loop bounds")
	return stampProgramPos(err, p, pc, r)
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
