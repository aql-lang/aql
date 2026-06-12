package eng

import "fmt"

// This file is the single home of the fn-frame tape protocol: the
// marked open paren that starts a spliced fn-body frame, the canonical
// cleanup tail that ends it, and the Go-side expression of what the
// tail's markers do. Every site that splices a frame onto the tape
// (buildFnBodyHandler, compileFnDef via buildFnBodyHandler, and
// execFnDefSig's no-captured-registry branch) builds its frame from
// these pieces, so the on-tape frame shape cannot diverge between
// dispatch paths. See design/TCO-STAGED.0.md Stage 1.
//
// A frame on the tape is:
//
//	(ₘ  unnamed-args…  body…  __DC  __pa  undef n₁ … undef nₖ  [__RC]  )
//
// where (ₘ is a FrameOpenInfo-marked open paren, __DC is the
// DefCleanup marker (truncates body-local defs to the entry snapshot),
// __pa pops the per-call Args list and FnBaseline, the undef pairs
// tear down captures+params (reverse install order), and __RC is the
// ReturnCheck when returns are declared.

// FnFrameMeta identifies one compiled fn overload across the frames it
// splices. The SAME pointer is attached to the compiled Signature
// (Signature.FnFrame) and stamped on the open paren of every frame
// that signature splices, so "is this dispatch a self-recursive call
// of the enclosing frame's fn?" is a pointer comparison — no name
// lookup, no structural sig equality.
type FnFrameMeta struct {
	// Name is the fn's registered name; "<fn>" for frames spliced from
	// an unregistered Function value (execFnDefSig's splice branch).
	Name string
}

// fnValueFrameMeta marks frames spliced by execFnDefSig for a Function
// value dispatched straight off the tape. No registered Signature
// carries this meta, so such frames anchor probe scans but never
// satisfy a sig-identity gate.
var fnValueFrameMeta = &FnFrameMeta{Name: "<fn>"}

// FrameOpenInfo is the payload on a fn frame's open paren. The token
// remains an ordinary OpenParen for every structural purpose (IsOpenParen
// is Parent-based, rendering is "(", collapse removes it like any paren);
// the payload only adds provenance: which fn overload opened this frame.
type FrameOpenInfo struct {
	Meta *FnFrameMeta
}

// NewFrameOpen creates the open paren of a spliced fn-body frame,
// carrying the splicing overload's identity.
func NewFrameOpen(meta *FnFrameMeta) Value {
	return NewValueRaw(TOpenParen, FrameOpenInfo{Meta: meta})
}

// IsFrameOpen reports whether v is a fn frame's open paren (as opposed
// to a plain grouping paren).
func IsFrameOpen(v Value) bool {
	if !IsOpenParen(v) {
		return false
	}
	_, ok := v.Data.(FrameOpenInfo)
	return ok
}

// AsFrameOpen returns the FrameOpenInfo carried by a frame-open paren.
func AsFrameOpen(v Value) (FrameOpenInfo, error) {
	info, ok := v.Data.(FrameOpenInfo)
	if !ok {
		return FrameOpenInfo{}, fmt.Errorf("AsFrameOpen: not a frame-open paren (got %T)", v.Data)
	}
	return info, nil
}

// FrameTailSpec describes one frame's synthesized cleanup tail.
type FrameTailSpec struct {
	// Registry receives the DefCleanup truncation and __pa pops.
	Registry *Registry
	// Snapshot is the def-depth snapshot taken AFTER captures+params
	// were installed (so DefCleanup tears down only body-local defs —
	// and the generic type-param bindings installed after it, see
	// buildFnBodyHandler).
	Snapshot map[string]int
	// Names are the installed captures+params in install order; the
	// tail undefs them in reverse.
	Names []string
	// Returns / UnnamedCount / FuncName / Pos populate the ReturnCheck;
	// no ReturnCheck is emitted when Returns is empty. A zero Pos means
	// "stamp later": execMatch stamps handler-produced ReturnChecks
	// with the call-site position (stampResultPos keys on Pos.Row == 0).
	Returns      []*Type
	UnnamedCount int
	FuncName     string
	Pos          SrcPos
}

// AppendFrameTail appends the canonical frame cleanup tail to tokens:
// the DefCleanup marker, the __pa word, the undef pairs for
// captures+params (reverse install order, force-forward so undef takes
// the name word that follows rather than a same-typed value from the
// prefix stack), and the ReturnCheck when returns are declared. The
// frame's close paren is NOT appended — callers own the (ₘ … ) pair.
func AppendFrameTail(tokens []Value, spec FrameTailSpec) []Value {
	tokens = append(tokens, NewDefCleanup(DefCleanupInfo{
		Snapshot: spec.Snapshot,
		Registry: spec.Registry,
	}))
	tokens = append(tokens, NewWord("__pa"))
	for i := len(spec.Names) - 1; i >= 0; i-- {
		tokens = append(tokens,
			NewWordModified("undef", -1, false, true),
			NewWord(spec.Names[i]),
		)
	}
	if len(spec.Returns) > 0 {
		tokens = append(tokens, NewReturnCheck(ReturnCheckInfo{
			FuncName:     spec.FuncName,
			Returns:      spec.Returns,
			UnnamedCount: spec.UnnamedCount,
			Pos:          spec.Pos,
		}))
	}
	return tokens
}

// PopFrameArgs is the Go-side expression of the synthesized __pa
// token: pop the per-call Args list and, in lockstep, the enclosing-fn
// baseline pushed at fn entry (closure-capture detection on subsequent
// fn constructions reads the baseline, so the two must move together —
// see eng/go/CLAUDE.md "Per-Call Stacks"). The __pa word's handler
// delegates here; an eager frame teardown can call it directly.
func PopFrameArgs(r *Registry) error {
	if _, err := r.Args.Pop(); err != nil {
		return err
	}
	r.PopFnBaseline()
	return nil
}
