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
	}
	return fmt.Sprintf("OP(%d)", uint8(o))
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

// Program is a compiled unit: code, interned constants, the signature
// table, a pc → source-position map, and the precomputed stack bound.
type Program struct {
	Code      []Instr
	Consts    []Value
	Types     []TypeRef
	Sigs      []SigRef
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
	NLocals int
	Code    []Instr
	Debug   []SrcPos
}

// Disassemble renders the program for golden tests and debugging.
func (p *Program) Disassemble() string {
	var sb strings.Builder
	p.disasmUnit(&sb, p.Code)
	for fi := range p.Fns {
		fmt.Fprintf(&sb, "fn f%d %s/%d (locals=%d):\n", fi, p.Fns[fi].Name, p.Fns[fi].NParams, p.Fns[fi].NLocals)
		p.disasmUnit(&sb, p.Fns[fi].Code)
	}
	fmt.Fprintf(&sb, "; consts=%d types=%d sigs=%d fns=%d max-stack=%d locals=%d\n",
		len(p.Consts), len(p.Types), len(p.Sigs), len(p.Fns), p.MaxStack, p.NumLocals)
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
		case OpPushLocal, OpForSetup:
			fmt.Fprintf(sb, " l%d", in.Arg)
		case OpPushType:
			fmt.Fprintf(sb, " t%-3d ; %s", in.Arg, p.Types[in.Arg].Name)
		case OpCallUser, OpTailCallUser:
			fmt.Fprintf(sb, " f%-3d ; %s/%d", in.Arg, p.Fns[in.Arg].Name, p.Fns[in.Arg].NParams)
		}
		sb.WriteByte('\n')
	}
}
