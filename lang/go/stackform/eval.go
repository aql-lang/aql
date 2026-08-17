package stackform

import (
	"errors"

	core "github.com/boru-lang/boru/core/go"
)

// ErrUnnamedApply reports a form that cannot be replayed because it
// applies a function VALUE rather than calling a word: an inline lambda
// (`([n:Integer] => [n add 1]) 5`), or a fn read out of a container or a
// parameter and applied. The engine records these with an empty Call
// name, which is what this refusal keys on.
//
// The op vocabulary cannot express them. `Call{Name, Arity}` re-invokes
// BY NAME and does not consume a receiver, while an application consumes
// the fn value the stack already holds — so even when the applied value
// carries a name, replaying it as a Call would strand that value and
// produce it twice. It needs an apply-style Op; see NUR074.
//
// Refusing is the point. The alternative — what this replaced — is a
// form that silently replays to a different answer than the program it
// was recorded from, which is how the PBT shrinker came to report
// counterexamples its own generator cannot produce.
var ErrUnnamedApply = errors.New(
	"stackform: cannot replay a function-value application — " +
		"the op vocabulary can call a word by name but cannot apply a value")

// Flatten serialises a StackForm into a token sequence the kernel
// engine can execute. Because the form is already strict-stack, the
// serialisation is direct: each PushLit emits its value, each Call
// emits the word name (which the engine will then dispatch in
// stack-only mode against the previously-pushed args), each Quote
// recursively flattens to a list value.
//
// The resulting token sequence does NOT use any forward-collection
// or paren grouping — it is a pure post-fix program.
func Flatten(form *StackForm) []core.Value {
	if form == nil {
		return nil
	}
	out := make([]core.Value, 0, len(form.Ops))
	for _, op := range form.Ops {
		switch o := op.(type) {
		case PushLit:
			// A Function reaching the engine pointer DISPATCHES
			// (core's stepLiteral); only a Quoted one stays data. In a
			// strict-stack form a function that should be called is a
			// Call op, so a PushLit of one is by construction data —
			// the `/r`-held reference the recorder saw. Marking it
			// Quoted is what makes that inertness survive the round
			// trip, since `/r` parks positionally and stamps nothing on
			// the value (design/FUNCTION-VALUE-SCOPE.0.md §12.6).
			v := o.V
			if v.Parent != nil && v.Parent.Equal(core.TFunction) && !v.Quoted {
				v.Quoted = true
			}
			out = append(out, v)
		case Call:
			// stack-only invocation: the engine reaches the Word
			// with all args already on the stack below it.
			w := core.NewWordModified(o.Name, o.Arity, true, false)
			out = append(out, w)
		case Quote:
			// nested form serialises to a list literal. The list
			// is marked Quoted so the kernel doesn't auto-eval it
			// when consumed.
			inner := Flatten(o.Body)
			lst := core.NewList(inner)
			lst.Quoted = true
			out = append(out, lst)
		case DoEval:
			out = append(out, core.NewWord("do"))
		}
	}
	return out
}

// Replayable reports whether Eval can faithfully replay this form,
// returning ErrUnnamedApply when it cannot.
//
// The check is deliberately structural rather than a comparison against
// a direct run: a form is replayed to DECIDE things (the PBT reducer
// takes a Fail/Pass verdict from each candidate), so "run it twice and
// compare" would both double every side effect and be meaningless for a
// non-deterministic program.
func Replayable(form *StackForm) error {
	if form == nil {
		return nil
	}
	for _, op := range form.Ops {
		switch o := op.(type) {
		case Call:
			if o.Name == "" {
				return ErrUnnamedApply
			}
		case Quote:
			if err := Replayable(o.Body); err != nil {
				return err
			}
		}
	}
	return nil
}

// Eval runs the StackForm through a fresh engine on the given
// registry and returns the resulting stack. This is the round-trip
// partner of Compile — Eval(Compile(reg, src).form) produces the same
// final stack as running `src` directly (modulo PRNG state for
// non-deterministic programs).
//
// A form the recorder could not capture faithfully is REFUSED here
// rather than replayed to a wrong answer; see Replayable.
func Eval(reg *core.Registry, form *StackForm) ([]core.Value, error) {
	if err := Replayable(form); err != nil {
		return nil, err
	}
	tokens := Flatten(form)
	e := core.NewTop(reg)
	return e.Run(tokens)
}
