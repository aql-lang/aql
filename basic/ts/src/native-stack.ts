// The stack-manipulation primitives — the TS twin of
// basic/go/native_stack.go.
//
// All are stack-only (barrierPos 0). The argument convention is the
// post-§1.4 unified one: args[0] is the TOP of stack, args[1] the
// next-deeper element, and so on. Splice ordering: the returned array is
// laid back onto the stack in source order, so an N-arg word returning
// the same N values leaves the inputs unchanged — `swap` is the worked
// example (args[0] is the top, and returning [args[0], args[1]] puts the
// old top underneath).
//
// That inversion is the single thing to get right when reading these
// against a Forth reference: `over` is documented as ( a b -- a b a ),
// which in this convention is args[1]=a, args[0]=b and a return of
// [a, b, a] = [args[1], args[0], args[1]].
//
// PARITY NOTE — three words are NOT here yet. `depth`, `pick` and `roll`
// are FULL-STACK words: their Go handlers take the whole stack and return
// a replacement for it, via the `FullStack()` dispatch knob. core/ts has
// no such knob (signature.ts says so explicitly), so porting them means
// adding the capability to core first. They are the next increment, and
// basic/spec/stack.tsv carries no rows for them.

import { returnsIdentity, TAny, type NativeFunc, type Value } from '@boru-lang/core'

/** dup ( a -- a a ) */
function dupHandler(args: Value[]): Value[] {
  return [args[0]!, args[0]!]
}

/** swap ( a b -- b a ) */
function swapHandler(args: Value[]): Value[] {
  return [args[0]!, args[1]!]
}

/** drop ( a -- ) */
function dropHandler(): Value[] {
  return []
}

/** over ( a b -- a b a ) */
function overHandler(args: Value[]): Value[] {
  return [args[1]!, args[0]!, args[1]!]
}

/** rot ( a b c -- b c a ) */
function rotHandler(args: Value[]): Value[] {
  return [args[1]!, args[0]!, args[2]!]
}

/** nip ( a b -- b ) */
function nipHandler(args: Value[]): Value[] {
  return [args[0]!]
}

/** tuck ( a b -- b a b ) */
function tuckHandler(args: Value[]): Value[] {
  return [args[0]!, args[1]!, args[0]!]
}

/** dup2 ( a b -- a b a b ) */
function dup2Handler(args: Value[]): Value[] {
  return [args[1]!, args[0]!, args[1]!, args[0]!]
}

/** swap2 ( a b c d -- c d a b ) */
function swap2Handler(args: Value[]): Value[] {
  return [args[1]!, args[0]!, args[3]!, args[2]!]
}

/** drop2 ( a b -- ) */
function drop2Handler(): Value[] {
  return []
}

/** over2 ( a b c d -- a b c d a b ) */
function over2Handler(args: Value[]): Value[] {
  return [args[3]!, args[2]!, args[1]!, args[0]!, args[3]!, args[2]!]
}

/**
 * stackNatives is the registration slice, mirroring Go's StackNatives —
 * same words, same order, same signatures, so the two registries present
 * an identical vocabulary.
 */
export const stackNatives: NativeFunc[] = [
  {
    name: 'dup',
    signatures: [
      { args: [TAny], handler: dupHandler, returnsFn: returnsIdentity(0, 0), barrierPos: 0 },
    ],
  },
  {
    name: 'swap',
    signatures: [
      { args: [TAny, TAny], handler: swapHandler, returnsFn: returnsIdentity(0, 1), barrierPos: 0 },
    ],
  },
  {
    name: 'drop',
    signatures: [{ args: [TAny], handler: dropHandler, returns: [], barrierPos: 0 }],
  },
  {
    name: 'over',
    signatures: [
      {
        args: [TAny, TAny],
        handler: overHandler,
        returnsFn: returnsIdentity(1, 0, 1),
        barrierPos: 0,
      },
    ],
  },
  {
    name: 'rot',
    signatures: [
      {
        args: [TAny, TAny, TAny],
        handler: rotHandler,
        returnsFn: returnsIdentity(1, 0, 2),
        barrierPos: 0,
      },
    ],
  },
  {
    name: 'nip',
    signatures: [
      { args: [TAny, TAny], handler: nipHandler, returnsFn: returnsIdentity(0), barrierPos: 0 },
    ],
  },
  {
    name: 'tuck',
    signatures: [
      {
        args: [TAny, TAny],
        handler: tuckHandler,
        returnsFn: returnsIdentity(0, 1, 0),
        barrierPos: 0,
      },
    ],
  },
  {
    name: 'dup2',
    signatures: [
      {
        args: [TAny, TAny],
        handler: dup2Handler,
        returnsFn: returnsIdentity(1, 0, 1, 0),
        barrierPos: 0,
      },
    ],
  },
  {
    name: 'swap2',
    signatures: [
      {
        args: [TAny, TAny, TAny, TAny],
        handler: swap2Handler,
        returnsFn: returnsIdentity(1, 0, 3, 2),
        barrierPos: 0,
      },
    ],
  },
  {
    name: 'drop2',
    signatures: [{ args: [TAny, TAny], handler: drop2Handler, returns: [], barrierPos: 0 }],
  },
  {
    name: 'over2',
    signatures: [
      {
        args: [TAny, TAny, TAny, TAny],
        handler: over2Handler,
        returnsFn: returnsIdentity(3, 2, 1, 0, 3, 2),
        barrierPos: 0,
      },
    ],
  },
]
