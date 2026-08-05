package core

// Seam-8 (cluster W8_eng_rest): in-package unit tests for previously-
// unreached branches in fn_capture.go — the Reach computed-key walk, the
// XML-template attribute / nested-child walk, and the ComputeCaptures nil
// guard. Per design/TEST-SEAMS.10.md.

import "testing"

func w8collectWords(walk func(func(WordInfo, Value))) map[string]bool {
	seen := map[string]bool{}
	walk(func(w WordInfo, _ Value) { seen[w.Name] = true })
	return seen
}

func TestW8WalkBodyWordsReachComputedKey(t *testing.T) {
	reach := NewReach(ReachInfo{
		Receiver: []Value{NewWord("recv")},
		Segments: []ReachSeg{{Computed: true, KeyExpr: []Value{NewWord("keyw")}}},
	})
	seen := w8collectWords(func(cb func(WordInfo, Value)) {
		WalkBodyWords([]Value{reach}, cb)
	})
	if !seen["recv"] || !seen["keyw"] {
		t.Fatalf("reach walk must see receiver and computed key words, saw %v", seen)
	}
}

func TestW8WalkXmlTmplExprs(t *testing.T) {
	tmpl := XmlTmpl{
		Attr: []XmlAttrTmpl{{Name: "a", Parts: []InterpPart{{Expr: []Value{NewWord("attrw")}}}}},
		Cren: []XmlCren{
			{Kind: XmlCrenExpr, Expr: []Value{NewWord("exprw")}},
			{Kind: XmlCrenChild, Child: &XmlTmpl{
				Cren: []XmlCren{{Kind: XmlCrenExpr, Expr: []Value{NewWord("childw")}}},
			}},
		},
	}
	seen := w8collectWords(func(cb func(WordInfo, Value)) {
		walkXmlTmplExprs(tmpl, cb, nil)
	})
	for _, w := range []string{"attrw", "exprw", "childw"} {
		if !seen[w] {
			t.Fatalf("xml walk must see %q, saw %v", w, seen)
		}
	}
}

func TestW8ComputeCapturesNilRegistry(t *testing.T) {
	if got := ComputeCaptures(nil, &FnSig{}); got != nil {
		t.Fatalf("nil registry yields no captures, got %v", got)
	}
}
