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

import (
	"fmt"
	"sync/atomic"
)

// RunProgram executes a compiled Program against a registry and
// returns the residual value stack (bottom → top), matching what the
// interpreter's Run returns for the same source. The step budget is
// the interpreter's DefaultStepLimit: a runaway that never grows the
// stack (a tail-recursive spin — frames are REPLACED, so neither
// ceiling trips) fails with the same evaluation_limit taxonomy the
// interpreter raises, instead of hanging.
//
// Concurrency: a *Registry must not be driven by two executions at once —
// and that means ANY pairing, not just two compiled runs. For its duration
// this run installs/restores r.Invoker (the body-closure seam) and mutates the
// registry's scopes, so a concurrent RunProgram OR a concurrent interpreter
// Run on the same registry would race on r.Invoker and the shared defs/types.
// Two guards run at entry: the vmRunning CAS rejects an overlapping compiled
// run, and an interpRunActive() check rejects starting a compiled run while an
// interpreter run is in flight (the depth counter Engine.Run maintains). What
// neither can catch is a fresh interpreter Run STARTING on another goroutine
// once this compiled run is already underway — the VM's own islands re-enter
// Engine.Run on this same registry, so a registry-level flag cannot tell a
// legitimate island from a foreign run without goroutine identity. That last
// shape stays the caller's responsibility under the same rule the interpreter
// already follows: give each goroutine its own *Registry. AQL's concurrent
// words honour this by forking an isolated registry per branch
// (ForkConcurrent); host callers run each instance on its own registry.
func RunProgram(p *Program, r *Registry) ([]Value, error) {
	return runProgram(p, r, DefaultStepLimit)
}

// vmLoop is one open counted loop's iteration state.
type vmLoop struct {
	cur, end, step int64
	slot           int
}

// vmFrame remembers a caller's resumption point across a CALL_USER: the
// unit and pc to return to, the caller's frame locals, the open-loop
// count so RET cannot leak loop state, and the operand-stack depth at
// call entry (after the callee's params were popped) so RET can verify
// the body left EXACTLY its declared return count.
type vmFrame struct {
	retUnit, retPC int
	locals         []Value
	loopBase       int
	stackBase      int
}

// vmContext holds the state SHARED across a program run and every re-entrant
// closure invocation it spawns: the program, the registry, the resource
// ceilings, the running step count (one global budget), and the reused island
// sub-engine / args scratch. Per-run state (operand stack, frame locals,
// frames, open loops, pc) lives in run() so a body closure invoked
// mid-dispatch executes on its own stack without disturbing the caller.
type vmContext struct {
	p         *Program
	r         *Registry
	ceiling   int
	stepLimit int
	steps     int
	// frameDepth counts live VM activations — user-call frames AND re-entrant
	// run() invocations (a closure invoked from a native handler via
	// invokeClosure starts a FRESH run with its own frames slice). The per-run
	// frames slice alone does NOT bound depth that flows through such
	// re-entrant invocations, so the count lives here, shared across them, and
	// the per-instruction guard checks it against the same ceiling the operand
	// stack uses. This keeps deep higher-order recursion failing with the
	// tape_exhausted memory taxonomy (as the interpreter does) instead of
	// growing the Go stack until it overflows — a panic the top-level recover
	// would otherwise mask as internal_error.
	frameDepth int
	// islandEng is a single sub-engine reused across every OpFallback /
	// CALL_DYNAMIC island in this run, with reuseTape set so a hot island in
	// a loop does not allocate a fresh engine+tape per iteration. Reuse is
	// sound ONLY because island runs are never nested or concurrent within a
	// run: the main VM loop is suspended while islandEng.Run executes, and a
	// higher-order body reached from inside an island re-enters via a FRESH
	// New(r) sub-engine (invokeClosure's non-closure branch), never this one.
	// A future change that makes an island re-enter islandEng would corrupt
	// its in-place-reloaded tape — keep island execution non-reentrant.
	islandEng *Engine
}

// tapeCoupled reports whether any result value is a tape-coupled token
// (Word/Mark/Move/Forward/OpenParen/Splice) — a value the interpreter would
// re-STEP on the tape rather than treat as data. No compiled-reachable handler
// should produce one (the emitter refuses fn-invoking / code-splicing words),
// so every dispatch site that funnels handler/island results back onto the
// operand stack screens for them and fails loudly instead of pushing a token
// as data. The single definition keeps the four call sites in lockstep.
func tapeCoupled(results []Value) bool {
	for _, rv := range results {
		if IsWord(rv) || IsMark(rv) || IsMove(rv) || IsForward(rv) ||
			IsOpenParen(rv) || IsSplice(rv) {
			return true
		}
	}
	return false
}

func runProgram(p *Program, r *Registry, stepLimit int) (result []Value, runErr error) {
	if p == nil {
		return nil, fmt.Errorf("bytecode: nil program")
	}
	// Concurrency guard: a single registry cannot drive two OVERLAPPING runs —
	// the shared Invoker install/restore below (and the mutable scopes the run
	// touches) would race. Catch the misuse with a clear error instead of
	// silent data corruption; concurrent runs must each own a registry
	// (ForkConcurrent). Nested SEQUENTIAL reuse is unaffected: the flag resets
	// on exit before the next run begins, which is the normal RunCompiled path.
	if r != nil {
		if !atomic.CompareAndSwapInt32(&r.vmRunning, 0, 1) {
			return nil, makeAqlError("concurrency_error",
				"bytecode: a compiled program is already running on this registry; concurrent runs need their own registry (ForkConcurrent)",
				"", "", "")
		}
		defer atomic.StoreInt32(&r.vmRunning, 0)
		// Also reject starting a compiled run while an INTERPRETER run is in
		// flight on this same registry — the cross-engine race the CAS above
		// cannot catch. Safe to check here: RunProgram has not yet spawned any
		// island sub-engine, so a non-zero depth means a DISTINCT interpreter
		// run (the compiled run's own islands increment the depth only later).
		if r.interpRunActive() {
			// The deferred StoreInt32 above releases vmRunning on this return.
			return nil, makeAqlError("concurrency_error",
				"bytecode: an interpreter run is already active on this registry; concurrent runs need their own registry (ForkConcurrent)",
				"", "", "")
		}
	}
	// Last-resort panic guard, mirroring the interpreter's top-level recover
	// (engine.go Run): a bug in a compiled-reachable handler or in the VM loop
	// must surface as a clean internal_error AqlError — which RunCompiled then
	// resolves by falling back to the interpreter — never as a goroutine stack
	// trace. Errors returned normally are untouched.
	defer func() {
		if rec := recover(); rec != nil {
			src := ""
			if r != nil {
				src = r.Source
			}
			result = nil
			runErr = makeAqlError("internal_error",
				fmt.Sprintf("internal bytecode VM error: %v", rec), "", src, "")
		}
	}()
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
	// Inputs fill the leading param slots, captures the trailing ones
	// (StartFnCompile registers params before captures).
	nInputs := fn.NParams - len(cl.Captures)
	for i := 0; i < len(inputs) && i < nInputs; i++ {
		locals[i] = inputs[i]
	}
	for i, cv := range cl.Captures {
		if slot := nInputs + i; slot < len(locals) {
			locals[slot] = cv
		}
	}
	return vc.run(cl.Unit, locals, nil)
}

// callPoly dispatches a native word by matching the kernel's own
// MatchSignature over the word's signatures against the top Arity stack
// values — the same first-match the interpreter takes — then calls the
// matched handler (plan P3). A no-match raises signature_error, the same
// taxonomy the interpreter's sigError raises.
func (vc *vmContext) callPoly(pr *PolyRef, stack []Value, curDebug []SrcPos, pc int) ([]Value, error) {
	r := vc.r
	n := pr.Arity
	if len(stack) < n {
		return nil, vmErrAt(curDebug, pc, "CALL_NATIVE_POLY underflow at "+pr.Word)
	}
	var sigs []Signature
	if fn := r.Lookup(pr.Word); fn != nil {
		sigs = fn.Signatures
	}
	// Build the args in sig order (position 0 = top of stack, as OpCallNative
	// does), then match: MatchSignature's positionalMatch reads values[i] as
	// sig position i.
	window := make([]Value, n)
	for i := 0; i < n; i++ {
		window[i] = stack[len(stack)-1-i]
	}
	mr := MatchSignature(sigs, window, WordInfo{ArgCount: n})
	if mr == nil || mr.Sig == nil || mr.Sig.Handler == nil {
		return nil, stampAt(makeAqlError("signature_error", "no matching signature for "+pr.Word, pr.Word, r.Source, ""), curDebug, pc, r)
	}
	results, err := mr.Sig.Handler(mr.Args, r.Contexts.TopData(), nil, r)
	if err != nil {
		return nil, stampAt(err, curDebug, pc, r)
	}
	// A get/getr whose result is a NAMED trivial-delegation METHOD with a 0-arg
	// overload is auto-applied by the interpreter the instant it is produced
	// (`r.bool`). Mirror that: dispatch it 0-arg VM-native. A method needing args
	// (`r.int`) has no 0-arg sig, so it stays a value and flows to a later
	// CALL_DYNAMIC; an anonymous fn stays data (the interpreter does not
	// auto-invoke a 0-arg anonymous value). The isDelegationFnDef guard is
	// essential: a USER fn ref (`{b:f/r}`) is NOT a 0-arg method — tryNativeFnApply
	// would match its InstallFnDef-registered handler and call it argless,
	// diverging. A user fn stays a value here and applies correctly at CALL_DYNAMIC.
	if (pr.Word == "get" || pr.Word == "getr") && len(results) == 1 {
		if fnDef, ok := results[0].Data.(FnDefInfo); ok && !fnDef.Anonymous && isDelegationFnDef(fnDef) {
			if applied, done, aerr := vc.tryNativeFnApply(fnDef, nil); done {
				if aerr != nil {
					return nil, stampAt(aerr, curDebug, pc, r)
				}
				results = applied
			}
		}
	}
	if tapeCoupled(results) {
		return nil, vmErrAt(curDebug, pc, "tape-coupled poly result at "+pr.Word)
	}
	return append(stack[:len(stack)-n], results...), nil
}

// callDynamic applies a runtime fn VALUE (sitting below n trailing args) to
// those args — the fn-value-call boundary (plan P4). A compiled closure runs
// VM-native via the re-entrant runner; any other callable (an FnDef method
// like `r.int`) is applied through the island sub-engine, which auto-applies
// it exactly as the interpreter does. A NON-callable value is left on the
// stack with its args (the interpreter leaves a non-callable residual
// untouched), so a dynamic value that turns out to be data does not diverge.
func (vc *vmContext) callDynamic(n int, stack []Value, curDebug []SrcPos, pc int) ([]Value, error) {
	r := vc.r
	if len(stack) < n+1 {
		return nil, vmErrAt(curDebug, pc, "CALL_DYNAMIC underflow")
	}
	base := len(stack) - n - 1
	fnVal := stack[base]
	args := stack[base+1:]

	if _, ok := fnVal.Data.(ClosurePayload); ok {
		// Pass fnVal directly so the payload's InShape rides along (invokeClosure
		// only fills param slots, but a downstream handler may read the shape).
		results, err := vc.invokeClosure(fnVal, append([]Value(nil), args...))
		if err != nil {
			return nil, stampAt(err, curDebug, pc, r)
		}
		return append(stack[:base], results...), nil
	}
	if !isAppliableFn(fnVal) {
		// Not callable: leave the value and its args as the residual,
		// matching the interpreter (it does not apply a non-Function).
		return stack, nil
	}
	// A trivial-delegation native method (its dispatchable sig is `[Word(name)]`
	// — a module wrapper like rand-int) dispatches VM-NATIVE: MatchSignature
	// picks the overload the interpreter would and the inner handler runs
	// directly — no sub-engine. A user fn carries a REAL body, NOT a delegation
	// word, so it must NOT take this path: tryNativeFnApply would match the
	// InstallFnDef-registered Handler and call it outside the dispatch frame it
	// expects — diverging. Those fall through to the island, which runs the body
	// faithfully as a nested Run.
	if fnDef, ok := fnVal.Data.(FnDefInfo); ok && isDelegationFnDef(fnDef) {
		if results, done, err := vc.tryNativeFnApply(fnDef, args); done {
			if err != nil {
				return nil, stampAt(err, curDebug, pc, r)
			}
			return append(stack[:base], results...), nil
		}
	}
	// Non-trivial fn (user body): apply via the island sub-engine, which
	// auto-applies the Function to the forward args exactly as a nested Run.
	island := make([]Value, 0, n+1)
	island = append(island, fnVal)
	island = append(island, args...)
	if vc.islandEng == nil {
		vc.islandEng = New(r)
		vc.islandEng.SetSource(r.Source)
		vc.islandEng.reuseTape = true
	}
	results, err := vc.islandEng.Run(island)
	if err != nil {
		return nil, stampAt(err, curDebug, pc, r)
	}
	if tapeCoupled(results) {
		return nil, vmErrAt(curDebug, pc, "tape-coupled dynamic result")
	}
	return append(stack[:base], results...), nil
}

// isDelegationFnDef reports whether a Function VALUE is a trivial-delegation
// wrapper — EVERY own sig is a `[Word(inner)]` pass-through to an inner native
// (a module method like rand-int / MathUtil.sqrt), safely dispatched VM-native
// via tryNativeFnApply. A user fn carries a REAL body, so it is NOT a delegation
// and must island instead. An anonymous lambda or a sig-less value is not one.
func isDelegationFnDef(fd FnDefInfo) bool {
	sigs := fd.OwnSigs()
	if len(sigs) == 0 {
		return false
	}
	for i := range sigs {
		if _, ok := trivialDelegationTarget(&sigs[i]); !ok {
			return false
		}
	}
	return true
}

// tryNativeFnApply dispatches a Function VALUE VM-native when it resolves to a
// handler-bearing signature (a trivial-delegation native — a method field like
// rand-int): MatchSignature over the dispatchable signatures picks the
// overload the interpreter would, and the handler runs directly. done is false
// when the fn has a non-trivial (user) body that needs the interpreter — the
// caller then islands. The island stays the correctness backstop, so any
// divergence from this fast path is caught by the differential gate.
func (vc *vmContext) tryNativeFnApply(fnDef FnDefInfo, args []Value) ([]Value, bool, error) {
	reg := fnDef.Registry
	if reg == nil {
		reg = vc.r
	}
	var sigs []Signature
	if inner := reg.Lookup(fnDef.Name); inner != nil {
		sigs = inner.Signatures
	} else if len(fnDef.Signatures) > 0 {
		sigs = fnDef.Signatures
	}
	if len(sigs) == 0 {
		return nil, false, nil
	}
	mr := MatchSignature(sigs, args, WordInfo{ArgCount: len(args)})
	if mr == nil || mr.Sig == nil || mr.Sig.Handler == nil {
		return nil, false, nil
	}
	results, err := mr.Sig.Handler(mr.Args, reg.Contexts.TopData(), nil, reg)
	return results, true, err
}

// isAppliableFn reports whether a runtime value is a callable the interpreter
// would auto-apply: a Function-typed value or an FnDef payload.
func isAppliableFn(v Value) bool {
	if _, ok := v.Data.(FnDefInfo); ok {
		return true
	}
	return v.Parent != nil && v.Parent.ConformsTo(TFunction)
}

// runFallback executes one interpreter island (OpFallback): it preloads the
// NIn threaded inputs (deepest-first) then the recorded span tokens onto a
// reused sub-engine, runs it, and returns the operand stack with the island's
// residual pushed. break/continue/return raised across the boundary propagate
// via the shared registry FlowCtrl, as in any nested Run. (Deleted in plan
// P7 once every shape compiles natively.)
func (vc *vmContext) runFallback(fb *FallbackSpan, stack []Value, curDebug []SrcPos, pc int) ([]Value, error) {
	r := vc.r
	if len(stack) < fb.NIn {
		return nil, vmErrAt(curDebug, pc, "FALLBACK underflow at "+fb.Desc)
	}
	island := make([]Value, 0, fb.NIn+len(fb.Tokens))
	island = append(island, stack[len(stack)-fb.NIn:]...)
	island = append(island, fb.Tokens...)
	stack = stack[:len(stack)-fb.NIn]
	// Reuse one island sub-engine across every OpFallback in this run: it
	// reloads its tape in place, so a hot island in a loop does not allocate
	// a fresh engine+tape per iteration.
	if vc.islandEng == nil {
		vc.islandEng = New(r)
		vc.islandEng.SetSource(r.Source)
		vc.islandEng.reuseTape = true
	}
	results, err := vc.islandEng.Run(island)
	if err != nil {
		return nil, stampAt(err, curDebug, pc, r)
	}
	if tapeCoupled(results) {
		return nil, vmErrAt(curDebug, pc, "tape-coupled island result at "+fb.Desc)
	}
	return append(stack, results...), nil
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
	// This activation counts against the shared frame ceiling; restore the
	// entry baseline on exit so sequential runs and error unwinds never leak
	// (the per-CALL_USER increments below are balanced by their RETs, and the
	// reset catches any frames still open on an early error return).
	entryFrameDepth := vc.frameDepth
	vc.frameDepth++
	defer func() { vc.frameDepth = entryFrameDepth }()
	var loops []vmLoop
	var frames []vmFrame
	// argScratch is per-run (NOT shared on vc): a re-entrant closure run's
	// CALL_NATIVE must not clobber an outer handler's args slice, which
	// aliases this buffer until the handler returns (a higher-order handler
	// holds its data arg across the InvokeBody call that drives the nested
	// run).
	var argScratch []Value
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
		if len(stack) > ceiling || vc.frameDepth > ceiling {
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
		case OpStoreLocal:
			// Pop the producing event's single result into a frame local;
			// each reference re-pushes it via PUSH_LOCAL (value-def locals).
			if len(stack) == 0 {
				return nil, vmErrAt(curDebug, pc, "STORE_LOCAL stack underflow")
			}
			locals[in.Arg] = stack[len(stack)-1]
			stack = stack[:len(stack)-1]
		case OpDrop:
			// Discard the top value — the computed else value on the taken
			// (then) path of `if cond [then] (expr)`.
			if len(stack) == 0 {
				return nil, vmErrAt(curDebug, pc, "DROP stack underflow")
			}
			stack = stack[:len(stack)-1]
		case OpMakeList:
			// Assemble the top Arg values into a list (a computed list literal,
			// `[1 add 2]`); order preserved, deepest becomes element 0.
			n := int(in.Arg)
			if len(stack) < n {
				return nil, vmErrAt(curDebug, pc, "MAKE_LIST stack underflow")
			}
			elems := make([]Value, n)
			copy(elems, stack[len(stack)-n:])
			stack = stack[:len(stack)-n]
			stack = append(stack, NewList(elems))
		case OpMakeMap:
			// Assemble the top values into a map paired with the spec's keys (a
			// computed make-construction body, `make Outer {i:(make Inner …)}`).
			var err error
			if stack, err = vmMakeMap(p, stack, in.Arg, curDebug, pc); err != nil {
				return nil, err
			}
		case OpTrap:
			// A check-mode-suppressed runtime error compiled in place: raise the
			// byte-identical AQL error (the interpreter errors at this same point).
			tr := &p.Traps[in.Arg]
			var err error
			if tr.Hint != "" {
				err = r.AqlErrorHint(tr.Code, tr.Detail, tr.Word, tr.Hint)
			} else {
				err = r.AqlError(tr.Code, tr.Detail, tr.Word)
			}
			return nil, stampAt(err, curDebug, pc, r)
		case OpPushClosure:
			nc := p.Fns[in.Arg].NCaptures
			if len(stack) < nc {
				return nil, vmErrAt(curDebug, pc, "PUSH_CLOSURE capture underflow")
			}
			var caps []Value
			if nc > 0 {
				caps = make([]Value, nc)
				copy(caps, stack[len(stack)-nc:])
				stack = stack[:len(stack)-nc]
			}
			cl := ClosurePayload{Unit: int(in.Arg), Captures: caps, InShape: p.Fns[in.Arg].InShape}
			stack = append(stack, Value{Parent: TFunction, Data: cl})
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
		case OpSwap, OpReverse:
			// SWAP is reverse-of-2; OpReverse reverses the top Arg. Shared helper.
			var err error
			if stack, err = vmShuffle(stack, in.Op, int(in.Arg), curDebug, pc); err != nil {
				return nil, err
			}
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
			// + combination matrix catch any handler that does — and a
			// -tags aqldebug build (vmFreshArgsPerCall) allocates fresh per
			// call to localize a violator directly. See vm_args_release.go.
			var args []Value
			if vmFreshArgsPerCall {
				args = make([]Value, n)
			} else {
				if cap(argScratch) < n {
					argScratch = make([]Value, n)
				}
				args = argScratch[:n]
			}
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
			if tapeCoupled(results) {
				return nil, vmErrAt(curDebug, pc, "tape-coupled handler result at "+s.Word)
			}
			stack = append(stack, results...)
		case OpFallback:
			ns, err := vc.runFallback(&p.Fallbacks[in.Arg], stack, curDebug, pc)
			if err != nil {
				return nil, err
			}
			stack = ns
		case OpCallNativePoly:
			ns, err := vc.callPoly(&p.PolyRefs[in.Arg], stack, curDebug, pc)
			if err != nil {
				return nil, err
			}
			stack = ns
		case OpCallDynamic:
			ns, err := vc.callDynamic(int(in.Arg), stack, curDebug, pc)
			if err != nil {
				return nil, err
			}
			stack = ns
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
				frames = append(frames, vmFrame{retUnit: curUnit, retPC: pc + 1, locals: locals, loopBase: len(loops), stackBase: len(stack)})
				vc.frameDepth++ // balanced by the matching RET below
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
			// Return-type check — the compiled mirror of the interpreter's
			// ReturnCheck (__RC, engine.go): the body's result must satisfy
			// each declared return type via v.Is(exp), the SAME membership
			// the parameter boundary asks, so a predicate refine runs its
			// predicate, a bare refine stays nominal, and builtins are
			// unchanged. The body nets exactly len(Returns) values (the
			// lowerer enforces single-result bodies), sitting on top.
			// Applies to a nested RET (back to a CALL_USER caller) AND the
			// top RET of a re-entrant unit run. (A closure body unit carries
			// Returns=[Any] — compileClosureBody's declared return — so this
			// check is a guaranteed-pass v.Is(Any) for closures; the cost is
			// one trivial membership test per closure return, deliberately
			// kept uniform with user-fn return enforcement.)
			if curUnit >= 0 {
				stackBase := 0
				if len(frames) > 0 {
					stackBase = frames[len(frames)-1].stackBase
				}
				if err := checkReturnContract(r, &p.Fns[curUnit], stack, stackBase, len(frames) > 0); err != nil {
					return nil, stampAt(err, curDebug, pc, r)
				}
			}
			if len(frames) == 0 {
				// Top RET of a re-entrant unit run (a body closure invoked
				// via invokeClosure): the residual stack is the unit's
				// result, threaded back through the InvokeBody seam. The
				// main program (unit -1) never RETs — it runs off the end —
				// so this path is closure/fn-root only.
				return stack, nil
			}
			f := frames[len(frames)-1]
			frames = frames[:len(frames)-1]
			vc.frameDepth-- // matches the OpCallUser increment
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

// vmReturnTypeErr / vmReturnCountErr raise the interpreter's
// returnTypeError / returnCountError text — same detail/hint, same type_error
// taxonomy — so error-scraping tooling never learns which engine ran. The
// strings come from the shared returnTypeErrorText / returnCountErrorText
// (return_check_msg.go); only the error-construction plumbing differs.
func vmReturnTypeErr(r *Registry, funcName string, index int, expected *Type, got Value) error {
	detail, hint := returnTypeErrorText(funcName, index, expected, got)
	return r.AqlErrorHint("type_error", detail, funcName, hint)
}

func vmReturnCountErr(r *Registry, funcName string, expected, got int) error {
	return r.AqlError("type_error", returnCountErrorText(funcName, expected, got), funcName)
}

// vmShuffle reverses the top n operand-stack values in place: OpSwap is the n=2
// case, OpReverse takes n from arg. Used to seat an N-operand call's computed
// args (which evaluate into reverse sig order) onto the stack in sig order.
func vmShuffle(stack []Value, op Opcode, arg int, debug []SrcPos, pc int) ([]Value, error) {
	n := 2
	if op == OpReverse {
		n = arg
	}
	if len(stack) < n {
		return nil, vmErrAt(debug, pc, op.String()+" underflow")
	}
	for i, j := len(stack)-n, len(stack)-1; i < j; i, j = i+1, j-1 {
		stack[i], stack[j] = stack[j], stack[i]
	}
	return stack, nil
}

// checkReturnContract enforces a compiled fn's declared return contract at a RET
// — the compiled mirror of the interpreter's ReturnCheck (__RC, engine.go). Each
// declared return must satisfy its type via v.Is(exp), the SAME membership the
// parameter boundary asks (so a predicate refine runs its predicate, a bare
// refine stays nominal). When hasFrame is set (a genuine user fn entered via
// CALL_USER), the body must leave EXACTLY len(Returns) values measured from the
// frame's entry stackBase — too few OR too many is the return-count type_error.
// The re-entrant closure / fn-root RET (no frame) runs on a fresh stack and only
// the underflow is a count error there (a surplus is the higher-order caller's
// domain) — preserving the prior closure behaviour. Returns nil when satisfied.
func checkReturnContract(r *Registry, fn *CompiledFn, stack []Value, stackBase int, hasFrame bool) error {
	rets := fn.Returns
	if len(rets) == 0 {
		return nil
	}
	if hasFrame {
		if produced := len(stack) - stackBase; produced != len(rets) {
			return vmReturnCountErr(r, fn.Name, len(rets), produced)
		}
	} else if len(stack) < len(rets) {
		return vmReturnCountErr(r, fn.Name, len(rets), len(stack))
	}
	base := len(stack) - len(rets)
	for k, exp := range rets {
		if !stack[base+k].Is(CanonicalType(r, exp)) {
			return vmReturnTypeErr(r, fn.Name, k+1, exp, stack[base+k])
		}
	}
	return nil
}

// vmErrAt builds an internal_error AqlError for a VM-internal soundness
// violation (a simulated/runtime stack disagreement the lowerer thought
// impossible). It carries the AQL taxonomy code — so a direct RunProgram
// caller and error-scraping tooling see a structured error, not a raw Go
// string — and RunCompiled treats it as a fall-back-to-interpreter signal.
// Reaching one is a compiler bug; the message keeps the pc/source detail.
// vmMakeMap pops the values of an OpMakeMap assembly off the top of stack and
// returns the stack with the assembled map pushed: the deepest of the popped
// run is value 0, paired with Keys[0]. Extracted from vmContext.run to keep that
// loop's cyclomatic complexity bounded.
func vmMakeMap(p *Program, stack []Value, arg int32, debug []SrcPos, pc int) ([]Value, error) {
	spec := p.MakeMaps[arg]
	n := len(spec.Keys)
	if len(stack) < n {
		return nil, vmErrAt(debug, pc, "MAKE_MAP stack underflow")
	}
	vals := stack[len(stack)-n:]
	om := NewOrderedMap()
	om.Implicit = spec.Implicit
	for i, k := range spec.Keys {
		om.Set(k, vals[i])
	}
	return append(stack[:len(stack)-n], NewMap(om)), nil
}

func vmErrAt(debug []SrcPos, pc int, msg string) error {
	pos := SrcPos{}
	if pc >= 0 && pc < len(debug) {
		pos = debug[pc]
	}
	return makeAqlErrorAt("internal_error",
		fmt.Sprintf("bytecode: internal: %s (pc=%d, src %d:%d)", msg, pc, pos.Row, pos.Col),
		"", "", "", pos)
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
