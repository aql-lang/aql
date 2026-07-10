# AQL for Nova

Language support for [AQL](https://github.com/aql-lang/aql) — a
concatenative, strongly-typed query language — inside
[Nova](https://nova.app) (Panic's macOS editor).

## Features

- **Syntax highlighting** for `.aql` files:
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
- **Language-server integration** via the bundled `aql lsp` stdio server:
  diagnostics, hover, completion and formatting (whatever the server
  advertises).
- Bracket matching, auto-closing pairs and bracket-nesting indentation.

## Requirements

The `aql` command-line tool. Either put it on your `$PATH` or set an
absolute path in the extension preferences (see below). Build it from
this repository with `make build`, or install a released binary.

## Install

This extension is distributed as a self-contained `.novaextension`
bundle.

- **Double-click** `aql.novaextension` in Finder — Nova offers to
  install it.
- **or**, for local development, open Nova ▸ **Extensions ▸ Extension
  Library…**, then choose **Activate Project as Extension** (or drag the
  `aql.novaextension` folder onto Nova) to sideload it from this
  directory without packaging.

Once installed, open any `.aql` file (for example
`design/examples/todo/todo.aql`) — Nova selects the AQL syntax
automatically and, if `aql` is available, starts the language server.

## Configuration

Open **Extensions ▸ AQL ▸ Preferences** (or the project-scoped settings)
to configure:

- **AQL binary path** (`aql.lsp.path`, default `aql`) — the `aql`
  executable used to run `aql lsp`. Leave as `aql` to use the binary on
  your `$PATH`, or give an absolute path. A leading `~` is expanded to
  your home directory.
- **Enable language server** (`aql.lsp.enabled`, default `on`) — turn
  the `aql lsp` server on or off. Disable it for syntax highlighting
  only. Changing either setting restarts the client automatically.

## Notes

Nova compiles its own tree-sitter-style XML grammars; this extension
maps AQL tokens onto Nova's standard scope names (`comment`, `string`,
`string.escape`, `value.number.*`, `keyword`, `identifier.type`,
`identifier.function`, `operator`, …) so highlighting follows whichever
Nova theme you use.
