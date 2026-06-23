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
  TDisjunct,
  TEnum,
  TFnUndef,
  TFloat,
  TForward,
  TInspect,
  TFunction,
  TInteger,
  TList,
  TMap,
  TMark,
  TMove,
  TNone,
  TString,
  TStringEmpty,
  TStringProper,
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
  /** Optional (`?`) params default to their type's base value when omitted. */
  optional?: boolean
}

/**
 * A typed parameter on a function definition.
 *
 * Forward dispatch deferral:
 *
 * When a function's match has uncollected forward args at dispatch
 * time, we replace the function word with a `ForwardMarker` Value.
 * The engine continues stepping; intervening function calls produce
 * literals that flow back to the marker via stepLiteral. Once
 * `collected.length === expectedForward`, the marker fires the
 * original handler and is itself replaced by the handler's result.
 * Mirrors the (insertForward, stepLiteral, ForwardInfo) trio in
 * aqleng/go/engine.go.
 */
export interface ForwardMarker {
  funcName: string
  sig: import('./signature.ts').Signature
  expectedForward: number
  /** Forward args in collection order (== sig order from sig[0]). */
  collected: Value[]
  /** Stack args resolved at match time. Combined with `collected` to
   *  build the full sig-ordered arg list. Stored in sig order. */
  stackArgs: Value[]
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
  /**
   * For TList values: when true, the list contents will auto-evaluate
   * when the list is consumed as a word argument (or when it's
   * unconsumed at end of Run). Parser/tokenizer produces lists with
   * eval=true; quote produces lists with quoted=true to suppress.
   * Mirrors Go Value.Eval / Value.Quoted on TList.
   */
  readonly eval: boolean
  readonly quoted: boolean
  readonly carrier: boolean

  constructor(
    vType: AqlType,
    data: unknown,
    opts?: { eval?: boolean; quoted?: boolean; carrier?: boolean },
  ) {
    this.vType = vType
    this.data = data
    this.eval = opts?.eval ?? false
    this.quoted = opts?.quoted ?? false
    this.carrier = opts?.carrier ?? false
  }

  /** True iff this Value is the unique `none` value (None's sole inhabitant). */
  isNone(): boolean {
    return this.vType.equal(TNone) && this.data === NONE_SENTINEL
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

  asFloat(): number {
    if (this.data === null) throw new Error('AsFloat: nil data')
    if (typeof this.data !== 'number') {
      throw new Error(`AsFloat: not a float value (got ${typeof this.data})`)
    }
    return this.data
  }

  /** Back-compat alias for asFloat. */
  asDecimal(): number {
    return this.asFloat()
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

  isMap(): boolean {
    return this.vType.equal(TMap)
  }

  asMap(): OrderedMap {
    if (this.data === null || !(this.data instanceof OrderedMap)) {
      throw new Error('AsMap: not a map value')
    }
    return this.data
  }

  /** True iff this is a typed list `[:T]` (ChildType payload, VType List). */
  isTypedList(): boolean {
    return this.data instanceof ChildType && this.vType.equal(TList)
  }

  /** True iff this is a typed map `{:T}` (ChildType payload, VType Map). */
  isTypedMap(): boolean {
    return this.data instanceof ChildType && this.vType.equal(TMap)
  }

  asChildType(): ChildType {
    if (!(this.data instanceof ChildType)) throw new Error('AsChildType: not a typed container')
    return this.data
  }

  /** True iff this is a Disjunct or Enum value (carries DisjunctInfo). */
  isDisjunct(): boolean {
    return this.vType.matches(TDisjunct) && this.data !== null
  }

  asDisjunct(): DisjunctInfo {
    if (this.data === null || typeof this.data !== 'object') {
      throw new Error('AsDisjunct: not a disjunct value')
    }
    return this.data as DisjunctInfo
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

  isForward(): boolean {
    return this.vType.equal(TForward)
  }

  asForward(): ForwardMarker {
    if (this.data === null) throw new Error('AsForward: nil data')
    return this.data as ForwardMarker
  }

  isMark(): boolean {
    return this.vType.equal(TMark)
  }

  asMark(): MarkInfo {
    if (this.data === null) throw new Error('AsMark: nil data')
    return this.data as MarkInfo
  }

  isMove(): boolean {
    return this.vType.equal(TMove)
  }

  asMove(): MoveInfo {
    if (this.data === null) throw new Error('AsMove: nil data')
    return this.data as MoveInfo
  }

  /** Stringify in a parser-style debug form: words as word(name), strings quoted, etc. */
  toString(): string {
    if (this.isNone()) return 'none'
    if (this.data === null) {
      if (this.vType.equal(TNone)) return 'none'
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
    if (this.vType.matches(TFloat)) {
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
    if (this.data instanceof OrderedMap) {
      const m = this.data
      const parts = m.sortedKeys().map((k) => `${k}:${m.get(k)!.toString()}`)
      return `{${parts.join(' ')}}`
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
 * instead — see PORT_OBSERVATIONS.5.md §1.1.
 *
 * BigInt is used because the Go engine carries int64 — TS's number
 * loses precision past 2^53.
 */
export function newInteger(n: bigint | number): Value {
  const big = typeof n === 'bigint' ? n : BigInt(n)
  return new Value(TInteger, big)
}

export function newFloat(f: number): Value {
  return new Value(TFloat, f)
}

/** Back-compat alias: the decimal scalar is named Float in the lattice. */
export const newDecimal = newFloat

/**
 * Construct a string value. The VType carries the concrete leaf so
 * `typeof` renders the precise name: a non-empty string is
 * Scalar/String/ProperString, an empty string is
 * Scalar/String/EmptyString. Mirrors eng/go/value.go::NewString.
 */
export function newString(s: string): Value {
  return new Value(s.length === 0 ? TStringEmpty : TStringProper, s)
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

// The unique payload that marks the `none` value (None's sole
// inhabitant), distinguishing it from the `None` type literal (which
// has a null payload). Mirrors eng/go's noneSentinel.
const NONE_SENTINEL: unique symbol = Symbol('none')

/** Construct the unique `none` value. */
export function newNone(): Value {
  return new Value(TNone, NONE_SENTINEL)
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
 * of element Values. The `eval` option controls auto-evaluation when
 * the list is consumed as a word argument: parser/tokenizer-built
 * lists pass true; lists synthesised by handlers (e.g. quote, fn
 * params) pass false. Mirrors Go's NewList vs NewEvalList split.
 */
export function newList(elems: Value[], opts?: { eval?: boolean; quoted?: boolean }): Value {
  return new Value(TList, elems, { eval: opts?.eval, quoted: opts?.quoted })
}

/**
 * OrderedMap preserves key insertion order. Mirrors
 * eng/go/value.go::OrderedMap (the subset the spec reaches).
 */
export class OrderedMap {
  private readonly _keys: string[] = []
  private readonly vals = new Map<string, Value>()

  set(key: string, val: Value): void {
    if (!this.vals.has(key)) this._keys.push(key)
    this.vals.set(key, val)
  }

  get(key: string): Value | undefined {
    return this.vals.get(key)
  }

  has(key: string): boolean {
    return this.vals.has(key)
  }

  keys(): string[] {
    return [...this._keys]
  }

  /** Keys in sorted order — the canonical render order. */
  sortedKeys(): string[] {
    return [...this._keys].sort()
  }

  get size(): number {
    return this._keys.length
  }
}

/** Construct a map value with VType = Node/Map wrapping an OrderedMap. */
export function newMap(m: OrderedMap): Value {
  return new Value(TMap, m)
}

/**
 * Construct an inspection map (VType Node/Map/Inspect). Renders in
 * insertion order with word values bare (e.g. `kind:native`), unlike a
 * plain map which sorts keys.
 */
export function newInspect(m: OrderedMap): Value {
  return new Value(TInspect, m)
}

/**
 * ChildType backs a typed list `[:T]` / typed map `{:T}` (and their
 * concrete-with-elements forms `[:T 1 2 3]`). Mirrors the
 * ChildTypeInfo payload in eng/go/value.go. `child` is the element/value
 * type constraint (a type literal); `elements` carries concrete
 * elements when present.
 */
export class ChildType {
  readonly child: Value
  readonly elements: Value[]
  constructor(child: Value, elements: Value[] = []) {
    this.child = child
    this.elements = elements
  }
}

/** Construct a typed list value (VType Node/List, ChildType payload). */
export function newTypedList(child: Value, elements: Value[] = []): Value {
  return new Value(TList, new ChildType(child, elements))
}

/** Construct a typed map value (VType Node/Map, ChildType payload). */
export function newTypedMap(child: Value): Value {
  return new Value(TMap, new ChildType(child))
}

/**
 * DisjunctInfo holds the alternatives of a disjunction (union) type, or
 * the members of an enum. Mirrors eng/go/value.go::DisjunctInfo.
 */
export interface DisjunctInfo {
  alternatives: Value[]
}

/** Construct a Disjunct (union) value — VType Type/Disjunct. */
export function newDisjunct(alternatives: Value[]): Value {
  return new Value(TDisjunct, { alternatives } satisfies DisjunctInfo)
}

/** Construct a FunctionSignature (fnsig) value — VType Type/FunctionSignature. */
export function newFnUndef(sigs: Value[]): Value {
  return new Value(TFnUndef, sigs)
}

/** Construct an Enum value — VType Type/Disjunct/Enum (a Disjunct subtype). */
export function newEnum(alternatives: Value[]): Value {
  return new Value(TEnum, { alternatives } satisfies DisjunctInfo)
}

/** Clone a Value with `quoted=true` (used by `quote` for lists). */
export function withQuoted(v: Value): Value {
  return new Value(v.vType, v.data, { eval: v.eval, quoted: true, carrier: v.carrier })
}

/**
 * Construct a function-definition value. VType = Word/Function so
 * sigTypeMatches can route fn refs through TFunction sig slots, and
 * the engine's def-sub branch knows to dispatch instead of substitute.
 */
export function newFnDef(info: FnDefInfo): Value {
  return new Value(TFunction, info)
}

/**
 * Construct a forward-dispatch marker. The engine inserts these in
 * place of a function word whose match still needs forward args; as
 * those args resolve they're collected into `marker.collected` and
 * the marker fires once full.
 */
export function newForwardMarker(info: ForwardMarker): Value {
  return new Value(TForward, info)
}

/**
 * MarkInfo / MoveInfo: control-flow primitives. A handler emits a
 * Mark followed by a body sequence followed by a Move; when the
 * engine reaches the Move it walks back to the matching Mark,
 * splices the original body in place of [Mark .. body .. Move], and
 * sets the pointer to the start so the body re-runs. One-shot
 * replay only — the body is removed when the move fires.
 */
export interface MarkInfo {
  id: string
  body: Value[]
}

export interface MoveInfo {
  to: string
}

export function newMark(id: string, body: Value[]): Value {
  return new Value(TMark, { id, body } satisfies MarkInfo)
}

export function newMove(to: string): Value {
  return new Value(TMove, { to } satisfies MoveInfo)
}
