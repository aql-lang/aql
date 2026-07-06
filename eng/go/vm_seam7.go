package eng

// screenResultsFn is a test seam (design/TEST-SEAMS.10.md) over
// (*vmContext).screenResults, the choke point every VM dispatch site funnels
// handler / island / closure results through to reject a tape-coupled token
// (Word/Mark/Move/Forward/OpenParen/Splice). No compiled-reachable handler or
// island can PRODUCE such a token — the emitter refuses fn-invoking /
// code-splicing words, and the interpreter re-steps a token rather than
// returning it — so the `return nil, err` arm at each island/handler call site
// is defensive-only and cannot be reached through valid input. The seam lets a
// test swap the screen to force the rejection and exercise those arms without a
// value the seal makes unconstructible. The default is the real method, so
// production dispatch is unchanged; tests restore it via t.Cleanup.
var screenResultsFn = (*vmContext).screenResults
