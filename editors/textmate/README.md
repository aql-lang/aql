# AQL TextMate grammar

`aql.tmLanguage.json` is the **canonical TextMate grammar** for AQL — a
concatenative, strongly-typed query language (`.aql`). It defines the scope
name `source.aql` and highlights comments, the three string kinds (single,
double, and backtick template strings with `${…}` interpolation), every
number form (`0x` hex, `0b` binary, `0d` big-integer, floats, and
underscore-separated integers), the language constants (`true false none
inf nan`), control/declaration/query keywords, built-in library words,
capitalised type / module names, `def` binding targets, and the `/modifier`
dispatch suffixes.

The grammar uses conventional TextMate scope names (`comment.*`,
`string.*`, `constant.*`, `keyword.control`, `support.function`,
`entity.name.type`, `keyword.operator.*`) so that standard colour themes
render AQL without any theme-specific tweaks.

## Source of truth

**`editors/textmate/aql.tmLanguage.json` is the single source of truth.**
It is copied verbatim into the VS Code extension at
[`editors/vscode/syntaxes/aql.tmLanguage.json`](../vscode/syntaxes/aql.tmLanguage.json),
because VS Code loads grammars only from inside the extension directory.
The two files are byte-for-byte identical — after editing the canonical
copy, re-sync and re-validate:

```sh
cp editors/textmate/aql.tmLanguage.json editors/vscode/syntaxes/aql.tmLanguage.json
jq . editors/textmate/aql.tmLanguage.json          >/dev/null
jq . editors/vscode/syntaxes/aql.tmLanguage.json   >/dev/null
diff editors/textmate/aql.tmLanguage.json editors/vscode/syntaxes/aql.tmLanguage.json
```

The lexical rules mirror the reference Emacs mode
([`editors/emacs/aql-mode.el`](../emacs/aql-mode.el)); the authoritative word
lists come straight from the tool via `aql describe`.

## Reusing the grammar

Any editor or tool that consumes a TextMate grammar can reuse this one file.

### TextMate (macOS)

TextMate reads `.tmLanguage` bundles. Point it at the JSON grammar (TextMate 2
accepts JSON grammars directly), or convert to the classic plist form:

```sh
# JSON -> plist, if your TextMate build wants the old format
plutil -convert xml1 -o aql.tmLanguage aql.tmLanguage.json
```

Drop the result into a bundle's `Syntaxes/` folder
(`~/Library/Application Support/TextMate/Bundles/AQL.tmbundle/Syntaxes/`).

### Sublime Text

Sublime Text loads `.tmLanguage` / `.sublime-syntax` grammars. The simplest
path is to save this grammar under a `Packages/AQL/` directory:

```sh
# ~/Library/Application Support/Sublime Text/Packages/AQL/   (macOS)
# %APPDATA%\Sublime Text\Packages\AQL\                       (Windows)
cp aql.tmLanguage.json "…/Packages/AQL/AQL.tmLanguage.json"
```

Sublime maps `fileTypes: ["aql"]` / `scopeName: "source.aql"` automatically,
so `*.aql` files pick up highlighting. Pair it with the LSP package (see
[`editors/sublime.json`](../sublime.json)) for `aql lsp` diagnostics,
hover, and completion on top of the syntax colouring.

### BBEdit (macOS)

BBEdit reads `.tmLanguage`/`.plist` grammars from its Language Modules
folder. Convert as for TextMate above and copy the result into:

```
~/Library/Application Support/BBEdit/Language Modules/
```

BBEdit associates the module with `*.aql` via the grammar's `fileTypes`.

### Any other TextMate-grammar consumer

The same file drives every editor built on the TextMate grammar model —
including anything backed by the [`vscode-textmate`](https://github.com/microsoft/vscode-textmate)
tokenizer (Monaco, Shiki-based highlighters, static-site code renderers,
GitHub Linguist-style pipelines, etc.). Register it under the scope name
`source.aql` and the file type `aql`.

## Validating a change

```sh
jq . aql.tmLanguage.json        # structural JSON check
```

The regexes are written for the Oniguruma engine used by TextMate,
Sublime, and `vscode-textmate`; they rely only on portable features
(word boundaries, lookbehind/lookahead, alternation) that are also valid
under PCRE-style engines.
