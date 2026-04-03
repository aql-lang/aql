package engine

import "fmt"

// NativeFunc describes a built-in native function with its name, signatures,
// and configuration. All predefined words (core and extension) use this
// type for registration.
type NativeFunc struct {
	Name              string
	ForwardPrecedence bool
	SkipSafetyCheck   bool // when true, bypass safety check (for type-inspecting words)
	Signatures        []NativeSig
}

// NativeSig describes one overload of a native function.
type NativeSig struct {
	Args    []Type
	Handler Handler

	// FullStack, when true, causes the engine to pass the full resolved
	// stack (excluding matched args) and to splice the results as a
	// complete replacement for base..pointer.
	FullStack bool

	// Patterns holds optional structural patterns for arguments.
	Patterns map[int]Value

	// QuoteArgs marks arg positions with the /q modifier ("implicit quote").
	QuoteArgs map[int]bool

	// NoEvalArgs marks arg positions where list auto-evaluation should be
	// suppressed.
	NoEvalArgs map[int]bool

	// BarrierPos is the arg index where forward collection must stop.
	BarrierPos int

	// Fallback marks this as the generic 0-arg fallback handler.
	Fallback bool
}

// RegisterNativeFunc installs a NativeFunc into the registry. It applies
// safety checking (unless SkipSafetyCheck is set), converts NativeSig to
// Signature, and registers with the appropriate precedence.
func (r *Registry) RegisterNativeFunc(fn NativeFunc) {
	for _, sig := range fn.Signatures {
		handler := sig.Handler
		if !fn.SkipSafetyCheck {
			handler = WrapSafetyCheck(handler)
		}
		s := Signature{
			Args:       sig.Args,
			Handler:    handler,
			FullStack:  sig.FullStack,
			Patterns:   sig.Patterns,
			QuoteArgs:  sig.QuoteArgs,
			NoEvalArgs: sig.NoEvalArgs,
			BarrierPos: sig.BarrierPos,
			Fallback:   sig.Fallback,
		}
		if fn.ForwardPrecedence {
			r.Register(fn.Name, s)
		} else {
			r.RegisterStackOnly(fn.Name, s)
		}
	}
}

// wrapSafetyCheck wraps a Handler to reject type literals and Options types
// before the handler runs. This prevents nil pointer dereferences in native
// handlers that expect concrete data.
func WrapSafetyCheck(h Handler) Handler {
	return func(args []Value, ctx map[string]Value, stack []Value, r *Registry) ([]Value, error) {
		for _, arg := range args {
			if arg.Data == nil && !arg.VType.Equal(TNone) {
				return nil, fmt.Errorf("expected a concrete value, got type literal %s", arg.VType)
			}
			if arg.IsOptionsType() {
				return nil, fmt.Errorf("expected a concrete map, got options type %s", arg.String())
			}
		}
		return h(args, ctx, stack, r)
	}
}
