// Sugar-marker lowering — the engine-side half of ADR-012 rule 3
// (2026-08-04 amendment). The parser emits structural Word/__SG markers
// (value.ts newSugar); when a marker is stepped the engine lowers it to
// word dispatches through the registry's sugar-role table
// (Registry.bindSugarWord / sugarWord). A registry with no binding for
// a stepped role fails loudly (`sugar_unbound`), so a host that doesn't
// support a sugar refuses cleanly and the bare kernel runs with zero
// bindings. Mirrors eng/go/sugar.go.

import { BoruError } from "./error.ts";
import type { Registry } from "./registry.ts";
import { TType } from "./type.ts";
import {
  type SugarInfo,
  type Value,
  newInteger,
  newList,
  newParenExpr,
  newString,
  newTypeLiteral,
  newWord,
} from "./value.ts";

/**
 * Resolve a role binding to a Word token, or raise loudly when the
 * registry binds no word for the role. Mirrors Go sugarBoundWord.
 */
function sugarBoundWord(r: Registry, kind: string): Value {
  const name = r.sugarWord(kind);
  if (name === undefined) {
    throw new BoruError(
      "sugar_unbound",
      `no word bound for sugar role "${kind}" — the host registry does not support this surface form`,
      kind,
    );
  }
  return newWord(name);
}

/**
 * Lower a sugar marker to the token run that replaces it on the tape.
 * headForm selects the Angle marker's generic-def head expansion
 * (`Name gen [params]`) over the use-site paren (`(Name of [args])`).
 * Mirrors Go SugarExpansion.
 */
export function sugarExpansion(
  r: Registry,
  info: SugarInfo,
  headForm: boolean,
): Value[] {
  switch (info.kind) {
    case "usurp":
    case "stack-args":
    case "forward-args":
    case "lambda":
    case "gen-default":
      return [sugarBoundWord(r, info.kind)];
    case "force-arity":
      return [sugarBoundWord(r, info.kind), newInteger(info.n ?? 0n)];
    case "mini":
      return [
        sugarBoundWord(r, info.kind),
        newWord(info.name ?? ""),
        newString(info.src ?? ""),
      ];
    case "type-bound":
      return [
        newParenExpr([
          newTypeLiteral(TType),
          sugarBoundWord(r, "gen-apply"),
          ...(info.items ?? []),
        ]),
      ];
    case "angle": {
      const recv = newWord(info.name ?? "");
      if (headForm) {
        if (info.headErr !== undefined && info.headErr !== "") {
          throw new BoruError("syntax_error", info.headErr, info.name ?? "");
        }
        return [recv, sugarBoundWord(r, "gen-head"), info.head!];
      }
      const args = newList(info.items ?? [], { eval: true });
      return [newParenExpr([recv, sugarBoundWord(r, "gen-apply"), args])];
    }
  }
  throw new BoruError(
    "sugar_unbound",
    `unknown sugar kind "${String(info.kind)}"`,
    String(info.kind),
  );
}
