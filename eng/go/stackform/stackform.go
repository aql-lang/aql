// Package stackform defines a canonical strict-stack representation
// of an AQL program. It is the form the property-based-testing
// reducer operates on (design/PBT-PLAN.0.md) and the first half of
// the work the bytecode-emission proposal in
// design/aql-bytecode-report.0.md needs.
//
// A StackForm is a flat sequence of Ops. Each Op corresponds to one
// observable engine action — either pushing a literal value onto
// the stack, dispatching a word (handler invocation), or pushing a
// quoted sub-program for later evaluation. Surface syntax (forward
// argument collection, dotted-access expansion, paren groupings)
// has been resolved by the time a value reaches StackForm — every
// call is in strict-stack order with the arity it was matched at.
//
// Build a StackForm with Compile(); evaluate it with Eval();
// render it back to readable AQL with Pretty(). Equivalent
// programs produce equal StackForms (Equal); cost-driven shrinking
// uses Cost.
package stackform

import "github.com/aql-lang/aql/eng/go"

// Op is one element of a StackForm. The concrete variants are
// PushLit, Call, Quote, DoEval (see below).
type Op interface {
	opMarker()
}

// PushLit pushes a literal value onto the engine's stack.
//
// Literal here means anything that arrives at the engine pointer
// pre-resolved: integers, strings, decimals, booleans, atoms, type
// literals, concrete maps, and concrete lists with Eval=false.
// Lists with Eval=true that were consumed as code bodies live as
// Quote, not PushLit.
type PushLit struct {
	V eng.Value
}

func (PushLit) opMarker() {}

// Call dispatches a word handler with `Arity` stack-resolved
// arguments. Arity matches the matched Signature.Args count
// (post-forward-resolution); the Recorder fires after sig matching
// completes so the arity is the runtime-observed arity, not a
// surface-syntax artifact.
type Call struct {
	Name  string
	Arity int
}

func (Call) opMarker() {}

// Quote pushes a quoted sub-program as a list-shaped value. The
// Body is the StackForm of the inner tokens (recursively Compiled).
// Quote appears when a code-body argument (NoEvalArgs position,
// like def/fn/if branches) is consumed.
//
// Implementation note: in the current Recorder model, Quote is not
// emitted by the engine's primary recorder — quoted bodies stay as
// PushLit of the raw list. A future enhancement can promote those
// to Quote when the body is later executed via call/do. See
// design/PBT-PLAN.0.md "Out of scope".
type Quote struct {
	Body *StackForm
}

func (Quote) opMarker() {}

// DoEval represents an explicit `do` on the top-of-stack quotation.
// Reserved for future work; not emitted by Compile today.
type DoEval struct{}

func (DoEval) opMarker() {}

// StackForm is the canonical strict-stack representation. It is an
// ordered sequence of Ops.
type StackForm struct {
	Ops []Op
}

// Len returns the number of Ops in the form.
func (f *StackForm) Len() int {
	if f == nil {
		return 0
	}
	return len(f.Ops)
}

// Append adds an Op to the form. Used by the Compile-time recorder
// and by reducer rewrite passes.
func (f *StackForm) Append(op Op) {
	f.Ops = append(f.Ops, op)
}
