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

// Program is a compiled unit: code, interned constants, the signature
// table, a pc → source-position map, and the precomputed stack bound.
type Program struct {
	Code      []Instr
	Consts    []Value
	Sigs      []SigRef
	Debug     []SrcPos // 1:1 with Code
	MaxStack  int      // a floor when the program loops (results accumulate)
	NumLocals int
}

// Disassemble renders the program for golden tests and debugging.
func (p *Program) Disassemble() string {
	var sb strings.Builder
	for i, in := range p.Code {
		fmt.Fprintf(&sb, "%04d %-11s", i, in.Op.String())
		switch in.Op {
		case OpPushConst:
			c := p.Consts[in.Arg]
			fmt.Fprintf(&sb, " k%-3d ; %s (%s)", in.Arg, CanonValue(c), c.Parent.Leaf())
		case OpCallNative:
			s := p.Sigs[in.Arg]
			names := make([]string, len(s.Sig.Args))
			for j, t := range s.Sig.Args {
				names[j] = t.Leaf()
			}
			fmt.Fprintf(&sb, " s%-3d ; %s (%s)", in.Arg, s.Word, strings.Join(names, ", "))
		case OpJmp, OpJmpIfFalse, OpForNext:
			fmt.Fprintf(&sb, " -> %04d", in.Arg)
		case OpPushLocal, OpForSetup:
			fmt.Fprintf(&sb, " l%d", in.Arg)
		}
		sb.WriteByte('\n')
	}
	fmt.Fprintf(&sb, "; consts=%d sigs=%d max-stack=%d locals=%d\n",
		len(p.Consts), len(p.Sigs), p.MaxStack, p.NumLocals)
	return sb.String()
}
