; injections.scm — language injections for AQL.
;
; AQL has no first-class embedded foreign languages in its core syntax, so this
; file is intentionally minimal. The `${ … }` interpolation inside a template
; string is itself AQL (handled by the grammar's own expression rules), so no
; injection is needed there.
;
; If you maintain domain-specific string DSLs (e.g. SQL in a query string) you
; can add rules here, for example:
;
;   ((string) @injection.content
;    (#match? @injection.content "^\"SELECT ")
;    (#set! injection.language "sql"))
