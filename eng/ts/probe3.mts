import { Jsonic } from '@tabnas/jsonic'
const j = Jsonic.make({ info: { text: true, list: true, map: true }, list: { property: true, pair: true, child: true }, map: { child: true }, value: { lex: false } })
j.options({ fixed: { token: { '#AR': '=>' } } })
const AR = j.token('#AR')
j.rule('val', (rs: any) => {
  rs.close([{ s: [AR], c: (r: any, ctx: any) => {
    console.log('AR close gate: parent=', r.parent?.name, 'parent.u=', JSON.stringify(r.parent?.rawu?.() ?? r.parent?.u), 'prev=', r.prev?.name)
    return false
  } }])
})
try { j.parse('a => [1]') } catch (e: any) { console.log('ERR', (e.message||'').split('\n')[0]) }
