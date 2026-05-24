;;; eglot.el --- AQL Language Server via eglot   -*- lexical-binding: t -*-

;; Drop the snippet below into your init.el. eglot ships with Emacs
;; 29+; for earlier versions install it from MELPA.

;; Define an aql-mode if you don't already have one. fundamental-mode
;; works in a pinch, but a dedicated mode lets eglot bind only inside
;; .aql buffers.
(define-derived-mode aql-mode prog-mode "AQL"
  "Major mode for editing AQL source files."
  (setq-local comment-start "#")
  (setq-local comment-end ""))

(add-to-list 'auto-mode-alist '("\\.aql\\'" . aql-mode))

;; Tell eglot how to spawn the server. Server is launched on demand
;; when the first aql-mode buffer is visited or M-x eglot is invoked.
(with-eval-after-load 'eglot
  (add-to-list 'eglot-server-programs
               '(aql-mode . ("aql" "lsp"))))

;; Auto-start eglot in every aql-mode buffer.
(add-hook 'aql-mode-hook #'eglot-ensure)

;; Optional: bind LSP actions to convenient keys inside aql-mode.
;; (define-key aql-mode-map (kbd "C-c C-f") #'eglot-format-buffer)
;; (define-key aql-mode-map (kbd "C-c C-h") #'eldoc)

(provide 'aql-eglot)
;;; eglot.el ends here
