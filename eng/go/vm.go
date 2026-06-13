package eng

// The bytecode VM — the execution half of Stages 1–3 of
// design/aql-bytecode-plan.0.md: straight-line natives, control flow
// (JMP / JMP_IF_FALSE / FOR_SETUP / FOR_NEXT), and user-fn frames
// with CALL_USER / TAIL_CALL_USER / RET.
//
// Termination and resource parity (plan R6 #27): the only back-edge
// the emitter produces is a counted loop's trailing JMP to its
// FOR_NEXT, and the VM enforces exactly that shape — so every loop
// is bounded by its popped count, and an emitted body nets one value
// per iteration. Unbounded value accumulation (`for huge [i]`) hits
// the same growth ceiling the tape enforces: the VM stack's ceiling
// is computed from the registry's TapeConfig and overflowing it
// raises the interpreter's tape_exhausted taxonomy; frame depth
// shares the same ceiling. A runaway that consumes neither (a
// tail-call spin) trips the step budget with the interpreter's
// evaluation_limit taxonomy.

import "fmt"

// RunProgram executes a compiled Program against a registry and
// returns the residual value stack (bottom → top), matching what the
// interpreter's Run returns for the same source. The step budget is
// the interpreter's DefaultStepLimit: a runaway that never grows the
// stack (a tail-recursive spin — frames are REPLACED, so neither
// ceiling trips) fails with the same evaluation_limit taxonomy the
// interpreter raises, instead of hanging.
func RunProgram(p *Program, r *Registry) ([]Value, error) {
	return runProgram(p, r, DefaultStepLimit)
}

// vmLoop is one open counted loop's iteration state.
type vmLoop struct {
	cur, end, step int64
	slot           int
}

// vmFrame remembers a caller's resumption point across a CALL_USER: the
// unit and pc to return to, the caller's frame locals, and the open-loop
// count so RET cannot leak loop state.
type vmFrame struct {
	retUnit, retPC int
	locals         []Value
	loopBase       int
}

// vmContext holds the state SHARED across a program run and every re-entrant
// closure invocation it spawns: the program, the registry, the resource
// ceilings, the running step count (one global budget), and the reused island
// sub-engine / args scratch. Per-run state (operand stack, frame locals,
// frames, open loops, pc) lives in run() so a body closure invoked
// mid-dispatch executes on its own stack without disturbing the caller.
type vmContext struct {
	p          *Program
	r          *Registry
	ceiling    int
	stepLimit  int
	steps      int
	islandEng  *Engine
	argScratch []Value
}

func runProgram(p *Program, r *Registry, stepLimit int) ([]Value, error) {
	if p == nil {
		return nil, fmt.Errorf("bytecode: nil program")
	}
	vc := &vmContext{p: p, r: r, ceiling: vmStackCeiling(r), stepLimit: stepLimit}
	// Install the body-closure invoker so a higher-order word's handler runs
	// its body through the VM (InvokeBody → r.Invoker → invokeClosure). The
	// shared registry means the island sub-engine inherits it too, so the
	// invoker dispatches on the body VALUE: a compiled closure runs in the
	// VM, a raw token-list body (an island's interpreter run reaching a
	// handler) runs through a sub-engine — identical to InvokeBody's nil
	// branch. Restored on exit so nested RunProgram calls nest cleanly.
	prevInvoker := r.Invoker
	r.Invoker = vc.invokeClosure
	defer func() { r.Invoker = prevInvoker }()
	return vc.run(-1, make([]Value, p.NumLocals), make([]Value, 0, p.MaxStack))
}

// invokeClosure runs a code body for the InvokeBody seam. A compiled closure
// (OpPushClosure's value) executes in the VM's re-entrant runner: its inputs
// bind to the body unit's leading param slots and its captures to the trailing
// slots, then the unit runs on a fresh operand stack. Any other body value (a
// raw token list — an island's interpreter run reaching a higher-order
// handler) runs through a sub-engine exactly as InvokeBody does with no
// Invoker, so the island path is unchanged.
func (vc *vmContext) invokeClosure(body Value, inputs []Value) ([]Value, error) {
	cl, ok := body.Data.(ClosurePayload)
	if !ok {
		toks := bodyTokens(body)
		input := make([]Value, len(inputs)+len(toks))
		copy(input, inputs)
		copy(input[len(inputs):], toks)
		return New(vc.r).Run(input)
	}
	fn := &vc.p.Fns[cl.Unit]
	locals := make([]Value, fn.NLocals)
	for i := 0; i < len(inputs) && i < fn.NParams; i++ {
		locals[i] = inputs[i]
	}
	for i, cv := range cl.Captures {
		if slot := fn.NParams + i; slot < len(locals) {
			locals[slot] = cv
		}
	}
	return vc.run(cl.Unit, locals, nil)
}

// run executes from startUnit (unit -1 is the main program; >=0 indexes
// p.Fns) with the given frame locals and initial operand stack, returning the
// residual stack when the unit runs off the end of its code (the main program
// — and body closures, which carry no trailing RET). A RET propagates back to
// its CALL_USER caller within this run. Re-entrant: a body closure invoked
// from a native handler calls run() again on a fresh stack, sharing vc's step
// budget and island engine.
func (vc *vmContext) run(startUnit int, locals []Value, stack []Value) ([]Value, error) {
	p, r := vc.p, vc.r
	ceiling := vc.ceiling
	var loops []vmLoop
	var frames []vmFrame
	curUnit := startUnit
	var curCode []Instr
	var curDebug []SrcPos
	enterUnit := func(u int) {
		curUnit = u
		if u < 0 {
			curCode, curDebug = p.Code, p.Debug
		} else {
			curCode, curDebug = p.Fns[u].Code, p.Fns[u].Debug
		}
	}
	enterUnit(startUnit)
	for pc := 0; pc < len(curCode); pc++ {
		if len(stack) > ceiling || len(frames) > ceiling {
			return nil, vmExhaustedAt(curDebug, pc, r, ceiling)
		}
		vc.steps++
		if vc.steps > vc.stepLimit {
			return nil, vmEvalLimitAt(curDebug, pc, r, vc.stepLimit)
		}
		in := curCode[pc]
		switch in.Op {
		case OpPushConst:
			stack = append(stack, p.Consts[in.Arg])
		case OpPushLocal:
			stack = append(stack, locals[in.Arg])
		case OpPushClosure:
			stack = append(stack, NewClosure(int(in.Arg), nil))
		case OpPushType:
			// Resolve the CANONICAL node at run time — never a pooled
			// copy (eng/go/CLAUDE.md, Canonical *Type Pointers). Types
			// the check pass minted (def Foo …) live in the registry's
			// table; kernel builtins in the package Builtin table.
			var t *Type
			if r != nil {
				t = r.Types.LookupByID(p.Types[in.Arg].ID)
			}
			if t == nil {
				t = Builtin.LookupByID(p.Types[in.Arg].ID)
			}
			if t == nil {
				return nil, vmErrAt(curDebug, pc, "unresolvable type operand "+p.Types[in.Arg].Name)
			}
			stack = append(stack, NewTypeLiteral(t))
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
			// Reuse a per-RunProgram scratch buffer instead of allocating
			// an args slice every dispatch — the dominant per-CALL_NATIVE
			// allocation on the compute path. Safe: the handler's result
			// is COPIED into the operand stack by the append below before
			// the next call reuses the buffer, and compiled-reachable
			// natives (the monomorphic math/compare/etc. words the emitter
			// admits) do not retain the args slice. The 0-divergence gate
			// + combination matrix catch any handler that does.
			if cap(vc.argScratch) < n {
				vc.argScratch = make([]Value, n)
			}
			args := vc.argScratch[:n]
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
		case OpFallback:
			fb := &p.Fallbacks[in.Arg]
			if len(stack) < fb.NIn {
				return nil, vmErrAt(curDebug, pc, "FALLBACK underflow at "+fb.Desc)
			}
			// Build the island token stream: the NIn threaded inputs
			// (deepest-first, exactly their operand-stack order) preloaded
			// as literals, then the recorded span tokens. The island's
			// sub-engine re-derives the construct's result the same way
			// the interpreter does — soundness rides on the differential
			// gate. break/continue/return raised across the island
			// boundary propagate via the shared registry FlowCtrl, the
			// same as any nested Run.
			island := make([]Value, 0, fb.NIn+len(fb.Tokens))
			island = append(island, stack[len(stack)-fb.NIn:]...)
			island = append(island, fb.Tokens...)
			stack = stack[:len(stack)-fb.NIn]
			// Reuse one island sub-engine across every OpFallback in this
			// RunProgram: it reloads its tape in place, so a hot island in
			// a loop does not allocate a fresh engine+tape per iteration.
			if vc.islandEng == nil {
				vc.islandEng = New(r)
				vc.islandEng.SetSource(r.Source)
				vc.islandEng.reuseTape = true
			}
			results, err := vc.islandEng.Run(island)
			if err != nil {
				return nil, stampAt(err, curDebug, pc, r)
			}
			for _, rv := range results {
				if IsWord(rv) || IsMark(rv) || IsMove(rv) || IsForward(rv) ||
					IsOpenParen(rv) || IsSplice(rv) {
					return nil, vmErrAt(curDebug, pc, "tape-coupled island result at "+fb.Desc)
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
			// Return-type check — the compiled mirror of the interpreter's
			// ReturnCheck (__RC, engine.go): the body's result must satisfy
			// each declared return type via v.Is(exp), the SAME membership
			// the parameter boundary asks, so a predicate refine runs its
			// predicate, a bare refine stays nominal, and builtins are
			// unchanged. The body nets exactly len(Returns) values (the
			// lowerer enforces single-result bodies), sitting on top.
			if curUnit >= 0 {
				rets := p.Fns[curUnit].Returns
				if len(rets) > 0 {
					if len(stack) < len(rets) {
						return nil, stampAt(vmReturnCountErr(r, p.Fns[curUnit].Name, len(rets), len(stack)), curDebug, pc, r)
					}
					base := len(stack) - len(rets)
					for k, exp := range rets {
						if !stack[base+k].Is(CanonicalType(r, exp)) {
							return nil, stampAt(vmReturnTypeErr(r, p.Fns[curUnit].Name, k+1, exp, stack[base+k]), curDebug, pc, r)
						}
					}
				}
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

// vmReturnTypeErr / vmReturnCountErr mirror the interpreter's
// returnTypeError / returnCountError (engine.go) byte-for-byte — same
// detail text, same type_error taxonomy — so error-scraping tooling
// never learns which engine ran.
func vmReturnTypeErr(r *Registry, funcName string, index int, expected *Type, got Value) error {
	detail := fmt.Sprintf("%s: return value %d: expected %s, got %s", funcName, index, expected, got.Parent)
	return r.AqlErrorHint("type_error", detail, funcName, "value: "+diagValue(got))
}

func vmReturnCountErr(r *Registry, funcName string, expected, got int) error {
	detail := fmt.Sprintf("%s: expected %d return value(s), got %d", funcName, expected, got)
	return r.AqlError("type_error", detail, funcName)
}

func vmErrAt(debug []SrcPos, pc int, msg string) error {
	pos := SrcPos{}
	if pc >= 0 && pc < len(debug) {
		pos = debug[pc]
	}
	return fmt.Errorf("bytecode: internal: %s (pc=%d, src %d:%d)", msg, pc, pos.Row, pos.Col)
}

// vmEvalLimitAt mirrors the interpreter's evalLimitError: the
// step-count (CPU) guard, distinct from the stack/frame ceiling
// (the memory guard).
func vmEvalLimitAt(debug []SrcPos, pc int, r *Registry, limit int) error {
	err := r.AqlErrorHint("evaluation_limit",
		fmt.Sprintf("evaluation exceeded the step limit of %d — the program ran too long (an infinite loop or unbounded recursion?)", limit),
		"",
		"if this is a legitimately long computation, raise the limit via the engine's step budget; otherwise check for a loop or recursion that never terminates")
	return stampAt(err, debug, pc, r)
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
