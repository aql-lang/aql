// AQL types form a slash-separated path lattice.
//
// A child type matches a parent pattern: Scalar/String/ProperString
// matches Scalar/String matches Scalar. The reverse is not true. The
// lattice and the short-name expansion table mirror the Go engine
// (eng/go/types.go + eng/go/typetable.go) so that a Type built here
// from "Integer" expands to "Scalar/Number/Integer" identically, and
// `typeof` / `pathof` render the same leaf names ("Float",
// "ProperString", …) as the reference engine.
//
// PARITY NOTE: this is a strict subset of the Go Type. It carries the
// path parts and supports matches / equal / specificity / toString /
// leaf. It does NOT implement: ID assignment, Behavior dispatch, Rank
// ordering, the Dependent-leaf bolt-on, metatype derivation, or
// MintType. User-minted types are handled separately where the specs
// reach them.

// builtinDecl mirrors eng/go/typetable.go::builtinDecl (the subset of
// fields the TS port needs: path, an optional friendly short alias for
// expansion, and the internal flag that keeps a type out of the
// user-facing name table).
interface BuiltinDecl {
  path: string
  alias?: string
  internal?: boolean
}

// builtinDecls mirrors eng/go/typetable.go::builtinDecls (parent-first
// order). Ranks/FixedIDs are omitted — the TS port does not serialise
// IDs or use Rank ordering yet.
const builtinDecls: BuiltinDecl[] = [
  // Degenerate roots.
  { path: 'Any' },
  { path: 'None' },
  { path: 'Never' },
  { path: 'Absent' },

  // Branch roots (sit under Any in the lattice; Path() stays the leaf).
  { path: 'Scalar' },
  { path: 'Node' },
  { path: 'Ideal' },
  { path: 'Word' },
  { path: 'Type' },

  // Scalar branch.
  { path: 'Scalar/Atom' },
  { path: 'Scalar/Boolean' },
  { path: 'Scalar/Number' },
  { path: 'Scalar/Number/Integer' },
  { path: 'Scalar/Number/Float' },
  { path: 'Scalar/Number/BigInteger' },
  { path: 'Scalar/Number/BigDecimal' },
  { path: 'Scalar/String' },
  { path: 'Scalar/String/EmptyString' },
  { path: 'Scalar/String/ProperString' },
  { path: 'Scalar/Path' },

  // Node branch.
  { path: 'Node/List' },
  { path: 'Node/List/Args' },
  { path: 'Node/List/FlexList' },
  { path: 'Node/Map' },
  { path: 'Node/Map/Inspect' },
  { path: 'Node/Map/FlexMap' },
  { path: 'Node/Xml' },
  { path: 'Node/Xml/FlexXml' },

  // Ideal branch.
  { path: 'Ideal/Object' },
  { path: 'Ideal/Object/Resource' },
  { path: 'Ideal/Object/Resource/Entity' },
  { path: 'Ideal/Array' },
  { path: 'Ideal/Record' },
  { path: 'Ideal/Options' },
  { path: 'Ideal/Error' },
  { path: 'Ideal/Store' },
  { path: 'Ideal/Store/System' },
  { path: 'Ideal/Table' },
  { path: 'Ideal/Reach' },
  { path: 'Ideal/Class' },
  { path: 'Ideal/Surface' },

  // Word branch — interpreter-internal markers (kept out of byName).
  { path: 'Word/__FW', alias: 'Forward', internal: true },
  { path: 'Word/__OP', alias: 'Paren', internal: true },
  { path: 'Word/__CP', alias: 'CloseParen', internal: true },
  { path: 'Word/__ED', alias: 'End', internal: true },
  { path: 'Word/__PE', internal: true },
  { path: 'Word/__IS', internal: true },
  { path: 'Word/__XI', internal: true },
  { path: 'Word/__FN', alias: 'Fndef', internal: true },
  { path: 'Word/__RC', alias: 'Returncheck', internal: true },
  { path: 'Word/__MK', alias: 'Mark', internal: true },
  { path: 'Word/__MV', alias: 'Move', internal: true },
  { path: 'Word/__IN', internal: true },
  { path: 'Word/__IN/__DC', internal: true },
  { path: 'Word/__SP', alias: 'Splice', internal: true },
  { path: 'Word/__DM', alias: 'DispatchMod', internal: true },

  // Type (metatype) branch.
  { path: 'Type/Function' },
  { path: 'Type/FunctionSignature' },
  { path: 'Type/Disjunct' },
  { path: 'Type/Disjunct/Enum' },
  { path: 'Type/Negation' },
  { path: 'Type/Self' },
  { path: 'Type/TypeParam' },
  { path: 'Type/GenSpec' },
  { path: 'Type/GenParam' },
]

// Derived lookups, built once from builtinDecls.
const ROOTS = new Set<string>()
const BY_PATH = new Set<string>()
// leafIndex maps a leaf/alias name to its full path, or '' when the
// name is ambiguous (declared by more than one path). Mirrors
// TypeTable.leafIndex + ExpandShortName.
const LEAF_INDEX = new Map<string, string>()
// byName maps a user-facing leaf name to its full path (non-internal
// types only). Mirrors TypeTable.byName / TypeNameTable.
const BY_NAME = new Map<string, string>()

for (const d of builtinDecls) {
  const parts = d.path.split('/')
  const name = parts[parts.length - 1]!
  BY_PATH.add(d.path)
  if (parts.length === 1) ROOTS.add(d.path)
  if (!d.internal) BY_NAME.set(name, d.path)

  if (LEAF_INDEX.has(name)) {
    if (LEAF_INDEX.get(name) !== '') LEAF_INDEX.set(name, '')
  } else {
    LEAF_INDEX.set(name, d.path)
  }
  if (d.alias) LEAF_INDEX.set(d.alias, d.path)
}

export class AqlType {
  readonly parts: readonly string[]

  constructor(parts: readonly string[]) {
    this.parts = parts
  }

  /** True iff this type satisfies (conforms to) the given pattern. */
  matches(pattern: AqlType): boolean {
    if (pattern.parts.length === 1 && pattern.parts[0] === 'Any') return true
    return this.pathSubtype(pattern)
  }

  /** Strict path-prefix subtype: every part of pattern is a part of this at the same index. */
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
 * Build a Type from a slash-separated path. Short names like "Integer"
 * expand to their full hierarchy ("Scalar/Number/Integer") via the
 * leaf index, matching the Go engine's NewType + ExpandShortName.
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
  if (!ROOTS.has(head) && !BY_PATH.has(path)) {
    const expanded = LEAF_INDEX.get(head)
    if (expanded !== undefined && expanded !== '') {
      const tail = parts.slice(1).join('/')
      const full = tail.length > 0 ? `${expanded}/${tail}` : expanded
      parts = full.split('/')
    }
  }
  return new AqlType(parts)
}

// Well-known types. Mirrors the var block in eng/go/types.go.
export const TAny = newType('Any')
export const TNone = newType('None')
export const TNever = newType('Never')
export const TAbsent = newType('Absent')
export const TScalar = newType('Scalar')
export const TString = newType('Scalar/String')
export const TStringProper = newType('Scalar/String/ProperString')
export const TStringEmpty = newType('Scalar/String/EmptyString')
export const TNumber = newType('Scalar/Number')
export const TInteger = newType('Scalar/Number/Integer')
export const TFloat = newType('Scalar/Number/Float')
export const TBigInteger = newType('Scalar/Number/BigInteger')
export const TBigDecimal = newType('Scalar/Number/BigDecimal')
export const TBoolean = newType('Scalar/Boolean')
export const TPath = newType('Scalar/Path')
export const TAtom = newType('Scalar/Atom')
export const TNode = newType('Node')
export const TIdeal = newType('Ideal')
export const TList = newType('Node/List')
export const TMap = newType('Node/Map')
export const TInspect = newType('Node/Map/Inspect')
export const TXml = newType('Node/Xml')
export const TObject = newType('Ideal/Object')
export const TArray = newType('Ideal/Array')
export const TRecord = newType('Ideal/Record')
export const TOptions = newType('Ideal/Options')
export const TError = newType('Ideal/Error')
export const TStore = newType('Ideal/Store')
export const TTable = newType('Ideal/Table')
export const TReach = newType('Ideal/Reach')
export const TWord = newType('Word')
export const TType = newType('Type')
export const TFunction = newType('Type/Function')
export const TFnUndef = newType('Type/FunctionSignature')
export const TDisjunct = newType('Type/Disjunct')
export const TEnum = newType('Type/Disjunct/Enum')
export const TNegation = newType('Type/Negation')
export const TForward = newType('Word/__FW')
export const TOpenParen = newType('Word/__OP')
export const TMark = newType('Word/__MK')
export const TMove = newType('Word/__MV')
export const TSplice = newType('Word/__SP')
export const TFnDef = newType('Word/__FN')
export const TInterpString = newType('Word/__IS')

/** Back-compat alias: the decimal scalar is named Float in the lattice. */
export const TDecimal = TFloat

/**
 * Return the canonical lookup table of well-known type names. Mirrors
 * `eng.TypeNameTable()` — the user-facing leaf name → Type map the
 * parser and fn-param resolution consult for bare type-name words.
 */
export function typeNameTable(): Map<string, AqlType> {
  const m = new Map<string, AqlType>()
  for (const [name, path] of BY_NAME) {
    m.set(name, newType(path))
  }
  return m
}
