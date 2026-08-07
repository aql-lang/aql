// The declarative grammar loader — the TS twin of parser/go/
// declgrammar.go. Both engines load the SAME committed artifact
// (parser/go/grammar.json; the Go side embeds it, this side reads
// it relatively, the SPEC_DIR precedent) so the grammar's structure —
// the token table and the rule-spec amendments — has one source of
// truth. Behavior that data cannot express binds by NAME from the
// host hook tables; an unknown name throws at parser construction.

import * as fs from 'node:fs'
import * as path from 'node:path'
import { fileURLToPath } from 'node:url'

const __dirname = path.dirname(fileURLToPath(import.meta.url))
// The twins are siblings under parser/, so the artifact is two levels up
// then into go/: parser/ts/src -> parser/ -> parser/go/grammar.json. The
// Go side embeds this same file with go:embed, which is what makes the
// grammar ONE artifact rather than two that drift (DECLARATIVE-GRAMMAR.0).
// It was eng/ts/src/parser reaching four levels up to the repo root until
// the TS parser was cut out of the kernel into its own module.
const GRAMMAR_PATH = path.resolve(__dirname, '..', '..', 'go', 'grammar.json')

export interface DeclToken {
  name: string
  text?: string
}

export interface DeclAlt {
  s?: string[][]
  p?: string
  r?: string
  b?: number
  text?: string
  a?: string
  c?: string
}

export interface DeclEdit {
  rule: string
  op: string
  alts: DeclAlt[]
}

export interface DeclGrammar {
  schema: number
  tokens: DeclToken[]
  edits: DeclEdit[]
}

export interface DeclHooks {
  actions: Record<string, (r: any, ctx: any) => void>
  conds: Record<string, (r: any, ctx: any) => boolean>
}

let cached: DeclGrammar | undefined

export function loadDeclGrammar(path: string = GRAMMAR_PATH): DeclGrammar {
  const isDefault = path === GRAMMAR_PATH
  if (isDefault && cached !== undefined) return cached
  const g = JSON.parse(fs.readFileSync(path, 'utf8')) as DeclGrammar
  if (g.schema !== 1) {
    throw new Error(`parser: grammar.json: unsupported schema ${g.schema}`)
  }
  if (isDefault) cached = g
  return g
}

// fixedTokenOptions returns the fixed-token map for jsonic's options()
// (name '#OP' → source text), in artifact order.
export function fixedTokenOptions(g: DeclGrammar): Record<string, string> {
  const out: Record<string, string> = {}
  for (const tok of g.tokens) {
    if (tok.text !== undefined) out['#' + tok.name] = tok.text
  }
  return out
}

// resolveDeclTokens resolves every artifact token to its Tin — fixed
// tokens were pre-declared via options(), matcher-produced ones mint
// here, in artifact order.
export function resolveDeclTokens(j: any, g: DeclGrammar): Record<string, number> {
  const tins: Record<string, number> = {}
  for (const tok of g.tokens) {
    tins[tok.name] = j.token('#' + tok.name)
  }
  return tins
}

// applyDeclEdits applies every rule amendment in the artifact, binding
// named hooks from the tables — the exact twin of the Go loader. The
// helpers carry the host grammar's edit idioms plus its Text-marker
// constructor (the boxed-String newText).
export interface DeclHelpers {
  prependOpen: (rs: any, alts: any[]) => void
  prependClose: (rs: any, alts: any[]) => void
  setOpen: (rs: any, alts: any[]) => void
  setClose: (rs: any, alts: any[]) => void
  newText: (str: string) => unknown
}

export function applyDeclEdits(
  j: any,
  g: DeclGrammar,
  tins: Record<string, number>,
  hooks: DeclHooks,
  helpers: DeclHelpers,
): void {
  for (const edit of g.edits) {
    const alts = edit.alts.map((da) => buildDeclAlt(edit, da, tins, hooks, helpers))
    j.rule(edit.rule, (rs: any) => {
      switch (edit.op) {
        case 'prependOpen':
          helpers.prependOpen(rs, alts)
          break
        case 'appendOpen':
          rs.open(alts)
          break
        case 'setOpen':
          helpers.setOpen(rs, alts)
          break
        case 'prependClose':
          helpers.prependClose(rs, alts)
          break
        case 'appendClose':
          rs.close(alts)
          break
        case 'setClose':
          helpers.setClose(rs, alts)
          break
        default:
          throw new Error(`parser: grammar.json: unknown edit op ${edit.op} on rule ${edit.rule}`)
      }
    })
  }
}

function buildDeclAlt(
  edit: DeclEdit,
  da: DeclAlt,
  tins: Record<string, number>,
  hooks: DeclHooks,
  helpers: DeclHelpers,
): Record<string, unknown> {
  const alt: Record<string, unknown> = {}
  if (da.s !== undefined) {
    // The artifact's canonical shape is a sequence of token-alternative
    // groups; the TS jsonic flattens singleton groups.
    alt['s'] = da.s.map((group) => {
      const row = group.map((name) => {
        const tin = tins[name]
        if (tin === undefined) {
          throw new Error(`parser: grammar.json: unknown token ${name} in rule ${edit.rule}`)
        }
        return tin
      })
      return row.length === 1 ? row[0] : row
    })
  }
  if (da.p !== undefined) alt['p'] = da.p
  if (da.r !== undefined) alt['r'] = da.r
  if (da.b !== undefined) alt['b'] = da.b
  if (da.text !== undefined) {
    const marker = da.text
    alt['a'] = (r: any) => {
      r.node = helpers.newText(marker)
    }
  } else if (da.a !== undefined) {
    const fn = hooks.actions[da.a]
    if (fn === undefined) {
      throw new Error(`parser: grammar.json: unknown action hook ${da.a} in rule ${edit.rule}`)
    }
    alt['a'] = fn
  }
  if (da.c !== undefined) {
    const fn = hooks.conds[da.c]
    if (fn === undefined) {
      throw new Error(`parser: grammar.json: unknown condition hook ${da.c} in rule ${edit.rule}`)
    }
    alt['c'] = fn
  }
  return alt
}
