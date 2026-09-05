package compiler

import (
	"strings"
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// wordReadUnit opens a user-fn unit over one fn-typed param `g` and one
// gradual `x:Any` param on an active state, returning the state, the unit's
// record and the two param carriers (the frame locals a body reads).
func wordReadUnit(t *testing.T) (*EmitState, *fnUnitRec, core.Value, core.Value) {
	t.Helper()
	es := NewEmitState()
	g := core.NewCarrier(core.TFunction)
	x := core.NewDynamicCarrier(core.TAny)
	unit, _, ok := es.StartFnCompile("k", "f", nil, []core.Value{g, x}, []*core.Type{core.TAny}, []string{"g", "x"}, nil, false, core.SrcPos{})
	if !ok || unit < 0 {
		t.Fatalf("StartFnCompile declined: %d %v", unit, ok)
	}
	return es, es.fnRecs[unit], g, x
}

// TestNoteWordReadArms pins the recorder's counting (NUR123): an inactive
// state, an empty id or name, no open unit and a non-local value all note
// nothing; a fn-typed local counts strictly, a gradual one only names
// itself; a `/v` read counts on the unit.
func TestNoteWordReadArms(t *testing.T) {
	es := NewEmitState()
	es.NoteWordRead(core.NewCarrier(core.TFunction), "g", core.SrcPos{})
	es.NoteValRead("id")
	if len(es.fnRecs) != 0 {
		t.Fatal("no open unit: nothing to record on")
	}
	es, rec, g, x := wordReadUnit(t)
	es.NoteWordRead(core.Value{}, "g", core.SrcPos{})
	es.NoteWordRead(g, "", core.SrcPos{})
	es.NoteWordRead(core.NewCarrier(core.TFunction), "other", core.SrcPos{})
	es.NoteValRead("")
	if rec.wordReadNames != nil || rec.valReads != nil {
		t.Errorf("empty ids/names and a non-local value note nothing: %v %v", rec.wordReadNames, rec.valReads)
	}
	es.NoteWordRead(g, "g", core.SrcPos{Row: 1, Col: 7})
	es.NoteWordRead(x, "x", core.SrcPos{Row: 1, Col: 9})
	if rec.wordReads[g.ID] != 1 || rec.wordReads[x.ID] != 0 {
		t.Errorf("only the fn-typed read is accounted strictly: %v", rec.wordReads)
	}
	if rec.wordReadNames[x.ID] != "x" || rec.wordReadPos[g.ID].Col != 7 {
		t.Errorf("both reads keep their name and position: %v %v", rec.wordReadNames, rec.wordReadPos)
	}
	es.NoteValRead(g.ID)
	if rec.valReads[g.ID] != 1 {
		t.Errorf("a /v read counts: %v", rec.valReads)
	}
	es.Compilable = false
	es.NoteWordRead(g, "g", core.SrcPos{})
	es.NoteValRead(g.ID)
	if rec.wordReads[g.ID] != 1 || rec.valReads[g.ID] != 1 {
		t.Error("an inactive state records nothing")
	}
}

// TestCreditWordReadArms pins the apply-lowering credit: a nil state, an
// empty id, no open unit and an id never read bare credit nothing; a read
// id credits once per lowering (RecordDynApply and RegisterTrailingApply
// both route here).
func TestCreditWordReadArms(t *testing.T) {
	var nilES *EmitState
	nilES.creditWordRead("x")
	es := NewEmitState()
	es.creditWordRead("x")
	es, rec, g, _ := wordReadUnit(t)
	es.creditWordRead("")
	es.creditWordRead(g.ID)
	if rec.wordReadCredit != nil {
		t.Errorf("an id never read bare takes no credit: %v", rec.wordReadCredit)
	}
	es.NoteWordRead(g, "g", core.SrcPos{})
	es.RegisterTrailingApply(g.ID, 1)
	if _, ok := es.RecordDynApply([]core.Value{core.NewInteger(5)}, g, core.NewCarrier(core.TInteger), core.SrcPos{}); !ok {
		t.Fatal("a plain paren apply over a carrier lead records")
	}
	if rec.wordReadCredit[g.ID] != 2 {
		t.Errorf("each apply lowering credits one read: %v", rec.wordReadCredit)
	}
}

// TestWordReadAccounting pins the unit-finish rule: no reads pass; a read
// consumed nowhere the replay or an apply lowering saw refuses (a container
// member, an arm residual); a credited read passes; a read seated in the
// replay window passes; an id read both bare and by /v refuses.
func TestWordReadAccounting(t *testing.T) {
	es, rec, g, x := wordReadUnit(t)
	if r := es.wordReadAccounting(rec); r != "" {
		t.Errorf("no reads: %q", r)
	}
	es.NoteWordRead(g, "g", core.SrcPos{})
	if r := es.wordReadAccounting(rec); !strings.Contains(r, "consumed where the interpreter dispatches it") {
		t.Errorf("an unseated fn-typed read refuses: %q", r)
	}
	es.creditWordRead(g.ID)
	if r := es.wordReadAccounting(rec); r != "" {
		t.Errorf("a credited read passes: %q", r)
	}
	es.NoteWordRead(g, "g", core.SrcPos{})
	rec.outOpsVals = []core.Value{x, g}
	rec.dynFrameW = 2
	rec.dynFrameWords = []DynFrameWord{{}, {Name: "g"}}
	if r := es.wordReadAccounting(rec); r != "" {
		t.Errorf("a read seated in the replay window passes: %q", r)
	}
	es.NoteValRead(g.ID)
	if r := es.wordReadAccounting(rec); !strings.Contains(r, "read both bare and by /v") {
		t.Errorf("a mixed read refuses: %q", r)
	}
}

// TestWordReadReplayArms pins wordReadName, dynFrameWordsFor,
// replayValueApplicables and noteWordReadReplay: a nil record, a quoted
// value and an apply-pending id are not word reads; a residual with no
// word read arms nothing; a gradual read the window cannot seat keeps the
// slot push (true, unarmed); a fn-typed one refuses (false); a seatable
// read arms the replay with the word table.
func TestWordReadReplayArms(t *testing.T) {
	es, rec, g, x := wordReadUnit(t)
	u := es.units[len(es.units)-1]
	if es.wordReadName(nil, g) != "" || es.wordReadName(rec, g) != "" {
		t.Error("a nil record and an un-noted value are not word reads")
	}
	es.NoteWordRead(g, "g", core.SrcPos{Row: 1, Col: 5})
	es.NoteWordRead(x, "x", core.SrcPos{Row: 1, Col: 8})
	quoted := g
	quoted.Quoted = true
	if es.wordReadName(rec, quoted) != "" {
		t.Error("a quoted value is data")
	}
	u.pendingApply = append(u.pendingApply, x.ID)
	if es.wordReadName(rec, x) != "" {
		t.Error("an apply-pending id is the apply word's")
	}
	u.pendingApply = nil
	if es.dynFrameWordsFor(u, rec, []core.Value{core.NewInteger(1)}) != nil {
		t.Error("a window without a word read has no table")
	}
	if !es.noteWordReadReplay(u, rec, []core.Value{core.NewInteger(1)}) || rec.dynFrameW != 0 {
		t.Error("no word read: nothing to arm, nothing to refuse")
	}
	// A window whose events run AFTER the read is not a body tail: a fragment
	// event positioned past the read.
	rec.frag = &EmitFragment{events: []EmitEvent{{seq: 1, kind: evCall, call: emitCall{pos: core.SrcPos{Row: 2, Col: 1}}}}}
	if !es.noteWordReadReplay(u, rec, []core.Value{x}) || rec.dynFrameW != 0 {
		t.Error("a gradual read the window cannot seat keeps the slot push, unarmed")
	}
	if es.noteWordReadReplay(u, rec, []core.Value{g}) {
		t.Error("a fn-typed read the window cannot seat refuses")
	}
	rec.frag = &EmitFragment{}
	names := es.dynFrameWordsFor(u, rec, []core.Value{core.NewInteger(1), g})
	if replayValueApplicables([]core.Value{core.NewInteger(1), g}, names) != 0 {
		t.Error("a word-read entry is not a value-semantics applicable")
	}
	if replayValueApplicables([]core.Value{core.NewCarrier(core.TFunction), g}, names) != 1 {
		t.Error("a value-read fn beside it is")
	}
	if !es.noteWordReadReplay(u, rec, []core.Value{core.NewInteger(1), g}) || rec.dynFrameW != 2 || !rec.retReplay ||
		rec.dynFrameWords[1].Name != "g" || rec.dynFrameWords[1].Pos.Col != 5 || rec.dynFrameWords[0].Name != "" {
		t.Errorf("a seatable read arms the replay with its word table: w=%d words=%v", rec.dynFrameW, rec.dynFrameWords)
	}
	// Two value-semantics applicables beside a word read decline.
	rec.dynFrameW = 0
	if es.noteWordReadReplay(u, rec, []core.Value{core.NewCarrier(core.TFunction), core.NewCarrier(core.TFunction), g}) {
		t.Error("two value applicables cannot be ordered by the flat re-step")
	}
	// An unnamed-param prefix swallowing the read leaves no window.
	rec.nUnnamed, rec.nParams = 2, 2
	if es.noteWordReadReplay(u, rec, []core.Value{g}) {
		t.Error("a read the prefix swallows has no window to seat in")
	}
}

// TestSeatDynFrameWords pins the unit-side seat: an empty table records
// nothing, a table keys the pc and allocates the map once.
func TestSeatDynFrameWords(t *testing.T) {
	var cf CompiledFn
	seatDynFrameWords(&cf, 3, nil)
	if cf.DynFrameWords != nil {
		t.Error("no words, no table")
	}
	seatDynFrameWords(&cf, 3, []DynFrameWord{{Name: "g"}})
	seatDynFrameWords(&cf, 7, []DynFrameWord{{Name: "x"}})
	if len(cf.DynFrameWords) != 2 || cf.DynFrameWords[3][0].Name != "g" || cf.DynFrameWords[7][0].Name != "x" {
		t.Errorf("each replay keys its own pc: %v", cf.DynFrameWords)
	}
}

// TestLamParamContract pins the closure unit's declared param contract: a
// nil signature carries none; a typed param keeps its type; a pattern-only
// param declares Any with its pattern.
func TestLamParamContract(t *testing.T) {
	if lamParamContract(nil) != nil {
		t.Error("a nil signature has no contract")
	}
	pat := core.NewInteger(7)
	sig := &core.Signature{Params: []core.FnParam{{Name: "n", Type: core.TInteger}, {Name: "p", Pattern: &pat}}}
	ps := lamParamContract(sig)
	if ps == nil || len(ps.Types) != 2 || !ps.Types[0].Equal(core.TInteger) || !ps.Types[1].Equal(core.TAny) ||
		ps.Patterns[0] != nil || ps.Patterns[1] == nil {
		t.Errorf("the contract is the declared types, Any for a pattern-only param, patterns kept: %+v", ps)
	}
}

// TestFnResidualReplayReasonArms pins the shared refusal site's three
// verdicts: a closure unit and a trailing apply take none; a fn-typed
// word read the window cannot seat (an event after the read) refuses with
// the NUR123 reason; a seated read passes the accounting.
func TestFnResidualReplayReasonArms(t *testing.T) {
	es, rec, g, _ := wordReadUnit(t)
	u := es.units[len(es.units)-1]
	rec.returns = []*core.Type{core.TAny}
	es.NoteWordRead(g, "g", core.SrcPos{Row: 1, Col: 5})
	op, _ := es.resolveOperand(g)
	ops := []EmitOperand{op}
	if r := es.fnResidualReplayReason(u, rec, []core.Value{g}, ops, 1); r != "" {
		t.Errorf("a trailing apply takes no verdict here: %q", r)
	}
	rec.closure = true
	if r := es.fnResidualReplayReason(u, rec, []core.Value{g}, ops, 0); r != "" {
		t.Errorf("a closure unit takes no verdict here: %q", r)
	}
	rec.closure = false
	rec.frag = &EmitFragment{events: []EmitEvent{{seq: 1, kind: evCall, call: emitCall{pos: core.SrcPos{Row: 2, Col: 1}}}}}
	if r := es.fnResidualReplayReason(u, rec, []core.Value{g}, ops, 0); !strings.Contains(r, "cannot seat (NUR123)") {
		t.Errorf("a fn-typed read after which the body still runs events cannot seat: %q", r)
	}
	rec.frag = &EmitFragment{}
	if r := es.fnResidualReplayReason(u, rec, []core.Value{g}, ops, 0); r != "" || rec.dynFrameW != 1 {
		t.Errorf("a seated read arms the replay and passes the accounting: %q w=%d", r, rec.dynFrameW)
	}
}
