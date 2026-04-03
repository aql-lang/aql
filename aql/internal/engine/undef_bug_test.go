package engine

import "testing"

func TestUndefBugNamedStringParams(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	// def joiner fn [[a:string b:string c:string] [string] [a b add c add]] end
	// Named string params: body concatenates a+b+c via add.
	// All prefix: "A" "B" "C" joiner → nearest to joiner is "C"→sig[0](a),
	// "B"→sig[1](b), "A"→sig[2](c). Body: a b add c add → "CB" + "A" → "CBA".
	aParam := NewOrderedMap()
	aParam.Set("a", NewWord("String"))
	bParam := NewOrderedMap()
	bParam.Set("b", NewWord("String"))
	cParam := NewOrderedMap()
	cParam.Set("c", NewWord("String"))

	fnBody := NewList([]Value{
		NewList([]Value{NewImplicitMap(aParam), NewImplicitMap(bParam), NewImplicitMap(cParam)}),
		NewList([]Value{NewWord("String")}),
		NewList([]Value{NewWord("a"), NewWord("b"), NewWord("add"), NewWord("c"), NewWord("add")}),
	})
	tokens := []Value{
		NewWord("def"), NewWord("joiner"), NewWord("fn"), fnBody, NewWord("end"),
		NewString("A"), NewString("B"), NewString("C"), NewWord("joiner"),
	}
	result, err := NewTop(r).Run(tokens)
	if err != nil {
		t.Fatalf("bug confirmed: %v", err)
	}
	_as0, _ := result[0].AsString()
	if len(result) != 1 || _as0 != "CBA" {
		t.Errorf("got %v, want [CBA]", result)
	}
}
