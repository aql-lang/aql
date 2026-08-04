// PORT STUB — replaced by the parse_error.go port (see the port
// workflow). Contract: translateParseError converts a thrown
// JsonicError (or any unknown failure) into the boru syntax_error
// taxonomy, mirroring eng/go/parser/parse_error.go.

import { BoruError } from '../error.ts'

export function translateParseError(e: unknown, _src: string): BoruError {
  if (e instanceof BoruError) return e
  return new BoruError('syntax_error', e instanceof Error ? e.message : String(e), '')
}
