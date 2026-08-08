// `if` — the TS twin of basic/go's conditional.go plus native_control.go's
// three `if` signatures.
//
// PORT STATUS: the RUNTIME half, all three call shapes. No ReturnsFn, so
// no analysis half — Go's if3ReturnsFn/If2ReturnsFn/IfListReturnsFn do
// static branch reduction, unreachable-branch diagnostics and carrier
// joins, none of which core/ts's lattice models yet.
//
// The mechanism worth reading twice is the LIST condition. A condition
// that is a code body cannot be evaluated before the branches are chosen:
// running it eagerly would run it in the wrong place on the tape, ahead
// of whatever precedes the `if`. So the word emits
//
//     Mark(id, cond…) · cond… · MoveIf(id, { then, else })
//
// and hands that back as its result. The engine runs the condition where
// it stands and the move reads its RESULT to splice in one branch. That
// is why core/ts needed `newMoveIf` before this file could exist.

import {
  coerceBoolean,
  newCloseParen,
  newMark,
  newMoveIf,
  newOpenParen,
  nextMarkId,
  TAny,
  TList,
  type NativeFunc,
  type Value,
} from "@boru-lang/core";

/**
 * A branch's token span. A code body is wrapped in PARENS rather than
 * spliced bare: the branch's values must be sealed off from whatever
 * surrounds the `if`, or a trailing forward word would collect through
 * them. Any other value is itself, one token.
 */
function spliceArg(v: Value): Value[] {
  if (!isCodeBody(v)) return [v];
  return [newOpenParen(), ...v.asList(), newCloseParen()];
}

/** A plain concrete list — something to RUN, as against a value to use. */
function isCodeBody(v: Value): boolean {
  return v.vType.equal(TList) && Array.isArray(v.data);
}

/**
 * The mark/move token span for a LIST condition, or undefined when the
 * condition is a plain value the caller should coerce directly. Shared by
 * the two- and three-argument forms; the two-argument one passes no else.
 */
function ifMarkMoveTokens(
  cond: Value,
  thenBranch: Value[],
  elseBranch?: Value[],
): Value[] | undefined {
  if (!isCodeBody(cond)) return undefined;
  const body = cond.asList();
  const id = nextMarkId();
  return [
    newMark(id, [...body]),
    ...body,
    newMoveIf(
      id,
      "if",
      elseBranch === undefined
        ? { then: thenBranch }
        : { then: thenBranch, else: elseBranch },
    ),
  ];
}

function if3Handler(args: Value[]): Value[] {
  const thenBranch = spliceArg(args[1]!);
  const elseBranch = spliceArg(args[2]!);
  const tokens = ifMarkMoveTokens(args[0]!, thenBranch, elseBranch);
  if (tokens !== undefined) return tokens;
  return coerceBoolean(args[0]!) ? thenBranch : elseBranch;
}

function if2Handler(args: Value[]): Value[] {
  const thenBranch = spliceArg(args[1]!);
  const tokens = ifMarkMoveTokens(args[0]!, thenBranch);
  if (tokens !== undefined) return tokens;
  // No else: a false condition produces NOTHING, which is what makes the
  // two-argument form a statement guard rather than an expression.
  return coerceBoolean(args[0]!) ? thenBranch : [];
}

/**
 * The clause-list form `if [c1 b1 c2 b2 … else]`. Even elements are
 * conditions, each followed by its body, and a trailing element (an
 * odd-length list) is the else. Lowered right-to-left into nested
 * two-way ifs, so the first truthy condition's body runs and the rest
 * are never evaluated.
 */
function ifClause(elems: Value[]): Value[] {
  if (elems.length === 0) return [];
  if (elems.length === 1) return spliceArg(elems[0]!);
  const cond = elems[0]!;
  const thenBranch = spliceArg(elems[1]!);
  const elseBranch = ifClause(elems.slice(2));
  const tokens = ifMarkMoveTokens(cond, thenBranch, elseBranch);
  if (tokens !== undefined) return tokens;
  return coerceBoolean(cond) ? thenBranch : elseBranch;
}

function ifListHandler(args: Value[]): Value[] {
  // Go guards here with an IsConcrete check raising if_error. That guard
  // is NOT reachable in the TS pieces, for the same reason `do`'s is not:
  // a bare type literal fails to match the TList signature and dispatch
  // raises signature_error first (pinned by the `if List` row). Go's
  // carriers come from check mode, which core/ts does not have, so
  // writing the guard would be unreachable code — ADR-008 removes dead
  // code. It comes back with check mode, and the row changes with it.
  return ifClause([...args[0]!.asList()]);
}

export const conditionalNatives: NativeFunc[] = [
  {
    name: "if",
    signatures: [
      // Order matters: the three-argument form must be tried first, or a
      // trailing else would be read as a clause list.
      {
        args: [TAny, TAny, TAny],
        noEvalArgs: new Set([0, 1, 2]),
        handler: if3Handler,
        barrierPos: 3,
      },
      {
        args: [TAny, TAny],
        noEvalArgs: new Set([0, 1]),
        handler: if2Handler,
        barrierPos: 2,
      },
      {
        args: [TList],
        noEvalArgs: new Set([0]),
        handler: ifListHandler,
        barrierPos: 1,
      },
    ],
  },
];
