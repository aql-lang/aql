import { strict as assert } from 'node:assert'
import * as fs from 'node:fs'
import * as path from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, it } from 'node:test'

// Keep every shipped parser implementation file in the coverage universe.
// Node instruments loaded modules rather than every --test-coverage-include
// match, so these explicit imports plus the directory assertion prevent a new
// production file from silently escaping a nominal 100% report.
import './convert.ts'
import './declgrammar.ts'
import './errors.ts'
import './grammar.ts'
import './index.ts'
import './nodes.ts'
import './public.ts'
import './xml.ts'

const COVERED_SOURCES = [
  'convert.ts',
  'declgrammar.ts',
  'errors.ts',
  'grammar.ts',
  'index.ts',
  'nodes.ts',
  'public.ts',
  'xml.ts',
]

describe('parser coverage source manifest', () => {
  it('names every production module', () => {
    const dir = path.dirname(fileURLToPath(import.meta.url))
    const sources = fs.readdirSync(dir)
      .filter((name) => name.endsWith('.ts'))
      .filter((name) => !name.endsWith('.test.ts'))
      // The stream dumper is test tooling, matching parser/go's _test.go
      // dumper, and is exercised by the separate parser-crossdiff gate.
      .filter((name) => name !== 'streamdump.ts')
      .sort()
    assert.deepEqual(sources, COVERED_SOURCES)
  })
})
