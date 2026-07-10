# AQL Language Server — editor integration

`aql lsp` is the AQL Language Server Protocol implementation. This
directory ships reference configuration for the most common editors —
copy the relevant snippet into your editor's config and adjust the
binary path if `aql` is not on your PATH.

## What the server provides

| Capability    | Backed by                                |
| ------------- | ---------------------------------------- |
| Diagnostics   | `lang.Check` (errors + warnings)         |
| Hover         | `help.FormatDynamic` / `help.Format`     |
| Completion    | `help.Words` (all registered words)      |
| Formatting    | `formatter.Format` (whole-buffer)        |

## Transports

- **stdio** (default) — every editor below uses this.
- **TCP** — `aql lsp -p <port>` for debugging or remote-attach.

## Files

| File                  | Editor / Plugin                                  |
| --------------------- | ------------------------------------------------ |
| `vscode/`             | VS Code extension (minimal client)               |
| `neovim.lua`          | Neovim — `vim.lsp.config` (0.11+) and lspconfig  |
| `vim-lsp.vim`         | classic Vim — `prabirshrestha/vim-lsp`           |
| `coc-settings.json`   | classic Vim / Neovim — `coc.nvim`                |
| `eglot.el`            | Emacs 29+ — built-in `eglot`                     |
| `lsp-mode.el`         | Emacs — `emacs-lsp/lsp-mode`                     |
| `helix.toml`          | Helix — `languages.toml` snippet                 |
| `sublime.json`        | Sublime Text — `LSP` package settings            |
| `zed.json`            | Zed — `settings.json` snippet                    |
| `kate.json`           | Kate — `lspclient/settings.json`                 |

## Prerequisites

Install the `aql` binary:

```sh
go install github.com/aql-lang/aql/cmd/go/aql@latest
```

Confirm the server starts:

```sh
echo '' | aql lsp        # waits for input on stdio; Ctrl-D to exit
aql lsp -p 9999          # listens on TCP :9999
```

## File association

Every snippet binds the server to files matched by `*.aql`. If you
use a different extension, change the language ID / glob accordingly.
