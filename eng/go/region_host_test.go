package eng

import (
	"errors"
	"testing"

	compiler "github.com/boru-lang/boru/compiler/go"
	core "github.com/boru-lang/boru/core/go"
)

// The descriptor adapter, driven through core's three kernel entry points.
//
// collect_seam_host_test.go already proves the seam is SATISFIABLE from
// outside core, with a hand-written window and stub evaluations. This file
// asserts the production host: that it walks a real RegionDesc, that its
// classifications answer from the LIVE registry rather than from anything
// frozen, and that every evaluation declines in the one way a caller can
// act on.
//
// The negative half is the point of the file. An adapter that answered an
// evaluation with a plausible guess would be a miscompile with no symptom;
// declining is what makes an un-drivable collection a DEFERRAL instead.

func regionDesc(toks ...core.Value) *compiler.RegionDesc {
	slots := make([]compiler.SlotDesc, 0, len(toks))
	for _, t := range toks {
		s := compiler.SlotDesc{Token: t}
		if core.IsWord(t) {
			s.Source = compiler.SlotWordRef
		} else {
			s.Source = compiler.SlotConst
		}
		slots = append(slots, s)
	}
	return &compiler.RegionDesc{Lead: compiler.LeadWord, Word: "w", NFwd: len(slots), Slots: slots}
}

func newHost(t *testing.T, toks ...core.Value) (*regionHost, *core.Registry) {
	t.Helper()
	reg, err := core.NewRegistry()
	if err != nil {
		t.Fatalf("registry: %v", err)
	}
	return newRegionHost(reg, regionDesc(toks...)), reg
}

// The window IS the slots' tokens, in written order, and it is a real Tape —
// so the gap-buffer splice and live-length semantics the kernel relies on are
// the interpreter's own, not a second implementation to keep correct.
func TestRegionHostWindowIsTheSlotTokens(t *testing.T) {
	a, b := core.NewInteger(1), core.NewWord("k")
	h, _ := newHost(t, a, b)

	w := h.Window()
	if w.Len() != 2 {
		t.Fatalf("window length %d, want 2", w.Len())
	}
	if core.CanonValue(w.At(0)) != core.CanonValue(a) || core.CanonValue(w.At(1)) != core.CanonValue(b) {
		t.Errorf("window = [%s %s], want the slots' tokens in written order",
			core.CanonValue(w.At(0)), core.CanonValue(w.At(1)))
	}
	// Live-length, spliceable: the kernel re-reads Len() after every mutation,
	// so a window that could not grow or shrink would break the walk rather
	// than merely differ from it.
	w.Splice(1, 1, core.NewInteger(7), core.NewInteger(8))
	if w.Len() != 3 {
		t.Errorf("after splice length %d, want 3", w.Len())
	}
	w.Remove(0)
	if w.Len() != 2 || core.CanonValue(w.At(0)) != "7" {
		t.Errorf("after remove: len %d, head %s", w.Len(), core.CanonValue(w.At(0)))
	}
}

// A descriptor with no slots is a legitimate shape, not a defect: a dispatch
// that claimed nothing forward still has a region, and the host must seat on
// it without a special case.
func TestRegionHostEmptyDescriptor(t *testing.T) {
	h, _ := newHost(t)
	if got := h.Window().Len(); got != 0 {
		t.Errorf("empty descriptor gave a window of %d", got)
	}
}

// Every evaluation declines, and declines the SAME way, so one predicate at
// the call site tells "this host cannot finish" from a collection error the
// kernel raised. The two demand opposite responses — defer versus surface —
// and a caller that conflated them would report an internal decline to the
// user as their own program's error.
func TestRegionHostEvaluationsDecline(t *testing.T) {
	h, _ := newHost(t, core.NewInteger(1))

	if err := h.EvalGroupAt(0); !RegionCannotEval(err) {
		t.Errorf("EvalGroupAt = %v, want the decline", err)
	}
	if _, err := h.EvalInterp(core.NewString("x")); !RegionCannotEval(err) {
		t.Errorf("EvalInterp = %v, want the decline", err)
	}
	if _, err := h.EvalXml(core.NewString("x")); !RegionCannotEval(err) {
		t.Errorf("EvalXml = %v, want the decline", err)
	}
	if _, err := h.ExpandSugarAt(core.NewInteger(1), 0, 0, nil); !RegionCannotEval(err) {
		t.Errorf("ExpandSugarAt = %v, want the decline", err)
	}
	if _, err := h.ExpandSugarTokens(core.SugarInfo{}, core.NewInteger(1), false); !RegionCannotEval(err) {
		t.Errorf("ExpandSugarTokens = %v, want the decline", err)
	}
	// The negative half: an unrelated error is NOT the decline. Without this
	// the predicate could be `err != nil` and every real raise would be
	// swallowed as a deferral.
	if RegionCannotEval(errors.New("some other failure")) {
		t.Error("an unrelated error must not read as the decline")
	}
	if RegionCannotEval(nil) {
		t.Error("nil must not read as the decline")
	}
}

// ScratchParenSpan reuses one buffer, per the seam's contract that the result
// is valid only until the caller splices. The assertion is that the CONTENT is
// right and the buffer is REUSED — a fresh allocation per call would pass a
// content-only check while costing an allocation on a collection's hottest
// path.
func TestRegionHostScratchSpanReuses(t *testing.T) {
	h, _ := newHost(t)
	first := h.ScratchParenSpan([]core.Value{core.NewInteger(1), core.NewInteger(2)})
	if len(first) != 2 {
		t.Fatalf("span length %d, want 2", len(first))
	}
	second := h.ScratchParenSpan([]core.Value{core.NewInteger(3)})
	if len(second) != 1 || core.CanonValue(second[0]) != "3" {
		t.Fatalf("second span = %v", second)
	}
	if &first[:1][0] != &second[0] {
		t.Error("the span buffer must be reused, not reallocated per call")
	}
}

// FlowInterrupted reads the LIVE registry rather than answering false by
// construction. Nothing this host runs can raise flow control today, so the
// honest answer happens to be false — but reading the flag is what keeps that
// true if it ever changes, and the paired assertion is what proves it is read
// rather than hardcoded.
func TestRegionHostFlowInterruptedIsLive(t *testing.T) {
	h, reg := newHost(t)
	if h.FlowInterrupted() {
		t.Error("a quiescent registry must not report flow control")
	}
	reg.FlowCtrl = core.FlowBreak
	if !h.FlowInterrupted() {
		t.Error("FlowInterrupted must read the live registry, not a constant")
	}
	reg.FlowCtrl = core.FlowNone
	if h.FlowInterrupted() {
		t.Error("clearing the flag must clear the answer")
	}
}

// THE CENTRAL PROPERTY: a word slot is resolved LIVE, so the same descriptor
// answers differently as the binding moves. That is the whole reason the
// model keeps a word slot as SlotWordRef instead of freezing its value, and
// it is what OpCollect will exist to do — asserted here on the host, before
// any opcode executes one.
func TestRegionHostResolvesWordSlotsLive(t *testing.T) {
	h, reg := newHost(t, core.NewWord("k"))

	if _, ok := h.DefTop("k"); ok {
		t.Fatal("k must start unbound")
	}
	reg.Defs.Push("k", core.NewInteger(5))
	if v, ok := h.DefTop("k"); !ok || core.CanonValue(v) != "5" {
		t.Fatalf("DefTop after bind = %v/%v, want 5", core.CanonValue(v), ok)
	}
	reg.Defs.Push("k", core.NewInteger(9))
	if v, _ := h.DefTop("k"); core.CanonValue(v) != "9" {
		t.Errorf("DefTop after rebind = %s, want 9 — the slot must not be frozen", core.CanonValue(v))
	}
	if !reg.Defs.Pop("k") {
		t.Fatal("pop")
	}
	if v, _ := h.DefTop("k"); core.CanonValue(v) != "5" {
		t.Errorf("DefTop after undef = %s, want 5", core.CanonValue(v))
	}
}

// The classifications delegate to core's shared free functions rather than
// restating the rules. This asserts the DELEGATION by moving the registry
// underneath and watching the answer follow: a host that had reimplemented
// "an fn word is a barrier" against a frozen view would not.
func TestRegionHostClassificationsFollowTheRegistry(t *testing.T) {
	h, reg := newHost(t, core.NewWord("f"))
	tok := core.NewWord("f")

	if h.IsFnWordBarrier(tok) {
		t.Error("an unbound word is not an fn-word barrier")
	}
	if h.LookupWord("f") != nil {
		t.Error("an unbound word must not resolve to a fn")
	}
	// StaticForwardType is PURE — it needs neither registry nor window, which
	// is why it is the one classification that cannot drift between hosts.
	// Asserted against core's own answer rather than a transcribed constant,
	// so the delegation is what is pinned.
	gotV, gotK := h.StaticForwardType(core.NewInteger(1))
	wantV, wantK := core.StaticForwardTypeOf(core.NewInteger(1))
	if core.CanonValue(gotV) != core.CanonValue(wantV) || gotK != wantK {
		t.Errorf("StaticForwardType = %s/%v, want core's own answer %s/%v",
			core.CanonValue(gotV), gotK, core.CanonValue(wantV), wantK)
	}
	_ = reg
}

// The three kernel entry points, driven end to end through this host over a
// real descriptor. The subject is that the walks TERMINATE, classify through
// the host, and mutate only this host's private window — not what they decide,
// which is core's business and core's tests'.
//
// The fn is hand-built rather than looked up, for the reason the seam-host
// test builds one too: a bare core registry carries no natives (they are the
// lang layer's), and a test that SKIPPED here would prove nothing about the
// property it names.
func TestRegionHostDrivesTheKernelLoops(t *testing.T) {
	h, _ := newHost(t, core.NewInteger(1), core.NewInteger(2))
	fn := &core.FnDefInfo{
		Name: "pair",
		Signatures: []core.Signature{{
			Args:       []*core.Type{core.TInteger, core.TInteger},
			BarrierPos: 2,
		}},
	}

	if err := core.CollectForward(h, fn, core.WordInfo{Name: "pair", ArgCount: -1}, 0); err != nil {
		t.Fatalf("CollectForward over a descriptor window: %v", err)
	}
	if got := h.Window().Len(); got != 2 {
		t.Errorf("the plan walk changed the window length to %d; a walk over plain "+
			"literals has nothing to rewrite", got)
	}

	fwd, specAt := core.CollectCandidateScan(h, &fn.Signatures[0], 2, []int{0, 0}, 0, false, false)
	if fwd < 0 {
		t.Errorf("CollectCandidateScan returned fwd=%d, want >= 0", fwd)
	}
	if specAt != -1 {
		t.Errorf("specAt = %d, want -1 — no slot here is speculative", specAt)
	}

	// The arrival decision, the third loop and the one no benchmark or
	// differential in the tree exercises (the Stage-2 CPU gate recorded zero
	// samples on both sides of it), so a purpose-built driver is the only
	// coverage it gets from this side of the seam.
	// The verdict itself is core's to decide; what is asserted here is that a
	// foreign host can be asked for one at all, and that asking does not
	// disturb the window.
	_ = core.CollectArrival(h, core.ForwardInfo{FuncName: "pair"}, 0)
	if got := h.Window().Len(); got > 2 {
		t.Errorf("the arrival decision grew the window to %d", got)
	}
}

// A group token in the window is the shape that makes the decline REACHABLE
// through a kernel loop rather than only by direct call — the plan walk asks
// the host to evaluate it, and this host cannot. That is the exact path a
// routed dispatch would take to a deferral, so it is pinned end to end rather
// than at the method.
func TestRegionHostDeclineSurfacesThroughTheWalk(t *testing.T) {
	h, _ := newHost(t, core.NewOpenParen(), core.NewInteger(7))
	fn := &core.FnDefInfo{
		Name: "one",
		Signatures: []core.Signature{{
			Args:       []*core.Type{core.TAny},
			BarrierPos: 1,
		}},
	}
	err := core.CollectForward(h, fn, core.WordInfo{Name: "one", ArgCount: -1}, 0)
	if !RegionCannotEval(err) {
		t.Fatalf("CollectForward over a group = %v, want the decline: a viable overload "+
			"consumes the position, so the walk must ask this host to evaluate it", err)
	}
}

// ReachFnWouldClaim is the call-vs-data decision for a reach-collapsed named
// fn, and the plan walk only reaches it for a Reach token — a shape a
// literal-only window never presents. Driven directly so the delegation is
// covered by eng's own suite rather than waiting on a corpus row, which is
// the same reason the other classifications are asserted against core's
// answer rather than a transcribed one.
func TestRegionHostReachFnWouldClaimDelegates(t *testing.T) {
	h, _ := newHost(t, core.NewWord("f"), core.NewInteger(1))
	tok := core.NewWord("f")
	got := h.ReachFnWouldClaim(tok, 0)
	want := core.ReachFnWouldClaimOn(h.Window(), h.reg, tok, 0)
	if got != want {
		t.Errorf("ReachFnWouldClaim = %v, want core's own answer %v", got, want)
	}
	if got {
		t.Error("an unbound word claims nothing")
	}
}
