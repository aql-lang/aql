import { makeBoruJsonic } from './src/parser/grammar.ts'
import { ParenGroup, AngleGroup, InterpGroup, IexprGroup, MiniLitVal, NumberVal, ArrowTag, Sited, UnclosedParen, UnclosedAngle } from './src/parser/nodes.ts'

function show(v: any, d = 0): string {
  if (v instanceof Sited) return `Sited@${v.pos.row}:${v.pos.col}(${show(v.node, d+1)})`
  if (v instanceof ParenGroup) return `Paren[${v.items.map(x=>show(x,d+1)).join(' ')}]`
  if (v instanceof UnclosedParen) return `UnParen[${v.items.map(x=>show(x,d+1)).join(' ')}]`
  if (v instanceof AngleGroup) return `Angle<${v.name}>[${v.items.map(x=>show(x,d+1)).join(' ')}]`
  if (v instanceof UnclosedAngle) return `UnAngle<${v.name}>`
  if (v instanceof InterpGroup) return `Interp[${v.parts.map(x=>show(x,d+1)).join(' ')}]`
  if (v instanceof IexprGroup) return `Iexpr[${v.items.map(x=>show(x,d+1)).join(' ')}]`
  if (v instanceof MiniLitVal) return `Mini(${v.name},${JSON.stringify(v.src)})`
  if (v instanceof NumberVal) return `Num(${v.val},${JSON.stringify(v.src)}@${v.row}:${v.col})`
  if (v instanceof ArrowTag) return `Arrow`
  if (v instanceof String) return `Text(${JSON.stringify(String(v))},q=${JSON.stringify((v as any).__info__?.quote)})`
  if (Array.isArray(v)) {
    const info = (v as any).__info__
    return `List${info?`{imp:${info.implicit},meta:${JSON.stringify(info.meta)}}`:''}[${v.map(x=>show(x,d+1)).join(' ')}]`
  }
  if (v && 'object' === typeof v) {
    const info = (v as any).__info__
    return `Map${info?`{imp:${info.implicit},meta:${JSON.stringify(info.meta)}}`:''}{${Object.entries(v).map(([k,x])=>k+':'+show(x,d+1)).join(' ')}}`
  }
  return JSON.stringify(v)
}

const { j } = makeBoruJsonic()
const cases = [
  '1 2 add', '[1 2]', '(1 add 2)', 'a => [1]', 'Box<Integer>', '+re/x/',
  '`a ${x} b`', 'm.k', '{n: bf.n}', 'foo/s', '0d12.5', 'a; b', '{foo?}',
  '{[k]:1 aa:2}', '{foo}', '[x?:Integer]', '(x:Integer => [x mul 2])',
  'def double x:Integer => [x mul 2]', '(())', 'Name<>', '(1', 'Box<Integer',
  '{a: bf.n b: 9}', 'a!.b', '| ? !', "{'a/b': 1}", '{b:2 a:1}', '` `', '``',
  '`${}`', 'x:Integer => y:Integer => [x add y]',
]
for (const src of cases) {
  try {
    const out = j.parse(src)
    console.log(JSON.stringify(src), '=>', show(out))
  } catch (e: any) {
    console.log(JSON.stringify(src), '=> ERR', (e.message || '').split('\n')[0], '| code:', e.code)
  }
}
