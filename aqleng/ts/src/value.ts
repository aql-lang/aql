// Value is the engine's universal datum. It carries a runtime type
// (`vType`), an optional `data` payload, and a few flags. The Go
// engine puts these on a flat struct; in TS we use a class so we can
// hang accessor methods off it. PARITY NOTE: TS classes are reference
// types; the Go Value is a value type that handlers receive by copy.
// Mutation through copies is impossible in Go; in TS callers must be
// disciplined about not mutating shared Value instances. Constructors
// here always return fresh instances.
import {
  AqlType,
  TAny,
  TAtom,
  TBoolean,
  TDecimal,
  TFunction,
  TInteger,
  TList,
  TNone,
  TString,
  TWord,
} from './type.ts'

/** A reified word reference — produced by NewWord, dispatched by the engine. */
export interface WordInfo {
  name: string
  /** Optional argument-count modifier from /N suffix. Unused in spec subset. */
  argCount?: number
  forceStack?: boolean
  forceForward?: boolean
}

/** A typed parameter on a function definition. */
export interface FnParam {
  name: string
  type: AqlType
}

/**
 * A user-defined function: typed params + return-type slots + a body
 * the engine runs when the fn is invoked. The spec subset uses a
 * single-overload shape; the full Go FnDefInfo carries multiple
 * overloads (Sigs[]) plus optional Patterns / NoEvalArgs.
 */
export interface FnDefInfo {
  params: FnParam[]
  returns: AqlType[]
  body: Value[]
}

export class Value {
  readonly vType: AqlType
  readonly data: unknown
  readonly quoted: boolean
  readonly carrier: boolean

  constructor(vType: AqlType, data: unknown, opts?: { quoted?: boolean; carrier?: boolean }) {
    this.vType = vType
    this.data = data
    this.quoted = opts?.quoted ?? false
    this.carrier = opts?.carrier ?? false
  }

  /** True iff this Value is a type literal (no concrete payload). */
  isTypeLiteral(): boolean {
    return this.data === null && !this.carrier && !this.vType.equal(TNone)
  }

  /** True iff this Value carries a concrete payload (not a literal, not a carrier). */
  isConcrete(): boolean {
    return this.data !== null && !this.carrier
  }

  isWord(): boolean {
    return this.vType.equal(TWord)
  }

  asInteger(): bigint {
    if (this.data === null) throw new Error('AsInteger: nil data')
    if (typeof this.data !== 'bigint') {
      throw new Error(`AsInteger: not an integer value (got ${typeof this.data})`)
    }
    return this.data
  }

  asDecimal(): number {
    if (this.data === null) throw new Error('AsDecimal: nil data')
    if (typeof this.data !== 'number') {
      throw new Error(`AsDecimal: not a decimal value (got ${typeof this.data})`)
    }
    return this.data
  }

  asString(): string {
    if (this.data === null) throw new Error('AsString: nil data')
    if (typeof this.data !== 'string') {
      throw new Error(`AsString: not a string value (got ${typeof this.data})`)
    }
    return this.data
  }

  asBoolean(): boolean {
    if (this.data === null) throw new Error('AsBoolean: nil data')
    if (typeof this.data !== 'boolean') {
      throw new Error(`AsBoolean: not a boolean value (got ${typeof this.data})`)
    }
    return this.data
  }

  asWord(): WordInfo {
    if (this.data === null || typeof this.data !== 'object') {
      throw new Error('AsWord: not a word value')
    }
    return this.data as WordInfo
  }

  asAtom(): string {
    if (this.data === null) throw new Error('AsAtom: nil data')
    if (typeof this.data !== 'string') {
      throw new Error(`AsAtom: not an atom value (got ${typeof this.data})`)
    }
    return this.data
  }

  asList(): Value[] {
    if (this.data === null) throw new Error('AsList: nil data')
    if (!Array.isArray(this.data)) {
      throw new Error(`AsList: not a list value`)
    }
    return this.data as Value[]
  }

  asFnDef(): FnDefInfo {
    if (this.data === null) throw new Error('AsFnDef: nil data')
    if (typeof this.data !== 'object') {
      throw new Error(`AsFnDef: not a function value`)
    }
    return this.data as FnDefInfo
  }

  isFnDef(): boolean {
    return this.vType.matches(TFunction) && this.data !== null
  }

  /** Stringify in a parser-style debug form: words as word(name), strings quoted, etc. */
  toString(): string {
    if (this.data === null) {
      if (this.vType.equal(TNone)) return 'null'
      return this.vType.toString()
    }
    if (this.vType.matches(TWord)) {
      return `word(${(this.data as WordInfo).name})`
    }
    if (this.vType.matches(TString)) {
      return JSON.stringify(this.data)
    }
    if (this.vType.matches(TInteger)) {
      return String(this.data)
    }
    if (this.vType.matches(TDecimal)) {
      return String(this.data)
    }
    if (this.vType.matches(TBoolean)) {
      return String(this.data)
    }
    if (this.vType.matches(TAtom)) {
      return `atom(${this.data})`
    }
    if (this.vType.matches(TList) && Array.isArray(this.data)) {
      const elems = (this.data as Value[]).map((v) => v.toString())
      return `[${elems.join(' ')}]`
    }
    return String(this.data)
  }
}

// ── Constructors ────────────────────────────────────────────────────────────

/**
 * Construct an integer value with VType = Scalar/Number/Integer.
 *
 * Earlier versions of this engine encoded the literal in the type
 * path (e.g. `Scalar/Number/Integer/42`) so that signature dispatch
 * could fire on specific numeric values via subtype matching. That
 * "value-tagged subtype" trick made `v.vType.equal(TInteger)` return
 * false for any concrete integer, bloated the type lattice per
 * literal payload, and forced consumers to use `matches` everywhere.
 * Specific-value dispatch now goes through `Signature.patterns`
 * instead — see PORT_OBSERVATIONS.md §1.1.
 *
 * BigInt is used because the Go engine carries int64 — TS's number
 * loses precision past 2^53.
 */
export function newInteger(n: bigint | number): Value {
  const big = typeof n === 'bigint' ? n : BigInt(n)
  return new Value(TInteger, big)
}

export function newDecimal(f: number): Value {
  return new Value(TDecimal, f)
}

/**
 * Construct a string value with VType = Scalar/String. Empty vs
 * non-empty no longer affects the type — they share the kind, which
 * is what `equal(TString)` checks against.
 */
export function newString(s: string): Value {
  return new Value(TString, s)
}

export function newBoolean(b: boolean): Value {
  return new Value(TBoolean, b)
}

export function newAtom(name: string): Value {
  return new Value(TAtom, name)
}

export function newTypeLiteral(t: AqlType): Value {
  return new Value(t, null)
}

export function newWord(name: string): Value {
  return new Value(TWord, { name } satisfies WordInfo)
}

/** Convenience for constructing an Any-typed carrier (test harness use). */
export function newAny(data: unknown): Value {
  return new Value(TAny, data)
}

/**
 * Construct a list value with VType = Node/List. Data is the array
 * of element Values. Mirrors Go's NewList — no `Eval=true` flag here
 * because the spec subset doesn't reach the auto-evaluation path.
 */
export function newList(elems: Value[]): Value {
  return new Value(TList, elems)
}

/**
 * Construct a function-definition value. VType = Word/Function so
 * sigTypeMatches can route fn refs through TFunction sig slots, and
 * the engine's def-sub branch knows to dispatch instead of substitute.
 */
export function newFnDef(info: FnDefInfo): Value {
  return new Value(TFunction, info)
}
