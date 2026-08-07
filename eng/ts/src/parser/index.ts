// The boru TS parser — the jsonic-based twin of parser/go, built on
// @tabnas/parser + @tabnas/jsonic (the same two-layer engine/grammar
// system as the Go port, same version family). One public entry:
//
//   parse(src) → Value[]     (throws BoruError on syntax errors)
//
// The Go parser is the REFERENCE: every construct converts to the same
// value stream (verified by the stream-diff oracle against
// parser/go's TestStreamDump). ADR-012 holds here identically: the
// parser is type-name-opaque and emits structural sugar markers — it
// never invents a word name.
export { parse } from './convert.ts'
export type { SrcPos } from './nodes.ts'
