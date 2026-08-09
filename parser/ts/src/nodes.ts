// Shared intermediate node types for the boru TS parser — the contracts
// between the grammar layer (grammar.ts, which builds these inside jsonic
// rule actions) and the conversion layer (convert.ts, which type-switches
// on them to build engine Values). Each class mirrors the same-named Go
// type in parser/go (parse.go / grammar.go / xml_literal.go); keep
// the shapes in lockstep — the Go side is the reference.
//
// Discrimination is by `instanceof` (the TS analogue of Go's type
// switches), so every node the grammar hands the converter must be an
// instance of one of these classes, a jsonic node (boxed String with
// info marker, array, plain object, number, boolean, null), or a
// primitive produced by a custom matcher.

import type { Value } from '@boru-lang/core'
import type { XmlTmpl } from '@boru-lang/core'

/** 1-based source position; 0 row = unknown. Mirrors eng.SrcPos. */
export interface SrcPos {
  row: number
  col: number
  /** Source text of the token (used in error rendering). */
  src?: string
  /** Internal UTF-16 source index supplied by the TypeScript lexer. */
  si?: number
}

export const UNKNOWN_POS: SrcPos = { row: 0, col: 0 }

/** Convert an absolute JavaScript UTF-16 index to a 1-based code-point column. */
export function codePointColumnAt(src: string, si: number): number {
  const bounded = Math.max(0, Math.min(si, src.length))
  const lineStart = src.lastIndexOf('\n', bounded - 1) + 1
  return Array.from(src.slice(lineStart, bounded)).length + 1
}

/** Normalize a lexer position when its exact absolute source index is known. */
export function normalizeSrcPos(src: string, pos: SrcPos): SrcPos {
  if (pos.row === 0 || pos.si === undefined) return pos
  return { ...pos, col: codePointColumnAt(src, pos.si) }
}

/**
 * Normalize a Jsonic error position, whose public error surface exposes only
 * row/column. Stock matchers count UTF-16 code units while Boru's custom
 * matchers count code points, so the offending token text is the stable
 * anchor: choose the occurrence closest to either interpretation and return
 * its actual code-point column.
 */
export function normalizeReportedPos(src: string, pos: SrcPos): SrcPos {
  if (pos.row <= 0 || pos.col <= 0) return pos
  const line = src.split('\n')[pos.row - 1]
  if (line === undefined) return pos
  const tokenSrc = pos.src ?? ''
  if (tokenSrc !== '') {
    let bestColumn: number | undefined
    let bestDistance = Number.POSITIVE_INFINITY
    let from = 0
    for (;;) {
      const index = line.indexOf(tokenSrc, from)
      if (index < 0) break
      const codePointColumn = Array.from(line.slice(0, index)).length + 1
      const utf16Column = index + 1
      const distance = Math.min(
        Math.abs(pos.col - codePointColumn),
        Math.abs(pos.col - utf16Column),
      )
      if (distance < bestDistance) {
        bestDistance = distance
        bestColumn = codePointColumn
      }
      from = index + Math.max(1, tokenSrc.length)
    }
    if (bestColumn !== undefined) return { ...pos, col: bestColumn }
  }
  const codePointEnd = Array.from(line).length + 1
  if (pos.col <= codePointEnd) return pos
  return { ...pos, col: codePointColumnAt(line, pos.col - 1) }
}

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
  si: number | undefined
  constructor(val: number, src: string, row: number, col: number, si?: number) {
    this.val = val
    this.src = src
    this.row = row
    this.col = col
    this.si = si
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
 * Container-nesting depth guard. The explicit conversion work stack checks
 * the same logical frames as Go's recursive parseDepth walk, turning
 * pathological nesting into a clean evaluation_limit error without relying
 * on the JavaScript host stack.
 */
export const MAX_PARSE_NESTING_DEPTH = 10000

export class ParseDepth {
  cur = 0
  src: string
  private readonly columns: Uint32Array
  constructor(src: string) {
    this.src = src
    this.columns = new Uint32Array(src.length + 1)
    let col = 1
    let i = 0
    while (i < src.length) {
      this.columns[i] = col
      const codePoint = src.codePointAt(i)!
      const width = codePoint > 0xffff ? 2 : 1
      for (let inner = 1; inner < width; inner++) {
        this.columns[i + inner] = col
      }
      col = codePoint === 0x0a ? 1 : col + 1
      i += width
    }
    this.columns[src.length] = col
  }

  /** O(1) outward position normalization during the explicit tree walk. */
  normalizePos(pos: SrcPos): SrcPos {
    if (pos.row === 0 || pos.si === undefined) return pos
    const si = Math.max(0, Math.min(pos.si, this.src.length))
    return { ...pos, col: this.columns[si]! }
  }
}
