package aqleng

// NativeFunc describes a built-in native function with its name, signatures,
// and configuration. All predefined words (core and extension) use this
// type for registration.
type NativeFunc struct {
	Name              string
	ForwardPrecedence bool
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

	// NoEvalMapArgs marks arg positions where map auto-evaluation
	// should be suppressed. See Signature.NoEvalMapArgs.
	NoEvalMapArgs map[int]bool

	// BarrierPos is the arg index where forward collection must stop.
	BarrierPos int

	// Fallback marks this as the generic 0-arg fallback handler.
	Fallback bool

	// Returns lists the declared return types for static type-checking.
	// See Signature.Returns for details.
	Returns []Type

	// ReturnsFn computes the carrier return values for a signature in
	// static type-check mode. See Signature.ReturnsFn for details.
	ReturnsFn ReturnsFunc

	// RunInCheckMode runs the Handler even under CheckMode. See
	// Signature.RunInCheckMode for details.
	RunInCheckMode bool

	// CheckFullStackFn — see Signature.CheckFullStackFn.
	CheckFullStackFn CheckFullStackFunc
}

// RegisterNativeFunc installs a NativeFunc into the registry, converts
// NativeSig to Signature, and registers with the appropriate precedence.
func (r *Registry) RegisterNativeFunc(fn NativeFunc) {
	for _, sig := range fn.Signatures {
		s := Signature{
			Args:             sig.Args,
			Handler:          sig.Handler,
			FullStack:        sig.FullStack,
			Patterns:         sig.Patterns,
			QuoteArgs:        sig.QuoteArgs,
			NoEvalArgs:       sig.NoEvalArgs,
			NoEvalMapArgs:    sig.NoEvalMapArgs,
			BarrierPos:       sig.BarrierPos,
			Fallback:         sig.Fallback,
			Returns:          sig.Returns,
			ReturnsFn:        sig.ReturnsFn,
			RunInCheckMode:   sig.RunInCheckMode,
			CheckFullStackFn: sig.CheckFullStackFn,
		}
		if fn.ForwardPrecedence {
			r.Register(fn.Name, s)
		} else {
			r.RegisterStackOnly(fn.Name, s)
		}
	}
}
