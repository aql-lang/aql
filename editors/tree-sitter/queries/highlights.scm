; highlights.scm — syntax highlighting for BORU (tree-sitter-boru).
;
; Standard capture names (nvim-treesitter / Helix / Zed compatible):
;   @comment @string @string.escape @number @boolean @constant.builtin
;   @keyword @function.builtin @function @type @operator
;   @punctuation.bracket @punctuation.delimiter @variable
;
; BORU is concatenative, so a bare "word" node can be a keyword, a builtin, or a
; user word. We discriminate purely on the token text via #any-of? predicates
; whose lists are the authoritative keyword / builtin vocabularies.

; ---------------------------------------------------------------------------
; Comments.
; ---------------------------------------------------------------------------
(line_comment) @comment
(block_comment) @comment

; ---------------------------------------------------------------------------
; Strings (single, double, backtick template) + escapes + interpolation.
; ---------------------------------------------------------------------------
(string) @string
(template_string) @string
(escape_sequence) @string.escape

; The ${ … } interpolation delimiters read as punctuation; their contents are
; highlighted by the ordinary expression rules (they are real expressions).
(interpolation_start) @punctuation.special
(interpolation_end) @punctuation.special

; ---------------------------------------------------------------------------
; Numbers and literal constants.
; ---------------------------------------------------------------------------
(number) @number
(boolean) @boolean
(none) @constant.builtin
(float_const) @constant.builtin

; ---------------------------------------------------------------------------
; Type names / module namespaces — any Capitalised identifier.
; ---------------------------------------------------------------------------
(type_identifier) @type

; ---------------------------------------------------------------------------
; Keywords — control / declaration / query-DSL words.
; ---------------------------------------------------------------------------
((word) @keyword
 (#any-of? @keyword
  "def" "undef" "var" "fn" "afn" "class" "surface" "refine" "gen"
  "exposes" "make" "import" "export" "module" "load" "create" "update"
  "remove" "do" "if" "case" "for" "for-each" "each" "fold" "scan"
  "outer" "inner" "filter" "walk" "break" "continue" "guard" "raise"
  "return" "otherwise" "quote" "unquote" "splice" "word" "macro"
  "macroexpand" "codequote" "unpack" "unpack-prefix" "call" "apply"
  "from" "where" "select" "order" "group" "having" "join" "limit"
  "offset" "distinct" "union" "between" "in" "with" "with-decimal"
  "convert" "typeof" "inspect" "is" "istype" "of" "extends" "default"
  "const" "base" "behave" "pathof"))

; ---------------------------------------------------------------------------
; Builtins — library words.
; ---------------------------------------------------------------------------
((word) @function.builtin
 (#any-of? @function.builtin
  ; math
  "add" "sub" "mul" "div" "mod" "pow" "abs" "negate" "min" "max" "sign"
  "ceil" "floor" "round" "trunc" "round-even" "logb" "scalb" "fma"
  "sqrt" "cbrt" "exp" "log" "log2" "log10" "sin" "cos" "tan" "asin"
  "acos" "atan" "atan2" "hypot" "is-nan" "is-inf" "is-finite" "signbit"
  "remainder" "copysign" "nextafter" "math-pi" "math-e"
  ; compare
  "lt" "gt" "lte" "gte" "cmp" "tcmp" "eq" "neq" "deq"
  ; boolean
  "and" "or" "not" "xor" "nand" "implies" "nor" "iff" "xnor" "any" "all"
  ; binary
  "band" "bor" "bxor" "bnot" "bsl" "bsr" "busr"
  ; string
  "upper" "lower" "concat" "split" "trim" "contains" "indexof" "replace"
  "changecase" "normalize" "repeat" "pad" "match" "escape"
  ; stack
  "dup" "swap" "drop" "over" "rot" "nip" "tuck" "dup2" "swap2" "drop2"
  "over2" "depth" "pick" "roll" "stack"
  ; list
  "iota" "range" "reverse" "sort" "flatten" "slice" "take" "shed"
  "append" "push" "pop" "shift" "unshift" "size" "enum" "node" "flex"
  ; xml
  "xml-elem" "xml-text" "xml-attr"
  ; storage / access
  "set" "get" "getr" "dot" "dotr" "has" "context" "keys" "vals" "reach"
  "rebind" "ref" "referent" "patrun" "find" "patterns"
  ; type algebra
  "tor" "tand" "tany" "tall" "teq" "tpartial" "tis" "tnot" "fnsig" "unify"
  ; macro / meta
  "gensym" "canon" "mini" "parse" "emit"
  ; io
  "print" "printstr" "read" "write" "trace" "stdin" "stdout" "stderr"
  ; control helpers
  "args" "error" "force-arity" "usurp" "forward-args" "stack-args"
  ; concurrency
  "spawn" "self" "send" "receive" "register" "whereis" "unregister"
  "service" "state-of" "wrap"
  ; help
  "help" "describe"))

; ---------------------------------------------------------------------------
; Everything else that is a bare word is a user-defined value/function name.
; This rule sits AFTER the keyword/builtin rules so those win (later queries do
; not override earlier matches in tree-sitter; the specific lists are matched
; first, and this generic capture only applies to words the predicates missed).
; ---------------------------------------------------------------------------
(word) @variable

; ---------------------------------------------------------------------------
; The /modifier dispatch suffix (/q /s /f /r /t /u, /<digit>, combos).
; ---------------------------------------------------------------------------
(modifier) @attribute

; ---------------------------------------------------------------------------
; Operators & punctuation.
; ---------------------------------------------------------------------------
(operator) @operator
(punctuation) @punctuation.delimiter

[
  "("
  ")"
  "["
  "]"
  "{"
  "}"
] @punctuation.bracket

; The template-string backtick and string quotes are part of their (string)
; nodes, so no extra rule is needed for them.
