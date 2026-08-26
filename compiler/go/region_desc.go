package compiler

import core "github.com/boru-lang/boru/core/go"

// Region descriptors — the static record of what the interpreter would have
// read off the tape for one G-lane region (design/FULL-COMPILATION.0.md
// §6.2, Stage 4). A Program side table, peer to Dispatches.
//
// TWO CLASSIFICATIONS, deliberately kept apart. A slot carries both:
//
//   - SlotSource — where the VM gets the operand at run time (a const, a
//     frame local, a prior event's result, a compiled fragment). This is a
//     LOWERING fact, fixed at record time, and it cannot change under a
//     rebind because it describes the emitted code, not the language.
//   - Token — the underlying tape token, kept so OpCollect can RE-DERIVE
//     what the slot presents to signature matching against the LIVE binding
//     set, exactly as the interpreter does on every execution.
//
// Collapsing those two into one "class" is the mistake this type exists to
// prevent, and it is not hypothetical. core's own classifier says so
// (engine.go:1943): a WORD is classified fwdBoundary rather than as a
// prunable value precisely because "matchSignature's treatment of a word is
// contextual" and "pruning on a word's resolved binding would diverge from
// the binder". Measured, the divergence is a wrong ANSWER and not a crash:
//
//	def w fn [[a:Any b:Any][Any][a]] end
//	def k 5 end
//	def go fn [[][Any][w k 1]] end
//	go                                    -> 5       k is a VALUE: collected
//	… def k fn [[][Integer][9]] end  go   -> raises  k is a WORD: barrier
//
// Same body, same source position, same descriptor. A descriptor that froze
// k's record-time class as a value slot answers 5 where the interpreter
// raises. Today the compiler declines that program rather than getting it
// wrong ("module binding k rebound after a fn unit baked its value",
// emit.go:2474) — one of the interim rebind-staleness latches §6.5 retires
// once OpCollect re-derives. That pair is OpCollect's acceptance test: it
// must ANSWER both spellings, not decline the second.
type SlotSource uint8

const (
	// SlotConst is a value interned in Program.Consts.
	SlotConst SlotSource = iota
	// SlotLocal is a frame-local slot.
	SlotLocal
	// SlotEvent is a prior recorded event's result, taken from the stack.
	SlotEvent
	// SlotGroup is a pre-compiled paren fragment, run only if a viable
	// overload consumes the position — the interpreter's conditional
	// evaluation order, which the VM must preserve rather than
	// pre-evaluating every group.
	SlotGroup
)

// SlotQuote is the static dispatch-control modifier the tape carried at this
// position. It is record-time-final: unlike class, a modifier is syntax and
// no rebinding can move it.
type SlotQuote uint8

const (
	QuoteNone  SlotQuote = iota
	QuoteAtom            // the slot is already an Atom — a /q capture, done
	QuoteValue           // /v — take the binding's VALUE, disabling any call
	QuoteData            // /q on a RESULT — treat what the slot yields as data
)

// There is deliberately no QuoteUsurp. `/u` is not carried on the token: it
// desugars to the `usurp` WORD (see DispatchModInfo's doc in
// core/go/value.go), so it arrives as an ordinary lead rather than as a
// modifier on a slot. A constant for it would describe a token shape that
// does not exist.

// SlotDesc is one written-order position in a region.
type SlotDesc struct {
	Source SlotSource
	Quote  SlotQuote
	// Idx addresses Source: a Consts index, a frame-local slot, an event
	// index, or a compiled-fragment index. Unused for sources that need no
	// address.
	Idx int
	// Token is the tape token this slot stood for, kept for live
	// re-derivation (see the type doc). It is READ, never dispatched.
	Token core.Value
}

// RegionLead names what dispatches at the head of the region.
type RegionLead uint8

const (
	// LeadWord is a named word, resolved live at dispatch through the def
	// stack — so a rebind between two executions of the same region is
	// honoured, which is the whole point of re-deriving.
	LeadWord RegionLead = iota
	// LeadApply is an explicit apply.
	LeadApply
	// LeadFnValue is a function VALUE occupying the lead position.
	LeadFnValue
)

// RegionDesc is one G-lane region: what dispatches, the slots it may claim
// in written order to the next HARD delimiter, and the state the
// interpreter's raise-selection reads that collection alone does not carry.
//
// The extent is an OUTPUT, not an input — OpCollect returns a cursor saying
// where collection actually stopped, because its own evaluations can move it
// (a zero-value group collapse slides the next token into the evaluated
// slot; a multi-value collapse leaves extras to be re-examined). Slots is
// therefore the region's full syntactic span, not a claimed arity.
type RegionDesc struct {
	Lead RegionLead
	// Word names the lead when Lead is LeadWord. Resolution is LIVE.
	Word  string
	Slots []SlotDesc
	Pos   core.SrcPos

	// --- raise-selection state (§6.2, widened by falsifier F2) ---
	//
	// Collection's own register state selects most of the interpreter's
	// raise text, and that rides in the machine. These are the pieces that
	// do NOT, each measured rather than assumed. All are REGION-LOCAL: they
	// key on the enclosing collector's Forward, and at most one Forward is
	// live per paren scope (core/go/region_extent.go), so the collector they
	// find is always this region's own.

	// StackResidual is the enclosing value-stack residual reorderCandidates
	// reads — up to four values, which may belong to an EARLIER statement
	// and change the candidate notes.
	StackResidual []core.Value
	// VoidGroups records paren groups that collapsed to zero values. This
	// changes the error CODE, not merely its text.
	VoidGroups []int
	// BarrierReceiver is the live barrierReceiverWord probe's answer: whether
	// the boundary word reads a slot from the ENCLOSING stack, which a paren
	// group would seal off. It adds a third text variant.
	BarrierReceiver bool
	// SuppressForwardParens is the forward-parens suggestion's suppression
	// condition.
	SuppressForwardParens bool
	// FnShapeBinding is IsFnShapeTypedBindingContext's answer: the failing
	// word sits at the body slot of a typed binding whose constraint is
	// fn-shaped. It both adds a `/q` suggestion and GATES PolyNoMatchProbe,
	// so it changes whether the probe runs at all — not just how text reads.
	FnShapeBinding bool
	// PendingForward is pendingForwardFunc's answer: the enclosing
	// collector's NAME, which tailors an undefined-word hint when it is
	// `def`.
	PendingForward string
	// ReachBound carries polyReachBound's answer, which rides into the same
	// probe alongside StackResidual rather than being implied by it.
	ReachBound core.Value
	// ReachBoundOK reports whether ReachBound was derived at all.
	ReachBoundOK bool
}
