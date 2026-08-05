// The declarative-grammar loader's contract arms: the artifact loads
// and validates, and every unknown-name path — edit op, token, action
// hook, condition hook — fails parser construction loudly (the same
// contract the Go loader enforces with panics).

import { describe, it } from 'node:test'
import { strict as assert } from 'node:assert'

import {
  type DeclGrammar,
  type DeclHelpers,
  applyDeclEdits,
  fixedTokenOptions,
  loadDeclGrammar,
} from './declgrammar.ts'

const helpers: DeclHelpers = {
  prependOpen: () => {},
  prependClose: () => {},
  setOpen: () => {},
  setClose: () => {},
  newText: (s: string) => s,
}

const noHooks = { actions: {}, conds: {} }

function fakeJ(): { rule: (name: string, cb: (rs: unknown) => void) => void } {
  return { rule: (_name, cb) => cb({}) }
}

function grammarWith(edit: DeclGrammar['edits'][number]): DeclGrammar {
  return { schema: 1, tokens: [{ name: 'OP', text: '(' }], edits: [edit] }
}

describe('declarative grammar loader', () => {
  it('loads the committed artifact', () => {
    const g = loadDeclGrammar()
    assert.equal(g.schema, 1)
    assert.ok(g.tokens.length >= 15)
    assert.ok(g.edits.length >= 2)
    const fixed = fixedTokenOptions(g)
    assert.equal(fixed['#OP'], '(')
    assert.equal(fixed['#TL'], undefined)
  })

  it('rejects an unknown edit op', () => {
    const g = grammarWith({ rule: 'val', op: 'sideways', alts: [] })
    assert.throws(() => applyDeclEdits(fakeJ(), g, {}, noHooks, helpers), /unknown edit op/)
  })

  it('rejects an unknown token name', () => {
    const g = grammarWith({ rule: 'val', op: 'setOpen', alts: [{ s: [['ZZTOP']] }] })
    assert.throws(() => applyDeclEdits(fakeJ(), g, {}, noHooks, helpers), /unknown token/)
  })

  it('rejects an unknown action hook', () => {
    const g = grammarWith({ rule: 'val', op: 'setOpen', alts: [{ a: 'ghost' }] })
    assert.throws(() => applyDeclEdits(fakeJ(), g, {}, noHooks, helpers), /unknown action hook/)
  })

  it('rejects an unknown condition hook', () => {
    const g = grammarWith({ rule: 'val', op: 'setOpen', alts: [{ c: 'ghost' }] })
    assert.throws(() => applyDeclEdits(fakeJ(), g, {}, noHooks, helpers), /unknown condition hook/)
  })

  it('applies append and set ops through the rule spec', () => {
    let opened: unknown[] | null = null
    let closed: unknown[] | null = null
    const j = {
      rule: (_n: string, cb: (rs: unknown) => void) =>
        cb({
          open: (alts: unknown[]) => {
            opened = alts
          },
          close: (alts: unknown[]) => {
            closed = alts
          },
        }),
    }
    const tins = { OP: 1 }
    applyDeclEdits(
      j,
      {
        schema: 1,
        tokens: [],
        edits: [
          { rule: 'val', op: 'appendOpen', alts: [{ s: [['OP']], text: 'x' }] },
          { rule: 'val', op: 'appendClose', alts: [{ s: [['OP']], b: 1 }] },
        ],
      },
      tins,
      noHooks,
      helpers,
    )
    assert.ok(opened !== null && closed !== null)
  })
})
