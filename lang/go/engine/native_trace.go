package engine

import "github.com/aql-lang/aql/eng/go"

// traceNatives covers `trace [body]` — the step-by-step debug
// counterpart to `do`. The list is evaluated in a sub-engine with
// tracing enabled, and the stack evolution is printed to the
// registry's Output writer.
//
//	trace [1 add 2 mul 3]
//
// Algorithm (RunTrace and helpers) lives in eng/go/trace.go; this
// file owns the word name and dispatch wiring.
var traceNatives = []NativeFunc{
	{
		Name:        "trace",
		ForwardArgs: true,
		Signatures: []NativeSig{{
			Args:    []*Type{TList},
			Handler: eng.TraceHandler,
			Returns: []*Type{TAny},
		}},
	},
}
