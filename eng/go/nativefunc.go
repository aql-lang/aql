package eng

// NativeFunc describes a built-in native function with its name and
// signatures. All predefined words (core and extension) use this type
// for registration.
//
// There is no "ForwardArgs" / "stack-only" flag at this level — that
// distinction lives entirely in each `NativeSig.BarrierPos`:
//
//   - `BarrierPos: BarrierAllForward` (-1)  — default all-forward;
//     resolved to `len(Args)` at registration. The common case for
//     normal forward-collecting words.
//   - `BarrierPos: 0`  — explicit all-stack dispatch. Use for words
//     that take args strictly off the prefix stack (`drop`, `dup`,
//     stack manipulators).
//   - `BarrierPos: N`  — explicit barrier at position N (`|`).
type NativeFunc struct {
	Name       string
	Signatures []NativeSig
}

// NativeSig describes one overload of a native function.
type NativeSig struct {
	Args    []*Type
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

	// TypeArgs marks arg positions that must receive a type literal
	// rather than a concrete value. See Signature.TypeArgs.
	TypeArgs map[int]bool

	// BarrierPos is the arg index where forward collection must stop.
	BarrierPos int

	// Fallback marks this as the generic 0-arg fallback handler.
	Fallback bool

	// Returns lists the declared return types for static type-checking.
	// See Signature.Returns for details.
	Returns []*Type

	// ReturnsFn computes the carrier return values for a signature in
	// static type-check mode. See Signature.ReturnsFn for details.
	ReturnsFn ReturnsFunc

	// RunInCheckMode runs the Handler even under CheckMode. See
	// Signature.RunInCheckMode for details.
	RunInCheckMode bool

	// CheckFullStackFn — see Signature.CheckFullStackFn.
	CheckFullStackFn CheckFullStackFunc
}
