# GitHub Linguist support for boru

This directory contains everything needed to get **`.boru` files
syntax-highlighted on GitHub** and to have repositories that contain boru
**classified as boru** in the repository language bar.

GitHub uses [github/linguist](https://github.com/github-linguist/linguist)
for both jobs:

- **classification** — deciding a blob is boru (by extension + optional
  heuristics), which drives the per-repo language bar and stats;
- **highlighting** — colourising the blob using a TextMate/tree-sitter
  grammar whose scope name matches the language's `tm_scope`.

Getting boru fully supported means a pull request to `github/linguist`.
This directory holds the pieces that PR needs, plus a **local fallback**
(`.gitattributes`) you can use *today*, before the upstream PR lands.

---

## What's in here

| File                    | Purpose                                                                 |
| ----------------------- | ----------------------------------------------------------------------- |
| `languages.yml.patch`   | The exact YAML block to add to linguist's `lib/linguist/languages.yml`. |
| `samples/`              | 6 real, representative `.boru` programs (byte-identical repo copies).    |
| `gitattributes-example` | A `.gitattributes` snippet to force-classify `.boru` as boru locally.     |
| `README.md`             | This file — the full submission checklist.                              |

The `samples/` programs were copied unchanged from the repository and
between them exercise every construct the grammar must handle:

| Sample          | Copied from                        | Exercises                                                                        |
| --------------- | ---------------------------------- | -------------------------------------------------------------------------------- |
| `app.boru`       | `design/examples/todo/app.boru`     | `import`, `def`, paren pipelines, maps, numbers, `#` comments                     |
| `entity.boru`    | `design/examples/entity/entity.boru`| `export`, `def … fn [[…]]`, typed params (`name:Type`), `=>` lambdas, `/r` mod    |
| `todo.boru`      | `design/examples/todo/todo.boru`    | service handlers, `=>` lambdas, backtick template strings (`` `…${expr}…` ``)     |
| `auth.boru`      | `design/examples/todo/auth.boru`    | cross-cutting `wrap`, template strings, `@`-prefixed keys, `raise`                |
| `solardemo.boru` | `lang/go/test/solardemo.boru`       | `make`, `export`, capitalised type names, double-quoted strings                   |
| `echo_boru.boru`  | `bench/networking/echo_boru.boru`    | module namespaces (`Net.`/`TimeUtil.`), `convert`, `for`, typed `fn`, floats      |

Linguist wants a handful of real samples per language (a half-dozen is
plenty); these six give the classifier and any future heuristics a
realistic corpus.

---

## Submission checklist (upstream PR to github/linguist)

Work through this in order. Everything happens in a clone of
`github-linguist/linguist`, **not** in this repo.

### 1. Add the language entry

Open `lib/linguist/languages.yml`, find the alphabetical slot for `boru`
(after `Boogie`, before `BQN`), and paste the block from
[`languages.yml.patch`](./languages.yml.patch). Key fields:

- `type: programming`
- `color: "#4a7dbf"` — a valid, reasonably distinct hex colour (tweak to taste)
- `extensions: [".boru"]`
- `tm_scope: source.boru` — **must** equal the grammar's top-level `scopeName`
- `ace_mode: text` — honest fallback; boru has no dedicated Ace mode upstream
- `language_id:` — **do not invent one.** Generate it with the tool (step 5).

### 2. Add the grammar submodule

Linguist highlights via a grammar registered in `vendor/grammars/` as a
**git submodule**, listed in `vendor/README.md`, and referenced by
`tm_scope`. You need a published TextMate/tree-sitter grammar whose scope
is **`source.boru`**.

```bash
# from the linguist clone:
git submodule add https://github.com/boru-lang/<boru-grammar-repo> \
    vendor/grammars/<boru-grammar-repo>
```

Then add a line to `vendor/README.md` crediting the grammar and its
licence, and register the scope in `grammars.yml`:

```bash
script/add-grammar https://github.com/boru-lang/<boru-grammar-repo>
```

Notes:

- The grammar must live in its **own public repo** with an OSI licence
  (linguist requires this for vendored grammars).
- `scopeName` in the grammar (`source.boru`) must match `tm_scope` in
  `languages.yml` exactly, or highlighting silently no-ops.
- If boru does not yet have a standalone grammar repo, extract one from an
  existing editor integration (this repo already ships editor support —
  e.g. the reference Emacs mode at `editors/emacs/boru-mode.el` and the
  VS Code / Zed / Sublime / Kate / Helix configs under `editors/` — whose
  vocabulary and faces a TextMate grammar should mirror). A tree-sitter
  grammar is also accepted by linguist and is the modern preference.
- If you submit **without** a grammar, set `tm_scope: none` in the YAML.
  Blobs then classify as boru but render un-highlighted. A grammar is
  strongly preferred; prefer landing it in the same PR.

### 3. Add the samples

Copy the programs from [`samples/`](./samples) into linguist's sample
tree:

```bash
mkdir -p samples/boru
cp /path/to/this/repo/editors/linguist/samples/*.boru samples/boru/
```

Linguist's test suite tokenizes everything under `samples/<Language>/`
and uses it to train the Bayesian classifier that disambiguates shared
extensions. `.boru` is currently unique to boru, so no disambiguation
heuristic is needed — but the samples are still required and must parse
under the grammar you registered.

### 4. Add a heuristic only if an extension collides

`.boru` is not (currently) claimed by another language in linguist, so
**no heuristic is required**. If a future collision appears (another
language also using `.boru`), add a disambiguating rule to
`lib/linguist/heuristics.yml` keyed on `.boru`, matching a
boru-distinctive pattern — e.g. the concatenative `def … fn [[ … ]]`
shape, `import "boru:…"`, or `export "Name" { … }`. Example skeleton:

```yaml
disambiguations:
  - extensions: ['.boru']
    rules:
      - language: boru
        pattern: '(?m)^\s*(import\s+"boru:|export\s+"[A-Z]|def\s+\w+\s+fn\s+\[\[)'
      - language: <OtherLanguage>
        pattern: '<other-distinctive-pattern>'
```

### 5. Generate the `language_id`

`language_id` is a stable integer allocated by the linguist tooling — not
by hand. After steps 1–3, run the generator from the linguist clone:

```bash
bundle install
script/update-ids          # allocates + writes the new language_id
# or: bundle exec rake check_language_ids
```

Commit the generated id. In the PR description, note that the id came
from the tool (reviewers check this).

### 6. Run linguist's test suite

```bash
bundle install
bundle exec rake test       # full suite: YAML lint, samples, grammars, ids
# focused checks:
bundle exec rake check_yaml
bundle exec rake samples
```

All green is a prerequisite for review.

### 7. Open the pull request

Push a branch to your linguist fork and open a PR against
`github-linguist/linguist`. Follow
[`CONTRIBUTING.md`](https://github.com/github-linguist/linguist/blob/master/CONTRIBUTING.md).
Linguist's policy is that a language should be **in reasonable use** (a
few hundred repos / a meaningful public footprint) before it is added —
be ready to link to public boru repositories and the language's home
(https://github.com/boru-lang/boru). Include in the PR:

- a one-line description of boru (concatenative, strongly-typed query language),
- links to the spec / homepage and to real repositories using `.boru`,
- confirmation the grammar repo is public + OSI-licensed,
- confirmation `language_id` was generated by the tool.

After merge, GitHub picks up the change on its next linguist bump
(highlighting and classification then apply to all `.boru` files across
GitHub automatically).

---

## Local fallback — highlight & classify TODAY

You do not have to wait for the upstream PR to get correct
classification in your own repositories. Add the snippet from
[`gitattributes-example`](./gitattributes-example) to a `.gitattributes`
file at your repository root:

```gitattributes
*.boru linguist-language=boru
```

Commit and push. GitHub's Linguist honours per-repo `.gitattributes`, so
`.boru` files are immediately reported as boru in the language bar.

Caveat: `linguist-language` overrides **classification**, but syntax
**highlighting** still needs a grammar linguist knows about. Until the
grammar from step 2 ships upstream, force-classified blobs render as
plain text. Classification (the language bar, the "boru" label) works
regardless — highlighting follows once the grammar lands.
