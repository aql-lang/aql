package parser

import engparser "github.com/metsitaba/voxgig-exp/eng/parser"

// Parse re-exports the canonical AQL parser, which now lives in the
// standalone eng module (github.com/metsitaba/voxgig-exp/eng/parser).
// eng is dependency-free apart from jsonic, so the production parser
// can be used by anything that depends on eng — including eng's own
// kernel spec runner.
//
// The legacy hand-rolled parser still in this package (Parser / New /
// ParseProgram, used only by internal/evaluator's tests) is a
// separate, unrelated implementation kept for those tests.
var Parse = engparser.Parse
