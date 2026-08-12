package stackform

import (
	core "github.com/boru-lang/boru/core/go"
)

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
			out = append(out, o.V)
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

// Eval runs the StackForm through a fresh engine on the given
// registry and returns the resulting stack. This is the round-trip
// partner of Compile — Eval(Compile(reg, src).form) should produce
// the same final stack as running `src` directly (modulo PRNG state
// for non-deterministic programs).
func Eval(reg *core.Registry, form *StackForm) ([]core.Value, error) {
	tokens := Flatten(form)
	e := core.NewTop(reg)
	return e.Run(tokens)
}
