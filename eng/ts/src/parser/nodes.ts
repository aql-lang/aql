// Shared intermediate node types for the boru TS parser — the contracts
// between the grammar layer (grammar.ts, which builds these inside jsonic
// rule actions) and the conversion layer (convert.ts, which type-switches
// on them to build engine Values). Each class mirrors the same-named Go
// type in eng/go/parser (parse.go / grammar.go / xml_literal.go); keep
// the shapes in lockstep — the Go side is the reference.
//
// Discrimination is by `instanceof` (the TS analogue of Go's type
// switches), so every node the grammar hands the converter must be an
// instance of one of these classes, a jsonic node (boxed String with
// info marker, array, plain object, number, boolean, null), or a
// primitive produced by a custom matcher.

import type { Value } from '../value.ts'
import type { XmlTmpl } from '../value.ts'

/** 1-based source position; 0 row = unknown. Mirrors eng.SrcPos. */
export interface SrcPos {
  row: number
  col: number
  /** Source text of the token (used in error rendering). */
  src?: string
}

export const UNKNOWN_POS: SrcPos = { row: 0, col: 0 }

/**
 * A paren group's collected items (word context: `( … )`). Mirrors Go
 * `type parenGroup []any`.
 */
export class ParenGroup {
  items: unknown[]
  constructor(items: unknown[]) {
    this.items = items
  }
}

/** A paren group auto-closed at EOF (no matching `)`). */
export class UnclosedParen {
  items: unknown[]
  constructor(items: unknown[]) {
    this.items = items
  }
}

/**
 * A generics angle-bracket group: `Box<Integer>` folds the receiver
 * name and the collected items into one node. Converts to ONE
 * structural SugarAngle marker (ADR-012) in every position.
 */
export class AngleGroup {
  name: string
  items: unknown[]
  constructor(name: string, items: unknown[]) {
    this.name = name
    this.items = items
  }
}

/** An angle group auto-closed at EOF (no matching `>`). */
export class UnclosedAngle {
  name: string
  constructor(name: string) {
    this.name = name
  }
}

/**
 * The parts collected between backticks by the interp grammar rule.
 * Each element is either a boxed-String text token with quote 'tl'
 * (literal segment) or an IexprGroup (interpolated expression).
 */
export class InterpGroup {
  parts: unknown[]
  constructor(parts: unknown[]) {
    this.parts = parts
  }
}

/** The expression values collected between `${` and `}`. */
export class IexprGroup {
  items: unknown[]
  constructor(items: unknown[]) {
    this.items = items
  }
}

/**
 * A minilang-literal match (`+name<delim>src<delim>`) carried from the
 * lex matcher through the val rule to the converter, which emits the
 * SugarMini marker.
 */
export class MiniLitVal {
  name: string
  src: string
  constructor(name: string, src: string) {
    this.name = name
    this.src = src
  }
}

/**
 * A parsed number token: jsonic's numeric value plus the raw source so
 * the converter can (1) keep whole-valued float literals (`5.0`) as
 * floats, (2) parse integers from their exact digits, and (3) locate
 * an out-of-range literal. Injected by the number lex sub.
 */
export class NumberVal {
  val: number
  src: string
  row: number
  col: number
  constructor(val: number, src: string, row: number, col: number) {
    this.val = val
    this.src = src
    this.row = row
    this.col = col
  }
}

/**
 * The `=>` arrow substitution marker. The converter turns it into the
 * SugarLambda marker (the parser emits no names — ADR-012).
 */
export class ArrowTag {}

/**
 * A finished embedded XML literal (or the reason it failed to build),
 * carried on the #XML token from the xml matcher. Mirrors Go
 * xmlElemVal — the build failure rides in `err` so the converter can
 * surface a clean error rather than a generic jsonic parse failure.
 */
export class XmlElemVal {
  v: Value | undefined
  err: string | undefined
  /** The unresolved template when the literal carries ${…} holes. */
  tmpl?: XmlTmpl
  constructor(v: Value | undefined, err: string | undefined, tmpl?: XmlTmpl) {
    this.v = v
    this.err = err
    this.tmpl = tmpl
  }
}

/**
 * A source-position wrapper around any intermediate node. Mirrors Go
 * `type sited struct { Node any; Pos eng.SrcPos }`. deSite unwraps.
 */
export class Sited {
  node: unknown
  pos: SrcPos
  constructor(node: unknown, pos: SrcPos) {
    this.node = node
    this.pos = pos
  }
}

/** Unwrap a Sited wrapper; a bare node returns with unknown pos. */
export function deSite(v: unknown): [unknown, SrcPos] {
  if (v instanceof Sited) return [v.node, v.pos]
  return [v, UNKNOWN_POS]
}

/**
 * Container-nesting depth guard, threaded through the recursive
 * converters. Mirrors Go parseDepth/maxParseNestingDepth: turns a
 * pathological deep nesting into a clean evaluation_limit error
 * instead of a stack overflow.
 */
export const MAX_PARSE_NESTING_DEPTH = 10000

export class ParseDepth {
  cur = 0
  src: string
  constructor(src: string) {
    this.src = src
  }
}
