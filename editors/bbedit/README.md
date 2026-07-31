# boru for BBEdit (macOS)

Two complementary layers of boru support for [BBEdit](https://www.barebones.com/products/bbedit/):

1. **`boru.plist`** — a **Codeless Language Module** (CLM) that gives
   `.boru` files syntax colouring, comment awareness, string/number
   recognition, and a function-navigation menu. No compiled plug-in
   required.
2. **BBEdit's built-in Language Server client** — point it at the bundled
   `boru lsp` server for diagnostics, hover, completion, and whole-buffer
   formatting.

Use either layer on its own, or both together (recommended).

## Install the Codeless Language Module

1. Copy `boru.plist` into BBEdit's Language Modules folder:

   ```sh
   mkdir -p "$HOME/Library/Application Support/BBEdit/Language Modules"
   cp boru.plist "$HOME/Library/Application Support/BBEdit/Language Modules/"
   ```

2. Restart BBEdit, or choose **BBEdit ▸ (menu) ▸ Reload Language Modules**
   if your version exposes it.

3. Open any `.boru` file. The language pop-up at the bottom of the editing
   window should read **boru**. If it does not, pick **boru** manually from
   that pop-up, or confirm the `.boru` suffix mapping in
   **Settings ▸ Languages**.

## What the CLM colours

The module recognises the boru lexical grammar as verified against the
`boru` CLI:

| Element | What is coloured |
| ------- | ---------------- |
| **Line comments** | `# ...` to end of line |
| **Block comments** | `/* ... */` (may span lines; not nestable) |
| **Strings** | `'single'`, `"double"`, and `` `template` `` backtick strings, all with `\n \t \\ \' \" \` \uXXXX` escapes |
| **Numbers** | integers (`1_000`), floats/exponents, `0x`/`0b`/`0d` literals, with an optional leading `-` |
| **Keywords** | control / declaration / query-DSL words — `def fn class from where select for-each fold` … |
| **Constants** | `true false none inf nan` |
| **Builtins** | library words — `add sub map filter dup swap get set patrun` … |
| **Function menu** | `def`/`fn`/`afn`/`class`/`surface`/`gen`/`module`/`export`/`macro`/`word`/`var` declarations, for jump-to navigation |

### CLM limitations

A Codeless Language Module supports exactly **one** line-comment prefix and
**one** block-comment pair, so the CLM registers `#` for line comments and
`/* */` for block comments. boru also accepts `//` line comments; those stay
valid boru but the CLM does not colour a bare `//` line. Capitalised type
names (`Integer`, `Emailon`, `MathUtil`, user types) and `/q` `/s` `/2`
dispatch modifiers are likewise beyond a CLM's keyword-list model — enable
the LSP below for full-fidelity handling and semantic features.

## Enable the boru Language Server (`boru lsp`)

BBEdit 14.5+ ships a built-in LSP client. The boru server is the `lsp`
subcommand of the `boru` binary and speaks stdio by default.

### Prerequisites

Install `boru` and confirm the server starts:

```sh
go install github.com/boru-lang/boru/cmd/go/boru@latest
echo '' | boru lsp        # stdio mode; waits for input, Ctrl-D to exit
```

Note the absolute path — BBEdit does not always inherit your shell `PATH`:

```sh
command -v boru           # e.g. /Users/you/go/bin/boru
```

### Configure BBEdit

1. Open **Settings ▸ Languages**.
2. In the language list, select **boru** (added by the CLM above). If boru is
   not listed, click **+**, then **Add**, and choose boru / the `.boru`
   suffix.
3. In the per-language pane on the right, open the **Language Server** tab
   and set:

   | Field | Value |
   | ----- | ----- |
   | **Enabled** | ✓ (checked) |
   | **Command** | the absolute path to `boru`, e.g. `/Users/you/go/bin/boru` |
   | **Arguments** | `lsp` |

   Leave **Working directory** empty to use the document's folder. If `boru`
   is reliably on the `PATH` BBEdit sees, you may set **Command** to just
   `boru`; the absolute path is the safe default.

4. Apply, then reopen a `.boru` file. Diagnostics appear in the gutter and
   the status bar; hover, completion, and formatting are available from the
   **Edit** and contextual menus.

### What the server provides

| Capability   | Backed by                          |
| ------------ | ---------------------------------- |
| Diagnostics  | `lang.Check` (errors + warnings)   |
| Hover        | `help.FormatDynamic` / `help.Format` |
| Completion   | `help.Words` (all registered words) |
| Formatting   | `formatter.Format` (whole-buffer)  |

### TCP transport (optional)

For debugging or remote-attach the server can listen on a socket instead of
stdio:

```sh
boru lsp -p 9999          # listens on 127.0.0.1:9999
```

Most BBEdit setups should use the default stdio command above.

## Validate the module

The plist is a standard XML property list and can be linted with the
system tool:

```sh
plutil -lint boru.plist
# boru.plist: OK
```

## Files

| File        | Purpose                                            |
| ----------- | -------------------------------------------------- |
| `boru.plist` | BBEdit Codeless Language Module (syntax + nav)     |
| `README.md` | this document                                      |
