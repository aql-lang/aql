;;; aql-mode.el --- Major mode for the AQL query language  -*- lexical-binding: t; -*-

;; Author: AQL contributors
;; Version: 0.1.0
;; Package-Requires: ((emacs "26.1"))
;; Keywords: languages, aql
;; URL: https://github.com/aql-lang/aql

;; This file is not part of GNU Emacs.

;;; Commentary:

;; A major mode for editing AQL (https://github.com/aql-lang/aql) source
;; files.  AQL is a concatenative, strongly-typed query language.  This
;; mode provides:
;;
;;   * syntax highlighting for words, types, literals and comments,
;;   * bracket-aware indentation,
;;   * comment/uncomment and `imenu' support,
;;   * one-line integration with the bundled `aql lsp' server for both
;;     `lsp-mode' and `eglot' (diagnostics, hover, completion, format).
;;
;; Quick start — drop aql-mode.el on your `load-path' and add:
;;
;;   (require 'aql-mode)
;;   ;; pick ONE language client (optional but recommended):
;;   (add-hook 'aql-mode-hook #'eglot-ensure)          ; Emacs 29+ built-in
;;   ;; ...or with lsp-mode:
;;   ;; (add-hook 'aql-mode-hook #'lsp-deferred)
;;
;; The mode registers `aql lsp' with whichever client is installed; the
;; server is discovered on `exec-path' (customise `aql-lsp-command' to
;; point at a non-PATH binary).  `.aql' files select the mode
;; automatically.

;;; Code:

(require 'imenu)

(defgroup aql nil
  "Major mode for editing AQL source."
  :group 'languages
  :prefix "aql-")

(defcustom aql-indent-offset 2
  "Number of columns to indent per bracket nesting level in `aql-mode'."
  :type 'integer
  :safe #'integerp
  :group 'aql)

(defcustom aql-lsp-command '("aql" "lsp")
  "Command (program and arguments) that starts the AQL language server.
The default runs the `aql lsp' stdio server found on `exec-path'.
Point the first element at an absolute path if `aql' is not on PATH."
  :type '(repeat string)
  :group 'aql)

;;;; Vocabulary -------------------------------------------------------------

;; Structural / control / declaration / query-DSL words.  Highlighted as
;; keywords.  Kept in sync with `aql describe' categories.
(defconst aql-keywords
  '("def" "undef" "var" "fn" "afn" "class" "surface" "refine" "gen"
    "exposes" "make" "import" "export" "module" "load" "create" "update"
    "remove" "do" "if" "case" "for" "for-each" "each" "fold" "scan"
    "outer" "inner" "filter" "walk" "break" "continue" "guard" "raise"
    "return" "otherwise" "quote" "unquote" "splice" "word" "macro"
    "macroexpand" "codequote" "unpack" "unpack-prefix" "call" "apply"
    "from" "where" "select" "order" "group" "having" "join" "limit"
    "offset" "distinct" "union" "between" "in" "with" "with-decimal"
    "convert" "typeof" "inspect" "is" "istype" "of" "extends" "default"
    "const" "base" "behave" "pathof" "exposes")
  "AQL structural and control keywords.")

;; Library / built-in words.  Highlighted as builtins.
(defconst aql-builtins
  '(;; math
    "add" "sub" "mul" "div" "mod" "pow" "abs" "negate" "min" "max" "sign"
    "ceil" "floor" "round" "trunc" "round-even" "logb" "scalb" "fma"
    "sqrt" "cbrt" "exp" "log" "log2" "log10" "sin" "cos" "tan" "asin"
    "acos" "atan" "atan2" "hypot" "is-nan" "is-inf" "is-finite" "signbit"
    "remainder" "copysign" "nextafter" "math-pi" "math-e"
    ;; compare
    "lt" "gt" "lte" "gte" "cmp" "tcmp" "eq" "neq" "deq"
    ;; boolean
    "and" "or" "not" "xor" "nand" "implies" "nor" "iff" "xnor" "any" "all"
    ;; binary
    "band" "bor" "bxor" "bnot" "bsl" "bsr" "busr"
    ;; string
    "upper" "lower" "concat" "split" "trim" "contains" "indexof" "replace"
    "changecase" "normalize" "repeat" "pad" "match" "escape"
    ;; stack
    "dup" "swap" "drop" "over" "rot" "nip" "tuck" "dup2" "swap2" "drop2"
    "over2" "depth" "pick" "roll" "stack"
    ;; list
    "iota" "range" "reverse" "sort" "flatten" "slice" "take" "shed"
    "append" "push" "pop" "shift" "unshift" "size" "enum" "node" "flex"
    ;; xml
    "xml-elem" "xml-text" "xml-attr"
    ;; storage / access
    "set" "get" "getr" "dot" "dotr" "has" "context" "keys" "vals" "reach"
    "rebind" "ref" "referent" "patrun" "find" "patterns"
    ;; type algebra
    "tor" "tand" "tany" "tall" "teq" "tpartial" "tis" "tnot" "fnsig"
    "unify"
    ;; macro / meta
    "gensym" "canon" "mini" "parse" "emit"
    ;; io
    "print" "printstr" "read" "write" "trace" "stdin" "stdout" "stderr"
    ;; control helpers
    "args" "error" "force-arity" "usurp" "forward-args" "stack-args"
    ;; concurrency
    "spawn" "self" "send" "receive" "register" "whereis" "unregister"
    "service" "state-of" "wrap"
    ;; help
    "help" "describe")
  "AQL built-in library words.")

(defconst aql-constants
  '("true" "false" "none" "inf" "nan")
  "AQL literal constants.")

;;;; Syntax table -----------------------------------------------------------

(defvar aql-mode-syntax-table
  (let ((table (make-syntax-table)))
    ;; Comments: `#' starts a line comment (the dominant style); `/* … */'
    ;; is a block comment (`/' is comment-start-1 / comment-end-2, `*' is
    ;; comment-start-2 / comment-end-1).  Newline ends the line comment.
    ;; (`//' line comments are also valid AQL but are left un-fontified —
    ;; a second line-comment style collides with the block-comment `/'.)
    (modify-syntax-entry ?# "<" table)
    (modify-syntax-entry ?\n ">" table)
    (modify-syntax-entry ?/ ". 14" table)
    (modify-syntax-entry ?* ". 23" table)
    ;; Strings: single-quote, double-quote and backtick template strings.
    (modify-syntax-entry ?\' "\"" table)
    (modify-syntax-entry ?\" "\"" table)
    (modify-syntax-entry ?\` "\"" table)
    ;; Words may contain hyphens and underscores (math-pi, is-nan, foo_bar).
    (modify-syntax-entry ?- "_" table)
    (modify-syntax-entry ?_ "_" table)
    ;; Brackets pair up for `show-paren-mode', `forward-sexp', indentation.
    (modify-syntax-entry ?\( "()" table)
    (modify-syntax-entry ?\) ")(" table)
    (modify-syntax-entry ?\[ "(]" table)
    (modify-syntax-entry ?\] ")[" table)
    (modify-syntax-entry ?\{ "(}" table)
    (modify-syntax-entry ?\} "){" table)
    ;; Everyday operators / separators are punctuation (`/' and `*' are set
    ;; above for the block-comment syntax).
    (dolist (ch '(?. ?\; ?: ?| ?! ?, ?@ ?$ ?%))
      (modify-syntax-entry ch "." table))
    table)
  "Syntax table for `aql-mode'.")

;;;; Font lock ---------------------------------------------------------------

(defconst aql-font-lock-keywords
  (let ((kw (regexp-opt aql-keywords 'symbols))
        (bi (regexp-opt aql-builtins 'symbols))
        (co (regexp-opt aql-constants 'symbols)))
    `(;; `def Foo …' / `def foo …' — the binding target.  A capitalised
      ;; target names a type; a lowercase one names a value/fn.
      ("\\_<def\\_>[ \t]+\\(\\(?:[A-Z][A-Za-z0-9_-]*\\)\\)"
       (1 font-lock-type-face))
      ("\\_<\\(?:def\\|undef\\|var\\|fn\\)\\_>[ \t]+\\([a-z_][A-Za-z0-9_-]*\\)"
       (1 font-lock-function-name-face))
      ;; Structural / control keywords.
      (,kw . font-lock-keyword-face)
      ;; Literal constants.
      (,co . font-lock-constant-face)
      ;; Built-in library words.
      (,bi . font-lock-builtin-face)
      ;; Numbers: integers, decimals, exponents, and 0d big integers.
      ("\\_<\\(?:0d[0-9]+\\|[0-9][0-9_]*\\(?:\\.[0-9]+\\)?\\(?:[eE][+-]?[0-9]+\\)?\\)\\_>"
       . font-lock-constant-face)
      ;; Type names / module namespaces: capitalised identifiers
      ;; (Integer, Emailon, MathUtil, user types, …).
      ("\\_<[A-Z][A-Za-z0-9]*\\_>" . font-lock-type-face)
      ;; Path/dispatch modifiers: foo/q, add/s, X/t, name/f, /N arity.
      ("/\\(?:[qsfrtuN]+\\|[0-9]+\\)\\_>" . font-lock-preprocessor-face)))
  "Font-lock rules for `aql-mode'.")

;;;; Indentation -------------------------------------------------------------

(defun aql--calculate-indent ()
  "Return the target indentation column for the current line, or nil."
  (save-excursion
    (beginning-of-line)
    (let* ((ppss (syntax-ppss))
           (open (nth 1 ppss)))          ; innermost containing open bracket
      (cond
       ((nth 3 ppss) nil)               ; inside a string — leave alone
       ((null open) 0)                  ; top level
       (t
        (let ((closes-here (looking-at-p "[ \t]*[])}]")))
          (goto-char open)
          (+ (current-indentation)
             (if closes-here 0 aql-indent-offset))))))))

(defun aql-indent-line ()
  "Indent the current line for `aql-mode' by bracket nesting."
  (interactive)
  (let ((target (aql--calculate-indent)))
    (when target
      (if (<= (current-column) (current-indentation))
          (indent-line-to target)
        (save-excursion (indent-line-to target))))))

;;;; imenu -------------------------------------------------------------------

(defconst aql-imenu-generic-expression
  '(("Types"   "\\_<def\\_>[ \t]+\\([A-Z][A-Za-z0-9_-]*\\)" 1)
    ("Defs"    "\\_<def\\_>[ \t]+\\([a-z_][A-Za-z0-9_-]*\\)" 1)
    ("Exports" "\\_<export\\_>[ \t]+\"\\([^\"]+\\)\"" 1))
  "`imenu-generic-expression' for `aql-mode'.")

;;;; Language-server registration -------------------------------------------

(with-eval-after-load 'lsp-mode
  (add-to-list 'lsp-language-id-configuration '(aql-mode . "aql"))
  (lsp-register-client
   (make-lsp-client
    :new-connection (lsp-stdio-connection (lambda () aql-lsp-command))
    :activation-fn (lsp-activate-on "aql")
    :server-id 'aql-lsp)))

(with-eval-after-load 'eglot
  ;; A function contact is re-evaluated per session, so a customised
  ;; `aql-lsp-command' is always honoured.
  (add-to-list 'eglot-server-programs
               (cons 'aql-mode (lambda (&rest _) aql-lsp-command))))

;;;; Mode definition ---------------------------------------------------------

;;;###autoload
(define-derived-mode aql-mode prog-mode "AQL"
  "Major mode for editing AQL source files.

\\{aql-mode-map}"
  :syntax-table aql-mode-syntax-table
  (setq-local comment-start "# ")
  (setq-local comment-start-skip "#+[ \t]*")
  (setq-local comment-end "")
  (setq-local font-lock-defaults '(aql-font-lock-keywords))
  (setq-local indent-line-function #'aql-indent-line)
  (setq-local electric-indent-chars (append "()[]{}" electric-indent-chars))
  (setq-local imenu-generic-expression aql-imenu-generic-expression)
  (imenu-add-to-menubar "AQL"))

;;;###autoload
(add-to-list 'auto-mode-alist '("\\.aql\\'" . aql-mode))

(provide 'aql-mode)

;;; aql-mode.el ends here
