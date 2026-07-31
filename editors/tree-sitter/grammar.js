/**
 * @file Tree-sitter grammar for boru — a concatenative, strongly-typed query language.
 * @author boru contributors
 * @license MIT
 *
 * boru is CONCATENATIVE. This grammar is deliberately PRAGMATIC: it models the
 * token / lexeme layer faithfully (comments, the three string kinds with `${}`
 * interpolation, every number form, words, capitalised type identifiers,
 * operators / brackets and the `/modifier` suffixes) and then treats a program
 * as a flat sequence of expressions. It does NOT attempt full precedence
 * parsing — the goal is a robust token tree that never ERRORs on real files.
 *
 * Lexical spec source: the repository's authoritative lexical spec, cross-checked
 * against editors/emacs/boru-mode.el and the example programs under
 * design/examples/, lang/go/test/solardemo.boru and bench/networking/.
 */

/* eslint-disable arrow-parens */
/* eslint-disable camelcase */
/* eslint-disable-next-line spaced-comment */
/// <reference types="tree-sitter-cli/dsl" />
// @ts-check

// A word: [A-Za-z][A-Za-z0-9_-]* — letters, digits, internal hyphen/underscore.
// Hyphens are INTERNAL (math-pi, for-each, round-even); a trailing hyphen is not
// part of the word. This regex only matches a hyphen when a word char follows.
const WORD = /[A-Za-z](?:[A-Za-z0-9_]|-[A-Za-z0-9_])*/;

// A capitalised identifier — type name or module namespace ([A-Z][A-Za-z0-9]*).
// Deliberately NARROWER than WORD (no hyphen/underscore) to mirror the type rule.
const TYPE_IDENT = /[A-Z][A-Za-z0-9]*/;

module.exports = grammar({
  name: 'boru',

  // Whitespace and all comment forms float between tokens.
  extras: $ => [
    /\s/,
    $.line_comment,
    $.block_comment,
  ],

  // The scanner needs no external tokens; everything is handled by the JS lexer.
  externals: $ => [],

  word: $ => $._word_token,

  // The interpolation `${ … }` inside a template string embeds a full boru
  // expression list; keep it in the same rule set.
  rules: {
    // ---------------------------------------------------------------------
    // Program: a flat sequence of expressions/atoms/brackets.
    // ---------------------------------------------------------------------
    source_file: $ => repeat($._expression),

    _expression: $ => choice(
      $.line_comment,
      $.block_comment,
      $._literal,
      $.type_identifier,
      $.word,
      $.modifier,
      $.list,
      $.map,
      $.group,
      $.operator,
      $.punctuation,
    ),

    _literal: $ => choice(
      $.string,
      $.template_string,
      $.number,
      $.boolean,
      $.none,
      $.float_const,
    ),

    // ---------------------------------------------------------------------
    // Comments — line (# … and // …) and block (/* … */, not nestable).
    // token(prec(…)) so comments win against the `/` operator and `#`/`//`.
    // ---------------------------------------------------------------------
    line_comment: _ => token(prec(2, choice(
      seq('#', /[^\n]*/),
      seq('//', /[^\n]*/),
    ))),

    block_comment: _ => token(prec(2, seq(
      '/*',
      /[^*]*\*+(?:[^/*][^*]*\*+)*/,
      '/',
    ))),

    // ---------------------------------------------------------------------
    // Strings — single, double and backtick template. All support the
    // backslash escapes  \n \t \\ \' \" \`  and \uXXXX.
    // ---------------------------------------------------------------------
    // Escape: the spec lists  \n \t \\ \' \" \`  and \uXXXX — but real code
    // also uses \r etc., so any \<char> is accepted as an escape (an unknown
    // escape is still visually an escape and must never ERROR).
    escape_sequence: _ => token.immediate(prec(10, seq(
      '\\',
      choice(
        /u[0-9a-fA-F]{4}/,
        /[^u]/,
        /u/,
      ),
    ))),

    string: $ => choice(
      $._single_string,
      $._double_string,
    ),

    // The content immediate tokens carry a precedence ABOVE the comment tokens
    // (prec 2) so that `#`, `//` and `/*` sequences INSIDE a string are treated
    // as string content, not as the start of a comment extra.
    _single_string: $ => seq(
      "'",
      repeat(choice(
        token.immediate(prec(5, /[^'\\]+/)),
        $.escape_sequence,
      )),
      "'",
    ),

    _double_string: $ => seq(
      '"',
      repeat(choice(
        token.immediate(prec(5, /[^"\\]+/)),
        $.escape_sequence,
      )),
      '"',
    ),

    // Backtick template string with `${ … }` interpolation (nestable — the
    // interpolation body is a full expression list that can contain another
    // template string).
    template_string: $ => seq(
      '`',
      repeat(choice(
        $._template_chars,
        $.escape_sequence,
        $.interpolation,
      )),
      '`',
    ),

    // Literal run inside a template: anything that is not a backtick, a
    // backslash, or the start of an interpolation `${`.
    // A lone `$` that is not followed by `{` is literal text; it is matched
    // as a single-char token (lower immediate prec than the `${` start).
    _template_chars: _ => token.immediate(prec(5, choice(
      /[^`\\$]+/,
      /\$[^{`\\]/,
      '$',
    ))),

    interpolation: $ => seq(
      $.interpolation_start,
      repeat($._expression),
      $.interpolation_end,
    ),

    // `${` must out-rank the bare `$` fallback in `_template_chars` (both are
    // token.immediate; higher prec wins the longest-match tie).
    interpolation_start: _ => token.immediate(prec(6, '${')),
    interpolation_end: _ => '}',

    // ---------------------------------------------------------------------
    // Numbers — an optional leading '-' may precede any form.
    // token(prec(…)) so a numeric literal wins over the `-` operator and the
    // word rule.
    // ---------------------------------------------------------------------
    number: $ => choice(
      $.hex,
      $.binary,
      $.big_number,
      $.float,
      $.integer,
    ),

    // Hexadecimal: 0x[0-9a-fA-F_]+
    hex: _ => token(prec(3, /-?0x[0-9a-fA-F_]+/)),

    // Binary: 0b[01_]+
    binary: _ => token(prec(3, /-?0b[01_]+/)),

    // Big integer (arbitrary precision): 0d[0-9]+
    big_number: _ => token(prec(3, /-?0d[0-9]+/)),

    // Decimal float:
    //   [0-9][0-9_]*\.[0-9_]+([eE][+-]?[0-9]+)?
    //   [0-9][0-9_]*[eE][+-]?[0-9]+
    float: _ => token(prec(3, choice(
      /-?[0-9][0-9_]*\.[0-9_]+(?:[eE][+-]?[0-9]+)?/,
      /-?[0-9][0-9_]*[eE][+-]?[0-9]+/,
    ))),

    // Decimal integer: [0-9][0-9_]*
    integer: _ => token(prec(2, /-?[0-9][0-9_]*/)),

    // ---------------------------------------------------------------------
    // Constant words: true false none  and the float constants inf nan
    // (also -inf, -nan). Treated as language constants.
    // ---------------------------------------------------------------------
    boolean: _ => token(prec(4, choice('true', 'false'))),

    none: _ => token(prec(4, 'none')),

    float_const: _ => token(prec(4, choice(
      'inf', 'nan', '-inf', '-nan',
    ))),

    // ---------------------------------------------------------------------
    // Words and type identifiers.
    // ---------------------------------------------------------------------
    // The lexer `word` token — used by tree-sitter for keyword extraction.
    _word_token: _ => WORD,

    // A plain word (built-in, keyword or user word). The highlight query
    // discriminates keyword / builtin / plain via #match?/#any-of? on the text.
    word: $ => $._word_token,

    // Capitalised identifier — a type name or module namespace. Higher prec
    // than `word` so `Integer`, `MathUtil`, `Emailon` classify as types.
    type_identifier: _ => token(prec(1, TYPE_IDENT)),

    // ---------------------------------------------------------------------
    // Bracket groups. Flat contents — a sequence of expressions.
    //   [ ] lists & quotations   { } maps   ( ) grouping / word-context
    // ---------------------------------------------------------------------
    list: $ => seq('[', repeat($._expression), ']'),
    map: $ => seq('{', repeat($._expression), '}'),
    group: $ => seq('(', repeat($._expression), ')'),

    // ---------------------------------------------------------------------
    // The /modifier dispatch suffix: /q /s /f /r /t /u, a force-arity digit
    // run /<digit>, and any combination (/sq, /qs, /2, …). Immediately follows
    // a word or a closing group; token(prec) keeps it distinct from the `/`
    // div operator and from a line/block comment start.
    // ---------------------------------------------------------------------
    modifier: _ => token(prec(3, /\/(?:[qsfrtu]+[0-9]*|[0-9]+[qsfrtu]*)/)),

    // ---------------------------------------------------------------------
    // Operators & punctuation.
    //   =>  lambda arrow           !. / !  strict access / strict prefix
    //   .   dotted field access    :  typed / namespace / map-key
    //   |   fn-signature barrier   /  dispatch / div (bare slash)
    //   @   used in field names (req.@user, set @user …) — kept as operator
    //   ?   optional / predicate-suffix marker (none?, opts ? default)
    // ---------------------------------------------------------------------
    operator: _ => token(prec(1, choice(
      '=>',   // lambda arrow
      '!.',   // strict dotted access
      '.',    // dotted field access
      ':',    // typed param / namespace / map key
      '|',    // signature barrier
      '!',    // strict prefix
      '@',    // field-name sigil (req.@user)
      '?',    // optional / predicate marker
      '/',    // bare dispatch / division (a plain slash, not a /modifier run)
    ))),

    // Statement separator `;` (aliases to `end`).
    punctuation: _ => token(prec(1, ';')),
  },
});
