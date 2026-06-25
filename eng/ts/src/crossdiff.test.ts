// Cross-engine differential gate, TS half: runs the shared corpus through the
// TS engine and compares each row to the Go kernel's result for the SAME row,
// obtained by executing the Go dumper (test/go/engspec TestCrossDump, gated on
// CROSSDUMP_FILE). This is the mirror of test/go/engspec/crossdiff_test.go —
// each engine's suite uses the other.
//
// Dispositions (joined on mode:file:line): agree / codeDiff (both error,
// different code — reported) / gap (one errors where the other succeeds —
// permitted, reported) / divergence (both succeed, values differ — FAIL).
//
// Skips when the Go toolchain is unavailable so `npm test` stays green in
// node-only environments.
import { describe, it } from 'node:test'
import { strict as assert } from 'node:assert'
import { execFileSync } from 'node:child_process'
import * as fs from 'node:fs'
import * as os from 'node:os'
import * as path from 'node:path'
import { fileURLToPath } from 'node:url'

import { type CrossRec, tsRows } from './crossdiff-shared.ts'

const __dirname = path.dirname(fileURLToPath(import.meta.url))
const goDir = path.resolve(__dirname, '..', '..', '..', 'test', 'go')

function goAvailable(): boolean {
  try {
    execFileSync('go', ['version'], { stdio: 'ignore' })
    return true
  } catch {
    return false
  }
}

function goRows(): Map<string, CrossRec> {
  const outFile = path.join(os.tmpdir(), `aql-crossdump-go-${process.pid}.jsonl`)
  try {
    execFileSync('go', ['test', './engspec/', '-run', 'TestCrossDump', '-count=1'], {
      cwd: goDir,
      env: { ...process.env, CROSSDUMP_FILE: outFile },
      stdio: 'ignore',
    })
    const text = fs.readFileSync(outFile, 'utf8')
    const m = new Map<string, CrossRec>()
    for (const line of text.split('\n')) {
      const t = line.trim()
      if (t === '') continue
      const r = JSON.parse(t) as CrossRec
      m.set(`${r.mode}|${r.file}:${r.line}`, r)
    }
    return m
  } finally {
    try {
      fs.rmSync(outFile, { force: true })
    } catch {
      /* ignore */
    }
  }
}

describe('cross-engine differential (TS vs Go)', () => {
  it('shared corpus results agree; gaps reported, divergences fail', { skip: goAvailable() ? false : 'go toolchain unavailable' }, () => {
    const go = goRows()
    const ts = tsRows()

    let agree = 0
    let codeDiff = 0
    let gaps = 0
    let value = 0
    let check = 0
    const gapMsgs: string[] = []
    const divMsgs: string[] = []
    const codeMsgs: string[] = []

    for (const r of ts) {
      if (r.mode === 'value') value++
      else check++
      const key = `${r.mode}|${r.file}:${r.line}`
      const g = go.get(key)
      if (g === undefined) continue
      const loc = `${r.mode} ${r.file}:L${r.line} ${r.input}`
      if (r.ok && g.ok) {
        if (r.out === g.out) agree++
        else if (divMsgs.length < 25) divMsgs.push(`${loc}\n    ts= ${r.out}\n    go= ${g.out}`)
      } else if (!r.ok && !g.ok) {
        if (r.out === g.out) agree++
        else {
          codeDiff++
          if (codeMsgs.length < 40) codeMsgs.push(`${loc}  (ts=${r.out} go=${g.out})`)
        }
      } else {
        gaps++
        const which = r.ok ? 'ts=ok go=err' : 'ts=err go=ok'
        if (gapMsgs.length < 50) gapMsgs.push(`${loc}  (${which}; ts=${r.out} go=${g.out})`)
      }
    }

    const divergences = divMsgs.length
    const lines = [
      '',
      `  cross-engine differential (TS vs Go): ${agree + codeDiff + gaps + divergences} rows compared (value=${value} check=${check})`,
      `    agree:          ${agree}`,
      `    error codeDiff:  ${codeDiff}`,
      `    GAPS:           ${gaps}  (one engine errors where the other succeeds — permitted)`,
      `    divergences:    ${divergences}  (both succeed, values differ — fail)`,
      ...codeMsgs.map((m) => `    codeDiff: ${m}`),
      ...gapMsgs.map((m) => `    gap: ${m}`),
    ]
    console.log(lines.join('\n'))

    assert.equal(divergences, 0, `cross-engine divergences:\n${divMsgs.join('\n')}`)
  })
})
