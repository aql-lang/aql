package modules

func init() {
	registerDocs("aql:debug", map[string]string{
		"tap":     "Print a value (formatted), then return it unchanged — the pipeline tap `print` can't be.",
		"label":   "Print a labelled value (`value label Debug.label`) and return the value.",
		"dump":    "Print a value with its type, then return it unchanged.",
		"assert":  "Raise assertion_failure with a message when the condition is false (`cond msg Debug.assert`).",
		"todo":    "A typed hole: always raise not_implemented with the given message (returns Never).",
		"parse":   "Parse AQL source to its token/value list without running it.",
		"deps":    "The distinct word names a quoted body references.",
		"explain": "The full `describe` text for a word, returned as a String.",
		"sig":     "The signatures of a word as structured {args, returns} data.",
		"body":    "The quoted body of an AQL-defined word; `native` for a host word.",
		"watch":   "Print a name's current binding and return it (None if unbound).",
		"stack":   "A snapshot of the current data stack at the call site, as a List.",
		"disasm":  "Compile a quoted body to its StackForm disassembly (a String).",
		"heap":    "Go-runtime heap stats: {alloc, total-alloc, heap-objects, num-gc}.",
		"gc":      "Force a GC and report before/after heap deltas.",
		"words":   "Every documented built-in word name.",
		"defs":    "Current def-bound names mapped to their active top binding.",
		"modules": "The native modules available to import.",
		"sizeof":  "Estimated retained byte size of a value (deep walk).",
		"shape":   "Structural census of a value: counts by kind, node count, and max depth.",
		"steps":   "Engine step count for a body — deterministic, clock-free perf signal.",
		"time":    "Run a body once, returning {result, elapsed-ms, steps}.",
		"bench":   "Run a body n times (`body n Debug.bench`), returning timing stats.",
		"trace":   "Run a body with step-by-step stack tracing printed to output.",
		"profile": "Per-word step-count profile of a body, costliest first.",
	})
}
