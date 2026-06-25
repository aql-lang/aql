// Check-mode parity tests for the TypeScript engine. Runs the SAME
// corpus the Go engine-level check runner uses (eng/spec/check/*.tsv)
// and renders each row to the identical string (see renderCheck /
// test/go/engspec/engcheck_test.go::renderCheck). The TS engine must
// match the Go checker row-for-row.
//
// The fixtures + runner live in ./check-fixture.ts (shared with the
// cross-engine differential dumper, so the two never drift).
import { describe, it } from 'node:test'
import { strict as assert } from 'node:assert'
import * as fs from 'node:fs'
import * as path from 'node:path'

import { CHECK_SPEC_DIR, parseCheckSpec, runCheckRow } from './check-fixture.ts'

const specFiles = fs
  .readdirSync(CHECK_SPEC_DIR)
  .filter((f) => f.endsWith('.tsv'))
  .sort()

describe('check', () => {
  for (const file of specFiles) {
    describe(file.replace(/\.tsv$/, ''), () => {
      for (const row of parseCheckSpec(path.join(CHECK_SPEC_DIR, file))) {
        it(`L${row.lineNum} ${row.input}`, () => {
          if (row.expected.startsWith('ERROR:')) {
            const want = row.expected.slice('ERROR:'.length)
            assert.throws(
              () => runCheckRow(row.input),
              (e: Error) => want === '' || e.message.toLowerCase().includes(want.toLowerCase()),
            )
            return
          }
          assert.equal(runCheckRow(row.input), row.expected)
        })
      }
    })
  }
})
