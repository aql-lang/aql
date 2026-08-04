import { makeBoruJsonic } from './src/parser/grammar.ts'
import { Sited, ParenGroup, ArrowTag } from './src/parser/nodes.ts'
const { j } = makeBoruJsonic()
function show(v: any): any {
  if (v instanceof Sited) return { sited: show(v.node) }
  if (v instanceof ParenGroup) return { paren: v.items.map(show) }
  if (v instanceof ArrowTag) return 'ARROW'
  if (v instanceof String) return { text: String(v), q: (v as any).__info__?.quote }
  if (Array.isArray(v)) return v.map(show)
  return v
}
for (const src of [';', 'a ;', '(a ; b)', '(m.k)', '(a => [1])', 'a => [1]', '?']) {
  try { console.log(JSON.stringify(src), JSON.stringify(show(j.parse(src)))) }
  catch (e: any) { console.log(JSON.stringify(src), 'ERR', (e.message||'').split('\n')[0]) }
}
