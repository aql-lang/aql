// @boru-lang/basic — the boru base language layer, the TS twin of the
// basic/go module.
//
// Same dependency rule as its Go twin (ADR-013, as amended 2026-08-08): it
// depends on @boru-lang/core and NOTHING else. A word's analysis half
// belongs in core's carrier vocabulary, not in a dependency on the
// checker.
//
// PORT STATUS: the stack vocabulary (all of it, full-stack words
// included), the escape hatch (`do` and `error`) and `if` in all three
// of its call shapes. The rest of
// basic/go's surface (the definition, conditional, loop and type-generics
// words, and the predefined content types) is not ported yet;
// native-control.ts records what each remaining word is waiting on.

export { stackNatives } from "./native-stack.ts";
export { controlNatives } from "./native-control.ts";
