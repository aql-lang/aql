// The boru TS parser — the jsonic-based twin of parser/go, built on
// @tabnas/parser + @tabnas/jsonic (the same two-layer engine/grammar
// system as the Go port, same version family). Its primary entry is
// parse(src) → Value[] (throwing BoruError on syntax errors).
//
// Both ports implement the same language contract. Go is the historical
// reference by convention, while the shared independent oracles adjudicate
// behavior where measurement finds either port wrong.
// The remaining exports mirror Go's host-facing config, safe-construction,
// data-number, and trivia-preserving lexer seams. ADR-012 holds here
// identically: the parser is type-name-opaque and emits structural sugar
// markers — it never invents a word name.
export { convertParsedNumber, parse } from './convert.ts'
export {
  guardMake,
  lexTokens,
  parseConfig,
  plainify,
  safeMake,
  safeParse,
  safeParseData,
  type LexToken,
  type LexTokensResult,
} from './public.ts'
export type { SrcPos } from './nodes.ts'
