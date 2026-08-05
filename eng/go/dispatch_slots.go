package eng

// Engine dispatch-hook slots — Stage 3b of the four-piece split
// (design/ENG-FOUR-PIECE.0.md seam S9). A compiler-piece behavior the
// core step loop must be able to OFFER without naming compiler symbols
// registers itself here at init; a nil slot simply declines. At the
// package cut these become the compiler's registrations onto core's
// exported hook points.

// driftWindowRecorder is the compiler's stack-drift island hook: offered
// a matched dispatch whose forward window drifted, it may record the
// window as a runtime island (drift_window.go) and report true to skip
// the refusal path. Installed by the compiler piece's init; nil declines.
var driftWindowRecorder func(e *Engine, w WordInfo, sig *Signature, positions []int) bool
