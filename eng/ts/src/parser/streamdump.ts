// TS half of the parser-parity stream oracle: parse every eng/spec row
// with the jsonic-based TS parser and print one line per row in the
// SAME format as parser/go's TestStreamDump —
//
//	<file>:<line>\tOK\t<Value.toString of each value, space-joined>
//	<file>:<line>\tERR\t<error text first line>
//
// Run:  node --experimental-strip-types src/parser/streamdump.ts > ts-streams.tsv
// Diff against the Go dump to drive the port to parity.
import * as fs from 'node:fs'
import * as path from 'node:path'
import { fileURLToPath } from 'node:url'

import { BoruError } from '@voxgig/borucore'
import { parse } from './convert.ts'

const __dirname = path.dirname(fileURLToPath(import.meta.url))
const specDir = path.resolve(__dirname, '..', '..', '..', 'spec')

const names = fs
  .readdirSync(specDir)
  .filter((f) => f.endsWith('.tsv'))
  .sort()

for (const name of names) {
  const text = fs.readFileSync(path.join(specDir, name), 'utf8')
  let lineNum = 0
  for (const raw of text.split('\n')) {
    lineNum++
    const line = raw.replace(/[ \t]+$/, '')
    if (line === '' || line.startsWith('#')) continue
    const parts = line.split('\t')
    if (parts.length < 2) continue
    const input = parts[0]!.trim()
    if (input === '') continue
    try {
      const vals = parse(input)
      console.log(`${name}:${lineNum}\tOK\t${vals.map((v) => v.toString()).join(' ')}`)
    } catch (e) {
      // Plain (non-BoruError) errors render their bare message — Go
      // surfaces raw parser errors (the xml matcher's) unprefixed.
      const msg =
        e instanceof BoruError
          ? `[boru/${e.code}]: ${e.detail}`
          : e instanceof Error
            ? e.message
            : String(e)
      console.log(`${name}:${lineNum}\tERR\t${msg.split('\n')[0]}`)
    }
  }
}
