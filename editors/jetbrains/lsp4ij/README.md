# LSP4IJ template files for BORU

These files describe the **user-defined language server** that talks to
`boru lsp`. LSP4IJ (the Red Hat "LSP4IJ" plugin for IntelliJ-based IDEs)
lets you create such a server entirely from its UI — there is **no
single dotfile** the IDE reads on startup. The files here therefore serve
two purposes:

1. **As a copy/paste reference** — every value maps 1:1 to a field in the
   "New Language Server" dialog (see the parent
   [`../README.md`](../README.md) for the click-by-click walkthrough).
2. **As a template you can import.** LSP4IJ can export the servers you
   configure as a *template folder* and import one back via
   **New Language Server ▸ Template ▸ Import from custom template…**.
   A template folder is exactly this shape:

   ```
   BORU/
     template.json               # name + command + file/language mappings
     settings.json               # per-feature toggles (optional)
     initializationOptions.json  # LSP `initialize.initializationOptions` (optional)
   ```

   Point the importer at the folder that contains these files.

> **Accuracy note.** LSP4IJ's template-folder layout is stable across
> recent releases, but the plugin is developed independently of BORU and
> its JSON schema is not part of BORU's compatibility surface. If your
> LSP4IJ version rejects an import, fall back to the UI steps in the
> parent README — those always work. The only field that truly matters is
> the **command**, `boru lsp`, and the **`*.boru`** mapping.

## Files

| File                         | Dialog field                                            |
| ---------------------------- | ------------------------------------------------------- |
| `template.json`              | Server **Name**, **Command**, **Mappings** tab          |
| `settings.json`              | Per-server feature check-boxes (Configuration tab)      |
| `initializationOptions.json` | Sent as `initialize.initializationOptions` (empty here) |

## Adjusting the command

`template.json` uses the bare command `boru lsp`, which resolves `boru`
from your PATH. To pin an explicit binary, replace the `programArgs`
value with an absolute path, quoting if it contains spaces:

```json
"programArgs": { "default": "/usr/local/bin/boru lsp" }
```

```json
"programArgs": { "windows": "\"C:\\Program Files\\boru\\boru.exe\" lsp" }
```

`boru lsp` speaks LSP over **stdio**, which is what LSP4IJ's user-defined
servers use. (`boru lsp -p <port>` serves the same protocol over TCP for
remote-attach, but LSP4IJ spawns a process, so stdio is the right choice.)
