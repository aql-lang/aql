package core

// CodeEffectInfo lives HERE, not in the compiler, because it is a PAYLOAD:
// it rides in Value.Data behind Parent=TList/Carrier=true, it embeds
// PayloadBase, and payload.go's seal already lists it among the payload
// family. A payload type defined outside core forces every reader of that
// payload — including word libraries that only want to inspect a code
// value's stack effect — to depend on the compiler.
//
// The ANALYSIS that produces one stays in the compiler; only the shape is
// core's.

// CodeEffectInfo is the carrier payload for a typed CODE VALUE: the
// stack effect of a quoted code list held as data. It mirrors
// ChildTypeInfo's conventions — a Value with Parent=TList and
// Carrier=true carries it as Data, the way typed-list carriers carry
// ChildTypeInfo (the non-nil payload also keeps satisfying
// positionalMatch's concrete-list rule for TList slots).
type CodeEffectInfo struct {
	PayloadBase
	// In is the input types the body consumes from the stack, top
	// first. Stage-1 producers analyse bodies against NO inputs (the
	// `do` invocation shape, which runs its body in a fresh sub-stack),
	// so In is always empty today; the field exists for the staged
	// higher-order consumers (each/fold/filter bodies), which analyse
	// against synthetic inputs.
	In []*Type
	// Out is the body's net residual types in bottom-to-top stack
	// order — the FULL residual `do` leaves (mirroring doListHandler's
	// whole-stack return, so a multi-value body nets every value).
	Out []*Type
	// Analysed is true when the body analysis completed cleanly (no
	// diagnostics, no divergence, every residual representable). A
	// carrier without the payload — or with Analysed=false — has an
	// UNKNOWN effect and must stay dynamic(Any) at every consumer.
	Analysed bool
}
