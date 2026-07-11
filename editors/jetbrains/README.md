# AQL in JetBrains IDEs (IntelliJ IDEA, GoLand, PyCharm, WebStorm, …)

The lightest way to get AQL language support in any JetBrains IDE is the
**LSP4IJ** plugin, which lets you register `aql lsp` as a *user-defined
language server* without writing a custom plugin. You get live
diagnostics, hover docs, word completion, and whole-buffer formatting —
all served by the same `aql lsp` binary every other editor uses.

This directory contains:

- this `README.md` — the full walkthrough (LSP4IJ path + alternatives);
- [`lsp4ij/`](lsp4ij/) — LSP4IJ template files you can import instead of
  typing the dialog in by hand, plus a field-by-field mapping.

> LSP4IJ works in every IntelliJ-platform IDE: IntelliJ IDEA (Community
> or Ultimate), GoLand, PyCharm, WebStorm, PhpStorm, RubyMine, CLion,
> Rider, DataGrip, and Android Studio. The steps below are identical
> across all of them; only the product name in the title bar differs.

---

## What you get

The `aql lsp` server advertises exactly four capabilities. LSP4IJ maps
each to a native IDE feature:

| AQL server capability | Backed by                     | In the IDE                          |
| --------------------- | ----------------------------- | ----------------------------------- |
| Diagnostics           | `lang.Check` (errors + warns) | Red/yellow squiggles, Problems view |
| Hover                 | `help` word docs              | Hover popups (Ctrl/⌘ + hover)       |
| Completion            | all registered words          | `Ctrl+Space` completion             |
| Formatting            | `formatter.Format`            | **Reformat Code** (`Ctrl/⌘+Alt+L`)  |

There is **no** signature help, go-to-definition, rename, document
outline, or code actions — the server does not advertise them, so those
IDE features stay dark for `.aql` files. That is expected.

---

## 0. Prerequisites

Install the `aql` binary and make sure it is on your PATH:

```sh
go install github.com/aql-lang/aql/cmd/go/aql@latest
aql lsp -h        # should print the lsp usage; Ctrl-C to exit
```

If you don't want `aql` on your PATH, note its absolute path — you'll
plug it into the server **command** in step 3 (see
[Using a non-PATH binary](#using-a-non-path-aql-binary)).

---

## 1. Install the LSP4IJ plugin

1. Open **Settings/Preferences** (`Ctrl+Alt+S` / `⌘,`).
2. Go to **Plugins**.
3. Select the **Marketplace** tab and search for **`LSP4IJ`**
   (publisher: *Red Hat*).
4. Click **Install**, then **Restart IDE** when prompted.

A new **LSP4IJ** tool window (bottom-left) appears after restart; you'll
use it later to watch server status and traces.

---

## 2. (Recommended) Register an "AQL" file type

LSP4IJ can bind a server to a raw glob, but giving `.aql` a real IDE file
type also buys you bracket matching, comment toggling (`Ctrl/⌘+/`), and a
stable *language id* to map the server to.

1. **Settings ▸ Editor ▸ File Types**.
2. Under **Recognized File Types**, click **+** to add a new type:
   - **Name:** `AQL`
   - **Description:** `AQL query language`
   - **Line comment:** `#`   *(AQL also accepts `//`; the IDE allows one
     line-comment token here — pick `#`)*
   - **Block comment start:** `/*`   **Block comment end:** `*/`
   - Tick **Support paired braces**, **Support paired brackets**, and
     **Support paired parens** (AQL uses `()`, `[]`, and `{}`).
3. In the **File name patterns** panel for the new type, click **+** and
   add `*.aql`.
4. Click **OK**.

This gives you basic editing niceties even before the server attaches.
For real syntax **colours** without the LSP, see
[Colours via TextMate](#colours-via-a-textmate-bundle) below — the two
approaches compose.

---

## 3. Add `aql lsp` as a language server

You can do this by hand (3a) or by importing the template in
[`lsp4ij/`](lsp4ij/) (3b). Both produce the same result.

### 3a. By hand (authoritative — always works)

1. **Settings ▸ Languages & Frameworks ▸ Language Servers**.
2. Click **+** (Add) ▸ this opens **New Language Server**.
3. On the **Server** tab:
   - **Name:** `AQL`
   - **Command:** `aql lsp`
     *(On Windows, or to pin a specific build, use the full path — see
     [Using a non-PATH binary](#using-a-non-path-aql-binary).)*
4. On the **Mappings** tab, add **one** mapping so the server attaches to
   `.aql` files. Use whichever row matches how you set things up:
   - **File name patterns** ▸ **+** ▸ pattern `*.aql`,
     **Language id** `aql`; **or**
   - **Language** ▸ **+** ▸ select the `AQL` file type from step 2,
     **Language id** `aql`.

   The **language id** should be `aql` (lower-case). This is the value the
   server receives in `textDocument/languageId`; the AQL server ignores
   it, but keeping it `aql` matches every other editor config in the
   repo.
5. *(Optional)* On the **Configuration** tab you can paste the contents
   of [`lsp4ij/settings.json`](lsp4ij/settings.json) to make the enabled
   features explicit, and [`lsp4ij/initializationOptions.json`](lsp4ij/initializationOptions.json)
   (an empty `{}`) as the initialization options. LSP4IJ enables
   diagnostics, hover, completion, and formatting by default, so this
   step is not required.
6. Click **OK**.

### 3b. By importing the template

1. **Settings ▸ Languages & Frameworks ▸ Language Servers ▸ +**.
2. Choose **Template ▸ Import from custom template…**
   (menu wording varies slightly by LSP4IJ version).
3. Select the [`lsp4ij/`](lsp4ij/) folder in this repository (the folder
   that contains `template.json`).
4. Confirm the command shows `aql lsp` and the mapping shows `*.aql`,
   then **OK**.

See [`lsp4ij/README.md`](lsp4ij/README.md) for how each template field
maps to the dialog, and the caveat about LSP4IJ's own schema.

---

## 4. Verify it works

1. Open (or create) any `*.aql` file — e.g. copy one from
   `design/examples/todo/` in this repo.
2. Within a second or two the **LSP4IJ** tool window should show the
   **AQL** server as **Started / Running**. If not, click the server and
   read its console pane (see [Troubleshooting](#troubleshooting)).
3. Check each feature:
   - **Diagnostics** — introduce a deliberate error (e.g. an unterminated
     `[`) and confirm a red squiggle plus an entry in the **Problems**
     view.
   - **Hover** — hover a builtin such as `add` or `filter`; a doc popup
     should appear.
   - **Completion** — start typing a word and press `Ctrl+Space`; the
     list should include registered AQL words.
   - **Formatting** — run **Code ▸ Reformat Code** (`Ctrl+Alt+L` /
     `⌘⌥L`); the buffer is reformatted by `aql fmt`'s engine.

---

## Using a non-PATH `aql` binary

If `aql` isn't on the IDE's PATH (common on macOS GUI launches, or when
you keep multiple builds), give the **Command** an absolute path instead
of the bare `aql`:

- macOS / Linux:
  ```
  /usr/local/bin/aql lsp
  ```
- Windows (quote a path containing spaces):
  ```
  "C:\Program Files\aql\aql.exe" lsp
  ```

The command string is parsed like a shell command line, so keep `lsp` as
a separate trailing argument. In the importable template this is the
`programArgs.default` value — see
[`lsp4ij/README.md`](lsp4ij/README.md#adjusting-the-command).

> **Why this happens:** JetBrains IDEs launched from the Dock/Start menu
> often inherit a minimal PATH that omits `~/go/bin`, Homebrew, etc.
> An absolute path sidesteps the whole issue.

---

## Colours via a TextMate bundle

LSP4IJ gives you diagnostics/hover/completion/formatting but **not**
syntax colouring — the AQL server has no semantic-tokens capability. To
add colours without writing a custom plugin, JetBrains IDEs can import a
**TextMate bundle** (they bundle the *TextMate Bundles* support):

1. **Settings ▸ Editor ▸ TextMate Bundles**.
2. Click **+** and select a directory containing an AQL TextMate grammar.
   This repo ships one at [`../textmate/`](../textmate/): the folder holds
   `aql.tmLanguage.json` (scope `source.aql`, file type `aql`). Point the
   importer at that directory.
3. **OK**, then reopen an `.aql` file.

> **JetBrains TextMate quirk.** The IDE's TextMate importer expects a
> *bundle folder*, ideally with an `info.plist`/`package.json` manifest
> listing the grammar. It will happily consume a folder that contains a
> lone `*.tmLanguage.json`; if your IDE version insists on a manifest,
> the same grammar is embedded in the VS Code extension at
> [`../vscode/syntaxes/aql.tmLanguage.json`](../vscode/syntaxes/aql.tmLanguage.json),
> whose `package.json` provides the VS Code-style manifest JetBrains can
> also read.

TextMate colouring and the LSP4IJ language server coexist: TextMate paints
the tokens, LSP4IJ supplies the smarts. If you'd rather not add colours at
all, the file type from
[step 2](#2-recommended-register-an-aql-file-type) still gives you
brackets and comment toggling, and LSP4IJ handles the rest.

---

## The heavier alternative: a full custom plugin

If you need first-class colouring, folding, structure view, and
navigation beyond what the LSP provides, the "proper" route is a native
IntelliJ-platform plugin built with the
[IntelliJ Platform Plugin SDK](https://plugins.jetbrains.com/docs/intellij/welcome.html),
optionally embedding the language server through LSP4IJ's *external
annotator* / *programmatic* API instead of the user-defined-server UI.
That is a Gradle project with a `Lexer`/`ParserDefinition` (or a
generated grammar), and is far more work than the LSP4IJ path above. It
is out of scope for this directory; the LSP4IJ + TextMate combination
covers the common cases without any compiled plugin.

---

## Troubleshooting

- **Server won't start / "command not found".** The IDE can't find `aql`.
  Use an absolute path in the **Command**
  (see [above](#using-a-non-path-aql-binary)).
- **No diagnostics/hover/completion.** Open the **LSP4IJ** tool window,
  select the **AQL** server, and check that it is *Running*. Set the
  server's **Trace** level (Configuration tab, or
  [`lsp4ij/settings.json`](lsp4ij/settings.json) → `debug.traceLevel`) to
  `verbose` to see the JSON-RPC exchange in the console.
- **Server started but nothing binds to my file.** The mapping isn't
  matching. Confirm the **Mappings** tab has a `*.aql` file-name pattern
  (or the `AQL` file type) with language id `aql`, and that your file
  really ends in `.aql`.
- **Formatting does nothing / reverts.** Make sure you're invoking
  **Reformat Code**, and that **Formatting** is enabled for the server
  (it is by default; see `features.formatting` in
  [`lsp4ij/settings.json`](lsp4ij/settings.json)).
- **Want to watch it live from a terminal instead?** `aql lsp` also
  serves over TCP: `aql lsp -p 9999`. LSP4IJ user-defined servers spawn a
  process over stdio, so this is only for manual debugging, not for the
  IDE binding.

---

## See also

- [`../README.md`](../README.md) — the editor-integration index and the
  full list of what `aql lsp` provides.
- [`../vscode/`](../vscode/) — the VS Code client (same server).
- [`../emacs/`](../emacs/) — the reference `aql-mode` (grammar source of
  truth).
- [`../textmate/`](../textmate/) — the TextMate grammar for colours.
- [`lsp4ij/`](lsp4ij/) — importable LSP4IJ template + field mapping.
