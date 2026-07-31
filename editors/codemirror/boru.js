// boru.js — a CodeMirror 6 language mode for boru (the concatenative,
// strongly-typed query language). Implemented as a StreamLanguage so it needs
// no Lezer grammar build step and stays dependency-light.
//
// It exports:
//   * boruLanguage — a StreamLanguage instance (via StreamLanguage.define)
//   * boru()       — a convenience extension array [boruLanguage, ...]
//
// This mode mirrors the vocabulary and faces of the reference Emacs mode
// (editors/emacs/boru-mode.el) and also drives the boru web playground.
//
// The token classifier recognises:
//   - line comments  (`# ...`  and  `// ...`)
//   - block comments (`/* ... */`, non-nesting, spanning lines via state)
//   - three string kinds: 'single' "double" `template` with `${ ... }`
//     interpolation (the embedded expression is highlighted distinctly)
//   - all number forms (decimal int/float, 0x hex, 0b binary, 0d bigint,
//     underscore digit separators, optional leading `-`)
//   - constant words: true false none inf nan (and -inf -nan)
//   - keyword words, builtin words
//   - capitalised type names / module namespaces
//   - `def Foo`/`def foo` binding targets
//   - `/modifier` dispatch suffixes  (/q /s /f /r /t /u /<digit>, combinations)
//
// Self-check: `node --check boru.js` — the only import is guarded below so the
// bare syntax check passes even without the CodeMirror packages installed.

// `@codemirror/language` is a peer dependency, present in any real editor
// bundle. `node --check` only PARSES this module (it never executes the import
// or resolves the specifier), so the bare syntax self-check passes even when
// the peer dep is not installed; the import resolves normally at runtime.
import { StreamLanguage } from "@codemirror/language";

// --- Vocabulary -----------------------------------------------------------
// Kept in sync with editors/emacs/boru-mode.el and the boru lexical spec.

const KEYWORDS = new Set([
  "def", "undef", "var", "fn", "afn", "class", "surface", "refine", "gen",
  "exposes", "make", "import", "export", "module", "load", "create", "update",
  "remove", "do", "if", "case", "for", "for-each", "each", "fold", "scan",
  "outer", "inner", "filter", "walk", "break", "continue", "guard", "raise",
  "return", "otherwise", "quote", "unquote", "splice", "word", "macro",
  "macroexpand", "codequote", "unpack", "unpack-prefix", "call", "apply",
  "from", "where", "select", "order", "group", "having", "join", "limit",
  "offset", "distinct", "union", "between", "in", "with", "with-decimal",
  "convert", "typeof", "inspect", "is", "istype", "of", "extends", "default",
  "const", "base", "behave", "pathof",
]);

const BUILTINS = new Set([
  // math
  "add", "sub", "mul", "div", "mod", "pow", "abs", "negate", "min", "max",
  "sign", "ceil", "floor", "round", "trunc", "round-even", "logb", "scalb",
  "fma", "sqrt", "cbrt", "exp", "log", "log2", "log10", "sin", "cos", "tan",
  "asin", "acos", "atan", "atan2", "hypot", "is-nan", "is-inf", "is-finite",
  "signbit", "remainder", "copysign", "nextafter", "math-pi", "math-e",
  // compare
  "lt", "gt", "lte", "gte", "cmp", "tcmp", "eq", "neq", "deq",
  // boolean
  "and", "or", "not", "xor", "nand", "implies", "nor", "iff", "xnor", "any",
  "all",
  // binary
  "band", "bor", "bxor", "bnot", "bsl", "bsr", "busr",
  // string
  "upper", "lower", "concat", "split", "trim", "contains", "indexof",
  "replace", "changecase", "normalize", "repeat", "pad", "match", "escape",
  // stack
  "dup", "swap", "drop", "over", "rot", "nip", "tuck", "dup2", "swap2",
  "drop2", "over2", "depth", "pick", "roll", "stack",
  // list
  "iota", "range", "reverse", "sort", "flatten", "slice", "take", "shed",
  "append", "push", "pop", "shift", "unshift", "size", "enum", "node", "flex",
  // xml
  "xml-elem", "xml-text", "xml-attr",
  // storage / access
  "set", "get", "getr", "dot", "dotr", "has", "context", "keys", "vals",
  "reach", "rebind", "ref", "referent", "patrun", "find", "patterns",
  // type algebra
  "tor", "tand", "tany", "tall", "teq", "tpartial", "tis", "tnot", "fnsig",
  "unify",
  // macro / meta
  "gensym", "canon", "mini", "parse", "emit",
  // io
  "print", "printstr", "read", "write", "trace", "stdin", "stdout", "stderr",
  // control helpers
  "args", "error", "force-arity", "usurp", "forward-args", "stack-args",
  // concurrency
  "spawn", "self", "send", "receive", "register", "whereis", "unregister",
  "service", "state-of", "wrap",
  // help
  "help", "describe",
]);

// Constant words. `inf`/`nan` also as `-inf`/`-nan`; handled where numbers are
// scanned so the leading `-` is consumed as part of the constant.
const CONSTANTS = new Set(["true", "false", "none", "inf", "nan"]);

// A word: [A-Za-z][A-Za-z0-9_-]* — hyphens/underscores are internal.
const WORD_RE = /^[A-Za-z][A-Za-z0-9_-]*/;

// Number forms (no leading sign — the sign is handled by the caller):
//   0x hex, 0b binary, 0d bigint, decimal float w/ exponent, decimal int.
// Underscore digit separators are allowed inside the digit runs.
const NUMBER_RE =
  /^(?:0x[0-9a-fA-F_]+|0b[01_]+|0d[0-9]+|[0-9][0-9_]*(?:\.[0-9_]+)?(?:[eE][+-]?[0-9]+)?)/;

// A `/modifier` dispatch suffix: /q /s /f /r /t /u letters (combinable) or a
// forced-arity /<digit>. Matched only when it directly follows a word/group.
const MODIFIER_RE = /^\/(?:[qsfrtu]+|[0-9]+)/;

// --- Classifier helpers ---------------------------------------------------

// Classify a bare word already captured from the stream.
function classifyWord(word) {
  if (CONSTANTS.has(word)) {
    // true/false read as booleans; none/inf/nan read as atoms/constants.
    return word === "true" || word === "false" ? "bool" : "atom";
  }
  if (KEYWORDS.has(word)) return "keyword";
  if (BUILTINS.has(word)) return "builtin";
  // Capitalised identifier -> a type name or module namespace.
  if (/^[A-Z]/.test(word)) return "typeName";
  return "variableName";
}

// --- Sub-scanners ----------------------------------------------------------

// Consume the body of a single- or double-quoted string. Returns true when the
// closing quote was reached on this line (so the caller can clear the state).
// Handles backslash escapes; an unterminated string ends at end-of-line.
function scanQuoted(stream, quote) {
  let escaped = false;
  while (!stream.eol()) {
    const ch = stream.next();
    if (escaped) {
      escaped = false;
      continue;
    }
    if (ch === "\\") {
      escaped = true;
      continue;
    }
    if (ch === quote) return true;
  }
  return false;
}

// --- The StreamParser ------------------------------------------------------

const parser = {
  // Per-document mutable state carried across lines.
  startState() {
    return {
      inBlockComment: false, // inside a /* ... */ run
      inString: null,        // active quote char for a multi-line string, or null
      // Template-string interpolation nesting. When > 0 we are inside a
      // `${ ... }` embedded expression and tokenise boru normally, tracking
      // `{`/`}` depth so the closing `}` returns us to template text.
      templateStack: [],     // stack of interpolation `{` depths, per open backtick
      interpDepth: 0,        // brace depth inside the current `${ ... }`
      afterDef: false,       // previous significant token was `def`/`var`/`fn`...
      afterDefKw: null,      // which keyword set afterDef (for binding colouring)
    };
  },

  token(stream, state) {
    // 1) Continue a multi-line block comment.
    if (state.inBlockComment) {
      if (stream.skipTo("*/")) {
        stream.match("*/");
        state.inBlockComment = false;
      } else {
        stream.skipToEnd();
      }
      return "comment";
    }

    // 2) Continue a multi-line ordinary string ('...' or "...").
    if (state.inString) {
      if (scanQuoted(stream, state.inString)) state.inString = null;
      return "string";
    }

    // 3) Continue a multi-line template string (backtick), unless we are
    //    currently inside a `${ ... }` interpolation (handled as code below).
    if (state.templateStack.length > 0 && state.interpDepth === 0) {
      return scanTemplate(stream, state);
    }

    // Skip whitespace between tokens.
    if (stream.eatSpace()) return null;

    const ch = stream.peek();

    // 4) Comments.
    if (ch === "#") {
      stream.skipToEnd();
      return "comment";
    }
    if (ch === "/" && stream.match("//", false)) {
      stream.skipToEnd();
      return "comment";
    }
    if (ch === "/" && stream.match("/*", false)) {
      stream.match("/*");
      if (stream.skipTo("*/")) {
        stream.match("*/");
      } else {
        stream.skipToEnd();
        state.inBlockComment = true;
      }
      return "comment";
    }

    // 5) Strings.
    if (ch === "'" || ch === '"') {
      stream.next();
      if (!scanQuoted(stream, ch)) state.inString = ch; // spans to next line
      return "string";
    }
    if (ch === "`") {
      stream.next();
      state.templateStack.push(0);
      return scanTemplate(stream, state);
    }

    // 6) Close of a `${ ... }` interpolation: a `}` at interp depth 0 returns
    //    us to template text.
    if (ch === "}" && state.templateStack.length > 0 && state.interpDepth === 0) {
      stream.next();
      return "operator";
    }

    // Track `{`/`}` nesting while inside an interpolation so a nested map
    // literal's `}` does not prematurely close the `${ ... }`.
    if (state.templateStack.length > 0 && state.interpDepth > 0) {
      if (ch === "{") {
        stream.next();
        state.interpDepth++;
        return "operator";
      }
      if (ch === "}") {
        stream.next();
        state.interpDepth--;
        return "operator";
      }
    }

    // 7) Numbers (with an optional leading `-`). A lone `-` that is not a
    //    number becomes an operator below. `-inf`/`-nan` are constants.
    if (ch === "-" || (ch >= "0" && ch <= "9")) {
      const neg = ch === "-";
      if (neg && stream.match(/^-(?:inf|nan)\b/)) {
        return "atom";
      }
      // Try a signed number: consume the `-` only if a digit follows.
      const m = stream.match(neg ? /^-(?=[0-9])/ : /^(?=[0-9])/, false);
      if (m) {
        if (neg) stream.next(); // eat the sign
        if (stream.match(NUMBER_RE)) return "number";
        // Should not happen given the look-ahead, but stay safe.
      }
      if (neg) {
        stream.next();
        return "operator";
      }
    }

    // 8) `/modifier` suffix (only meaningful attached to the prior token, but
    //    tokenising it standalone is harmless and keeps the scanner simple).
    if (ch === "/" && stream.match(MODIFIER_RE)) {
      return "operator";
    }

    // 9) Words (identifiers, keywords, builtins, types, constants).
    if (/[A-Za-z]/.test(ch)) {
      const m = stream.match(WORD_RE);
      const word = m[0];

      // `def Foo`/`def foo` binding target colouring.
      if (state.afterDef) {
        state.afterDef = false;
        const kw = state.afterDefKw;
        state.afterDefKw = null;
        // `def` may bind a type (capitalised) or a value/function.
        if (kw === "def" && /^[A-Z]/.test(word)) return "typeName";
        // def/var/fn lowercase target -> a definition (function/value) name.
        // `variableName.definition` is a base tag + the `definition` modifier;
        // `definition` alone is a modifier and would not resolve to a style.
        if (/^[a-z_]/.test(word)) return "variableName.definition";
        // Fall through to normal classification for odd cases.
      }

      const style = classifyWord(word);

      // Arm binding-target colouring for the *next* word after a binder.
      if (word === "def" || word === "var" || word === "fn") {
        state.afterDef = true;
        state.afterDefKw = word;
      }

      // A capitalised word immediately followed by `.` is a module namespace
      // access (Pkg.word) — still a typeName face, which is what we return.
      return style;
    }

    // 10) Operators & punctuation.
    //     .  !.  ;  :  |  =>  and brackets/braces.
    if (stream.match("=>")) return "operator";
    if (stream.match("!.")) return "operator";
    if (ch === "." || ch === ";" || ch === ":" || ch === "|" ||
        ch === "!" || ch === "=" || ch === "," || ch === "@" ||
        ch === "%" || ch === "?" || ch === "$" || ch === "&") {
      stream.next();
      return "operator";
    }
    if (ch === "(" || ch === ")" || ch === "[" || ch === "]" ||
        ch === "{" || ch === "}") {
      stream.next();
      return "bracket";
    }

    // Fallback: consume one char so the stream always advances.
    stream.next();
    return null;
  },

  languageData: {
    commentTokens: { line: "#", block: { open: "/*", close: "*/" } },
    closeBrackets: { brackets: ["(", "[", "{", '"', "'", "`"] },
    wordChars: "-_",
  },
};

// Scan template-string text starting at (or continuing into) a backtick run.
// Emits "string" for literal text and, on encountering `${`, opens an
// interpolation by setting interpDepth and returning the `${` as an operator so
// the embedded expression is tokenised as ordinary boru code.
function scanTemplate(stream, state) {
  let escaped = false;
  while (!stream.eol()) {
    // Start of an interpolation: `${`.
    if (!escaped && stream.match("${", false)) {
      if (stream.pos > stream.start) {
        // Emit the literal text accumulated so far first; the `${` is handled
        // on the next token() call.
        return "string";
      }
      stream.match("${");
      state.interpDepth = 1;
      return "operator";
    }
    const chr = stream.next();
    if (escaped) {
      escaped = false;
      continue;
    }
    if (chr === "\\") {
      escaped = true;
      continue;
    }
    if (chr === "`") {
      // Close of this template string.
      state.templateStack.pop();
      return "string";
    }
  }
  // Reached end of line inside a template string — it continues next line.
  return "string";
}

// --- Public API ------------------------------------------------------------

/**
 * The boru language, as a CodeMirror StreamLanguage.
 * @type {import("@codemirror/language").StreamLanguage<unknown>}
 */
export const boruLanguage = StreamLanguage.define(parser);

/**
 * Convenience extension: the boru language plus room for companion extensions
 * (e.g. a highlight style) added by callers.
 * @returns {import("@codemirror/state").Extension[]}
 */
export function boru() {
  return [boruLanguage];
}

export default boru;
