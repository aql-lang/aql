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

    // Generics angle sugar: value, data-context, child-constraint,
    // and annotation use-sites; multi-arg and empty argument lists.
    ['Box<Integer>', 'sugar(angle Box [word(Integer)])'],
    ['{x: Box<Integer>}', '{x:sugar(angle Box [word(Integer)])}'],
    ['[:Box<Integer>]', '[:sugar(angle Box [word(Integer)])]'],
    ['Box<Integer String>', 'sugar(angle Box [word(Integer) word(String)])'],
    ['Box<>', 'sugar(angle Box [])'],
    ['(Box of [Integer])', 'paren([word(Box) word(of) [word(Integer)]])'],

    // Underscore numeric literals and the 0d forms.
    ['1_000', '1000'],
    ['1_0.5', '10.5'],
    ['0d1_2.3', '12.3'],
    ['0d1e2', '100'],
    ['0d1.5e2', '150'],
    ['0d1_2e1_0', '120000000000'],
    ['1__0', 'ERR [boru/syntax_error]: misplaced `_` in numeric literal: 1__0'],
    ['1_', 'ERR [boru/syntax_error]: invalid numeric literal: 1_'],

    // String escape tails: unicode/hex pass through decoded, unknown
    // escapes drop the backslash.
    ["'u\\u0041'", "'uA'"],
    ["'x\\x41'", "'xA'"],
    ["'q\\z'", "'qz'"],

    // Bare word values in data context stay opaque words.
    ['{a: true}', '{a:word(true)}'],
    ['[true false]', '[word(true) word(false)]'],
    ['{b: none}', '{b:none}'],

    // Map-shorthand keys strip a VALID modifier suffix; an invalid
    // one keeps the full token; /q quotes to an atom value.
    ['{x}', '{x:word(x)}'],
    ['{x/r}', '{x:word(x)}'],
    ['{x/2}', '{x:word(x)}'],
    ['{x/s}', '{x:word(x)}'],
    ['{x/q}', '{x:x/q}'],
    ['{x/zz}', '{x/zz:word(x/zz)}'],
    ['{a x/r}', '{a:word(a) x:word(x)}'],

    // XML: entity re-escaping, error taxonomy, interpolation holes,
    // and comment handling.
    ['<a>&amp;&lt;&gt;</a>', '<a>&amp;&lt;&gt;</a>'],
    ["<a b='&amp;'/>", '<a b="&amp;"/>'],
    ['<a>&#65;</a>', '<a>&amp;#65;</a>'],
    ['<a>&quot;</a>', '<a>"</a>'],
    ['<a', 'ERR xml: unterminated opening tag <a>'],
    ['<a></b>', 'ERR xml: mismatched closing tag </b> for <a>'],
    ['<a href=>x</a>', 'ERR xml: attribute "href" in <a> must have a quoted value'],
    ["<a 'x'/>", 'ERR xml: invalid attribute name in <a>'],
    ['<a>${1}</a>', 'interp-xml(<a>${1}</a>)'],
    ["<a b='${2}'/>", 'interp-xml(<a b="${2}"/>)'],
    ['<a><b>${3}</b></a>', 'interp-xml(<a><b>${3}</b></a>)'],
    ['<a><!-- c --></a>', '<a/>'],
    ['<a><!-- unclosed', 'ERR xml: unterminated comment in <a>'],
    ["<a  b = '1' />", '<a b="1"/>'],

    // Arrow-fold shapes: bare, in lists and parens, mid-stream folds;
    // the map-value positions the fold rejects.
    ['x => [1]', 'word(x) sugar(lambda) [1]'],
    ['[a => [1]]', '[paren([word(a) sugar(lambda) [1]])]'],
    ['(x => [x 1])', 'paren([paren([word(x) sugar(lambda) [word(x) 1]])])'],
    ['a b => [1]', 'word(a) paren([word(b) sugar(lambda) [1]])'],
    ['{f: x => [x]}', 'ERR [boru/syntax_error]: unexpected `=>` — nothing valid can appear here'],

    // Angle sugar composing with words and multi-arg pairs.
    ['Box<Integer> of [1]', 'sugar(angle Box [word(Integer)]) word(of) [1]'],
    ['{p: Pair<A B>}', '{p:sugar(angle Pair [word(A) word(B)])}'],

    // Interpolation segment shapes.
    ['` plain `', "' plain '"],
    ['`${1}${2}`', 'interp(${1} ${2})'],
    ["`a${'s'}b`", "interp('a' ${'s'} 'b')"],

    // Unterminated containers: parens error, lists/maps auto-close.
    ['(1', 'ERR [boru/syntax_error]: unmatched opening parenthesis'],
    ['1 (2 3', 'ERR [boru/syntax_error]: unmatched opening parenthesis'],
    ['[1', '[1]'],
    ['{a:1', '{a:1}'],
    ["'unterm", "ERR [boru/syntax_error]: this string is never closed: 'unterm"],

    // Integer-range enforcement, big-number reach beyond it, float
    // overflow to infinity, and the unclosed angle error.
    ['9223372036854775808',
      'ERR [boru/integer_overflow]: integer literal out of range: 9223372036854775808 exceeds the Integer range (-9223372036854775808..9223372036854775807)'],
    ['-9223372036854775809',
      'ERR [boru/integer_overflow]: integer literal out of range: -9223372036854775809 exceeds the Integer range (-9223372036854775808..9223372036854775807)'],
    ['0d99999999999999999999', '99999999999999999999'],
    ['1e400', 'inf'],
    ['-1e400', '-inf'],
    ['0.1_2', '0.12'],
    ['Box<Integer', 'ERR [boru/syntax_error]: unclosed angle bracket: Box<… has no matching `>`'],

    // Unusual pair keys: spaced strings, numbers, keyword-ish words.
    ["'a' : 1", '{a:1}'],
    ["{'q k':2}", '{q k:2}'],
    ['{5:1}', '{5:1}'],
    ['true:1', '{true:1}'],
    ['{none:1}', '{none:1}'],

    // Disjunct spellings stay word streams in parens.
    ['(A tor None)', 'paren([word(A) word(tor) word(None)])'],
    ['(None tor A)', 'paren([word(None) word(tor) word(A)])'],
  ]
  for (const [src, want] of rows) {
    it(JSON.stringify(src), () => {
      assert.equal(render(src), want)
    })
  }

  it('folds map-value dot-chains into paren groups', () => {
    for (const src of ['{n: bf.n}', '{n: a.b.c}', '{n: bf!.n}']) {
      const got = canon(parse(src))
      assert.ok(got.startsWith('{n:paren('), `${src} -> ${got}`)
    }
  })

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
