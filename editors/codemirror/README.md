# codemirror-lang-aql

A [CodeMirror 6](https://codemirror.net/) language mode for **AQL** — the
concatenative, strongly-typed query language (`.aql`).

It is implemented as a
[`StreamLanguage`](https://codemirror.net/docs/ref/#language.StreamLanguage)
(a hand-written `StreamParser`), so it needs **no Lezer grammar build step**
and stays dependency-light: the only runtime dependency is a peer dependency on
`@codemirror/language` (plus `@codemirror/state`, and `@codemirror/view` for a
running editor).

This mode also drives the **AQL web playground**.

## What it highlights

- Line comments (`# …` and `// …`) and block comments (`/* … */`, spanning
  lines).
- All three string kinds — `'single'`, `"double"`, and `` `template ${expr}` ``
  backtick strings, where the `${ … }` interpolation is tokenised as embedded
  AQL code.
- Every number form: decimal ints/floats (with `_` digit separators and
  exponents), `0x` hex, `0b` binary, `0d` big integers, and an optional leading
  `-`.
- Constant words `true` `false` `none` `inf` `nan` (and `-inf` / `-nan`).
- Keyword words (`def`, `fn`, `from`/`where`/`select`, `if`/`case`/`for`, …),
  builtin library words (`add`, `dup`, `concat`, `patrun`, …), and capitalised
  type names / module namespaces (`Integer`, `Emailon`, `MathUtil`, your own
  types).
- `def Foo …` (a type) vs `def foo …` (a value/function) binding targets.
- `/modifier` dispatch suffixes: `/q` `/s` `/f` `/r` `/t` `/u`, `/<digit>`
  arity, and combinations (`/sq`, `/2`, …).

The token vocabulary and faces mirror the reference Emacs mode
([`editors/emacs/aql-mode.el`](../emacs/aql-mode.el)).

## Install

```sh
npm install codemirror-lang-aql \
  @codemirror/language @codemirror/state @codemirror/view
```

The `@codemirror/*` packages are peer dependencies — install the versions your
app already uses (CodeMirror 6, `^6.0.0`).

## Usage

Add the `aql()` extension to your editor's extensions. Pair it with a syntax
highlighting extension (e.g. `@codemirror/language`'s `syntaxHighlighting` with
the `defaultHighlightStyle`, or a theme such as `@codemirror/theme-one-dark`)
so the token classes are painted.

```js
import { EditorState } from "@codemirror/state";
import { EditorView, keymap } from "@codemirror/view";
import { defaultKeymap } from "@codemirror/commands";
import {
  syntaxHighlighting,
  defaultHighlightStyle,
} from "@codemirror/language";
import { aql } from "codemirror-lang-aql";

const state = EditorState.create({
  doc: 'def add-two fn [[x:Integer] [Integer] [ x 2 add ]]',
  extensions: [
    aql(),
    syntaxHighlighting(defaultHighlightStyle),
    keymap.of(defaultKeymap),
  ],
});

new EditorView({ state, parent: document.querySelector("#editor") });
```

You can also import the language object directly if you prefer to assemble the
extension array yourself:

```js
import { aqlLanguage } from "codemirror-lang-aql";
// aqlLanguage is a StreamLanguage; aql() returns [aqlLanguage].
```

The mode advertises `languageData` for the host editor:

- `commentTokens` — `#` line comments and `/* … */` block comments (drives the
  toggle-comment command).
- `closeBrackets` — auto-closes `(` `[` `{` `"` `'` `` ` ``.

## Demo

Open [`index.html`](index.html) in a browser — it loads CodeMirror from
[esm.sh](https://esm.sh) and mounts an editor over a sample AQL program, so you
can eyeball the highlighting without a build step.

## Self-check

```sh
node --check aql.js
```

`node --check` only parses the module, so the peer-dependency import does not
need to be installed for the syntax check to pass.

## License

MIT
