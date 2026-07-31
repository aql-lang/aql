# BORU for Nova

Language support for [BORU](https://github.com/boru-lang/boru) — a
concatenative, strongly-typed query language — inside
[Nova](https://nova.app) (Panic's macOS editor).

## Features

- **Syntax highlighting** for `.boru` files:
  - line comments (`#`, `//`) and block comments (`/* … */`);
  - all three string kinds — single-quoted, double-quoted and backtick
    template strings — with `\n \t \\ \' \" \`` and `\uXXXX` escapes and
    `${ … }` interpolation scopes highlighted inside template strings;
  - numbers (decimal integers with `_` separators, floats/exponents,
    `0x` hex, `0b` binary, `0d` big-integer literals);
  - language constants (`true false none inf nan`, and `-inf`/`-nan`);
  - control/declaration/query keywords and built-in library words;
  - capitalised type names and module namespaces (`Integer`, `Emailon`,
    `MathUtil`, …);
  - operators and `/modifier` dispatch suffixes (`/q /s /f /r /t /u`,
    combinations, and `/<digit>` arity forms);
  - `def Foo` / `def foo` binding targets highlighted as a type vs a
    value/function respectively.
- **Language-server integration** via the bundled `boru lsp` stdio server:
  diagnostics, hover, completion and formatting (whatever the server
  advertises).
- Bracket matching, auto-closing pairs and bracket-nesting indentation.

## Requirements

The `boru` command-line tool. Either put it on your `$PATH` or set an
absolute path in the extension preferences (see below). Build it from
this repository with `make build`, or install a released binary.

## Install

This extension is distributed as a self-contained `.novaextension`
bundle.

- **Double-click** `boru.novaextension` in Finder — Nova offers to
  install it.
- **or**, for local development, open Nova ▸ **Extensions ▸ Extension
  Library…**, then choose **Activate Project as Extension** (or drag the
  `boru.novaextension` folder onto Nova) to sideload it from this
  directory without packaging.

Once installed, open any `.boru` file (for example
`design/examples/todo/todo.boru`) — Nova selects the BORU syntax
automatically and, if `boru` is available, starts the language server.

## Configuration

Open **Extensions ▸ BORU ▸ Preferences** (or the project-scoped settings)
to configure:

- **BORU binary path** (`boru.lsp.path`, default `boru`) — the `boru`
  executable used to run `boru lsp`. Leave as `boru` to use the binary on
  your `$PATH`, or give an absolute path. A leading `~` is expanded to
  your home directory.
- **Enable language server** (`boru.lsp.enabled`, default `on`) — turn
  the `boru lsp` server on or off. Disable it for syntax highlighting
  only. Changing either setting restarts the client automatically.

## Notes

Nova compiles its own tree-sitter-style XML grammars; this extension
maps BORU tokens onto Nova's standard scope names (`comment`, `string`,
`string.escape`, `value.number.*`, `keyword`, `identifier.type`,
`identifier.function`, `operator`, …) so highlighting follows whichever
Nova theme you use.
