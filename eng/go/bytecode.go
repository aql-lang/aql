package eng

import (
	"fmt"
	"strings"
)

// Bytecode Program model — Stage 1 of design/aql-bytecode-plan.0.md.
//
// A Program is the flat, linear lowering of the typed region the
// carrier checker resolved: literal pushes, fixed-arity native calls
// with the checker-selected signature baked in, and (Stage 1) the
// occasional SWAP the operand layout needs. There is no VM yet — the
// Stage 1 gate is structural: a disassembler plus golden tests that
// the recording pass (emit.go) lowers each accepted source form to
// the expected instruction stream, with the three call forms
// (`add 1 2` ≡ `1 add 2` ≡ `1 2 add`) producing identical bytecode.
//
// Stack convention: matches the kernel's one argument convention —
// position 0 is the top of stack. CALL_NATIVE pops len(sig.Args)
// values, the top being sig position 0, and pushes the single
// result. Operand emission orders pushes so that layout holds.

// Opcode identifies one VM instruction.
type Opcode uint8

const (
	// OpPushConst pushes Consts[Arg].
	OpPushConst Opcode = iota + 1
	// OpSwap exchanges the top two stack values.
	OpSwap
	// OpCallNative pops Sigs[Arg].Sig's arity, calls, pushes one result.
	OpCallNative
	// OpJmp jumps to the absolute pc in Arg (forward-only until the
	// loop stage lands the step budget).
	OpJmp
	// OpJmpIfFalse pops the condition and jumps to Arg when it is
	// falsy under the engine's CoerceBoolean — the same truthiness
	// stepMove applies to a MoveIf condition.
	OpJmpIfFalse
	// OpPushLocal pushes locals[Arg] (loop iterator bindings).
	OpPushLocal
	// OpForSetup pops the iteration count and opens a counted loop
	// whose iterator writes locals[Arg].
	OpForSetup
	// OpForNext advances the innermost loop: binds the iterator and
	// falls through into the body, or closes the loop and jumps to
	// Arg when exhausted. The body's trailing JMP back to this
	// instruction is the program's only back-edge.
	OpForNext
	// OpCallUser pops Fns[Arg].NParams args (sig position 0 on top)
	// into a fresh frame's locals and enters the unit.
	OpCallUser
	// OpTailCallUser binds args like OpCallUser but REPLACES the
	// current frame — the language's tail-call guarantee: self and
	// mutual tail recursion run in O(1) frames.
	OpTailCallUser
	// OpRet pops the frame; the unit's single result stays on the
	// shared operand stack for the caller.
	OpRet
	// OpPushType pushes the type literal for Types[Arg], resolved
	// through the registry's TypeTable at RUN time — type nodes are
	// never pooled as constants because a by-value copy goes stale
	// against the canonical pointer (eng/go/CLAUDE.md, Canonical
	// *Type Pointers); the ID lookup always yields the canonical
	// node, including types the check pass minted (def Foo …).
	OpPushType
	// OpFallback runs Fallbacks[Arg] as an interpreter island: a
	// construct the checker could not type (a dynamic dispatch, a
	// code-body higher-order word) re-executes through a sub-engine
	// over its recorded tokens, threading the operand stack. The
	// compiled code on either side keeps running — this is Stage 5's
	// span-level FALLBACK_INTERP (plan §Stage 5). NIn values are
	// popped (top of stack = sig position NIn-of-the-span's first
	// threaded arg) and pushed onto the island; the island's residual
	// is pushed back.
	OpFallback
	// OpPushClosure pushes a Closure VALUE over the compiled body unit
	// Fns[Arg] (Value{Parent: TFunction, Data: ClosurePayload}). A
	// higher-order word's CODE body compiles to its own unit and rides
	// as the word's operand; at run time the word's native handler
	// invokes it through the VM's re-entrant runner via the InvokeBody
	// seam — never the interpreter (plan P2). Capture-carrying closures
	// take their captures from the operand stack below the closure (not
	// yet emitted; capture-free bodies only for now).
	OpPushClosure
	// OpCallNativePoly dispatches PolyRefs[Arg].Word at RUN time: it runs
	// the kernel's own MatchSignature over the word's signatures against the
	// Arity values on top of the stack — the SAME first-match the
	// interpreter takes — then calls the matched handler. Used where the
	// checker could not commit to one overload (a dynamic operand the
	// checker widened to Any), so the VM selects faithfully at run time
	// instead of islanding through a sub-engine (plan P3). Pops Arity,
	// pushes one result; a no-match raises signature_error.
	OpCallNativePoly
	// OpCallDynamic applies a runtime FUNCTION value to Arg trailing args:
	// the value sits below the Arg args on the stack. If it is callable (a
	// compiled closure, or an FnDef the interpreter auto-applies — a method
	// field like `r.int`), it is invoked and the result replaces value+args;
	// if it is NOT callable, the value and args stay on the stack unchanged
	// (matching the interpreter, which leaves a non-callable residual). This
	// is the fn-value-call boundary (plan P4): a dynamic value preceding
	// residual args.
	OpCallDynamic
	// OpStoreLocal POPS the top of stack into locals[Arg]. Emitted right
	// after the event that produces a COMPUTED value referenced more than
	// once — a value-`def` bound to an event result and used several times
	// (`def a (make …) a eq a`). The single VM-stack copy would be consumed
	// by the first use; storing it once and re-pushing each reference via
	// PUSH_LOCAL gives the lowerer's single-consume stack discipline a sound
	// multiply-referenced value (the carrier-identity item's value-def
	// locals).
	OpStoreLocal
	// OpDrop pops and discards the top stack value. Emitted on the TRUE path of
	// a computed-else `if cond [then] (expr)`: the else value `(expr)` is
	// eagerly computed onto the stack before the branch, so the taken (then)
	// path drops it before running the then-body, while the false path leaves
	// it as the branch result.
	OpDrop
	// OpMakeList pops Arg values off the top of the stack and pushes a single
	// list of them, preserving order (deepest of the Arg becomes element 0). It
	// lowers a list literal whose elements are COMPUTED (`[1 add 2]` -> `[3]`,
	// `[1 (2 add 3) 4]` -> `[1 5 4]`): the elements are evaluated onto the stack,
	// then assembled. A fully-literal list (`[1 2 3]`) stays a pooled const and
	// never needs this.
	OpMakeList
	// OpMakeMap pops the values of a COMPUTED map literal off the top of the
	// stack and assembles them with the key list held in Program.MakeMaps[Arg]
	// into a single map (Keys[i] ← the i-th value, deepest of the popped run
	// becomes value 0). It lowers `make`'s construction body whose field VALUES
	// are computed and not bakeable as an inert const — `make Outer {i:(make
	// Inner …)}` — so the inner instance is freshly built each run rather than a
	// frozen, aliasable const. A fully-literal / const-foldable map stays a
	// pooled const and never needs this.
	OpMakeMap
	// OpTrap raises the AQL error described by Program.Traps[Arg] and aborts the
	// run. It is the compiled form of a check-mode-suppressed runtime error: a
	// word that is deliberately lenient in check mode but raises at run time (an
	// orphan `gen [...]`, an `unpack` of a missing key). The checker is lenient,
	// so the compiled stream would otherwise silently succeed where the
	// interpreter errors; instead the trap raises the byte-identical error
	// (shared taxonomy text) at exactly the point execution reaches it. Terminal:
	// the recorder ends the program at the trap (everything after is unreachable).
	OpTrap
	// OpReverse reverses the top Arg operand-stack values in place. It is the
	// stack-scheduling primitive for an N-operand call whose computed args sit in
	// exact REVERSE signature order — the common forward-call shape `f (a)(b)(c)`,
	// where the args evaluate left→right so sig position 0 ends up DEEPEST, but the
	// call wants it on top. SWAP handles N=2; OpReverse generalises it to N≥3
	// (the 3-deep rotate the VM previously had no opcode for, so layoutOperands
	// refused). Emitted only when layoutOperands recognises an exact reverse, so
	// it can never seat an operand wrongly.
	OpReverse
	// OpCallDynamicTrailing applies a runtime FUNCTION value to the Arg values
	// BENEATH it — the source shape where the fn TRAILS its argument (`5 m.f`,
	// where the fn `m.f` produces auto-applies to the 5 beneath it; `[..] r.one-of`).
	// The recorder lays the operands out exactly like OpCallDynamic — fn at the
	// base, the Arg args above — so the callable path is identical. The ONLY
	// difference is the NON-callable residual: where leading OpCallDynamic leaves
	// [fn, args] (fn below), the interpreter's trailing form leaves the fn ON TOP
	// of its args, so this op rotates the fn back above its args when the value
	// turns out not to be callable — keeping the residual byte-identical to the
	// interpreter. Bounded to Arg==1: with >1 arg the island's forward collection
	// would order them opposite to the interpreter's top-down stack collection.
	OpCallDynamicTrailing
	// OpFlowBreak / OpFlowContinue lower a break / continue that sits in a FN
	// BODY with no enclosing loop in its OWN unit — a flow-control signal that
	// targets the nearest loop in an ANCESTOR frame (the interpreter's
	// cross-frame FlowCtrl, compiled). The VM unwinds the frames opened since
	// that loop, discards the current iteration's partial operand pushes (down
	// to the loop's iteration base — matching the interpreter, which drops the
	// values between the loop mark and the signal), then jumps to the loop's
	// exit (break) or its FOR_NEXT (continue). A same-unit break/continue stays
	// a static JMP (lowerBreak / lowerContinue) — these ops are the cross-frame
	// case only. Reaching one with no open loop at all is a "break outside loop"
	// runtime error, surfaced as an internal_error so RunCompiled falls back and
	// the interpreter raises the canonical taxonomy.
	OpFlowBreak
	OpFlowContinue
	// OpStackMark / OpDropToMark / OpPopMark implement a VARIADIC stack region for
	// the chained variadic-statement-`if` (a 2-arg `if` whose 0-or-1 result is
	// claimed as the else of a following `if`). The stack depth of a 0-or-1 result
	// is only known at run time, so a fixed-offset DROP cannot remove it. OpStackMark
	// pushes the current stack depth onto a per-run mark stack (emitted before the
	// variadic producer's region). On the claiming if's TRUE path, OpDropToMark pops
	// the mark and truncates the stack to it (discarding the 0-or-1 eager regardless
	// of its count); on the FALSE path, OpPopMark discards the mark and keeps the
	// eager as the result. The merged result is itself a 0-or-1 the program residual
	// absorbs. Arg is unused.
	OpStackMark
	OpDropToMark
	OpPopMark
	// OpCallDynamicMixed handles the MIXED fn-value-call boundary: a runtime
	// FUNCTION value sitting INTERIOR to the program residual, with static args
	// both BELOW it and ABOVE it (`3 m.f 2` — `m.f` is a 2-arg fn collecting the
	// forward `2` into sig[0] and the stack `3` into sig[1]). Neither the leading
	// (OpCallDynamic) nor the trailing (OpCallDynamicTrailing) layout fits, since
	// args straddle the fn. Arg is the WINDOW size (everything from the deepest
	// before-arg through the last after-arg, inclusive of the fn). The VM islands
	// that window verbatim — it is the same token sequence the interpreter ran, so
	// the fn auto-applies (or, if the value is not callable, stays put) with full
	// fidelity for any arity and any before/after split. The recorder promotes the
	// fn's producing event to a frame local so the residual re-pushes in source
	// order (the fn sits above a before-arg literal, which the in-order
	// reconciliation otherwise forbids).
	OpCallDynamicMixed
	// OpInterp assembles a template string (`` `got ${x}` ``) from its
	// computed holes. Program.Interps[Arg] holds the ordered template — literal
	// segments interleaved with HOLE markers. The VM pops one operand-stack value
	// per hole (deepest popped = hole 0, source order), then walks the template:
	// a literal segment appends verbatim, a hole appends ValToString of the next
	// popped value — byte-identical to the interpreter's evalInterpParts. Lowers
	// any interpolated string whose holes are runtime- or natively-computed
	// (`${1 add 2}`, `${x}` for a fn param, `${typeof x}`); a fully literal or
	// pure-binding-fold template stays a pooled const and never needs this.
	OpInterp
	// OpCallUserPoly dispatches a MULTI-OVERLOAD user fn at RUN time: it runs
	// the kernel's own MatchSignature over the recorded same-arity overload
	// subset of UserPolys[Arg].Word against the Arity values on top of the
	// stack — the SAME first-match the interpreter takes — then enters the
	// matched overload's compiled body unit exactly as OpCallUser would
	// (pop the args into frame locals, checkParamContract, push a frame).
	// The user-fn mirror of OpCallNativePoly: used where a gradual-Any arg
	// made the dispatch ambiguous at check time (two or more same-arity
	// overloads reachable), so the VM selects the arm faithfully at run time
	// instead of refusing the whole program. A no-match (or any drift between
	// the recorded overload set and the word's live dispatch table) defers to
	// the interpreter through the whole-program fallback, which raises the
	// canonical signature_error / runs the live definition.
	OpCallUserPoly
	// OpCallDynTrailTop applies a runtime FUNCTION value sitting ON TOP of its Arg
	// args to those args — the TRAILING fn-value-call boundary BOUNDED BY A PAREN
	// (`(prev key comp)`, a captured/param comparator applied to two computed args).
	// Unlike OpCallDynamicTrailing (recorder lays the fn at the BASE and rotates the
	// non-callable residual, bounded to 1 arg), the fn stays on TOP here, so NO
	// rotation is ever needed and it is sound for ANY arity: the args are laid out
	// in source/stack order with the fn above them; on apply the fn auto-applies to
	// the Arg args beneath it exactly as the interpreter's paren auto-dispatch (the
	// fn-on-top order matches the interpreter's top-down stack bind); if the value
	// is NOT callable, [args, fn] is ALREADY the interpreter's trailing residual, so
	// it is left untouched. Arg is the argument count (the paren's value count minus
	// the trailing fn). The recorder captures this at the paren-collapse boundary
	// (engine.go) where the arity is known, since the flattened residual cannot
	// recover the paren-group size.
	OpCallDynTrailTop
	// OpCallDynApplyTop is OpCallDynTrailTop with the `apply` WORD's semantics
	// (Stage M2a): the fn value on top came through `…args fn apply`, whose
	// interpreter handler (applyHandler) UNQUOTES the value before re-stepping
	// it — so a /r-parked (Quoted) fn value STILL applies here, where the
	// paren-bounded OpCallDynTrailTop would leave it as data. The fn is popped,
	// unquoted, and applied to the Arg args beneath it (top arg → first param,
	// the same reversed-window forward bind as OpCallDynTrailTop); a compiled
	// closure runs VM-native. A non-fn payload raises the applyHandler's own
	// byte-identical error. Emitted for a fn-body-tail `apply` over a
	// Function-typed carrier (StartFnCompile's pendingApply) and for the
	// paren-bounded RecordDynApply event when the apply word drove it.
	OpCallDynApplyTop

	// OpPushConstFresh pushes a deep clone of Consts[Arg] with fresh container
	// identity (CloneValue) instead of the pooled instance. A compound VALUE
	// literal written in a fn body is re-EVALUATED per call by the interpreter,
	// constructing a fresh List/Map each time — so a pooled const pushed from a
	// compiled fn unit must not leak one shared identity across calls
	// (`def mk fn [[] [List] [[1]]]  (mk) eq (mk)` must stay false; miscompile
	// mechanism A, design/MISCOMPILE-HUNT-FINDINGS.0.md §A). The finalize pass
	// rewrites OpPushConst → OpPushConstFresh in place (freshenFnUnitConsts,
	// emit.go) exactly for fn-unit consts that (a) materialised from a literal
	// written in the body — an ENCLOSING binding's value read by name keeps the
	// shared push, matching the interpreter's one-instance-per-binding
	// semantics — and (b) have a single push site, so within-call reads of one
	// binding still share one instance. Main-unit pushes stay bare: top-level
	// straight-line code evaluates each literal once, and loop-spread
	// consumption refuses before lowering. Captured containers arrive through
	// capture slots (locals), never consts, so closure identity is preserved.
	OpPushConstFresh

	// OpBindTyped is the runtime validate/reparent step of a typed value-def
	// (`def x:Pos n`) whose constraint is a REFINEMENT — a predicate type, a
	// bare-refine newtype, or an inline/named DepScalar subset — and whose body
	// is DYNAMIC (a param / computed carrier). It pops the body value, runs the
	// SAME membership check the interpreter's defTypedHandler runs (RunPredicate
	// / Unify against the builtin ancestor / the self-contained DepScalar
	// predicate — see RunTypedBind, typed_bind.go), raising the byte-identical
	// (position-less, plain — exactly what the interpreter surfaces) error on
	// failure, then pushes the value the interpreter would bind — reparented via
	// ReparentValue where the interpreter reparents, so a downstream typeof /
	// sig dispatch sees the refined tag. The STORE half of the binding stays
	// with the ordinary value-def machinery (planValueDefLocals promotes a
	// multiply-read binding to a frame local via OpStoreLocal; a dead binding
	// drops the result AFTER the validation ran, matching the interpreter, which
	// validates even a never-read typed def). Arg indexes Program.TypedBinds.
	// A STATIC (concrete) typed-def body never emits this — its reparent rides
	// the const pool, unchanged.
	OpBindTyped

	// OpCallDynMethod is the GUARDED mid-stream shaped-instance-method apply
	// (Stage M2c): a method read from a shape-instance container (a logger /
	// span / instrument / rand handle, whose check-mode ReturnsFn instance
	// resolves the member's SIGNATURES but whose runtime instance carries
	// per-call state — the freeze-gate) is applied to a statically-known
	// window of inert args at the exact point the interpreter auto-dispatches
	// it. Stack layout is the LEADING boundary's ([fn, a1..aN] with aN on
	// top, forward order); Arg indexes Program.DynMethods, whose spec pins
	// the claimed arity AND the claimed result count. Unlike OpCallDynamic
	// (residual position, count-unchecked, non-callable stays data), the
	// program CONTINUES past this op with NOut results committed downstream,
	// so every shape-claim failure — a non-callable/quoted runtime value, or
	// a result count differing from the claim — raises internal_error, which
	// RunCompiled resolves by re-running the interpreter (slow, not wrong;
	// runtimeShouldFallback) and --force-compile surfaces loudly. A genuine
	// AQL error raised by the method surfaces as-is (the interpreter raises
	// the same at the same point, prior side effects included).
	OpCallDynMethod

	// OpLookupDynScope is a DYNAMIC-SCOPE name read: a fn body's word that
	// resolves through no lexical home — not a param, capture, local, const,
	// or module binding the recorder can bake — but which SOME live frame
	// binds at run time (a callee reading the caller's param, a recursive
	// base case reading the previous frame's body-local — recursion.tsv
	// 71/72). Arg indexes Program.Consts (a String holding the name); the VM
	// reads r.Defs.Top(name) exactly as the interpreter's stepWord
	// substitution does. A miss, or a binding the substitution would NOT
	// push as a simple value (an FnDef/class dispatch, a splice/reach
	// marker), defers to the interpreter via internal_error
	// (runtimeShouldFallback — slow, not wrong). The binder side is
	// OpBindDynScope: the whole-program lowering pass installs it in every
	// unit that binds a dynamically-read name.
	OpLookupDynScope

	// OpBindDynScope makes a frame binding REGISTRY-VISIBLE for dynamic-scope
	// readers: it pops the top value and installs it under the name in
	// Program.Consts[Arg] via InstallDef — the same installer the
	// interpreter's `def` runs — recording the name's prior depth so the
	// frame's exit (RET) truncates the binding stack back, exactly the
	// interpreter's def-cleanup discipline. Emitted only for names some
	// OpLookupDynScope reads (the DynScopeNames set), at the def site for
	// body-locals and at unit entry for params; top-level binds are never
	// popped (the interpreter leaves top-level defs installed too). A unit
	// containing binds never TAIL-calls (the interpreter keeps the frame's
	// bindings live across the call in its body tail), so bindings stack
	// per-activation and pop innermost-first.
	OpBindDynScope
)

// opcodeNames is the single source of each opcode's disassembler mnemonic,
// indexed by the opcode value so the name and the iota order cannot drift. The
// disassembler renders the mnemonic via Op.String() and switches separately
// only to format each opcode's ARGUMENT (a different concern — it reads the
// program's const/sig/type/fallback pools).
var opcodeNames = [...]string{
	OpPushConst:           "PUSH_CONST",
	OpSwap:                "SWAP",
	OpCallNative:          "CALL_NATIVE",
	OpJmp:                 "JMP",
	OpJmpIfFalse:          "JMP_IF_FALSE",
	OpPushLocal:           "PUSH_LOCAL",
	OpForSetup:            "FOR_SETUP",
	OpForNext:             "FOR_NEXT",
	OpCallUser:            "CALL_USER",
	OpTailCallUser:        "TAIL_CALL_USER",
	OpRet:                 "RET",
	OpPushType:            "PUSH_TYPE",
	OpFallback:            "FALLBACK",
	OpPushClosure:         "PUSH_CLOSURE",
	OpCallNativePoly:      "CALL_NATIVE_POLY",
	OpCallDynamic:         "CALL_DYNAMIC",
	OpStoreLocal:          "STORE_LOCAL",
	OpDrop:                "DROP",
	OpMakeList:            "MAKE_LIST",
	OpMakeMap:             "MAKE_MAP",
	OpTrap:                "TRAP",
	OpReverse:             "REVERSE",
	OpCallDynamicTrailing: "CALL_DYNAMIC_TRAILING",
	OpFlowBreak:           "FLOW_BREAK",
	OpFlowContinue:        "FLOW_CONTINUE",
	OpStackMark:           "STACK_MARK",
	OpDropToMark:          "DROP_TO_MARK",
	OpPopMark:             "POP_MARK",
	OpCallDynamicMixed:    "CALL_DYNAMIC_MIXED",
	OpInterp:              "INTERP",
	OpCallUserPoly:        "CALL_USER_POLY",
	OpCallDynTrailTop:     "CALL_DYN_TRAIL_TOP",
	OpCallDynApplyTop:     "CALL_DYN_APPLY_TOP",
	OpPushConstFresh:      "PUSH_CONST_FRESH",
	OpBindTyped:           "BIND_TYPED",
	OpCallDynMethod:       "CALL_DYN_METHOD",
	OpLookupDynScope:      "LOOKUP_DYN_SCOPE",
	OpBindDynScope:        "BIND_DYN_SCOPE",
}

func (o Opcode) String() string {
	if int(o) < len(opcodeNames) && opcodeNames[o] != "" {
		return opcodeNames[o]
	}
	return fmt.Sprintf("OP(%d)", uint8(o))
}

// PolyRef names one runtime-dispatched native call: the word and the arity
// (operand count) the checker fixed at the call site. OpCallNativePoly runs
// MatchSignature over the word's signatures against that many stack values.
type PolyRef struct {
	Word  string
	Arity int
	// NOut is the result-count CLAIM the recorder's stack model committed
	// downstream ops to. The runtime re-match may land on a DIFFERENT
	// overload than the checker's model (that is poly's point), and when
	// that overload's result count differs from the claim the program's
	// stack layout no longer holds — the VM defers to the interpreter via
	// internal_error (runtimeShouldFallback: slow, not wrong) instead of
	// silently shifting every downstream operand.
	NOut int
	// Reg is the sub-registry whose signatures the VM re-matches a MODULE poly
	// word over (`StructUtil.getpath` — a sub-registry word). Nil means the main
	// registry (the common case: a core builtin like get/size/is). The pointer
	// is the same sub-registry the check pass created on the shared registry, so
	// it stays valid for the compiled run (RunProgram runs on that registry).
	Reg *Registry
}

// UserPolyRef names one runtime-dispatched multi-overload USER-FN call: the
// word, the arity the checker fixed at the call site, and the recorded
// same-arity overload arms — SigIdx[i] indexes the word's aggregated dispatch
// table (Registry.Lookup(Word).Signatures), Units[i] is the arm's compiled
// body unit in Program.Fns, and Impls[i] is the arm's run-implementation
// identity (Signature.Impl is a stable pointer per installed overload). The VM
// re-derives the arm subset from the LIVE table at run time and verifies each
// entry's Impl against Impls — any drift (a re-def between the compile and the
// run) defers to the interpreter rather than running a stale body unit.
// The user-fn mirror of PolyRef (OpCallNativePoly).
type UserPolyRef struct {
	Word  string
	Arity int
	// Reg is the registry the check pass dispatched the word against (a module
	// fn re-matches over its own sub-registry). Nil means the VM's registry.
	// Like PolyRef.Reg, the pointer is the same registry the check pass ran on,
	// so it stays valid for the compiled run.
	Reg    *Registry
	SigIdx []int
	Units  []int
	Impls  []SigImpl
}

// ClosureInShape tags HOW a higher-order word must present each per-invocation
// input to a compiled closure — the divergence the per-word callback
// conventions create. A map-iteration handler (each/fold/scan over a map) runs
// BOTH a token-quotation body (sees the bare value) and a lambda body (sees a
// KeyVal {k v i n}); the closure value alone cannot say which, so the compile
// records the shape on the unit and the handler reads it back here. The
// unambiguous handlers (filter list/map) build their own fixed shape and ignore
// it.
type ClosureInShape uint8

const (
	// ClosureInValue passes the per-invocation inputs through unchanged — the
	// token-quotation form (list element / map value, plus fold's accumulator).
	ClosureInValue ClosureInShape = iota
	// ClosureInKeyVal wraps a map entry as a KeyVal {k v i n} before the (last)
	// input — the map-iteration LAMBDA convention (`each (kv => …) {m}`).
	ClosureInKeyVal
)

// ClosurePayload is a runtime fn VALUE backed by a compiled body unit:
// OpPushClosure pushes one, and a higher-order word's native handler invokes
// it through the VM's re-entrant runner (via the InvokeBody seam) — never the
// interpreter (plan P2). Unit indexes Program.Fns; Captures are the
// construction-time lexical captures bound into the body's trailing local
// slots at invocation (empty for a capture-free body). InShape carries the
// unit's input convention (copied from CompiledFn at OpPushClosure) so the
// driving handler shapes each input correctly.
type ClosurePayload struct {
	Unit     int
	Captures []Value
	InShape  ClosureInShape
}

// NewClosure builds a closure Value over a compiled body unit (default value
// input shape). The VM stamps the unit's real InShape at OpPushClosure.
func NewClosure(unit int, captures []Value) Value {
	return Value{Parent: TFunction, Data: ClosurePayload{Unit: unit, Captures: captures}}
}

// ClosureWantsKeyVal reports whether v is a compiled closure whose body expects
// a map entry presented as a KeyVal (the map-iteration lambda convention), so a
// map-iteration handler wraps the entry rather than passing the bare value.
func ClosureWantsKeyVal(v Value) bool {
	cl, ok := v.Data.(ClosurePayload)
	return ok && cl.InShape == ClosureInKeyVal
}

// IsCompiledClosure reports whether v is a compiled-closure VALUE (a body unit
// the VM runs via InvokeBody), as opposed to an interpreter FnDefInfo lambda.
// Both are Parent=TFunction, so a higher-order handler that treats a lambda
// differently from a plain code body (e.g. map iteration hands a lambda a
// KeyVal but a body the value) must discriminate on this.
func IsCompiledClosure(v Value) bool {
	_, ok := v.Data.(ClosurePayload)
	return ok
}

// Instr is one fixed-width instruction.
type Instr struct {
	Op  Opcode
	Arg int32
}

// SigRef names one interned signature: the word plus the exact
// *Signature the checker selected at the call sites that reference it.
type SigRef struct {
	Word string
	Sig  *Signature
	// Guard marks a CALL_NATIVE recorded for a dispatch the checker could NOT
	// statically commit (a concrete-mismatch / Any-carrier recovery over a
	// SINGLE-overload native) — the compiled mirror of the interpreter's runtime
	// matchSignature. The VM re-checks the concrete args against Sig.Args before
	// the handler (checkNativeParamContract): on match it dispatches Sig.Handler
	// (== the interpreter's sole-overload dispatch), on mismatch it raises the
	// byte-identical signature_error (== the interpreter, which finds no overload).
	// Sound ONLY for a single-overload word: with a sibling overload a runtime arg
	// that misses Sig could match the sibling, where the interpreter dispatches but
	// the guard raises. Unlike OpCallNativePoly (which re-matches ALL overloads and
	// can OPTIMISTICALLY dispatch a sibling the interpreter rejects — proven unsound
	// for the concrete-mismatch case), the guard never re-matches: it commits to the
	// one sig and raises otherwise, so it diverges from the interpreter only if a
	// sibling exists — which the single-overload gate forbids.
	Guard bool
}

// TypeRef names one type operand: the canonical type ID (resolved
// through the registry at run time) plus the display name for the
// disassembler.
type TypeRef struct {
	Name string
	ID   string
}

// FallbackSpan is one interpreter island: a recorded token sequence
// the VM re-runs through a sub-engine for a construct the compiler
// could not lower (Stage 5 FALLBACK_INTERP). NIn operand-stack values
// are popped and pre-loaded onto the island (deepest first), the
// island runs Tokens, and its residual is pushed back. Desc is a
// human label for the disassembler.
type FallbackSpan struct {
	Tokens []Value
	NIn    int
	Desc   string
}

// MakeMapSpec names the key list (and the source map's Implicit flag) of one
// OpMakeMap assembly: the VM pops len(Keys) values and pairs Keys[i] with the
// i-th value (deepest popped = value 0). The keys ride here rather than as
// stack operands so OpMakeMap only handles the VALUE operands (which may be
// computed event results), reusing the same operand-layout engine as a call.
type MakeMapSpec struct {
	Keys     []string
	Implicit bool
}

// InterpSeg is one segment of an OpInterp template: either a literal run of
// text (Hole false, Lit set) or a hole that consumes the next popped value
// (Hole true). Segments are in source order.
type InterpSeg struct {
	Lit  string
	Hole bool
}

// InterpSpec is the template of one OpInterp assembly: the ordered segments
// and the hole count (= how many operand-stack values the op pops). The
// literal text rides here rather than as stack operands, so OpInterp only
// pops the computed hole VALUES — reusing the same operand-layout engine as a
// call (mirrors MakeMapSpec).
type InterpSpec struct {
	Segs   []InterpSeg
	NHoles int
}

// TypedBindKind selects which of defTypedHandler's refinement branches one
// OpBindTyped mirrors at run time. Explicit non-zero values (iota+1) so the
// struct zero value is an invalid kind that RunTypedBind rejects loudly, never
// a silently-valid predicate bind (No-Zero-Value-Overload, eng/go/CLAUDE.md).
type TypedBindKind uint8

const (
	// TypedBindPredicate: the constraint is a predicate TYPE (an fn body —
	// `def Positive fn [[n:Integer] [Boolean] [n gt 0]]`). Runtime runs
	// RunPredicate over the value; Def (when non-nil) is the minted type the
	// passing value reparents to (the interpreter reparents only for a NAMED
	// non-builtin predicate type with a concrete declared input).
	TypedBindPredicate TypedBindKind = iota + 1
	// TypedBindRefine: the constraint is a bare-refine NEWTYPE (`def Pos
	// (refine Integer)`). Runtime unifies the value against Def's nearest
	// builtin ancestor, then reparents to Def.
	TypedBindRefine
	// TypedBindDepScalar: the constraint is a DepScalar subset (`(Integer gt
	// 10)`, inline or named). Runtime unifies the value against the
	// self-contained Constraint; the value keeps its base tag (no reparent).
	TypedBindDepScalar
)

// TypedBindSpec describes one OpBindTyped: the typed-def name (for the error
// text), the rendered constraint (Describe — the annotation NAME when the
// constraint was named, else the constraint's rendering, mirroring
// defTypedHandler's describeType), the reparent target type, and the
// constraint VALUE for the kinds that validate against a self-contained value
// (the predicate fn / the DepScalar). Def rides as a *Type pointer — the same
// convention CompiledFn.Returns/Params use — resolved through CanonicalType at
// run time, so a type minted in a module sub-registry (whose ID the main
// TypeTable does not know) still reaches its live canonical node.
type TypedBindSpec struct {
	Kind     TypedBindKind
	Name     string
	Describe string
	Def      *Type  // reparent target: TypedBindRefine always; TypedBindPredicate when the interpreter reparents; nil otherwise
	Cons     *Value // constraint value: TypedBindPredicate (the fn) and TypedBindDepScalar (the DepScalar); nil for TypedBindRefine
}

// TrapSpec describes the AQL error one OpTrap raises: the taxonomy code, the
// detail message, the word it is attributed to, and an optional hint — built
// from the SAME strings the interpreter raises for the matching runtime error,
// so error-scraping tooling can never tell which engine ran. It lowers a
// check-mode-suppressed runtime error (an orphan gen, an unpack of a missing
// key) into the compiled stream rather than refusing the whole program.
type TrapSpec struct {
	Code   string
	Detail string
	Word   string
	Hint   string
}

// DynMethodSpec is one OpCallDynMethod's shape claim (Stage M2c): the member
// word name (diagnostics only — dispatch is over the runtime VALUE, never the
// name), the arity the check-mode match consumed, and the result count the
// matched member signature declares. The VM enforces both halves of the claim
// and defers to the interpreter via internal_error when either fails.
type DynMethodSpec struct {
	Word  string
	NArgs int
	NOut  int
}

// Program is a compiled unit: code, interned constants, the signature
// table, a pc → source-position map, and the precomputed stack bound.
type Program struct {
	Code       []Instr
	Consts     []Value
	Types      []TypeRef
	Sigs       []SigRef
	PolyRefs   []PolyRef
	UserPolys  []UserPolyRef
	Fallbacks  []FallbackSpan
	MakeMaps   []MakeMapSpec
	Interps    []InterpSpec
	Traps      []TrapSpec
	TypedBinds []TypedBindSpec
	DynMethods []DynMethodSpec
	Fns        []CompiledFn
	Debug      []SrcPos // 1:1 with Code
	MaxStack   int      // a floor when the program loops (results accumulate)
	NumLocals  int
}

// CompiledFn is one compiled AQL fn overload at one arg shape: its
// own code unit with frame-relative locals (params in slots 0..N-1,
// sig order).
type CompiledFn struct {
	Name    string
	NParams int
	// NCaptures is how many of the NParams leading slots are CAPTURES (for a
	// closure body unit): the per-invocation inputs fill slots
	// 0..NParams-NCaptures-1, the captures fill the trailing NCaptures slots.
	// OpPushClosure pops NCaptures values off the stack into the closure's
	// captures; 0 for an ordinary user fn or a capture-free body. (For a user
	// fn the captures still ride as trailing CALL_USER args, so NCaptures
	// stays 0 there — it is closure-specific.)
	NCaptures int
	// NUnnamed is how many of the params are UNNAMED (stack-flowing): the
	// lowering re-pushes each unnamed param onto the operand stack at unit
	// entry (mirroring the interpreter's frame, where unnamed args sit
	// resolved at the body's bottom), so an unnamed arg the body never
	// consumes remains in the frame region at RET. The RET contract
	// (checkReturnContract) DISCARDS up to NUnnamed extra bottom values —
	// the exact __RC discipline (engine.go: extras beyond the declared
	// returns are tolerated up to the unnamed-arg allowance, then trimmed).
	// 0 for all-named fns and closures.
	NUnnamed int
	NLocals  int
	// InShape is the input convention a closure over this unit presents to its
	// driving handler (ClosureInValue for an ordinary fn / token body, or
	// ClosureInKeyVal for a map-iteration lambda body). Copied onto the
	// ClosurePayload at OpPushClosure. The VM never branches on it; the native
	// map-iteration handler reads it via ClosureWantsKeyVal.
	InShape ClosureInShape
	Code    []Instr
	Debug   []SrcPos
	// Returns are the declared return types, enforced at RET against the
	// body's result the same way the interpreter's ReturnCheck (__RC)
	// does — via v.Is(exp), so a predicate refine runs its predicate, a
	// bare refine stays nominal, and builtins are unchanged. Empty for a
	// fn with no declared return (no check runs).
	Returns []*Type
	// Params are the declared PARAM types (param slots 0..len(Params)-1, which
	// align with the leading param locals; captures, if any, follow and are NOT
	// listed). Enforced at CALL_USER entry against the incoming arg the same way
	// Returns is enforced at RET — via v.Is(exp). This guards the gradual-Any
	// boundary: a value of static type exactly Any optimistically matches a
	// concrete param at check time, but the runtime value may not match; the
	// interpreter runtime-matches it, so the compiled OpCallUser must too, else a
	// laundered List bound to an `m:Map` param silently runs the body. A nil
	// entry (a closure's [Any] input) is a guaranteed-pass, like Returns=[Any].
	Params []*Type
	// ParamPatterns are the per-param structural/value patterns (FnParam.Pattern):
	// an inline disjunct (`x:(Integer tor String)`), inline predicate
	// (`b:(Integer gt 10)`), bounded (`x:Map/t`), or map/list shape — the
	// constraint that rides in Pattern, NOT Type (Type is left a loose root for
	// these). Checked at CALL_USER alongside Params via the SAME OpenUnifyMap /
	// Unify the interpreter's dispatch runs, so a value laundered past such a
	// param raises the same signature_error rather than running the body. Nil
	// where the param has no pattern.
	ParamPatterns []*Value
	// LocalNames maps a frame local slot to its source name (params in
	// slots 0..NParams-1, then captures), for a debugger / disassembler.
	// Body-local iterator slots have no name (empty string). Purely
	// metadata — the VM never reads it.
	LocalNames []string
}

// slotNames renders a CompiledFn's slot→name table for the
// disassembler: " [n acc]" with empty (anonymous body-local) slots shown
// as "_". Empty string when no names are known.
func slotNames(names []string) string {
	if len(names) == 0 {
		return ""
	}
	parts := make([]string, len(names))
	for i, n := range names {
		if n == "" {
			parts[i] = "_"
		} else {
			parts[i] = n
		}
	}
	return " [" + strings.Join(parts, " ") + "]"
}

// Disassemble renders the program for golden tests and debugging.
func (p *Program) Disassemble() string {
	var sb strings.Builder
	p.disasmUnit(&sb, p.Code)
	for fi := range p.Fns {
		fmt.Fprintf(&sb, "fn f%d %s/%d (locals=%d)%s:\n", fi, p.Fns[fi].Name, p.Fns[fi].NParams, p.Fns[fi].NLocals, slotNames(p.Fns[fi].LocalNames))
		p.disasmUnit(&sb, p.Fns[fi].Code)
	}
	fmt.Fprintf(&sb, "; consts=%d types=%d sigs=%d fallbacks=%d fns=%d max-stack=%d locals=%d",
		len(p.Consts), len(p.Types), len(p.Sigs), len(p.Fallbacks), len(p.Fns), p.MaxStack, p.NumLocals)
	// polyrefs is appended only when present, so the common (no-poly) summary —
	// and every golden asserting it — stays byte-identical while a poly program
	// still surfaces its CALL_NATIVE_POLY table count.
	if len(p.PolyRefs) > 0 {
		fmt.Fprintf(&sb, " polyrefs=%d", len(p.PolyRefs))
	}
	// userpolys likewise appears only when present, keeping the common summary
	// (and every golden asserting it) byte-identical.
	if len(p.UserPolys) > 0 {
		fmt.Fprintf(&sb, " userpolys=%d", len(p.UserPolys))
	}
	sb.WriteByte('\n')
	return sb.String()
}

func (p *Program) disasmUnit(sb *strings.Builder, code []Instr) {
	for i, in := range code {
		fmt.Fprintf(sb, "%04d %-11s", i, in.Op.String())
		switch in.Op {
		case OpPushConst, OpLookupDynScope, OpBindDynScope:
			c := p.Consts[in.Arg]
			fmt.Fprintf(sb, " k%-3d ; %s (%s)", in.Arg, CanonValue(c), c.Parent.Leaf())
		case OpCallNative:
			s := p.Sigs[in.Arg]
			names := make([]string, s.Sig.TotalArgs())
			for j, t := range s.Sig.ArgTypes() {
				names[j] = t.Leaf()
			}
			guard := ""
			if s.Guard {
				guard = " [guarded]"
			}
			fmt.Fprintf(sb, " s%-3d ; %s (%s)%s", in.Arg, s.Word, strings.Join(names, ", "), guard)
		case OpJmp, OpJmpIfFalse, OpForNext:
			fmt.Fprintf(sb, " -> %04d", in.Arg)
		case OpPushLocal, OpForSetup, OpStoreLocal:
			fmt.Fprintf(sb, " l%d", in.Arg)
		case OpPushType:
			fmt.Fprintf(sb, " t%-3d ; %s", in.Arg, p.Types[in.Arg].Name)
		case OpFallback:
			fb := p.Fallbacks[in.Arg]
			fmt.Fprintf(sb, " b%-3d ; %s (nin=%d)", in.Arg, fb.Desc, fb.NIn)
		case OpCallUser, OpTailCallUser:
			fmt.Fprintf(sb, " f%-3d ; %s/%d", in.Arg, p.Fns[in.Arg].Name, p.Fns[in.Arg].NParams)
		case OpPushClosure:
			fmt.Fprintf(sb, " f%-3d ; closure %s/%d", in.Arg, p.Fns[in.Arg].Name, p.Fns[in.Arg].NParams)
		case OpCallNativePoly:
			pr := p.PolyRefs[in.Arg]
			fmt.Fprintf(sb, " p%-3d ; %s/%d (poly)", in.Arg, pr.Word, pr.Arity)
		case OpCallUserPoly:
			up := p.UserPolys[in.Arg]
			fmt.Fprintf(sb, " u%-3d ; %s/%d (user poly, %d arms)", in.Arg, up.Word, up.Arity, len(up.Units))
		case OpCallDynamic:
			fmt.Fprintf(sb, " /%d ; apply fn-value", in.Arg)
		case OpCallDynamicTrailing:
			fmt.Fprintf(sb, " /%d ; apply trailing fn-value", in.Arg)
		case OpMakeList:
			fmt.Fprintf(sb, " n%-3d ; assemble %d into a list", in.Arg, in.Arg)
		case OpMakeMap:
			mm := p.MakeMaps[in.Arg]
			fmt.Fprintf(sb, " m%-3d ; assemble {%s}", in.Arg, strings.Join(mm.Keys, " "))
		case OpTrap:
			fmt.Fprintf(sb, " x%-3d ; trap %s", in.Arg, p.Traps[in.Arg].Code)
		case OpReverse:
			fmt.Fprintf(sb, " n%-3d ; reverse top %d", in.Arg, in.Arg)
		case OpInterp:
			fmt.Fprintf(sb, " i%-3d ; interpolate %d hole(s)", in.Arg, p.Interps[in.Arg].NHoles)
		case OpBindTyped:
			tb := p.TypedBinds[in.Arg]
			fmt.Fprintf(sb, " y%-3d ; typed bind %s:%s", in.Arg, tb.Name, tb.Describe)
		case OpCallDynMethod:
			dm := p.DynMethods[in.Arg]
			fmt.Fprintf(sb, " d%-3d ; %s/%d -> %d (shaped method)", in.Arg, dm.Word, dm.NArgs, dm.NOut)
		}
		sb.WriteByte('\n')
	}
}
