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
)

func (o Opcode) String() string {
	switch o {
	case OpPushConst:
		return "PUSH_CONST"
	case OpSwap:
		return "SWAP"
	case OpCallNative:
		return "CALL_NATIVE"
	case OpJmp:
		return "JMP"
	case OpJmpIfFalse:
		return "JMP_IF_FALSE"
	case OpPushLocal:
		return "PUSH_LOCAL"
	case OpForSetup:
		return "FOR_SETUP"
	case OpForNext:
		return "FOR_NEXT"
	case OpCallUser:
		return "CALL_USER"
	case OpTailCallUser:
		return "TAIL_CALL_USER"
	case OpRet:
		return "RET"
	case OpPushType:
		return "PUSH_TYPE"
	case OpFallback:
		return "FALLBACK"
	case OpPushClosure:
		return "PUSH_CLOSURE"
	case OpCallNativePoly:
		return "CALL_NATIVE_POLY"
	case OpCallDynamic:
		return "CALL_DYNAMIC"
	case OpStoreLocal:
		return "STORE_LOCAL"
	case OpDrop:
		return "DROP"
	}
	return fmt.Sprintf("OP(%d)", uint8(o))
}

// PolyRef names one runtime-dispatched native call: the word and the arity
// (operand count) the checker fixed at the call site. OpCallNativePoly runs
// MatchSignature over the word's signatures against that many stack values.
type PolyRef struct {
	Word  string
	Arity int
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

// Program is a compiled unit: code, interned constants, the signature
// table, a pc → source-position map, and the precomputed stack bound.
type Program struct {
	Code      []Instr
	Consts    []Value
	Types     []TypeRef
	Sigs      []SigRef
	PolyRefs  []PolyRef
	Fallbacks []FallbackSpan
	Fns       []CompiledFn
	Debug     []SrcPos // 1:1 with Code
	MaxStack  int      // a floor when the program loops (results accumulate)
	NumLocals int
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
	NLocals   int
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
	fmt.Fprintf(&sb, "; consts=%d types=%d sigs=%d fallbacks=%d fns=%d max-stack=%d locals=%d\n",
		len(p.Consts), len(p.Types), len(p.Sigs), len(p.Fallbacks), len(p.Fns), p.MaxStack, p.NumLocals)
	return sb.String()
}

func (p *Program) disasmUnit(sb *strings.Builder, code []Instr) {
	for i, in := range code {
		fmt.Fprintf(sb, "%04d %-11s", i, in.Op.String())
		switch in.Op {
		case OpPushConst:
			c := p.Consts[in.Arg]
			fmt.Fprintf(sb, " k%-3d ; %s (%s)", in.Arg, CanonValue(c), c.Parent.Leaf())
		case OpCallNative:
			s := p.Sigs[in.Arg]
			names := make([]string, len(s.Sig.Args))
			for j, t := range s.Sig.Args {
				names[j] = t.Leaf()
			}
			fmt.Fprintf(sb, " s%-3d ; %s (%s)", in.Arg, s.Word, strings.Join(names, ", "))
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
		case OpCallDynamic:
			fmt.Fprintf(sb, " /%d ; apply fn-value", in.Arg)
		}
		sb.WriteByte('\n')
	}
}
