// PORT STUB — replaced by the xml_literal.go port (see the port
// workflow). Contract: setupXmlMatcher registers the #XML lex matcher
// (armed only inside the xml rule via a keep-prop, producing one #XML
// token carrying an XmlElemVal); setupXmlGrammar wires the "xml" rule
// the val.Open `<` alternate pushes.

/* eslint-disable @typescript-eslint/no-explicit-any */

import type { ParserTokens } from './grammar.ts'

export function setupXmlMatcher(_j: any, _t: ParserTokens): void {
  // no-op until the port lands — `<tag …>` inputs fail to parse
}

export function setupXmlGrammar(_j: any, _t: ParserTokens): void {
  // no-op until the port lands
}
