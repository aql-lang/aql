package core

import "testing"

// hazardEmit is the inactive recorder plus the collection-hazard note
// (NUR121): it records what noteCollectionHazards marks and answers the
// paren classifier's CollectionHazard read from the same set.
type hazardEmit struct {
	EmitRecorder
	active  bool
	marked  map[string]bool
	leadOK  bool
	applyOK bool
}

func newHazardEmit() *hazardEmit {
	return &hazardEmit{EmitRecorder: TheInactiveEmit, active: true, marked: map[string]bool{}, leadOK: true, applyOK: true}
}

func (h *hazardEmit) Active() bool                   { return h.active }
func (h *hazardEmit) NoteCollectionHazard(id string) { h.marked[id] = true }
func (h *hazardEmit) CollectionHazard(id string) bool {
	return h.marked[id]
}
func (h *hazardEmit) DynApplyLeadEligible(Value) bool { return h.leadOK }
func (h *hazardEmit) RecordDynApply(args []Value, fn, out Value, pos SrcPos) (int, bool) {
	return len(args), h.applyOK
}

func hazardEngine(t *testing.T, es EmitRecorder) *Engine {
	t.Helper()
	r := covRegistry(t, nil)
	r.Check.Emit = es
	return NewTop(r)
}

// TestNoteCollectionHazardsScope pins the scan's SCOPE: an unapplied
// fn-typed value below the lowest stack-collected index is marked; an open
// paren between them seals it off; a frame's resolved-argument prefix
// (FrameOpenInfo.ArgSpan) and the run's StartAt prefix are never marked
// (arguments are inert); a quoted fn and a plain value are not fn leads.
func TestNoteCollectionHazardsScope(t *testing.T) {
	fn := NewCarrier(TFunction)
	dyn := NewDynamicCarrier(TAny)
	quoted := NewCarrier(TFunction)
	quoted.Quoted = true
	five := NewInteger(5)

	t.Run("marks the lead below the collected value", func(t *testing.T) {
		es := newHazardEmit()
		e := hazardEngine(t, es)
		// [fn dyn 5 ^word]: the word stack-collects 5 (index 2); both fn-shaped
		// values below it are marked.
		e.Tape = NewTape([]Value{fn, dyn, five, NewWord("add")}, StackHeadroom)
		e.Pointer = 3
		e.noteCollectionHazards(nil, []int{2})
		if !es.marked[fn.ID] || !es.marked[dyn.ID] {
			t.Errorf("both fn-shaped values below the collected index must be marked: %v", es.marked)
		}
	})
	t.Run("an open paren seals the scope", func(t *testing.T) {
		es := newHazardEmit()
		e := hazardEngine(t, es)
		e.Tape = NewTape([]Value{fn, NewOpenParen(), five, NewWord("add")}, StackHeadroom)
		e.Pointer = 3
		e.noteCollectionHazards(nil, []int{2})
		if len(es.marked) != 0 {
			t.Errorf("a lead outside the paren scope must not be marked: %v", es.marked)
		}
	})
	t.Run("a frame's argument prefix is inert", func(t *testing.T) {
		es := newHazardEmit()
		e := hazardEngine(t, es)
		// FrameOpen(ArgSpan 1) fn 5 ^word: fn is the spliced unnamed argument.
		e.Tape = NewTape([]Value{NewFrameOpenSpan(&FnFrameMeta{Name: "f"}, 1), fn, five, NewWord("add")}, StackHeadroom)
		e.Pointer = 3
		e.noteCollectionHazards(nil, []int{2})
		if len(es.marked) != 0 {
			t.Errorf("a frame-prefix argument must not be marked: %v", es.marked)
		}
	})
	t.Run("the run's StartAt prefix is inert", func(t *testing.T) {
		es := newHazardEmit()
		e := hazardEngine(t, es)
		e.Tape = NewTape([]Value{fn, five, NewWord("add")}, StackHeadroom)
		e.Pointer = 2
		e.inertPrefix = 1
		e.noteCollectionHazards(nil, []int{1})
		if len(es.marked) != 0 {
			t.Errorf("a StartAt-prefix argument must not be marked: %v", es.marked)
		}
	})
	t.Run("quoted and plain values are not leads", func(t *testing.T) {
		es := newHazardEmit()
		e := hazardEngine(t, es)
		e.Tape = NewTape([]Value{quoted, NewInteger(1), five, NewWord("add")}, StackHeadroom)
		e.Pointer = 3
		e.noteCollectionHazards(nil, []int{2})
		if len(es.marked) != 0 {
			t.Errorf("nothing to mark: %v", es.marked)
		}
	})
	t.Run("forward-only collection marks nothing", func(t *testing.T) {
		es := newHazardEmit()
		e := hazardEngine(t, es)
		e.Tape = NewTape([]Value{fn, NewWord("add"), five}, StackHeadroom)
		e.Pointer = 1
		e.noteCollectionHazards(nil, nil)
		e.noteCollectionHazards(nil, []int{2})
		if len(es.marked) != 0 {
			t.Errorf("a forward-collected index consumes nothing below the word: %v", es.marked)
		}
	})
	t.Run("an inactive recorder marks nothing", func(t *testing.T) {
		es := newHazardEmit()
		es.active = false
		e := hazardEngine(t, es)
		e.Tape = NewTape([]Value{fn, five, NewWord("add")}, StackHeadroom)
		e.Pointer = 2
		e.noteCollectionHazards(nil, []int{1})
		if len(es.marked) != 0 {
			t.Errorf("inactive: %v", es.marked)
		}
	})
	t.Run("a strip-input hop is transparent", func(t *testing.T) {
		es := newHazardEmit()
		e := hazardEngine(t, es)
		e.Tape = NewTape([]Value{fn, five, NewWord("error")}, StackHeadroom)
		e.Pointer = 2
		strip := &Signature{Callable: &CallableSpec{StripsUnconsumedInput: true}}
		e.noteCollectionHazards(strip, []int{1})
		if len(es.marked) != 0 {
			t.Errorf("a strip-input hop passes the region through and marks nothing: %v", es.marked)
		}
		plain := &Signature{Callable: &CallableSpec{}}
		e.noteCollectionHazards(plain, []int{1})
		if !es.marked[fn.ID] {
			t.Errorf("an ordinary collection past the lead marks it: %v", es.marked)
		}
	})
	t.Run("a full-stack scope floor bounds the scan", func(t *testing.T) {
		es := newHazardEmit()
		e := hazardEngine(t, es)
		other := NewCarrier(TFunction)
		e.Tape = NewTape([]Value{other, fn, five, NewWord("depth")}, StackHeadroom)
		e.Pointer = 3
		e.noteCollectionHazardsBelow(1, 3)
		if es.marked[other.ID] || !es.marked[fn.ID] {
			t.Errorf("only values at or above the floor are in scope: %v", es.marked)
		}
	})
}

// TestParenLeadFnApplyIdxDeclinesAHazardLead pins the classifier's consumer
// half: a lead the scan marked declines the window (`(g x add 1)` — the
// window's argument is add's result), an unmarked twin admits.
func TestParenLeadFnApplyIdxDeclinesAHazardLead(t *testing.T) {
	es := newHazardEmit()
	e := hazardEngine(t, es)
	lead := NewCarrier(TFunction)
	e.Tape = NewTape([]Value{NewOpenParen(), lead, NewInteger(5), NewCloseParen()}, StackHeadroom)
	if got := e.parenLeadFnApplyIdx(es, 0, 3, 2, 2); got != 1 {
		t.Fatalf("an unmarked lead admits the window, got %d", got)
	}
	es.marked[lead.ID] = true
	if got := e.parenLeadFnApplyIdx(es, 0, 3, 2, 2); got != -1 {
		t.Errorf("a hazard-marked lead must decline the window, got %d", got)
	}
}
