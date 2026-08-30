package eng

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// The collection seam is exported (core/go/collect_kernel.go, Stage 4) so a
// SECOND host can implement it — the VM's region-descriptor adapter. This
// file is the proof that the export is actually sufficient from OUTSIDE
// core: everything the three loops need is reachable with core's public
// surface alone, and nothing here touches *Engine or *Tape.
//
// It lives in eng rather than core deliberately. A core-internal fake could
// satisfy the interface using unexported helpers and prove nothing about
// cross-module implementability, which is the only property at issue.

// seamWindow is a CollectWindow over a plain slice — the shape the VM's
// adapter will have, since OpCollect materialises its window from a
// RegionDesc's Slots[].Token rather than owning a tape.
type seamWindow struct{ toks []core.Value }

func (w *seamWindow) Len() int                { return len(w.toks) }
func (w *seamWindow) At(i int) core.Value     { return w.toks[i] }
func (w *seamWindow) Set(i int, v core.Value) { w.toks[i] = v }
func (w *seamWindow) Remove(i int)            { w.toks = append(w.toks[:i], w.toks[i+1:]...) }

func (w *seamWindow) Splice(i, count int, repl ...core.Value) {
	tail := append([]core.Value(nil), w.toks[i+count:]...)
	w.toks = append(append(w.toks[:i:i], repl...), tail...)
}

// seamHost implements core.CollectHost against a registry and a slice
// window. The classification half is registry + window reads; the
// evaluation half records that it was asked, which is all this test needs —
// the VM's real adapter runs a compiled fragment there.
type seamHost struct {
	reg  *core.Registry
	win  *seamWindow
	span []core.Value

	calls map[string]int
}

func (h *seamHost) note(name string) { h.calls[name]++ }

func (h *seamHost) Window() core.CollectWindow { h.note("Window"); return h.win }

func (h *seamHost) EvalGroupAt(i int) error {
	h.note("EvalGroupAt")
	// A group that collapses to nothing: the shape the extent walk must
	// tolerate, and the reason the window is live-length.
	h.win.Remove(i)
	return nil
}

func (h *seamHost) EvalInterp(tok core.Value) (core.Value, error) {
	h.note("EvalInterp")
	return core.NewString("interp"), nil
}

func (h *seamHost) EvalXml(tok core.Value) (core.Value, error) {
	h.note("EvalXml")
	return core.NewString("xml"), nil
}

func (h *seamHost) ExpandSugarAt(tok core.Value, pos, i int, viable []core.ViableSig) (bool, error) {
	h.note("ExpandSugarAt")
	return false, nil
}

func (h *seamHost) FlowInterrupted() bool { h.note("FlowInterrupted"); return false }

func (h *seamHost) ScratchParenSpan(items []core.Value) []core.Value {
	h.note("ScratchParenSpan")
	h.span = append(h.span[:0], items...)
	return h.span
}

func (h *seamHost) DefTop(name string) (core.Value, bool) {
	h.note("DefTop")
	return h.reg.Defs.Top(name)
}

func (h *seamHost) IsFnWordBarrier(tok core.Value) bool {
	h.note("IsFnWordBarrier")
	wi, err := core.AsWord(tok)
	return err == nil && h.reg.Lookup(wi.Name) != nil && !wi.ForceVal
}

func (h *seamHost) IsReachCallHead(tok core.Value, viable []core.ViableSig, pos, i int) bool {
	h.note("IsReachCallHead")
	return false
}

func (h *seamHost) StaticForwardType(tok core.Value) (core.Value, core.FwdKind) {
	h.note("StaticForwardType")
	switch {
	case core.IsOpenParen(tok) || core.IsParenExpr(tok) || core.IsReach(tok):
		return core.Value{}, core.FwdGroup
	case core.IsWord(tok):
		return core.Value{}, core.FwdBoundary
	case core.IsConcrete(tok):
		return tok, core.FwdValue
	}
	return core.Value{}, core.FwdBoundary
}

func (h *seamHost) LookupWord(name string) *core.FnDefInfo {
	h.note("LookupWord")
	return h.reg.Lookup(name)
}

func (h *seamHost) ReachFnWouldClaim(tok core.Value, i int) bool {
	h.note("ReachFnWouldClaim")
	return false
}

func (h *seamHost) ExpandSugarTokens(sinfo core.SugarInfo, tok core.Value, head bool) ([]core.Value, error) {
	h.note("ExpandSugarTokens")
	return nil, nil
}

// Compile-time proof: the seam is satisfiable from outside core.
var _ core.CollectHost = (*seamHost)(nil)

func newSeamHost(t *testing.T, toks ...core.Value) *seamHost {
	t.Helper()
	reg, err := core.NewRegistry()
	if err != nil {
		t.Fatalf("registry: %v", err)
	}
	return &seamHost{
		reg:   reg,
		win:   &seamWindow{toks: toks},
		calls: map[string]int{},
	}
}

// TestSeamHostDrivesForward runs the PHASE-1 plan walk on a foreign host.
// A two-arg signature with a forward barrier collects the literals after
// the word, which is the ordinary shape OpCollect will replay.
func TestSeamHostDrivesForward(t *testing.T) {
	h := newSeamHost(t, core.NewInteger(1), core.NewInteger(2))
	fn := &core.FnDefInfo{
		Name: "pair",
		Signatures: []core.Signature{{
			Args:       []*core.Type{core.TInteger, core.TInteger},
			BarrierPos: 2,
		}},
	}
	if err := core.CollectForward(h, fn, core.WordInfo{Name: "pair", ArgCount: -1}, 0); err != nil {
		t.Fatalf("CollectForward: %v", err)
	}
	if h.calls["Window"] == 0 {
		t.Error("the walk never asked the host for its window")
	}
	if h.calls["StaticForwardType"] == 0 {
		t.Error("the walk never classified a token through the host")
	}
}

// TestSeamHostEvaluatesAGroup proves the EVALUATION half is reachable from a
// foreign host, and that a group collapsing to ZERO values is tolerated —
// the case the live-length window exists for.
func TestSeamHostEvaluatesAGroup(t *testing.T) {
	h := newSeamHost(t, core.NewOpenParen(), core.NewInteger(7))
	fn := &core.FnDefInfo{
		Name: "one",
		Signatures: []core.Signature{{
			Args:       []*core.Type{core.TAny},
			BarrierPos: 1,
		}},
	}
	if err := core.CollectForward(h, fn, core.WordInfo{Name: "one", ArgCount: -1}, 0); err != nil {
		t.Fatalf("CollectForward: %v", err)
	}
	if h.calls["EvalGroupAt"] == 0 {
		t.Fatal("a viable overload consumed the group position but the host was never asked to evaluate it")
	}
}

// TestSeamHostDrivesCandidateScan and TestSeamHostDrivesArrival reach the two
// loops CollectForward does not, so every host method has a foreign caller.
func TestSeamHostDrivesCandidateScan(t *testing.T) {
	h := newSeamHost(t, core.NewWord("w"), core.NewInteger(3))
	sig := &core.Signature{Args: []*core.Type{core.TAny}, BarrierPos: 1}
	fwd, _ := core.CollectCandidateScan(h, sig, 1, []int{0}, 0, false, false)
	if fwd < 0 {
		t.Fatalf("CollectCandidateScan returned fwd=%d, want >= 0", fwd)
	}
	if h.calls["Window"] == 0 {
		t.Error("the candidate scan never asked the host for its window")
	}
}

func TestSeamHostDrivesArrival(t *testing.T) {
	h := newSeamHost(t, core.NewInteger(4))
	// Sig is load-bearing: the arrival decision asks it whether the value
	// fills the pending slot, so a ForwardInfo without one is malformed
	// rather than merely sparse.
	sig := &core.Signature{Args: []*core.Type{core.TInteger}, BarrierPos: 1}
	v := core.CollectArrival(h, core.ForwardInfo{
		FuncName:     "f",
		ExpectedArgs: 1,
		Sig:          sig,
	}, 0)
	switch v {
	case core.ArrivalCollect, core.ArrivalDispatchFn, core.ArrivalBarrierClose, core.ArrivalImplicitEnd:
	default:
		t.Fatalf("CollectArrival returned an unnamed verdict %v", v)
	}
}
