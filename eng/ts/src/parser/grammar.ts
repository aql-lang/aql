// PORT STUB — replaced by the grammar.go port (see the port workflow).
// Contract: makeBoruJsonic() builds the configured jsonic instance with
// every boru token, matcher, and rule extension installed (stages 1-2
// of Go Parse), returning the instance plus the registered token ids.

/* eslint-disable @typescript-eslint/no-explicit-any */

export interface ParserTokens {
  OP: number // (
  CP: number // )
  DT: number // .
  SC: number // ;
  QM: number // ?
  BG: number // !
  PI: number // |
  AR: number // =>
  BT: number // ` (backtick)
  IS: number // ${ (interp start)
  TL: number // template literal segment
  LA: number // < (angle open)
  RA: number // > (angle close)
  ML: number // minilang literal (matcher-produced)
  XML: number // embedded XML literal (matcher-produced)
}

export function makeBoruJsonic(): { j: any; t: ParserTokens } {
  throw new Error('parser grammar port pending')
}
