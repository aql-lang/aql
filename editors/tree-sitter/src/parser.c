#include "tree_sitter/parser.h"

#if defined(__GNUC__) || defined(__clang__)
#pragma GCC diagnostic ignored "-Wmissing-field-initializers"
#endif

#define LANGUAGE_VERSION 14
#define STATE_COUNT 68
#define LARGE_STATE_COUNT 19
#define SYMBOL_COUNT 48
#define ALIAS_COUNT 0
#define TOKEN_COUNT 30
#define EXTERNAL_TOKEN_COUNT 0
#define FIELD_COUNT 0
#define MAX_ALIAS_SEQUENCE_LENGTH 3
#define PRODUCTION_ID_COUNT 1

enum ts_symbol_identifiers {
  sym__word_token = 1,
  sym_line_comment = 2,
  sym_block_comment = 3,
  sym_escape_sequence = 4,
  anon_sym_SQUOTE = 5,
  aux_sym__single_string_token1 = 6,
  anon_sym_DQUOTE = 7,
  aux_sym__double_string_token1 = 8,
  anon_sym_BQUOTE = 9,
  sym__template_chars = 10,
  sym_interpolation_start = 11,
  anon_sym_RBRACE = 12,
  sym_hex = 13,
  sym_binary = 14,
  sym_big_number = 15,
  sym_float = 16,
  sym_integer = 17,
  sym_boolean = 18,
  sym_none = 19,
  sym_float_const = 20,
  sym_type_identifier = 21,
  anon_sym_LBRACK = 22,
  anon_sym_RBRACK = 23,
  anon_sym_LBRACE = 24,
  anon_sym_LPAREN = 25,
  anon_sym_RPAREN = 26,
  sym_modifier = 27,
  sym_operator = 28,
  sym_punctuation = 29,
  sym_source_file = 30,
  sym__expression = 31,
  sym__literal = 32,
  sym_string = 33,
  sym__single_string = 34,
  sym__double_string = 35,
  sym_template_string = 36,
  sym_interpolation = 37,
  sym_interpolation_end = 38,
  sym_number = 39,
  sym_word = 40,
  sym_list = 41,
  sym_map = 42,
  sym_group = 43,
  aux_sym_source_file_repeat1 = 44,
  aux_sym__single_string_repeat1 = 45,
  aux_sym__double_string_repeat1 = 46,
  aux_sym_template_string_repeat1 = 47,
};

static const char * const ts_symbol_names[] = {
  [ts_builtin_sym_end] = "end",
  [sym__word_token] = "_word_token",
  [sym_line_comment] = "line_comment",
  [sym_block_comment] = "block_comment",
  [sym_escape_sequence] = "escape_sequence",
  [anon_sym_SQUOTE] = "'",
  [aux_sym__single_string_token1] = "_single_string_token1",
  [anon_sym_DQUOTE] = "\"",
  [aux_sym__double_string_token1] = "_double_string_token1",
  [anon_sym_BQUOTE] = "`",
  [sym__template_chars] = "_template_chars",
  [sym_interpolation_start] = "interpolation_start",
  [anon_sym_RBRACE] = "}",
  [sym_hex] = "hex",
  [sym_binary] = "binary",
  [sym_big_number] = "big_number",
  [sym_float] = "float",
  [sym_integer] = "integer",
  [sym_boolean] = "boolean",
  [sym_none] = "none",
  [sym_float_const] = "float_const",
  [sym_type_identifier] = "type_identifier",
  [anon_sym_LBRACK] = "[",
  [anon_sym_RBRACK] = "]",
  [anon_sym_LBRACE] = "{",
  [anon_sym_LPAREN] = "(",
  [anon_sym_RPAREN] = ")",
  [sym_modifier] = "modifier",
  [sym_operator] = "operator",
  [sym_punctuation] = "punctuation",
  [sym_source_file] = "source_file",
  [sym__expression] = "_expression",
  [sym__literal] = "_literal",
  [sym_string] = "string",
  [sym__single_string] = "_single_string",
  [sym__double_string] = "_double_string",
  [sym_template_string] = "template_string",
  [sym_interpolation] = "interpolation",
  [sym_interpolation_end] = "interpolation_end",
  [sym_number] = "number",
  [sym_word] = "word",
  [sym_list] = "list",
  [sym_map] = "map",
  [sym_group] = "group",
  [aux_sym_source_file_repeat1] = "source_file_repeat1",
  [aux_sym__single_string_repeat1] = "_single_string_repeat1",
  [aux_sym__double_string_repeat1] = "_double_string_repeat1",
  [aux_sym_template_string_repeat1] = "template_string_repeat1",
};

static const TSSymbol ts_symbol_map[] = {
  [ts_builtin_sym_end] = ts_builtin_sym_end,
  [sym__word_token] = sym__word_token,
  [sym_line_comment] = sym_line_comment,
  [sym_block_comment] = sym_block_comment,
  [sym_escape_sequence] = sym_escape_sequence,
  [anon_sym_SQUOTE] = anon_sym_SQUOTE,
  [aux_sym__single_string_token1] = aux_sym__single_string_token1,
  [anon_sym_DQUOTE] = anon_sym_DQUOTE,
  [aux_sym__double_string_token1] = aux_sym__double_string_token1,
  [anon_sym_BQUOTE] = anon_sym_BQUOTE,
  [sym__template_chars] = sym__template_chars,
  [sym_interpolation_start] = sym_interpolation_start,
  [anon_sym_RBRACE] = anon_sym_RBRACE,
  [sym_hex] = sym_hex,
  [sym_binary] = sym_binary,
  [sym_big_number] = sym_big_number,
  [sym_float] = sym_float,
  [sym_integer] = sym_integer,
  [sym_boolean] = sym_boolean,
  [sym_none] = sym_none,
  [sym_float_const] = sym_float_const,
  [sym_type_identifier] = sym_type_identifier,
  [anon_sym_LBRACK] = anon_sym_LBRACK,
  [anon_sym_RBRACK] = anon_sym_RBRACK,
  [anon_sym_LBRACE] = anon_sym_LBRACE,
  [anon_sym_LPAREN] = anon_sym_LPAREN,
  [anon_sym_RPAREN] = anon_sym_RPAREN,
  [sym_modifier] = sym_modifier,
  [sym_operator] = sym_operator,
  [sym_punctuation] = sym_punctuation,
  [sym_source_file] = sym_source_file,
  [sym__expression] = sym__expression,
  [sym__literal] = sym__literal,
  [sym_string] = sym_string,
  [sym__single_string] = sym__single_string,
  [sym__double_string] = sym__double_string,
  [sym_template_string] = sym_template_string,
  [sym_interpolation] = sym_interpolation,
  [sym_interpolation_end] = sym_interpolation_end,
  [sym_number] = sym_number,
  [sym_word] = sym_word,
  [sym_list] = sym_list,
  [sym_map] = sym_map,
  [sym_group] = sym_group,
  [aux_sym_source_file_repeat1] = aux_sym_source_file_repeat1,
  [aux_sym__single_string_repeat1] = aux_sym__single_string_repeat1,
  [aux_sym__double_string_repeat1] = aux_sym__double_string_repeat1,
  [aux_sym_template_string_repeat1] = aux_sym_template_string_repeat1,
};

static const TSSymbolMetadata ts_symbol_metadata[] = {
  [ts_builtin_sym_end] = {
    .visible = false,
    .named = true,
  },
  [sym__word_token] = {
    .visible = false,
    .named = true,
  },
  [sym_line_comment] = {
    .visible = true,
    .named = true,
  },
  [sym_block_comment] = {
    .visible = true,
    .named = true,
  },
  [sym_escape_sequence] = {
    .visible = true,
    .named = true,
  },
  [anon_sym_SQUOTE] = {
    .visible = true,
    .named = false,
  },
  [aux_sym__single_string_token1] = {
    .visible = false,
    .named = false,
  },
  [anon_sym_DQUOTE] = {
    .visible = true,
    .named = false,
  },
  [aux_sym__double_string_token1] = {
    .visible = false,
    .named = false,
  },
  [anon_sym_BQUOTE] = {
    .visible = true,
    .named = false,
  },
  [sym__template_chars] = {
    .visible = false,
    .named = true,
  },
  [sym_interpolation_start] = {
    .visible = true,
    .named = true,
  },
  [anon_sym_RBRACE] = {
    .visible = true,
    .named = false,
  },
  [sym_hex] = {
    .visible = true,
    .named = true,
  },
  [sym_binary] = {
    .visible = true,
    .named = true,
  },
  [sym_big_number] = {
    .visible = true,
    .named = true,
  },
  [sym_float] = {
    .visible = true,
    .named = true,
  },
  [sym_integer] = {
    .visible = true,
    .named = true,
  },
  [sym_boolean] = {
    .visible = true,
    .named = true,
  },
  [sym_none] = {
    .visible = true,
    .named = true,
  },
  [sym_float_const] = {
    .visible = true,
    .named = true,
  },
  [sym_type_identifier] = {
    .visible = true,
    .named = true,
  },
  [anon_sym_LBRACK] = {
    .visible = true,
    .named = false,
  },
  [anon_sym_RBRACK] = {
    .visible = true,
    .named = false,
  },
  [anon_sym_LBRACE] = {
    .visible = true,
    .named = false,
  },
  [anon_sym_LPAREN] = {
    .visible = true,
    .named = false,
  },
  [anon_sym_RPAREN] = {
    .visible = true,
    .named = false,
  },
  [sym_modifier] = {
    .visible = true,
    .named = true,
  },
  [sym_operator] = {
    .visible = true,
    .named = true,
  },
  [sym_punctuation] = {
    .visible = true,
    .named = true,
  },
  [sym_source_file] = {
    .visible = true,
    .named = true,
  },
  [sym__expression] = {
    .visible = false,
    .named = true,
  },
  [sym__literal] = {
    .visible = false,
    .named = true,
  },
  [sym_string] = {
    .visible = true,
    .named = true,
  },
  [sym__single_string] = {
    .visible = false,
    .named = true,
  },
  [sym__double_string] = {
    .visible = false,
    .named = true,
  },
  [sym_template_string] = {
    .visible = true,
    .named = true,
  },
  [sym_interpolation] = {
    .visible = true,
    .named = true,
  },
  [sym_interpolation_end] = {
    .visible = true,
    .named = true,
  },
  [sym_number] = {
    .visible = true,
    .named = true,
  },
  [sym_word] = {
    .visible = true,
    .named = true,
  },
  [sym_list] = {
    .visible = true,
    .named = true,
  },
  [sym_map] = {
    .visible = true,
    .named = true,
  },
  [sym_group] = {
    .visible = true,
    .named = true,
  },
  [aux_sym_source_file_repeat1] = {
    .visible = false,
    .named = false,
  },
  [aux_sym__single_string_repeat1] = {
    .visible = false,
    .named = false,
  },
  [aux_sym__double_string_repeat1] = {
    .visible = false,
    .named = false,
  },
  [aux_sym_template_string_repeat1] = {
    .visible = false,
    .named = false,
  },
};

static const TSSymbol ts_alias_sequences[PRODUCTION_ID_COUNT][MAX_ALIAS_SEQUENCE_LENGTH] = {
  [0] = {0},
};

static const uint16_t ts_non_terminal_alias_map[] = {
  0,
};

static const TSStateId ts_primary_state_ids[STATE_COUNT] = {
  [0] = 0,
  [1] = 1,
  [2] = 2,
  [3] = 3,
  [4] = 4,
  [5] = 5,
  [6] = 6,
  [7] = 7,
  [8] = 5,
  [9] = 9,
  [10] = 10,
  [11] = 11,
  [12] = 10,
  [13] = 9,
  [14] = 11,
  [15] = 15,
  [16] = 2,
  [17] = 6,
  [18] = 7,
  [19] = 19,
  [20] = 20,
  [21] = 21,
  [22] = 22,
  [23] = 23,
  [24] = 24,
  [25] = 25,
  [26] = 26,
  [27] = 27,
  [28] = 28,
  [29] = 29,
  [30] = 30,
  [31] = 31,
  [32] = 32,
  [33] = 33,
  [34] = 28,
  [35] = 24,
  [36] = 26,
  [37] = 33,
  [38] = 23,
  [39] = 20,
  [40] = 32,
  [41] = 25,
  [42] = 31,
  [43] = 30,
  [44] = 29,
  [45] = 19,
  [46] = 27,
  [47] = 21,
  [48] = 22,
  [49] = 49,
  [50] = 50,
  [51] = 51,
  [52] = 49,
  [53] = 51,
  [54] = 54,
  [55] = 55,
  [56] = 56,
  [57] = 57,
  [58] = 58,
  [59] = 59,
  [60] = 60,
  [61] = 61,
  [62] = 62,
  [63] = 57,
  [64] = 55,
  [65] = 59,
  [66] = 60,
  [67] = 67,
};

static bool ts_lex(TSLexer *lexer, TSStateId state) {
  START_LEXER();
  eof = lexer->eof(lexer);
  switch (state) {
    case 0:
      if (eof) ADVANCE(25);
      ADVANCE_MAP(
        '!', 79,
        '"', 37,
        '#', 26,
        '$', 13,
        '\'', 30,
        '(', 71,
        ')', 72,
        '-', 6,
        '/', 78,
        '0', 60,
        ';', 80,
        '=', 7,
        '[', 68,
        '\\', 12,
        ']', 69,
        '`', 44,
        'i', 65,
        'n', 63,
        '{', 70,
        '}', 54,
        '.', 77,
        ':', 77,
        '?', 77,
        '@', 77,
        '|', 77,
      );
      if (('\t' <= lookahead && lookahead <= '\r') ||
          lookahead == ' ') SKIP(24);
      if (('1' <= lookahead && lookahead <= '9')) ADVANCE(61);
      if (('A' <= lookahead && lookahead <= 'Z') ||
          ('a' <= lookahead && lookahead <= 'z')) ADVANCE(67);
      END_STATE();
    case 1:
      if (lookahead == '"') ADVANCE(37);
      if (lookahead == '#') ADVANCE(38);
      if (lookahead == '/') ADVANCE(40);
      if (lookahead == '\\') ADVANCE(12);
      if (('\t' <= lookahead && lookahead <= '\r') ||
          lookahead == ' ') ADVANCE(39);
      if (lookahead != 0) ADVANCE(43);
      END_STATE();
    case 2:
      if (lookahead == '#') ADVANCE(46);
      if (lookahead == '$') ADVANCE(51);
      if (lookahead == '/') ADVANCE(48);
      if (lookahead == '\\') ADVANCE(12);
      if (lookahead == '`') ADVANCE(44);
      if (('\t' <= lookahead && lookahead <= '\r') ||
          lookahead == ' ') ADVANCE(47);
      if (lookahead != 0) ADVANCE(52);
      END_STATE();
    case 3:
      if (lookahead == '#') ADVANCE(31);
      if (lookahead == '\'') ADVANCE(30);
      if (lookahead == '/') ADVANCE(33);
      if (lookahead == '\\') ADVANCE(12);
      if (('\t' <= lookahead && lookahead <= '\r') ||
          lookahead == ' ') ADVANCE(32);
      if (lookahead != 0) ADVANCE(36);
      END_STATE();
    case 4:
      if (lookahead == '*') ADVANCE(4);
      if (lookahead == '/') ADVANCE(27);
      if (lookahead != 0) ADVANCE(5);
      END_STATE();
    case 5:
      if (lookahead == '*') ADVANCE(4);
      if (lookahead != 0) ADVANCE(5);
      END_STATE();
    case 6:
      if (lookahead == '0') ADVANCE(60);
      if (lookahead == 'i') ADVANCE(10);
      if (lookahead == 'n') ADVANCE(8);
      if (('1' <= lookahead && lookahead <= '9')) ADVANCE(61);
      END_STATE();
    case 7:
      if (lookahead == '>') ADVANCE(77);
      END_STATE();
    case 8:
      if (lookahead == 'a') ADVANCE(11);
      END_STATE();
    case 9:
      if (lookahead == 'f') ADVANCE(62);
      END_STATE();
    case 10:
      if (lookahead == 'n') ADVANCE(9);
      END_STATE();
    case 11:
      if (lookahead == 'n') ADVANCE(62);
      END_STATE();
    case 12:
      if (lookahead == 'u') ADVANCE(29);
      if (lookahead != 0) ADVANCE(28);
      END_STATE();
    case 13:
      if (lookahead == '{') ADVANCE(53);
      END_STATE();
    case 14:
      if (lookahead == '+' ||
          lookahead == '-') ADVANCE(17);
      if (('0' <= lookahead && lookahead <= '9')) ADVANCE(59);
      END_STATE();
    case 15:
      if (lookahead == '0' ||
          lookahead == '1' ||
          lookahead == '_') ADVANCE(56);
      END_STATE();
    case 16:
      if (('0' <= lookahead && lookahead <= '9')) ADVANCE(57);
      END_STATE();
    case 17:
      if (('0' <= lookahead && lookahead <= '9')) ADVANCE(59);
      END_STATE();
    case 18:
      if (('0' <= lookahead && lookahead <= '9') ||
          lookahead == '_') ADVANCE(58);
      END_STATE();
    case 19:
      if (('0' <= lookahead && lookahead <= '9') ||
          ('A' <= lookahead && lookahead <= 'F') ||
          ('a' <= lookahead && lookahead <= 'f')) ADVANCE(28);
      END_STATE();
    case 20:
      if (('0' <= lookahead && lookahead <= '9') ||
          ('A' <= lookahead && lookahead <= 'F') ||
          ('a' <= lookahead && lookahead <= 'f')) ADVANCE(19);
      END_STATE();
    case 21:
      if (('0' <= lookahead && lookahead <= '9') ||
          ('A' <= lookahead && lookahead <= 'F') ||
          ('a' <= lookahead && lookahead <= 'f')) ADVANCE(20);
      END_STATE();
    case 22:
      if (('0' <= lookahead && lookahead <= '9') ||
          ('A' <= lookahead && lookahead <= 'F') ||
          lookahead == '_' ||
          ('a' <= lookahead && lookahead <= 'f')) ADVANCE(55);
      END_STATE();
    case 23:
      if (('0' <= lookahead && lookahead <= '9') ||
          ('A' <= lookahead && lookahead <= 'Z') ||
          lookahead == '_' ||
          ('a' <= lookahead && lookahead <= 'z')) ADVANCE(67);
      END_STATE();
    case 24:
      if (eof) ADVANCE(25);
      ADVANCE_MAP(
        '!', 79,
        '"', 37,
        '#', 26,
        '\'', 30,
        '(', 71,
        ')', 72,
        '-', 6,
        '/', 78,
        '0', 60,
        ';', 80,
        '=', 7,
        '[', 68,
        ']', 69,
        '`', 44,
        'i', 65,
        'n', 63,
        '{', 70,
        '}', 54,
        '.', 77,
        ':', 77,
        '?', 77,
        '@', 77,
        '|', 77,
      );
      if (('\t' <= lookahead && lookahead <= '\r') ||
          lookahead == ' ') SKIP(24);
      if (('1' <= lookahead && lookahead <= '9')) ADVANCE(61);
      if (('A' <= lookahead && lookahead <= 'Z') ||
          ('a' <= lookahead && lookahead <= 'z')) ADVANCE(67);
      END_STATE();
    case 25:
      ACCEPT_TOKEN(ts_builtin_sym_end);
      END_STATE();
    case 26:
      ACCEPT_TOKEN(sym_line_comment);
      if (lookahead != 0 &&
          lookahead != '\n') ADVANCE(26);
      END_STATE();
    case 27:
      ACCEPT_TOKEN(sym_block_comment);
      END_STATE();
    case 28:
      ACCEPT_TOKEN(sym_escape_sequence);
      END_STATE();
    case 29:
      ACCEPT_TOKEN(sym_escape_sequence);
      if (('0' <= lookahead && lookahead <= '9') ||
          ('A' <= lookahead && lookahead <= 'F') ||
          ('a' <= lookahead && lookahead <= 'f')) ADVANCE(21);
      END_STATE();
    case 30:
      ACCEPT_TOKEN(anon_sym_SQUOTE);
      END_STATE();
    case 31:
      ACCEPT_TOKEN(aux_sym__single_string_token1);
      if (lookahead == '\n') ADVANCE(36);
      if (lookahead != 0 &&
          lookahead != '\'' &&
          lookahead != '\\') ADVANCE(31);
      END_STATE();
    case 32:
      ACCEPT_TOKEN(aux_sym__single_string_token1);
      if (lookahead == '#') ADVANCE(31);
      if (lookahead == '/') ADVANCE(33);
      if (('\t' <= lookahead && lookahead <= '\r') ||
          lookahead == ' ') ADVANCE(32);
      if (lookahead != 0 &&
          lookahead != '\'' &&
          lookahead != '\\') ADVANCE(36);
      END_STATE();
    case 33:
      ACCEPT_TOKEN(aux_sym__single_string_token1);
      if (lookahead == '*') ADVANCE(35);
      if (lookahead == '/') ADVANCE(31);
      if (lookahead != 0 &&
          lookahead != '\'' &&
          lookahead != '\\') ADVANCE(36);
      END_STATE();
    case 34:
      ACCEPT_TOKEN(aux_sym__single_string_token1);
      if (lookahead == '*') ADVANCE(34);
      if (lookahead == '/') ADVANCE(36);
      if (lookahead != 0 &&
          lookahead != '\'' &&
          lookahead != '\\') ADVANCE(35);
      END_STATE();
    case 35:
      ACCEPT_TOKEN(aux_sym__single_string_token1);
      if (lookahead == '*') ADVANCE(34);
      if (lookahead != 0 &&
          lookahead != '\'' &&
          lookahead != '\\') ADVANCE(35);
      END_STATE();
    case 36:
      ACCEPT_TOKEN(aux_sym__single_string_token1);
      if (lookahead != 0 &&
          lookahead != '\'' &&
          lookahead != '\\') ADVANCE(36);
      END_STATE();
    case 37:
      ACCEPT_TOKEN(anon_sym_DQUOTE);
      END_STATE();
    case 38:
      ACCEPT_TOKEN(aux_sym__double_string_token1);
      if (lookahead == '\n') ADVANCE(43);
      if (lookahead != 0 &&
          lookahead != '"' &&
          lookahead != '\\') ADVANCE(38);
      END_STATE();
    case 39:
      ACCEPT_TOKEN(aux_sym__double_string_token1);
      if (lookahead == '#') ADVANCE(38);
      if (lookahead == '/') ADVANCE(40);
      if (('\t' <= lookahead && lookahead <= '\r') ||
          lookahead == ' ') ADVANCE(39);
      if (lookahead != 0 &&
          lookahead != '"' &&
          lookahead != '#' &&
          lookahead != '\\') ADVANCE(43);
      END_STATE();
    case 40:
      ACCEPT_TOKEN(aux_sym__double_string_token1);
      if (lookahead == '*') ADVANCE(42);
      if (lookahead == '/') ADVANCE(38);
      if (lookahead != 0 &&
          lookahead != '"' &&
          lookahead != '\\') ADVANCE(43);
      END_STATE();
    case 41:
      ACCEPT_TOKEN(aux_sym__double_string_token1);
      if (lookahead == '*') ADVANCE(41);
      if (lookahead == '/') ADVANCE(43);
      if (lookahead != 0 &&
          lookahead != '"' &&
          lookahead != '\\') ADVANCE(42);
      END_STATE();
    case 42:
      ACCEPT_TOKEN(aux_sym__double_string_token1);
      if (lookahead == '*') ADVANCE(41);
      if (lookahead != 0 &&
          lookahead != '"' &&
          lookahead != '\\') ADVANCE(42);
      END_STATE();
    case 43:
      ACCEPT_TOKEN(aux_sym__double_string_token1);
      if (lookahead != 0 &&
          lookahead != '"' &&
          lookahead != '\\') ADVANCE(43);
      END_STATE();
    case 44:
      ACCEPT_TOKEN(anon_sym_BQUOTE);
      END_STATE();
    case 45:
      ACCEPT_TOKEN(sym__template_chars);
      END_STATE();
    case 46:
      ACCEPT_TOKEN(sym__template_chars);
      if (lookahead == '\n') ADVANCE(52);
      if (lookahead != 0 &&
          lookahead != '$' &&
          lookahead != '\\' &&
          lookahead != '`') ADVANCE(46);
      END_STATE();
    case 47:
      ACCEPT_TOKEN(sym__template_chars);
      if (lookahead == '#') ADVANCE(46);
      if (lookahead == '/') ADVANCE(48);
      if (('\t' <= lookahead && lookahead <= '\r') ||
          lookahead == ' ') ADVANCE(47);
      if (lookahead != 0 &&
          lookahead != '#' &&
          lookahead != '$' &&
          lookahead != '\\' &&
          lookahead != '`') ADVANCE(52);
      END_STATE();
    case 48:
      ACCEPT_TOKEN(sym__template_chars);
      if (lookahead == '*') ADVANCE(50);
      if (lookahead == '/') ADVANCE(46);
      if (lookahead != 0 &&
          lookahead != '$' &&
          lookahead != '\\' &&
          lookahead != '`') ADVANCE(52);
      END_STATE();
    case 49:
      ACCEPT_TOKEN(sym__template_chars);
      if (lookahead == '*') ADVANCE(49);
      if (lookahead == '/') ADVANCE(52);
      if (lookahead != 0 &&
          lookahead != '$' &&
          lookahead != '\\' &&
          lookahead != '`') ADVANCE(50);
      END_STATE();
    case 50:
      ACCEPT_TOKEN(sym__template_chars);
      if (lookahead == '*') ADVANCE(49);
      if (lookahead != 0 &&
          lookahead != '$' &&
          lookahead != '\\' &&
          lookahead != '`') ADVANCE(50);
      END_STATE();
    case 51:
      ACCEPT_TOKEN(sym__template_chars);
      if (lookahead == '{') ADVANCE(53);
      if (lookahead != 0 &&
          lookahead != '\\' &&
          lookahead != '`') ADVANCE(45);
      END_STATE();
    case 52:
      ACCEPT_TOKEN(sym__template_chars);
      if (lookahead != 0 &&
          lookahead != '$' &&
          lookahead != '\\' &&
          lookahead != '`') ADVANCE(52);
      END_STATE();
    case 53:
      ACCEPT_TOKEN(sym_interpolation_start);
      END_STATE();
    case 54:
      ACCEPT_TOKEN(anon_sym_RBRACE);
      END_STATE();
    case 55:
      ACCEPT_TOKEN(sym_hex);
      if (('0' <= lookahead && lookahead <= '9') ||
          ('A' <= lookahead && lookahead <= 'F') ||
          lookahead == '_' ||
          ('a' <= lookahead && lookahead <= 'f')) ADVANCE(55);
      END_STATE();
    case 56:
      ACCEPT_TOKEN(sym_binary);
      if (lookahead == '0' ||
          lookahead == '1' ||
          lookahead == '_') ADVANCE(56);
      END_STATE();
    case 57:
      ACCEPT_TOKEN(sym_big_number);
      if (('0' <= lookahead && lookahead <= '9')) ADVANCE(57);
      END_STATE();
    case 58:
      ACCEPT_TOKEN(sym_float);
      if (lookahead == 'E' ||
          lookahead == 'e') ADVANCE(14);
      if (('0' <= lookahead && lookahead <= '9') ||
          lookahead == '_') ADVANCE(58);
      END_STATE();
    case 59:
      ACCEPT_TOKEN(sym_float);
      if (('0' <= lookahead && lookahead <= '9')) ADVANCE(59);
      END_STATE();
    case 60:
      ACCEPT_TOKEN(sym_integer);
      if (lookahead == '.') ADVANCE(18);
      if (lookahead == 'b') ADVANCE(15);
      if (lookahead == 'd') ADVANCE(16);
      if (lookahead == 'x') ADVANCE(22);
      if (lookahead == 'E' ||
          lookahead == 'e') ADVANCE(14);
      if (('0' <= lookahead && lookahead <= '9') ||
          lookahead == '_') ADVANCE(61);
      END_STATE();
    case 61:
      ACCEPT_TOKEN(sym_integer);
      if (lookahead == '.') ADVANCE(18);
      if (lookahead == 'E' ||
          lookahead == 'e') ADVANCE(14);
      if (('0' <= lookahead && lookahead <= '9') ||
          lookahead == '_') ADVANCE(61);
      END_STATE();
    case 62:
      ACCEPT_TOKEN(sym_float_const);
      END_STATE();
    case 63:
      ACCEPT_TOKEN(sym__word_token);
      if (lookahead == '-') ADVANCE(23);
      if (lookahead == 'a') ADVANCE(66);
      if (('0' <= lookahead && lookahead <= '9') ||
          ('A' <= lookahead && lookahead <= 'Z') ||
          lookahead == '_' ||
          ('b' <= lookahead && lookahead <= 'z')) ADVANCE(67);
      END_STATE();
    case 64:
      ACCEPT_TOKEN(sym__word_token);
      if (lookahead == '-') ADVANCE(23);
      if (lookahead == 'f') ADVANCE(62);
      if (('0' <= lookahead && lookahead <= '9') ||
          ('A' <= lookahead && lookahead <= 'Z') ||
          lookahead == '_' ||
          ('a' <= lookahead && lookahead <= 'z')) ADVANCE(67);
      END_STATE();
    case 65:
      ACCEPT_TOKEN(sym__word_token);
      if (lookahead == '-') ADVANCE(23);
      if (lookahead == 'n') ADVANCE(64);
      if (('0' <= lookahead && lookahead <= '9') ||
          ('A' <= lookahead && lookahead <= 'Z') ||
          lookahead == '_' ||
          ('a' <= lookahead && lookahead <= 'z')) ADVANCE(67);
      END_STATE();
    case 66:
      ACCEPT_TOKEN(sym__word_token);
      if (lookahead == '-') ADVANCE(23);
      if (lookahead == 'n') ADVANCE(62);
      if (('0' <= lookahead && lookahead <= '9') ||
          ('A' <= lookahead && lookahead <= 'Z') ||
          lookahead == '_' ||
          ('a' <= lookahead && lookahead <= 'z')) ADVANCE(67);
      END_STATE();
    case 67:
      ACCEPT_TOKEN(sym__word_token);
      if (lookahead == '-') ADVANCE(23);
      if (('0' <= lookahead && lookahead <= '9') ||
          ('A' <= lookahead && lookahead <= 'Z') ||
          lookahead == '_' ||
          ('a' <= lookahead && lookahead <= 'z')) ADVANCE(67);
      END_STATE();
    case 68:
      ACCEPT_TOKEN(anon_sym_LBRACK);
      END_STATE();
    case 69:
      ACCEPT_TOKEN(anon_sym_RBRACK);
      END_STATE();
    case 70:
      ACCEPT_TOKEN(anon_sym_LBRACE);
      END_STATE();
    case 71:
      ACCEPT_TOKEN(anon_sym_LPAREN);
      END_STATE();
    case 72:
      ACCEPT_TOKEN(anon_sym_RPAREN);
      END_STATE();
    case 73:
      ACCEPT_TOKEN(sym_modifier);
      if (lookahead == 'f' ||
          ('q' <= lookahead && lookahead <= 'u')) ADVANCE(73);
      if (('0' <= lookahead && lookahead <= '9')) ADVANCE(76);
      END_STATE();
    case 74:
      ACCEPT_TOKEN(sym_modifier);
      if (lookahead == 'f' ||
          ('q' <= lookahead && lookahead <= 'u')) ADVANCE(74);
      END_STATE();
    case 75:
      ACCEPT_TOKEN(sym_modifier);
      if (lookahead == 'f' ||
          ('q' <= lookahead && lookahead <= 'u')) ADVANCE(74);
      if (('0' <= lookahead && lookahead <= '9')) ADVANCE(75);
      END_STATE();
    case 76:
      ACCEPT_TOKEN(sym_modifier);
      if (('0' <= lookahead && lookahead <= '9')) ADVANCE(76);
      END_STATE();
    case 77:
      ACCEPT_TOKEN(sym_operator);
      END_STATE();
    case 78:
      ACCEPT_TOKEN(sym_operator);
      if (lookahead == '*') ADVANCE(5);
      if (lookahead == '/') ADVANCE(26);
      if (lookahead == 'f' ||
          ('q' <= lookahead && lookahead <= 'u')) ADVANCE(73);
      if (('0' <= lookahead && lookahead <= '9')) ADVANCE(75);
      END_STATE();
    case 79:
      ACCEPT_TOKEN(sym_operator);
      if (lookahead == '.') ADVANCE(77);
      END_STATE();
    case 80:
      ACCEPT_TOKEN(sym_punctuation);
      END_STATE();
    default:
      return false;
  }
}

static bool ts_lex_keywords(TSLexer *lexer, TSStateId state) {
  START_LEXER();
  eof = lexer->eof(lexer);
  switch (state) {
    case 0:
      if (lookahead == 'f') ADVANCE(1);
      if (lookahead == 'n') ADVANCE(2);
      if (lookahead == 't') ADVANCE(3);
      if (('\t' <= lookahead && lookahead <= '\r') ||
          lookahead == ' ') SKIP(0);
      if (('A' <= lookahead && lookahead <= 'Z')) ADVANCE(4);
      END_STATE();
    case 1:
      if (lookahead == 'a') ADVANCE(5);
      END_STATE();
    case 2:
      if (lookahead == 'o') ADVANCE(6);
      END_STATE();
    case 3:
      if (lookahead == 'r') ADVANCE(7);
      END_STATE();
    case 4:
      ACCEPT_TOKEN(sym_type_identifier);
      if (('0' <= lookahead && lookahead <= '9') ||
          ('A' <= lookahead && lookahead <= 'Z') ||
          ('a' <= lookahead && lookahead <= 'z')) ADVANCE(4);
      END_STATE();
    case 5:
      if (lookahead == 'l') ADVANCE(8);
      END_STATE();
    case 6:
      if (lookahead == 'n') ADVANCE(9);
      END_STATE();
    case 7:
      if (lookahead == 'u') ADVANCE(10);
      END_STATE();
    case 8:
      if (lookahead == 's') ADVANCE(11);
      END_STATE();
    case 9:
      if (lookahead == 'e') ADVANCE(12);
      END_STATE();
    case 10:
      if (lookahead == 'e') ADVANCE(13);
      END_STATE();
    case 11:
      if (lookahead == 'e') ADVANCE(13);
      END_STATE();
    case 12:
      ACCEPT_TOKEN(sym_none);
      END_STATE();
    case 13:
      ACCEPT_TOKEN(sym_boolean);
      END_STATE();
    default:
      return false;
  }
}

static const TSLexMode ts_lex_modes[STATE_COUNT] = {
  [0] = {.lex_state = 0},
  [1] = {.lex_state = 0},
  [2] = {.lex_state = 0},
  [3] = {.lex_state = 0},
  [4] = {.lex_state = 0},
  [5] = {.lex_state = 0},
  [6] = {.lex_state = 0},
  [7] = {.lex_state = 0},
  [8] = {.lex_state = 0},
  [9] = {.lex_state = 0},
  [10] = {.lex_state = 0},
  [11] = {.lex_state = 0},
  [12] = {.lex_state = 0},
  [13] = {.lex_state = 0},
  [14] = {.lex_state = 0},
  [15] = {.lex_state = 0},
  [16] = {.lex_state = 0},
  [17] = {.lex_state = 0},
  [18] = {.lex_state = 0},
  [19] = {.lex_state = 0},
  [20] = {.lex_state = 0},
  [21] = {.lex_state = 0},
  [22] = {.lex_state = 0},
  [23] = {.lex_state = 0},
  [24] = {.lex_state = 0},
  [25] = {.lex_state = 0},
  [26] = {.lex_state = 0},
  [27] = {.lex_state = 0},
  [28] = {.lex_state = 0},
  [29] = {.lex_state = 0},
  [30] = {.lex_state = 0},
  [31] = {.lex_state = 0},
  [32] = {.lex_state = 0},
  [33] = {.lex_state = 0},
  [34] = {.lex_state = 0},
  [35] = {.lex_state = 0},
  [36] = {.lex_state = 0},
  [37] = {.lex_state = 0},
  [38] = {.lex_state = 0},
  [39] = {.lex_state = 0},
  [40] = {.lex_state = 0},
  [41] = {.lex_state = 0},
  [42] = {.lex_state = 0},
  [43] = {.lex_state = 0},
  [44] = {.lex_state = 0},
  [45] = {.lex_state = 0},
  [46] = {.lex_state = 0},
  [47] = {.lex_state = 0},
  [48] = {.lex_state = 0},
  [49] = {.lex_state = 2},
  [50] = {.lex_state = 2},
  [51] = {.lex_state = 2},
  [52] = {.lex_state = 2},
  [53] = {.lex_state = 2},
  [54] = {.lex_state = 3},
  [55] = {.lex_state = 1},
  [56] = {.lex_state = 2},
  [57] = {.lex_state = 3},
  [58] = {.lex_state = 1},
  [59] = {.lex_state = 3},
  [60] = {.lex_state = 1},
  [61] = {.lex_state = 2},
  [62] = {.lex_state = 2},
  [63] = {.lex_state = 3},
  [64] = {.lex_state = 1},
  [65] = {.lex_state = 3},
  [66] = {.lex_state = 1},
  [67] = {.lex_state = 0},
};

static const uint16_t ts_parse_table[LARGE_STATE_COUNT][SYMBOL_COUNT] = {
  [0] = {
    [ts_builtin_sym_end] = ACTIONS(1),
    [sym__word_token] = ACTIONS(1),
    [sym_line_comment] = ACTIONS(3),
    [sym_block_comment] = ACTIONS(3),
    [sym_escape_sequence] = ACTIONS(1),
    [anon_sym_SQUOTE] = ACTIONS(1),
    [anon_sym_DQUOTE] = ACTIONS(1),
    [anon_sym_BQUOTE] = ACTIONS(1),
    [sym_interpolation_start] = ACTIONS(1),
    [anon_sym_RBRACE] = ACTIONS(1),
    [sym_hex] = ACTIONS(1),
    [sym_binary] = ACTIONS(1),
    [sym_big_number] = ACTIONS(1),
    [sym_float] = ACTIONS(1),
    [sym_integer] = ACTIONS(1),
    [sym_boolean] = ACTIONS(1),
    [sym_none] = ACTIONS(1),
    [sym_float_const] = ACTIONS(1),
    [sym_type_identifier] = ACTIONS(1),
    [anon_sym_LBRACK] = ACTIONS(1),
    [anon_sym_RBRACK] = ACTIONS(1),
    [anon_sym_LBRACE] = ACTIONS(1),
    [anon_sym_LPAREN] = ACTIONS(1),
    [anon_sym_RPAREN] = ACTIONS(1),
    [sym_modifier] = ACTIONS(1),
    [sym_operator] = ACTIONS(1),
    [sym_punctuation] = ACTIONS(1),
  },
  [1] = {
    [sym_source_file] = STATE(67),
    [sym__expression] = STATE(15),
    [sym__literal] = STATE(15),
    [sym_string] = STATE(15),
    [sym__single_string] = STATE(42),
    [sym__double_string] = STATE(42),
    [sym_template_string] = STATE(15),
    [sym_number] = STATE(15),
    [sym_word] = STATE(15),
    [sym_list] = STATE(15),
    [sym_map] = STATE(15),
    [sym_group] = STATE(15),
    [aux_sym_source_file_repeat1] = STATE(15),
    [ts_builtin_sym_end] = ACTIONS(5),
    [sym__word_token] = ACTIONS(7),
    [sym_line_comment] = ACTIONS(9),
    [sym_block_comment] = ACTIONS(9),
    [anon_sym_SQUOTE] = ACTIONS(11),
    [anon_sym_DQUOTE] = ACTIONS(13),
    [anon_sym_BQUOTE] = ACTIONS(15),
    [sym_hex] = ACTIONS(17),
    [sym_binary] = ACTIONS(17),
    [sym_big_number] = ACTIONS(17),
    [sym_float] = ACTIONS(17),
    [sym_integer] = ACTIONS(19),
    [sym_boolean] = ACTIONS(9),
    [sym_none] = ACTIONS(9),
    [sym_float_const] = ACTIONS(9),
    [sym_type_identifier] = ACTIONS(9),
    [anon_sym_LBRACK] = ACTIONS(21),
    [anon_sym_LBRACE] = ACTIONS(23),
    [anon_sym_LPAREN] = ACTIONS(25),
    [sym_modifier] = ACTIONS(9),
    [sym_operator] = ACTIONS(27),
    [sym_punctuation] = ACTIONS(9),
  },
  [2] = {
    [sym__expression] = STATE(2),
    [sym__literal] = STATE(2),
    [sym_string] = STATE(2),
    [sym__single_string] = STATE(31),
    [sym__double_string] = STATE(31),
    [sym_template_string] = STATE(2),
    [sym_number] = STATE(2),
    [sym_word] = STATE(2),
    [sym_list] = STATE(2),
    [sym_map] = STATE(2),
    [sym_group] = STATE(2),
    [aux_sym_source_file_repeat1] = STATE(2),
    [sym__word_token] = ACTIONS(29),
    [sym_line_comment] = ACTIONS(32),
    [sym_block_comment] = ACTIONS(32),
    [anon_sym_SQUOTE] = ACTIONS(35),
    [anon_sym_DQUOTE] = ACTIONS(38),
    [anon_sym_BQUOTE] = ACTIONS(41),
    [anon_sym_RBRACE] = ACTIONS(44),
    [sym_hex] = ACTIONS(46),
    [sym_binary] = ACTIONS(46),
    [sym_big_number] = ACTIONS(46),
    [sym_float] = ACTIONS(46),
    [sym_integer] = ACTIONS(49),
    [sym_boolean] = ACTIONS(32),
    [sym_none] = ACTIONS(32),
    [sym_float_const] = ACTIONS(32),
    [sym_type_identifier] = ACTIONS(32),
    [anon_sym_LBRACK] = ACTIONS(52),
    [anon_sym_RBRACK] = ACTIONS(44),
    [anon_sym_LBRACE] = ACTIONS(55),
    [anon_sym_LPAREN] = ACTIONS(58),
    [anon_sym_RPAREN] = ACTIONS(44),
    [sym_modifier] = ACTIONS(32),
    [sym_operator] = ACTIONS(61),
    [sym_punctuation] = ACTIONS(32),
  },
  [3] = {
    [sym__expression] = STATE(2),
    [sym__literal] = STATE(2),
    [sym_string] = STATE(2),
    [sym__single_string] = STATE(31),
    [sym__double_string] = STATE(31),
    [sym_template_string] = STATE(2),
    [sym_interpolation_end] = STATE(56),
    [sym_number] = STATE(2),
    [sym_word] = STATE(2),
    [sym_list] = STATE(2),
    [sym_map] = STATE(2),
    [sym_group] = STATE(2),
    [aux_sym_source_file_repeat1] = STATE(2),
    [sym__word_token] = ACTIONS(64),
    [sym_line_comment] = ACTIONS(66),
    [sym_block_comment] = ACTIONS(66),
    [anon_sym_SQUOTE] = ACTIONS(68),
    [anon_sym_DQUOTE] = ACTIONS(70),
    [anon_sym_BQUOTE] = ACTIONS(72),
    [anon_sym_RBRACE] = ACTIONS(74),
    [sym_hex] = ACTIONS(76),
    [sym_binary] = ACTIONS(76),
    [sym_big_number] = ACTIONS(76),
    [sym_float] = ACTIONS(76),
    [sym_integer] = ACTIONS(78),
    [sym_boolean] = ACTIONS(66),
    [sym_none] = ACTIONS(66),
    [sym_float_const] = ACTIONS(66),
    [sym_type_identifier] = ACTIONS(66),
    [anon_sym_LBRACK] = ACTIONS(80),
    [anon_sym_LBRACE] = ACTIONS(82),
    [anon_sym_LPAREN] = ACTIONS(84),
    [sym_modifier] = ACTIONS(66),
    [sym_operator] = ACTIONS(86),
    [sym_punctuation] = ACTIONS(66),
  },
  [4] = {
    [sym__expression] = STATE(3),
    [sym__literal] = STATE(3),
    [sym_string] = STATE(3),
    [sym__single_string] = STATE(31),
    [sym__double_string] = STATE(31),
    [sym_template_string] = STATE(3),
    [sym_interpolation_end] = STATE(62),
    [sym_number] = STATE(3),
    [sym_word] = STATE(3),
    [sym_list] = STATE(3),
    [sym_map] = STATE(3),
    [sym_group] = STATE(3),
    [aux_sym_source_file_repeat1] = STATE(3),
    [sym__word_token] = ACTIONS(64),
    [sym_line_comment] = ACTIONS(88),
    [sym_block_comment] = ACTIONS(88),
    [anon_sym_SQUOTE] = ACTIONS(68),
    [anon_sym_DQUOTE] = ACTIONS(70),
    [anon_sym_BQUOTE] = ACTIONS(72),
    [anon_sym_RBRACE] = ACTIONS(74),
    [sym_hex] = ACTIONS(76),
    [sym_binary] = ACTIONS(76),
    [sym_big_number] = ACTIONS(76),
    [sym_float] = ACTIONS(76),
    [sym_integer] = ACTIONS(78),
    [sym_boolean] = ACTIONS(88),
    [sym_none] = ACTIONS(88),
    [sym_float_const] = ACTIONS(88),
    [sym_type_identifier] = ACTIONS(88),
    [anon_sym_LBRACK] = ACTIONS(80),
    [anon_sym_LBRACE] = ACTIONS(82),
    [anon_sym_LPAREN] = ACTIONS(84),
    [sym_modifier] = ACTIONS(88),
    [sym_operator] = ACTIONS(90),
    [sym_punctuation] = ACTIONS(88),
  },
  [5] = {
    [sym__expression] = STATE(2),
    [sym__literal] = STATE(2),
    [sym_string] = STATE(2),
    [sym__single_string] = STATE(31),
    [sym__double_string] = STATE(31),
    [sym_template_string] = STATE(2),
    [sym_number] = STATE(2),
    [sym_word] = STATE(2),
    [sym_list] = STATE(2),
    [sym_map] = STATE(2),
    [sym_group] = STATE(2),
    [aux_sym_source_file_repeat1] = STATE(2),
    [sym__word_token] = ACTIONS(64),
    [sym_line_comment] = ACTIONS(66),
    [sym_block_comment] = ACTIONS(66),
    [anon_sym_SQUOTE] = ACTIONS(68),
    [anon_sym_DQUOTE] = ACTIONS(70),
    [anon_sym_BQUOTE] = ACTIONS(72),
    [sym_hex] = ACTIONS(76),
    [sym_binary] = ACTIONS(76),
    [sym_big_number] = ACTIONS(76),
    [sym_float] = ACTIONS(76),
    [sym_integer] = ACTIONS(78),
    [sym_boolean] = ACTIONS(66),
    [sym_none] = ACTIONS(66),
    [sym_float_const] = ACTIONS(66),
    [sym_type_identifier] = ACTIONS(66),
    [anon_sym_LBRACK] = ACTIONS(80),
    [anon_sym_RBRACK] = ACTIONS(92),
    [anon_sym_LBRACE] = ACTIONS(82),
    [anon_sym_LPAREN] = ACTIONS(84),
    [sym_modifier] = ACTIONS(66),
    [sym_operator] = ACTIONS(86),
    [sym_punctuation] = ACTIONS(66),
  },
  [6] = {
    [sym__expression] = STATE(2),
    [sym__literal] = STATE(2),
    [sym_string] = STATE(2),
    [sym__single_string] = STATE(31),
    [sym__double_string] = STATE(31),
    [sym_template_string] = STATE(2),
    [sym_number] = STATE(2),
    [sym_word] = STATE(2),
    [sym_list] = STATE(2),
    [sym_map] = STATE(2),
    [sym_group] = STATE(2),
    [aux_sym_source_file_repeat1] = STATE(2),
    [sym__word_token] = ACTIONS(64),
    [sym_line_comment] = ACTIONS(66),
    [sym_block_comment] = ACTIONS(66),
    [anon_sym_SQUOTE] = ACTIONS(68),
    [anon_sym_DQUOTE] = ACTIONS(70),
    [anon_sym_BQUOTE] = ACTIONS(72),
    [sym_hex] = ACTIONS(76),
    [sym_binary] = ACTIONS(76),
    [sym_big_number] = ACTIONS(76),
    [sym_float] = ACTIONS(76),
    [sym_integer] = ACTIONS(78),
    [sym_boolean] = ACTIONS(66),
    [sym_none] = ACTIONS(66),
    [sym_float_const] = ACTIONS(66),
    [sym_type_identifier] = ACTIONS(66),
    [anon_sym_LBRACK] = ACTIONS(80),
    [anon_sym_LBRACE] = ACTIONS(82),
    [anon_sym_LPAREN] = ACTIONS(84),
    [anon_sym_RPAREN] = ACTIONS(94),
    [sym_modifier] = ACTIONS(66),
    [sym_operator] = ACTIONS(86),
    [sym_punctuation] = ACTIONS(66),
  },
  [7] = {
    [sym__expression] = STATE(2),
    [sym__literal] = STATE(2),
    [sym_string] = STATE(2),
    [sym__single_string] = STATE(31),
    [sym__double_string] = STATE(31),
    [sym_template_string] = STATE(2),
    [sym_number] = STATE(2),
    [sym_word] = STATE(2),
    [sym_list] = STATE(2),
    [sym_map] = STATE(2),
    [sym_group] = STATE(2),
    [aux_sym_source_file_repeat1] = STATE(2),
    [sym__word_token] = ACTIONS(64),
    [sym_line_comment] = ACTIONS(66),
    [sym_block_comment] = ACTIONS(66),
    [anon_sym_SQUOTE] = ACTIONS(68),
    [anon_sym_DQUOTE] = ACTIONS(70),
    [anon_sym_BQUOTE] = ACTIONS(72),
    [anon_sym_RBRACE] = ACTIONS(96),
    [sym_hex] = ACTIONS(76),
    [sym_binary] = ACTIONS(76),
    [sym_big_number] = ACTIONS(76),
    [sym_float] = ACTIONS(76),
    [sym_integer] = ACTIONS(78),
    [sym_boolean] = ACTIONS(66),
    [sym_none] = ACTIONS(66),
    [sym_float_const] = ACTIONS(66),
    [sym_type_identifier] = ACTIONS(66),
    [anon_sym_LBRACK] = ACTIONS(80),
    [anon_sym_LBRACE] = ACTIONS(82),
    [anon_sym_LPAREN] = ACTIONS(84),
    [sym_modifier] = ACTIONS(66),
    [sym_operator] = ACTIONS(86),
    [sym_punctuation] = ACTIONS(66),
  },
  [8] = {
    [sym__expression] = STATE(2),
    [sym__literal] = STATE(2),
    [sym_string] = STATE(2),
    [sym__single_string] = STATE(31),
    [sym__double_string] = STATE(31),
    [sym_template_string] = STATE(2),
    [sym_number] = STATE(2),
    [sym_word] = STATE(2),
    [sym_list] = STATE(2),
    [sym_map] = STATE(2),
    [sym_group] = STATE(2),
    [aux_sym_source_file_repeat1] = STATE(2),
    [sym__word_token] = ACTIONS(64),
    [sym_line_comment] = ACTIONS(66),
    [sym_block_comment] = ACTIONS(66),
    [anon_sym_SQUOTE] = ACTIONS(68),
    [anon_sym_DQUOTE] = ACTIONS(70),
    [anon_sym_BQUOTE] = ACTIONS(72),
    [sym_hex] = ACTIONS(76),
    [sym_binary] = ACTIONS(76),
    [sym_big_number] = ACTIONS(76),
    [sym_float] = ACTIONS(76),
    [sym_integer] = ACTIONS(78),
    [sym_boolean] = ACTIONS(66),
    [sym_none] = ACTIONS(66),
    [sym_float_const] = ACTIONS(66),
    [sym_type_identifier] = ACTIONS(66),
    [anon_sym_LBRACK] = ACTIONS(80),
    [anon_sym_RBRACK] = ACTIONS(98),
    [anon_sym_LBRACE] = ACTIONS(82),
    [anon_sym_LPAREN] = ACTIONS(84),
    [sym_modifier] = ACTIONS(66),
    [sym_operator] = ACTIONS(86),
    [sym_punctuation] = ACTIONS(66),
  },
  [9] = {
    [sym__expression] = STATE(6),
    [sym__literal] = STATE(6),
    [sym_string] = STATE(6),
    [sym__single_string] = STATE(31),
    [sym__double_string] = STATE(31),
    [sym_template_string] = STATE(6),
    [sym_number] = STATE(6),
    [sym_word] = STATE(6),
    [sym_list] = STATE(6),
    [sym_map] = STATE(6),
    [sym_group] = STATE(6),
    [aux_sym_source_file_repeat1] = STATE(6),
    [sym__word_token] = ACTIONS(64),
    [sym_line_comment] = ACTIONS(100),
    [sym_block_comment] = ACTIONS(100),
    [anon_sym_SQUOTE] = ACTIONS(68),
    [anon_sym_DQUOTE] = ACTIONS(70),
    [anon_sym_BQUOTE] = ACTIONS(72),
    [sym_hex] = ACTIONS(76),
    [sym_binary] = ACTIONS(76),
    [sym_big_number] = ACTIONS(76),
    [sym_float] = ACTIONS(76),
    [sym_integer] = ACTIONS(78),
    [sym_boolean] = ACTIONS(100),
    [sym_none] = ACTIONS(100),
    [sym_float_const] = ACTIONS(100),
    [sym_type_identifier] = ACTIONS(100),
    [anon_sym_LBRACK] = ACTIONS(80),
    [anon_sym_LBRACE] = ACTIONS(82),
    [anon_sym_LPAREN] = ACTIONS(84),
    [anon_sym_RPAREN] = ACTIONS(102),
    [sym_modifier] = ACTIONS(100),
    [sym_operator] = ACTIONS(104),
    [sym_punctuation] = ACTIONS(100),
  },
  [10] = {
    [sym__expression] = STATE(7),
    [sym__literal] = STATE(7),
    [sym_string] = STATE(7),
    [sym__single_string] = STATE(31),
    [sym__double_string] = STATE(31),
    [sym_template_string] = STATE(7),
    [sym_number] = STATE(7),
    [sym_word] = STATE(7),
    [sym_list] = STATE(7),
    [sym_map] = STATE(7),
    [sym_group] = STATE(7),
    [aux_sym_source_file_repeat1] = STATE(7),
    [sym__word_token] = ACTIONS(64),
    [sym_line_comment] = ACTIONS(106),
    [sym_block_comment] = ACTIONS(106),
    [anon_sym_SQUOTE] = ACTIONS(68),
    [anon_sym_DQUOTE] = ACTIONS(70),
    [anon_sym_BQUOTE] = ACTIONS(72),
    [anon_sym_RBRACE] = ACTIONS(108),
    [sym_hex] = ACTIONS(76),
    [sym_binary] = ACTIONS(76),
    [sym_big_number] = ACTIONS(76),
    [sym_float] = ACTIONS(76),
    [sym_integer] = ACTIONS(78),
    [sym_boolean] = ACTIONS(106),
    [sym_none] = ACTIONS(106),
    [sym_float_const] = ACTIONS(106),
    [sym_type_identifier] = ACTIONS(106),
    [anon_sym_LBRACK] = ACTIONS(80),
    [anon_sym_LBRACE] = ACTIONS(82),
    [anon_sym_LPAREN] = ACTIONS(84),
    [sym_modifier] = ACTIONS(106),
    [sym_operator] = ACTIONS(110),
    [sym_punctuation] = ACTIONS(106),
  },
  [11] = {
    [sym__expression] = STATE(5),
    [sym__literal] = STATE(5),
    [sym_string] = STATE(5),
    [sym__single_string] = STATE(31),
    [sym__double_string] = STATE(31),
    [sym_template_string] = STATE(5),
    [sym_number] = STATE(5),
    [sym_word] = STATE(5),
    [sym_list] = STATE(5),
    [sym_map] = STATE(5),
    [sym_group] = STATE(5),
    [aux_sym_source_file_repeat1] = STATE(5),
    [sym__word_token] = ACTIONS(64),
    [sym_line_comment] = ACTIONS(112),
    [sym_block_comment] = ACTIONS(112),
    [anon_sym_SQUOTE] = ACTIONS(68),
    [anon_sym_DQUOTE] = ACTIONS(70),
    [anon_sym_BQUOTE] = ACTIONS(72),
    [sym_hex] = ACTIONS(76),
    [sym_binary] = ACTIONS(76),
    [sym_big_number] = ACTIONS(76),
    [sym_float] = ACTIONS(76),
    [sym_integer] = ACTIONS(78),
    [sym_boolean] = ACTIONS(112),
    [sym_none] = ACTIONS(112),
    [sym_float_const] = ACTIONS(112),
    [sym_type_identifier] = ACTIONS(112),
    [anon_sym_LBRACK] = ACTIONS(80),
    [anon_sym_RBRACK] = ACTIONS(114),
    [anon_sym_LBRACE] = ACTIONS(82),
    [anon_sym_LPAREN] = ACTIONS(84),
    [sym_modifier] = ACTIONS(112),
    [sym_operator] = ACTIONS(116),
    [sym_punctuation] = ACTIONS(112),
  },
  [12] = {
    [sym__expression] = STATE(18),
    [sym__literal] = STATE(18),
    [sym_string] = STATE(18),
    [sym__single_string] = STATE(31),
    [sym__double_string] = STATE(31),
    [sym_template_string] = STATE(18),
    [sym_number] = STATE(18),
    [sym_word] = STATE(18),
    [sym_list] = STATE(18),
    [sym_map] = STATE(18),
    [sym_group] = STATE(18),
    [aux_sym_source_file_repeat1] = STATE(18),
    [sym__word_token] = ACTIONS(64),
    [sym_line_comment] = ACTIONS(118),
    [sym_block_comment] = ACTIONS(118),
    [anon_sym_SQUOTE] = ACTIONS(68),
    [anon_sym_DQUOTE] = ACTIONS(70),
    [anon_sym_BQUOTE] = ACTIONS(72),
    [anon_sym_RBRACE] = ACTIONS(120),
    [sym_hex] = ACTIONS(76),
    [sym_binary] = ACTIONS(76),
    [sym_big_number] = ACTIONS(76),
    [sym_float] = ACTIONS(76),
    [sym_integer] = ACTIONS(78),
    [sym_boolean] = ACTIONS(118),
    [sym_none] = ACTIONS(118),
    [sym_float_const] = ACTIONS(118),
    [sym_type_identifier] = ACTIONS(118),
    [anon_sym_LBRACK] = ACTIONS(80),
    [anon_sym_LBRACE] = ACTIONS(82),
    [anon_sym_LPAREN] = ACTIONS(84),
    [sym_modifier] = ACTIONS(118),
    [sym_operator] = ACTIONS(122),
    [sym_punctuation] = ACTIONS(118),
  },
  [13] = {
    [sym__expression] = STATE(17),
    [sym__literal] = STATE(17),
    [sym_string] = STATE(17),
    [sym__single_string] = STATE(31),
    [sym__double_string] = STATE(31),
    [sym_template_string] = STATE(17),
    [sym_number] = STATE(17),
    [sym_word] = STATE(17),
    [sym_list] = STATE(17),
    [sym_map] = STATE(17),
    [sym_group] = STATE(17),
    [aux_sym_source_file_repeat1] = STATE(17),
    [sym__word_token] = ACTIONS(64),
    [sym_line_comment] = ACTIONS(124),
    [sym_block_comment] = ACTIONS(124),
    [anon_sym_SQUOTE] = ACTIONS(68),
    [anon_sym_DQUOTE] = ACTIONS(70),
    [anon_sym_BQUOTE] = ACTIONS(72),
    [sym_hex] = ACTIONS(76),
    [sym_binary] = ACTIONS(76),
    [sym_big_number] = ACTIONS(76),
    [sym_float] = ACTIONS(76),
    [sym_integer] = ACTIONS(78),
    [sym_boolean] = ACTIONS(124),
    [sym_none] = ACTIONS(124),
    [sym_float_const] = ACTIONS(124),
    [sym_type_identifier] = ACTIONS(124),
    [anon_sym_LBRACK] = ACTIONS(80),
    [anon_sym_LBRACE] = ACTIONS(82),
    [anon_sym_LPAREN] = ACTIONS(84),
    [anon_sym_RPAREN] = ACTIONS(126),
    [sym_modifier] = ACTIONS(124),
    [sym_operator] = ACTIONS(128),
    [sym_punctuation] = ACTIONS(124),
  },
  [14] = {
    [sym__expression] = STATE(8),
    [sym__literal] = STATE(8),
    [sym_string] = STATE(8),
    [sym__single_string] = STATE(31),
    [sym__double_string] = STATE(31),
    [sym_template_string] = STATE(8),
    [sym_number] = STATE(8),
    [sym_word] = STATE(8),
    [sym_list] = STATE(8),
    [sym_map] = STATE(8),
    [sym_group] = STATE(8),
    [aux_sym_source_file_repeat1] = STATE(8),
    [sym__word_token] = ACTIONS(64),
    [sym_line_comment] = ACTIONS(130),
    [sym_block_comment] = ACTIONS(130),
    [anon_sym_SQUOTE] = ACTIONS(68),
    [anon_sym_DQUOTE] = ACTIONS(70),
    [anon_sym_BQUOTE] = ACTIONS(72),
    [sym_hex] = ACTIONS(76),
    [sym_binary] = ACTIONS(76),
    [sym_big_number] = ACTIONS(76),
    [sym_float] = ACTIONS(76),
    [sym_integer] = ACTIONS(78),
    [sym_boolean] = ACTIONS(130),
    [sym_none] = ACTIONS(130),
    [sym_float_const] = ACTIONS(130),
    [sym_type_identifier] = ACTIONS(130),
    [anon_sym_LBRACK] = ACTIONS(80),
    [anon_sym_RBRACK] = ACTIONS(132),
    [anon_sym_LBRACE] = ACTIONS(82),
    [anon_sym_LPAREN] = ACTIONS(84),
    [sym_modifier] = ACTIONS(130),
    [sym_operator] = ACTIONS(134),
    [sym_punctuation] = ACTIONS(130),
  },
  [15] = {
    [sym__expression] = STATE(16),
    [sym__literal] = STATE(16),
    [sym_string] = STATE(16),
    [sym__single_string] = STATE(42),
    [sym__double_string] = STATE(42),
    [sym_template_string] = STATE(16),
    [sym_number] = STATE(16),
    [sym_word] = STATE(16),
    [sym_list] = STATE(16),
    [sym_map] = STATE(16),
    [sym_group] = STATE(16),
    [aux_sym_source_file_repeat1] = STATE(16),
    [ts_builtin_sym_end] = ACTIONS(136),
    [sym__word_token] = ACTIONS(7),
    [sym_line_comment] = ACTIONS(138),
    [sym_block_comment] = ACTIONS(138),
    [anon_sym_SQUOTE] = ACTIONS(11),
    [anon_sym_DQUOTE] = ACTIONS(13),
    [anon_sym_BQUOTE] = ACTIONS(15),
    [sym_hex] = ACTIONS(17),
    [sym_binary] = ACTIONS(17),
    [sym_big_number] = ACTIONS(17),
    [sym_float] = ACTIONS(17),
    [sym_integer] = ACTIONS(19),
    [sym_boolean] = ACTIONS(138),
    [sym_none] = ACTIONS(138),
    [sym_float_const] = ACTIONS(138),
    [sym_type_identifier] = ACTIONS(138),
    [anon_sym_LBRACK] = ACTIONS(21),
    [anon_sym_LBRACE] = ACTIONS(23),
    [anon_sym_LPAREN] = ACTIONS(25),
    [sym_modifier] = ACTIONS(138),
    [sym_operator] = ACTIONS(140),
    [sym_punctuation] = ACTIONS(138),
  },
  [16] = {
    [sym__expression] = STATE(16),
    [sym__literal] = STATE(16),
    [sym_string] = STATE(16),
    [sym__single_string] = STATE(42),
    [sym__double_string] = STATE(42),
    [sym_template_string] = STATE(16),
    [sym_number] = STATE(16),
    [sym_word] = STATE(16),
    [sym_list] = STATE(16),
    [sym_map] = STATE(16),
    [sym_group] = STATE(16),
    [aux_sym_source_file_repeat1] = STATE(16),
    [ts_builtin_sym_end] = ACTIONS(44),
    [sym__word_token] = ACTIONS(142),
    [sym_line_comment] = ACTIONS(145),
    [sym_block_comment] = ACTIONS(145),
    [anon_sym_SQUOTE] = ACTIONS(148),
    [anon_sym_DQUOTE] = ACTIONS(151),
    [anon_sym_BQUOTE] = ACTIONS(154),
    [sym_hex] = ACTIONS(157),
    [sym_binary] = ACTIONS(157),
    [sym_big_number] = ACTIONS(157),
    [sym_float] = ACTIONS(157),
    [sym_integer] = ACTIONS(160),
    [sym_boolean] = ACTIONS(145),
    [sym_none] = ACTIONS(145),
    [sym_float_const] = ACTIONS(145),
    [sym_type_identifier] = ACTIONS(145),
    [anon_sym_LBRACK] = ACTIONS(163),
    [anon_sym_LBRACE] = ACTIONS(166),
    [anon_sym_LPAREN] = ACTIONS(169),
    [sym_modifier] = ACTIONS(145),
    [sym_operator] = ACTIONS(172),
    [sym_punctuation] = ACTIONS(145),
  },
  [17] = {
    [sym__expression] = STATE(2),
    [sym__literal] = STATE(2),
    [sym_string] = STATE(2),
    [sym__single_string] = STATE(31),
    [sym__double_string] = STATE(31),
    [sym_template_string] = STATE(2),
    [sym_number] = STATE(2),
    [sym_word] = STATE(2),
    [sym_list] = STATE(2),
    [sym_map] = STATE(2),
    [sym_group] = STATE(2),
    [aux_sym_source_file_repeat1] = STATE(2),
    [sym__word_token] = ACTIONS(64),
    [sym_line_comment] = ACTIONS(66),
    [sym_block_comment] = ACTIONS(66),
    [anon_sym_SQUOTE] = ACTIONS(68),
    [anon_sym_DQUOTE] = ACTIONS(70),
    [anon_sym_BQUOTE] = ACTIONS(72),
    [sym_hex] = ACTIONS(76),
    [sym_binary] = ACTIONS(76),
    [sym_big_number] = ACTIONS(76),
    [sym_float] = ACTIONS(76),
    [sym_integer] = ACTIONS(78),
    [sym_boolean] = ACTIONS(66),
    [sym_none] = ACTIONS(66),
    [sym_float_const] = ACTIONS(66),
    [sym_type_identifier] = ACTIONS(66),
    [anon_sym_LBRACK] = ACTIONS(80),
    [anon_sym_LBRACE] = ACTIONS(82),
    [anon_sym_LPAREN] = ACTIONS(84),
    [anon_sym_RPAREN] = ACTIONS(175),
    [sym_modifier] = ACTIONS(66),
    [sym_operator] = ACTIONS(86),
    [sym_punctuation] = ACTIONS(66),
  },
  [18] = {
    [sym__expression] = STATE(2),
    [sym__literal] = STATE(2),
    [sym_string] = STATE(2),
    [sym__single_string] = STATE(31),
    [sym__double_string] = STATE(31),
    [sym_template_string] = STATE(2),
    [sym_number] = STATE(2),
    [sym_word] = STATE(2),
    [sym_list] = STATE(2),
    [sym_map] = STATE(2),
    [sym_group] = STATE(2),
    [aux_sym_source_file_repeat1] = STATE(2),
    [sym__word_token] = ACTIONS(64),
    [sym_line_comment] = ACTIONS(66),
    [sym_block_comment] = ACTIONS(66),
    [anon_sym_SQUOTE] = ACTIONS(68),
    [anon_sym_DQUOTE] = ACTIONS(70),
    [anon_sym_BQUOTE] = ACTIONS(72),
    [anon_sym_RBRACE] = ACTIONS(177),
    [sym_hex] = ACTIONS(76),
    [sym_binary] = ACTIONS(76),
    [sym_big_number] = ACTIONS(76),
    [sym_float] = ACTIONS(76),
    [sym_integer] = ACTIONS(78),
    [sym_boolean] = ACTIONS(66),
    [sym_none] = ACTIONS(66),
    [sym_float_const] = ACTIONS(66),
    [sym_type_identifier] = ACTIONS(66),
    [anon_sym_LBRACK] = ACTIONS(80),
    [anon_sym_LBRACE] = ACTIONS(82),
    [anon_sym_LPAREN] = ACTIONS(84),
    [sym_modifier] = ACTIONS(66),
    [sym_operator] = ACTIONS(86),
    [sym_punctuation] = ACTIONS(66),
  },
};

static const uint16_t ts_small_parse_table[] = {
  [0] = 2,
    ACTIONS(179), 3,
      sym_integer,
      sym__word_token,
      sym_operator,
    ACTIONS(181), 21,
      sym_line_comment,
      sym_block_comment,
      anon_sym_SQUOTE,
      anon_sym_DQUOTE,
      anon_sym_BQUOTE,
      anon_sym_RBRACE,
      sym_hex,
      sym_binary,
      sym_big_number,
      sym_float,
      sym_boolean,
      sym_none,
      sym_float_const,
      sym_type_identifier,
      anon_sym_LBRACK,
      anon_sym_RBRACK,
      anon_sym_LBRACE,
      anon_sym_LPAREN,
      anon_sym_RPAREN,
      sym_modifier,
      sym_punctuation,
  [29] = 2,
    ACTIONS(183), 3,
      sym_integer,
      sym__word_token,
      sym_operator,
    ACTIONS(185), 21,
      sym_line_comment,
      sym_block_comment,
      anon_sym_SQUOTE,
      anon_sym_DQUOTE,
      anon_sym_BQUOTE,
      anon_sym_RBRACE,
      sym_hex,
      sym_binary,
      sym_big_number,
      sym_float,
      sym_boolean,
      sym_none,
      sym_float_const,
      sym_type_identifier,
      anon_sym_LBRACK,
      anon_sym_RBRACK,
      anon_sym_LBRACE,
      anon_sym_LPAREN,
      anon_sym_RPAREN,
      sym_modifier,
      sym_punctuation,
  [58] = 2,
    ACTIONS(187), 3,
      sym_integer,
      sym__word_token,
      sym_operator,
    ACTIONS(189), 21,
      sym_line_comment,
      sym_block_comment,
      anon_sym_SQUOTE,
      anon_sym_DQUOTE,
      anon_sym_BQUOTE,
      anon_sym_RBRACE,
      sym_hex,
      sym_binary,
      sym_big_number,
      sym_float,
      sym_boolean,
      sym_none,
      sym_float_const,
      sym_type_identifier,
      anon_sym_LBRACK,
      anon_sym_RBRACK,
      anon_sym_LBRACE,
      anon_sym_LPAREN,
      anon_sym_RPAREN,
      sym_modifier,
      sym_punctuation,
  [87] = 2,
    ACTIONS(191), 3,
      sym_integer,
      sym__word_token,
      sym_operator,
    ACTIONS(193), 21,
      sym_line_comment,
      sym_block_comment,
      anon_sym_SQUOTE,
      anon_sym_DQUOTE,
      anon_sym_BQUOTE,
      anon_sym_RBRACE,
      sym_hex,
      sym_binary,
      sym_big_number,
      sym_float,
      sym_boolean,
      sym_none,
      sym_float_const,
      sym_type_identifier,
      anon_sym_LBRACK,
      anon_sym_RBRACK,
      anon_sym_LBRACE,
      anon_sym_LPAREN,
      anon_sym_RPAREN,
      sym_modifier,
      sym_punctuation,
  [116] = 2,
    ACTIONS(195), 3,
      sym_integer,
      sym__word_token,
      sym_operator,
    ACTIONS(197), 21,
      sym_line_comment,
      sym_block_comment,
      anon_sym_SQUOTE,
      anon_sym_DQUOTE,
      anon_sym_BQUOTE,
      anon_sym_RBRACE,
      sym_hex,
      sym_binary,
      sym_big_number,
      sym_float,
      sym_boolean,
      sym_none,
      sym_float_const,
      sym_type_identifier,
      anon_sym_LBRACK,
      anon_sym_RBRACK,
      anon_sym_LBRACE,
      anon_sym_LPAREN,
      anon_sym_RPAREN,
      sym_modifier,
      sym_punctuation,
  [145] = 2,
    ACTIONS(199), 3,
      sym_integer,
      sym__word_token,
      sym_operator,
    ACTIONS(201), 21,
      sym_line_comment,
      sym_block_comment,
      anon_sym_SQUOTE,
      anon_sym_DQUOTE,
      anon_sym_BQUOTE,
      anon_sym_RBRACE,
      sym_hex,
      sym_binary,
      sym_big_number,
      sym_float,
      sym_boolean,
      sym_none,
      sym_float_const,
      sym_type_identifier,
      anon_sym_LBRACK,
      anon_sym_RBRACK,
      anon_sym_LBRACE,
      anon_sym_LPAREN,
      anon_sym_RPAREN,
      sym_modifier,
      sym_punctuation,
  [174] = 2,
    ACTIONS(203), 3,
      sym_integer,
      sym__word_token,
      sym_operator,
    ACTIONS(205), 21,
      sym_line_comment,
      sym_block_comment,
      anon_sym_SQUOTE,
      anon_sym_DQUOTE,
      anon_sym_BQUOTE,
      anon_sym_RBRACE,
      sym_hex,
      sym_binary,
      sym_big_number,
      sym_float,
      sym_boolean,
      sym_none,
      sym_float_const,
      sym_type_identifier,
      anon_sym_LBRACK,
      anon_sym_RBRACK,
      anon_sym_LBRACE,
      anon_sym_LPAREN,
      anon_sym_RPAREN,
      sym_modifier,
      sym_punctuation,
  [203] = 2,
    ACTIONS(207), 3,
      sym_integer,
      sym__word_token,
      sym_operator,
    ACTIONS(209), 21,
      sym_line_comment,
      sym_block_comment,
      anon_sym_SQUOTE,
      anon_sym_DQUOTE,
      anon_sym_BQUOTE,
      anon_sym_RBRACE,
      sym_hex,
      sym_binary,
      sym_big_number,
      sym_float,
      sym_boolean,
      sym_none,
      sym_float_const,
      sym_type_identifier,
      anon_sym_LBRACK,
      anon_sym_RBRACK,
      anon_sym_LBRACE,
      anon_sym_LPAREN,
      anon_sym_RPAREN,
      sym_modifier,
      sym_punctuation,
  [232] = 2,
    ACTIONS(211), 3,
      sym_integer,
      sym__word_token,
      sym_operator,
    ACTIONS(213), 21,
      sym_line_comment,
      sym_block_comment,
      anon_sym_SQUOTE,
      anon_sym_DQUOTE,
      anon_sym_BQUOTE,
      anon_sym_RBRACE,
      sym_hex,
      sym_binary,
      sym_big_number,
      sym_float,
      sym_boolean,
      sym_none,
      sym_float_const,
      sym_type_identifier,
      anon_sym_LBRACK,
      anon_sym_RBRACK,
      anon_sym_LBRACE,
      anon_sym_LPAREN,
      anon_sym_RPAREN,
      sym_modifier,
      sym_punctuation,
  [261] = 2,
    ACTIONS(215), 3,
      sym_integer,
      sym__word_token,
      sym_operator,
    ACTIONS(217), 21,
      sym_line_comment,
      sym_block_comment,
      anon_sym_SQUOTE,
      anon_sym_DQUOTE,
      anon_sym_BQUOTE,
      anon_sym_RBRACE,
      sym_hex,
      sym_binary,
      sym_big_number,
      sym_float,
      sym_boolean,
      sym_none,
      sym_float_const,
      sym_type_identifier,
      anon_sym_LBRACK,
      anon_sym_RBRACK,
      anon_sym_LBRACE,
      anon_sym_LPAREN,
      anon_sym_RPAREN,
      sym_modifier,
      sym_punctuation,
  [290] = 2,
    ACTIONS(219), 3,
      sym_integer,
      sym__word_token,
      sym_operator,
    ACTIONS(221), 21,
      sym_line_comment,
      sym_block_comment,
      anon_sym_SQUOTE,
      anon_sym_DQUOTE,
      anon_sym_BQUOTE,
      anon_sym_RBRACE,
      sym_hex,
      sym_binary,
      sym_big_number,
      sym_float,
      sym_boolean,
      sym_none,
      sym_float_const,
      sym_type_identifier,
      anon_sym_LBRACK,
      anon_sym_RBRACK,
      anon_sym_LBRACE,
      anon_sym_LPAREN,
      anon_sym_RPAREN,
      sym_modifier,
      sym_punctuation,
  [319] = 2,
    ACTIONS(223), 3,
      sym_integer,
      sym__word_token,
      sym_operator,
    ACTIONS(225), 21,
      sym_line_comment,
      sym_block_comment,
      anon_sym_SQUOTE,
      anon_sym_DQUOTE,
      anon_sym_BQUOTE,
      anon_sym_RBRACE,
      sym_hex,
      sym_binary,
      sym_big_number,
      sym_float,
      sym_boolean,
      sym_none,
      sym_float_const,
      sym_type_identifier,
      anon_sym_LBRACK,
      anon_sym_RBRACK,
      anon_sym_LBRACE,
      anon_sym_LPAREN,
      anon_sym_RPAREN,
      sym_modifier,
      sym_punctuation,
  [348] = 2,
    ACTIONS(227), 3,
      sym_integer,
      sym__word_token,
      sym_operator,
    ACTIONS(229), 21,
      sym_line_comment,
      sym_block_comment,
      anon_sym_SQUOTE,
      anon_sym_DQUOTE,
      anon_sym_BQUOTE,
      anon_sym_RBRACE,
      sym_hex,
      sym_binary,
      sym_big_number,
      sym_float,
      sym_boolean,
      sym_none,
      sym_float_const,
      sym_type_identifier,
      anon_sym_LBRACK,
      anon_sym_RBRACK,
      anon_sym_LBRACE,
      anon_sym_LPAREN,
      anon_sym_RPAREN,
      sym_modifier,
      sym_punctuation,
  [377] = 2,
    ACTIONS(231), 3,
      sym_integer,
      sym__word_token,
      sym_operator,
    ACTIONS(233), 21,
      sym_line_comment,
      sym_block_comment,
      anon_sym_SQUOTE,
      anon_sym_DQUOTE,
      anon_sym_BQUOTE,
      anon_sym_RBRACE,
      sym_hex,
      sym_binary,
      sym_big_number,
      sym_float,
      sym_boolean,
      sym_none,
      sym_float_const,
      sym_type_identifier,
      anon_sym_LBRACK,
      anon_sym_RBRACK,
      anon_sym_LBRACE,
      anon_sym_LPAREN,
      anon_sym_RPAREN,
      sym_modifier,
      sym_punctuation,
  [406] = 2,
    ACTIONS(235), 3,
      sym_integer,
      sym__word_token,
      sym_operator,
    ACTIONS(237), 21,
      sym_line_comment,
      sym_block_comment,
      anon_sym_SQUOTE,
      anon_sym_DQUOTE,
      anon_sym_BQUOTE,
      anon_sym_RBRACE,
      sym_hex,
      sym_binary,
      sym_big_number,
      sym_float,
      sym_boolean,
      sym_none,
      sym_float_const,
      sym_type_identifier,
      anon_sym_LBRACK,
      anon_sym_RBRACK,
      anon_sym_LBRACE,
      anon_sym_LPAREN,
      anon_sym_RPAREN,
      sym_modifier,
      sym_punctuation,
  [435] = 2,
    ACTIONS(215), 3,
      sym_integer,
      sym__word_token,
      sym_operator,
    ACTIONS(217), 19,
      ts_builtin_sym_end,
      sym_line_comment,
      sym_block_comment,
      anon_sym_SQUOTE,
      anon_sym_DQUOTE,
      anon_sym_BQUOTE,
      sym_hex,
      sym_binary,
      sym_big_number,
      sym_float,
      sym_boolean,
      sym_none,
      sym_float_const,
      sym_type_identifier,
      anon_sym_LBRACK,
      anon_sym_LBRACE,
      anon_sym_LPAREN,
      sym_modifier,
      sym_punctuation,
  [462] = 2,
    ACTIONS(199), 3,
      sym_integer,
      sym__word_token,
      sym_operator,
    ACTIONS(201), 19,
      ts_builtin_sym_end,
      sym_line_comment,
      sym_block_comment,
      anon_sym_SQUOTE,
      anon_sym_DQUOTE,
      anon_sym_BQUOTE,
      sym_hex,
      sym_binary,
      sym_big_number,
      sym_float,
      sym_boolean,
      sym_none,
      sym_float_const,
      sym_type_identifier,
      anon_sym_LBRACK,
      anon_sym_LBRACE,
      anon_sym_LPAREN,
      sym_modifier,
      sym_punctuation,
  [489] = 2,
    ACTIONS(207), 3,
      sym_integer,
      sym__word_token,
      sym_operator,
    ACTIONS(209), 19,
      ts_builtin_sym_end,
      sym_line_comment,
      sym_block_comment,
      anon_sym_SQUOTE,
      anon_sym_DQUOTE,
      anon_sym_BQUOTE,
      sym_hex,
      sym_binary,
      sym_big_number,
      sym_float,
      sym_boolean,
      sym_none,
      sym_float_const,
      sym_type_identifier,
      anon_sym_LBRACK,
      anon_sym_LBRACE,
      anon_sym_LPAREN,
      sym_modifier,
      sym_punctuation,
  [516] = 2,
    ACTIONS(235), 3,
      sym_integer,
      sym__word_token,
      sym_operator,
    ACTIONS(237), 19,
      ts_builtin_sym_end,
      sym_line_comment,
      sym_block_comment,
      anon_sym_SQUOTE,
      anon_sym_DQUOTE,
      anon_sym_BQUOTE,
      sym_hex,
      sym_binary,
      sym_big_number,
      sym_float,
      sym_boolean,
      sym_none,
      sym_float_const,
      sym_type_identifier,
      anon_sym_LBRACK,
      anon_sym_LBRACE,
      anon_sym_LPAREN,
      sym_modifier,
      sym_punctuation,
  [543] = 2,
    ACTIONS(195), 3,
      sym_integer,
      sym__word_token,
      sym_operator,
    ACTIONS(197), 19,
      ts_builtin_sym_end,
      sym_line_comment,
      sym_block_comment,
      anon_sym_SQUOTE,
      anon_sym_DQUOTE,
      anon_sym_BQUOTE,
      sym_hex,
      sym_binary,
      sym_big_number,
      sym_float,
      sym_boolean,
      sym_none,
      sym_float_const,
      sym_type_identifier,
      anon_sym_LBRACK,
      anon_sym_LBRACE,
      anon_sym_LPAREN,
      sym_modifier,
      sym_punctuation,
  [570] = 2,
    ACTIONS(183), 3,
      sym_integer,
      sym__word_token,
      sym_operator,
    ACTIONS(185), 19,
      ts_builtin_sym_end,
      sym_line_comment,
      sym_block_comment,
      anon_sym_SQUOTE,
      anon_sym_DQUOTE,
      anon_sym_BQUOTE,
      sym_hex,
      sym_binary,
      sym_big_number,
      sym_float,
      sym_boolean,
      sym_none,
      sym_float_const,
      sym_type_identifier,
      anon_sym_LBRACK,
      anon_sym_LBRACE,
      anon_sym_LPAREN,
      sym_modifier,
      sym_punctuation,
  [597] = 2,
    ACTIONS(231), 3,
      sym_integer,
      sym__word_token,
      sym_operator,
    ACTIONS(233), 19,
      ts_builtin_sym_end,
      sym_line_comment,
      sym_block_comment,
      anon_sym_SQUOTE,
      anon_sym_DQUOTE,
      anon_sym_BQUOTE,
      sym_hex,
      sym_binary,
      sym_big_number,
      sym_float,
      sym_boolean,
      sym_none,
      sym_float_const,
      sym_type_identifier,
      anon_sym_LBRACK,
      anon_sym_LBRACE,
      anon_sym_LPAREN,
      sym_modifier,
      sym_punctuation,
  [624] = 2,
    ACTIONS(203), 3,
      sym_integer,
      sym__word_token,
      sym_operator,
    ACTIONS(205), 19,
      ts_builtin_sym_end,
      sym_line_comment,
      sym_block_comment,
      anon_sym_SQUOTE,
      anon_sym_DQUOTE,
      anon_sym_BQUOTE,
      sym_hex,
      sym_binary,
      sym_big_number,
      sym_float,
      sym_boolean,
      sym_none,
      sym_float_const,
      sym_type_identifier,
      anon_sym_LBRACK,
      anon_sym_LBRACE,
      anon_sym_LPAREN,
      sym_modifier,
      sym_punctuation,
  [651] = 2,
    ACTIONS(227), 3,
      sym_integer,
      sym__word_token,
      sym_operator,
    ACTIONS(229), 19,
      ts_builtin_sym_end,
      sym_line_comment,
      sym_block_comment,
      anon_sym_SQUOTE,
      anon_sym_DQUOTE,
      anon_sym_BQUOTE,
      sym_hex,
      sym_binary,
      sym_big_number,
      sym_float,
      sym_boolean,
      sym_none,
      sym_float_const,
      sym_type_identifier,
      anon_sym_LBRACK,
      anon_sym_LBRACE,
      anon_sym_LPAREN,
      sym_modifier,
      sym_punctuation,
  [678] = 2,
    ACTIONS(223), 3,
      sym_integer,
      sym__word_token,
      sym_operator,
    ACTIONS(225), 19,
      ts_builtin_sym_end,
      sym_line_comment,
      sym_block_comment,
      anon_sym_SQUOTE,
      anon_sym_DQUOTE,
      anon_sym_BQUOTE,
      sym_hex,
      sym_binary,
      sym_big_number,
      sym_float,
      sym_boolean,
      sym_none,
      sym_float_const,
      sym_type_identifier,
      anon_sym_LBRACK,
      anon_sym_LBRACE,
      anon_sym_LPAREN,
      sym_modifier,
      sym_punctuation,
  [705] = 2,
    ACTIONS(219), 3,
      sym_integer,
      sym__word_token,
      sym_operator,
    ACTIONS(221), 19,
      ts_builtin_sym_end,
      sym_line_comment,
      sym_block_comment,
      anon_sym_SQUOTE,
      anon_sym_DQUOTE,
      anon_sym_BQUOTE,
      sym_hex,
      sym_binary,
      sym_big_number,
      sym_float,
      sym_boolean,
      sym_none,
      sym_float_const,
      sym_type_identifier,
      anon_sym_LBRACK,
      anon_sym_LBRACE,
      anon_sym_LPAREN,
      sym_modifier,
      sym_punctuation,
  [732] = 2,
    ACTIONS(179), 3,
      sym_integer,
      sym__word_token,
      sym_operator,
    ACTIONS(181), 19,
      ts_builtin_sym_end,
      sym_line_comment,
      sym_block_comment,
      anon_sym_SQUOTE,
      anon_sym_DQUOTE,
      anon_sym_BQUOTE,
      sym_hex,
      sym_binary,
      sym_big_number,
      sym_float,
      sym_boolean,
      sym_none,
      sym_float_const,
      sym_type_identifier,
      anon_sym_LBRACK,
      anon_sym_LBRACE,
      anon_sym_LPAREN,
      sym_modifier,
      sym_punctuation,
  [759] = 2,
    ACTIONS(211), 3,
      sym_integer,
      sym__word_token,
      sym_operator,
    ACTIONS(213), 19,
      ts_builtin_sym_end,
      sym_line_comment,
      sym_block_comment,
      anon_sym_SQUOTE,
      anon_sym_DQUOTE,
      anon_sym_BQUOTE,
      sym_hex,
      sym_binary,
      sym_big_number,
      sym_float,
      sym_boolean,
      sym_none,
      sym_float_const,
      sym_type_identifier,
      anon_sym_LBRACK,
      anon_sym_LBRACE,
      anon_sym_LPAREN,
      sym_modifier,
      sym_punctuation,
  [786] = 2,
    ACTIONS(187), 3,
      sym_integer,
      sym__word_token,
      sym_operator,
    ACTIONS(189), 19,
      ts_builtin_sym_end,
      sym_line_comment,
      sym_block_comment,
      anon_sym_SQUOTE,
      anon_sym_DQUOTE,
      anon_sym_BQUOTE,
      sym_hex,
      sym_binary,
      sym_big_number,
      sym_float,
      sym_boolean,
      sym_none,
      sym_float_const,
      sym_type_identifier,
      anon_sym_LBRACK,
      anon_sym_LBRACE,
      anon_sym_LPAREN,
      sym_modifier,
      sym_punctuation,
  [813] = 2,
    ACTIONS(191), 3,
      sym_integer,
      sym__word_token,
      sym_operator,
    ACTIONS(193), 19,
      ts_builtin_sym_end,
      sym_line_comment,
      sym_block_comment,
      anon_sym_SQUOTE,
      anon_sym_DQUOTE,
      anon_sym_BQUOTE,
      sym_hex,
      sym_binary,
      sym_big_number,
      sym_float,
      sym_boolean,
      sym_none,
      sym_float_const,
      sym_type_identifier,
      anon_sym_LBRACK,
      anon_sym_LBRACE,
      anon_sym_LPAREN,
      sym_modifier,
      sym_punctuation,
  [840] = 6,
    ACTIONS(241), 1,
      sym_escape_sequence,
    ACTIONS(243), 1,
      anon_sym_BQUOTE,
    ACTIONS(245), 1,
      sym__template_chars,
    ACTIONS(247), 1,
      sym_interpolation_start,
    ACTIONS(239), 2,
      sym_line_comment,
      sym_block_comment,
    STATE(50), 2,
      sym_interpolation,
      aux_sym_template_string_repeat1,
  [861] = 6,
    ACTIONS(249), 1,
      sym_escape_sequence,
    ACTIONS(252), 1,
      anon_sym_BQUOTE,
    ACTIONS(254), 1,
      sym__template_chars,
    ACTIONS(257), 1,
      sym_interpolation_start,
    ACTIONS(239), 2,
      sym_line_comment,
      sym_block_comment,
    STATE(50), 2,
      sym_interpolation,
      aux_sym_template_string_repeat1,
  [882] = 6,
    ACTIONS(247), 1,
      sym_interpolation_start,
    ACTIONS(260), 1,
      sym_escape_sequence,
    ACTIONS(262), 1,
      anon_sym_BQUOTE,
    ACTIONS(264), 1,
      sym__template_chars,
    ACTIONS(239), 2,
      sym_line_comment,
      sym_block_comment,
    STATE(52), 2,
      sym_interpolation,
      aux_sym_template_string_repeat1,
  [903] = 6,
    ACTIONS(241), 1,
      sym_escape_sequence,
    ACTIONS(245), 1,
      sym__template_chars,
    ACTIONS(247), 1,
      sym_interpolation_start,
    ACTIONS(266), 1,
      anon_sym_BQUOTE,
    ACTIONS(239), 2,
      sym_line_comment,
      sym_block_comment,
    STATE(50), 2,
      sym_interpolation,
      aux_sym_template_string_repeat1,
  [924] = 6,
    ACTIONS(247), 1,
      sym_interpolation_start,
    ACTIONS(268), 1,
      sym_escape_sequence,
    ACTIONS(270), 1,
      anon_sym_BQUOTE,
    ACTIONS(272), 1,
      sym__template_chars,
    ACTIONS(239), 2,
      sym_line_comment,
      sym_block_comment,
    STATE(49), 2,
      sym_interpolation,
      aux_sym_template_string_repeat1,
  [945] = 4,
    ACTIONS(277), 1,
      anon_sym_SQUOTE,
    STATE(54), 1,
      aux_sym__single_string_repeat1,
    ACTIONS(239), 2,
      sym_line_comment,
      sym_block_comment,
    ACTIONS(274), 2,
      sym_escape_sequence,
      aux_sym__single_string_token1,
  [960] = 4,
    ACTIONS(281), 1,
      anon_sym_DQUOTE,
    STATE(58), 1,
      aux_sym__double_string_repeat1,
    ACTIONS(239), 2,
      sym_line_comment,
      sym_block_comment,
    ACTIONS(279), 2,
      sym_escape_sequence,
      aux_sym__double_string_token1,
  [975] = 3,
    ACTIONS(239), 2,
      sym_line_comment,
      sym_block_comment,
    ACTIONS(283), 2,
      sym_escape_sequence,
      sym_interpolation_start,
    ACTIONS(285), 2,
      anon_sym_BQUOTE,
      sym__template_chars,
  [988] = 4,
    ACTIONS(289), 1,
      anon_sym_SQUOTE,
    STATE(54), 1,
      aux_sym__single_string_repeat1,
    ACTIONS(239), 2,
      sym_line_comment,
      sym_block_comment,
    ACTIONS(287), 2,
      sym_escape_sequence,
      aux_sym__single_string_token1,
  [1003] = 4,
    ACTIONS(294), 1,
      anon_sym_DQUOTE,
    STATE(58), 1,
      aux_sym__double_string_repeat1,
    ACTIONS(239), 2,
      sym_line_comment,
      sym_block_comment,
    ACTIONS(291), 2,
      sym_escape_sequence,
      aux_sym__double_string_token1,
  [1018] = 4,
    ACTIONS(298), 1,
      anon_sym_SQUOTE,
    STATE(63), 1,
      aux_sym__single_string_repeat1,
    ACTIONS(239), 2,
      sym_line_comment,
      sym_block_comment,
    ACTIONS(296), 2,
      sym_escape_sequence,
      aux_sym__single_string_token1,
  [1033] = 4,
    ACTIONS(302), 1,
      anon_sym_DQUOTE,
    STATE(64), 1,
      aux_sym__double_string_repeat1,
    ACTIONS(239), 2,
      sym_line_comment,
      sym_block_comment,
    ACTIONS(300), 2,
      sym_escape_sequence,
      aux_sym__double_string_token1,
  [1048] = 3,
    ACTIONS(239), 2,
      sym_line_comment,
      sym_block_comment,
    ACTIONS(304), 2,
      sym_escape_sequence,
      sym_interpolation_start,
    ACTIONS(306), 2,
      anon_sym_BQUOTE,
      sym__template_chars,
  [1061] = 3,
    ACTIONS(239), 2,
      sym_line_comment,
      sym_block_comment,
    ACTIONS(308), 2,
      sym_escape_sequence,
      sym_interpolation_start,
    ACTIONS(310), 2,
      anon_sym_BQUOTE,
      sym__template_chars,
  [1074] = 4,
    ACTIONS(312), 1,
      anon_sym_SQUOTE,
    STATE(54), 1,
      aux_sym__single_string_repeat1,
    ACTIONS(239), 2,
      sym_line_comment,
      sym_block_comment,
    ACTIONS(287), 2,
      sym_escape_sequence,
      aux_sym__single_string_token1,
  [1089] = 4,
    ACTIONS(314), 1,
      anon_sym_DQUOTE,
    STATE(58), 1,
      aux_sym__double_string_repeat1,
    ACTIONS(239), 2,
      sym_line_comment,
      sym_block_comment,
    ACTIONS(279), 2,
      sym_escape_sequence,
      aux_sym__double_string_token1,
  [1104] = 4,
    ACTIONS(318), 1,
      anon_sym_SQUOTE,
    STATE(57), 1,
      aux_sym__single_string_repeat1,
    ACTIONS(239), 2,
      sym_line_comment,
      sym_block_comment,
    ACTIONS(316), 2,
      sym_escape_sequence,
      aux_sym__single_string_token1,
  [1119] = 4,
    ACTIONS(322), 1,
      anon_sym_DQUOTE,
    STATE(55), 1,
      aux_sym__double_string_repeat1,
    ACTIONS(239), 2,
      sym_line_comment,
      sym_block_comment,
    ACTIONS(320), 2,
      sym_escape_sequence,
      aux_sym__double_string_token1,
  [1134] = 2,
    ACTIONS(324), 1,
      ts_builtin_sym_end,
    ACTIONS(3), 2,
      sym_line_comment,
      sym_block_comment,
};

static const uint32_t ts_small_parse_table_map[] = {
  [SMALL_STATE(19)] = 0,
  [SMALL_STATE(20)] = 29,
  [SMALL_STATE(21)] = 58,
  [SMALL_STATE(22)] = 87,
  [SMALL_STATE(23)] = 116,
  [SMALL_STATE(24)] = 145,
  [SMALL_STATE(25)] = 174,
  [SMALL_STATE(26)] = 203,
  [SMALL_STATE(27)] = 232,
  [SMALL_STATE(28)] = 261,
  [SMALL_STATE(29)] = 290,
  [SMALL_STATE(30)] = 319,
  [SMALL_STATE(31)] = 348,
  [SMALL_STATE(32)] = 377,
  [SMALL_STATE(33)] = 406,
  [SMALL_STATE(34)] = 435,
  [SMALL_STATE(35)] = 462,
  [SMALL_STATE(36)] = 489,
  [SMALL_STATE(37)] = 516,
  [SMALL_STATE(38)] = 543,
  [SMALL_STATE(39)] = 570,
  [SMALL_STATE(40)] = 597,
  [SMALL_STATE(41)] = 624,
  [SMALL_STATE(42)] = 651,
  [SMALL_STATE(43)] = 678,
  [SMALL_STATE(44)] = 705,
  [SMALL_STATE(45)] = 732,
  [SMALL_STATE(46)] = 759,
  [SMALL_STATE(47)] = 786,
  [SMALL_STATE(48)] = 813,
  [SMALL_STATE(49)] = 840,
  [SMALL_STATE(50)] = 861,
  [SMALL_STATE(51)] = 882,
  [SMALL_STATE(52)] = 903,
  [SMALL_STATE(53)] = 924,
  [SMALL_STATE(54)] = 945,
  [SMALL_STATE(55)] = 960,
  [SMALL_STATE(56)] = 975,
  [SMALL_STATE(57)] = 988,
  [SMALL_STATE(58)] = 1003,
  [SMALL_STATE(59)] = 1018,
  [SMALL_STATE(60)] = 1033,
  [SMALL_STATE(61)] = 1048,
  [SMALL_STATE(62)] = 1061,
  [SMALL_STATE(63)] = 1074,
  [SMALL_STATE(64)] = 1089,
  [SMALL_STATE(65)] = 1104,
  [SMALL_STATE(66)] = 1119,
  [SMALL_STATE(67)] = 1134,
};

static const TSParseActionEntry ts_parse_actions[] = {
  [0] = {.entry = {.count = 0, .reusable = false}},
  [1] = {.entry = {.count = 1, .reusable = false}}, RECOVER(),
  [3] = {.entry = {.count = 1, .reusable = true}}, SHIFT_EXTRA(),
  [5] = {.entry = {.count = 1, .reusable = true}}, REDUCE(sym_source_file, 0, 0, 0),
  [7] = {.entry = {.count = 1, .reusable = false}}, SHIFT(40),
  [9] = {.entry = {.count = 1, .reusable = true}}, SHIFT(15),
  [11] = {.entry = {.count = 1, .reusable = true}}, SHIFT(65),
  [13] = {.entry = {.count = 1, .reusable = true}}, SHIFT(66),
  [15] = {.entry = {.count = 1, .reusable = true}}, SHIFT(53),
  [17] = {.entry = {.count = 1, .reusable = true}}, SHIFT(39),
  [19] = {.entry = {.count = 1, .reusable = false}}, SHIFT(39),
  [21] = {.entry = {.count = 1, .reusable = true}}, SHIFT(11),
  [23] = {.entry = {.count = 1, .reusable = true}}, SHIFT(12),
  [25] = {.entry = {.count = 1, .reusable = true}}, SHIFT(13),
  [27] = {.entry = {.count = 1, .reusable = false}}, SHIFT(15),
  [29] = {.entry = {.count = 2, .reusable = false}}, REDUCE(aux_sym_source_file_repeat1, 2, 0, 0), SHIFT_REPEAT(32),
  [32] = {.entry = {.count = 2, .reusable = true}}, REDUCE(aux_sym_source_file_repeat1, 2, 0, 0), SHIFT_REPEAT(2),
  [35] = {.entry = {.count = 2, .reusable = true}}, REDUCE(aux_sym_source_file_repeat1, 2, 0, 0), SHIFT_REPEAT(59),
  [38] = {.entry = {.count = 2, .reusable = true}}, REDUCE(aux_sym_source_file_repeat1, 2, 0, 0), SHIFT_REPEAT(60),
  [41] = {.entry = {.count = 2, .reusable = true}}, REDUCE(aux_sym_source_file_repeat1, 2, 0, 0), SHIFT_REPEAT(51),
  [44] = {.entry = {.count = 1, .reusable = true}}, REDUCE(aux_sym_source_file_repeat1, 2, 0, 0),
  [46] = {.entry = {.count = 2, .reusable = true}}, REDUCE(aux_sym_source_file_repeat1, 2, 0, 0), SHIFT_REPEAT(20),
  [49] = {.entry = {.count = 2, .reusable = false}}, REDUCE(aux_sym_source_file_repeat1, 2, 0, 0), SHIFT_REPEAT(20),
  [52] = {.entry = {.count = 2, .reusable = true}}, REDUCE(aux_sym_source_file_repeat1, 2, 0, 0), SHIFT_REPEAT(14),
  [55] = {.entry = {.count = 2, .reusable = true}}, REDUCE(aux_sym_source_file_repeat1, 2, 0, 0), SHIFT_REPEAT(10),
  [58] = {.entry = {.count = 2, .reusable = true}}, REDUCE(aux_sym_source_file_repeat1, 2, 0, 0), SHIFT_REPEAT(9),
  [61] = {.entry = {.count = 2, .reusable = false}}, REDUCE(aux_sym_source_file_repeat1, 2, 0, 0), SHIFT_REPEAT(2),
  [64] = {.entry = {.count = 1, .reusable = false}}, SHIFT(32),
  [66] = {.entry = {.count = 1, .reusable = true}}, SHIFT(2),
  [68] = {.entry = {.count = 1, .reusable = true}}, SHIFT(59),
  [70] = {.entry = {.count = 1, .reusable = true}}, SHIFT(60),
  [72] = {.entry = {.count = 1, .reusable = true}}, SHIFT(51),
  [74] = {.entry = {.count = 1, .reusable = true}}, SHIFT(61),
  [76] = {.entry = {.count = 1, .reusable = true}}, SHIFT(20),
  [78] = {.entry = {.count = 1, .reusable = false}}, SHIFT(20),
  [80] = {.entry = {.count = 1, .reusable = true}}, SHIFT(14),
  [82] = {.entry = {.count = 1, .reusable = true}}, SHIFT(10),
  [84] = {.entry = {.count = 1, .reusable = true}}, SHIFT(9),
  [86] = {.entry = {.count = 1, .reusable = false}}, SHIFT(2),
  [88] = {.entry = {.count = 1, .reusable = true}}, SHIFT(3),
  [90] = {.entry = {.count = 1, .reusable = false}}, SHIFT(3),
  [92] = {.entry = {.count = 1, .reusable = true}}, SHIFT(36),
  [94] = {.entry = {.count = 1, .reusable = true}}, SHIFT(23),
  [96] = {.entry = {.count = 1, .reusable = true}}, SHIFT(33),
  [98] = {.entry = {.count = 1, .reusable = true}}, SHIFT(26),
  [100] = {.entry = {.count = 1, .reusable = true}}, SHIFT(6),
  [102] = {.entry = {.count = 1, .reusable = true}}, SHIFT(22),
  [104] = {.entry = {.count = 1, .reusable = false}}, SHIFT(6),
  [106] = {.entry = {.count = 1, .reusable = true}}, SHIFT(7),
  [108] = {.entry = {.count = 1, .reusable = true}}, SHIFT(24),
  [110] = {.entry = {.count = 1, .reusable = false}}, SHIFT(7),
  [112] = {.entry = {.count = 1, .reusable = true}}, SHIFT(5),
  [114] = {.entry = {.count = 1, .reusable = true}}, SHIFT(46),
  [116] = {.entry = {.count = 1, .reusable = false}}, SHIFT(5),
  [118] = {.entry = {.count = 1, .reusable = true}}, SHIFT(18),
  [120] = {.entry = {.count = 1, .reusable = true}}, SHIFT(35),
  [122] = {.entry = {.count = 1, .reusable = false}}, SHIFT(18),
  [124] = {.entry = {.count = 1, .reusable = true}}, SHIFT(17),
  [126] = {.entry = {.count = 1, .reusable = true}}, SHIFT(48),
  [128] = {.entry = {.count = 1, .reusable = false}}, SHIFT(17),
  [130] = {.entry = {.count = 1, .reusable = true}}, SHIFT(8),
  [132] = {.entry = {.count = 1, .reusable = true}}, SHIFT(27),
  [134] = {.entry = {.count = 1, .reusable = false}}, SHIFT(8),
  [136] = {.entry = {.count = 1, .reusable = true}}, REDUCE(sym_source_file, 1, 0, 0),
  [138] = {.entry = {.count = 1, .reusable = true}}, SHIFT(16),
  [140] = {.entry = {.count = 1, .reusable = false}}, SHIFT(16),
  [142] = {.entry = {.count = 2, .reusable = false}}, REDUCE(aux_sym_source_file_repeat1, 2, 0, 0), SHIFT_REPEAT(40),
  [145] = {.entry = {.count = 2, .reusable = true}}, REDUCE(aux_sym_source_file_repeat1, 2, 0, 0), SHIFT_REPEAT(16),
  [148] = {.entry = {.count = 2, .reusable = true}}, REDUCE(aux_sym_source_file_repeat1, 2, 0, 0), SHIFT_REPEAT(65),
  [151] = {.entry = {.count = 2, .reusable = true}}, REDUCE(aux_sym_source_file_repeat1, 2, 0, 0), SHIFT_REPEAT(66),
  [154] = {.entry = {.count = 2, .reusable = true}}, REDUCE(aux_sym_source_file_repeat1, 2, 0, 0), SHIFT_REPEAT(53),
  [157] = {.entry = {.count = 2, .reusable = true}}, REDUCE(aux_sym_source_file_repeat1, 2, 0, 0), SHIFT_REPEAT(39),
  [160] = {.entry = {.count = 2, .reusable = false}}, REDUCE(aux_sym_source_file_repeat1, 2, 0, 0), SHIFT_REPEAT(39),
  [163] = {.entry = {.count = 2, .reusable = true}}, REDUCE(aux_sym_source_file_repeat1, 2, 0, 0), SHIFT_REPEAT(11),
  [166] = {.entry = {.count = 2, .reusable = true}}, REDUCE(aux_sym_source_file_repeat1, 2, 0, 0), SHIFT_REPEAT(12),
  [169] = {.entry = {.count = 2, .reusable = true}}, REDUCE(aux_sym_source_file_repeat1, 2, 0, 0), SHIFT_REPEAT(13),
  [172] = {.entry = {.count = 2, .reusable = false}}, REDUCE(aux_sym_source_file_repeat1, 2, 0, 0), SHIFT_REPEAT(16),
  [175] = {.entry = {.count = 1, .reusable = true}}, SHIFT(38),
  [177] = {.entry = {.count = 1, .reusable = true}}, SHIFT(37),
  [179] = {.entry = {.count = 1, .reusable = false}}, REDUCE(sym__double_string, 3, 0, 0),
  [181] = {.entry = {.count = 1, .reusable = true}}, REDUCE(sym__double_string, 3, 0, 0),
  [183] = {.entry = {.count = 1, .reusable = false}}, REDUCE(sym_number, 1, 0, 0),
  [185] = {.entry = {.count = 1, .reusable = true}}, REDUCE(sym_number, 1, 0, 0),
  [187] = {.entry = {.count = 1, .reusable = false}}, REDUCE(sym__single_string, 3, 0, 0),
  [189] = {.entry = {.count = 1, .reusable = true}}, REDUCE(sym__single_string, 3, 0, 0),
  [191] = {.entry = {.count = 1, .reusable = false}}, REDUCE(sym_group, 2, 0, 0),
  [193] = {.entry = {.count = 1, .reusable = true}}, REDUCE(sym_group, 2, 0, 0),
  [195] = {.entry = {.count = 1, .reusable = false}}, REDUCE(sym_group, 3, 0, 0),
  [197] = {.entry = {.count = 1, .reusable = true}}, REDUCE(sym_group, 3, 0, 0),
  [199] = {.entry = {.count = 1, .reusable = false}}, REDUCE(sym_map, 2, 0, 0),
  [201] = {.entry = {.count = 1, .reusable = true}}, REDUCE(sym_map, 2, 0, 0),
  [203] = {.entry = {.count = 1, .reusable = false}}, REDUCE(sym_template_string, 3, 0, 0),
  [205] = {.entry = {.count = 1, .reusable = true}}, REDUCE(sym_template_string, 3, 0, 0),
  [207] = {.entry = {.count = 1, .reusable = false}}, REDUCE(sym_list, 3, 0, 0),
  [209] = {.entry = {.count = 1, .reusable = true}}, REDUCE(sym_list, 3, 0, 0),
  [211] = {.entry = {.count = 1, .reusable = false}}, REDUCE(sym_list, 2, 0, 0),
  [213] = {.entry = {.count = 1, .reusable = true}}, REDUCE(sym_list, 2, 0, 0),
  [215] = {.entry = {.count = 1, .reusable = false}}, REDUCE(sym_template_string, 2, 0, 0),
  [217] = {.entry = {.count = 1, .reusable = true}}, REDUCE(sym_template_string, 2, 0, 0),
  [219] = {.entry = {.count = 1, .reusable = false}}, REDUCE(sym__double_string, 2, 0, 0),
  [221] = {.entry = {.count = 1, .reusable = true}}, REDUCE(sym__double_string, 2, 0, 0),
  [223] = {.entry = {.count = 1, .reusable = false}}, REDUCE(sym__single_string, 2, 0, 0),
  [225] = {.entry = {.count = 1, .reusable = true}}, REDUCE(sym__single_string, 2, 0, 0),
  [227] = {.entry = {.count = 1, .reusable = false}}, REDUCE(sym_string, 1, 0, 0),
  [229] = {.entry = {.count = 1, .reusable = true}}, REDUCE(sym_string, 1, 0, 0),
  [231] = {.entry = {.count = 1, .reusable = false}}, REDUCE(sym_word, 1, 0, 0),
  [233] = {.entry = {.count = 1, .reusable = true}}, REDUCE(sym_word, 1, 0, 0),
  [235] = {.entry = {.count = 1, .reusable = false}}, REDUCE(sym_map, 3, 0, 0),
  [237] = {.entry = {.count = 1, .reusable = true}}, REDUCE(sym_map, 3, 0, 0),
  [239] = {.entry = {.count = 1, .reusable = false}}, SHIFT_EXTRA(),
  [241] = {.entry = {.count = 1, .reusable = true}}, SHIFT(50),
  [243] = {.entry = {.count = 1, .reusable = false}}, SHIFT(41),
  [245] = {.entry = {.count = 1, .reusable = false}}, SHIFT(50),
  [247] = {.entry = {.count = 1, .reusable = true}}, SHIFT(4),
  [249] = {.entry = {.count = 2, .reusable = true}}, REDUCE(aux_sym_template_string_repeat1, 2, 0, 0), SHIFT_REPEAT(50),
  [252] = {.entry = {.count = 1, .reusable = false}}, REDUCE(aux_sym_template_string_repeat1, 2, 0, 0),
  [254] = {.entry = {.count = 2, .reusable = false}}, REDUCE(aux_sym_template_string_repeat1, 2, 0, 0), SHIFT_REPEAT(50),
  [257] = {.entry = {.count = 2, .reusable = true}}, REDUCE(aux_sym_template_string_repeat1, 2, 0, 0), SHIFT_REPEAT(4),
  [260] = {.entry = {.count = 1, .reusable = true}}, SHIFT(52),
  [262] = {.entry = {.count = 1, .reusable = false}}, SHIFT(28),
  [264] = {.entry = {.count = 1, .reusable = false}}, SHIFT(52),
  [266] = {.entry = {.count = 1, .reusable = false}}, SHIFT(25),
  [268] = {.entry = {.count = 1, .reusable = true}}, SHIFT(49),
  [270] = {.entry = {.count = 1, .reusable = false}}, SHIFT(34),
  [272] = {.entry = {.count = 1, .reusable = false}}, SHIFT(49),
  [274] = {.entry = {.count = 2, .reusable = true}}, REDUCE(aux_sym__single_string_repeat1, 2, 0, 0), SHIFT_REPEAT(54),
  [277] = {.entry = {.count = 1, .reusable = false}}, REDUCE(aux_sym__single_string_repeat1, 2, 0, 0),
  [279] = {.entry = {.count = 1, .reusable = true}}, SHIFT(58),
  [281] = {.entry = {.count = 1, .reusable = false}}, SHIFT(45),
  [283] = {.entry = {.count = 1, .reusable = true}}, REDUCE(sym_interpolation, 3, 0, 0),
  [285] = {.entry = {.count = 1, .reusable = false}}, REDUCE(sym_interpolation, 3, 0, 0),
  [287] = {.entry = {.count = 1, .reusable = true}}, SHIFT(54),
  [289] = {.entry = {.count = 1, .reusable = false}}, SHIFT(47),
  [291] = {.entry = {.count = 2, .reusable = true}}, REDUCE(aux_sym__double_string_repeat1, 2, 0, 0), SHIFT_REPEAT(58),
  [294] = {.entry = {.count = 1, .reusable = false}}, REDUCE(aux_sym__double_string_repeat1, 2, 0, 0),
  [296] = {.entry = {.count = 1, .reusable = true}}, SHIFT(63),
  [298] = {.entry = {.count = 1, .reusable = false}}, SHIFT(30),
  [300] = {.entry = {.count = 1, .reusable = true}}, SHIFT(64),
  [302] = {.entry = {.count = 1, .reusable = false}}, SHIFT(29),
  [304] = {.entry = {.count = 1, .reusable = true}}, REDUCE(sym_interpolation_end, 1, 0, 0),
  [306] = {.entry = {.count = 1, .reusable = false}}, REDUCE(sym_interpolation_end, 1, 0, 0),
  [308] = {.entry = {.count = 1, .reusable = true}}, REDUCE(sym_interpolation, 2, 0, 0),
  [310] = {.entry = {.count = 1, .reusable = false}}, REDUCE(sym_interpolation, 2, 0, 0),
  [312] = {.entry = {.count = 1, .reusable = false}}, SHIFT(21),
  [314] = {.entry = {.count = 1, .reusable = false}}, SHIFT(19),
  [316] = {.entry = {.count = 1, .reusable = true}}, SHIFT(57),
  [318] = {.entry = {.count = 1, .reusable = false}}, SHIFT(43),
  [320] = {.entry = {.count = 1, .reusable = true}}, SHIFT(55),
  [322] = {.entry = {.count = 1, .reusable = false}}, SHIFT(44),
  [324] = {.entry = {.count = 1, .reusable = true}},  ACCEPT_INPUT(),
};

#ifdef __cplusplus
extern "C" {
#endif
#ifdef TREE_SITTER_HIDE_SYMBOLS
#define TS_PUBLIC
#elif defined(_WIN32)
#define TS_PUBLIC __declspec(dllexport)
#else
#define TS_PUBLIC __attribute__((visibility("default")))
#endif

TS_PUBLIC const TSLanguage *tree_sitter_boru(void) {
  static const TSLanguage language = {
    .version = LANGUAGE_VERSION,
    .symbol_count = SYMBOL_COUNT,
    .alias_count = ALIAS_COUNT,
    .token_count = TOKEN_COUNT,
    .external_token_count = EXTERNAL_TOKEN_COUNT,
    .state_count = STATE_COUNT,
    .large_state_count = LARGE_STATE_COUNT,
    .production_id_count = PRODUCTION_ID_COUNT,
    .field_count = FIELD_COUNT,
    .max_alias_sequence_length = MAX_ALIAS_SEQUENCE_LENGTH,
    .parse_table = &ts_parse_table[0][0],
    .small_parse_table = ts_small_parse_table,
    .small_parse_table_map = ts_small_parse_table_map,
    .parse_actions = ts_parse_actions,
    .symbol_names = ts_symbol_names,
    .symbol_metadata = ts_symbol_metadata,
    .public_symbol_map = ts_symbol_map,
    .alias_map = ts_non_terminal_alias_map,
    .alias_sequences = &ts_alias_sequences[0][0],
    .lex_modes = ts_lex_modes,
    .lex_fn = ts_lex,
    .keyword_lex_fn = ts_lex_keywords,
    .keyword_capture_token = sym__word_token,
    .primary_state_ids = ts_primary_state_ids,
  };
  return &language;
}
#ifdef __cplusplus
}
#endif
