// Cross-engine differential dumper.
//
// Emits one JSONL record per shared-corpus row so the Go cross-diff harness
// (test/go/engspec/crossdiff_test.go) can compare the TS engine's result to
// the Go engine's, row by row. The actual row computation lives in
// ./crossdiff-shared.ts (tsRows), shared with the TS-side diff test so the two
// never drift.
//
// Run standalone for inspection:  node --experimental-strip-types src/crossdump.ts
import { tsRows } from './crossdiff-shared.ts'

process.stdout.write(tsRows().map((r) => JSON.stringify(r)).join('\n') + '\n')
