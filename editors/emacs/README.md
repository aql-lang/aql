# AQL for Emacs

`aql-mode.el` is a self-contained major mode for editing AQL source
(`.aql`). It provides syntax highlighting, bracket-aware indentation,
comment/`imenu` support, and one-line integration with the bundled
`aql lsp` language server for **both** `lsp-mode` and `eglot`
(diagnostics, hover, completion, whole-buffer formatting).

## Install

The file is a normal Emacs package — put it on your `load-path` and
`require` it.

**Manual**

```elisp
(add-to-list 'load-path "/path/to/aql/editors/emacs")
(require 'aql-mode)
```

**`use-package` + `straight.el` / vc**

```elisp
;; straight.el
(use-package aql-mode
  :straight (aql-mode :type git :host github :repo "aql-lang/aql"
                      :files ("editors/emacs/aql-mode.el")))

;; built-in package-vc (Emacs 29+)
(use-package aql-mode
  :vc (:url "https://github.com/aql-lang/aql" :lisp-dir "editors/emacs"))
```

Opening any `.aql` file then selects `aql-mode` automatically.

## Enable the language server (recommended)

`aql-mode` already registers the `aql lsp` server with whichever client
you use; you just choose one and tell it to start. The server is found
on your `PATH` (build it with `cd cmd/go && make build`, or
`go install github.com/aql-lang/aql/cmd/go/aql@latest`).

**Emacs 29+ built-in eglot**

```elisp
(add-hook 'aql-mode-hook #'eglot-ensure)
```

**lsp-mode (from MELPA)**

```elisp
(add-hook 'aql-mode-hook #'lsp-deferred)
```

That's it — you get inline error/warning diagnostics as you type, hover
docs (`aql describe`-backed), completion of every registered word, and
`M-x eglot-format-buffer` / `lsp-format-buffer` (which run
`formatter.Format` over the whole buffer). To format on save:

```elisp
;; eglot
(add-hook 'aql-mode-hook
          (lambda () (add-hook 'before-save-hook #'eglot-format-buffer nil t)))
```

## Customisation

| Variable            | Default          | Meaning                                        |
| ------------------- | ---------------- | ---------------------------------------------- |
| `aql-indent-offset` | `2`              | Columns of indent per bracket nesting level.   |
| `aql-lsp-command`   | `("aql" "lsp")`  | Program + args that start the language server. |

If `aql` is not on your `PATH`, point the command at the binary:

```elisp
(setq aql-lsp-command '("/Users/me/go/bin/aql" "lsp"))
```

## Notes

- The mode is standalone: syntax highlighting and indentation work
  without any language client installed.
- The server is single-client (one connection per buffer session, per
  the LSP convention) and evaluates the buffer's static checker — no code
  is executed.
- For the raw client snippets used by other editors, or a TCP transport
  (`aql lsp -p <port>`), see the parent [`editors/`](../README.md)
  directory.
