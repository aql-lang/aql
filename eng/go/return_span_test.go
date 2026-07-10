package eng

import (
	"strings"
	"testing"
)

// Phase-5 return errors carry two secondary spans — where the offending
// value was produced and where the contract was declared — paired with
// the negatives: no declaration site → no span (never guess), and a
// value produced at the error position is not re-labelled.

func TestReturnTypeErrorSpans(t *testing.T) {
	e := engWithTape(t, nil, 0)
	e.SetSource("def f fn [[n:Integer] String [n]]\n42 f")
	rc := ReturnCheckInfo{
		FuncName: "f",
		Returns:  []*Type{TString},
		Pos:      SrcPos{Row: 2, Col: 4, Src: "f"},
		Decl:     DeclSite{Pos: SrcPos{Row: 1, Col: 23, Src: "String"}},
	}
	got := NewInteger(42)
	got.pos = &SrcPos{Row: 2, Col: 1, Src: "42"}
	ae := e.returnTypeError(rc, 1, TString, got)
	if len(ae.Spans) != 2 {
		t.Fatalf("want 2 spans, got %+v", ae.Spans)
	}
	if ae.Spans[0].Label != "the returned value was produced here" || ae.Spans[0].Pos.Row != 2 {
		t.Errorf("value span = %+v", ae.Spans[0])
	}
	if !strings.Contains(ae.Spans[1].Label, "the declaration says `f` returns String") ||
		ae.Spans[1].Pos.Row != 1 {
		t.Errorf("decl span = %+v", ae.Spans[1])
	}
	rendered := ae.Error()
	if !strings.Contains(rendered, "------ the declaration says `f` returns String") {
		t.Errorf("declaration span must render with its excerpt:\n%s", rendered)
	}
}

func TestReturnTypeErrorSpanNegatives(t *testing.T) {
	e := engWithTape(t, nil, 0)
	// No declaration site, and the value sits AT the error position:
	// neither span attaches.
	rc := ReturnCheckInfo{FuncName: "f", Pos: SrcPos{Row: 2, Col: 4}}
	got := NewInteger(42)
	got.pos = &SrcPos{Row: 2, Col: 4}
	ae := e.returnTypeError(rc, 1, TString, got)
	if len(ae.Spans) != 0 {
		t.Errorf("no spans expected, got %+v", ae.Spans)
	}
	// A positionless value attaches no value span either.
	ae = e.returnTypeError(rc, 1, TString, NewInteger(42))
	if len(ae.Spans) != 0 {
		t.Errorf("positionless value must attach no span, got %+v", ae.Spans)
	}
}

func TestReturnCountErrorDeclSpan(t *testing.T) {
	e := engWithTape(t, nil, 0)
	rc := ReturnCheckInfo{
		FuncName: "f",
		Pos:      SrcPos{Row: 2, Col: 4},
		Decl:     DeclSite{Pos: SrcPos{Row: 1, Col: 23}, Source: "src", File: "mod.aql"},
	}
	ae := e.returnCountError(rc, 2, 1)
	if len(ae.Spans) != 1 {
		t.Fatalf("want the decl span, got %+v", ae.Spans)
	}
	sp := ae.Spans[0]
	if !strings.Contains(sp.Label, "expects 2 return value(s)") ||
		sp.Source != "src" || sp.File != "mod.aql" {
		t.Errorf("decl span = %+v", sp)
	}
	// Zero declaration → no span.
	rc.Decl = DeclSite{}
	if ae := e.returnCountError(rc, 2, 1); len(ae.Spans) != 0 {
		t.Errorf("zero decl must attach nothing, got %+v", ae.Spans)
	}
}
