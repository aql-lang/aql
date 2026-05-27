;;; lsp-mode.el --- AQL Language Server via lsp-mode   -*- lexical-binding: t -*-

;; Drop the snippet below into your init.el. Requires lsp-mode
;; installed from MELPA (https://github.com/emacs-lsp/lsp-mode).

;; Define an aql-mode if you don't already have one.
(define-derived-mode aql-mode prog-mode "AQL"
  "Major mode for editing AQL source files."
  (setq-local comment-start "#")
  (setq-local comment-end ""))

(add-to-list 'auto-mode-alist '("\\.aql\\'" . aql-mode))

;; Register the aql LSP server.
(with-eval-after-load 'lsp-mode
  (add-to-list 'lsp-language-id-configuration '(aql-mode . "aql"))
  (lsp-register-client
   (make-lsp-client
    :new-connection (lsp-stdio-connection '("aql" "lsp"))
    :activation-fn (lsp-activate-on "aql")
    :server-id 'aql-lsp)))

;; Start lsp-mode automatically in aql-mode buffers.
(add-hook 'aql-mode-hook #'lsp-deferred)

;; Optional: tighter formatter integration when lsp-mode is active.
;; (add-hook 'aql-mode-hook (lambda ()
;;   (add-hook 'before-save-hook #'lsp-format-buffer nil t)))

(provide 'aql-lsp-mode)
;;; lsp-mode.el ends here
