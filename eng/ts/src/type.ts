// AQL types form a slash-separated path lattice.
//
// A child type matches a parent pattern: Scalar/String/Proper matches
// Scalar/String matches Scalar. The reverse is not true. The lattice
// roots and the short-name expansion table mirror the Go engine
// (aqleng/go/types.go) so that a Type built here from "Integer"
// expands to "Scalar/Number/Integer" identically.
//
// PARITY NOTE: this is a strict subset of the Go Type. It carries
// the path parts and supports Matches / Equal / Specificity / String.
// It does NOT implement: ID assignment, Dependent-leaf bolt-on for
// `Matches`, metatype derivation, or `ResolveTypePath`. Those are
// out of scope for the spec set; a full port would replicate
// `mustType` + `BuiltinTypeIDs` and the `DependentLeaf*` helpers.

const ROOTS = new Set([
  'Scalar',
  'Node',
  'Word',
  'Object',
  'Any',
  'None',
  'Never',
  'Type',
  'Dependent',
])

const ANCESTRY: Record<string, string> = {
  String: 'Scalar/String',
  Number: 'Scalar/Number',
  Integer: 'Scalar/Number/Integer',
  Decimal: 'Scalar/Number/Decimal',
  Boolean: 'Scalar/Boolean',
  Path: 'Scalar/Path',
  Atom: 'Scalar/Atom',
  List: 'Node/List',
  Map: 'Node/Map',
  Table: 'Object/Table',
  Record: 'Object/Record',
  Store: 'Object/Store',
  Array: 'Object/Array',
  Function: 'Word/Function',
  Error: 'Object/Error',
}

export class AqlType {
  readonly parts: readonly string[]

  constructor(parts: readonly string[]) {
    this.parts = parts
  }

  /** True iff this type satisfies the given pattern. Lattice-aware. */
  matches(pattern: AqlType): boolean {
    if (pattern.parts.length === 1 && pattern.parts[0] === 'Any') return true
    return this.pathSubtype(pattern)
  }

  /** Strict path-prefix subtype: every Part of pattern is a Part of this at the same index. */
  pathSubtype(pattern: AqlType): boolean {
    if (this.parts.length < pattern.parts.length) return false
    for (let i = 0; i < pattern.parts.length; i++) {
      if (this.parts[i] !== pattern.parts[i]) return false
    }
    return true
  }

  equal(other: AqlType): boolean {
    if (this.parts.length !== other.parts.length) return false
    for (let i = 0; i < this.parts.length; i++) {
      if (this.parts[i] !== other.parts[i]) return false
    }
    return true
  }

  specificity(): number {
    return this.parts.length
  }

  toString(): string {
    return this.parts.join('/')
  }

  leaf(): string {
    return this.parts[this.parts.length - 1] ?? ''
  }
}

/**
 * Build a Type from a slash-separated path. Short names like
 * "Integer" expand to their full hierarchy ("Scalar/Number/Integer"),
 * matching the Go engine's NewType.
 */
export function newType(path: string): AqlType {
  const rawParts = path.split('/')
  for (const p of rawParts) {
    const ch = p[0]
    if (ch && /[a-z]/.test(ch)) {
      throw new Error(
        `aql: type part ${JSON.stringify(p)} in ${JSON.stringify(path)} must start with an uppercase letter`,
      )
    }
  }

  let parts = rawParts
  const head = parts[0]!
  if (!ROOTS.has(head)) {
    const expanded = ANCESTRY[head]
    if (expanded !== undefined) {
      const tail = parts.slice(1).join('/')
      const full = tail.length > 0 ? `${expanded}/${tail}` : expanded
      parts = full.split('/')
    }
  }
  return new AqlType(parts)
}

// Well-known types. Mirrors the var block in aqleng/go/types.go.
export const TAny = newType('Any')
export const TNone = newType('None')
export const TNever = newType('Never')
export const TScalar = newType('Scalar')
export const TString = newType('Scalar/String')
export const TStringProper = newType('Scalar/String/Proper')
export const TStringEmpty = newType('Scalar/String/Empty')
export const TNumber = newType('Scalar/Number')
export const TInteger = newType('Scalar/Number/Integer')
export const TDecimal = newType('Scalar/Number/Decimal')
export const TBoolean = newType('Scalar/Boolean')
export const TPath = newType('Scalar/Path')
export const TAtom = newType('Scalar/Atom')
export const TNode = newType('Node')
export const TList = newType('Node/List')
export const TMap = newType('Node/Map')
export const TTable = newType('Object/Table')
export const TRecord = newType('Object/Record')
export const TWord = newType('Word')
export const TFunction = newType('Word/Function')
export const TForward = newType('Word/__FW')
export const TOpenParen = newType('Word/__OP')
export const TMark = newType('Word/__MK')
export const TMove = newType('Word/__MV')
export const TObject = newType('Object')
export const TError = newType('Object/Error')

/**
 * Return the canonical lookup table of well-known type names. Mirrors
 * `aqleng.TypeNameTable()` in Go — used by the parser side in the Go
 * codebase, included here for parity-of-shape.
 */
export function typeNameTable(): Map<string, AqlType> {
  return new Map<string, AqlType>([
    ['Any', TAny],
    ['None', TNone],
    ['Never', TNever],
    ['Scalar', TScalar],
    ['String', TString],
    ['Number', TNumber],
    ['Integer', TInteger],
    ['Decimal', TDecimal],
    ['Boolean', TBoolean],
    ['Path', TPath],
    ['Atom', TAtom],
    ['Node', TNode],
    ['List', TList],
    ['Map', TMap],
    ['Table', TTable],
    ['Record', TRecord],
    ['Word', TWord],
    ['Function', TFunction],
    ['Object', TObject],
    ['Error', TError],
  ])
}
