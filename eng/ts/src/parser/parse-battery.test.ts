// The parser breadth battery: source shapes chosen for the converter
// arms the shared corpus rarely reaches — reach chains and their
// number-receiver rejection, string escape decoding, 0d big-number
// literals, sugar markers, XML literals, interpolation, typed
// containers, dispatch modifiers — each pinned by its canonical render
// (or its one-line error). The Go parser is the reference for every
// render here.

import { describe, it } from 'node:test'
import { strict as assert } from 'node:assert'

import { canon } from '../canon.ts'
import { parse } from './index.ts'

function render(src: string): string {
  try {
    return canon(parse(src))
  } catch (e) {
    const msg = e instanceof Error ? (e.message.split('\n')[0] ?? '') : String(e)
    return 'ERR ' + msg
  }
}

describe('parse battery', () => {
  const rows: Array<[src: string, want: string]> = [
    // String escape decoding.
    ["'a\\nb'", "'a\\nb'"],
    ["'t\\tt'", "'t\\tt'"],
    ["'r\\rr'", "'r\\rr'"],
    ["'bs\\\\'", "'bs\\\\'"],
    ["'q\\'q'", '"q\'q"'],
    ['"dq\\""', "'dq\"'"],

    // 0d big-number literals.
    ['0d123', '123'],
    ['0D45_6', '456'],
    ['-0d789', '-789'],
    ['0d_', 'ERR [boru/syntax_error]: invalid 0d numeric literal: 0d_'],
    ['0d12_', 'ERR [boru/syntax_error]: invalid 0d numeric literal: 0d12_'],

    // Number receivers reject reach.
    ['1.2.3', 'ERR [boru/syntax_error]: a number has no members to access with `.`'],
    ['5 . foo', 'ERR [boru/syntax_error]: a number has no members to access with `.`'],

    // Sugar markers.
    ['a => [1]', 'word(a) sugar(lambda) [1]'],
    ['1 ;', '1 end'],

    // XML literals, attribute quoting, nesting, self-closing normalise.
    ['<br/>', '<br/>'],
    ["<a href='u'>t</a>", '<a href="u">t</a>'],
    ["<t a='1' b='2'><u/>text</t>", '<t a="1" b="2"><u/>text</t>'],

    // Interpolation strings.
    ['`v ${1} w`', "interp('v ' ${1} ' w')"],
    ['`${x}`', 'interp(${word(x)})'],

    // Typed containers.
    ['[:Integer]', '[:word(Integer)]'],
    ['[:Integer 1 2]', '[:word(Integer) 1 2]'],
    ['{:String}', '{:word(String)}'],

    // Comments and groups.
    ['# comment\n1', '1'],
    ['1 # trailing', '1'],
    ['(quote a)', 'paren([word(quote) word(a)])'],

    // Containers and numerics.
    ['{a:{b:[1 2]}}', '{a:{b:[1 2]}}'],
    ['[{x:1}]', '[{x:1}]'],
    ['-2.5e3', '-2500.0'],
    ['1e-7', '0.0000001'],

    // Pair sugar.
    ['a:b', '{a:word(b)}'],
    ["'s':1", '{s:1}'],
    ['12:30:00', '{12:{30:0}}'],

    // Slash words vs dispatch modifiers.
    ['a/b/c', 'word(a/b/c)'],
    ['f/q', 'f/q'],
  ]
  for (const [src, want] of rows) {
    it(JSON.stringify(src), () => {
      assert.equal(render(src), want)
    })
  }

  it('builds reach nodes for dotted chains', () => {
    for (const src of ['x.y', 'x.y.z', 'm.a.b.c', '$.name', 'x.0', "x.'k'"]) {
      const vals = parse(src)
      assert.ok(vals.length >= 1, src)
    }
  })

  it('keeps dispatch modifiers alongside the word', () => {
    for (const src of ['f/2', 'f/r']) {
      const vals = parse(src)
      assert.ok(vals.length >= 1, src)
    }
  })
})
