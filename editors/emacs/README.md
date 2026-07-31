# boru for Emacs

`boru-mode.el` is a self-contained major mode for editing boru source
(`.boru`). It provides syntax highlighting, bracket-aware indentation,
comment/`imenu` support, and one-line integration with the bundled
`boru lsp` language server for **both** `lsp-mode` and `eglot`
(diagnostics, hover, completion, whole-buffer formatting).

## Install

The file is a normal Emacs package — put it on your `load-path` and
`require` it.

**Manual**

```elisp
(add-to-list 'load-path "/path/to/boru/editors/emacs")
(require 'boru-mode)
```

**`use-package` + `straight.el` / vc**

```elisp
;; straight.el
(use-package boru-mode
  :straight (boru-mode :type git :host github :repo "boru-lang/boru"
                      :files ("editors/emacs/boru-mode.el")))

;; built-in package-vc (Emacs 29+)
(use-package boru-mode
  :vc (:url "https://github.com/boru-lang/boru" :lisp-dir "editors/emacs"))
```

Opening any `.boru` file then selects `boru-mode` automatically.

## Enable the language server (recommended)

`boru-mode` already registers the `boru lsp` server with whichever client
you use; you just choose one and tell it to start. The server is found
on your `PATH` (build it with `cd cmd/go && make build`, or
`go install github.com/boru-lang/boru/cmd/go/boru@latest`).

**Emacs 29+ built-in eglot**

```elisp
(add-hook 'boru-mode-hook #'eglot-ensure)
```

**lsp-mode (from MELPA)**

```elisp
(add-hook 'boru-mode-hook #'lsp-deferred)
```

That's it — you get inline error/warning diagnostics as you type, hover
docs (`boru describe`-backed), completion of every registered word, and
`M-x eglot-format-buffer` / `lsp-format-buffer` (which run
`formatter.Format` over the whole buffer). To format on save:

```elisp
;; eglot
(add-hook 'boru-mode-hook
          (lambda () (add-hook 'before-save-hook #'eglot-format-buffer nil t)))
```

## Customisation

| Variable            | Default          | Meaning                                        |
| ------------------- | ---------------- | ---------------------------------------------- |
| `boru-indent-offset` | `2`              | Columns of indent per bracket nesting level.   |
| `boru-lsp-command`   | `("boru" "lsp")`  | Program + args that start the language server. |

If `boru` is not on your `PATH`, point the command at the binary:

```elisp
(setq boru-lsp-command '("/Users/me/go/bin/boru" "lsp"))
```

## Notes

- The mode is standalone: syntax highlighting and indentation work
  without any language client installed.
- The server is single-client (one connection per buffer session, per
  the LSP convention) and evaluates the buffer's static checker — no code
  is executed.
- For the raw client snippets used by other editors, or a TCP transport
  (`boru lsp -p <port>`), see the parent [`editors/`](../README.md)
  directory.
