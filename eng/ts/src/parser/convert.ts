// PORT STUB — replaced by the parse.go port (see the port workflow).
// Contract: parse(src) runs the configured jsonic instance
// (grammar.ts makeBoruJsonic) over src and converts the jsonic output
// into engine Values — stage 3 of Go Parse — throwing BoruError on
// syntax errors (via errors.ts translateParseError).

import type { Value } from '../value.ts'

export function parse(_src: string): Value[] {
  throw new Error('parser conversion port pending')
}
