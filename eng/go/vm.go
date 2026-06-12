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
		cur, end, step int64
		slot           int
	}
	var loops []vmLoop
	// Frames: unit -1 is the main program; >=0 indexes p.Fns. The
	// operand stack is shared across frames (results flow to the
	// caller); locals are per-frame. A frame also remembers the
	// caller's open-loop count so RET cannot leak loop state.
	type vmFrame struct {
		retUnit, retPC int
		locals         []Value
		loopBase       int
	}
	var frames []vmFrame
	curUnit := -1
	curCode := p.Code
	curDebug := p.Debug
	enterUnit := func(u int) {
		curUnit = u
		if u < 0 {
			curCode, curDebug = p.Code, p.Debug
		} else {
			curCode, curDebug = p.Fns[u].Code, p.Fns[u].Debug
		}
	}
	for pc := 0; pc < len(curCode); pc++ {
		if len(stack) > ceiling || len(frames) > ceiling {
			return nil, vmExhaustedAt(curDebug, pc, r, ceiling)
		}
		in := curCode[pc]
		switch in.Op {
		case OpPushConst:
			stack = append(stack, p.Consts[in.Arg])
		case OpPushLocal:
			stack = append(stack, locals[in.Arg])
		case OpForSetup:
			if len(stack) < 3 {
				return nil, vmErrAt(curDebug, pc, "FOR_SETUP underflow")
			}
			// Pops start (top), end, step — the same range triple
			// parseRange yields; step semantics match runForLoop,
			// including the zero-step error and negative steps.
			start, err1 := stack[len(stack)-1].AsConcreteInteger()
			endV, err2 := stack[len(stack)-2].AsConcreteInteger()
			stepV, err3 := stack[len(stack)-3].AsConcreteInteger()
			stack = stack[:len(stack)-3]
			if err1 != nil || err2 != nil || err3 != nil {
				return nil, stampAt(r.AqlError("for_error", "for: range must be concrete Integers", "for"), curDebug, pc, r)
			}
			if stepV == 0 {
				return nil, stampAt(r.AqlError("for_error", "for: step cannot be zero", "for"), curDebug, pc, r)
			}
			loops = append(loops, vmLoop{cur: start, end: endV, step: stepV, slot: int(in.Arg)})
		case OpForNext:
			if len(loops) == 0 {
				return nil, vmErrAt(curDebug, pc, "FOR_NEXT without a loop")
			}
			lp := &loops[len(loops)-1]
			done := lp.cur >= lp.end
			if lp.step < 0 {
				done = lp.cur <= lp.end
			}
			if done {
				loops = loops[:len(loops)-1]
				pc = int(in.Arg) - 1
				continue
			}
			locals[lp.slot] = NewInteger(lp.cur)
			lp.cur += lp.step
		case OpSwap:
			if len(stack) < 2 {
				return nil, vmErrAt(curDebug, pc, "SWAP underflow")
			}
			stack[len(stack)-1], stack[len(stack)-2] = stack[len(stack)-2], stack[len(stack)-1]
		case OpCallNative:
			s := p.Sigs[in.Arg]
			n := len(s.Sig.Args)
			if len(stack) < n {
				return nil, vmErrAt(curDebug, pc, "CALL_NATIVE underflow at "+s.Word)
			}
			// One argument convention: position 0 is the top of stack.
			args := make([]Value, n)
			for i := 0; i < n; i++ {
				args[i] = stack[len(stack)-1-i]
			}
			stack = stack[:len(stack)-n]
			results, err := s.Sig.Handler(args, r.Contexts.TopData(), nil, r)
			if err != nil {
				return nil, stampAt(err, curDebug, pc, r)
			}
			// Belt-and-braces: a handler that returns tape tokens (to
			// be re-stepped by the engine) must never have been
			// compiled — the emitter refuses fn-invoking and
			// code-splicing words. Fail loudly, never push tokens as
			// data.
			for _, rv := range results {
				if IsWord(rv) || IsMark(rv) || IsMove(rv) || IsForward(rv) ||
					IsOpenParen(rv) || IsSplice(rv) {
					return nil, vmErrAt(curDebug, pc, "tape-coupled handler result at "+s.Word)
				}
			}
			stack = append(stack, results...)
		case OpJmp:
			t := int(in.Arg)
			// The only legal back-edge is a counted loop's trailing
			// jump to its FOR_NEXT — termination then rides the loop
			// counter.
			if t <= pc && (t < 0 || t >= len(curCode) || curCode[t].Op != OpForNext) {
				return nil, vmErrAt(curDebug, pc, "backward jump not to a FOR_NEXT")
			}
			pc = t - 1
		case OpJmpIfFalse:
			if len(stack) < 1 {
				return nil, vmErrAt(curDebug, pc, "JMP_IF_FALSE underflow")
			}
			cond := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if !CoerceBoolean(cond) {
				if int(in.Arg) <= pc {
					return nil, vmErrAt(curDebug, pc, "backward conditional jump")
				}
				pc = int(in.Arg) - 1
			}
		case OpCallUser, OpTailCallUser:
			fn := &p.Fns[in.Arg]
			if len(stack) < fn.NParams {
				return nil, vmErrAt(curDebug, pc, "CALL_USER underflow at "+fn.Name)
			}
			nl := make([]Value, fn.NLocals)
			for i := 0; i < fn.NParams; i++ {
				nl[i] = stack[len(stack)-1-i]
			}
			stack = stack[:len(stack)-fn.NParams]
			if in.Op == OpCallUser {
				frames = append(frames, vmFrame{retUnit: curUnit, retPC: pc + 1, locals: locals, loopBase: len(loops)})
			} else {
				// Tail call: REPLACE the frame — the language's
				// tail-call guarantee in compiled form. The caller's
				// return slot is untouched; loop state cannot leak
				// across a tail boundary in the compiled subset (tail
				// position excludes open loops by construction), but
				// trim defensively to the frame's base.
				if len(frames) > 0 {
					loops = loops[:frames[len(frames)-1].loopBase]
				}
			}
			locals = nl
			enterUnit(int(in.Arg))
			pc = -1
		case OpRet:
			if len(frames) == 0 {
				return nil, vmErrAt(curDebug, pc, "RET without a frame")
			}
			f := frames[len(frames)-1]
			frames = frames[:len(frames)-1]
			loops = loops[:f.loopBase]
			locals = f.locals
			enterUnit(f.retUnit)
			pc = f.retPC - 1
		default:
			return nil, vmErrAt(curDebug, pc, "unknown opcode")
		}
	}
	if len(frames) != 0 {
		return nil, vmErrAt(curDebug, len(curCode)-1, "code unit ended without RET")
	}
	return stack, nil
}

// stampAt / vmErrAt are the per-unit debug-table variants of the
// program-level error helpers.
func stampAt(err error, debug []SrcPos, pc int, r *Registry) error {
	ae, ok := err.(*AqlError)
	if !ok || pc < 0 || pc >= len(debug) {
		return err
	}
	if ae.Row == 0 {
		ae.Row = debug[pc].Row
		ae.Col = debug[pc].Col
	}
	if r != nil && ae.fullSource == "" {
		ae.fullSource = r.Source
	}
	return ae
}

func vmErrAt(debug []SrcPos, pc int, msg string) error {
	pos := SrcPos{}
	if pc >= 0 && pc < len(debug) {
		pos = debug[pc]
	}
	return fmt.Errorf("bytecode: internal: %s (pc=%d, src %d:%d)", msg, pc, pos.Row, pos.Col)
}

func vmExhaustedAt(debug []SrcPos, pc int, r *Registry, ceiling int) error {
	err := r.AqlErrorHint("tape_exhausted",
		fmt.Sprintf("evaluation stack exhausted its growth ceiling of %d entries — the program consumed unbounded space (an unbounded loop accumulating results, or unbounded non-tail recursion?)", ceiling),
		"",
		"raise the tape size via options (initial size / grow count / growth factor) for a legitimately large program; otherwise check the loop bounds / recursion")
	return stampAt(err, debug, pc, r)
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
