# AQL editor support

Everything for editing AQL (`.aql`) in your editor lives here. There are
two, complementary layers — most editors can use both:

1. **Syntax grammars** — colourise AQL with no server or network. One of
   these drives almost every editor and code-renderer in existence.
2. **Language-server clients** — richer help via `aql lsp` (the
   AQL Language Server): live diagnostics, hover docs, completion, and
   whole-buffer formatting.

## Syntax grammars (highlighting)

| Grammar                       | Powers                                                                 |
| ----------------------------- | ---------------------------------------------------------------------- |
| [`textmate/`](textmate/)      | VS Code, Sublime Text, TextMate, BBEdit — any TextMate-grammar host    |
| [`tree-sitter/`](tree-sitter/)| Neovim, Helix, Zed, Emacs `treesit`, GitHub Linguist                   |
| [`codemirror/`](codemirror/)  | CodeMirror 6 — the web playground and browser editors                  |
| [`pygments/`](pygments/)      | Pygments / Chroma — Sphinx, MkDocs, `pygmentize`, static code blocks   |
| [`emacs/`](emacs/)            | Emacs — a hand-written `aql-mode` font-lock (bundled with the mode)    |

The TextMate grammar (`textmate/aql.tmLanguage.json`, scope `source.aql`)
is the single source of truth for TextMate-based hosts; a byte-identical
copy lives in `vscode/syntaxes/` because VS Code loads grammars from
inside the extension.

## GitHub

[`linguist/`](linguist/) has real `.aql` samples, the `languages.yml`
entry, and the submission checklist so GitHub highlights and classifies
`.aql` (plus a `.gitattributes` snippet for immediate local classification
before the upstream change lands).

## Language-server clients (`aql lsp`)

`aql lsp` speaks stdio by default (every client below); `aql lsp -p <port>`
serves TCP for debugging/remote-attach. It provides:

| Capability  | Backed by                            |
| ----------- | ------------------------------------ |
| Diagnostics | `lang.Check` (errors + warnings)     |
| Hover       | `help.FormatDynamic` / `help.Format` |
| Completion  | `help.Words` (all registered words)  |
| Formatting  | `formatter.Format` (whole buffer)    |

| Editor                 | Where                                                 |
| ---------------------- | ----------------------------------------------------- |
| Emacs                  | [`emacs/`](emacs/) — full `aql-mode` (eglot + lsp-mode) |
| VS Code                | [`vscode/`](vscode/) — extension (grammar + LSP client) |
| Neovim                 | [`neovim.lua`](neovim.lua) — `vim.lsp` / lspconfig    |
| Vim (classic)          | [`vim-lsp.vim`](vim-lsp.vim) — `prabirshrestha/vim-lsp` |
| Vim / Neovim (coc)     | [`coc-settings.json`](coc-settings.json)              |
| Helix                  | [`helix.toml`](helix.toml)                            |
| Zed                    | [`zed.json`](zed.json)                                |
| Sublime Text           | [`sublime.json`](sublime.json) — `LSP` package        |
| Kate                   | [`kate.json`](kate.json)                              |
| JetBrains (IntelliJ/GoLand/…) | [`jetbrains/`](jetbrains/) — via LSP4IJ        |
| BBEdit                 | [`bbedit/`](bbedit/) — CLM syntax + LSP               |
| Nova                   | [`nova/`](nova/) — extension (syntax + LSP client)    |

## Prerequisites

Install the `aql` binary (needed only for the language-server features —
the grammars work on their own):

```sh
go install github.com/aql-lang/aql/cmd/go/aql@latest
# ...or from a checkout:  cd cmd/go && make build
```

Confirm the server starts:

```sh
echo '' | aql lsp        # waits for input on stdio; Ctrl-D to exit
aql lsp -p 9999          # listens on TCP :9999
```

## File association

Everything here binds to `*.aql` (language id `aql`, TextMate scope
`source.aql`). If you use a different extension, adjust the glob / language
id accordingly.
