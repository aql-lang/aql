# -*- coding: utf-8 -*-
"""
    aql_lexer
    ~~~~~~~~~

    A Pygments lexer for AQL (https://github.com/aql-lang/aql), a
    concatenative, strongly-typed query language.  Source files use the
    ``.aql`` extension.

    The vocabulary (keywords / builtins / constants) is kept in sync with the
    reference Emacs mode ``editors/emacs/aql-mode.el`` and with ``aql
    describe``.  Token names are Chroma-compatible so the lexer works both with
    upstream Pygments and Chroma's Python-lexer bridge.

    Install this package (see ``README.md``) to get ``pygmentize -l aql``, or
    run it without installing::

        pygmentize -x -l aql_lexer.py:AqlLexer file.aql

    :license: same as the AQL project.
"""

from pygments.lexer import RegexLexer, bygroups, include, words
from pygments.token import (
    Comment,
    Keyword,
    Name,
    Number,
    Operator,
    Punctuation,
    String,
    Text,
    Whitespace,
)

__all__ = ["AqlLexer"]


# --- Vocabulary -----------------------------------------------------------
#
# Structural / control / declaration / query-DSL words -> Keyword.
KEYWORDS = (
    "def", "undef", "var", "fn", "afn", "class", "surface", "refine", "gen",
    "exposes", "make", "import", "export", "module", "load", "create",
    "update", "remove", "do", "if", "case", "for", "for-each", "each",
    "fold", "scan", "outer", "inner", "filter", "walk", "break", "continue",
    "guard", "raise", "return", "otherwise", "quote", "unquote", "splice",
    "word", "macro", "macroexpand", "codequote", "unpack", "unpack-prefix",
    "call", "apply", "from", "where", "select", "order", "group", "having",
    "join", "limit", "offset", "distinct", "union", "between", "in", "with",
    "with-decimal", "convert", "typeof", "inspect", "is", "istype", "of",
    "extends", "default", "const", "base", "behave", "pathof",
)

# Library / built-in words -> Name.Builtin.
BUILTINS = (
    # math
    "add", "sub", "mul", "div", "mod", "pow", "abs", "negate", "min", "max",
    "sign", "ceil", "floor", "round", "trunc", "round-even", "logb", "scalb",
    "fma", "sqrt", "cbrt", "exp", "log", "log2", "log10", "sin", "cos", "tan",
    "asin", "acos", "atan", "atan2", "hypot", "is-nan", "is-inf", "is-finite",
    "signbit", "remainder", "copysign", "nextafter", "math-pi", "math-e",
    # compare
    "lt", "gt", "lte", "gte", "cmp", "tcmp", "eq", "neq", "deq",
    # boolean
    "and", "or", "not", "xor", "nand", "implies", "nor", "iff", "xnor",
    "any", "all",
    # binary
    "band", "bor", "bxor", "bnot", "bsl", "bsr", "busr",
    # string
    "upper", "lower", "concat", "split", "trim", "contains", "indexof",
    "replace", "changecase", "normalize", "repeat", "pad", "match", "escape",
    # stack
    "dup", "swap", "drop", "over", "rot", "nip", "tuck", "dup2", "swap2",
    "drop2", "over2", "depth", "pick", "roll", "stack",
    # list
    "iota", "range", "reverse", "sort", "flatten", "slice", "take", "shed",
    "append", "push", "pop", "shift", "unshift", "size", "enum", "node",
    "flex",
    # xml
    "xml-elem", "xml-text", "xml-attr",
    # storage / access
    "set", "get", "getr", "dot", "dotr", "has", "context", "keys", "vals",
    "reach", "rebind", "ref", "referent", "patrun", "find", "patterns",
    # type algebra
    "tor", "tand", "tany", "tall", "teq", "tpartial", "tis", "tnot", "fnsig",
    "unify",
    # macro / meta
    "gensym", "canon", "mini", "parse", "emit",
    # io
    "print", "printstr", "read", "write", "trace", "stdin", "stdout",
    "stderr",
    # control helpers
    "args", "error", "force-arity", "usurp", "forward-args", "stack-args",
    # concurrency
    "spawn", "self", "send", "receive", "register", "whereis", "unregister",
    "service", "state-of", "wrap",
    # help
    "help", "describe",
)

# Literal constants -> Keyword.Constant.
CONSTANTS = ("true", "false", "none", "inf", "nan")


# A word is [A-Za-z][A-Za-z0-9_-]* — hyphens and underscores are INTERNAL, so
# a keyword/builtin must be bounded on both sides by a non-word/non-hyphen
# character to keep hyphenated words (math-pi, is-nan) as single tokens and to
# avoid matching inside a larger identifier (e.g. `in` inside `inner`).
_WORD_BOUND = r"(?![\w-])"


class AqlLexer(RegexLexer):
    """Lexer for the AQL concatenative query language."""

    name = "AQL"
    aliases = ["aql"]
    filenames = ["*.aql"]
    mimetypes = ["text/x-aql"]
    url = "https://github.com/aql-lang/aql"

    tokens = {
        "root": [
            # -- whitespace ------------------------------------------------
            (r"\s+", Whitespace),

            # -- comments --------------------------------------------------
            (r"#.*?$", Comment.Single),
            (r"//.*?$", Comment.Single),
            (r"/\*", Comment.Multiline, "comment"),

            # -- strings ---------------------------------------------------
            (r"'", String.Single, "sstring"),
            (r'"', String.Double, "dstring"),
            (r"`", String.Backtick, "template"),

            # -- numbers (leading '-' allowed) -----------------------------
            # Big integer: 0d[0-9]+ (arbitrary precision).
            (r"-?0[dD][0-9]+" + _WORD_BOUND, Number),
            # Hexadecimal.
            (r"-?0[xX][0-9a-fA-F_]+" + _WORD_BOUND, Number.Hex),
            # Binary.
            (r"-?0[bB][01_]+" + _WORD_BOUND, Number.Bin),
            # Float: mantissa with a fractional part, optional exponent ...
            (r"-?[0-9][0-9_]*\.[0-9_]+(?:[eE][+-]?[0-9]+)?" + _WORD_BOUND,
             Number.Float),
            # ... or a bare exponent form (1e9, 2E-3).
            (r"-?[0-9][0-9_]*[eE][+-]?[0-9]+" + _WORD_BOUND, Number.Float),
            # Decimal integer.
            (r"-?[0-9][0-9_]*" + _WORD_BOUND, Number.Integer),

            # -- constant words --------------------------------------------
            # Signed non-finite constants (-inf, -nan) as well as the bare
            # forms; the plain names are also caught by the `words(...)` rule
            # below, but the signed spellings must be handled explicitly.
            (r"-(?:inf|nan)" + _WORD_BOUND, Keyword.Constant),
            (words(CONSTANTS, prefix=r"(?<![\w-])", suffix=_WORD_BOUND),
             Keyword.Constant),

            # -- def/undef/var/fn binding target ---------------------------
            # `def Foo …` names a (capitalised) type; `def foo …` names a
            # value/function.  Colour the binding name accordingly.
            (r"(def)(\s+)([A-Z][A-Za-z0-9_-]*)",
             bygroups(Keyword, Whitespace, Name.Class)),
            (r"(def|undef|var|fn|afn)(\s+)([a-z_][A-Za-z0-9_-]*)",
             bygroups(Keyword, Whitespace, Name.Function)),

            # -- keywords --------------------------------------------------
            (words(KEYWORDS, prefix=r"(?<![\w-])", suffix=_WORD_BOUND),
             Keyword),

            # -- builtins --------------------------------------------------
            (words(BUILTINS, prefix=r"(?<![\w-])", suffix=_WORD_BOUND),
             Name.Builtin),

            # -- type names / module namespaces ----------------------------
            # Any capitalised identifier: Integer, Emailon, MathUtil, user
            # types, …  A trailing ``.word`` makes it a namespace access.
            (r"[A-Z][A-Za-z0-9]*", Name.Class),

            # -- dispatch / path modifier suffixes -------------------------
            # /q /s /f /r /t /u and combinations, or /<digit> force-arity.
            (r"/(?:[qsfrtu]+|[0-9]+)" + _WORD_BOUND, Operator),

            # -- lambda arrow / operators ----------------------------------
            (r"=>", Operator),
            (r"!\.", Operator),        # strict access (leading-! dotr)
            (r"[.!]", Operator),       # dotted field access / strict marker
            (r"[|:]", Operator),       # barrier marker / typed-name / map key
            (r"/", Operator),          # bare dispatch/path slash

            # -- ordinary words --------------------------------------------
            (r"[A-Za-z][A-Za-z0-9_-]*", Name),

            # -- punctuation / separators ----------------------------------
            (r"[\[\]{}()]", Punctuation),
            (r";", Punctuation),
            (r"[@$%,]", Punctuation),

            # -- anything else (be permissive, never fail) -----------------
            (r".", Text),
        ],

        # /* ... */ block comment (NOT nestable).
        "comment": [
            (r"[^*/]+", Comment.Multiline),
            (r"\*/", Comment.Multiline, "#pop"),
            (r"[*/]", Comment.Multiline),
        ],

        # 'single-quoted' string.
        "sstring": [
            (r"\\[nt\\'\"`]", String.Escape),
            (r"\\u[0-9a-fA-F]{4}", String.Escape),
            (r"\\.", String.Escape),
            (r"[^\\']+", String.Single),
            (r"'", String.Single, "#pop"),
        ],

        # "double-quoted" string.
        "dstring": [
            (r"\\[nt\\'\"`]", String.Escape),
            (r"\\u[0-9a-fA-F]{4}", String.Escape),
            (r"\\.", String.Escape),
            (r'[^\\"]+', String.Double),
            (r'"', String.Double, "#pop"),
        ],

        # `backtick template ${expr} string` — the ${ … } holds an embedded
        # AQL expression (nestable).
        "template": [
            (r"\\[nt\\'\"`]", String.Escape),
            (r"\\u[0-9a-fA-F]{4}", String.Escape),
            (r"\\.", String.Escape),
            (r"\$\{", String.Interpol, "interp"),
            (r"[^\\`$]+", String.Backtick),
            (r"\$", String.Backtick),
            (r"`", String.Backtick, "#pop"),
        ],

        # ${ … } interpolation: highlight the embedded expression with the
        # full root grammar, tracking nested braces so the ${...} scope ends
        # at its matching close brace.  The close ``}`` that matches the
        # opening ``${`` pops us back into the ``template`` string.
        "interp": [
            (r"\}", String.Interpol, "#pop"),
            (r"\{", Punctuation, "interp-brace"),
            include("root"),
        ],

        # A `{ … }` map/quotation nested inside an interpolation — its close
        # brace is punctuation, not the end of the interpolation scope.
        "interp-brace": [
            (r"\}", Punctuation, "#pop"),
            (r"\{", Punctuation, "#push"),
            include("root"),
        ],
    }
